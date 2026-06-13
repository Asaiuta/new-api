package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	ModelRequestRateLimitCountMark        = "MRRL"
	ModelRequestRateLimitSuccessCountMark = "MRRLS"
)

var redisModelSuccessLimitCheckScript = redis.NewScript(`
local key = KEYS[1]
local max_count = tonumber(ARGV[1])
local duration = tonumber(ARGV[2])
local expiration = tonumber(ARGV[3])

if max_count <= 0 then
	return 1
end

local now = redis.call('TIME')
local now_seconds = tonumber(now[1])
local length = redis.call('LLEN', key)

if length < max_count then
	return 1
end

local oldest = redis.call('LINDEX', key, -1)
local oldest_seconds = tonumber(oldest)

if oldest_seconds == nil then
	redis.call('DEL', key)
	return 1
end

if now_seconds - oldest_seconds < duration then
	redis.call('EXPIRE', key, expiration)
	return 0
end

return 1
`)

var redisModelSuccessLimitRecordScript = redis.NewScript(`
local key = KEYS[1]
local max_count = tonumber(ARGV[1])
local expiration = tonumber(ARGV[2])

if max_count <= 0 then
	return 1
end

local now = redis.call('TIME')
local now_seconds = tonumber(now[1])

redis.call('LPUSH', key, now_seconds)
redis.call('LTRIM', key, 0, max_count - 1)
redis.call('EXPIRE', key, expiration)
return 1
`)

func modelRequestRateLimitExpiration(duration int64) time.Duration {
	expiration := time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute
	if expiration <= 0 {
		expiration = time.Duration(duration) * time.Second
	}
	return expiration
}

// 检查Redis中的请求限制
func checkRedisRateLimit(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64) (bool, error) {
	// 如果maxCount为0，表示不限制
	if maxCount <= 0 {
		return true, nil
	}

	ctx, cancel := context.WithTimeout(ctx, redisRateLimitOpTimeout)
	defer cancel()

	result, err := redisModelSuccessLimitCheckScript.Run(
		ctx,
		rdb,
		[]string{key},
		maxCount,
		duration,
		int64(modelRequestRateLimitExpiration(duration).Seconds()),
	).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// 记录Redis请求
func recordRedisRequest(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64) error {
	// 如果maxCount为0，不记录请求
	if maxCount <= 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, redisRateLimitOpTimeout)
	defer cancel()

	return redisModelSuccessLimitRecordScript.Run(
		ctx,
		rdb,
		[]string{key},
		maxCount,
		int64(modelRequestRateLimitExpiration(duration).Seconds()),
	).Err()
}

// Redis限流处理器
func redisRateLimitHandler(duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		ctx := rateLimitContext(c)
		rdb := common.RDB

		// 1. 检查成功请求数限制
		successKey := fmt.Sprintf("rateLimit:%s:%s", ModelRequestRateLimitSuccessCountMark, userId)
		allowed, err := checkRedisRateLimit(ctx, rdb, successKey, successMaxCount, duration)
		if err != nil {
			fmt.Println("检查成功请求数限制失败:", err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", setting.ModelRequestRateLimitDurationMinutes, successMaxCount))
			return
		}

		//2.检查总请求数限制并记录总请求（当totalMaxCount为0时会自动跳过，使用令牌桶限流器
		if totalMaxCount > 0 {
			totalKey := fmt.Sprintf("rateLimit:%s", userId)
			// 初始化
			tb := limiter.New(ctx, rdb)
			allowed, err = tb.Allow(
				ctx,
				totalKey,
				limiter.WithCapacity(int64(totalMaxCount)*duration),
				limiter.WithRate(int64(totalMaxCount)),
				limiter.WithRequested(duration),
			)

			if err != nil {
				fmt.Println("检查总请求数限制失败:", err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
				return
			}

			if !allowed {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", setting.ModelRequestRateLimitDurationMinutes, totalMaxCount))
			}
		}

		// 4. 处理请求
		c.Next()

		// 5. 如果请求成功，记录成功请求
		if c.Writer.Status() < 400 {
			if err := recordRedisRequest(ctx, rdb, successKey, successMaxCount, duration); err != nil {
				fmt.Println("记录成功请求失败:", err.Error())
			}
		}
	}
}

// 内存限流处理器
func memoryRateLimitHandler(duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)

	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		totalKey := ModelRequestRateLimitCountMark + userId
		successKey := ModelRequestRateLimitSuccessCountMark + userId

		// 1. 检查总请求数限制（当totalMaxCount为0时跳过）
		if totalMaxCount > 0 && !inMemoryRateLimiter.Request(totalKey, totalMaxCount, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		// 2. 检查成功请求数限制
		// 使用一个临时key来检查限制，这样可以避免实际记录
		checkKey := successKey + "_check"
		if !inMemoryRateLimiter.Request(checkKey, successMaxCount, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		// 3. 处理请求
		c.Next()

		// 4. 如果请求成功，记录到实际的成功请求计数中
		if c.Writer.Status() < 400 {
			inMemoryRateLimiter.Request(successKey, successMaxCount, duration)
		}
	}
}

// ModelRequestRateLimit 模型请求限流中间件
func ModelRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 在每个请求时检查是否启用限流
		if !setting.ModelRequestRateLimitEnabled {
			c.Next()
			return
		}

		// 计算限流参数
		duration := int64(setting.ModelRequestRateLimitDurationMinutes * 60)
		totalMaxCount := setting.ModelRequestRateLimitCount
		successMaxCount := setting.ModelRequestRateLimitSuccessCount

		// 获取分组
		group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
		if group == "" {
			group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		}

		//获取分组的限流配置
		groupTotalCount, groupSuccessCount, found := setting.GetGroupRateLimit(group)
		if found {
			totalMaxCount = groupTotalCount
			successMaxCount = groupSuccessCount
		}

		// 根据存储类型选择并执行限流处理器
		if common.RedisEnabled {
			redisRateLimitHandler(duration, totalMaxCount, successMaxCount)(c)
		} else {
			memoryRateLimitHandler(duration, totalMaxCount, successMaxCount)(c)
		}
	}
}
