package conf_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eden9th/bedrock/conf"
)

// chanSetter 实现 conf.Setter，把收到的文本发送到 channel
type chanSetter struct {
	ch chan string
}

func (s *chanSetter) Set(text string) error {
	s.ch <- text
	return nil
}

// errorSetter 实现 conf.Setter，每次 Set 返回错误（用于测试错误不崩溃）
type errorSetter struct{}

func (s *errorSetter) Set(text string) error {
	return os.ErrInvalid
}

// panicSetter 实现 conf.Setter，每次 Set 触发 panic（用于测试 recover）
type panicSetter struct{}

func (s *panicSetter) Set(text string) error {
	panic("setter panic!")
}

func TestUnmarshalTOML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.toml"), []byte("name = \"bedrock\"\nversion = 2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := conf.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conf.Close)

	var cfg struct {
		Name    string `toml:"name"`
		Version int    `toml:"version"`
	}
	if err := conf.Get("app.toml").UnmarshalTOML(&cfg); err != nil {
		t.Fatalf("UnmarshalTOML failed: %v", err)
	}
	if cfg.Name != "bedrock" {
		t.Errorf("expected name=%q, got %q", "bedrock", cfg.Name)
	}
	if cfg.Version != 2 {
		t.Errorf("expected version=2, got %d", cfg.Version)
	}
}

func TestWatch_SetterCalledOnFileWrite(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "watch.toml")
	if err := os.WriteFile(cfgFile, []byte("key = \"init\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := conf.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conf.Close)

	ch := make(chan string, 1)
	s := &chanSetter{ch: ch}
	if err := conf.Watch("watch.toml", s); err != nil {
		t.Fatal(err)
	}

	// 写入新内容触发 fsnotify 事件
	if err := os.WriteFile(cfgFile, []byte("key = \"updated\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case text := <-ch:
		if text == "" {
			t.Fatal("setter received empty text")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("setter was not called within 500ms after file write")
	}
}

func TestWatch_ErrorSetterDoesNotCrash(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "err.toml")
	if err := os.WriteFile(cfgFile, []byte("x = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := conf.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conf.Close)

	if err := conf.Watch("err.toml", &errorSetter{}); err != nil {
		t.Fatal(err)
	}

	// 写入触发 Set，Set 返回错误，程序不应崩溃
	if err := os.WriteFile(cfgFile, []byte("x = 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// 等待 watchLoop 处理，若 panic 则测试会失败
	time.Sleep(200 * time.Millisecond)
}

func TestWatch_PanicSetterDoesNotCrash(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "panic.toml")
	if err := os.WriteFile(cfgFile, []byte("x = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := conf.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conf.Close)

	if err := conf.Watch("panic.toml", &panicSetter{}); err != nil {
		t.Fatal(err)
	}

	// 写入触发 Set，Set panic，safeSet 应 recover，程序不崩溃
	if err := os.WriteFile(cfgFile, []byte("x = 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
}

func TestInit_InvalidDir(t *testing.T) {
	err := conf.Init("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for invalid dir, got nil")
	}
}
