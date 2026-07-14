// Package lifecycle 提供服务生命周期管理：有序启动、信号驱动的优雅关闭。
//
// # 核心设计
//
// 每个组件实现 Service 接口（Start / Stop），注册到 Manager。
// Manager 按注册顺序启动，收到 SIGTERM/SIGINT 后按逆序关闭。
//
// # 使用示例
//
//	m := lifecycle.New()
//	m.Register(httpserver)
//	m.Register(dbservice)
//
//	// 阻塞直到信号到达，然后有序关闭
//	if err := m.Run(); err != nil {
//	    log.Fatal(err)
//	}
package lifecycle

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Service 是受生命周期管理的组件接口。
type Service interface {
	// Name 返回服务名称（用于日志）。
	Name() string
	// Start 启动服务。应阻塞直到服务就绪或出错。
	// ctx 在 Manager 关闭时被取消，服务应据此退出。
	Start(ctx context.Context) error
	// Stop 停止服务。应在 d 时间内完成清理。
	// ctx 带有超时，服务应据此放弃等待。
	Stop(ctx context.Context) error
}

// Manager 管理一组 Service 的生命周期。
type Manager struct {
	services []Service

	// ShutdownTimeout 是每个 service 的 Stop 超时时间。默认 10s。
	ShutdownTimeout time.Duration
}

// New 创建一个空的 Manager。
func New() *Manager {
	return &Manager{
		ShutdownTimeout: 10 * time.Second,
	}
}

// Register 注册一个服务。按注册顺序启动，逆序关闭。
func (m *Manager) Register(svc Service) {
	m.services = append(m.services, svc)
}

// Run 启动所有已注册服务，阻塞直到收到 SIGTERM/SIGINT，
// 然后按注册逆序优雅关闭所有服务。
//
// 返回第一个启动失败或关闭失败的 error。
func (m *Manager) Run() error {
	if len(m.services) == 0 {
		return fmt.Errorf("lifecycle: no services registered")
	}

	// 1. 顺序启动
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, len(m.services))
	var wg sync.WaitGroup

	for _, svc := range m.services {
		wg.Add(1)
		go func(s Service) {
			defer wg.Done()
			if err := s.Start(ctx); err != nil {
				errCh <- fmt.Errorf("lifecycle: %s start: %w", s.Name(), err)
			}
		}(svc)
	}

	// 2. 等待信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		fmt.Fprintf(os.Stderr, "[lifecycle] received signal %v, shutting down...\n", sig)
	case err := <-errCh:
		cancel()
		return err
	}

	// 3. 逆序关闭
	cancel() // 通知所有 Start goroutine 退出
	wg.Wait()

	for i := len(m.services) - 1; i >= 0; i-- {
		svc := m.services[i]
		stopCtx, stopCancel := context.WithTimeout(context.Background(), m.ShutdownTimeout)
		fmt.Fprintf(os.Stderr, "[lifecycle] stopping %s...\n", svc.Name())
		if err := svc.Stop(stopCtx); err != nil {
			fmt.Fprintf(os.Stderr, "[lifecycle] %s stop error: %v\n", svc.Name(), err)
		}
		stopCancel()
	}

	return nil
}
