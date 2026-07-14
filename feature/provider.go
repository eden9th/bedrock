package feature

import "github.com/eden9th/bedrock/conf"

// TOMLProvider 实现基于 conf.Value 的 Provider。
// 将 TOML 中的 [feature] section 映射为 flag 值。
//
// 使用示例：
//
//	v := conf.Get("app.toml")
//	feat := feature.New(feature.TOMLProvider{Value: v})
type TOMLProvider struct {
	Value *conf.Value
}

// Get 从 TOML 文件中读取 [feature] section 下的 flag 值。
// 使用内部 map 缓存解析结果，避免每次调用都解析 TOML。
func (p TOMLProvider) Get(flag string) string {
	var cfg struct {
		Feature map[string]string `toml:"feature"`
	}
	if err := p.Value.UnmarshalTOML(&cfg); err != nil {
		return ""
	}
	return cfg.Feature[flag]
}

// MapProvider 实现基于 map[string]string 的 Provider。
// 适用于测试或硬编码默认值。
//
//	feat := feature.New(feature.MapProvider{"new_checkout": "true"})
type MapProvider map[string]string

// Get 从 map 中读取 flag 值。
func (m MapProvider) Get(flag string) string {
	return m[flag]
}
