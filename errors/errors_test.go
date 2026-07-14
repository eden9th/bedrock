package errors_test

import (
	stderrors "errors"
	"strings"
	"testing"

	"github.com/eden9th/bedrock/errors"
)

// ----- CodeError / CodeErrorf -----

func TestCodeError(t *testing.T) {
	err := errors.CodeError(10001, "活动已结束")
	if err.Code != 10001 {
		t.Fatalf("want Code=10001, got %d", err.Code)
	}
	if err.Message != "活动已结束" {
		t.Fatalf("want Message=%q, got %q", "活动已结束", err.Message)
	}
	if err.Cause() != nil {
		t.Fatal("want Cause=nil")
	}
}

func TestCodeErrorf(t *testing.T) {
	err := errors.CodeErrorf(10002, "用户 %d 不存在", 42)
	if err.Code != 10002 {
		t.Fatalf("want Code=10002, got %d", err.Code)
	}
	if err.Message != "用户 42 不存在" {
		t.Fatalf("want Message=%q, got %q", "用户 42 不存在", err.Message)
	}
}

// ----- 预设构造函数 -----

func TestInvalidArgument(t *testing.T) {
	err := errors.InvalidArgument("user_id", "不能为空")
	if err.Code != 400 {
		t.Fatalf("want Code=400, got %d", err.Code)
	}
	if !strings.Contains(err.Message, "user_id") {
		t.Fatalf("message should contain field name, got %q", err.Message)
	}
	if !strings.Contains(err.Message, "不能为空") {
		t.Fatalf("message should contain reason, got %q", err.Message)
	}
}

func TestNotFound(t *testing.T) {
	err := errors.NotFound("user#42")
	if err.Code != 404 {
		t.Fatalf("want Code=404, got %d", err.Code)
	}
	if !strings.Contains(err.Message, "user#42") {
		t.Fatalf("message should contain resource, got %q", err.Message)
	}
}

func TestForbidden(t *testing.T) {
	err := errors.Forbidden("无权访问该资源")
	if err.Code != 403 {
		t.Fatalf("want Code=403, got %d", err.Code)
	}
}

func TestInternal(t *testing.T) {
	cause := stderrors.New("db connection refused")
	err := errors.Internal(cause, "查询用户失败")
	if err.Code != 500 {
		t.Fatalf("want Code=500, got %d", err.Code)
	}
	if err.Message != "internal server error" {
		t.Fatalf("want generic message, got %q", err.Message)
	}
	// cause 应该被包含在 Cause() 中
	if err.Cause() == nil {
		t.Fatal("want non-nil Cause")
	}
	if !strings.Contains(err.Cause().Error(), "查询用户失败") {
		t.Fatalf("cause should contain context, got %q", err.Cause().Error())
	}
	if !strings.Contains(err.Cause().Error(), "db connection refused") {
		t.Fatalf("cause should contain original error, got %q", err.Cause().Error())
	}
}

// ----- HTTPStatus -----

func TestHTTPStatus(t *testing.T) {
	cases := []struct {
		code       int
		wantStatus int
	}{
		{400, 400},
		{403, 403},
		{404, 404},
		{500, 500},
		{422, 422},
		{10001, 200}, // 业务错误码 -> 200
		{0, 200},
		{1, 200},
	}
	for _, tc := range cases {
		err := errors.CodeError(tc.code, "test")
		got := err.HTTPStatus()
		if got != tc.wantStatus {
			t.Errorf("code=%d: want HTTPStatus=%d, got %d", tc.code, tc.wantStatus, got)
		}
	}
}

// ----- Error() string -----

func TestErrorString(t *testing.T) {
	// 无 cause
	err := errors.CodeError(10001, "test")
	s := err.Error()
	if !strings.Contains(s, "10001") {
		t.Errorf("Error() should contain code, got %q", s)
	}

	// 有 cause
	cause := stderrors.New("underlying")
	wrapped := errors.Internal(cause, "ctx")
	s = wrapped.Error()
	if !strings.Contains(s, "500") {
		t.Errorf("Error() should contain code, got %q", s)
	}
}

// ----- WithCause -----

func TestWithCause(t *testing.T) {
	base := errors.CodeError(10001, "活动已结束")
	cause := stderrors.New("timeout")
	derived := base.WithCause(cause)

	// 原对象不变
	if base.Cause() != nil {
		t.Fatal("base should not be modified")
	}
	// 新对象有 cause
	if derived.Cause() == nil {
		t.Fatal("derived should have cause")
	}
	if derived.Code != base.Code || derived.Message != base.Message {
		t.Fatal("derived should keep original Code/Message")
	}
}

// ----- Wrap -----

func TestWrap(t *testing.T) {
	// nil 穿透
	if errors.Wrap(nil, "ctx") != nil {
		t.Fatal("Wrap(nil) should return nil")
	}

	inner := stderrors.New("inner error")
	outer := errors.Wrap(inner, "操作上下文")
	if outer == nil {
		t.Fatal("Wrap should return non-nil")
	}
	if !strings.Contains(outer.Error(), "inner error") {
		t.Errorf("wrapped error should contain inner, got %q", outer.Error())
	}
	if !strings.Contains(outer.Error(), "操作上下文") {
		t.Errorf("wrapped error should contain context, got %q", outer.Error())
	}
	// errors.Is 应能穿透
	if !errors.Is(outer, inner) {
		t.Error("errors.Is should unwrap through Wrap")
	}
}

// ----- errors.As 穿透 -----

func TestAs(t *testing.T) {
	appErr := errors.CodeError(10001, "test")
	wrapped := errors.Wrap(appErr, "outer context")

	var target *errors.AppError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should find AppError through Wrap")
	}
	if target.Code != 10001 {
		t.Fatalf("want Code=10001, got %d", target.Code)
	}
}

// ----- Unwrap（AppError 内部 cause 穿透）-----

func TestUnwrap(t *testing.T) {
	sentinel := stderrors.New("sentinel")
	appErr := errors.Internal(sentinel, "ctx")

	// errors.Is 应能穿透 AppError.cause
	if !stderrors.Is(appErr, sentinel) {
		t.Error("errors.Is should unwrap AppError.cause")
	}
}
