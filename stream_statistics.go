package trixorm

import (
	"math"
	"strconv"
	"strings"
	"time"
)

type RedisStreamStatistics struct {
	Stream             string
	RedisPool          string
	Len                uint64
	OldestEventSeconds int
	Groups             []*RedisStreamGroupStatistics
}

type RedisStreamGroupStatistics struct {
	Group                 string
	Lag                   int64
	Pending               uint64
	LastDeliveredID       string
	LastDeliveredDuration time.Duration
	LowerID               string
	LowerDuration         time.Duration
	Consumers             []*RedisStreamConsumerStatistics
}

type RedisStreamConsumerStatistics struct {
	Name    string
	Pending uint64
}

func (eb *eventBroker) GetStreamStatistics(stream string) *RedisStreamStatistics {
	stats := eb.GetStreamsStatistics(stream)
	if len(stats) > 0 {
		return stats[0]
	}
	return nil
}

func (eb *eventBroker) GetStreamGroupStatistics(stream, group string) *RedisStreamGroupStatistics {
	stats := eb.GetStreamStatistics(stream)
	if stats == nil {
		return &RedisStreamGroupStatistics{Group: group}
	}
	for _, groupStats := range stats.Groups {
		if groupStats.Group == group {
			return groupStats
		}
	}
	return &RedisStreamGroupStatistics{
		Group: group,
		Lag:   int64(stats.Len),
	}
}

func (eb *eventBroker) GetStreamsStatistics(stream ...string) []*RedisStreamStatistics {
	now := time.Now()
	results := make([]*RedisStreamStatistics, 0)
	for redisPool, channels := range eb.engine.GetRegistry().GetRedisStreams() {
		redisCache := eb.engine.GetRedis(redisPool)
		for streamName := range channels {
			if !isStreamStatisticsNameRequested(streamName, stream) {
				continue
			}
			stat := &RedisStreamStatistics{Stream: streamName, RedisPool: redisPool}
			results = append(results, stat)
			stat.Groups = make([]*RedisStreamGroupStatistics, 0)
			stat.Len = uint64(redisCache.XLen(streamName))
			minPending := -1
			for _, group := range redisCache.XInfoGroups(streamName) {
				groupStats := &RedisStreamGroupStatistics{Group: group.Name, Pending: uint64(group.Pending)}
				groupStats.LastDeliveredID = group.LastDeliveredID
				groupStats.LastDeliveredDuration, _ = redisStreamIDToSince(group.LastDeliveredID, now)
				groupStats.Consumers = make([]*RedisStreamConsumerStatistics, 0)

				pending := redisCache.XPending(streamName, group.Name)
				groupStats.LowerID = pending.Lower
				if pending.Count > 0 {
					lower, lowerTime := redisStreamIDToSince(pending.Lower, now)
					groupStats.LowerDuration = lower
					if lower != 0 {
						since := time.Since(lowerTime)
						if minPending == -1 || int(since.Seconds()) > minPending {
							stat.OldestEventSeconds = int(since.Seconds())
							minPending = int(since.Seconds())
						}
					}
					for name, pendingCount := range pending.Consumers {
						consumer := &RedisStreamConsumerStatistics{Name: name, Pending: uint64(pendingCount)}
						groupStats.Consumers = append(groupStats.Consumers, consumer)
					}
				}
				stat.Groups = append(stat.Groups, groupStats)
			}
		}
	}
	return results
}

func isStreamStatisticsNameRequested(streamName string, requestedStreams []string) bool {
	if len(requestedStreams) == 0 {
		return true
	}
	for _, requestedStream := range requestedStreams {
		if requestedStream == streamName {
			return true
		}
	}
	return false
}

func redisStreamIDToSince(id string, now time.Time) (time.Duration, time.Time) {
	if id == "" || id == "0-0" {
		return 0, time.Now()
	}
	unixInt, _ := strconv.ParseInt(strings.Split(id, "-")[0], 10, 64)
	unix := time.Unix(0, unixInt*1000000)
	return time.Duration(int64(math.Max(float64(now.Sub(unix).Nanoseconds()), 0))), unix
}
