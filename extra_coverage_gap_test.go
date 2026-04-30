package trixorm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type extraCoverageGapEntity struct {
	ORM
	ID   uint
	Code string `orm:"unique=Code"`
	Name string `orm:"unique=Name"`
}

func TestExtraCoverageGapWrapperScenarios(t *testing.T) {
	var entity *extraCoverageGapEntity
	engine, def := prepareTables(t, &Registry{}, 8, "", "2.0", entity)
	defer def()

	t.Run("engine flush with full check returns duplicate key errors", func(t *testing.T) {
		engine.Flush(&extraCoverageGapEntity{Code: "dup", Name: "first"})

		err := engine.FlushWithFullCheck(&extraCoverageGapEntity{Code: "dup", Name: "second"})
		require.Error(t, err)
		assert.IsType(t, &DuplicatedKeyError{}, err)
	})

	t.Run("table schema unique indexes are exposed as a copy", func(t *testing.T) {
		schema := engine.GetRegistry().GetTableSchemaForEntity(entity)
		uniqueIndexes := schema.GetUniqueIndexes()

		assert.Equal(t, []string{"Code"}, uniqueIndexes["Code"])
		assert.Equal(t, []string{"Name"}, uniqueIndexes["Name"])

		uniqueIndexes["Code"] = []string{"Changed"}
		assert.Equal(t, []string{"Code"}, schema.GetUniqueIndexes()["Code"])
	})
}

func TestExtraCoverageGapRedisPubSubScenarios(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterRedis("localhost:6382", "extra_pubsub", 15)
	registry.RegisterRedisStream("extra-pubsub-stream", "default", []string{"extra-pubsub-group"})
	validatedRegistry, def, err := registry.Validate()
	require.NoError(t, err)
	defer def()

	engine := validatedRegistry.CreateEngine()
	redisCache := engine.GetRedis()
	redisCache.FlushDB()

	pubSub := redisCache.Subscribe("updates")
	defer pubSub.Close()
	_, err = pubSub.Receive(context.Background())
	require.NoError(t, err)

	published := redisCache.Publish("updates", "payload")
	assert.Equal(t, int64(1), published)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	message, err := pubSub.ReceiveMessage(ctx)
	require.NoError(t, err)
	assert.Equal(t, "extra_pubsub:updates", message.Channel)
	assert.Equal(t, "payload", message.Payload)
}

func TestExtraCoverageGapRedisSearchScenarios(t *testing.T) {
	t.Run("aggregate load all appends wildcard load arguments", func(t *testing.T) {
		aggregate := NewRedisSearchQuery().Aggregate().LoadAll()

		assert.Equal(t, []interface{}{"LOAD", "*"}, aggregate.args)
	})

	t.Run("force reindex without drop publishes indexer event", func(t *testing.T) {
		registry := NewRegistry()
		index := NewRedisSearchIndex("extra-gap-index", "search", []string{"extra-gap:"})
		index.AddTextField("Name", 1, false, false, false)
		registry.RegisterRedisSearchIndex(index)

		engine, def := prepareTables(t, registry, 8, "extra_gap_reindex", "2.0")
		defer def()
		engine.GetRedis().FlushDB()

		engine.GetRedisSearch("search").ForceReindexWithoutDrop("extra-gap-index")

		assert.Equal(t, int64(1), engine.GetRedis().XLen(RedisSearchIndexerChannelName))
	})
}
