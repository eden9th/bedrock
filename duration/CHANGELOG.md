# CHANGELOG — duration

---

## [0.1.0] — 2026-07-12

### Added

- Duration 类型：封装 time.Duration，实现 toml.Unmarshaler
- Duration() 方法：返回 time.Duration
- UnmarshalTOML() 方法：从 TOML 字符串解析 duration
