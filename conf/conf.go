// Package conf 提供本地 TOML 配置文件加载和热更新（fsnotify）
package conf

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"
)

// Setter 配置热更新回调接口，与 paladin 保持兼容
type Setter interface {
	Set(text string) error
}

// SetterFunc 将 func(string) error 转换为 Setter（类似 http.HandlerFunc）
type SetterFunc func(text string) error

func (f SetterFunc) Set(text string) error { return f(text) }

// Validator 配置校验接口。Setter 在调用 Set 前可先调用 Validate 校验配置有效性。
// 校验失败时跳过 Set 调用，避免应用错误配置导致运行时异常。
type Validator interface {
	// Validate 校验配置文本是否有效。返回 error 表示校验失败。
	Validate(text string) error
}

var (
	mu        sync.RWMutex
	confDir   string
	watchers  map[string][]Setter
	watcher   *fsnotify.Watcher
	envPrefix string // 环境变量前缀，如 "APP_"
)

// Option 是 Init 的函数式配置项。
type Option func(*initOptions)

type initOptions struct {
	envPrefix string
}

// WithEnvPrefix 设置环境变量前缀，用于覆盖 TOML 配置值。
//
// TOML key 到环境变量名的转换规则：
//   - 点分隔 → 下划线：db.host → DB_HOST
//   - 加上前缀：DB_HOST → APP_DB_HOST
//   - 全部大写
//
// 环境变量优先级高于 TOML 文件值。
// 前缀为空时不启用环境变量覆盖。
func WithEnvPrefix(prefix string) Option {
	return func(o *initOptions) { o.envPrefix = prefix }
}

// Init 初始化配置，从 dir 目录加载所有 TOML 文件
func Init(dir string, opts ...Option) error {
	o := initOptions{}
	for _, opt := range opts {
		opt(&o)
	}

	nextWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("conf: create watcher: %w", err)
	}
	if err := nextWatcher.Add(dir); err != nil {
		_ = nextWatcher.Close()
		return fmt.Errorf("conf: watch dir %s: %w", dir, err)
	}

	mu.Lock()
	oldWatcher := watcher
	envPrefix = o.envPrefix
	confDir = dir
	watchers = make(map[string][]Setter)
	watcher = nextWatcher
	mu.Unlock()

	if oldWatcher != nil {
		_ = oldWatcher.Close()
	}

	go watchLoop(nextWatcher, o.envPrefix)
	return nil
}

// Close 关闭文件监听
func Close() {
	mu.Lock()
	oldWatcher := watcher
	confDir = ""
	watchers = nil
	watcher = nil
	envPrefix = ""
	mu.Unlock()

	if oldWatcher != nil {
		_ = oldWatcher.Close()
	}
}

// Get 返回指定配置文件的 Value，用于 UnmarshalTOML
func Get(filename string) *Value {
	mu.RLock()
	dir := confDir
	prefix := envPrefix
	mu.RUnlock()
	return &Value{path: filepath.Join(dir, filename), envPrefix: prefix}
}

// Watch 注册热更新回调，文件变更时调用 s.Set(rawText)
func Watch(filename string, s Setter) error {
	mu.Lock()
	defer mu.Unlock()
	if watchers == nil {
		return fmt.Errorf("conf: Init must be called before Watch")
	}
	watchers[filename] = append(watchers[filename], s)
	return nil
}

// Value 表示一个配置文件
type Value struct {
	path      string
	envPrefix string
}

// UnmarshalTOML 将配置文件内容解析到 v，环境变量自动覆盖。
// 先解析 TOML 文件为 map，再将匹配前缀的环境变量 deep merge 进去，
// env 值优先级高于文件值。
func (v *Value) UnmarshalTOML(out any) error {
	// 1. 读取并解析 TOML 文件为 map
	raw, err := os.ReadFile(v.path)
	if err != nil {
		return fmt.Errorf("conf: read %s: %w", v.path, err)
	}
	base := make(map[string]any)
	if _, err := toml.Decode(string(raw), &base); err != nil {
		return fmt.Errorf("conf: decode %s: %w", v.path, err)
	}

	// 2. 将环境变量展开为嵌套 map，deep merge 覆盖 base
	if v.envPrefix != "" {
		overlay := envOverlayMap(v.envPrefix)
		deepMerge(base, overlay)
	}

	// 3. 将 merge 后的 map 重新编码为 TOML，再解析到 out
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(base); err != nil {
		return fmt.Errorf("conf: re-encode %s: %w", v.path, err)
	}
	if _, err := toml.Decode(buf.String(), out); err != nil {
		return fmt.Errorf("conf: final decode %s: %w", v.path, err)
	}
	return nil
}

// envOverlayMap 将匹配前缀的环境变量展开为嵌套 map。
// 使用双下划线 __ 作为层级分隔符，单下划线保留在 key 内。
// 例：APP_DB__MAX_OPEN=100 → {"db": {"max_open": "100"}}
// 例：APP_NAME=foo → {"name": "foo"}
func envOverlayMap(prefix string) map[string]any {
	result := make(map[string]any)
	for _, env := range os.Environ() {
		kv := strings.SplitN(env, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key, val := kv[0], kv[1]
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		// 去掉前缀，按 __ 分层，转小写
		rest := strings.TrimPrefix(key, prefix)
		if rest == "" {
			continue
		}
		parts := strings.Split(strings.ToLower(rest), "__")
		setNested(result, parts, val)
	}
	return result
}

// setNested 按 parts 路径在 m 中设置值，中间层自动创建 map。
// val 会被自动转为合适的 Go 类型（bool/int64/float64/string），
// 以确保 TOML re-encode 后类型与目标结构体兼容。
func setNested(m map[string]any, parts []string, val string) {
	if len(parts) == 1 {
		m[parts[0]] = parseEnvVal(val)
		return
	}
	next, ok := m[parts[0]].(map[string]any)
	if !ok {
		next = make(map[string]any)
	}
	m[parts[0]] = next
	setNested(next, parts[1:], val)
}

// parseEnvVal 将环境变量字符串值转为合适的 Go 类型。
func parseEnvVal(val string) any {
	lower := strings.ToLower(val)
	if lower == "true" {
		return true
	}
	if lower == "false" {
		return false
	}
	// 尝试整数
	if isInteger(val) {
		var n int64
		fmt.Sscan(val, &n)
		return n
	}
	// 尝试浮点
	if isFloat(val) {
		var f float64
		fmt.Sscan(val, &f)
		return f
	}
	return val
}

func isInteger(s string) bool {
	if s == "" {
		return false
	}
	start := 0
	if s[0] == '-' || s[0] == '+' {
		start = 1
	}
	if start >= len(s) {
		return false
	}
	for i := start; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isFloat(s string) bool {
	if s == "" || s == "-" || s == "+" {
		return false
	}
	start := 0
	if s[0] == '-' || s[0] == '+' {
		start = 1
	}
	hasDot := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if c == '.' {
			if hasDot {
				return false
			}
			hasDot = true
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return hasDot && len(s) > start
}

// deepMerge 将 src 的值递归 merge 到 dst，src 优先级更高。
func deepMerge(dst, src map[string]any) {
	for k, sv := range src {
		dv, exists := dst[k]
		if !exists {
			dst[k] = sv
			continue
		}
		// 双方都是 map 则递归 merge
		dstMap, dstIsMap := dv.(map[string]any)
		srcMap, srcIsMap := sv.(map[string]any)
		if dstIsMap && srcIsMap {
			deepMerge(dstMap, srcMap)
			continue
		}
		// 否则 src 直接覆盖 dst
		dst[k] = sv
	}
}

// envToTOMLKey 将环境变量名转为 TOML key（保留，仅用于测试和文档说明）。
// 使用双下划线 __ 作为层级分隔符：APP_DB__MAX_OPEN → db.max_open
func envToTOMLKey(key, prefix string) string {
	rest := strings.TrimPrefix(key, prefix)
	if rest == "" {
		return ""
	}
	// __ 表示层级（转为 .），_ 保留在 key 内
	lower := strings.ToLower(rest)
	return strings.ReplaceAll(lower, "__", ".")
}

// safeSet 调用 s.Set(raw)，捕获 panic 和错误，均输出到 stderr。
// 使用 fmt.Fprintf(os.Stderr) 而非 log 包，避免循环依赖。
func safeSet(s Setter, filename, raw string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "conf: setter panic for %s: %v\n", filename, r)
		}
	}()
	if err := s.Set(raw); err != nil {
		fmt.Fprintf(os.Stderr, "conf: setter error for %s: %v\n", filename, err)
	}
}

// watchLoop 监听 w 上的文件系统事件，驱动热更新回调。
// 接收局部 watcher 参数而非读取包级变量，避免并发写入时的 data race。
func watchLoop(w *fsnotify.Watcher, prefix string) {
	for {
		select {
		case event, ok := <-w.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			filename := filepath.Base(event.Name)
			mu.RLock()
			if w != watcher {
				mu.RUnlock()
				return
			}
			setters := append([]Setter(nil), watchers[filename]...)
			mu.RUnlock()
			if len(setters) == 0 {
				continue
			}
			raw, err := os.ReadFile(event.Name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "conf: read %s: %v\n", event.Name, err)
				continue
			}
			rawStr := string(raw)

			// 将环境变量 deep merge 后重新编码为 TOML 文本传给 Setter
			if prefix != "" {
				base := make(map[string]any)
				if _, err := toml.Decode(rawStr, &base); err == nil {
					overlay := envOverlayMap(prefix)
					deepMerge(base, overlay)
					var buf bytes.Buffer
					if err := toml.NewEncoder(&buf).Encode(base); err == nil {
						rawStr = buf.String()
					}
				}
			}

			for _, s := range setters {
				// 如果 Setter 实现了 Validator，先校验
				if v, ok := s.(Validator); ok {
					if err := v.Validate(rawStr); err != nil {
						fmt.Fprintf(os.Stderr, "conf: validation failed for %s: %v\n", filename, err)
						continue
					}
				}
				safeSet(s, filename, rawStr)
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			fmt.Fprintf(os.Stderr, "conf: watcher error: %v\n", err)
		}
	}
}
