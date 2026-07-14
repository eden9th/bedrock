package bm

import (
	"net/http"
	"strconv"
	"strings"
)

// CORSConfig 定义 CORS（跨域资源共享）中间件的配置。
// 零值不可用，使用 DefaultCORS 或 CORSAllowAll。
type CORSConfig struct {
	// AllowOrigins 允许的源列表。设置 "*" 允许所有源。
	// 注意：AllowCredentials=true 时不能使用 "*"。
	AllowOrigins []string

	// AllowMethods 允许的 HTTP 方法。默认为 GET,POST,PUT,DELETE,OPTIONS。
	AllowMethods []string

	// AllowHeaders 允许的请求头。默认为 Origin,Content-Type,Authorization。
	AllowHeaders []string

	// ExposeHeaders 暴露给客户端的响应头。
	ExposeHeaders []string

	// AllowCredentials 是否允许携带 cookie / Authorization header。
	AllowCredentials bool

	// MaxAge 预检请求（OPTIONS）的缓存时间（秒）。默认 3600（1 小时）。
	MaxAge int
}

// DefaultCORS 返回一个适合开发环境的 CORS 配置：
// 允许 localhost，常用方法和头。
func DefaultCORS() CORSConfig {
	return CORSConfig{
		AllowOrigins:     []string{"http://localhost:3000", "http://localhost:8080"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: false,
		MaxAge:           3600,
	}
}

// CORSAllowAll 返回允许所有源的 CORS 配置（仅适合开发/内部服务）。
func CORSAllowAll() CORSConfig {
	return CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders: []string{"*"},
		MaxAge:       3600,
	}
}

// CORS 返回一个 CORS 中间件，处理跨域请求。
//
// 行为：
//   - 所有请求设置 Access-Control-Allow-* 响应头
//   - OPTIONS 预检请求返回 204 No Content，不执行后续 handler
//   - 非预检请求正常执行 handler 链
//
// 使用：
//
//	e.Use(bm.CORS(bm.DefaultCORS()))     // 所有路由
//	api := e.Group("/api", bm.CORS(bm.DefaultCORS()))
func CORS(cfg CORSConfig) HandlerFunc {
	allowOrigin := strings.Join(cfg.AllowOrigins, ", ")
	allowMethods := strings.Join(cfg.AllowMethods, ", ")
	allowHeaders := strings.Join(cfg.AllowHeaders, ", ")
	exposeHeaders := strings.Join(cfg.ExposeHeaders, ", ")
	maxAge := strconv.Itoa(cfg.MaxAge)

	return func(c *Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		// 设置通用 CORS 头
		if cfg.AllowCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
			// AllowCredentials 时不能使用通配符，必须反射具体的 Origin
			if cfg.AllowOrigins[0] == "*" {
				c.Header("Access-Control-Allow-Origin", origin)
			} else {
				c.Header("Access-Control-Allow-Origin", matchOrigin(cfg.AllowOrigins, origin))
			}
		} else {
			c.Header("Access-Control-Allow-Origin", allowOrigin)
		}

		c.Header("Access-Control-Allow-Methods", allowMethods)
		c.Header("Access-Control-Allow-Headers", allowHeaders)
		c.Header("Access-Control-Max-Age", maxAge)

		if len(cfg.ExposeHeaders) > 0 {
			c.Header("Access-Control-Expose-Headers", exposeHeaders)
		}

		// OPTIONS 预检请求直接返回 204，不执行后续 handler
		if c.Request.Method == http.MethodOptions {
			c.Writer.WriteHeader(http.StatusNoContent)
			c.Abort()
			return
		}

		c.Next()
	}
}

// matchOrigin 检查 origin 是否在允许列表中。是则返回 origin，否则返回空字符串。
func matchOrigin(allowed []string, origin string) string {
	for _, a := range allowed {
		if a == "*" || strings.EqualFold(a, origin) {
			return origin
		}
	}
	return ""
}
