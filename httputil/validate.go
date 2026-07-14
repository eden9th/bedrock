package httputil

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/eden9th/bedrock/bm"
	"github.com/eden9th/bedrock/errors"
	"github.com/go-playground/validator/v10"
)

// validate 是全局单例，validator 内部有缓存，重复创建有性能损耗。
var (
	globalValidator *validator.Validate
	validatorOnce   sync.Once
)

func getValidator() *validator.Validate {
	validatorOnce.Do(func() {
		globalValidator = validator.New()
		// 使用 json tag 作为字段名（而非 Go struct 字段名），与前端字段对齐
		globalValidator.RegisterTagNameFunc(func(fld reflect.StructField) string {
			name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
			if name == "-" {
				return ""
			}
			return name
		})
	})
	return globalValidator
}

// BindValidate 将请求体 JSON 解析到 T，然后用 go-playground/validator 执行结构体校验。
//
// 解析失败返回 HTTP 400（AppError code=400）。
// 校验失败返回 HTTP 422（AppError code=422），Message 包含所有字段错误。
// 成功时返回 (*T, nil)，handler 可直接使用。
//
// 结构体字段使用标准 validate tag：
//
//	type CreateReq struct {
//	    Name  string `json:"name"  validate:"required,max=50"`
//	    Email string `json:"email" validate:"required,email"`
//	    Age   int    `json:"age"   validate:"gte=0,lte=150"`
//	}
//
//	req, appErr := httputil.BindValidate[CreateReq](c)
//	if appErr != nil {
//	    httputil.JSONError(c, appErr.HTTPStatus(), appErr.Message)
//	    return
//	}
func BindValidate[T any](c *bm.Context) (*T, *errors.AppError) {
	var v T
	if !Bind(c, &v) {
		// Bind 内部已写入 400 响应并 Abort，此处只需返回错误供调用方感知
		return nil, errors.InvalidArgument("body", "invalid JSON")
	}

	if err := getValidator().Struct(v); err != nil {
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			msg := formatValidationErrors(ve)
			appErr := errors.CodeError(422, msg)
			JSONError(c, appErr.HTTPStatus(), appErr.Message)
			return nil, appErr
		}
		// 非 ValidationErrors（理论上不应出现）
		appErr := errors.InvalidArgument("body", err.Error())
		JSONError(c, appErr.HTTPStatus(), appErr.Message)
		return nil, appErr
	}

	return &v, nil
}

// formatValidationErrors 将 validator.ValidationErrors 格式化为单行可读消息。
// 示例："name: required; email: email"
func formatValidationErrors(ve validator.ValidationErrors) string {
	parts := make([]string, 0, len(ve))
	for _, fe := range ve {
		parts = append(parts, fmt.Sprintf("%s: %s", fe.Field(), fe.Tag()))
	}
	return strings.Join(parts, "; ")
}
