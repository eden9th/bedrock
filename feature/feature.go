// Package feature 提供极简功能开关（Feature Flag）。
//
// # 核心设计
//
// 支持两种后端：
//   - TOML 文件：本地配置驱动（默认）
//   - 自定义 provider：如数据库、配置中心
//
// 每个 flag 支持：
//   - 布尔开关：true/false
//   - 百分比灰度：0.0~1.0，基于 key（如 uid）的 hash 判断
//
// # 使用示例
//
//	// TOML 配置（app.toml）
//	[feature]
//	new_checkout = true
//	dark_mode = 0.1   # 10% 灰度
//
//	// 实现 Provider 接口，读取 TOML 配置
//	type tomlProvider struct{ v *conf.Value }
//	func (p *tomlProvider) Get(flag string) string {
//	    var cfg struct{ Feature map[string]string `toml:"feature"` }
//	    if err := p.v.UnmarshalTOML(&cfg); err != nil { return "" }
//	    return cfg.Feature[flag]
//	}
//
//	// 创建并使用
//	feat := feature.New(&tomlProvider{v: conf.Get("app.toml")})
//	if feat.Enabled("new_checkout") { ... }
//	if feat.EnabledFor("dark_mode", uid) { ... }  // 基于 uid hash 的灰度
//
// # 线程安全
//
// 所有方法并发安全。
package feature

import (
	"sync"
)

// Provider 是特性开关的后端接口。
type Provider interface {
	// Get 返回 flag 的值。flag 不存在时返回空字符串。
	Get(flag string) string
}

// Flags 管理特性开关。
type Flags struct {
	mu       sync.RWMutex
	provider Provider
}

// New 创建特性开关管理器。
func New(provider Provider) *Flags {
	return &Flags{provider: provider}
}

// Enabled 判断 flag 是否为 true 状态。
// 不存在的 flag 返回 false。
func (f *Flags) Enabled(flag string) bool {
	v := f.get(flag)
	return v == "true" || v == "1"
}

// EnabledFor 判断 flag 是否对指定 key 生效。
// flag 值为浮点数时作为百分比灰度（0.0~1.0）；
// flag 值为 "true"/"1" 时全量开启；
// 其他值视为关闭。
func (f *Flags) EnabledFor(flag string, key string) bool {
	v := f.get(flag)
	if v == "" {
		return false
	}
	if v == "true" || v == "1" {
		return true
	}
	if v == "false" || v == "0" {
		return false
	}

	// 解析为百分比
	var pct float64
	for _, c := range v {
		if c == '.' {
			continue
		}
		if c >= '0' && c <= '9' {
			pct = pct*10 + float64(c-'0')
		}
	}
	// 按位数还原：0.1 → 10%, 0.25 → 25%
	digits := 0
	dotFound := false
	for _, c := range v {
		if c == '.' {
			dotFound = true
			continue
		}
		if dotFound {
			digits++
		}
	}
	for i := 0; i < digits; i++ {
		pct /= 10
	}

	if pct <= 0 {
		return false
	}
	if pct >= 1 {
		return true
	}

	return hashKey(key)%10000 < int64(pct*10000)
}

func (f *Flags) get(flag string) string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.provider.Get(flag)
}

// hashKey 对 key 做简单 hash，用于灰度判定。
func hashKey(key string) int64 {
	var h uint64 = 14695981039346656037 // FNV offset basis
	for i := 0; i < len(key); i++ {
		h ^= uint64(key[i])
		h *= 1099511628211 // FNV prime
	}
	return int64(h & 0x7FFFFFFFFFFFFFFF) // 确保非负
}
