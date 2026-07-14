# xslices — 泛型切片工具

> Map / Filter / RemoveDuplicate / Splits / IfIntersect / Reverse。对标 util/xslices，纯标准库。

## 快速开始

```go
import "github.com/eden9th/bedrock/xslices"

// 映射
ids := xslices.Map(users, func(u User) int64 { return u.ID })

// 过滤
active := xslices.Filter(users, func(u User) bool { return u.Active })

// 去重（sniper CLAUDE.md 推荐模式）
ids = xslices.RemoveDuplicate(ids)

// 分批（SQL IN 查询 1000 条限制的标准解法）
for _, batch := range xslices.Splits(ids, 1000) {
    db.Query("SELECT ... WHERE id IN (?)", batch)
}

// 交集判断
if xslices.IfIntersect(a, b) { ... }

// 反转
reversed := xslices.Reverse(items)
```

## API 参考

```go
func Map[T, R any]([]T, func(T) R) []R
func Filter[T any]([]T, func(T) bool) []T
func RemoveDuplicate[S ~[]E, E comparable](S) S
func Splits[S ~[]E, E any](S, int) []S
func IfIntersect[E comparable]([]E, []E) bool
func Reverse[S ~[]E, E any](S) S
```

## 依赖

纯标准库。
