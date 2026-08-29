package trixorm

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type extraInventoryQuotedInsertEntity struct {
	ORM
	ID     uint
	Order  string
	Group  string
	Select string
}

type extraInventoryManualIDEntity struct {
	ORM
	ID   uint
	Name string `orm:"unique=Name"`
}

type extraInventorySetFieldNumericEntity struct {
	ORM
	ID            uint
	Uint          uint
	Int           int
	Float         float64
	UintNullable  *uint
	IntNullable   *int
	FloatNullable *float64
}

type extraInventoryCloneEmbedded struct {
	Label string
	Count int
}

type extraInventoryCloneEntity struct {
	ORM
	ID   uint
	Name string
	Age  int
	extraInventoryCloneEmbedded
}

type extraInventoryTemporalEntity struct {
	ORM
	ID          uint
	Name        string
	DateTime    time.Time `orm:"time"`
	DateOnly    time.Time
	NullableAt  *time.Time `orm:"time"`
	NullableDay *time.Time
}

type extraInventoryCachedDeleteEntity struct {
	ORM `orm:"localCache;asyncRedisLazyFlush=default"`
	ID  uint

	Name  string       `orm:"unique=Name"`
	Age   uint16       `orm:"index=Age"`
	ByAge *CachedQuery `query:":Age = ? ORDER BY :Age"`
}

type extraInventoryUUIDLazyEntity struct {
	ORM  `orm:"uuid;localCache;redisCache;asyncRedisLazyFlush=default"`
	ID   uint64
	Name string `orm:"unique=Name"`
}

type extraInventoryUUIDInvalidEntity struct {
	ORM `orm:"uuid"`
	ID  uint
}

type extraInventoryMissingRefParentEntity struct {
	ORM `orm:"localCache"`
	ID  uint
	Key string `orm:"unique=Key"`
}

type extraInventoryMissingRefChildEntity struct {
	ORM
	ID     uint
	Name   string
	Parent *extraInventoryMissingRefParentEntity
}

type extraInventoryFakeDeleteEntity struct {
	ORM
	ID         uint
	Name       string `orm:"index=Name"`
	FakeDelete bool
}

func TestExtraBackportInventoryCurrentAPISurface(t *testing.T) {
	t.Run("redis lock and rate limit APIs match current TrixORM surface", func(t *testing.T) {
		_, hasRateLimit := reflect.TypeOf(&RedisCache{}).MethodByName("RateLimit")
		require.True(t, hasRateLimit)
		rateLimit, _ := reflect.TypeOf(&RedisCache{}).MethodByName("RateLimit")
		assert.Equal(t, 4, rateLimit.Type.NumIn())
		assert.Equal(t, reflect.TypeOf(""), rateLimit.Type.In(1))
		assert.Equal(t, reflect.TypeOf(time.Duration(0)), rateLimit.Type.In(2))
		assert.Equal(t, reflect.TypeOf(0), rateLimit.Type.In(3))
		assert.Equal(t, 1, rateLimit.Type.NumOut())
		assert.Equal(t, reflect.TypeOf(false), rateLimit.Type.Out(0))

		obtain, hasObtain := reflect.TypeOf(&Locker{}).MethodByName("Obtain")
		require.True(t, hasObtain)
		assert.Equal(t, 4, obtain.Type.NumIn())
		assert.Equal(t, reflect.TypeOf(""), obtain.Type.In(1))
		assert.Equal(t, reflect.TypeOf(time.Duration(0)), obtain.Type.In(2))
		assert.Equal(t, reflect.TypeOf(time.Duration(0)), obtain.Type.In(3))
		assert.Equal(t, 2, obtain.Type.NumOut())

		obtainContext, hasObtainContext := reflect.TypeOf(&Locker{}).MethodByName("ObtainContext")
		require.True(t, hasObtainContext)
		assert.Equal(t, 5, obtainContext.Type.NumIn())
		assert.Equal(t, reflect.TypeOf((*context.Context)(nil)).Elem(), obtainContext.Type.In(1))
		assert.Equal(t, reflect.TypeOf(""), obtainContext.Type.In(2))
		assert.Equal(t, reflect.TypeOf(time.Duration(0)), obtainContext.Type.In(3))
		assert.Equal(t, reflect.TypeOf(time.Duration(0)), obtainContext.Type.In(4))
		assert.Equal(t, 2, obtainContext.Type.NumOut())

		refresh, hasRefresh := reflect.TypeOf(&Lock{}).MethodByName("Refresh")
		require.True(t, hasRefresh)
		assert.Equal(t, 2, refresh.Type.NumIn())
		assert.Equal(t, reflect.TypeOf((*context.Context)(nil)).Elem(), refresh.Type.In(1))
		assert.Equal(t, 1, refresh.Type.NumOut())
	})

	t.Run("redis search and concrete engine are retained", func(t *testing.T) {
		assert.Equal(t, reflect.Struct, reflect.TypeOf(Engine{}).Kind())

		engineType := reflect.TypeOf(&Engine{})
		for _, method := range []string{
			"RedisSearch",
			"RedisSearchIds",
			"RedisSearchAggregate",
			"NewRedisSearchIndexPusher",
		} {
			_, hasMethod := engineType.MethodByName(method)
			assert.True(t, hasMethod, method)
		}
	})

	t.Run("registry validate still returns cleanup and sentinel options are exposed", func(t *testing.T) {
		validate, hasValidate := reflect.TypeOf(&Registry{}).MethodByName("Validate")
		require.True(t, hasValidate)
		assert.Equal(t, 3, validate.Type.NumOut())

		sentinelOptionsMethod, hasSentinelOptions := reflect.TypeOf(&Registry{}).MethodByName("RegisterRedisSentinelWithOptions")
		require.True(t, hasSentinelOptions)
		assert.True(t, sentinelOptionsMethod.Type.IsVariadic())
		assert.Equal(t, 6, sentinelOptionsMethod.Type.NumIn())
		assert.Equal(t, "string", sentinelOptionsMethod.Type.In(1).String())
		assert.Equal(t, "redis.FailoverOptions", sentinelOptionsMethod.Type.In(2).String())
		assert.Equal(t, "int", sentinelOptionsMethod.Type.In(3).String())
		assert.Equal(t, "[]string", sentinelOptionsMethod.Type.In(4).String())
		assert.Equal(t, "[]string", sentinelOptionsMethod.Type.In(5).String())
	})

	t.Run("many dirty and fake-delete compatibility APIs are retained", func(t *testing.T) {
		engineType := reflect.TypeOf(&Engine{})
		for _, method := range []string{
			"FlushMany",
			"FlushLazyMany",
			"DeleteMany",
			"ForceDeleteMany",
			"MarkDirty",
			"SearchWithFakeDeleted",
		} {
			_, hasMethod := engineType.MethodByName(method)
			assert.True(t, hasMethod, method)
		}

		showFakeDeleted, hasWhereShowFakeDeleted := reflect.TypeOf(&Where{}).MethodByName("ShowFakeDeleted")
		require.True(t, hasWhereShowFakeDeleted)
		assert.Equal(t, 1, showFakeDeleted.Type.NumOut())
		assert.Equal(t, reflect.TypeOf(&Where{}), showFakeDeleted.Type.Out(0))
	})

	t.Run("category one helper APIs are now present", func(t *testing.T) {
		clone, hasEntityClone := reflect.TypeOf(&extraInventoryManualIDEntity{}).MethodByName("Clone")
		require.True(t, hasEntityClone)
		assert.Equal(t, 1, clone.Type.NumOut())
		assert.Equal(t, reflect.TypeOf((*Entity)(nil)).Elem(), clone.Type.Out(0))

		assert.Equal(t, reflect.Func, reflect.ValueOf(SetUUIDServerID).Kind())
	})

	t.Run("category three helper APIs are now present", func(t *testing.T) {
		_, hasIsToDelete := reflect.TypeOf(&ORM{}).MethodByName("IsToDelete")
		assert.True(t, hasIsToDelete)

		_, hasLazyResolver := reflect.TypeOf(&BackgroundConsumer{}).MethodByName("RegisterLazyFlushQueryErrorResolver")
		assert.True(t, hasLazyResolver)

		flusherType := reflect.TypeOf((*Flusher)(nil)).Elem()
		_, hasCancelDelete := flusherType.MethodByName("CancelDelete")
		assert.True(t, hasCancelDelete)
	})

	t.Run("category four helper APIs are now present", func(t *testing.T) {
		_, hasPagerString := reflect.TypeOf(&Pager{}).MethodByName("String")
		assert.True(t, hasPagerString)

		_, hasSetBlockTime := reflect.TypeOf(&BackgroundConsumer{}).MethodByName("SetBlockTime")
		assert.True(t, hasSetBlockTime)

		eventBrokerType := reflect.TypeOf((*EventBroker)(nil)).Elem()
		for _, method := range []string{"GetStreamsStatistics", "GetStreamStatistics", "GetStreamGroupStatistics"} {
			_, hasMethod := eventBrokerType.MethodByName(method)
			assert.True(t, hasMethod, method)
		}

		eventConsumerType := reflect.TypeOf((*EventsConsumer)(nil)).Elem()
		_, hasConsumerSetBlockTime := eventConsumerType.MethodByName("SetBlockTime")
		assert.True(t, hasConsumerSetBlockTime)

		_, hasEntityLogs := reflect.TypeOf((*TableSchema)(nil)).Elem().MethodByName("GetEntityLogs")
		assert.True(t, hasEntityLogs)
	})

	t.Run("later helper APIs are still absent from the current surface", func(t *testing.T) {
		_, hasSetMemoryOnly := reflect.TypeOf(&ORM{}).MethodByName("SetMemoryOnly")
		assert.False(t, hasSetMemoryOnly)
	})
}

func TestExtraBackportInventoryShowFakeDeletedWhereCompatibility(t *testing.T) {
	var entity *extraInventoryFakeDeleteEntity
	engine, def := prepareTables(t, &Registry{}, 8, "", "2.0", entity)
	defer def()

	visible := &extraInventoryFakeDeleteEntity{Name: "visible"}
	deleted := &extraInventoryFakeDeleteEntity{Name: "deleted"}
	engine.FlushMany(visible, deleted)
	engine.Delete(deleted)

	var rows []*extraInventoryFakeDeleteEntity
	engine.Search(NewWhere("`ID` > 0 ORDER BY `ID`"), nil, &rows)
	require.Len(t, rows, 1)
	assert.Equal(t, "visible", rows[0].Name)

	engine.Search(NewWhere("`ID` > 0 ORDER BY `ID`").ShowFakeDeleted(), nil, &rows)
	require.Len(t, rows, 2)
	assert.Equal(t, []string{"visible", "deleted"}, []string{rows[0].Name, rows[1].Name})

	engine.SearchWithFakeDeleted(NewWhere("`ID` > 0 ORDER BY `ID`"), nil, &rows)
	require.Len(t, rows, 2)
	assert.Equal(t, []string{"visible", "deleted"}, []string{rows[0].Name, rows[1].Name})

	ids := engine.SearchIDs(NewWhere("`ID` > 0 ORDER BY `ID`").ShowFakeDeleted(), nil, &extraInventoryFakeDeleteEntity{})
	assert.Equal(t, []uint64{uint64(visible.ID), uint64(deleted.ID)}, ids)

	rows = nil
	total := engine.SearchWithCount(NewWhere("`ID` > 0 ORDER BY `ID`").ShowFakeDeleted(), NewPager(1, 1), &rows)
	assert.Equal(t, 2, total)
	require.Len(t, rows, 1)
	assert.Equal(t, "visible", rows[0].Name)

	one := &extraInventoryFakeDeleteEntity{}
	assert.False(t, engine.SearchOne(NewWhere("`Name` = ?", "deleted"), one))
	assert.True(t, engine.SearchOne(NewWhere("`Name` = ?", "deleted").ShowFakeDeleted(), one))
	assert.Equal(t, deleted.ID, one.ID)
}

func TestExtraBackportInventoryInsertColumnBackticks(t *testing.T) {
	var entity *extraInventoryQuotedInsertEntity
	engine, def := prepareTables(t, &Registry{}, 8, "", "2.0", entity)
	defer def()

	logger := &testLogHandler{}
	engine.RegisterQueryLogger(logger, true, false, false)

	engine.Flush(&extraInventoryQuotedInsertEntity{
		Order:  "first",
		Group:  "alpha",
		Select: "visible",
	})

	insertQuery := extraInventoryFindLoggedQuery(t, logger, "INSERT INTO `extraInventoryQuotedInsertEntity`")
	columnList := insertQuery[strings.Index(insertQuery, "(")+1 : strings.Index(insertQuery, ")")]
	columns := strings.Split(columnList, ",")
	require.ElementsMatch(t, []string{"`Order`", "`Group`", "`Select`"}, columns)
	for _, column := range columns {
		assert.True(t, strings.HasPrefix(column, "`"), column)
		assert.True(t, strings.HasSuffix(column, "`"), column)
	}
}

func TestExtraBackportInventorySetFieldEmptyNumericValues(t *testing.T) {
	var entity *extraInventorySetFieldNumericEntity
	engine, def := prepareTables(t, &Registry{}, 8, "", "2.0", entity)
	defer def()

	entity = &extraInventorySetFieldNumericEntity{}
	engine.Load(entity)

	require.NoError(t, entity.SetField("Uint", ""))
	assert.Equal(t, uint(0), entity.Uint)

	require.NoError(t, entity.SetField("Int", ""))
	assert.Equal(t, 0, entity.Int)

	require.NoError(t, entity.SetField("Float", ""))
	assert.Equal(t, float64(0), entity.Float)

	validUint := uint(44)
	entity.UintNullable = &validUint
	require.NoError(t, entity.SetField("UintNullable", ""))
	assert.Nil(t, entity.UintNullable)

	validInt := 45
	entity.IntNullable = &validInt
	require.NoError(t, entity.SetField("IntNullable", ""))
	assert.Nil(t, entity.IntNullable)

	validFloat := 46.7
	entity.FloatNullable = &validFloat
	require.NoError(t, entity.SetField("FloatNullable", ""))
	assert.Nil(t, entity.FloatNullable)
}

func TestExtraBackportInventorySetFieldIDFlushesExplicitID(t *testing.T) {
	var entity *extraInventoryManualIDEntity
	engine, def := prepareTables(t, &Registry{}, 8, "", "2.0", entity)
	defer def()

	entity = &extraInventoryManualIDEntity{}
	engine.Load(entity)
	require.NoError(t, entity.SetField("ID", "4242"))
	require.NoError(t, entity.SetField("Name", "manual-id"))
	engine.Flush(entity)

	loaded := &extraInventoryManualIDEntity{}
	require.True(t, engine.LoadByID(4242, loaded))
	assert.Equal(t, uint(4242), loaded.ID)
	assert.Equal(t, "manual-id", loaded.Name)
}

func TestExtraBackportInventoryEntityCloneCopiesFieldsWithoutID(t *testing.T) {
	var entity *extraInventoryCloneEntity
	engine, def := prepareTables(t, &Registry{}, 8, "", "2.0", entity)
	defer def()

	entity = &extraInventoryCloneEntity{
		Name: "source",
		Age:  30,
		extraInventoryCloneEmbedded: extraInventoryCloneEmbedded{
			Label: "embedded",
			Count: 7,
		},
	}
	engine.Flush(entity)

	cloned := entity.Clone().(*extraInventoryCloneEntity)
	assert.Equal(t, uint(0), cloned.ID)
	assert.Equal(t, "source", cloned.Name)
	assert.Equal(t, 30, cloned.Age)
	assert.Equal(t, "embedded", cloned.Label)
	assert.Equal(t, 7, cloned.Count)

	cloned.Name = "clone"
	cloned.Label = "changed"
	assert.Equal(t, "source", entity.Name)
	assert.Equal(t, "embedded", entity.Label)
}

func TestExtraBackportInventoryPost2038DateTimeRoundTrip(t *testing.T) {
	var entity *extraInventoryTemporalEntity
	engine, def := prepareTables(t, &Registry{}, 8, "", "2.0", entity)
	defer def()

	future := time.Date(2042, 6, 7, 8, 9, 10, 0, time.Local)
	row := &extraInventoryTemporalEntity{
		Name:        "future",
		DateTime:    future,
		DateOnly:    future,
		NullableAt:  &future,
		NullableDay: &future,
	}
	engine.Flush(row)

	loaded := &extraInventoryTemporalEntity{}
	require.True(t, engine.LoadByID(uint64(row.ID), loaded))
	assert.Equal(t, "2042-06-07 08:09:10", loaded.DateTime.Format(timeFormat))
	assert.Equal(t, "2042-06-07", loaded.DateOnly.Format(dateformat))
	require.NotNil(t, loaded.NullableAt)
	assert.Equal(t, "2042-06-07 08:09:10", loaded.NullableAt.Format(timeFormat))
	require.NotNil(t, loaded.NullableDay)
	assert.Equal(t, "2042-06-07", loaded.NullableDay.Format(dateformat))
	assert.False(t, loaded.IsDirty())

	loaded.Name = "future-updated"
	engine.Flush(loaded)

	reloaded := &extraInventoryTemporalEntity{}
	require.True(t, engine.LoadByID(uint64(row.ID), reloaded))
	assert.Equal(t, "future-updated", reloaded.Name)
	assert.Equal(t, "2042-06-07 08:09:10", reloaded.DateTime.Format(timeFormat))
	assert.Equal(t, "2042-06-07", reloaded.DateOnly.Format(dateformat))
}

func TestExtraBackportInventoryMissingReferenceCachesNil(t *testing.T) {
	var parent *extraInventoryMissingRefParentEntity
	var child *extraInventoryMissingRefChildEntity
	engine, def := prepareTables(t, &Registry{}, 8, "extra_inventory_missing_ref", "2.0", parent, child)
	defer def()

	parent = &extraInventoryMissingRefParentEntity{Key: "missing-parent"}
	child = &extraInventoryMissingRefChildEntity{Name: "child", Parent: parent}
	engine.FlushMany(parent, child)

	engine.GetMysql().Exec("SET FOREIGN_KEY_CHECKS = 0")
	engine.GetMysql().Exec("DELETE FROM `extraInventoryMissingRefParentEntity` WHERE `ID` = ?", parent.ID)
	engine.GetMysql().Exec("SET FOREIGN_KEY_CHECKS = 1")
	engine.GetLocalCache().Clear()

	loaded := &extraInventoryMissingRefChildEntity{}
	require.True(t, engine.LoadByID(uint64(child.ID), loaded, "Parent"))
	require.NotNil(t, loaded.Parent)
	assert.False(t, loaded.Parent.IsLoaded())

	loadedAgain := &extraInventoryMissingRefChildEntity{}
	assert.NotPanics(t, func() {
		require.True(t, engine.LoadByID(uint64(child.ID), loadedAgain, "Parent"))
	})
	require.NotNil(t, loadedAgain.Parent)
	assert.False(t, loadedAgain.Parent.IsLoaded())
}

func TestExtraBackportInventoryCachedSearchLazyDeleteNoNilRows(t *testing.T) {
	var entity *extraInventoryCachedDeleteEntity
	engine, def := prepareTables(t, &Registry{}, 8, "extra_inventory_cached_delete", "2.0", entity)
	defer def()
	engine.GetRedis().FlushDB()

	rows := []*extraInventoryCachedDeleteEntity{
		{Name: "alpha", Age: 44},
		{Name: "bravo", Age: 44},
		{Name: "charlie", Age: 44},
		{Name: "other", Age: 99},
	}
	flusher := engine.NewFlusher()
	for _, row := range rows {
		flusher.Track(row)
	}
	flusher.Flush()

	var found []*extraInventoryCachedDeleteEntity
	total := engine.CachedSearch(&found, "ByAge", NewPager(1, 10), 44)
	require.Equal(t, 3, total)
	require.Len(t, found, 3)

	engine.DeleteLazy(rows[1])
	found = nil
	total = engine.CachedSearch(&found, "ByAge", nil, 44)
	require.Equal(t, 2, total)
	require.Len(t, found, 2)
	for _, row := range found {
		require.NotNil(t, row)
	}
	assert.ElementsMatch(t, []string{"alpha", "charlie"}, []string{found[0].Name, found[1].Name})

	receiver := NewBackgroundConsumer(engine)
	receiver.DisableLoop()
	receiver.blockTime = time.Millisecond
	receiver.Digest(context.Background())

	found = nil
	total = engine.CachedSearch(&found, "ByAge", NewPager(1, 10), 44)
	require.Equal(t, 2, total)
	require.Len(t, found, 2)
	for _, row := range found {
		require.NotNil(t, row)
	}
	assert.ElementsMatch(t, []string{"alpha", "charlie"}, []string{found[0].Name, found[1].Name})
}

func TestExtraBackportInventoryCachedSearchReferenceLazyDelete(t *testing.T) {
	var entity *cachedSearchEntity
	var reference *cachedSearchRefEntity
	engine, def := prepareTables(t, &Registry{}, 8, "extra_inventory_cached_reference_delete", "2.0", reference, entity)
	defer def()
	engine.GetRedis().FlushDB()
	schema := engine.GetRegistry().GetTableSchemaForEntity(entity).(*tableSchema)
	schema.localCacheName = "default"
	schema.hasLocalCache = true

	reference = &cachedSearchRefEntity{Name: "reference-row"}
	engine.Flush(reference)
	entity = &cachedSearchEntity{Name: "indexed-row", Age: 33, ReferenceOne: reference}
	engine.Flush(entity)

	var rows []*cachedSearchEntity
	totalRows := engine.CachedSearch(&rows, "IndexReference", nil, reference.ID)
	require.Equal(t, 1, totalRows)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0])

	engine.DeleteLazy(entity)
	receiver := NewBackgroundConsumer(engine)
	receiver.DisableLoop()
	receiver.blockTime = time.Millisecond
	receiver.Digest(context.Background())

	rows = nil
	totalRows = engine.CachedSearch(&rows, "IndexReference", nil, reference.ID)
	assert.Equal(t, 0, totalRows)
	assert.Empty(t, rows)
}

func TestExtraBackportInventoryLazyFlushTransactionAfterCommit(t *testing.T) {
	var entity *lazyReceiverEntity
	var reference *lazyReceiverReference
	registry := NewRegistry()
	registry.RegisterEnum("trixorm.TestEnum", []string{"a", "b", "c"})
	engine, def := prepareTables(t, registry, 5, "extra_inventory_lazy_tx", "2.0", entity, reference)
	defer def()
	engine.GetRedis().FlushDB()

	row := &lazyReceiverEntity{Name: "before", Age: 1}
	engine.Flush(row)
	loaded := &lazyReceiverEntity{}
	require.True(t, engine.LoadByID(uint64(row.ID), loaded))
	loaded.Name = "after"

	receiver := NewBackgroundConsumer(engine)
	receiver.DisableLoop()
	receiver.blockTime = time.Millisecond

	engine.GetMysql().Begin()
	engine.FlushLazy(loaded)
	engine.GetMysql().Commit()

	sample := receiver.GetLazyFlushEventsSample(10)
	require.Len(t, sample, 1)
	parts := strings.SplitN(sample[0], "|", 2)
	require.Len(t, parts, 2)
	assert.NotEmpty(t, parts[0])
	assert.Contains(t, parts[1], "UPDATE `lazyReceiverEntity`")

	receiver.Digest(context.Background())
	engine.GetLocalCache().Clear()
	engine.GetRedis().FlushDB()

	reloaded := &lazyReceiverEntity{}
	require.True(t, engine.LoadByID(uint64(row.ID), reloaded))
	assert.Equal(t, "after", reloaded.Name)
}

func TestExtraBackportInventoryCancelDelete(t *testing.T) {
	var entity *extraInventoryManualIDEntity
	engine, def := prepareTables(t, &Registry{}, 8, "", "2.0", entity)
	defer def()

	cancelled := &extraInventoryManualIDEntity{Name: "cancelled"}
	engine.Flush(cancelled)

	flusher := engine.NewFlusher().Delete(cancelled)
	assert.True(t, cancelled.IsToDelete())

	flusher.CancelDelete(cancelled)
	assert.False(t, cancelled.IsToDelete())

	flusher.Flush()

	loadedCancelled := &extraInventoryManualIDEntity{}
	found := engine.LoadByID(uint64(cancelled.ID), loadedCancelled)
	assert.True(t, found)
	assert.Equal(t, "cancelled", loadedCancelled.Name)
}

func TestExtraBackportInventoryLazyFlushUpdateErrorResolver(t *testing.T) {
	prepare := func(t *testing.T, namespace string) (*Engine, func()) {
		t.Helper()
		var entity *lazyReceiverEntity
		var reference *lazyReceiverReference
		registry := NewRegistry()
		registry.RegisterEnum("trixorm.TestEnum", []string{"a", "b", "c"})
		return prepareTables(t, registry, 5, namespace, "2.0", entity, reference)
	}
	seedDuplicateLazyUpdate := func(t *testing.T, engine *Engine) *lazyReceiverEntity {
		t.Helper()
		taken := &lazyReceiverEntity{Name: "taken", Age: 10}
		engine.Flush(taken)
		row := &lazyReceiverEntity{Name: "candidate", Age: 11}
		engine.Flush(row)
		loaded := &lazyReceiverEntity{}
		require.True(t, engine.LoadByID(uint64(row.ID), loaded))
		loaded.Name = "taken"
		engine.FlushLazy(loaded)
		return row
	}

	t.Run("unresolved lazy update errors still panic", func(t *testing.T) {
		engine, def := prepare(t, "extra_inventory_lazy_error_panic")
		defer def()
		seedDuplicateLazyUpdate(t, engine)
		receiver := NewBackgroundConsumer(engine)
		receiver.DisableLoop()
		receiver.blockTime = time.Millisecond
		var panicValue interface{}
		func() {
			defer func() {
				panicValue = recover()
			}()
			receiver.Digest(context.Background())
		}()
		require.NotNil(t, panicValue)
		panicError, ok := panicValue.(error)
		require.True(t, ok)
		assert.Contains(t, panicError.Error(), "Duplicate entry")
	})

	t.Run("resolver can accept a lazy update mysql error", func(t *testing.T) {
		engine, def := prepare(t, "extra_inventory_lazy_error_resolved")
		defer def()
		row := seedDuplicateLazyUpdate(t, engine)
		receiver := NewBackgroundConsumer(engine)
		receiver.DisableLoop()
		receiver.blockTime = time.Millisecond
		called := false
		receiver.RegisterLazyFlushQueryErrorResolver(func(engine *Engine, db *DB, sql string, queryError *mysql.MySQLError) error {
			called = true
			assert.NotNil(t, engine)
			assert.Equal(t, "default", db.GetPoolConfig().GetCode())
			assert.Contains(t, sql, "UPDATE `lazyReceiverEntity`")
			assert.Equal(t, uint16(1062), queryError.Number)
			return nil
		})
		assert.NotPanics(t, func() {
			receiver.Digest(context.Background())
		})
		assert.True(t, called)

		engine.GetLocalCache().Clear()
		loaded := &lazyReceiverEntity{}
		require.True(t, engine.LoadByID(uint64(row.ID), loaded))
		assert.Equal(t, "candidate", loaded.Name)
	})
}

func TestExtraBackportInventoryUUIDLazyFlushRoundTrip(t *testing.T) {
	var entity *extraInventoryUUIDLazyEntity
	engine, def := prepareTables(t, &Registry{}, 8, "extra_inventory_uuid", "2.0", entity)
	defer def()
	engine.GetRedis().FlushDB()

	row := &extraInventoryUUIDLazyEntity{Name: "uuid-lazy"}
	engine.FlushLazy(row)
	require.NotZero(t, row.ID)

	beforeDigest := &extraInventoryUUIDLazyEntity{}
	require.True(t, engine.LoadByID(row.ID, beforeDigest))
	assert.Equal(t, "uuid-lazy", beforeDigest.Name)

	receiver := NewBackgroundConsumer(engine)
	receiver.DisableLoop()
	receiver.blockTime = time.Millisecond
	receiver.Digest(context.Background())

	engine.GetLocalCache().Clear()
	loaded := &extraInventoryUUIDLazyEntity{}
	require.True(t, engine.LoadByID(row.ID, loaded))
	assert.Equal(t, row.ID, loaded.ID)
	assert.Equal(t, "uuid-lazy", loaded.Name)
}

func TestExtraBackportInventoryUUIDSchemaAndServerID(t *testing.T) {
	registry := &Registry{}
	registry.RegisterMySQLPool("root:root@tcp(localhost:3312)/test")
	registry.RegisterEntity(&extraInventoryUUIDInvalidEntity{})
	_, def, err := registry.Validate()
	if def != nil {
		defer def()
	}
	require.EqualError(t, err, "entity trixorm.extraInventoryUUIDInvalidEntity with uuid enabled must be unit64")

	original := uuid()
	SetUUIDServerID(1)
	withServerID := uuid()
	assert.Equal(t, uint64(1), withServerID>>56)
	assert.Greater(t, withServerID, original)
	SetUUIDServerID(0)
}

func TestExtraBackportInventoryCachePrefixUsesFinalBeeORMLength(t *testing.T) {
	var entity *extraInventoryManualIDEntity
	engine, def := prepareTables(t, &Registry{}, 8, "", "2.0", entity)
	defer def()

	schema := engine.GetRegistry().GetTableSchemaForEntity(entity).(*tableSchema)
	assert.Len(t, schema.cachePrefix, 5)
	assert.Regexp(t, `^[0-9a-f]{5}:\d+$`, schema.getCacheKey(123))
}

func TestExtraBackportInventoryRedisNamespaceRegressionScenarios(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterRedis("localhost:6382", "extra_inventory_ns", 15)
	registry.RegisterRedisStream("extra-inventory-stream", "default", []string{"extra-inventory-group"})
	validatedRegistry, def, err := registry.Validate()
	require.NoError(t, err)
	defer def()

	engine := validatedRegistry.CreateEngine()
	redisCache := engine.GetRedis()
	redisCache.FlushDB()

	t.Run("mget with namespace does not mutate caller keys", func(t *testing.T) {
		redisCache.MSet("one", "1", "two", "2")
		keys := []string{"one", "missing", "two"}
		values := redisCache.MGet(keys...)

		assert.Equal(t, []string{"one", "missing", "two"}, keys)
		assert.Equal(t, []interface{}{"1", nil, "2"}, values)
	})

	t.Run("zrange args namespace is applied without mutating input args", func(t *testing.T) {
		redisCache.Del("scores")
		redisCache.ZAdd("scores", &redis.Z{Score: 1, Member: "low"}, &redis.Z{Score: 2, Member: "mid"}, &redis.Z{Score: 3, Member: "high"})

		args := redis.ZRangeArgs{Key: "scores", Start: 0, Stop: -1}
		values := redisCache.ZRangeArgs(args)

		assert.Equal(t, "scores", args.Key)
		assert.Equal(t, []string{"low", "mid", "high"}, values)

		withScoresArgs := redis.ZRangeArgs{Key: "scores", Start: 1, Stop: 2}
		valuesWithScores := redisCache.ZRangeArgsWithScores(withScoresArgs)

		assert.Equal(t, "scores", withScoresArgs.Key)
		require.Len(t, valuesWithScores, 2)
		assert.Equal(t, "mid", valuesWithScores[0].Member)
		assert.Equal(t, float64(2), valuesWithScores[0].Score)
		assert.Equal(t, "high", valuesWithScores[1].Member)
		assert.Equal(t, float64(3), valuesWithScores[1].Score)
	})

	t.Run("zrem range by rank is namespaced", func(t *testing.T) {
		redisCache.Del("ranked")
		redisCache.ZAdd("ranked", &redis.Z{Score: 1, Member: "a"}, &redis.Z{Score: 2, Member: "b"}, &redis.Z{Score: 3, Member: "c"})

		removed := redisCache.ZRemRangeByRank("ranked", 0, 0)
		assert.Equal(t, int64(1), removed)
		assert.Equal(t, []string{"b", "c"}, redisCache.ZRangeArgs(redis.ZRangeArgs{Key: "ranked", Start: 0, Stop: -1}))
		assert.Equal(t, int64(0), redisCache.client.Exists(context.Background(), "ranked").Val())
		assert.Equal(t, int64(1), redisCache.client.Exists(context.Background(), "extra_inventory_ns:ranked").Val())
	})
}

func TestExtraBackportInventoryRedisCredentialOptions(t *testing.T) {
	t.Run("direct redis credentials populate go redis options when username is set", func(t *testing.T) {
		registry := NewRegistry()
		registry.RegisterRedisWithCredentials("localhost:6382", "tenant", "user", "pass", 15, "auth")

		options := registry.redisPools["auth"].getClient().Options()
		assert.Equal(t, "localhost:6382", options.Addr)
		assert.Equal(t, "user", options.Username)
		assert.Equal(t, "pass", options.Password)
		assert.Equal(t, 15, options.DB)
	})

	t.Run("direct redis credentials allow password without username", func(t *testing.T) {
		registry := NewRegistry()
		registry.RegisterRedisWithCredentials("localhost:6382", "tenant", "", "pass-only", 15, "auth")

		options := registry.redisPools["auth"].getClient().Options()
		assert.Equal(t, "localhost:6382", options.Addr)
		assert.Equal(t, "", options.Username)
		assert.Equal(t, "pass-only", options.Password)
		assert.Equal(t, 15, options.DB)
	})

	t.Run("direct redis options registration carries pool settings", func(t *testing.T) {
		registry := NewRegistry()
		registry.RegisterRedisWithOptions("tenant", redis.Options{
			Addr:         "localhost:6382",
			DB:           15,
			PoolSize:     64,
			MinIdleConns: 8,
			PoolTimeout:  10 * time.Second,
		}, "streams")

		config := registry.redisPools["streams"]
		options := config.getClient().Options()
		assert.Equal(t, "tenant", config.GetNamespace())
		assert.Equal(t, "localhost:6382", config.GetAddress())
		assert.Equal(t, 15, config.GetDatabase())
		assert.Equal(t, 64, options.PoolSize)
		assert.Equal(t, 8, options.MinIdleConns)
		assert.Equal(t, 10*time.Second, options.PoolTimeout)
		assert.Equal(t, 2*time.Minute, options.MaxConnAge)
	})

	t.Run("sentinel credentials populate failover client options when username is set", func(t *testing.T) {
		registry := NewRegistry()
		registry.RegisterRedisSentinelWithCredentials("master", "tenant", "user", "pass", 4, []string{"s1:26379", "s2:26379"}, "sentinel")

		options := registry.redisPools["sentinel"].getClient().Options()
		assert.Equal(t, "user", options.Username)
		assert.Equal(t, "pass", options.Password)
		assert.Equal(t, 4, options.DB)
	})

	t.Run("sentinel credentials allow password without username", func(t *testing.T) {
		registry := NewRegistry()
		registry.RegisterRedisSentinelWithCredentials("master", "tenant", "", "pass-only", 4, []string{"s1:26379", "s2:26379"}, "sentinel")

		options := registry.redisPools["sentinel"].getClient().Options()
		assert.Equal(t, "", options.Username)
		assert.Equal(t, "pass-only", options.Password)
		assert.Equal(t, 4, options.DB)
	})

	t.Run("sentinel options registration carries provided options into pool config", func(t *testing.T) {
		registry := NewRegistry()
		registry.RegisterRedisSentinelWithOptions("tenant", redis.FailoverOptions{
			MasterName: "master",
			Username:   "user",
			Password:   "pass",
			MaxRetries: 6,
		}, 4, []string{"s1:26379", "s2:26379"}, "sentinel")

		config := registry.redisPools["sentinel"]
		options := config.getClient().Options()
		assert.Equal(t, "tenant", config.GetNamespace())
		assert.True(t, config.HasNamespace())
		assert.Equal(t, "[s1:26379 s2:26379]", config.GetAddress())
		assert.Equal(t, 4, config.GetDatabase())
		assert.Equal(t, "user", options.Username)
		assert.Equal(t, "pass", options.Password)
		assert.Equal(t, 4, options.DB)
		assert.Equal(t, 6, options.MaxRetries)
		assert.Equal(t, time.Minute*2, options.MaxConnAge)
	})
}

func TestExtraBackportInventoryLockWaitRefreshAndRelease(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterRedis("localhost:6382", "extra_inventory_lock", 15)
	validatedRegistry, def, err := registry.Validate()
	require.NoError(t, err)
	defer def()

	engine := validatedRegistry.CreateEngine()
	redisCache := engine.GetRedis()
	redisCache.FlushDB()
	locker := redisCache.GetLocker()

	lock, obtained := locker.ObtainContext(context.Background(), "shared", time.Second, 0)
	require.True(t, obtained)
	require.NotNil(t, lock)
	defer lock.Release()

	contender, contenderObtained := locker.ObtainContext(context.Background(), "shared", time.Second, 20*time.Millisecond)
	assert.False(t, contenderObtained)
	assert.Nil(t, contender)
	assert.Greater(t, lock.TTL(), time.Duration(0))
	assert.True(t, lock.Refresh(context.Background()))

	lock.Release()
	contender, contenderObtained = locker.ObtainContext(context.Background(), "shared", time.Second, 200*time.Millisecond)
	require.True(t, contenderObtained)
	contender.Release()
}

func extraInventoryFindLoggedQuery(t *testing.T, logger *testLogHandler, needle string) string {
	t.Helper()

	for _, logRow := range logger.Logs {
		query, ok := logRow["query"].(string)
		if ok && strings.Contains(query, needle) {
			return query
		}
	}
	require.Failf(t, "query not found", "missing query containing %q in %#v", needle, logger.Logs)
	return ""
}

func TestExtraBackportInventoryStreamsBackgroundConsumersAndLogs(t *testing.T) {
	t.Run("pager string and default block time", func(t *testing.T) {
		assert.Equal(t, "LIMIT 20,10", NewPager(3, 10).String())
		assert.Equal(t, "orm-async-consumer", AsyncConsumerGroupName)
		assert.Equal(t, AsyncConsumerGroupName, BackgroundConsumerGroupName)

		backgroundConsumer := NewBackgroundConsumer(&Engine{})
		assert.Equal(t, time.Second*10, backgroundConsumer.blockTime)
		backgroundConsumer.SetBlockTime(time.Millisecond * 25)
		assert.Equal(t, time.Millisecond*25, backgroundConsumer.blockTime)
	})

	t.Run("event broker exposes stream statistics and configurable block time", func(t *testing.T) {
		registry := NewRegistry()
		registry.RegisterRedis("localhost:6382", "extra_inventory_streams", 15)
		registry.RegisterRedisStream("extra-inventory-stream", "default", []string{"extra-inventory-group"})
		validatedRegistry, def, err := registry.Validate()
		require.NoError(t, err)
		defer def()

		engine := validatedRegistry.CreateEngine()
		redisCache := engine.GetRedis()
		redisCache.FlushDB()

		eventBroker := engine.GetEventBroker()
		eventsConsumer := eventBroker.Consumer("extra-inventory-group").(*eventsConsumer)
		assert.Equal(t, time.Second*10, eventsConsumer.blockTime)
		eventsConsumer.SetBlockTime(time.Millisecond * 50)
		assert.Equal(t, time.Millisecond*50, eventsConsumer.blockTime)

		redisCache.XGroupCreateMkStream("extra-inventory-stream", "extra-inventory-group", "0")
		eventID := eventBroker.Publish("extra-inventory-stream", map[string]string{"kind": "test"})
		require.NotEmpty(t, eventID)

		stats := eventBroker.GetStreamsStatistics("extra-inventory-stream")
		require.Len(t, stats, 1)
		assert.Equal(t, "extra-inventory-stream", stats[0].Stream)
		assert.Equal(t, "default", stats[0].RedisPool)
		assert.Equal(t, uint64(1), stats[0].Len)

		streamStats := eventBroker.GetStreamStatistics("extra-inventory-stream")
		require.NotNil(t, streamStats)
		assert.Equal(t, stats[0].Len, streamStats.Len)

		groupStats := eventBroker.GetStreamGroupStatistics("extra-inventory-stream", "extra-inventory-group")
		require.NotNil(t, groupStats)
		assert.Equal(t, "extra-inventory-group", groupStats.Group)
		assert.Equal(t, uint64(0), groupStats.Pending)

		missingGroupStats := eventBroker.GetStreamGroupStatistics("extra-inventory-stream", "missing-group")
		require.NotNil(t, missingGroupStats)
		assert.Equal(t, "missing-group", missingGroupStats.Group)
		assert.Equal(t, int64(1), missingGroupStats.Lag)
	})

	t.Run("entity log getter fills date and decoded payloads", func(t *testing.T) {
		var entity *logReceiverEntity1
		registry := NewRegistry()
		registry.ForceEntityLogInAllEntities("default")
		engine, def := prepareTables(t, registry, 8, "", "2.0", entity)
		defer def()
		engine.GetRedis().FlushDB()

		backgroundConsumer := NewBackgroundConsumer(engine)
		backgroundConsumer.DisableLoop()
		backgroundConsumer.SetBlockTime(time.Millisecond)

		entity = &logReceiverEntity1{Name: "Logged", LastName: "Entity", Country: "MK"}
		engine.Flush(entity)
		backgroundConsumer.Digest(context.Background())

		schema := engine.GetRegistry().GetTableSchemaForEntity(entity)
		logs := schema.GetEntityLogs(engine, uint64(entity.ID), NewPager(1, 10), nil)
		require.Len(t, logs, 1)
		assert.Equal(t, uint64(entity.ID), logs[0].EntityID)
		assert.False(t, logs[0].Date.IsZero())
		assert.Equal(t, "Logged", logs[0].Changes["Name"])
		assert.Equal(t, "Entity", logs[0].Changes["LastName"])
		assert.Equal(t, "MK", logs[0].Changes["Country"])
	})
}
