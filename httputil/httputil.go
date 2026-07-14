// Package httputil 提供 bm handler 常用工具：JSON 响应封装、请求体解析。
// JSON 输出裸数据（不包信封），与前端约定保持一致。
// 错误响应输出 {"detail": "..."} 格式，与前端 fmtApiError 约定保持一致。
package httputil

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/eden9th/bedrock/bm"
)

// DefaultMaxBodySize 是 Bind 默认的最大请求体大小（1MB）。
const DefaultMaxBodySize = 1 << 20

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

// Bind 将请求体 JSON 解析到 v，使用默认 1MB body 大小限制。
// 解析失败时自动返回 400 并中止，返回 false 表示失败，handler 应直接 return。
func Bind[T any](c *bm.Context, v *T) bool {
	return BindWithLimit(c, v, DefaultMaxBodySize)
}

// BindWithLimit 将请求体 JSON 解析到 v，指定最大 body 字节数。
// maxBytes 为 0 时不限制大小（仅用于信任的内部接口）。
func BindWithLimit[T any](c *bm.Context, v *T, maxBytes int64) bool {
	var reader io.Reader
	if maxBytes > 0 {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	}
	reader = c.Request.Body
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(v); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			JSONError(c, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		JSONError(c, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			JSONError(c, http.StatusBadRequest, "invalid request body: multiple JSON values")
			return false
		}
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			JSONError(c, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		JSONError(c, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}

// FieldError 表示单个字段的校验错误。
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// BindAndValidate 将请求体 JSON 解析到 v，然后执行校验函数。
// 校验失败时返回 422，body 为 {"errors": [{"field":"name","message":"必填"}]}。
//
// 使用示例：
//
//	type Req struct { Name string `json:"name"` }
//	var r Req
//	if !httputil.BindAndValidate(c, &r, func(r Req) []httputil.FieldError {
//	    var errs []httputil.FieldError
//	    if r.Name == "" { errs = append(errs, httputil.FieldError{Field:"name",Message:"必填"}) }
//	    return errs
//	}) {
//	    return
//	}
func BindAndValidate[T any](c *bm.Context, v *T, validate func(T) []FieldError) bool {
	if !Bind(c, v) {
		return false
	}
	errs := validate(*v)
	if len(errs) > 0 {
		c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		c.Writer.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(c.Writer).Encode(map[string]any{"errors": errs})
		c.Abort()
		return false
	}
	return true
}

// isSizeExceeded 判断是否为 io.LimitReader 触达上限导致的错误。
