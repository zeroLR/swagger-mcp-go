package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Limiter interface for rate limiting implementations
type Limiter interface {
	// Allow checks if a request is allowed, returns whether allowed and time until next allowed request
	Allow(key string) (allowed bool, retryAfter time.Duration)
	// Reset resets the rate limit for a key
	Reset(key string)
	// Config returns the current configuration
	Config() Config
}

// Config represents rate limiting configuration
type Config struct {
	RequestsPerMinute int           `yaml:"requestsPerMinute" json:"requestsPerMinute"`
	BurstSize         int           `yaml:"burstSize" json:"burstSize"`
	WindowSize        time.Duration `yaml:"windowSize" json:"windowSize"`
	KeyGenerator      KeyGenerator  `yaml:"-" json:"-"`
}

// KeyGenerator generates rate limiting keys from HTTP requests
type KeyGenerator func(*http.Request) string

// TokenBucketLimiter implements token bucket algorithm
type TokenBucketLimiter struct {
	config    Config
	buckets   map[string]*bucket
	mutex     sync.RWMutex
	logger    *zap.Logger
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

// bucket represents a token bucket for a specific key
type bucket struct {
	tokens     float64
	lastRefill time.Time
	mutex      sync.Mutex
}

// NewTokenBucketLimiter creates a new token bucket rate limiter
func NewTokenBucketLimiter(config Config, logger *zap.Logger) *TokenBucketLimiter {
	limiter := &TokenBucketLimiter{
		config:      config,
		buckets:     make(map[string]*bucket),
		logger:      logger,
		stopCleanup: make(chan struct{}),
	}

	// Set defaults
	if limiter.config.RequestsPerMinute <= 0 {
		limiter.config.RequestsPerMinute = 100
	}
	if limiter.config.BurstSize <= 0 {
		limiter.config.BurstSize = limiter.config.RequestsPerMinute
	}
	if limiter.config.WindowSize <= 0 {
		limiter.config.WindowSize = time.Minute
	}
	if limiter.config.KeyGenerator == nil {
		limiter.config.KeyGenerator = DefaultKeyGenerator
	}

	// Start cleanup goroutine
	limiter.cleanupTicker = time.NewTicker(5 * time.Minute)
	go limiter.cleanup()

	return limiter
}

// Allow checks if a request is allowed
func (l *TokenBucketLimiter) Allow(key string) (bool, time.Duration) {
	l.mutex.Lock()
	b, exists := l.buckets[key]
	if !exists {
		b = &bucket{
			tokens:     float64(l.config.BurstSize),
			lastRefill: time.Now(),
		}
		l.buckets[key] = b
	}
	l.mutex.Unlock()

	b.mutex.Lock()
	defer b.mutex.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastRefill)
	
	// Calculate tokens to add based on elapsed time
	tokensToAdd := elapsed.Seconds() * (float64(l.config.RequestsPerMinute) / l.config.WindowSize.Seconds())
	b.tokens = min(float64(l.config.BurstSize), b.tokens+tokensToAdd)
	b.lastRefill = now

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true, 0
	}

	// Calculate time until next token is available
	tokensNeeded := 1.0 - b.tokens
	tokensPerSecond := float64(l.config.RequestsPerMinute) / l.config.WindowSize.Seconds()
	secondsToWait := tokensNeeded / tokensPerSecond
	retryAfter := time.Duration(secondsToWait * float64(time.Second))

	return false, retryAfter
}

// Reset resets the rate limit for a key
func (l *TokenBucketLimiter) Reset(key string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	delete(l.buckets, key)
}

// Config returns the current configuration
func (l *TokenBucketLimiter) Config() Config {
	return l.config
}

// Stop stops the cleanup goroutine
func (l *TokenBucketLimiter) Stop() {
	select {
	case <-l.stopCleanup:
		// Already stopped
		return
	default:
		close(l.stopCleanup)
	}
	if l.cleanupTicker != nil {
		l.cleanupTicker.Stop()
	}
}

// cleanup removes old buckets
func (l *TokenBucketLimiter) cleanup() {
	for {
		select {
		case <-l.cleanupTicker.C:
			l.mutex.Lock()
			now := time.Now()
			for key, b := range l.buckets {
				b.mutex.Lock()
				if now.Sub(b.lastRefill) > 10*time.Minute {
					delete(l.buckets, key)
				}
				b.mutex.Unlock()
			}
			l.mutex.Unlock()
		case <-l.stopCleanup:
			return
		}
	}
}

// SlidingWindowLimiter implements sliding window algorithm
type SlidingWindowLimiter struct {
	config  Config
	windows map[string]*window
	mutex   sync.RWMutex
	logger  *zap.Logger
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

// window represents a sliding window for a specific key
type window struct {
	requests []time.Time
	mutex    sync.Mutex
}

// NewSlidingWindowLimiter creates a new sliding window rate limiter
func NewSlidingWindowLimiter(config Config, logger *zap.Logger) *SlidingWindowLimiter {
	limiter := &SlidingWindowLimiter{
		config:      config,
		windows:     make(map[string]*window),
		logger:      logger,
		stopCleanup: make(chan struct{}),
	}

	// Set defaults
	if limiter.config.RequestsPerMinute <= 0 {
		limiter.config.RequestsPerMinute = 100
	}
	if limiter.config.WindowSize <= 0 {
		limiter.config.WindowSize = time.Minute
	}
	if limiter.config.KeyGenerator == nil {
		limiter.config.KeyGenerator = DefaultKeyGenerator
	}

	// Start cleanup goroutine
	limiter.cleanupTicker = time.NewTicker(5 * time.Minute)
	go limiter.cleanup()

	return limiter
}

// Allow checks if a request is allowed
func (l *SlidingWindowLimiter) Allow(key string) (bool, time.Duration) {
	l.mutex.Lock()
	w, exists := l.windows[key]
	if !exists {
		w = &window{
			requests: make([]time.Time, 0),
		}
		l.windows[key] = w
	}
	l.mutex.Unlock()

	w.mutex.Lock()
	defer w.mutex.Unlock()

	now := time.Now()
	windowStart := now.Add(-l.config.WindowSize)

	// Remove old requests outside the window
	validRequests := make([]time.Time, 0)
	for _, reqTime := range w.requests {
		if reqTime.After(windowStart) {
			validRequests = append(validRequests, reqTime)
		}
	}
	w.requests = validRequests

	if len(w.requests) < l.config.RequestsPerMinute {
		w.requests = append(w.requests, now)
		return true, 0
	}

	// Calculate retry after time based on oldest request
	if len(w.requests) > 0 {
		oldestRequest := w.requests[0]
		retryAfter := oldestRequest.Add(l.config.WindowSize).Sub(now)
		if retryAfter < 0 {
			retryAfter = 0
		}
		return false, retryAfter
	}

	return false, l.config.WindowSize
}

// Reset resets the rate limit for a key
func (l *SlidingWindowLimiter) Reset(key string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	delete(l.windows, key)
}

// Config returns the current configuration
func (l *SlidingWindowLimiter) Config() Config {
	return l.config
}

// Stop stops the cleanup goroutine
func (l *SlidingWindowLimiter) Stop() {
	select {
	case <-l.stopCleanup:
		// Already stopped
		return
	default:
		close(l.stopCleanup)
	}
	if l.cleanupTicker != nil {
		l.cleanupTicker.Stop()
	}
}

// cleanup removes old windows
func (l *SlidingWindowLimiter) cleanup() {
	for {
		select {
		case <-l.cleanupTicker.C:
			l.mutex.Lock()
			now := time.Now()
			for key, w := range l.windows {
				w.mutex.Lock()
				if len(w.requests) == 0 || (len(w.requests) > 0 && now.Sub(w.requests[len(w.requests)-1]) > 10*time.Minute) {
					delete(l.windows, key)
				}
				w.mutex.Unlock()
			}
			l.mutex.Unlock()
		case <-l.stopCleanup:
			return
		}
	}
}

// Manager manages rate limiting across services
type Manager struct {
	limiters map[string]Limiter
	logger   *zap.Logger
	enabled  bool
	mutex    sync.RWMutex
}

// NewManager creates a new rate limiting manager
func NewManager(logger *zap.Logger, enabled bool) *Manager {
	return &Manager{
		limiters: make(map[string]Limiter),
		logger:   logger,
		enabled:  enabled,
	}
}

// SetServiceLimiter sets a rate limiter for a specific service
func (m *Manager) SetServiceLimiter(serviceName string, limiter Limiter) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.limiters[serviceName] = limiter
	m.logger.Info("Set rate limiter for service",
		zap.String("service", serviceName),
		zap.Int("requestsPerMinute", limiter.Config().RequestsPerMinute))
}

// SetGlobalLimiter sets a global rate limiter for all services
func (m *Manager) SetGlobalLimiter(limiter Limiter) {
	m.SetServiceLimiter("*", limiter)
}

// IsAllowed checks if a request is allowed for a service
func (m *Manager) IsAllowed(serviceName string, req *http.Request) (bool, time.Duration) {
	if !m.enabled {
		return true, 0
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Try service-specific limiter first
	limiter, exists := m.limiters[serviceName]
	if !exists {
		// Fall back to global limiter
		limiter, exists = m.limiters["*"]
		if !exists {
			return true, 0
		}
	}

	key := limiter.Config().KeyGenerator(req)
	return limiter.Allow(key)
}

// ResetKey resets rate limiting for a specific key across all services
func (m *Manager) ResetKey(key string) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, limiter := range m.limiters {
		limiter.Reset(key)
	}
}

// GetStats returns rate limiting statistics
func (m *Manager) GetStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := map[string]interface{}{
		"enabled":        m.enabled,
		"serviceLimiters": len(m.limiters),
		"limiters":       make(map[string]interface{}),
	}

	for serviceName, limiter := range m.limiters {
		config := limiter.Config()
		stats["limiters"].(map[string]interface{})[serviceName] = map[string]interface{}{
			"requestsPerMinute": config.RequestsPerMinute,
			"burstSize":         config.BurstSize,
			"windowSize":        config.WindowSize.String(),
		}
	}

	return stats
}

// Stop stops all rate limiters
func (m *Manager) Stop() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, limiter := range m.limiters {
		if tbl, ok := limiter.(*TokenBucketLimiter); ok {
			tbl.Stop()
		}
		if swl, ok := limiter.(*SlidingWindowLimiter); ok {
			swl.Stop()
		}
	}
}

// Middleware creates an HTTP middleware for rate limiting
func (m *Manager) Middleware(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, retryAfter := m.IsAllowed(serviceName, r)
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
				w.Header().Set("X-RateLimit-Limit", "")
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(retryAfter).Unix(), 10))
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Default key generators

// DefaultKeyGenerator generates keys based on client IP
func DefaultKeyGenerator(req *http.Request) string {
	return getClientIP(req)
}

// UserBasedKeyGenerator generates keys based on authenticated user
func UserBasedKeyGenerator(req *http.Request) string {
	// Try to get user from context first (set by auth middleware)
	if userID := getUserFromContext(req.Context()); userID != "" {
		return "user:" + userID
	}
	// Fall back to IP
	return "ip:" + getClientIP(req)
}

// ServiceBasedKeyGenerator generates keys based on service name
func ServiceBasedKeyGenerator(serviceName string) KeyGenerator {
	return func(req *http.Request) string {
		return fmt.Sprintf("service:%s:ip:%s", serviceName, getClientIP(req))
	}
}

// getClientIP extracts the client IP from the request
func getClientIP(req *http.Request) string {
	// Check X-Forwarded-For header first
	xff := req.Header.Get("X-Forwarded-For")
	if xff != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		if comma := req.Header.Get("X-Forwarded-For"); comma != "" {
			return comma
		}
	}

	// Check X-Real-IP header
	if xri := req.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return req.RemoteAddr
}

// getUserFromContext extracts user ID from request context
func getUserFromContext(ctx context.Context) string {
	// This would typically extract from auth context
	// For now, return empty string
	if authCtx := ctx.Value("authContext"); authCtx != nil {
		// This would extract user ID from auth context
		// Implementation depends on auth system
	}
	return ""
}

// Helper function for min
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}