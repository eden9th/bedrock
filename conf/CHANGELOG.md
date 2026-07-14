# CHANGELOG — conf

---

## [0.2.0] — 2026-07-14

### Added

- SetterFunc 类型：将 func(string) error 转换为 Setter（类似 http.HandlerFunc）

### Documentation

- README 重写：修正 API 文档（OnChange→Watch, File→Value），新增 SetterFunc 示例

---

## [0.1.0] — 2026-07-12

### Added

- Init(dir) — 从指定目录加载所有 TOML 文件
- Get(filename) — 按键（文件名）获取配置
- OnChange(filename, Setter) — 注册变更回调
- WatchAndUpdate() — 启动 fsnotify 文件监听
- Close() — 关闭监听
- File.Raw() — 获取原始 TOML 文本
- File.UnmarshalTOML(v) — 反序列化到 struct
- 依赖：BurntSushi/toml + fsnotify/fsnotify
