# httputil — Handler 工具函数

> 在 `bm.Context` 上封装常用响应和请求体解析。输出裸 JSON（不带信封），与 `c.JSON` 的标准信封格式互补。

## 设计哲学

**两种 JSON 格式，两种场景。**

| 函数 | 输出格式 | 适用场景 |
|------|----------|----------|
| `c.JSON(data, err)` | `{"code":0,"message":"ok","data":{...}}` | 标准 API 响应 |
| `httputil.JSON(c, data)` | `{...raw JSON...}` | 内部接口、Metrics、Health |
| `httputil.JSONError(c, 400, "msg")` | `{"detail":"msg"}` | 错误响应 |

## 快速开始

```go
import "github.com/eden9th/bedrock/httputil"

e.GET("/api/data", func(c *bm.Context) {
    // 解析请求体
    var req CreateReq
    if !httputil.Bind(c, &req) {
        return // Bind 失败时已自动返回 400
    }

    data, err := doSomething(req)
    if err != nil {
        httputil.JSONError(c, 500, err.Error())
        return
    }

    httputil.JSON(c, data)
})
```

## API 参考

```go
func JSON(c *bm.Context, data any)                       // 200 + 裸 JSON
func JSONError(c *bm.Context, httpStatus int, detail string) // 错误响应 + Abort
func Bind(c *bm.Context, v any) bool                      // 解析 JSON body
```

### JSON

输出裸 JSON，不带 `{"code":..., "message":..., "data":...}` 信封。

```go
httputil.JSON(c, map[string]string{"status": "ok"})
// → HTTP 200, Content-Type: application/json
// → {"status":"ok"}
```

### JSONError

终止中间件链并返回错误响应。

```go
httputil.JSONError(c, 400, "invalid user id")
// → HTTP 400, Content-Type: application/json
// → {"detail":"invalid user id"}
```

### Bind

解析 JSON 请求体到 struct。解析失败时自动调用 `JSONError(c, 400, ...)` 并返回 `false`。

```go
var req struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}
if !httputil.Bind(c, &req) {
    return // 解析失败，已自动 400
}
// req.Name, req.Age 可直接使用
```

## 常见问题

### Q: 什么时候用 `httputil.JSON`，什么时候用 `c.JSON`？

- **外部 API / 前端接口**：用 `c.JSON`（带 code/message 信封，前端统一处理）
- **内部接口 / 健康检查 / Metrics**：用 `httputil.JSON`（裸 JSON，机器可读）

### Q: Bind 支持哪些 Content-Type？

目前只支持 `application/json`。如果后续需要 form/multipart 支持，会扩展 `Bind`。

## 依赖

- `bedrock/bm`
