package tools

import (
	"testing"

	jsoniter "github.com/json-iterator/go"

	"github.com/coretrix/trixorm"
	"github.com/stretchr/testify/assert"
)

func TestRedisSearchStatistics(t *testing.T) {
	registry := &trixorm.Registry{}
	registry.RegisterRedis("localhost:6382", "", 0)
	index := trixorm.NewRedisSearchIndex("test", "default", []string{"test:"})
	index.AddTextField("title", 1, false, false, false)
	registry.RegisterRedisSearchIndex(index)
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
	assert.Equal(t, "test", stats[0].Index.Name)
	assert.Equal(t, "test", stats[0].Info.Name)
	asJSON, err := jsoniter.ConfigFastest.Marshal(stats)
	assert.NoError(t, err)
	assert.NotEmpty(t, asJSON)
}
