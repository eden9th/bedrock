// Package pager 提供内存分页边界计算。
//
// 对标 util/pager。统一分页的越界行为：尾页后续请求返回空切片，
// 让调用方正确感知列表触底。
//
// CLI 参考: CLAUDE.md — 分页尾页处理原则
package pager

// GetPager 根据总数、偏移量、页大小计算切片范围 [begin, end)。
//
// 越界时返回 (0, 0)，调用方判断 end == 0 即可区分"无数据"和"正常返回"。
//
//	items := []int{1, 2, 3, 4, 5}
//	begin, end := pager.GetPager(int32(len(items)), 0, 2)  → (0, 2)
//	begin, end = pager.GetPager(int32(len(items)), 4, 2)    → (4, 5)
//	begin, end = pager.GetPager(int32(len(items)), 5, 2)    → (0, 0) 尾页越界
func GetPager(length, offset, limit int32) (begin, end int32) {
	begin = offset
	end = begin + limit

	if end > length {
		end = length
	}

	if begin < 0 || begin >= length || end < 0 {
		begin, end = 0, 0
		return
	}

	return
}
