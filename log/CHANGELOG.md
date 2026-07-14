# CHANGELOG — log

---

## [0.2.0] — 2026-07-14

### Added

- Field 类型化日志字段（KVString / KVInt64 / KVInt / KVFloat64 / KVBool / KVDuration）
- Infov / Warnv / Errorv — 带类型化 KV 字段的结构化日志函数
- entry.Extra 字段 — JSON 日志中附加键值对，数值保留 number 类型

---

## [0.1.0] — 2026-07-12

### Added

- 结构化 JSON 日志输出：time / level / trace_id / caller / msg
- 四级日志：Debug / Info / Warn / Error
- 双输出：stderr（始终）+ 文件（可选，lumberjack 分片）
- Config 配置：FilePath / MaxSizeMB / MaxBackups / MaxAgeDays / Level
- 从 context 自动提取 trace_id（通过 internal/ctxkey 与 trace 包共享）
- 调用者信息：runtime.Caller 获取文件名和行号
- 支持配置 nil 时默认仅 stderr 输出
- log_test.go 全功能测试
