// Package xslices 提供泛型切片工具函数。
// 对标 util/xslices，去除 stdlib slices 已有功能（Index/Contains/IndexFunc），
// 保留无 stdlib 等价物的高频操作。
package xslices

// Map 将切片元素按 f 转换，返回新切片。
// 高阶函数：[]A → []B。
func Map[T any, R any](items []T, f func(T) R) []R {
	result := make([]R, 0, len(items))
	for _, item := range items {
		result = append(result, f(item))
	}
	return result
}

// Filter 保留满足 f 的元素，返回新切片。
func Filter[T any](items []T, f func(T) bool) []T {
	result := make([]T, 0)
	for _, item := range items {
		if f(item) {
			result = append(result, item)
		}
	}
	return result
}

// RemoveDuplicate 返回去重后的新切片，保持首次出现顺序。
// 对标 util/xslices.RemoveDuplicate，是 sniper CLAUDE.md 的推荐模式。
func RemoveDuplicate[S ~[]E, E comparable](s S) S {
	seen := make(map[E]struct{}, len(s))
	result := make(S, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}

// Splits 将切片按每组最多 l 个元素拆分为二维切片。
// 最后一组长度可能小于 l。l <= 0 时 panic。
//
//	xslices.Splits([]int{1,2,3,4,5}, 2) → [[1,2],[3,4],[5]]
func Splits[S ~[]E, E any](s S, l int) []S {
	if l <= 0 {
		panic("xslices: l <= 0")
	}
	if len(s) == 0 {
		return nil
	}
	n := (len(s) + l - 1) / l
	result := make([]S, 0, n)
	for i := 0; i < len(s); i += l {
		end := i + l
		if end > len(s) {
			end = len(s)
		}
		result = append(result, s[i:end])
	}
	return result
}

// IfIntersect 判断两个切片是否有交集。
func IfIntersect[E comparable](a, b []E) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	am := make(map[E]struct{}, len(a))
	for _, v := range a {
		am[v] = struct{}{}
	}
	for _, v := range b {
		if _, ok := am[v]; ok {
			return true
		}
	}
	return false
}

// Reverse 返回反转后的新切片，不修改原切片。
func Reverse[S ~[]E, E any](s S) S {
	result := make(S, len(s))
	for i, j := 0, len(s)-1; i < len(s); i, j = i+1, j-1 {
		result[i] = s[j]
	}
	return result
}

// Reduce 对切片进行归约，将每个元素累积到累加器。
// init 为初始累加器值，f 为每个元素调用的归约函数。
//
//	xslices.Reduce([]int{1,2,3}, 0, func(acc int, x int) int { return acc + x }) → 6
func Reduce[T any, R any](items []T, init R, f func(acc R, item T) R) R {
	acc := init
	for _, item := range items {
		acc = f(acc, item)
	}
	return acc
}
