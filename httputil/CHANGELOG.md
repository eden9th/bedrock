# CHANGELOG — httputil

---

## [0.1.0] — 2026-07-12

### Added

- JSON(c, data) — 输出裸 JSON 响应
- JSONError(c, status, detail) — 错误响应 + Abort
- Bind(c, v) — 解析 JSON 请求体，失败自动 400
