package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestTokenBucketLimiter(t *testing.T) {
	config := Config{
		RequestsPerMinute: 60, // 1 request per second
		BurstSize:         10,
		WindowSize:        time.Minute,
		KeyGenerator:      DefaultKeyGenerator,
	}

	logger := zap.NewNop()
	limiter := NewTokenBucketLimiter(config, logger)
	defer limiter.Stop()

	key := "test-key"

	// Should allow initial burst
	for i := 0; i < 10; i++ {
		allowed, _ := limiter.Allow(key)
		if !allowed {
			t.Errorf("Request %d should be allowed (burst)", i+1)
		}
	}

	// Next request should be rejected
	allowed, retryAfter := limiter.Allow(key)
	if allowed {
		t.Errorf("Request should be rejected after burst exhausted")
	}
	if retryAfter <= 0 {
		t.Errorf("Retry after should be positive")
	}

	// Test token refill after some time
	time.Sleep(2 * time.Second)
	allowed, _ = limiter.Allow(key)
	if !allowed {
		t.Errorf("Request should be allowed after token refill")
	}
}

func TestSlidingWindowLimiter(t *testing.T) {
	config := Config{
		RequestsPerMinute: 3,
		WindowSize:        time.Second, // 3 requests per second for testing
		KeyGenerator:      DefaultKeyGenerator,
	}

	logger := zap.NewNop()
	limiter := NewSlidingWindowLimiter(config, logger)
	defer limiter.Stop()

	key := "test-key"

	// Should allow 3 requests
	for i := 0; i < 3; i++ {
		allowed, _ := limiter.Allow(key)
		if !allowed {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 4th request should be rejected
	allowed, retryAfter := limiter.Allow(key)
	if allowed {
		t.Errorf("4th request should be rejected")
	}
	if retryAfter <= 0 {
		t.Errorf("Retry after should be positive")
	}

	// After window expires, should allow requests again
	time.Sleep(1100 * time.Millisecond) // Wait for window to expire
	allowed, _ = limiter.Allow(key)
	if !allowed {
		t.Errorf("Request should be allowed after window expiry")
	}
}

func TestManager(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger, true)
	defer manager.Stop()

	// Create test request
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:8080"

	// Set up rate limiter for service
	config := Config{
		RequestsPerMinute: 2,
		WindowSize:        time.Second,
		KeyGenerator:      DefaultKeyGenerator,
	}
	limiter := NewTokenBucketLimiter(config, logger)
	defer limiter.Stop()

	manager.SetServiceLimiter("test-service", limiter)

	// Should allow first 2 requests
	for i := 0; i < 2; i++ {
		allowed, _ := manager.IsAllowed("test-service", req)
		if !allowed {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 3rd request should be rejected
	allowed, retryAfter := manager.IsAllowed("test-service", req)
	if allowed {
		t.Errorf("3rd request should be rejected")
	}
	if retryAfter <= 0 {
		t.Errorf("Retry after should be positive")
	}

	// Test global limiter
	globalConfig := Config{
		RequestsPerMinute: 1,
		WindowSize:        time.Second,
		KeyGenerator:      DefaultKeyGenerator,
	}
	globalLimiter := NewTokenBucketLimiter(globalConfig, logger)
	defer globalLimiter.Stop()

	manager.SetGlobalLimiter(globalLimiter)

	// Should use global limiter for unknown service
	allowed, _ = manager.IsAllowed("unknown-service", req)
	if !allowed {
		t.Errorf("Request should be allowed by global limiter")
	}

	allowed, _ = manager.IsAllowed("unknown-service", req)
	if allowed {
		t.Errorf("Second request should be rejected by global limiter")
	}
}

func TestManagerMiddleware(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger, true)
	defer manager.Stop()

	// Set up very restrictive rate limiter
	config := Config{
		RequestsPerMinute: 1,
		BurstSize:         1,
		WindowSize:        time.Minute,
		KeyGenerator:      DefaultKeyGenerator,
	}
	limiter := NewTokenBucketLimiter(config, logger)
	defer limiter.Stop()

	manager.SetServiceLimiter("test-service", limiter)

	// Create test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	middleware := manager.Middleware("test-service")
	wrappedHandler := middleware(handler)

	// First request should succeed
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "192.168.1.1:8080"
	recorder1 := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(recorder1, req1)
	if recorder1.Code != http.StatusOK {
		t.Errorf("First request should succeed, got status %d", recorder1.Code)
	}

	// Second request should be rate limited
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "192.168.1.1:8080"
	recorder2 := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(recorder2, req2)
	if recorder2.Code != http.StatusTooManyRequests {
		t.Errorf("Second request should be rate limited, got status %d", recorder2.Code)
	}

	// Check rate limit headers
	if recorder2.Header().Get("Retry-After") == "" {
		t.Errorf("Retry-After header should be set")
	}
	if recorder2.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("X-RateLimit-Remaining should be 0")
	}
}

func TestKeyGenerators(t *testing.T) {
	// Test DefaultKeyGenerator
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:8080"

	key := DefaultKeyGenerator(req)
	if key != "192.168.1.1:8080" {
		t.Errorf("Expected key to be RemoteAddr, got %s", key)
	}

	// Test with X-Forwarded-For header
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	key = DefaultKeyGenerator(req)
	if key != "10.0.0.1" {
		t.Errorf("Expected key to be X-Forwarded-For value, got %s", key)
	}

	// Test ServiceBasedKeyGenerator
	serviceGen := ServiceBasedKeyGenerator("my-service")
	key = serviceGen(req)
	expected := "service:my-service:ip:10.0.0.1"
	if key != expected {
		t.Errorf("Expected service-based key %s, got %s", expected, key)
	}
}

func TestManagerStats(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger, true)
	defer manager.Stop()

	config := Config{
		RequestsPerMinute: 100,
		BurstSize:         10,
		WindowSize:        time.Minute,
	}
	limiter := NewTokenBucketLimiter(config, logger)
	defer limiter.Stop()

	manager.SetServiceLimiter("test-service", limiter)
	manager.SetGlobalLimiter(limiter)

	stats := manager.GetStats()

	if !stats["enabled"].(bool) {
		t.Errorf("Expected enabled to be true")
	}
	if stats["serviceLimiters"].(int) != 2 {
		t.Errorf("Expected 2 service limiters, got %d", stats["serviceLimiters"].(int))
	}

	limiters := stats["limiters"].(map[string]interface{})
	if len(limiters) != 2 {
		t.Errorf("Expected 2 limiters in stats, got %d", len(limiters))
	}

	if _, exists := limiters["test-service"]; !exists {
		t.Errorf("Expected test-service limiter in stats")
	}
	if _, exists := limiters["*"]; !exists {
		t.Errorf("Expected global limiter (*) in stats")
	}
}
