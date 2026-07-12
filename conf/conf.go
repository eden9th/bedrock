// Package conf 提供本地 TOML 配置文件加载和热更新（fsnotify）
package conf

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"
)

// Setter 配置热更新回调接口，与 paladin 保持兼容
type Setter interface {
	Set(text string) error
}

var (
	mu       sync.RWMutex
	confDir  string
	watchers map[string][]Setter
	watcher  *fsnotify.Watcher
)

// Init 初始化配置，从 dir 目录加载所有 TOML 文件
func Init(dir string) error {
	mu.Lock()
	defer mu.Unlock()
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

// UnmarshalTOML 将配置文件内容解析到 v
func (v *Value) UnmarshalTOML(out any) error {
	_, err := toml.DecodeFile(v.path, out)
	if err != nil {
		return fmt.Errorf("conf: decode %s: %w", v.path, err)
	}
	return nil
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
			for _, s := range setters {
				safeSet(s, filename, string(raw))
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			fmt.Fprintf(os.Stderr, "conf: watcher error: %v\n", err)
		}
	}
}
