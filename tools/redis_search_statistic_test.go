package tools

import (
	"testing"

	"github.com/coretrix/trixorm"
	jsoniter "github.com/json-iterator/go"

	"github.com/stretchr/testify/assert"
)

func TestRedisSearchStatistics(t *testing.T) {
	registry := &trixorm.Registry{}
	registry.RegisterRedis("localhost:6382", "", 0)
	registry.RegisterRedisSearchIndex(&trixorm.RedisSearchIndex{Name: "test", RedisPool: "default", Prefixes: []string{"test:"}})
	validatedRegistry, def, err := registry.Validate()
	assert.NoError(t, err)
	defer def()
	engine := validatedRegistry.CreateEngine()
	engine.GetRedis().FlushDB()
	for _, alter := range engine.GetRedisSearchIndexAlters() {
		alter.Execute()
	}
	stats := GetRedisSearchStatistics(engine)
	assert.Len(t, stats, 1)
	assert.Equal(t, "test", stats[0].Index)
	assert.Equal(t, "test", stats[0].Info)
	asJSON, err := jsoniter.ConfigFastest.Marshal(stats)
	assert.NoError(t, err)
	assert.NotEmpty(t, asJSON)
}
