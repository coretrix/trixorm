package trixorm

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisSearchAggregatePagination(t *testing.T) {
	var entity *redisSearchAggregateEntity
	engine, cleanup := prepareTables(t, &Registry{}, 5, "", "2.0", entity)
	defer cleanup()

	flusher := engine.NewFlusher()
	for i := 1; i <= 5; i++ {
		flusher.Track(&redisSearchAggregateEntity{Age: i})
	}
	flusher.Flush()

	for _, test := range []struct {
		name string
		page int
		ids  []uint64
	}{
		{name: "first page", page: 1, ids: []uint64{1, 2}},
		{name: "middle page", page: 2, ids: []uint64{3, 4}},
		{name: "partial last page", page: 3, ids: []uint64{5}},
		{name: "beyond last page", page: 4, ids: []uint64{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fields := &LoadFields{}
			fields.AddField("@ID")
			query := NewRedisSearchQuery().Aggregate().Load(fields).Sort(RedisSearchAggregateSort{Field: "@ID"})
			rows, total := engine.RedisSearchAggregate(entity, query, NewPager(test.page, 2))
			assert.Equal(t, uint64(5), total)
			assert.Len(t, rows, len(test.ids))
			ids := make([]uint64, 0, len(rows))
			for _, row := range rows {
				id, err := strconv.ParseUint(row["ID"], 10, 64)
				require.NoError(t, err)
				ids = append(ids, id)
			}
			assert.Equal(t, test.ids, ids)
		})
	}

	t.Run("no matches", func(t *testing.T) {
		query := NewRedisSearchQuery().FilterInt("Age", 100).Aggregate()
		rows, total := engine.RedisSearchAggregate(entity, query, NewPager(1, 2))
		assert.Zero(t, total)
		assert.Empty(t, rows)
	})
}
