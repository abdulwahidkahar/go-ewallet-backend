package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

var rateLimitScript = redis.NewScript(`
local current = redis.call('INCR', KEYS[1])
if tonumber(current) == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
end
local ttl = redis.call('TTL', KEYS[1])
return {current, ttl}
`)

// RateLimiter returns a Gin middleware that limits request rate using Redis.
// action: name of the action/endpoint (e.g., "login", "otp", "transfer", "topup", "refresh")
// limit: maximum allowed requests in the given window
// window: duration of the rate limit window (e.g., 1 * time.Minute)
func RateLimiter(rdb *redis.Client, action string, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rdb == nil {
			c.Next()
			return
		}

		var identifier string
		if userID, exists := c.Get("id"); exists && userID != nil {
			identifier = fmt.Sprintf("user:%v", userID)
		} else {
			identifier = fmt.Sprintf("ip:%s", c.ClientIP())
		}

		key := fmt.Sprintf("rate_limit:%s:%s", action, identifier)
		windowSeconds := int64(window.Seconds())
		if windowSeconds <= 0 {
			windowSeconds = 60
		}

		ctx := c.Request.Context()
		res, err := rateLimitScript.Run(ctx, rdb, []string{key}, windowSeconds).Result()
		if err != nil {
			// If Redis fails, log or allow request so system remains available
			c.Next()
			return
		}

		results, ok := res.([]interface{})
		if !ok || len(results) < 2 {
			c.Next()
			return
		}

		var count int64
		var ttl int64

		if val, ok := results[0].(int64); ok {
			count = val
		}
		if val, ok := results[1].(int64); ok {
			ttl = val
		}

		if ttl < 0 {
			ttl = windowSeconds
		}

		remaining := limit - int(count)
		if remaining < 0 {
			remaining = 0
		}

		now := time.Now().Unix()
		resetTime := now + ttl

		c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(resetTime, 10))

		if count > int64(limit) {
			c.Header("Retry-After", strconv.FormatInt(ttl, 10))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Too many requests. Please try again later.",
			})
			return
		}

		c.Next()
	}
}
