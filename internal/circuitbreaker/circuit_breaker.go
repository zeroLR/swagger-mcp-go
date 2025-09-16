package circuitbreaker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// State represents the circuit breaker state
type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config represents circuit breaker configuration
type Config struct {
	MaxFailures     int           `yaml:"maxFailures" json:"maxFailures"`
	ResetTimeout    time.Duration `yaml:"resetTimeout" json:"resetTimeout"`
	SuccessThreshold int          `yaml:"successThreshold" json:"successThreshold"`
	Timeout         time.Duration `yaml:"timeout" json:"timeout"`
}

// ExecutorFunc represents a function that can be executed by the circuit breaker
type ExecutorFunc func(ctx context.Context) (interface{}, error)

// FallbackFunc represents a fallback function to execute when circuit is open
type FallbackFunc func(ctx context.Context, err error) (interface{}, error)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config           Config
	state            State
	failures         int
	successes        int
	lastFailureTime  time.Time
	nextAttempt      time.Time
	mutex            sync.RWMutex
	logger           *zap.Logger
	name             string
	
	// Metrics
	totalRequests     int64
	totalFailures     int64
	totalSuccesses    int64
	totalTimeouts     int64
	totalRejected     int64
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(name string, config Config, logger *zap.Logger) *CircuitBreaker {
	// Set defaults
	if config.MaxFailures <= 0 {
		config.MaxFailures = 5
	}
	if config.ResetTimeout <= 0 {
		config.ResetTimeout = 60 * time.Second
	}
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = 1
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}

	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
		logger: logger,
		name:   name,
	}
}

// Execute executes a function with circuit breaker protection
func (cb *CircuitBreaker) Execute(ctx context.Context, executor ExecutorFunc) (interface{}, error) {
	return cb.ExecuteWithFallback(ctx, executor, nil)
}

// ExecuteWithFallback executes a function with circuit breaker protection and optional fallback
func (cb *CircuitBreaker) ExecuteWithFallback(ctx context.Context, executor ExecutorFunc, fallback FallbackFunc) (interface{}, error) {
	cb.mutex.Lock()
	cb.totalRequests++
	
	state := cb.state
	switch state {
	case StateOpen:
		if time.Now().Before(cb.nextAttempt) {
			cb.totalRejected++
			cb.mutex.Unlock()
			err := fmt.Errorf("circuit breaker '%s' is open", cb.name)
			if fallback != nil {
				return fallback(ctx, err)
			}
			return nil, err
		}
		// Time to attempt reset
		cb.state = StateHalfOpen
		cb.logger.Info("Circuit breaker transitioning to half-open",
			zap.String("name", cb.name))
		fallthrough
		
	case StateHalfOpen:
		// Allow limited requests through
		cb.mutex.Unlock()
		
	case StateClosed:
		// Normal operation
		cb.mutex.Unlock()
	}

	// Execute with timeout
	done := make(chan struct{})
	var result interface{}
	var err error

	go func() {
		defer close(done)
		result, err = executor(ctx)
	}()

	select {
	case <-done:
		// Execution completed
		cb.onResult(err)
		return result, err
		
	case <-time.After(cb.config.Timeout):
		// Execution timed out
		cb.mutex.Lock()
		cb.totalTimeouts++
		cb.mutex.Unlock()
		cb.onResult(fmt.Errorf("execution timeout"))
		
		timeoutErr := fmt.Errorf("circuit breaker '%s' execution timeout", cb.name)
		if fallback != nil {
			return fallback(ctx, timeoutErr)
		}
		return nil, timeoutErr
		
	case <-ctx.Done():
		// Context cancelled
		cb.onResult(ctx.Err())
		return nil, ctx.Err()
	}
}

// onResult handles the result of an execution
func (cb *CircuitBreaker) onResult(err error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if err != nil {
		cb.onFailure()
	} else {
		cb.onSuccess()
	}
}

// onFailure handles a failed execution
func (cb *CircuitBreaker) onFailure() {
	cb.failures++
	cb.totalFailures++
	cb.lastFailureTime = time.Now()
	cb.successes = 0

	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.config.MaxFailures {
			cb.setState(StateOpen)
		}
	case StateHalfOpen:
		cb.setState(StateOpen)
	}
}

// onSuccess handles a successful execution
func (cb *CircuitBreaker) onSuccess() {
	cb.totalSuccesses++
	
	switch cb.state {
	case StateClosed:
		cb.failures = 0
	case StateHalfOpen:
		cb.successes++
		if cb.successes >= cb.config.SuccessThreshold {
			cb.setState(StateClosed)
		}
	}
}

// setState changes the circuit breaker state
func (cb *CircuitBreaker) setState(state State) {
	oldState := cb.state
	cb.state = state
	
	switch state {
	case StateOpen:
		cb.nextAttempt = time.Now().Add(cb.config.ResetTimeout)
		cb.logger.Warn("Circuit breaker opened",
			zap.String("name", cb.name),
			zap.Int("failures", cb.failures),
			zap.Time("nextAttempt", cb.nextAttempt))
	case StateClosed:
		cb.failures = 0
		cb.successes = 0
		cb.logger.Info("Circuit breaker closed",
			zap.String("name", cb.name))
	case StateHalfOpen:
		cb.successes = 0
		cb.logger.Info("Circuit breaker half-open",
			zap.String("name", cb.name))
	}
	
	if oldState != state {
		cb.logger.Info("Circuit breaker state changed",
			zap.String("name", cb.name),
			zap.String("from", oldState.String()),
			zap.String("to", state.String()))
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() State {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

// GetStats returns statistics about the circuit breaker
func (cb *CircuitBreaker) GetStats() map[string]interface{} {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	return map[string]interface{}{
		"name":              cb.name,
		"state":             cb.state.String(),
		"failures":          cb.failures,
		"successes":         cb.successes,
		"totalRequests":     cb.totalRequests,
		"totalFailures":     cb.totalFailures,
		"totalSuccesses":    cb.totalSuccesses,
		"totalTimeouts":     cb.totalTimeouts,
		"totalRejected":     cb.totalRejected,
		"lastFailureTime":   cb.lastFailureTime,
		"nextAttempt":       cb.nextAttempt,
		"config":            cb.config,
	}
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	
	cb.setState(StateClosed)
	cb.logger.Info("Circuit breaker manually reset", zap.String("name", cb.name))
}

// Manager manages multiple circuit breakers
type Manager struct {
	breakers map[string]*CircuitBreaker
	mutex    sync.RWMutex
	logger   *zap.Logger
	enabled  bool
}

// NewManager creates a new circuit breaker manager
func NewManager(logger *zap.Logger, enabled bool) *Manager {
	return &Manager{
		breakers: make(map[string]*CircuitBreaker),
		logger:   logger,
		enabled:  enabled,
	}
}

// GetOrCreate gets an existing circuit breaker or creates a new one
func (m *Manager) GetOrCreate(name string, config Config) *CircuitBreaker {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if breaker, exists := m.breakers[name]; exists {
		return breaker
	}

	breaker := NewCircuitBreaker(name, config, m.logger.Named("cb"))
	m.breakers[name] = breaker
	
	m.logger.Info("Created circuit breaker",
		zap.String("name", name),
		zap.Int("maxFailures", config.MaxFailures),
		zap.Duration("resetTimeout", config.ResetTimeout))
	
	return breaker
}

// Execute executes a function with circuit breaker protection
func (m *Manager) Execute(name string, config Config, ctx context.Context, executor ExecutorFunc) (interface{}, error) {
	if !m.enabled {
		return executor(ctx)
	}

	breaker := m.GetOrCreate(name, config)
	return breaker.Execute(ctx, executor)
}

// ExecuteWithFallback executes a function with circuit breaker protection and fallback
func (m *Manager) ExecuteWithFallback(name string, config Config, ctx context.Context, executor ExecutorFunc, fallback FallbackFunc) (interface{}, error) {
	if !m.enabled {
		return executor(ctx)
	}

	breaker := m.GetOrCreate(name, config)
	return breaker.ExecuteWithFallback(ctx, executor, fallback)
}

// GetBreaker returns a circuit breaker by name
func (m *Manager) GetBreaker(name string) (*CircuitBreaker, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	breaker, exists := m.breakers[name]
	return breaker, exists
}

// ListBreakers returns all circuit breaker names
func (m *Manager) ListBreakers() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	names := make([]string, 0, len(m.breakers))
	for name := range m.breakers {
		names = append(names, name)
	}
	return names
}

// GetAllStats returns statistics for all circuit breakers
func (m *Manager) GetAllStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := map[string]interface{}{
		"enabled":  m.enabled,
		"count":    len(m.breakers),
		"breakers": make(map[string]interface{}),
	}

	for name, breaker := range m.breakers {
		stats["breakers"].(map[string]interface{})[name] = breaker.GetStats()
	}

	return stats
}

// ResetAll resets all circuit breakers
func (m *Manager) ResetAll() {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, breaker := range m.breakers {
		breaker.Reset()
	}
	
	m.logger.Info("All circuit breakers reset")
}

// ResetBreaker resets a specific circuit breaker
func (m *Manager) ResetBreaker(name string) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	breaker, exists := m.breakers[name]
	if !exists {
		return fmt.Errorf("circuit breaker '%s' not found", name)
	}

	breaker.Reset()
	return nil
}

// IsEnabled returns whether circuit breaker management is enabled
func (m *Manager) IsEnabled() bool {
	return m.enabled
}

// SetEnabled enables or disables circuit breaker management
func (m *Manager) SetEnabled(enabled bool) {
	m.enabled = enabled
	m.logger.Info("Circuit breaker management enabled state changed", zap.Bool("enabled", enabled))
}