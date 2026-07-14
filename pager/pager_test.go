package pager_test

import (
	"testing"

	"github.com/eden9th/bedrock/pager"
)

func TestGetPagerBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		length    int32
		offset    int32
		limit     int32
		wantBegin int32
		wantEnd   int32
	}{
		{name: "first page", length: 5, offset: 0, limit: 2, wantBegin: 0, wantEnd: 2},
		{name: "last partial page", length: 5, offset: 4, limit: 2, wantBegin: 4, wantEnd: 5},
		{name: "after last page", length: 5, offset: 5, limit: 2, wantBegin: 0, wantEnd: 0},
		{name: "negative offset", length: 5, offset: -1, limit: 2, wantBegin: 0, wantEnd: 0},
		{name: "zero limit", length: 5, offset: 0, limit: 0, wantBegin: 0, wantEnd: 0},
		{name: "negative limit", length: 5, offset: 2, limit: -1, wantBegin: 0, wantEnd: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			begin, end := pager.GetPager(tt.length, tt.offset, tt.limit)
			if begin != tt.wantBegin || end != tt.wantEnd {
				t.Fatalf("expected (%d,%d), got (%d,%d)", tt.wantBegin, tt.wantEnd, begin, end)
			}
		})
	}
}
