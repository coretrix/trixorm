package trixorm

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type extraRegistryEntity struct {
	ORM
	ID   uint
	Name string
}

func TestExtraRegistryYamlScenarios(t *testing.T) {
	t.Run("mysql uri appends multi statements", func(t *testing.T) {
		registry := NewRegistry()
		registry.InitByYaml(map[string]interface{}{
			"default": map[string]interface{}{
				"mysql": "user:pass@tcp(localhost:3306)/app",
			},
		})

		assert.Equal(t, "user:pass@tcp(localhost:3306)/app?multiStatements=true", registry.mysqlPools["default"].GetDataSourceURI())
		assert.Equal(t, "app", registry.mysqlPools["default"].GetDatabase())
	})

	t.Run("mysql uri preserves existing query string", func(t *testing.T) {
		registry := NewRegistry()
		registry.InitByYaml(map[string]interface{}{
			"default": map[string]interface{}{
				"mysql": "user:pass@tcp(localhost:3306)/app?parseTime=true",
			},
		})

		assert.Equal(t, "user:pass@tcp(localhost:3306)/app?parseTime=true&multiStatements=true", registry.mysqlPools["default"].GetDataSourceURI())
	})

	t.Run("mysql limit connections is parsed and removed from dsn", func(t *testing.T) {
		registry := NewRegistry()
		registry.InitByYaml(map[string]interface{}{
			"default": map[string]interface{}{
				"mysql": "user:pass@tcp(localhost:3306)/app?limit_connections=7&parseTime=true",
			},
		})

		pool := registry.mysqlPools["default"].(*mySQLPoolConfig)
		assert.Equal(t, 7, pool.maxConnections)
		assert.Equal(t, "user:pass@tcp(localhost:3306)/app?parseTime=true&multiStatements=true", pool.GetDataSourceURI())
	})

	t.Run("redis simple uri registers host and db", func(t *testing.T) {
		registry := NewRegistry()
		registry.InitByYaml(map[string]interface{}{
			"default": map[string]interface{}{
				"redis": "localhost:15",
			},
		})

		pool := registry.redisPools["default"]
		assert.Equal(t, "localhost", pool.GetAddress())
		assert.Equal(t, 15, pool.GetDatabase())
		assert.Equal(t, "", pool.GetNamespace())
	})

	t.Run("redis host port uri registers address", func(t *testing.T) {
		registry := NewRegistry()
		registry.InitByYaml(map[string]interface{}{
			"cache": map[string]interface{}{
				"redis": "localhost:6382:14",
			},
		})

		pool := registry.redisPools["cache"]
		assert.Equal(t, "localhost:6382", pool.GetAddress())
		assert.Equal(t, 14, pool.GetDatabase())
		assert.False(t, pool.HasNamespace())
	})

	t.Run("redis namespace and credentials are parsed", func(t *testing.T) {
		registry := NewRegistry()
		registry.InitByYaml(map[string]interface{}{
			"cache": map[string]interface{}{
				"redis": "localhost:6382:14:tenant?user=u&password=p",
			},
		})

		pool := registry.redisPools["cache"]
		assert.Equal(t, "localhost:6382", pool.GetAddress())
		assert.Equal(t, 14, pool.GetDatabase())
		assert.Equal(t, "tenant", pool.GetNamespace())
		assert.True(t, pool.HasNamespace())
	})

	t.Run("redis unix socket uri keeps socket address", func(t *testing.T) {
		registry := NewRegistry()
		registry.InitByYaml(map[string]interface{}{
			"cache": map[string]interface{}{
				"redis": "/tmp/redis.sock:3:tenant",
			},
		})

		pool := registry.redisPools["cache"]
		assert.Equal(t, "/tmp/redis.sock", pool.GetAddress())
		assert.Equal(t, 3, pool.GetDatabase())
		assert.Equal(t, "tenant", pool.GetNamespace())
	})

	t.Run("sentinel uri registers failover pool metadata", func(t *testing.T) {
		registry := NewRegistry()
		registry.InitByYaml(map[string]interface{}{
			"cache": map[string]interface{}{
				"sentinel": map[string]interface{}{
					"master:2:tenant?user=u&password=p": []interface{}{"s1:26379", "s2:26379"},
				},
			},
		})

		pool := registry.redisPools["cache"]
		assert.Equal(t, "[s1:26379 s2:26379]", pool.GetAddress())
		assert.Equal(t, 2, pool.GetDatabase())
		assert.Equal(t, "tenant", pool.GetNamespace())
	})

	t.Run("streams accept yaml interface maps", func(t *testing.T) {
		registry := NewRegistry()
		registry.InitByYaml(map[string]interface{}{
			"default": map[interface{}]interface{}{
				"streams": map[interface{}]interface{}{
					"events": []interface{}{"g1", "g2"},
				},
			},
		})

		assert.Equal(t, "default", registry.redisStreamPools["events"])
		assert.True(t, registry.redisStreamGroups["default"]["events"]["g1"])
		assert.True(t, registry.redisStreamGroups["default"]["events"]["g2"])
	})

	t.Run("local cache and mysql defaults are parsed", func(t *testing.T) {
		registry := NewRegistry()
		registry.InitByYaml(map[string]interface{}{
			"default": map[string]interface{}{
				"local_cache":   123,
				"mysqlEncoding": "utf8mb4",
				"mysqlCollate":  "unicode_ci",
			},
		})

		assert.Equal(t, 123, registry.localCachePools["default"].GetLimit())
		assert.Equal(t, "utf8mb4", registry.defaultEncoding)
		assert.Equal(t, "unicode_ci", registry.defaultCollate)
	})

	t.Run("invalid mysql value panics", func(t *testing.T) {
		assert.PanicsWithError(t, "mysql uri '42' is not valid", func() {
			NewRegistry().InitByYaml(map[string]interface{}{"default": map[string]interface{}{"mysql": 42}})
		})
	})

	t.Run("invalid redis value panics", func(t *testing.T) {
		assert.PanicsWithError(t, "redis uri '42' is not valid", func() {
			NewRegistry().InitByYaml(map[string]interface{}{"default": map[string]interface{}{"redis": 42}})
		})
	})

	t.Run("invalid redis shape panics", func(t *testing.T) {
		assert.PanicsWithError(t, "redis uri 'too:many:parts:for:this' is not valid", func() {
			NewRegistry().InitByYaml(map[string]interface{}{"default": map[string]interface{}{"redis": "too:many:parts:for:this"}})
		})
	})

	t.Run("invalid redis db panics", func(t *testing.T) {
		assert.PanicsWithError(t, "redis uri 'localhost:not-a-db' is not valid", func() {
			NewRegistry().InitByYaml(map[string]interface{}{"default": map[string]interface{}{"redis": "localhost:not-a-db"}})
		})
	})

	t.Run("invalid stream groups panic", func(t *testing.T) {
		assert.PanicsWithError(t, "streams 'not-a-list' is not valid", func() {
			NewRegistry().InitByYaml(map[string]interface{}{
				"default": map[string]interface{}{"streams": map[string]interface{}{"events": "not-a-list"}},
			})
		})
	})

	t.Run("invalid sentinel groups panic", func(t *testing.T) {
		assert.PanicsWithError(t, "sentinel 'map[master:not-a-list]' is not valid", func() {
			NewRegistry().InitByYaml(map[string]interface{}{
				"default": map[string]interface{}{"sentinel": map[string]interface{}{"master": "not-a-list"}},
			})
		})
	})

	t.Run("invalid local cache value panics", func(t *testing.T) {
		assert.PanicsWithError(t, "orm value for default: not-int is not valid", func() {
			NewRegistry().InitByYaml(map[string]interface{}{"default": map[string]interface{}{"local_cache": "not-int"}})
		})
	})

	t.Run("invalid string setting panics", func(t *testing.T) {
		assert.PanicsWithError(t, "orm value for default: 42 is not valid", func() {
			NewRegistry().InitByYaml(map[string]interface{}{"default": map[string]interface{}{"mysqlEncoding": 42}})
		})
	})

	t.Run("duplicate stream panics", func(t *testing.T) {
		registry := NewRegistry()
		registry.RegisterRedisStream("events", "default", []string{"g1"})

		assert.PanicsWithError(t, "stream with name events already exists", func() {
			registry.RegisterRedisStream("events", "default", []string{"g2"})
		})
	})
}

func TestExtraValidatedRegistryGetterScenarios(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterLocalCache(10)
	registry.RegisterEntity(&extraRegistryEntity{})
	registry.RegisterEnum("extra.enum", []string{"a", "b"}, "b")
	registry.RegisterRedisStream("extra-events", "default", []string{"g1", "g2"})
	index := NewRedisSearchIndex("extra-index", "default", []string{"extra:"})
	index.AddTextField("Name", 1, false, false, false)
	registry.RegisterRedisSearchIndex(index)

	engine, def := prepareTables(t, registry, 5, "extra_registry", "2.0", &extraRegistryEntity{})
	defer def()
	validatedRegistry := engine.GetRegistry()

	t.Run("registered entity is available by name and value", func(t *testing.T) {
		schemaByName := validatedRegistry.GetTableSchema("trixorm.extraRegistryEntity")
		schemaByEntity := validatedRegistry.GetTableSchemaForEntity(&extraRegistryEntity{})

		require.NotNil(t, schemaByName)
		assert.Equal(t, schemaByName, schemaByEntity)
		assert.Equal(t, reflect.TypeOf(extraRegistryEntity{}), schemaByName.GetType())
		assert.Nil(t, validatedRegistry.GetTableSchema("missing"))
	})

	t.Run("unregistered entity lookup panics for concrete entity", func(t *testing.T) {
		assert.PanicsWithError(t, "entity 'trixorm.extraSetFieldEntity' is not registered", func() {
			validatedRegistry.GetTableSchemaForEntity(&extraSetFieldEntity{})
		})
	})

	t.Run("cache prefix lookup finds the owning schema", func(t *testing.T) {
		schema := validatedRegistry.GetTableSchemaForEntity(&extraRegistryEntity{}).(*tableSchema)

		assert.Equal(t, schema, validatedRegistry.GetTableSchemaForCachePrefix(schema.cachePrefix))
		assert.Nil(t, validatedRegistry.GetTableSchemaForCachePrefix("not-a-prefix"))
	})

	t.Run("enum getter exposes default value", func(t *testing.T) {
		enum := validatedRegistry.GetEnum("extra.enum")

		assert.Equal(t, []string{"a", "b"}, enum.GetFields())
		assert.Equal(t, "b", enum.GetDefault())
	})

	t.Run("pool getters expose registered pools", func(t *testing.T) {
		assert.Contains(t, validatedRegistry.GetRedisPools(), "default")
		assert.Contains(t, validatedRegistry.GetLocalCachePools(), "default")
		assert.Contains(t, validatedRegistry.GetMySQLPools(), "default")
	})

	t.Run("redis stream getter returns grouped stream names", func(t *testing.T) {
		streams := validatedRegistry.GetRedisStreams()

		require.Contains(t, streams, "default")
		assert.ElementsMatch(t, []string{"g1", "g2"}, streams["default"]["extra-events"])
		assert.Contains(t, streams["default"], LazyChannelName)
		assert.Contains(t, streams["default"], RedisStreamGarbageCollectorChannelName)
		assert.Contains(t, streams["default"], RedisSearchIndexerChannelName)
	})

	t.Run("redis search getter returns registered index", func(t *testing.T) {
		indices := validatedRegistry.GetRedisSearchIndices()

		require.Contains(t, indices, "default")
		require.Len(t, indices["default"], 1)
		assert.Equal(t, "extra-index", indices["default"][0].Name)
	})

	t.Run("engine clone preserves registry metadata and lazy resources are independent", func(t *testing.T) {
		engineToClone := validatedRegistry.CreateEngine()
		engineToClone.EnableRequestCache()
		engineToClone.SetLogMetaData("request", "one")
		clone := engineToClone.Clone()

		assert.Equal(t, engineToClone.GetRegistry(), clone.GetRegistry())
		assert.True(t, clone.hasRequestCache)
		assert.Equal(t, Bind{"request": "one"}, clone.logMetaData)
		assert.Nil(t, clone.localCache)
		assert.NotSame(t, engineToClone, clone)
	})
}
