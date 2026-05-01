package trixorm

import (
	"context"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtraRedisCacheScenarios(t *testing.T) {
	registry := &Registry{}
	registry.RegisterRedis("localhost:6382", "extra_redis", 15)
	registry.RegisterRedisStream("extra-stream", "default", []string{"extra-group"})
	validatedRegistry, def, err := registry.Validate()
	require.NoError(t, err)
	defer def()

	engine := validatedRegistry.CreateEngine()
	redisCache := engine.GetRedis()
	redisCache.FlushDB()

	t.Run("namespace prefixes keys and removes only namespaced keys", func(t *testing.T) {
		redisCache.Set("plain", "value", 0)
		value, has := redisCache.Get("plain")
		require.True(t, has)
		assert.Equal(t, "value", value)
		assert.Equal(t, int64(1), redisCache.Exists("plain"))
		assert.Equal(t, "string", redisCache.Type("plain"))

		redisCache.FlushDB()
		_, has = redisCache.Get("plain")
		assert.False(t, has)
	})

	t.Run("get set stores msgpack encoded provider result", func(t *testing.T) {
		calls := 0
		first := redisCache.GetSet("get-set", 60, func() interface{} {
			calls++
			return map[string]interface{}{"name": "Ada", "age": int8(36)}
		})
		second := redisCache.GetSet("get-set", 60, func() interface{} {
			calls++
			return map[string]interface{}{"name": "Grace"}
		})

		assert.Equal(t, map[string]interface{}{"name": "Ada", "age": int8(36)}, first)
		secondMap, ok := second.(map[interface{}]interface{})
		require.True(t, ok)
		assert.Equal(t, "Ada", secondMap["name"])
		assert.Equal(t, uint8(36), secondMap["age"])
		assert.Equal(t, 1, calls)
	})

	t.Run("set nx only writes once", func(t *testing.T) {
		assert.True(t, redisCache.SetNX("set-nx", "first", 60))
		assert.False(t, redisCache.SetNX("set-nx", "second", 60))
		value, has := redisCache.Get("set-nx")
		require.True(t, has)
		assert.Equal(t, "first", value)
	})

	t.Run("hash operations preserve missing fields", func(t *testing.T) {
		redisCache.HSet("hash", "name", "Ada", "age", "36")
		assert.Equal(t, map[string]interface{}{"name": "Ada", "age": "36", "missing": nil}, redisCache.HMGet("hash", "name", "age", "missing"))
		value, has := redisCache.HGet("hash", "missing")
		assert.False(t, has)
		assert.Equal(t, "", value)
		assert.True(t, redisCache.HSetNx("hash", "once", "1"))
		assert.False(t, redisCache.HSetNx("hash", "once", "2"))
		assert.Equal(t, int64(3), redisCache.HLen("hash"))
		redisCache.HDel("hash", "age", "once")
		assert.Equal(t, map[string]string{"name": "Ada"}, redisCache.HGetAll("hash"))
	})

	t.Run("list operations cover push trim set remove and pop misses", func(t *testing.T) {
		redisCache.Del("list")
		assert.Equal(t, int64(2), redisCache.LPush("list", "b", "a"))
		assert.Equal(t, int64(4), redisCache.RPush("list", "c", "d"))
		assert.Equal(t, []string{"a", "b", "c", "d"}, redisCache.LRange("list", 0, -1))
		redisCache.LSet("list", 1, "B")
		redisCache.LRem("list", 1, "c")
		assert.Equal(t, []string{"a", "B", "d"}, redisCache.LRange("list", 0, -1))
		redisCache.Ltrim("list", 0, 1)
		value, has := redisCache.RPop("list")
		assert.True(t, has)
		assert.Equal(t, "B", value)
		value, has = redisCache.RPop("list")
		assert.True(t, has)
		assert.Equal(t, "a", value)
		value, has = redisCache.RPop("list")
		assert.False(t, has)
		assert.Equal(t, "", value)
	})

	t.Run("sorted set range variants and removal by rank", func(t *testing.T) {
		redisCache.Del("z")
		added := redisCache.ZAdd("z", &redis.Z{Member: "low", Score: 1}, &redis.Z{Member: "mid", Score: 2}, &redis.Z{Member: "high", Score: 3})
		assert.Equal(t, int64(3), added)
		assert.Equal(t, []string{"high", "mid"}, redisCache.ZRevRange("z", 0, 1))
		assert.Equal(t, int64(3), redisCache.ZCard("z"))
		assert.Equal(t, int64(2), redisCache.ZCount("z", "2", "3"))
		assert.Equal(t, float64(2), redisCache.ZScore("z", "mid"))

		forward := redisCache.ZRangeArgs(redis.ZRangeArgs{Key: "z", Start: 1, Stop: 3})
		assert.Equal(t, []string{"mid", "high"}, forward)
		withScores := redisCache.ZRangeArgsWithScores(redis.ZRangeArgs{Key: "z", Start: 0, Stop: 0})
		require.Len(t, withScores, 1)
		assert.Equal(t, "low", withScores[0].Member)
		assert.Equal(t, int64(1), redisCache.ZRemRangeByRank("z", 0, 0))
		assert.Equal(t, []string{"mid", "high"}, redisCache.ZRangeArgs(redis.ZRangeArgs{Key: "z", Start: 0, Stop: 2}))
	})

	t.Run("set operations pop members and then miss", func(t *testing.T) {
		redisCache.Del("set")
		assert.Equal(t, int64(3), redisCache.SAdd("set", "a", "b", "c", "a"))
		assert.Equal(t, int64(3), redisCache.SCard("set"))
		popped := redisCache.SPopN("set", 2)
		assert.Len(t, popped, 2)
		one, has := redisCache.SPop("set")
		assert.True(t, has)
		assert.NotEmpty(t, one)
		one, has = redisCache.SPop("set")
		assert.False(t, has)
		assert.Equal(t, "", one)
	})

	t.Run("mset and mget preserve requested order", func(t *testing.T) {
		redisCache.MSet("m1", "one", "m2", "two")

		assert.Equal(t, []interface{}{"two", nil, "one"}, redisCache.MGet("m2", "missing", "m1"))
	})

	t.Run("lua script load exists evalsha and missing sha", func(t *testing.T) {
		script := "return ARGV[1]"
		sha := redisCache.ScriptLoad(script)

		assert.True(t, redisCache.ScriptExists(sha))
		value, exists := redisCache.EvalSha(sha, nil, "ok")
		assert.True(t, exists)
		assert.Equal(t, "ok", value)
		value, exists = redisCache.EvalSha("ffffffffffffffffffffffffffffffffffffffff", nil)
		assert.False(t, exists)
		assert.Nil(t, value)
		assert.Equal(t, "direct", redisCache.Eval(script, nil, "direct"))
	})

	t.Run("pipeline can mix strings hashes counters expiry and delete", func(t *testing.T) {
		redisCache.Del("pipe", "pipe-hash")
		pipeline := redisCache.PipeLine()
		pipeline.Set("pipe", "value", time.Minute)
		getBeforeExec := pipeline.Get("pipe")
		counter := pipeline.HIncrBy("pipe-hash", "count", 3)
		pipeline.HSet("pipe-hash", "name", "Ada")
		expired := pipeline.Expire("pipe", time.Minute)
		pipeline.Exec()

		value, has := getBeforeExec.Result()
		assert.True(t, has)
		assert.Equal(t, "value", value)
		assert.Equal(t, int64(3), counter.Result())
		assert.True(t, expired.Result())
		assert.Equal(t, map[string]string{"count": "3", "name": "Ada"}, redisCache.HGetAll("pipe-hash"))

		pipeline.Del("pipe", "pipe-hash")
		pipeline.Exec()
		assert.Equal(t, int64(0), redisCache.Exists("pipe", "pipe-hash"))
	})

	t.Run("stream operations create group read acknowledge and delete", func(t *testing.T) {
		redisCache.Del("extra-stream-case")
		result, existed := redisCache.XGroupCreateMkStream("extra-stream-case", "extra-group-case", "0")
		assert.False(t, existed)
		assert.Equal(t, "OK", result)

		id := redisCache.xAdd("extra-stream-case", []string{"payload", "one"})
		require.NotEmpty(t, id)
		assert.Equal(t, int64(1), redisCache.XLen("extra-stream-case"))

		messages := redisCache.XReadGroup(context.Background(), &redis.XReadGroupArgs{
			Group:    "extra-group-case",
			Consumer: "extra-consumer",
			Streams:  []string{"extra-stream-case", ">"},
			Count:    1,
		})
		require.Len(t, messages, 1)
		require.Len(t, messages[0].Messages, 1)
		assert.Equal(t, id, messages[0].Messages[0].ID)

		pending := redisCache.XPending("extra-stream-case", "extra-group-case")
		assert.Equal(t, int64(1), pending.Count)
		assert.Equal(t, int64(1), redisCache.XAck("extra-stream-case", "extra-group-case", id))
		assert.Equal(t, int64(1), redisCache.XDel("extra-stream-case", id))
		assert.Equal(t, int64(0), redisCache.XLen("extra-stream-case"))
	})
}

func TestRedisRateLimitRealLifeLoginAttempts(t *testing.T) {
	registry := &Registry{}
	registry.RegisterRedis("localhost:6382", "rate_limit_life", 15)
	registry.RegisterRedis("localhost:6382", "rate_limit_life_other", 15, "other")
	validatedRegistry, def, err := registry.Validate()
	require.NoError(t, err)
	defer def()

	engine := validatedRegistry.CreateEngine()
	redisCache := engine.GetRedis()
	otherRedisCache := engine.GetRedis("other")
	redisCache.FlushDB()
	otherRedisCache.FlushDB()
	logger := &testLogHandler{}
	engine.RegisterQueryLogger(logger, false, true, false)

	key := "login:customer:42"
	period := 300 * time.Millisecond
	limit := 3

	assert.True(t, redisCache.RateLimit(key, period, limit))
	assert.True(t, redisCache.RateLimit(key, period, limit))
	assert.True(t, redisCache.RateLimit(key, period, limit))
	assert.False(t, redisCache.RateLimit(key, period, limit))

	require.Len(t, logger.Logs, 4)
	assert.Equal(t, "RATE", logger.Logs[0]["operation"])
	assert.Equal(t, "RATE rate_limit_life:"+key+" "+period.String(), logger.Logs[0]["query"])

	namespaceKey := "login:customer:namespace"
	assert.True(t, redisCache.RateLimit(namespaceKey, time.Minute, 1))
	assert.False(t, redisCache.RateLimit(namespaceKey, time.Minute, 1))
	assert.True(t, otherRedisCache.RateLimit(namespaceKey, time.Minute, 1))

	time.Sleep(period + 100*time.Millisecond)
	assert.True(t, redisCache.RateLimit(key, period, limit))
}
