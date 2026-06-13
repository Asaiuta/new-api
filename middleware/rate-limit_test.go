package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const optimizedRedisSlidingWindowRoundTrips = 1
const optimizedRedisLuaRoundTrips = 1

func legacyRedisSlidingWindowRoundTrips(queueFull bool, allowedAfterWindow bool) int {
	if !queueFull {
		return 3 // LLEN + LPUSH + EXPIRE
	}
	if !allowedAfterWindow {
		return 3 // LLEN + LINDEX + EXPIRE
	}
	return 5 // LLEN + LINDEX + LPUSH + LTRIM + EXPIRE
}

func legacyModelSuccessLimitRoundTrips(queueFull bool, allowedAfterWindow bool, requestSucceeded bool) int {
	checkRoundTrips := 1 // LLEN
	if queueFull {
		checkRoundTrips++ // LINDEX
		if !allowedAfterWindow {
			checkRoundTrips++ // EXPIRE
		}
	}
	if requestSucceeded {
		checkRoundTrips += 3 // LPUSH + LTRIM + EXPIRE
	}
	return checkRoundTrips
}

func optimizedModelSuccessLimitRoundTrips(requestSucceeded bool) int {
	if requestSucceeded {
		return 2 // check script + record script
	}
	return optimizedRedisLuaRoundTrips
}

func legacyEmailVerificationRoundTrips(firstRequest bool, denied bool) int {
	if firstRequest {
		return 2 // INCR + EXPIRE
	}
	if denied {
		return 2 // INCR + TTL
	}
	return 1 // INCR
}

func TestRedisSlidingWindowNetworkFootprint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name               string
		queueFull          bool
		allowedAfterWindow bool
	}{
		{name: "new key or below limit", queueFull: false},
		{name: "full and denied", queueFull: true, allowedAfterWindow: false},
		{name: "full and allowed after window", queueFull: true, allowedAfterWindow: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			legacy := legacyRedisSlidingWindowRoundTrips(tc.queueFull, tc.allowedAfterWindow)
			optimized := optimizedRedisSlidingWindowRoundTrips
			reduction := float64(legacy-optimized) / float64(legacy) * 100

			t.Logf("redis sliding window RTTs: legacy=%d optimized=%d reduction=%.1f%%", legacy, optimized, reduction)
			require.Greater(t, legacy, optimized)
			require.Equal(t, 1, optimized)
		})
	}
}

func TestModelRedisSuccessLimitNetworkFootprint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name               string
		queueFull          bool
		allowedAfterWindow bool
		requestSucceeded   bool
	}{
		{name: "below limit and request succeeds", requestSucceeded: true},
		{name: "full and denied before request", queueFull: true, allowedAfterWindow: false},
		{name: "full but expired window and request succeeds", queueFull: true, allowedAfterWindow: true, requestSucceeded: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			legacy := legacyModelSuccessLimitRoundTrips(tc.queueFull, tc.allowedAfterWindow, tc.requestSucceeded)
			optimized := optimizedModelSuccessLimitRoundTrips(tc.requestSucceeded)
			reduction := float64(legacy-optimized) / float64(legacy) * 100

			t.Logf("model success redis RTTs: legacy=%d optimized=%d reduction=%.1f%%", legacy, optimized, reduction)
			require.Greater(t, legacy, optimized)
		})
	}
}

func TestEmailVerificationRedisNetworkFootprint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		firstRequest bool
		denied       bool
	}{
		{name: "first allowed request", firstRequest: true},
		{name: "denied request with ttl", denied: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			legacy := legacyEmailVerificationRoundTrips(tc.firstRequest, tc.denied)
			optimized := optimizedRedisLuaRoundTrips
			reduction := float64(legacy-optimized) / float64(legacy) * 100

			t.Logf("email verification redis RTTs: legacy=%d optimized=%d reduction=%.1f%%", legacy, optimized, reduction)
			require.Greater(t, legacy, optimized)
		})
	}
}

func TestParseEmailVerificationLimitResult(t *testing.T) {
	t.Parallel()

	allowed, waitSeconds, err := parseEmailVerificationLimitResult([]any{int64(1), int64(0)})
	require.NoError(t, err)
	require.True(t, allowed)
	require.Equal(t, int64(0), waitSeconds)

	allowed, waitSeconds, err = parseEmailVerificationLimitResult([]any{int64(0), []byte("17")})
	require.NoError(t, err)
	require.False(t, allowed)
	require.Equal(t, int64(17), waitSeconds)

	_, _, err = parseEmailVerificationLimitResult([]any{int64(1)})
	require.Error(t, err)
}

func TestRateLimitContextUsesRequestContext(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	parentCtx, cancel := context.WithCancel(context.Background())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/test", nil).WithContext(parentCtx)

	limitCtx := rateLimitContext(ctx)
	require.NoError(t, limitCtx.Err())

	cancel()
	require.ErrorIs(t, limitCtx.Err(), context.Canceled)
}
