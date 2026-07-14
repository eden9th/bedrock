# xstrings — 泛型字符串-切片互转工具

> Join（切片→逗号字符串）/ Split（逗号字符串→切片）/ GenSQLPlaceholder（SQL 占位符）。对标 util/xstrings，纯标准库。

## 快速开始

```go
import "github.com/eden9th/bedrock/xstrings"

// 切片 → 字符串
xstrings.Join([]int32{1, 2, 3})       // "1,2,3"

// 字符串 → 切片
ids, err := xstrings.Split[int32]("1,2,3") // []int32{1,2,3}

// SQL 占位符
xstrings.GenSQLPlaceholder(3)          // "(?,?,?)"

// 自定义转换
xstrings.JoinFunc(users, func(u User) string { return u.Name }) // "alice,bob"
```

## API 参考

```go
func Join[E Integer]([]E) string
func JoinFunc[E any]([]E, func(E) string) string
func Split[T Integer](string) ([]T, error)
func SplitFunc[T any](string, func(string) (T, error)) ([]T, error)
func GenSQLPlaceholder(n int) string
```

## 常见问题

### Q: Integer 约束包含哪些类型？

`~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr`。`string` 和 `float` 类型请使用 `JoinFunc` / `SplitFunc`。

## 依赖

纯标准库。
