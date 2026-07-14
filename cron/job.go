// Package cron 提供定时任务调度器，封装 robfig/cron/v3。
//
// # 核心设计
//
// 每个任务实现 Job 接口（Name/Run），注册到 Scheduler。
// wrap() 在执行前后注入：新建 trace_id、panic recover、PAUSED 开关检查、
// 耗时日志、错误日志。
//
// Scheduler 实现 lifecycle.Service 接口，可直接注册到 lifecycle.Manager。
//
// # 使用示例
//
//	s := cron.New()
//
//	// 注册任务（支持标准 cron 表达式，5字段，分钟级）
//	s.AddFunc("0 * * * *", "hourly-cleanup", func(ctx context.Context) error {
//	    return cleanup(ctx)
//	})
//
//	// 或者注册实现了 Job 接口的结构体
//	s.AddJob("*/5 * * * *", &MyJob{})
//
//	// 接入 lifecycle 管理
//	m := lifecycle.New()
//	m.Register(s)
//	m.Run()
//
// # 手动触发（用于 admin 接口）
//
//	entry, ok := s.Entry("hourly-cleanup")
//	if ok {
//	    entry.Job.Run(ctx)
//	}
package cron

import (
	"context"
	"time"
)

// Job 是定时任务接口。
type Job interface {
	// Name 返回任务唯一名称（用于日志和手动触发）。
	Name() string
	// Run 执行任务。ctx 在 Scheduler 关闭时被取消。
	Run(ctx context.Context) error
}

// FuncJob 将普通函数包装为 Job 接口。
type FuncJob struct {
	name string
	fn   func(ctx context.Context) error
}

// Name 实现 Job 接口。
func (f *FuncJob) Name() string { return f.name }

// Run 实现 Job 接口。
func (f *FuncJob) Run(ctx context.Context) error { return f.fn(ctx) }

// EntryInfo 是已注册任务的元信息，用于 ListTasks / RunTask 接口。
type EntryInfo struct {
	// Name 是任务名称。
	Name string
	// Spec 是 cron 表达式（manual 任务为 "@manual"）。
	Spec string
	// Next 是下次执行时间（manual 任务为零值）。
	Next time.Time
	// Prev 是上次执行时间（尚未执行过则为零值）。
	Prev time.Time
	// job 用于手动触发时调用，不导出。
	job Job
}
