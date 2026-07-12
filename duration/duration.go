// Package duration 提供可从 TOML 字符串反序列化的 Duration 类型
package duration

import (
	"fmt"
	"time"
)

// Duration 支持 TOML 里写 "1h30m" 直接反序列化为 time.Duration
type Duration time.Duration

func (d *Duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("duration: parse %q: %w", text, err)
	}
	*d = Duration(v)
	return nil
}

func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

func (d Duration) String() string {
	return time.Duration(d).String()
}
