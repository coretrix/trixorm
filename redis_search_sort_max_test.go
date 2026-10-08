package trixorm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedisSearchAggregateSortWithMax(t *testing.T) {
	aggregate := NewRedisSearchQuery().Aggregate().SortWithMax(1000,
		RedisSearchAggregateSort{Field: "@CreatedAt"},
		RedisSearchAggregateSort{Field: "@ID", Desc: true},
	)
	require.Equal(t, []interface{}{"SORTBY", "4", "@CreatedAt", "ASC", "@ID", "DESC", "MAX", "1000"}, aggregate.args)

	search := &RedisSearch{}
	args := search.applyPager(NewPager(1, 1000), aggregate.args)
	require.Equal(t, []interface{}{"LIMIT", 0, 1000}, args[len(args)-3:])

	for _, maximum := range []int{0, -1} {
		require.Panics(t, func() {
			NewRedisSearchQuery().Aggregate().SortWithMax(maximum, RedisSearchAggregateSort{Field: "@ID"})
		})
	}
	require.Panics(t, func() { NewRedisSearchQuery().Aggregate().SortWithMax(1000) })
}
