// Package xstrings 提供泛型字符串与切片互转工具。
//
// 对标 util/xstrings，去处类型特定的 JoinInt32/SplitInt32 等重复函数，
// 用泛型提供统一的 Join/Split。
package xstrings

import (
	"strconv"
	"strings"
)

// Integer 是允许的整数类型约束。Go 1.25 中 constraints.Integer 已从 x/exp 移除。
type Integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

// Join 将整数切片以逗号连接为字符串。"" / "1" / "1,2,3"。
//
//	xstrings.Join([]int32{1, 2, 3}) → "1,2,3"
func Join[E Integer](s []E) string {
	return JoinFunc(s, func(e E) string { return strconv.Itoa(int(e)) })
}

// JoinFunc 将切片按 conv 转换后用逗号连接。
//
//	xstrings.JoinFunc([]string{"a","b"}, strings.ToUpper) → "A,B"
func JoinFunc[E any](s []E, conv func(E) string) string {
	if len(s) == 0 {
		return ""
	}
	var b strings.Builder
	for i, e := range s {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(conv(e))
	}
	return b.String()
}

// Split 将逗号分隔的字符串解析为整数切片。
//
//	xstrings.Split[int32]("1,2,3") → []int32{1,2,3}, nil
func Split[T Integer](origin string) ([]T, error) {
	return SplitFunc[T](origin, func(s string) (T, error) {
		i, err := strconv.Atoi(s)
		return T(i), err
	})
}

// SplitFunc 将逗号分隔的字符串按 conv 解析为任意类型切片。
func SplitFunc[T any](origin string, conv func(s string) (T, error)) ([]T, error) {
	if origin == "" {
		return nil, nil
	}
	parts := strings.Split(origin, ",")
	result := make([]T, 0, len(parts))
	for _, p := range parts {
		v, err := conv(p)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, nil
}

// GenSQLPlaceholder 生成 SQL IN 子句的占位符 "(?,?,...)"。
// n <= 0 时返回 "()"。
//
//	xstrings.GenSQLPlaceholder(3) → "(?,?,?)"
func GenSQLPlaceholder(n int) string {
	if n <= 0 {
		return "()"
	}
	var b strings.Builder
	b.WriteByte('(')
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('?')
	}
	b.WriteByte(')')
	return b.String()
}

// ─── 字符串判断工具 ──────────────────────────────────────────────────────────

// IsEmpty 判断字符串是否为空（长度为零）。
func IsEmpty(s string) bool {
	return len(s) == 0
}

// IsBlank 判断字符串是否为空或仅包含空白字符。
func IsBlank(s string) bool {
	return len(strings.TrimSpace(s)) == 0
}

// ─── 字符串截断工具 ──────────────────────────────────────────────────────────

// Truncate 将字符串截断到指定长度，超出部分用 suffix 表示。
// maxLen 必须 >= len(suffix)，否则 panic。
//
//	xstrings.Truncate("hello world", 8, "...") → "hello..."
func Truncate(s string, maxLen int, suffix string) string {
	if maxLen < len(suffix) {
		panic("xstrings: maxLen < len(suffix)")
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-len(suffix)] + suffix
}

// ─── 字符串填充工具 ──────────────────────────────────────────────────────────

// PadLeft 在左侧填充字符直到字符串达到指定长度。
// s 长度 >= totalLen 时不做变化。
//
//	xstrings.PadLeft("42", 5, '0') → "00042"
func PadLeft(s string, totalLen int, pad rune) string {
	if len(s) >= totalLen {
		return s
	}
	n := totalLen - len(s)
	var b strings.Builder
	b.Grow(totalLen)
	for i := 0; i < n; i++ {
		b.WriteRune(pad)
	}
	b.WriteString(s)
	return b.String()
}

// PadRight 在右侧填充字符直到字符串达到指定长度。
//
//	xstrings.PadRight("42", 5, ' ') → "42   "
func PadRight(s string, totalLen int, pad rune) string {
	if len(s) >= totalLen {
		return s
	}
	var b strings.Builder
	b.Grow(totalLen)
	b.WriteString(s)
	for i := len(s); i < totalLen; i++ {
		b.WriteRune(pad)
	}
	return b.String()
}
