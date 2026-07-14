# pager — 内存分页边界计算

> 统一分页越界行为。对标 util/pager，依据 sniper CLAUDE.md 的分页尾页规范实现。

## 设计哲学

**强制统一的越界行为。** `GetPager` 在越界时返回 `(0, 0)`，让调用方通过 `end == 0` 判断"无数据"并返回空列表——这是 CLAUDE.md 中反复强调的分页触底原则。

## 快速开始

```go
import "github.com/eden9th/bedrock/pager"

items := []int{1, 2, 3, 4, 5}

// 第一页
begin, end := pager.GetPager(int32(len(items)), 0, 2)  // (0, 2)
page1 := items[begin:end]  // [1, 2]

// 最后一页（不足一页）
begin, end = pager.GetPager(int32(len(items)), 4, 2)    // (4, 5)
page3 := items[begin:end]  // [5]

// 尾页越界（调用方感知触底，返回空列表）
begin, end = pager.GetPager(int32(len(items)), 5, 2)    // (0, 0)
if end == 0 {
    return []Item{}, true // 空列表 + isLast=true
}
```

## API 参考

```go
func GetPager(length, offset, limit int32) (begin, end int32)
```

返回值: `end == 0` 时表示无数据（越界），调用方应返回空列表。

## 依赖

纯标准库。
