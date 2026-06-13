package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	EmailVerificationRateLimitMark = "EV"
	EmailVerificationMaxRequests   = 2  // 30秒内最多2次
	EmailVerificationDuration      = 30 // 30秒时间窗口
)

var redisEmailVerificationLimiter = redis.NewScript(`
local key = KEYS[1]
local max_requests = tonumber(ARGV[1])
local duration = tonumber(ARGV[2])

if max_requests <= 0 then
	return {1, 0}
end

local count = redis.call('INCR', key)
if count == 1 then
	redis.call('EXPIRE', key, duration)
end

if count <= max_requests then
	return {1, 0}
end

local ttl = redis.call('TTL', key)
if ttl < 0 then
	ttl = duration
	redis.call('EXPIRE', key, duration)
end

return {0, ttl}
`)

func redisScriptInt64(v any) (int64, error) {
	switch value := v.(type) {
	case int64:
		return value, nil
	case int:
		return int64(value), nil
	case string:
		return strconv.ParseInt(value, 10, 64)
	case []byte:
		return strconv.ParseInt(string(value), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected redis script integer type %T", v)
	}
}

func parseEmailVerificationLimitResult(values []any) (bool, int64, error) {
	if len(values) != 2 {
		return false, 0, fmt.Errorf("unexpected redis email verification result length %d", len(values))
	}
	allowedNum, err := redisScriptInt64(values[0])
	if err != nil {
		return false, 0, err
	}
	waitSeconds, err := redisScriptInt64(values[1])
	if err != nil {
		return false, 0, err
	}
	return allowedNum == 1, waitSeconds, nil
}

func redisEmailVerificationAllow(ctx context.Context, rdb *redis.Client, key string, maxRequests int, durationSeconds int64) (bool, int64, error) {
	if maxRequests <= 0 {
		return true, 0, nil
	}
	if durationSeconds <= 0 {
		durationSeconds = EmailVerificationDuration
	}
	ctx, cancel := context.WithTimeout(ctx, redisRateLimitOpTimeout)
	defer cancel()

	values, err := redisEmailVerificationLimiter.Run(
		ctx,
		rdb,
		[]string{key},
		maxRequests,
		durationSeconds,
	).Slice()
	if err != nil {
		return false, 0, err
	}
	return parseEmailVerificationLimitResult(values)
}

func redisEmailVerificationRateLimiter(c *gin.Context) {
	rdb := common.RDB
	key := "emailVerification:" + EmailVerificationRateLimitMark + ":" + c.ClientIP()

	allowed, waitSeconds, err := redisEmailVerificationAllow(rateLimitContext(c), rdb, key, EmailVerificationMaxRequests, EmailVerificationDuration)
	if err != nil {
		// fallback
		memoryEmailVerificationRateLimiter(c)
		return
	}

	if allowed {
		c.Next()
		return
	}

	if waitSeconds <= 0 {
		waitSeconds = int64((time.Duration(EmailVerificationDuration) * time.Second).Seconds())
	}

	c.JSON(http.StatusTooManyRequests, gin.H{
		"success": false,
		"message": fmt.Sprintf("发送过于频繁，请等待 %d 秒后再试", waitSeconds),
	})
	c.Abort()
}

func memoryEmailVerificationRateLimiter(c *gin.Context) {
	key := EmailVerificationRateLimitMark + ":" + c.ClientIP()

	if !inMemoryRateLimiter.Request(key, EmailVerificationMaxRequests, EmailVerificationDuration) {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"success": false,
			"message": "发送过于频繁，请稍后再试",
		})
		c.Abort()
		return
	}

	c.Next()
}

func EmailVerificationRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if common.RedisEnabled {
			redisEmailVerificationRateLimiter(c)
		} else {
			inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
			memoryEmailVerificationRateLimiter(c)
		}
	}
}
