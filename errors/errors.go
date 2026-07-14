// Package errors 提供统一的结构化错误类型，适用于 HTTP/gRPC 服务。
//
// # 核心类型
//
// AppError 是业务错误的载体，包含面向客户端的 Code/Message 和
// 仅用于内部 log 的 cause（不暴露给前端）。
//
// # 快速上手
//
//	// 业务错误
//	return errors.CodeError(10001, "活动已结束")
//
//	// 参数错误（HTTP 400）
//	return errors.InvalidArgument("user_id", "不能为空")
//
//	// 包装底层错误（保留 cause 用于 log）
//	return errors.Internal(err, "查询用户失败")
//
//	// 错误处理
//	var appErr *errors.AppError
//	if errors.As(err, &appErr) {
//	    c.JSON(appErr.HTTPStatus(), appErr)
//	}
package errors

import (
	"errors"
	"fmt"
)

// AppError 是结构化业务错误。
//
// Code/Message 面向客户端（可序列化到 HTTP 响应）。
// cause 仅用于内部日志，不序列化、不暴露给调用方。
type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	cause   error  // 内部原因，仅 log 用，不序列化
}

// Error 实现 error 接口。返回面向日志的完整描述。
func (e *AppError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("AppError(code=%d, msg=%q, cause=%v)", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("AppError(code=%d, msg=%q)", e.Code, e.Message)
}

// Unwrap 实现 errors.Unwrap，使 errors.Is/As 可以穿透到 cause。
func (e *AppError) Unwrap() error {
	return e.cause
}

// Cause 返回底层原因错误（用于 log）。
// 若不存在，返回 nil。
func (e *AppError) Cause() error {
	return e.cause
}

// WithCause 返回一个携带 cause 的新 AppError 副本。
// 用于在已有 AppError 上附加底层错误，不修改原对象。
func (e *AppError) WithCause(cause error) *AppError {
	return &AppError{Code: e.Code, Message: e.Message, cause: cause}
}

// CodeError 创建一个业务错误。
//
// code 为业务错误码（通常 >= 1000），msg 为用户可读的错误信息。
func CodeError(code int, msg string) *AppError {
	return &AppError{Code: code, Message: msg}
}

// CodeErrorf 创建一个格式化消息的业务错误。
func CodeErrorf(code int, format string, args ...any) *AppError {
	return &AppError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// InvalidArgument 创建参数错误（HTTP 400）。
//
// field 为参数名，reason 为不合法的原因。
// 示例：errors.InvalidArgument("user_id", "不能为空")
func InvalidArgument(field, reason string) *AppError {
	return &AppError{
		Code:    400,
		Message: fmt.Sprintf("invalid argument: %s — %s", field, reason),
	}
}

// NotFound 创建资源不存在错误（HTTP 404）。
//
// resource 为资源名称，如 "user" 或 "activity#123"。
func NotFound(resource string) *AppError {
	return &AppError{
		Code:    404,
		Message: fmt.Sprintf("not found: %s", resource),
	}
}

// Forbidden 创建权限不足错误（HTTP 403）。
func Forbidden(reason string) *AppError {
	return &AppError{
		Code:    403,
		Message: reason,
	}
}

// Internal 创建服务内部错误（HTTP 500）。
//
// cause 为底层错误（仅 log，不暴露），context 为操作描述。
func Internal(cause error, context string) *AppError {
	return &AppError{
		Code:    500,
		Message: "internal server error",
		cause:   fmt.Errorf("%s: %w", context, cause),
	}
}

// Wrap 包装一个 error，附加上下文描述。
// 若 err 为 nil，返回 nil。
// 与标准库 fmt.Errorf("%w") 语义一致，保留 Is/As 穿透能力。
func Wrap(err error, context string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}

// Is 代理标准库 errors.Is。
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// As 代理标准库 errors.As。
func As(err error, target any) bool {
	return errors.As(err, target)
}

// New 代理标准库 errors.New。
func New(text string) error {
	return errors.New(text)
}

// Errorf 代理 fmt.Errorf，支持 %w。
func Errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
