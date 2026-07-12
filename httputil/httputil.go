// Package httputil 提供 bm handler 常用工具：JSON 响应封装、请求体解析。
// JSON 输出裸数据（不包信封），与前端约定保持一致。
// 错误响应输出 {"detail": "..."} 格式，与前端 fmtApiError 约定保持一致。
package httputil

import (
	"encoding/json"
	"net/http"

	"github.com/eden9th/bedrock/bm"
)

// JSON 输出裸 JSON，不带任何包装层，HTTP 200。
func JSON(c *bm.Context, data any) {
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Writer.WriteHeader(http.StatusOK)
	json.NewEncoder(c.Writer).Encode(data)
}

// JSONError 输出 {"detail": "..."} 格式的错误响应并中止 handler 链。
func JSONError(c *bm.Context, status int, detail string) {
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Writer.WriteHeader(status)
	json.NewEncoder(c.Writer).Encode(map[string]string{"detail": detail})
	c.Abort()
}

// Bind 将请求体 JSON 解析到 v。
// 解析失败时自动返回 400 并中止，返回 false 表示失败，handler 应直接 return。
func Bind[T any](c *bm.Context, v *T) bool {
	if err := json.NewDecoder(c.Request.Body).Decode(v); err != nil {
		JSONError(c, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}
