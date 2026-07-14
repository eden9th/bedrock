package bm

import (
	"net/http"
	"net/http/pprof"
)

// RegisterPprof 在 engine 上注册标准 /debug/pprof 路由。
// 提供 CPU profile、heap dump、goroutine dump 等标准运维端点。
//
// 使用：
//
//	e := bm.New()
//	bm.RegisterPprof(e) // 注册 /debug/pprof/*
//
// 注册的路由：
//
//	/debug/pprof/             — 索引页
//	/debug/pprof/cmdline      — 进程启动命令行
//	/debug/pprof/profile      — CPU profile（?seconds=30）
//	/debug/pprof/symbol       — 符号查找
//	/debug/pprof/trace        — 执行追踪
//	/debug/pprof/allocs       — 内存分配采样
//	/debug/pprof/block        — 阻塞分析
//	/debug/pprof/goroutine    — Goroutine 堆栈
//	/debug/pprof/heap         — 堆内存采样
//	/debug/pprof/mutex        — 互斥锁竞争
//	/debug/pprof/threadcreate — 线程创建分析
func RegisterPprof(e *Engine) {
	g := e.Group("/debug/pprof")
	g.GET("/", pprofHandler(pprof.Index))
	g.GET("/cmdline", pprofHandler(pprof.Cmdline))
	g.GET("/profile", pprofHandler(pprof.Profile))
	g.POST("/symbol", pprofHandler(pprof.Symbol))
	g.GET("/symbol", pprofHandler(pprof.Symbol))
	g.GET("/trace", pprofHandler(pprof.Trace))
	g.GET("/allocs", pprofHandler(pprof.Handler("allocs").ServeHTTP))
	g.GET("/block", pprofHandler(pprof.Handler("block").ServeHTTP))
	g.GET("/goroutine", pprofHandler(pprof.Handler("goroutine").ServeHTTP))
	g.GET("/heap", pprofHandler(pprof.Handler("heap").ServeHTTP))
	g.GET("/mutex", pprofHandler(pprof.Handler("mutex").ServeHTTP))
	g.GET("/threadcreate", pprofHandler(pprof.Handler("threadcreate").ServeHTTP))
}

func pprofHandler(h http.HandlerFunc) HandlerFunc {
	return func(c *Context) {
		h(c.Writer, c.Request)
	}
}
