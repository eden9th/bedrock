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
	go watchLoop()
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

func watchLoop() {
	for {
		select {
		case event, ok := <-watcher.Events:
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
				continue
			}
			for _, s := range setters {
				_ = s.Set(string(raw))
			}
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
		}
	}
}
