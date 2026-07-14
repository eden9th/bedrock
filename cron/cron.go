package cron

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/eden9th/bedrock/trace"
	"github.com/robfig/cron/v3"
)

// Scheduler 是定时任务调度器，封装 robfig/cron/v3。
//
// 实现 lifecycle.Service 接口，可直接注册到 lifecycle.Manager。
// 自动触发任务在 wrap() 中统一处理：PAUSED 开关、trace_id 注入、panic recover、耗时/错误日志。
// 手动触发（RunTask）跳过 PAUSED 检查，但仍享受 panic recover + 日志保护。
type Scheduler struct {
	c       *cron.Cron
	mu      sync.RWMutex
	entries []entryRecord
	paused  bool
}

type entryRecord struct {
	spec    string
	job     Job
	entryID cron.EntryID
}

func New() *Scheduler {
	return &Scheduler{c: cron.New()}
}

func (s *Scheduler) AddJob(spec string, job Job) error {
	if spec == "@manual" {
		s.mu.Lock()
		s.entries = append(s.entries, entryRecord{spec: spec, job: job, entryID: 0})
		s.mu.Unlock()
		return nil
	}

	id, err := s.c.AddFunc(spec, s.wrap(job))
	if err != nil {
		return fmt.Errorf("cron: AddJob %q spec=%q: %w", job.Name(), spec, err)
	}

	s.mu.Lock()
	s.entries = append(s.entries, entryRecord{spec: spec, job: job, entryID: id})
	s.mu.Unlock()
	return nil
}

func (s *Scheduler) AddFunc(spec, name string, fn func(ctx context.Context) error) error {
	return s.AddJob(spec, &FuncJob{name: name, fn: fn})
}

func (s *Scheduler) Pause() {
	s.mu.Lock()
	s.paused = true
	s.mu.Unlock()
	fmt.Fprintf(os.Stderr, "[cron] scheduler paused\n")
}

func (s *Scheduler) Resume() {
	s.mu.Lock()
	s.paused = false
	s.mu.Unlock()
	fmt.Fprintf(os.Stderr, "[cron] scheduler resumed\n")
}

func (s *Scheduler) IsPaused() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.paused
}

func (s *Scheduler) Entries() []EntryInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]EntryInfo, 0, len(s.entries))
	for _, rec := range s.entries {
		info := EntryInfo{Name: rec.job.Name(), Spec: rec.spec, job: rec.job}
		if rec.entryID != 0 {
			entry := s.c.Entry(rec.entryID)
			info.Next = entry.Next
			info.Prev = entry.Prev
		}
		result = append(result, info)
	}
	return result
}

func (s *Scheduler) Entry(name string) (EntryInfo, bool) {
	for _, e := range s.Entries() {
		if e.Name == name {
			return e, true
		}
	}
	return EntryInfo{}, false
}

// RunTask 手动触发指定名称的任务（同步执行）。
// 不受 PAUSED 开关限制。若 ctx 中无 trace_id，自动注入。
func (s *Scheduler) RunTask(ctx context.Context, name string) error {
	entry, ok := s.Entry(name)
	if !ok {
		return fmt.Errorf("cron: task %q not found", name)
	}
	if trace.GetTraceID(ctx) == "" {
		ctx = trace.WithTraceID(ctx, trace.NewTraceID())
	}
	return s.runWithRecover(ctx, entry.job)
}

func (s *Scheduler) Name() string { return "cron-scheduler" }

func (s *Scheduler) Start(ctx context.Context) error {
	s.c.Start()
	fmt.Fprintf(os.Stderr, "[cron] scheduler started, %d jobs registered\n", len(s.entries))
	<-ctx.Done()
	return nil
}

func (s *Scheduler) Stop(ctx context.Context) error {
	stopCtx := s.c.Stop()
	select {
	case <-stopCtx.Done():
		fmt.Fprintf(os.Stderr, "[cron] scheduler stopped gracefully\n")
	case <-ctx.Done():
		fmt.Fprintf(os.Stderr, "[cron] scheduler stop timed out\n")
	}
	return nil
}

// wrap 仅用于自动调度路径：PAUSED 检查、trace_id 注入，然后进入 runWithRecover。
func (s *Scheduler) wrap(job Job) func() {
	return func() {
		s.mu.RLock()
		paused := s.paused
		s.mu.RUnlock()
		if paused {
			fmt.Fprintf(os.Stderr, "[cron] skip job %s: scheduler paused\n", job.Name())
			return
		}

		ctx := trace.WithTraceID(context.Background(), trace.NewTraceID())

		if err := s.runWithRecover(ctx, job); err != nil {
			_ = err
		}
	}
}

// runWithRecover 执行任务，捕获 panic，记录耗时和错误日志。
// 不检查 PAUSED——该职责由 wrap()（自动调度）和 RunTask（手动触发）各自决定。
func (s *Scheduler) runWithRecover(ctx context.Context, job Job) (retErr error) {
	start := time.Now()

	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic: %v", r)
			fmt.Fprintf(os.Stderr, "[cron] job %s panic: %v\nstack:\n%s\n",
				job.Name(), r, debug.Stack())
		}
		duration := time.Since(start)
		if retErr != nil {
			fmt.Fprintf(os.Stderr, "[cron] job %s error: %v, cost=%.3fs\n",
				job.Name(), retErr, duration.Seconds())
		} else {
			fmt.Fprintf(os.Stderr, "[cron] job %s done, cost=%.3fs\n",
				job.Name(), duration.Seconds())
		}
	}()

	return job.Run(ctx)
}
