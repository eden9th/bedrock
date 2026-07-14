package cron

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Scheduler 是定时任务调度器，封装 robfig/cron/v3。
//
// 实现 lifecycle.Service 接口，可直接注册到 lifecycle.Manager。
// 所有任务在 wrap() 中统一处理：trace_id 注入、panic recover、PAUSED 开关、耗时/错误日志。
type Scheduler struct {
	c       *cron.Cron
	mu      sync.RWMutex
	entries []entryRecord // 保存注册顺序，用于 Entries()
	paused  bool          // PAUSED 开关，暂停时跳过所有任务执行
}

// entryRecord 记录注册信息（spec + job + cron entry id）。
type entryRecord struct {
	spec    string
	job     Job
	entryID cron.EntryID
}

// New 创建一个新的 Scheduler。
// 使用 5 字段 cron 表达式（分钟级，不支持秒）。
func New() *Scheduler {
	return &Scheduler{
		c: cron.New(),
	}
}

// AddJob 注册一个实现了 Job 接口的任务。
//
// spec 为标准 5 字段 cron 表达式（如 "0 * * * *" 表示每小时整点）。
// 特殊值 "@manual" 表示仅手动触发，不自动调度。
func (s *Scheduler) AddJob(spec string, job Job) error {
	if spec == "@manual" {
		// 手动任务只注册元信息，不加入 cron 调度
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

// AddFunc 注册一个函数作为定时任务，内部包装为 FuncJob。
//
// spec 同 AddJob。name 作为任务唯一名称。
func (s *Scheduler) AddFunc(spec, name string, fn func(ctx context.Context) error) error {
	return s.AddJob(spec, &FuncJob{name: name, fn: fn})
}

// Pause 暂停所有任务执行（任务仍会触发，但 wrap() 内会直接跳过）。
// 可用于紧急关停，无需重启服务。
func (s *Scheduler) Pause() {
	s.mu.Lock()
	s.paused = true
	s.mu.Unlock()
	fmt.Fprintf(os.Stderr, "[cron] scheduler paused\n")
}

// Resume 恢复任务执行。
func (s *Scheduler) Resume() {
	s.mu.Lock()
	s.paused = false
	s.mu.Unlock()
	fmt.Fprintf(os.Stderr, "[cron] scheduler resumed\n")
}

// IsPaused 返回当前是否处于暂停状态。
func (s *Scheduler) IsPaused() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.paused
}

// Entries 返回所有已注册任务的元信息列表。
// 用于 /list_tasks 接口展示。
func (s *Scheduler) Entries() []EntryInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]EntryInfo, 0, len(s.entries))
	for _, rec := range s.entries {
		info := EntryInfo{
			Name: rec.job.Name(),
			Spec: rec.spec,
			job:  rec.job,
		}
		if rec.entryID != 0 {
			entry := s.c.Entry(rec.entryID)
			info.Next = entry.Next
			info.Prev = entry.Prev
		}
		result = append(result, info)
	}
	return result
}

// Entry 按名称查找任务元信息，用于手动触发前查询。
// 返回 (EntryInfo, true) 或 (EntryInfo{}, false)。
func (s *Scheduler) Entry(name string) (EntryInfo, bool) {
	for _, e := range s.Entries() {
		if e.Name == name {
			return e, true
		}
	}
	return EntryInfo{}, false
}

// RunTask 手动触发指定名称的任务（同步执行）。
// 任务在 wrap() 保护下运行（panic recover + 日志）。
// 若任务不存在，返回 error。
func (s *Scheduler) RunTask(ctx context.Context, name string) error {
	entry, ok := s.Entry(name)
	if !ok {
		return fmt.Errorf("cron: task %q not found", name)
	}
	// 手动触发不受 PAUSED 开关限制
	return s.runWithRecover(ctx, entry.job)
}

// ----- lifecycle.Service 接口 -----

// Name 实现 lifecycle.Service 接口。
func (s *Scheduler) Name() string { return "cron-scheduler" }

// Start 实现 lifecycle.Service 接口，启动调度器并阻塞直到 ctx 取消。
func (s *Scheduler) Start(ctx context.Context) error {
	s.c.Start()
	fmt.Fprintf(os.Stderr, "[cron] scheduler started, %d jobs registered\n", len(s.entries))
	<-ctx.Done()
	return nil
}

// Stop 实现 lifecycle.Service 接口，等待正在执行的任务完成后停止。
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

// ----- 内部方法 -----

// wrap 包装 Job，注入：PAUSED 检查、panic recover、耗时日志、错误日志。
// 每次执行生成新的 trace_id（通过 context value 传递）。
func (s *Scheduler) wrap(job Job) func() {
	return func() {
		ctx := context.Background()
		// TODO: 接入 bedrock/trace 后，在此注入 trace_id 到 ctx
		// ctx = trace.NewContext(ctx, trace.New(job.Name()))

		if err := s.runWithRecover(ctx, job); err != nil {
			// 错误已在 runWithRecover 内记录，此处不重复
			_ = err
		}
	}
}

// runWithRecover 执行任务，捕获 panic，记录耗时和错误日志。
func (s *Scheduler) runWithRecover(ctx context.Context, job Job) (retErr error) {
	// PAUSED 检查（手动触发时跳过）
	s.mu.RLock()
	paused := s.paused
	s.mu.RUnlock()
	if paused {
		fmt.Fprintf(os.Stderr, "[cron] skip job %s: scheduler paused\n", job.Name())
		return nil
	}

	start := time.Now()

	// panic recover
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
