package trixorm

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

func (r *Registry) InitByYaml(yaml map[string]interface{}) {
	for key, data := range yaml {
		dataAsMap := fixYamlMap(data, "orm")
		redisValue, hasRedis := dataAsMap["redis"]
		redisOptionsValue, hasRedisOptions := dataAsMap["redis_options"]
		_, hasSentinel := dataAsMap["sentinel"]
		if hasRedisOptions && !hasRedis {
			if hasSentinel {
				panic(fmt.Errorf("redis_options for %s are only supported with redis", key))
			}
			panic(fmt.Errorf("redis_options for %s require redis", key))
		}
		if hasRedis {
			validateRedisURIWithOptions(r, redisValue, redisOptionsValue, hasRedisOptions, key)
		}
		for dataKey, value := range dataAsMap {
			switch dataKey {
			case "mysql":
				validateOrmMysqlURI(r, value, key)
			case "redis", "redis_options":
				// Redis is initialized above after the complete pool configuration is available.
			case "sentinel":
				validateSentinel(r, value, key)
			case "streams":
				validateStreams(r, value, key)
			case "mysqlEncoding":
				valAsString := validateOrmString(value, key)
				r.SetDefaultEncoding(valAsString)
			case "mysqlCollate":
				valAsString := validateOrmString(value, key)
				r.SetDefaultCollate(valAsString)
			case "disableCacheHashCheck":
				if value.(bool) {
					DisableCacheHashCheck()
				}
			case "local_cache":
				number := validateOrmInt(value, key)
				r.RegisterLocalCache(number, key)
			}
		}
	}
}

func validateOrmMysqlURI(registry *Registry, value interface{}, key string) {
	asString, ok := value.(string)
	if !ok {
		panic(fmt.Errorf("mysql uri '%v' is not valid", value))
	}
	registry.RegisterMySQLPool(asString, key)
}

func validateStreams(registry *Registry, value interface{}, key string) {
	def := fixYamlMap(value, key)
	for name, groups := range def {
		asSlice, ok := groups.([]interface{})
		if !ok {
			panic(fmt.Errorf("streams '%v' is not valid", groups))
		}
		asString := make([]string, len(asSlice))
		for i, val := range asSlice {
			asString[i] = fmt.Sprintf("%v", val)
		}
		registry.RegisterRedisStream(name, key, asString)
	}
}

func validateRedisURI(registry *Registry, value interface{}, key string) {
	validateRedisURIWithOptions(registry, value, nil, false, key)
}

func validateRedisURIWithOptions(
	registry *Registry,
	value interface{},
	optionsValue interface{},
	hasOptions bool,
	key string,
) {
	asString, ok := value.(string)
	if !ok {
		panic(fmt.Errorf("redis uri '%v' is not valid", value))
	}
	parts := strings.Split(asString, "?")
	elements := strings.Split(parts[0], ":")
	dbNumber := ""
	uri := ""
	namespace := ""
	isSocket := strings.Index(parts[0], ".sock") > 0
	l := len(elements)
	switch l {
	case 2:
		dbNumber = elements[1]
		uri = elements[0]
	case 3:
		if isSocket {
			dbNumber = elements[1]
			uri = elements[0]
			namespace = elements[2]
		} else {
			dbNumber = elements[2]
			uri = elements[0] + ":" + elements[1]
		}
	case 4:
		dbNumber = elements[2]
		namespace = elements[3]
		uri = elements[0] + ":" + elements[1]
	default:
		panic(fmt.Errorf("redis uri '%v' is not valid", value))
	}
	db, err := strconv.ParseUint(dbNumber, 10, 64)
	if err != nil {
		panic(fmt.Errorf("redis uri '%v' is not valid", value))
	}
	redisOptions := redis.Options{
		Addr: uri,
		DB:   int(db),
	}
	if len(parts) == 2 && parts[1] != "" {
		values, err := url.ParseQuery(parts[1])
		if err != nil {
			panic(fmt.Errorf("redis uri '%v' is not valid", value))
		}
		if values.Has("user") && values.Has("password") {
			redisOptions.Username = values.Get("user")
			redisOptions.Password = values.Get("password")
		}
	}
	if hasOptions {
		applyRedisOptions(&redisOptions, optionsValue, key)
	}
	registry.RegisterRedisWithOptions(namespace, redisOptions, key)
}

func applyRedisOptions(options *redis.Options, value interface{}, key string) {
	values := fixYamlMap(value, key+".redis_options")
	for option, rawValue := range values {
		switch option {
		case "pool_size":
			options.PoolSize = validateRedisPositiveInt(rawValue, key, option)
		case "min_idle_conns":
			options.MinIdleConns = validateRedisNonNegativeInt(rawValue, key, option)
		case "pool_timeout":
			options.PoolTimeout = validateRedisDuration(rawValue, key, option)
		case "dial_timeout":
			options.DialTimeout = validateRedisDuration(rawValue, key, option)
		case "read_timeout":
			options.ReadTimeout = validateRedisDuration(rawValue, key, option)
		case "write_timeout":
			options.WriteTimeout = validateRedisDuration(rawValue, key, option)
		case "max_retries":
			options.MaxRetries = validateRedisNonNegativeInt(rawValue, key, option)
		case "min_retry_backoff":
			options.MinRetryBackoff = validateRedisDuration(rawValue, key, option)
		case "max_retry_backoff":
			options.MaxRetryBackoff = validateRedisDuration(rawValue, key, option)
		case "pool_fifo":
			asBool, ok := rawValue.(bool)
			if !ok {
				panic(fmt.Errorf("redis option %s.%s '%v' is not valid", key, option, rawValue))
			}
			options.PoolFIFO = asBool
		default:
			panic(fmt.Errorf("redis option %s.%s is not supported", key, option))
		}
	}
}

func validateRedisPositiveInt(value interface{}, key, option string) int {
	asInt := validateRedisNonNegativeInt(value, key, option)
	if asInt == 0 {
		panic(fmt.Errorf("redis option %s.%s must be greater than zero", key, option))
	}
	return asInt
}

func validateRedisNonNegativeInt(value interface{}, key, option string) int {
	asInt, ok := value.(int)
	if !ok || asInt < 0 {
		panic(fmt.Errorf("redis option %s.%s '%v' is not valid", key, option, value))
	}
	return asInt
}

func validateRedisDuration(value interface{}, key, option string) time.Duration {
	asString, ok := value.(string)
	if !ok {
		panic(fmt.Errorf("redis option %s.%s '%v' is not valid", key, option, value))
	}
	duration, err := time.ParseDuration(asString)
	if err != nil || duration <= 0 {
		panic(fmt.Errorf("redis option %s.%s '%v' is not valid", key, option, value))
	}
	return duration
}

func validateSentinel(registry *Registry, value interface{}, key string) {
	def := fixYamlMap(value, key)
	for master, values := range def {
		asSlice, ok := values.([]interface{})
		if !ok {
			panic(fmt.Errorf("sentinel '%v' is not valid", value))
		}
		asStrings := make([]string, len(asSlice))
		for i, v := range asSlice {
			asStrings[i] = fmt.Sprintf("%v", v)
		}
		db := 0
		namespace := ""
		parts := strings.Split(master, "?")
		elements := strings.Split(parts[0], ":")
		l := len(elements)
		if l >= 2 {
			master = elements[0]
			nr, err := strconv.ParseUint(elements[1], 10, 64)
			if err != nil {
				panic(fmt.Errorf("sentinel db '%v' is not valid", value))
			}
			db = int(nr)
			if l == 3 {
				namespace = elements[2]
			}
		}
		if len(parts) == 2 && parts[1] != "" {
			values, err := url.ParseQuery(parts[1])
			if err != nil {
				panic(fmt.Errorf("sentinel uri '%v' is not valid", master))
			}
			if values.Has("user") && values.Has("password") {
				registry.RegisterRedisSentinelWithCredentials(master, namespace, values.Get("user"), values.Get("password"), db, asStrings, key)
				return
			}
		}
		registry.RegisterRedisSentinel(master, namespace, db, asStrings, key)
	}
}

func fixYamlMap(value interface{}, key string) map[string]interface{} {
	def, ok := value.(map[string]interface{})
	if !ok {
		def2, ok := value.(map[interface{}]interface{})
		if !ok {
			panic(fmt.Errorf("orm yaml key %s is not valid", key))
		}
		def = make(map[string]interface{})
		for k, v := range def2 {
			def[fmt.Sprintf("%v", k)] = v
		}
	}
	return def
}

func validateOrmInt(value interface{}, key string) int {
	asInt, ok := value.(int)
	if !ok {
		panic(fmt.Errorf("orm value for %s: %v is not valid", key, value))
	}
	return asInt
}

func validateOrmString(value interface{}, key string) string {
	asString, ok := value.(string)
	if !ok {
		panic(fmt.Errorf("orm value for %s: %v is not valid", key, value))
	}
	return asString
}
