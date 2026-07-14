// Package app 提供统一的应用生命周期管理——信号处理、优雅关闭、服务注册。
//
// 使用示例：
//
//	a := app.New(app.Config{ConfDir: "configs", Name: "myapp"})
//	utilconf.Init(a.ConfDir())
//	cfg := initConfig()
//	svc := service.New(cfg)
//	frontend, admin := serverhttp.Init(cfg, svc)
//	a.RegisterStop("frontend", frontend.Shutdown)
//	a.RegisterStop("admin", admin.Shutdown)
//	a.Run()
package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Config 应用启动配置。
type Config struct {
	ConfDir       string        // TOML 配置目录路径
	Name          string        // 应用名称（日志标识）
	ShutdownDelay time.Duration // 收到信号到开始关闭的等待时间，默认 0
	StopTimeout   time.Duration // 每个服务停止的超时时间，默认 10s
}

// App 管理应用生命周期——注册的服务按 LIFO 顺序在收到 SIGTERM/SIGINT 后优雅关闭。
type App struct {
	name     string
	confDir  string
	delay    time.Duration
	timeout  time.Duration
	stoppers []stopper
}

type stopper struct {
	name string
	fn   func(ctx context.Context) error
}

// New 创建应用实例。
func New(cfg Config) *App {
	timeout := cfg.StopTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &App{
		name:    cfg.Name,
		confDir: cfg.ConfDir,
		delay:   cfg.ShutdownDelay,
		timeout: timeout,
	}
}

// ConfDir 返回配置目录路径。
func (a *App) ConfDir() string { return a.confDir }

// RegisterStop 注册一个服务的关闭函数。关闭时按注册逆序调用。
func (a *App) RegisterStop(name string, fn func(ctx context.Context) error) {
	a.stoppers = append(a.stoppers, stopper{name: name, fn: fn})
}

// Run 阻塞等待 SIGTERM/SIGINT，收到信号后按注册逆序优雅关闭所有服务。
// 不在服务列表中的 goroutine 需要在业务代码中自行管理。
func (a *App) Run() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)

	s := <-c
	fmt.Fprintf(os.Stderr, "[%s] received signal %s, shutting down...\n", a.name, s)
	if a.delay > 0 {
		time.Sleep(a.delay)
	}

	// 逆序关闭
	for i := len(a.stoppers) - 1; i >= 0; i-- {
		st := a.stoppers[i]
		ctx, cancel := context.WithTimeout(context.Background(), a.timeout)

		fmt.Fprintf(os.Stderr, "[%s] stopping %s...\n", a.name, st.name)
		if err := st.fn(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] stop %s: %v\n", a.name, st.name, err)
		} else {
			fmt.Fprintf(os.Stderr, "[%s] %s stopped\n", a.name, st.name)
		}
		cancel()
	}

	fmt.Fprintf(os.Stderr, "[%s] shutdown complete\n", a.name)
}
