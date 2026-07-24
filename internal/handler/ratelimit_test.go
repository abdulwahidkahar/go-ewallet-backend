package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func TestRateLimiter_NilRedisClient(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(RateLimiter(nil, "test", 2, time.Minute))
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 when rdb is nil, got %d", w.Code)
	}
}

func TestRateLimiter_WithRedis(t *testing.T) {
	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}
	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = "6379"
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", redisHost, redisPort),
	})

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skip("skipping Redis rate limit test: Redis not reachable")
	}

	testAction := fmt.Sprintf("test_action_%d", time.Now().UnixNano())
	limit := 3
	window := 10 * time.Second

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimiter(rdb, testAction, limit, window))
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// Perform 3 requests (within limit)
	for i := 1; i <= limit; i++ {
		req, _ := http.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected status 200, got %d", i, w.Code)
		}

		limitHeader := w.Header().Get("X-RateLimit-Limit")
		if limitHeader != "3" {
			t.Fatalf("request %d: expected X-RateLimit-Limit 3, got %s", i, limitHeader)
		}

		remainingHeader := w.Header().Get("X-RateLimit-Remaining")
		expectedRemaining := fmt.Sprintf("%d", limit-i)
		if remainingHeader != expectedRemaining {
			t.Fatalf("request %d: expected X-RateLimit-Remaining %s, got %s", i, expectedRemaining, remainingHeader)
		}
	}

	// 4th request should be rate limited (429)
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429 for request beyond limit, got %d", w.Code)
	}

	if w.Header().Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header on 429 response")
	}
}
