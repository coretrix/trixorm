package trixorm

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeAllocationCleanupKeepsCachedSearchCompatibility(t *testing.T) {
	var entity *cachedSearchEntity
	var entityRef *cachedSearchRefEntity
	engine, def := prepareTables(t, &Registry{}, 5, "", "2.0", entityRef, entity)
	defer def()

	schema := engine.GetRegistry().GetTableSchemaForEntity(entity).(*tableSchema)
	schema.localCacheName = "default"
	schema.hasLocalCache = true
	schema.redisCacheName = "default"
	schema.hasRedisCache = true

	flusher := engine.NewFlusher()
	for index := 1; index <= 3; index++ {
		flusher.Track(&cachedSearchRefEntity{Name: "Reference " + strconv.Itoa(index)})
	}
	flusher.Flush()

	for index := 1; index <= 3; index++ {
		flusher.Track(&cachedSearchEntity{
			Name:         "Name " + strconv.Itoa(index),
			Age:          10,
			ReferenceOne: &cachedSearchRefEntity{ID: uint(index)},
		})
	}
	flusher.Flush()

	var rows []*cachedSearchEntity
	totalRows := engine.CachedSearch(&rows, "IndexAge", NewPager(1, 10), 10)
	require.Equal(t, 3, totalRows)
	require.Len(t, rows, 3)
	require.Equal(t, uint(1), rows[0].ID)

	definition := schema.cachedIndexes["IndexAge"]
	where := NewWhere(definition.Query, 10)
	cacheKey := getCacheKeySearch(schema, "IndexAge", where.GetParameters()...)

	localValue, hasLocalValue := engine.GetLocalCache().Get(cacheKey)
	require.True(t, hasLocalValue)
	assert.Equal(t, []uint64{3, 1, 2, 3}, localValue)

	redisValues := engine.GetRedis().HMGet(cacheKey, "1")
	assert.Equal(t, "3 1 2 3", redisValues["1"])

	entityCacheValue, hasEntityCacheValue := engine.GetLocalCache().Get(schema.getCacheKey(1))
	require.True(t, hasEntityCacheValue)
	_, entityCacheIsBinary := entityCacheValue.([]byte)
	assert.True(t, entityCacheIsBinary)

	rows[0].Name = "mutated without flush"
	loaded := &cachedSearchEntity{}
	require.True(t, engine.LoadByID(uint64(rows[0].ID), loaded))
	assert.Equal(t, "Name 1", loaded.Name)

	rows[0].Name = "mutated with flush"
	engine.Flush(rows[0])
	loaded = &cachedSearchEntity{}
	require.True(t, engine.LoadByID(uint64(rows[0].ID), loaded))
	assert.Equal(t, "mutated with flush", loaded.Name)

	totalRows, ids := engine.CachedSearchIDs(entity, "IndexAge", nil, 10)
	assert.Equal(t, 3, totalRows)
	assert.Equal(t, []uint64{1, 2, 3}, ids)

	rows = nil
	totalRows = engine.CachedSearchWithReferences(&rows, "IndexAge", nil, []interface{}{10}, []string{"ReferenceOne"})
	require.Equal(t, 3, totalRows)
	require.Len(t, rows, 3)
	require.NotNil(t, rows[0].ReferenceOne)
	assert.Equal(t, "Reference 1", rows[0].ReferenceOne.Name)
}

func TestSafeAllocationCleanupKeepsLoadByIDsDuplicateAndMissingBehavior(t *testing.T) {
	var entity *loadByIdsEntity
	engine, def := prepareTables(t, &Registry{}, 5, "", "2.0", entity)
	defer def()

	schema := engine.GetRegistry().GetTableSchemaForEntity(entity).(*tableSchema)
	schema.localCacheName = "default"
	schema.hasLocalCache = true
	schema.redisCacheName = "default"
	schema.hasRedisCache = true

	engine.FlushMany(&loadByIdsEntity{Name: "first"}, &loadByIdsEntity{Name: "second"})

	var rows []*loadByIdsEntity
	found := engine.LoadByIDs([]uint64{1, 3, 1, 2, 3}, &rows)
	assert.False(t, found)
	require.Len(t, rows, 5)
	assert.Equal(t, "first", rows[0].Name)
	assert.Nil(t, rows[1])
	assert.Equal(t, "first", rows[2].Name)
	assert.Equal(t, "second", rows[3].Name)
	assert.Nil(t, rows[4])
}

func BenchmarkSafeAllocationCachedSearchIDs(b *testing.B) {
	entity := &cachedSearchEntity{}
	ref := &cachedSearchRefEntity{}
	registry := &Registry{}
	registry.RegisterLocalCache(10000)
	engine, def := prepareTables(nil, registry, 5, "", "2.0", ref, entity)
	defer def()

	schema := engine.GetRegistry().GetTableSchemaForEntity(entity).(*tableSchema)
	schema.localCacheName = "default"
	schema.hasLocalCache = true

	flusher := engine.NewFlusher()
	for index := 1; index <= 100; index++ {
		flusher.Track(&cachedSearchEntity{Name: "Name " + strconv.Itoa(index), Age: 10})
	}
	flusher.Flush()
	_, _ = engine.CachedSearchIDs(entity, "IndexAge", NewPager(1, 100), 10)

	b.ResetTimer()
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		_, _ = engine.CachedSearchIDs(entity, "IndexAge", NewPager(1, 100), 10)
	}
}
