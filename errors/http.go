package errors

// HTTPStatus 返回适合 HTTP 响应的状态码。
//
// 规则：
//   - 400、403、404、422 等标准 HTTP 错误码直接返回
//   - 500 直接返回
//   - 业务错误码（通常 >= 1000）统一返回 200（由 code 字段区分业务含义）
//   - 其他情况返回 200
func (e *AppError) HTTPStatus() int {
	switch {
	case e.Code >= 400 && e.Code < 600:
		return e.Code
	default:
		return 200
	}
}
