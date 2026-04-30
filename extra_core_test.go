package trixorm

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type extraSetFieldStruct struct {
	Name string
}

type extraSetFieldRef struct {
	ORM
	ID uint
}

type extraSetFieldEntity struct {
	ORM
	ID            uint
	Name          string
	Count         uint
	CountPtr      *uint
	Signed        int
	SignedPtr     *int
	Enabled       bool
	EnabledPtr    *bool
	Ratio         float64
	RatioPtr      *float64
	StartedAt     time.Time
	StartedAtPtr  *time.Time
	Labels        []string
	Payload       []uint8
	Ref           *extraSetFieldRef
	Struct        extraSetFieldStruct `orm:"ignore"`
	StructSlice   []extraSetFieldStruct
	Unsupported   map[string]string `orm:"ignore"`
	privateString string
}

func TestExtraWhereScenarios(t *testing.T) {
	t.Run("scalar parameters stay positional", func(t *testing.T) {
		where := NewWhere("`Name` = ? AND `Age` > ?", "Ada", 18)

		assert.Equal(t, "`Name` = ? AND `Age` > ?", where.String())
		assert.Equal(t, []interface{}{"Ada", 18}, where.GetParameters())
	})

	t.Run("string slice expands one IN clause", func(t *testing.T) {
		where := NewWhere("`Status` IN ?", []string{"new", "paid", "closed"})

		assert.Equal(t, "`Status` IN (?,?,?)", where.String())
		assert.Equal(t, []interface{}{"new", "paid", "closed"}, where.GetParameters())
	})

	t.Run("integer array expands one IN clause", func(t *testing.T) {
		where := NewWhere("`ID` IN ?", [3]uint64{7, 8, 9})

		assert.Equal(t, "`ID` IN (?,?,?)", where.String())
		assert.Equal(t, []interface{}{uint64(7), uint64(8), uint64(9)}, where.GetParameters())
	})

	t.Run("multiple slices expand matching IN clauses in order", func(t *testing.T) {
		where := NewWhere("`ID` IN ? OR `Name` IN ?", []uint{1, 2}, []string{"a", "b"})

		assert.Equal(t, "`ID` IN (?,?) OR `Name` IN (?,?)", where.String())
		assert.Equal(t, []interface{}{uint(1), uint(2), "a", "b"}, where.GetParameters())
	})

	t.Run("empty slice keeps an empty SQL list", func(t *testing.T) {
		where := NewWhere("`ID` IN ?", []uint64{})

		assert.Equal(t, "`ID` IN ()", where.String())
		assert.Empty(t, where.GetParameters())
	})

	t.Run("byte slice expands as numeric parameters", func(t *testing.T) {
		where := NewWhere("`Blob` IN ?", []byte{1, 2, 3})

		assert.Equal(t, "`Blob` IN (?,?,?)", where.String())
		assert.Equal(t, []interface{}{uint8(1), uint8(2), uint8(3)}, where.GetParameters())
	})

	t.Run("time value stays scalar", func(t *testing.T) {
		moment := time.Date(2026, 4, 30, 10, 11, 12, 0, time.UTC)
		where := NewWhere("`CreatedAt` = ?", moment)

		assert.Equal(t, "`CreatedAt` = ?", where.String())
		assert.Equal(t, []interface{}{moment}, where.GetParameters())
	})

	t.Run("append adds query and parameters", func(t *testing.T) {
		where := NewWhere("`TenantID` = ?", 12)
		where.Append("AND `Status` IN ?", []string{"active", "paused"})

		assert.Equal(t, "`TenantID` = ? AND `Status` IN (?,?)", where.String())
		assert.Equal(t, []interface{}{12, "active", "paused"}, where.GetParameters())
	})

	t.Run("set parameter is one based", func(t *testing.T) {
		where := NewWhere("`A` = ? AND `B` = ?", "first", "second")
		where.SetParameter(2, "changed")

		assert.Equal(t, []interface{}{"first", "changed"}, where.GetParameters())
	})

	t.Run("set parameters replaces the whole parameter list", func(t *testing.T) {
		where := NewWhere("`A` = ? AND `B` = ?", "first", "second")
		where.SetParameters("only", "these", "now")

		assert.Equal(t, []interface{}{"only", "these", "now"}, where.GetParameters())
	})

	t.Run("out of range set parameter panics", func(t *testing.T) {
		where := NewWhere("`A` = ?", "first")

		assert.Panics(t, func() {
			where.SetParameter(2, "missing")
		})
	})

	t.Run("nil parameter currently panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewWhere("`A` IS ?", nil)
		})
	})
}

func TestExtraPagerScenarios(t *testing.T) {
	t.Run("constructor stores current page and size", func(t *testing.T) {
		pager := NewPager(3, 75)

		assert.Equal(t, 3, pager.GetCurrentPage())
		assert.Equal(t, 75, pager.GetPageSize())
	})

	t.Run("increment advances one page at a time", func(t *testing.T) {
		pager := NewPager(1, 10)
		pager.IncrementPage()
		pager.IncrementPage()

		assert.Equal(t, 3, pager.GetCurrentPage())
		assert.Equal(t, 10, pager.GetPageSize())
	})

	t.Run("zero values are preserved", func(t *testing.T) {
		pager := NewPager(0, 0)

		assert.Equal(t, 0, pager.GetCurrentPage())
		assert.Equal(t, 0, pager.GetPageSize())
	})

	t.Run("negative values are preserved", func(t *testing.T) {
		pager := NewPager(-1, -50)
		pager.IncrementPage()

		assert.Equal(t, 0, pager.GetCurrentPage())
		assert.Equal(t, -50, pager.GetPageSize())
	})
}

func TestExtraLocalCacheScenarios(t *testing.T) {
	registry := &Registry{}
	registry.RegisterLocalCache(2)
	validatedRegistry, def, err := registry.Validate()
	require.NoError(t, err)
	t.Cleanup(def)

	engine := validatedRegistry.CreateEngine()
	logger := &testLogHandler{}
	engine.RegisterQueryLogger(logger, false, false, true)
	cache := engine.GetLocalCache()

	t.Run("get set with zero ttl keeps provider value", func(t *testing.T) {
		cache.Clear()
		calls := 0

		value := cache.GetSet("extra:ttl:zero", 0, func() interface{} {
			calls++
			return "first"
		})
		assert.Equal(t, "first", value)

		value = cache.GetSet("extra:ttl:zero", 0, func() interface{} {
			calls++
			return "second"
		})
		assert.Equal(t, "first", value)
		assert.Equal(t, 1, calls)
	})

	t.Run("expired ttl calls provider again", func(t *testing.T) {
		cache.Clear()
		calls := 0

		value := cache.GetSet("extra:ttl:expired", -time.Second, func() interface{} {
			calls++
			return fmt.Sprintf("value-%d", calls)
		})
		assert.Equal(t, "value-1", value)

		value = cache.GetSet("extra:ttl:expired", -time.Second, func() interface{} {
			calls++
			return fmt.Sprintf("value-%d", calls)
		})
		assert.Equal(t, "value-2", value)
		assert.Equal(t, 2, calls)
	})

	t.Run("mget preserves order and misses", func(t *testing.T) {
		cache.Clear()
		cache.MSet("extra:m:1", "a", "extra:m:3", "c")

		assert.Equal(t, []interface{}{"a", nil, "c"}, cache.MGet("extra:m:1", "extra:m:2", "extra:m:3"))
	})

	t.Run("remove accepts multiple keys", func(t *testing.T) {
		cache.Clear()
		cache.MSet("extra:r:1", "a", "extra:r:2", "b", "extra:r:3", "c")
		cache.Remove("extra:r:1", "extra:r:3")

		assert.Equal(t, []interface{}{nil, "b", nil}, cache.MGet("extra:r:1", "extra:r:2", "extra:r:3"))
	})

	t.Run("clear removes all pools", func(t *testing.T) {
		cache.Clear()
		cache.MSet("extra:clear:1", "a", "extra:clear:2", "b")
		require.GreaterOrEqual(t, cache.GetObjectsCount(), 2)

		cache.Clear()
		assert.Equal(t, 0, cache.GetObjectsCount())
	})

	t.Run("request cache is created on demand", func(t *testing.T) {
		requestCache := engine.GetLocalCache(requestCacheKey)
		requestCache.Set("extra:request", "ok")
		value, has := requestCache.Get("extra:request")

		assert.True(t, has)
		assert.Equal(t, "ok", value)
		assert.Equal(t, requestCacheKey, requestCache.GetPoolConfig().GetCode())
		assert.Equal(t, 5000, requestCache.GetPoolConfig().GetLimit())
	})

	t.Run("unregistered local cache pool panics", func(t *testing.T) {
		assert.PanicsWithError(t, "unregistered local cache pool 'missing-extra-cache'", func() {
			engine.GetLocalCache("missing-extra-cache")
		})
	})

	t.Run("lru evicts least recently used key inside same pool", func(t *testing.T) {
		cache.Clear()
		keys := extraLocalCacheKeysInSamePool(cache, 3)

		cache.Set(keys[0], "first")
		cache.Set(keys[1], "second")
		_, has := cache.Get(keys[0])
		require.True(t, has)
		cache.Set(keys[2], "third")

		value, has := cache.Get(keys[0])
		assert.True(t, has)
		assert.Equal(t, "first", value)
		value, has = cache.Get(keys[1])
		assert.False(t, has)
		assert.Nil(t, value)
		value, has = cache.Get(keys[2])
		assert.True(t, has)
		assert.Equal(t, "third", value)
	})

	t.Run("logger receives miss and mutation fields", func(t *testing.T) {
		cache.Clear()
		logger.clear()

		_, has := cache.Get("extra:logs:missing")
		require.False(t, has)
		require.Len(t, logger.Logs, 1)
		assert.Equal(t, "GET", logger.Logs[0]["operation"])
		assert.Equal(t, true, logger.Logs[0]["miss"])

		cache.Set("extra:logs:present", "value")
		require.Len(t, logger.Logs, 2)
		assert.Equal(t, "SET", logger.Logs[1]["operation"])
		assert.Equal(t, "SET extra:logs:present value", logger.Logs[1]["query"])
	})
}

func TestExtraORMSetFieldScenarios(t *testing.T) {
	var entity *extraSetFieldEntity
	var ref *extraSetFieldRef
	engine, def := prepareTables(t, &Registry{}, 5, "", "2.0", entity, ref)
	defer def()

	entity = &extraSetFieldEntity{}
	engine.Load(entity)
	ref = &extraSetFieldRef{}
	engine.Flush(ref)

	t.Run("string accepts nil and scalar formatting", func(t *testing.T) {
		require.NoError(t, entity.SetField("Name", 123))
		assert.Equal(t, "123", entity.Name)
		require.NoError(t, entity.SetField("Name", nil))
		assert.Equal(t, "", entity.Name)
	})

	t.Run("uint accepts string and float values", func(t *testing.T) {
		require.NoError(t, entity.SetField("Count", "42"))
		assert.Equal(t, uint(42), entity.Count)
		require.NoError(t, entity.SetField("Count", float64(43)))
		assert.Equal(t, uint(43), entity.Count)
		require.NoError(t, entity.SetField("Count", nil))
		assert.Equal(t, uint(0), entity.Count)
	})

	t.Run("uint rejects invalid string", func(t *testing.T) {
		assert.EqualError(t, entity.SetField("Count", "not-a-number"), "Count value not-a-number not valid")
	})

	t.Run("uint pointer accepts values and clears nil", func(t *testing.T) {
		require.NoError(t, entity.SetField("CountPtr", "15"))
		require.NotNil(t, entity.CountPtr)
		assert.Equal(t, uint(15), *entity.CountPtr)
		require.NoError(t, entity.SetField("CountPtr", nil))
		assert.Nil(t, entity.CountPtr)
	})

	t.Run("signed int accepts negative string and positive float", func(t *testing.T) {
		require.NoError(t, entity.SetField("Signed", "-7"))
		assert.Equal(t, -7, entity.Signed)
		require.NoError(t, entity.SetField("Signed", float32(8)))
		assert.Equal(t, 8, entity.Signed)
	})

	t.Run("signed int rejects negative float", func(t *testing.T) {
		assert.EqualError(t, entity.SetField("Signed", float64(-8)), "Signed value -8 not valid")
	})

	t.Run("signed pointer accepts and clears values", func(t *testing.T) {
		require.NoError(t, entity.SetField("SignedPtr", "-15"))
		require.NotNil(t, entity.SignedPtr)
		assert.Equal(t, -15, *entity.SignedPtr)
		require.NoError(t, entity.SetField("SignedPtr", "null"))
		assert.Nil(t, entity.SignedPtr)
	})

	t.Run("bool accepts common truthy and falsey values", func(t *testing.T) {
		require.NoError(t, entity.SetField("Enabled", "1"))
		assert.True(t, entity.Enabled)
		require.NoError(t, entity.SetField("Enabled", "false"))
		assert.False(t, entity.Enabled)
		require.NoError(t, entity.SetField("Enabled", nil))
		assert.False(t, entity.Enabled)
	})

	t.Run("bool pointer accepts string and clears nil", func(t *testing.T) {
		require.NoError(t, entity.SetField("EnabledPtr", "true"))
		require.NotNil(t, entity.EnabledPtr)
		assert.True(t, *entity.EnabledPtr)
		require.NoError(t, entity.SetField("EnabledPtr", nil))
		assert.Nil(t, entity.EnabledPtr)
	})

	t.Run("float accepts comma decimal", func(t *testing.T) {
		require.NoError(t, entity.SetField("Ratio", "12,5"))
		assert.Equal(t, 12.5, entity.Ratio)
	})

	t.Run("float rejects invalid text", func(t *testing.T) {
		assert.EqualError(t, entity.SetField("Ratio", "bad"), "Ratio value bad is not valid")
	})

	t.Run("float pointer accepts and clears values", func(t *testing.T) {
		require.NoError(t, entity.SetField("RatioPtr", "3.75"))
		require.NotNil(t, entity.RatioPtr)
		assert.Equal(t, 3.75, *entity.RatioPtr)
		require.NoError(t, entity.SetField("RatioPtr", nil))
		assert.Nil(t, entity.RatioPtr)
	})

	t.Run("time accepts supported layouts", func(t *testing.T) {
		require.NoError(t, entity.SetField("StartedAt", "2026-04-30"))
		assert.Equal(t, "2026-04-30 00:00:00", entity.StartedAt.Format(timeFormat))
		require.NoError(t, entity.SetField("StartedAt", "2026-04-30 12:13:14"))
		assert.Equal(t, "2026-04-30 12:13:14", entity.StartedAt.Format(timeFormat))
		require.NoError(t, entity.SetField("StartedAt", "2026-04-30T09:08:07Z"))
		assert.Equal(t, "2026-04-30 09:08:07", entity.StartedAt.Format(timeFormat))
	})

	t.Run("time rejects invalid layout", func(t *testing.T) {
		assert.EqualError(t, entity.SetField("StartedAt", "30/04/2026"), "StartedAt value 30/04/2026 is not valid")
	})

	t.Run("time pointer accepts string and null alias", func(t *testing.T) {
		require.NoError(t, entity.SetField("StartedAtPtr", "2026-04-30 12:13:14"))
		require.NotNil(t, entity.StartedAtPtr)
		assert.Equal(t, "2026-04-30 12:13:14", entity.StartedAtPtr.Format(timeFormat))
		require.NoError(t, entity.SetField("StartedAtPtr", "nil"))
		assert.Nil(t, entity.StartedAtPtr)
	})

	t.Run("string slice accepts only string slices", func(t *testing.T) {
		require.NoError(t, entity.SetField("Labels", []string{"one", "two"}))
		assert.Equal(t, []string{"one", "two"}, entity.Labels)
		assert.EqualError(t, entity.SetField("Labels", "one,two"), "Labels value one,two not valid")
	})

	t.Run("bytes accept only byte slices", func(t *testing.T) {
		require.NoError(t, entity.SetField("Payload", []byte{1, 2, 3}))
		assert.Equal(t, []byte{1, 2, 3}, entity.Payload)
		assert.EqualError(t, entity.SetField("Payload", "abc"), "Payload value abc not valid")
	})

	t.Run("entity reference accepts entity id string zero and nil", func(t *testing.T) {
		require.NoError(t, entity.SetField("Ref", ref))
		assert.Equal(t, ref, entity.Ref)
		require.NoError(t, entity.SetField("Ref", fmt.Sprintf("%d", ref.ID)))
		require.NotNil(t, entity.Ref)
		assert.Equal(t, ref.ID, entity.Ref.ID)
		require.NoError(t, entity.SetField("Ref", "0"))
		assert.Nil(t, entity.Ref)
		require.NoError(t, entity.SetField("Ref", nil))
		assert.Nil(t, entity.Ref)
	})

	t.Run("entity reference rejects invalid id", func(t *testing.T) {
		assert.EqualError(t, entity.SetField("Ref", "abc"), "Ref value abc is not valid")
	})

	t.Run("struct and slice fields are set directly", func(t *testing.T) {
		require.NoError(t, entity.SetField("Struct", extraSetFieldStruct{Name: "nested"}))
		assert.Equal(t, "nested", entity.Struct.Name)
		require.NoError(t, entity.SetField("StructSlice", []extraSetFieldStruct{{Name: "a"}, {Name: "b"}}))
		assert.Equal(t, []extraSetFieldStruct{{Name: "a"}, {Name: "b"}}, entity.StructSlice)
	})

	t.Run("unsupported map field returns error", func(t *testing.T) {
		assert.EqualError(t, entity.SetField("Unsupported", map[string]string{"a": "b"}), "field Unsupported is not supported")
	})

	t.Run("private field returns public error", func(t *testing.T) {
		assert.EqualError(t, entity.SetField("privateString", "hidden"), "field privateString is not public")
	})

	t.Run("missing field returns not found error", func(t *testing.T) {
		assert.EqualError(t, entity.SetField("DoesNotExist", "x"), "field DoesNotExist not found")
	})

	t.Run("unloaded entity returns entity not loaded", func(t *testing.T) {
		notLoaded := &extraSetFieldEntity{}
		assert.EqualError(t, notLoaded.SetField("Name", "x"), "entity is not loaded")
	})
}

func extraLocalCacheKeysInSamePool(cache *LocalCache, count int) []string {
	keys := []string{"extra:lru:0"}
	target := cache.getLruMutex(keys[0])
	for i := 1; len(keys) < count; i++ {
		key := fmt.Sprintf("extra:lru:%d", i)
		if cache.getLruMutex(key) == target {
			keys = append(keys, key)
		}
	}
	return keys
}
