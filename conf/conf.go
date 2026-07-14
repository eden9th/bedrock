// Package conf 提供本地 TOML 配置文件加载和热更新（fsnotify）
package conf

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	mu.Lock()
	defer mu.Unlock()

	o := initOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	envPrefix = o.envPrefix

	confDir = dir
	watchers = make(map[string][]Setter)

	var err error
	watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("conf: create watcher: %w", err)
	}
	if err := watcher.Add(dir); err != nil {
		return fmt.Errorf("conf: watch dir %s: %w", dir, err)
	}
	go watchLoop(watcher)
	return nil
}

// Close 关闭文件监听
func Close() {
	if watcher != nil {
		watcher.Close()
	}
}

// Get 返回指定配置文件的 Value，用于 UnmarshalTOML
func Get(filename string) *Value {
	return &Value{path: filepath.Join(confDir, filename)}
}

// Watch 注册热更新回调，文件变更时调用 s.Set(rawText)
func Watch(filename string, s Setter) error {
	mu.Lock()
	defer mu.Unlock()
	watchers[filename] = append(watchers[filename], s)
	return nil
}

// Value 表示一个配置文件
type Value struct {
	path string
}

// UnmarshalTOML 将配置文件内容解析到 v，环境变量自动覆盖。
// 优先加载 TOML 文件，再用匹配的环境变量值覆盖。
func (v *Value) UnmarshalTOML(out any) error {
	// 1. 读取 TOML 文件原始内容
	raw, err := os.ReadFile(v.path)
	if err != nil {
		return fmt.Errorf("conf: read %s: %w", v.path, err)
	}

	// 2. 追加环境变量覆盖行（后面的值覆盖前面的）
	content := string(raw)
	if envPrefix != "" {
		content += envOverrideLines(envPrefix)
	}

	// 3. 解析合并后的 TOML
	if _, err := toml.Decode(content, out); err != nil {
		return fmt.Errorf("conf: decode %s: %w", v.path, err)
	}
	return nil
}

// envOverrideLines 生成环境变量覆盖的 TOML 文本。
// 例：APP_DB_MAX_OPEN="100" → db.max_open = 100
func envOverrideLines(prefix string) string {
	var lines []string
	for _, env := range os.Environ() {
		kv := strings.SplitN(env, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key, val := kv[0], kv[1]
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		// APP_DB_MAX_OPEN → db.max_open
		tomlKey := envToTOMLKey(key, prefix)
		if tomlKey == "" {
			continue
		}
		// 值需要 TOML 转义：字符串加引号，数字/布尔不加
		lines = append(lines, fmt.Sprintf("%s = %s", tomlKey, tomlValue(val)))
	}
	// 排序保证输出稳定
	sort.Strings(lines)
	if len(lines) == 0 {
		return ""
	}
	return "\n" + strings.Join(lines, "\n") + "\n"
}

// envToTOMLKey 将环境变量名转为 TOML key。
// APP_DB_MAX_OPEN → db.max_open
func envToTOMLKey(key, prefix string) string {
	rest := strings.TrimPrefix(key, prefix)
	if rest == "" {
		return ""
	}
	return strings.ToLower(rest)
}

// tomlValue 将环境变量值转为 TOML 值文本。
// 数字和布尔值不加引号，其余加双引号。
func tomlValue(val string) string {
	// 布尔值
	lower := strings.ToLower(val)
	if lower == "true" || lower == "false" {
		return lower
	}
	// 整数或浮点数（简单判断）
	if isNumeric(val) {
		return val
	}
	// 默认为字符串
	return fmt.Sprintf("%q", val)
}

func isNumeric(s string) bool {
	if s == "" || s == "-" {
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
	return len(s) > start
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
func watchLoop(w *fsnotify.Watcher) {
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
			setters := watchers[filename]
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

			// 追加环境变量覆盖
			if envPrefix != "" {
				rawStr += envOverrideLines(envPrefix)
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
