package trixorm

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type extraComplexRef struct {
	ORM `orm:"localCache;redisCache"`
	ID  uint
	Key string `orm:"unique=Key"`
}

type extraComplexChild struct {
	ORM   `orm:"localCache;redisCache"`
	ID    uint
	Label string
}

type extraComplexEntity struct {
	ORM        `orm:"localCache;redisCache"`
	ID         uint
	TenantID   uint16 `orm:"index=TenantStatus:1;unique=TenantName:1"`
	Status     string `orm:"length=20;index=TenantStatus:2,StatusIndex:1"`
	Name       string `orm:"length=100;unique=TenantName:2"`
	Score      int
	Balance    float64 `orm:"precision=2"`
	Enabled    bool
	AddedAt    time.Time `orm:"time"`
	Day        time.Time
	Meta       map[string]interface{}
	Ref        *extraComplexRef `orm:"index=RefIndex"`
	Children   []*extraComplexChild
	FakeDelete bool `orm:"index=TenantStatus:3,StatusIndex:2,RefIndex:2;unique=TenantName:3"`

	ByTenantStatus *CachedQuery `query:":TenantID = ? AND :Status = ? ORDER BY :Status"`
	ByStatus       *CachedQuery `query:":Status = ? ORDER BY :Status"`
	OneByName      *CachedQuery `queryOne:":TenantID = ? AND :Name = ?"`
	AllRows        *CachedQuery `query:""`
}

type extraRequestCacheEntity struct {
	ORM
	ID   uint
	Name string
}

func TestExtraSearchLoadAndCacheIntegrationScenarios(t *testing.T) {
	engine, def, seeded := prepareExtraComplexEngine(t)
	defer def()

	t.Run("load by id warms one and many references", func(t *testing.T) {
		loaded := &extraComplexEntity{}
		found := engine.LoadByID(uint64(seeded.entities[0].ID), loaded, "Ref", "Children")

		require.True(t, found)
		assert.Equal(t, "Alpha", loaded.Name)
		require.NotNil(t, loaded.Ref)
		assert.Equal(t, "north", loaded.Ref.Key)
		require.Len(t, loaded.Children, 2)
		assert.True(t, loaded.Children[0].IsLoaded())
		assert.Equal(t, "child-a", loaded.Children[0].Label)
		assert.True(t, loaded.Children[1].IsLoaded())
		assert.Equal(t, "child-b", loaded.Children[1].Label)
	})

	t.Run("load by ids preserves duplicates and nil missing rows", func(t *testing.T) {
		var rows []*extraComplexEntity
		foundAll := engine.LoadByIDs([]uint64{uint64(seeded.entities[1].ID), uint64(seeded.entities[1].ID), 999999}, &rows)

		assert.False(t, foundAll)
		require.Len(t, rows, 3)
		require.NotNil(t, rows[0])
		require.NotNil(t, rows[1])
		assert.Equal(t, seeded.entities[1].ID, rows[0].ID)
		assert.Equal(t, seeded.entities[1].ID, rows[1].ID)
		assert.Nil(t, rows[2])
	})

	t.Run("search one with nested reference path warms child references", func(t *testing.T) {
		row := &extraComplexEntity{}
		found := engine.SearchOne(NewWhere("`Name` = ?", "Beta"), row, "Ref")

		require.True(t, found)
		assert.Equal(t, seeded.entities[1].ID, row.ID)
		require.NotNil(t, row.Ref)
		assert.True(t, row.Ref.IsLoaded())
		assert.Equal(t, "north", row.Ref.Key)
	})

	t.Run("search one returns false without mutating id on miss", func(t *testing.T) {
		row := &extraComplexEntity{ID: 123}
		found := engine.SearchOne(NewWhere("`Name` = ?", "does-not-exist"), row)

		assert.False(t, found)
		assert.Equal(t, uint(123), row.ID)
	})

	t.Run("search with count uses pager offset and total rows", func(t *testing.T) {
		var rows []*extraComplexEntity
		total := engine.SearchWithCount(NewWhere("`TenantID` = ? ORDER BY `ID`", 1), NewPager(2, 2), &rows)

		assert.Equal(t, 4, total)
		require.Len(t, rows, 2)
		assert.Equal(t, []uint{seeded.entities[2].ID, seeded.entities[3].ID}, []uint{rows[0].ID, rows[1].ID})
	})

	t.Run("search ids with count mirrors row pagination", func(t *testing.T) {
		ids, total := engine.SearchIDsWithCount(NewWhere("`TenantID` = ? ORDER BY `ID`", 1), NewPager(1, 3), &extraComplexEntity{})

		assert.Equal(t, 4, total)
		assert.Equal(t, []uint64{uint64(seeded.entities[0].ID), uint64(seeded.entities[1].ID), uint64(seeded.entities[2].ID)}, ids)
	})

	t.Run("search handles IN slices and bool filters together", func(t *testing.T) {
		var rows []*extraComplexEntity
		engine.Search(NewWhere("`Status` IN ? AND `Enabled` = ? ORDER BY `ID`", []string{"active", "queued"}, true), nil, &rows)

		assert.Equal(t, []string{"Alpha", "Epsilon", "Gamma"}, extraComplexNames(rows))
	})

	t.Run("search invalid reference panics with the requested path", func(t *testing.T) {
		var rows []*extraComplexEntity

		assert.PanicsWithError(t, "reference MissingReference in extraComplexEntity is not valid", func() {
			engine.Search(NewWhere("`ID` > 0"), nil, &rows, "MissingReference")
		})
	})

	t.Run("load invalid reference panics with the requested path", func(t *testing.T) {
		row := &extraComplexEntity{ID: seeded.entities[0].ID}

		assert.PanicsWithError(t, "reference MissingReference in extraComplexEntity is not valid", func() {
			engine.Load(row, "MissingReference")
		})
	})

	t.Run("delete hides row from normal search", func(t *testing.T) {
		engine.Delete(seeded.entities[2])

		var rows []*extraComplexEntity
		engine.Search(NewWhere("`TenantID` = ? ORDER BY `ID`", 1), nil, &rows)
		assert.Equal(t, []string{"Alpha", "Beta", "Delta"}, extraComplexNames(rows))

		engine.SearchWithFakeDeleted(NewWhere("`TenantID` = ? ORDER BY `ID`", 1), nil, &rows)
		assert.Equal(t, []string{"Alpha", "Beta", "Delta", "Gamma"}, extraComplexNames(rows))
	})

	t.Run("force delete removes row from fake deleted search too", func(t *testing.T) {
		engine.ForceDelete(seeded.entities[3])

		var rows []*extraComplexEntity
		engine.SearchWithFakeDeleted(NewWhere("`TenantID` = ? ORDER BY `ID`", 1), nil, &rows)
		assert.Equal(t, []string{"Alpha", "Beta", "Gamma"}, extraComplexNames(rows))
	})

	t.Run("clear cache by ids allows direct db update to be observed", func(t *testing.T) {
		rowID := seeded.entities[0].ID
		first := &extraComplexEntity{}
		require.True(t, engine.LoadByID(uint64(rowID), first))
		require.Equal(t, "Alpha", first.Name)

		engine.GetMysql().Exec("UPDATE `extraComplexEntity` SET `Name` = ? WHERE `ID` = ?", "Alpha DB", rowID)
		fromCache := &extraComplexEntity{}
		require.True(t, engine.LoadByID(uint64(rowID), fromCache))
		assert.Equal(t, "Alpha", fromCache.Name)

		engine.ClearCacheByIDs(&extraComplexEntity{}, uint64(rowID))
		fromDB := &extraComplexEntity{}
		require.True(t, engine.LoadByID(uint64(rowID), fromDB))
		assert.Equal(t, "Alpha DB", fromDB.Name)
	})

	t.Run("cache hit avoids mysql after warm load", func(t *testing.T) {
		logger := &testLogHandler{}
		engine.RegisterQueryLogger(logger, true, false, false)
		warmed := &extraComplexEntity{}
		require.True(t, engine.LoadByID(uint64(seeded.entities[1].ID), warmed))
		logger.clear()

		again := &extraComplexEntity{}
		require.True(t, engine.LoadByID(uint64(seeded.entities[1].ID), again))

		assert.Equal(t, "Beta", again.Name)
		assert.Empty(t, logger.Logs)
	})
}

func TestExtraCachedSearchIntegrationScenarios(t *testing.T) {
	engine, def, seeded := prepareExtraComplexEngine(t)
	defer def()

	t.Run("cached search by tenant and status returns matching rows", func(t *testing.T) {
		var rows []*extraComplexEntity
		total := engine.CachedSearch(&rows, "ByTenantStatus", NewPager(1, 20), 1, "active")

		assert.Equal(t, 3, total)
		assert.ElementsMatch(t, []string{"Alpha", "Beta", "Delta"}, extraComplexNames(rows))
	})

	t.Run("cached search ids and count agree", func(t *testing.T) {
		total, ids := engine.CachedSearchIDs(&extraComplexEntity{}, "ByStatus", NewPager(1, 20), "active")

		assert.Equal(t, 4, total)
		assert.ElementsMatch(t, []uint64{
			uint64(seeded.entities[0].ID),
			uint64(seeded.entities[1].ID),
			uint64(seeded.entities[3].ID),
			uint64(seeded.entities[4].ID),
		}, ids)
		assert.Equal(t, total, engine.CachedSearchCount(&extraComplexEntity{}, "ByStatus", "active"))
	})

	t.Run("cached search one uses composite unique query", func(t *testing.T) {
		row := &extraComplexEntity{}
		found := engine.CachedSearchOne(row, "OneByName", 1, "Beta")

		require.True(t, found)
		assert.Equal(t, seeded.entities[1].ID, row.ID)
		assert.Equal(t, "Beta", row.Name)
	})

	t.Run("cached search one warms requested references", func(t *testing.T) {
		row := &extraComplexEntity{}
		found := engine.CachedSearchOneWithReferences(row, "OneByName", []interface{}{1, "Alpha"}, []string{"Ref", "Children"})

		require.True(t, found)
		require.NotNil(t, row.Ref)
		assert.Equal(t, "north", row.Ref.Key)
		require.Len(t, row.Children, 2)
		assert.Equal(t, "child-a", row.Children[0].Label)
	})

	t.Run("cached search miss is cached without finding a row", func(t *testing.T) {
		row := &extraComplexEntity{}
		assert.False(t, engine.CachedSearchOne(row, "OneByName", 9, "Nobody"))
		assert.Equal(t, uint(0), row.ID)

		logger := &testLogHandler{}
		engine.RegisterQueryLogger(logger, true, false, false)
		assert.False(t, engine.CachedSearchOne(row, "OneByName", 9, "Nobody"))
		assert.Empty(t, logger.Logs)
	})

	t.Run("cached search all excludes fake deleted rows", func(t *testing.T) {
		engine.Delete(seeded.entities[2])

		var rows []*extraComplexEntity
		total := engine.CachedSearch(&rows, "AllRows", NewPager(1, 20))

		assert.Equal(t, 4, total)
		assert.NotContains(t, extraComplexNames(rows), "Gamma")
	})

	t.Run("cached search invalidates after status update", func(t *testing.T) {
		row := &extraComplexEntity{ID: seeded.entities[0].ID}
		require.True(t, engine.Load(row))
		row.Status = "inactive"
		engine.Flush(row)

		var activeRows []*extraComplexEntity
		activeTotal := engine.CachedSearch(&activeRows, "ByTenantStatus", NewPager(1, 20), 1, "active")
		assert.Equal(t, 2, activeTotal)
		assert.NotContains(t, extraComplexNames(activeRows), "Alpha")

		var inactiveRows []*extraComplexEntity
		inactiveTotal := engine.CachedSearch(&inactiveRows, "ByTenantStatus", NewPager(1, 20), 1, "inactive")
		assert.Equal(t, 1, inactiveTotal)
		assert.Equal(t, []string{"Alpha"}, extraComplexNames(inactiveRows))
	})

	t.Run("cached search page beyond total returns empty rows with total", func(t *testing.T) {
		var rows []*extraComplexEntity
		total := engine.CachedSearch(&rows, "ByTenantStatus", NewPager(5, 2), 1, "active")

		assert.Equal(t, 2, total)
		assert.Empty(t, rows)
	})

	t.Run("cached search rejects page beyond max cached window", func(t *testing.T) {
		var rows []*extraComplexEntity

		assert.PanicsWithError(t, "max cache index page size (50000) exceeded ByStatus", func() {
			engine.CachedSearch(&rows, "ByStatus", NewPager(51, 1000), "active")
		})
	})

	t.Run("cached search rejects missing index", func(t *testing.T) {
		var rows []*extraComplexEntity

		assert.PanicsWithError(t, "index MissingIndex not found", func() {
			engine.CachedSearch(&rows, "MissingIndex", nil, "active")
		})
	})
}

func TestExtraRequestCacheIntegrationScenarios(t *testing.T) {
	var entity *extraRequestCacheEntity
	engine, def := prepareTables(t, &Registry{}, 5, "", "2.0", entity)
	defer def()
	engine.EnableRequestCache()

	row := &extraRequestCacheEntity{Name: "request-cache-original"}
	engine.Flush(row)

	t.Run("request cache serves warm load for entity without configured cache", func(t *testing.T) {
		first := &extraRequestCacheEntity{}
		require.True(t, engine.LoadByID(uint64(row.ID), first))
		assert.Equal(t, "request-cache-original", first.Name)

		engine.GetMysql().Exec("UPDATE `extraRequestCacheEntity` SET `Name` = ? WHERE `ID` = ?", "request-cache-db", row.ID)
		fromRequestCache := &extraRequestCacheEntity{}
		require.True(t, engine.LoadByID(uint64(row.ID), fromRequestCache))
		assert.Equal(t, "request-cache-original", fromRequestCache.Name)
	})

	t.Run("clear cache by ids clears request cache fallback", func(t *testing.T) {
		engine.ClearCacheByIDs(&extraRequestCacheEntity{}, uint64(row.ID))

		fromDB := &extraRequestCacheEntity{}
		require.True(t, engine.LoadByID(uint64(row.ID), fromDB))
		assert.Equal(t, "request-cache-db", fromDB.Name)
	})

	t.Run("missing row is cached as miss inside request cache", func(t *testing.T) {
		missing := &extraRequestCacheEntity{}
		assert.False(t, engine.LoadByID(987654, missing))

		logger := &testLogHandler{}
		engine.RegisterQueryLogger(logger, true, false, false)
		assert.False(t, engine.LoadByID(987654, &extraRequestCacheEntity{}))
		assert.Empty(t, logger.Logs)
	})
}

type extraComplexSeed struct {
	refs     []*extraComplexRef
	children []*extraComplexChild
	entities []*extraComplexEntity
}

func prepareExtraComplexEngine(t *testing.T) (*Engine, func(), *extraComplexSeed) {
	t.Helper()

	var entity *extraComplexEntity
	var ref *extraComplexRef
	var child *extraComplexChild
	engine, def := prepareTables(t, &Registry{}, 5, "", "2.0", ref, child, entity)

	refs := []*extraComplexRef{{Key: "north"}, {Key: "south"}}
	children := []*extraComplexChild{{Label: "child-a"}, {Label: "child-b"}, {Label: "child-c"}}
	engine.FlushMany(refs[0], refs[1], children[0], children[1], children[2])

	addedAt := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	entities := []*extraComplexEntity{
		{
			TenantID: 1, Status: "active", Name: "Alpha", Score: 10, Balance: 12.345, Enabled: true,
			AddedAt: addedAt, Day: addedAt, Ref: refs[0], Children: []*extraComplexChild{children[0], children[1]},
			Meta: map[string]interface{}{"city": "Skopje", "rank": float64(1)},
		},
		{
			TenantID: 1, Status: "active", Name: "Beta", Score: 20, Balance: 22.229, Enabled: false,
			AddedAt: addedAt.Add(time.Hour), Day: addedAt, Ref: refs[0], Children: []*extraComplexChild{children[1]},
			Meta: map[string]interface{}{"city": "Ohrid", "rank": float64(2)},
		},
		{
			TenantID: 1, Status: "queued", Name: "Gamma", Score: 30, Balance: 32.1, Enabled: true,
			AddedAt: addedAt.Add(2 * time.Hour), Day: addedAt, Ref: refs[1], Children: []*extraComplexChild{children[2]},
			Meta: map[string]interface{}{"city": "Bitola", "rank": float64(3)},
		},
		{
			TenantID: 1, Status: "active", Name: "Delta", Score: 40, Balance: 42.9, Enabled: false,
			AddedAt: addedAt.Add(3 * time.Hour), Day: addedAt, Ref: refs[1],
			Meta: map[string]interface{}{"city": "Tetovo", "rank": float64(4)},
		},
		{
			TenantID: 2, Status: "active", Name: "Epsilon", Score: 50, Balance: 52.2, Enabled: true,
			AddedAt: addedAt.Add(4 * time.Hour), Day: addedAt, Ref: refs[1],
			Meta: map[string]interface{}{"city": "Prilep", "rank": float64(5)},
		},
	}
	engine.FlushMany(entities[0], entities[1], entities[2], entities[3], entities[4])

	return engine, def, &extraComplexSeed{refs: refs, children: children, entities: entities}
}

func extraComplexNames(rows []*extraComplexEntity) []string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			names = append(names, row.Name)
		}
	}
	sort.Strings(names)
	return names
}
