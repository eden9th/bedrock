package cron_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eden9th/bedrock/cron"
)

// ----- 辅助 -----

// countJob 每次 Run 递增计数器，用于验证调度。
type countJob struct {
	name  string
	count atomic.Int64
	err   error // 非 nil 时 Run 返回此错误
}

func (j *countJob) Name() string { return j.name }
func (j *countJob) Run(_ context.Context) error {
	j.count.Add(1)
	return j.err
}

// ----- AddJob / AddFunc -----

func TestAddJob(t *testing.T) {
	s := cron.New()
	job := &countJob{name: "test-job"}
	if err := s.AddJob("* * * * *", job); err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}
	entries := s.Entries()
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "test-job" {
		t.Fatalf("want Name=test-job, got %s", entries[0].Name)
	}
	if entries[0].Spec != "* * * * *" {
		t.Fatalf("want Spec=* * * * *, got %s", entries[0].Spec)
	}
}

func TestAddFunc(t *testing.T) {
	s := cron.New()
	if err := s.AddFunc("0 * * * *", "hourly", func(_ context.Context) error { return nil }); err != nil {
		t.Fatalf("AddFunc failed: %v", err)
	}
	entries := s.Entries()
	if len(entries) != 1 || entries[0].Name != "hourly" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestAddJobInvalidSpec(t *testing.T) {
	s := cron.New()
	err := s.AddJob("not-a-cron-spec", &countJob{name: "bad"})
	if err == nil {
		t.Fatal("want error for invalid spec, got nil")
	}
}

// ----- @manual 任务 -----

func TestManualJob(t *testing.T) {
	s := cron.New()
	job := &countJob{name: "manual-job"}
	if err := s.AddJob("@manual", job); err != nil {
		t.Fatalf("AddJob @manual failed: %v", err)
	}
	entries := s.Entries()
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].Spec != "@manual" {
		t.Fatalf("want Spec=@manual, got %s", entries[0].Spec)
	}
}

// ----- Entry / RunTask -----

func TestEntry(t *testing.T) {
	s := cron.New()
	_ = s.AddJob("@manual", &countJob{name: "find-me"})

	entry, ok := s.Entry("find-me")
	if !ok {
		t.Fatal("want found=true")
	}
	if entry.Name != "find-me" {
		t.Fatalf("want Name=find-me, got %s", entry.Name)
	}

	_, ok = s.Entry("not-exist")
	if ok {
		t.Fatal("want found=false for non-existent entry")
	}
}

func TestRunTask(t *testing.T) {
	s := cron.New()
	job := &countJob{name: "run-me"}
	_ = s.AddJob("@manual", job)

	if err := s.RunTask(context.Background(), "run-me"); err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if job.count.Load() != 1 {
		t.Fatalf("want count=1, got %d", job.count.Load())
	}
}

func TestRunTaskNotFound(t *testing.T) {
	s := cron.New()
	err := s.RunTask(context.Background(), "no-such-task")
	if err == nil {
		t.Fatal("want error for unknown task")
	}
}

func TestRunTaskError(t *testing.T) {
	s := cron.New()
	job := &countJob{name: "err-job", err: errors.New("task failed")}
	_ = s.AddJob("@manual", job)

	err := s.RunTask(context.Background(), "err-job")
	if err == nil {
		t.Fatal("want error propagated from job.Run")
	}
	if job.count.Load() != 1 {
		t.Fatalf("want count=1, got %d", job.count.Load())
	}
}

// ----- Panic recover -----

type panicJob struct{}

func (p *panicJob) Name() string { return "panic-job" }
func (p *panicJob) Run(_ context.Context) error {
	panic("intentional panic")
}

func TestRunTaskPanicRecover(t *testing.T) {
	s := cron.New()
	_ = s.AddJob("@manual", &panicJob{})

	// RunTask 应该 recover panic 并返回 error，而不是让进程崩溃
	err := s.RunTask(context.Background(), "panic-job")
	if err == nil {
		t.Fatal("want error after panic recovery")
	}
}

// ----- Pause / Resume -----

func TestPauseResume(t *testing.T) {
	s := cron.New()

	s.Pause()
	if !s.IsPaused() {
		t.Fatal("want IsPaused=true after Pause()")
	}

	// 手动触发（RunTask）不受 PAUSED 限制：PAUSED 只阻断自动调度路径
	manualJob := &countJob{name: "manual-during-pause"}
	_ = s.AddJob("@manual", manualJob)
	if err := s.RunTask(context.Background(), "manual-during-pause"); err != nil {
		t.Fatalf("RunTask during pause should succeed (manual trigger bypasses PAUSED), got %v", err)
	}
	if manualJob.count.Load() != 1 {
		t.Fatalf("manual trigger during pause: want count=1, got %d", manualJob.count.Load())
	}

	s.Resume()
	if s.IsPaused() {
		t.Fatal("want IsPaused=false after Resume()")
	}

	// 恢复后手动触发同样正常
	resumeJob := &countJob{name: "manual-after-resume"}
	_ = s.AddJob("@manual", resumeJob)
	if err := s.RunTask(context.Background(), "manual-after-resume"); err != nil {
		t.Fatalf("RunTask after resume failed: %v", err)
	}
	if resumeJob.count.Load() != 1 {
		t.Fatalf("manual trigger after resume: want count=1, got %d", resumeJob.count.Load())
	}
}

// TestPauseBlocksAutoScheduled 验证 PAUSED 只阻断自动调度路径（robfig/cron 触发）。
// 手动触发（RunTask）不受影响——已在 TestPauseResume 中验证。
func TestPauseBlocksAutoScheduled(t *testing.T) {
	s := cron.New()
	job := &countJob{name: "auto-paused"}
	_ = s.AddJob("@every 100ms", job)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		_ = s.Stop(stopCtx)
	}()

	go func() { _ = s.Start(ctx) }()
	waitUntil(t, time.Second, func() bool { return job.count.Load() > 0 })

	s.Pause()
	countBeforePause := job.count.Load()
	time.Sleep(300 * time.Millisecond)

	// PAUSED 期间自动调度应跳过
	if job.count.Load() != countBeforePause {
		t.Fatalf("auto job during pause: count should not increase, was=%d now=%d", countBeforePause, job.count.Load())
	}

	s.Resume()
	waitUntil(t, time.Second, func() bool { return job.count.Load() > countBeforePause })
}

// ----- lifecycle Start/Stop -----

func TestStartStop(t *testing.T) {
	s := cron.New()
	_ = s.AddJob("@manual", &countJob{name: "bg-job"})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- s.Start(ctx)
	}()

	// 给调度器时间启动
	time.Sleep(20 * time.Millisecond)

	// 触发关闭
	cancel()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopCancel()

	if err := s.Stop(stopCtx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}
}

func TestName(t *testing.T) {
	s := cron.New()
	if s.Name() != "cron-scheduler" {
		t.Fatalf("want Name=cron-scheduler, got %s", s.Name())
	}
}

func waitUntil(t *testing.T, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
