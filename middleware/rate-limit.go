package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

var timeFormat = "2006-01-02T15:04:05.000Z"

var inMemoryRateLimiter common.InMemoryRateLimiter

const redisRateLimitOpTimeout = 2 * time.Second

var redisSlidingWindowLimiter = redis.NewScript(`
local key = KEYS[1]
local max_request_num = tonumber(ARGV[1])
local duration = tonumber(ARGV[2])
local expiration = tonumber(ARGV[3])

if max_request_num <= 0 then
	return 1
end

local now = redis.call('TIME')
local now_seconds = tonumber(now[1])
local length = redis.call('LLEN', key)

if length < max_request_num then
	redis.call('LPUSH', key, now_seconds)
	redis.call('EXPIRE', key, expiration)
	return 1
end

local oldest = redis.call('LINDEX', key, -1)
local oldest_seconds = tonumber(oldest)

if oldest_seconds == nil then
	redis.call('DEL', key)
	redis.call('LPUSH', key, now_seconds)
	redis.call('EXPIRE', key, expiration)
	return 1
end

if now_seconds - oldest_seconds < duration then
	redis.call('EXPIRE', key, expiration)
	return 0
end

redis.call('LPUSH', key, now_seconds)
redis.call('LTRIM', key, 0, max_request_num - 1)
redis.call('EXPIRE', key, expiration)
return 1
`)

var defNext = func(c *gin.Context) {
	c.Next()
}

func rateLimitContext(c *gin.Context) context.Context {
	if c != nil && c.Request != nil {
		return c.Request.Context()
	}
	return context.Background()
}

func redisSlidingWindowAllow(ctx context.Context, rdb *redis.Client, key string, maxRequestNum int, duration int64, expiration time.Duration) (bool, error) {
	if maxRequestNum <= 0 {
		return true, nil
	}
	if expiration <= 0 {
		expiration = time.Duration(duration) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, redisRateLimitOpTimeout)
	defer cancel()

	result, err := redisSlidingWindowLimiter.Run(
		ctx,
		rdb,
		[]string{key},
		maxRequestNum,
		duration,
		int64(expiration.Seconds()),
	).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func applyRedisRateLimit(c *gin.Context, key string, maxRequestNum int, duration int64) {
	allowed, err := redisSlidingWindowAllow(rateLimitContext(c), common.RDB, key, maxRequestNum, duration, common.RateLimitKeyExpirationDuration)
	if err != nil {
		fmt.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		c.Abort()
		return
	}
	if !allowed {
		c.Status(http.StatusTooManyRequests)
		c.Abort()
	}
}

func redisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	key := "rateLimit:" + mark + c.ClientIP()
	applyRedisRateLimit(c, key, maxRequestNum, duration)
}

func memoryRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	key := mark + c.ClientIP()
	if !inMemoryRateLimiter.Request(key, maxRequestNum, duration) {
		c.Status(http.StatusTooManyRequests)
		c.Abort()
		return
	}
}

func rateLimitFactory(maxRequestNum int, duration int64, mark string) func(c *gin.Context) {
	if common.RedisEnabled {
		return func(c *gin.Context) {
			redisRateLimiter(c, maxRequestNum, duration, mark)
		}
	} else {
		// It's safe to call multi times.
		inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
		return func(c *gin.Context) {
			memoryRateLimiter(c, maxRequestNum, duration, mark)
		}
	}
}

func GlobalWebRateLimit() func(c *gin.Context) {
	if common.GlobalWebRateLimitEnable {
		return rateLimitFactory(common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration, "GW")
	}
	return defNext
}

func GlobalAPIRateLimit() func(c *gin.Context) {
	if common.GlobalApiRateLimitEnable {
		return rateLimitFactory(common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration, "GA")
	}
	return defNext
}

func CriticalRateLimit() func(c *gin.Context) {
	if common.CriticalRateLimitEnable {
		return rateLimitFactory(common.CriticalRateLimitNum, common.CriticalRateLimitDuration, "CT")
	}
	return defNext
}

func DownloadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(common.DownloadRateLimitNum, common.DownloadRateLimitDuration, "DW")
}

func UploadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(common.UploadRateLimitNum, common.UploadRateLimitDuration, "UP")
}

// userRateLimitFactory creates a rate limiter keyed by authenticated user ID
// instead of client IP, making it resistant to proxy rotation attacks.
// Must be used AFTER authentication middleware (UserAuth).
func userRateLimitFactory(maxRequestNum int, duration int64, mark string) func(c *gin.Context) {
	if common.RedisEnabled {
		return func(c *gin.Context) {
			userId := c.GetInt("id")
			if userId == 0 {
				c.Status(http.StatusUnauthorized)
				c.Abort()
				return
			}
			key := fmt.Sprintf("rateLimit:%s:user:%d", mark, userId)
			userRedisRateLimiter(c, maxRequestNum, duration, key)
		}
	}
	// It's safe to call multi times.
	inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	return func(c *gin.Context) {
		userId := c.GetInt("id")
		if userId == 0 {
			c.Status(http.StatusUnauthorized)
			c.Abort()
			return
		}
		key := fmt.Sprintf("%s:user:%d", mark, userId)
		if !inMemoryRateLimiter.Request(key, maxRequestNum, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}
	}
}

// userRedisRateLimiter is like redisRateLimiter but accepts a pre-built key
// (to support user-ID-based keys).
func userRedisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, key string) {
	applyRedisRateLimit(c, key, maxRequestNum, duration)
}

// SearchRateLimit returns a per-user rate limiter for search endpoints.
// Configurable via SEARCH_RATE_LIMIT_ENABLE / SEARCH_RATE_LIMIT / SEARCH_RATE_LIMIT_DURATION.
func SearchRateLimit() func(c *gin.Context) {
	if !common.SearchRateLimitEnable {
		return defNext
	}
	return userRateLimitFactory(common.SearchRateLimitNum, common.SearchRateLimitDuration, "SR")
}
