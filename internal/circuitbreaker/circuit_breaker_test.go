package circuitbreaker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestCircuitBreaker_ClosedState(t *testing.T) {
	config := Config{
		MaxFailures:      3,
		ResetTimeout:     time.Second,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
	}

	logger := zap.NewNop()
	cb := NewCircuitBreaker("test-cb", config, logger)

	// Test successful executions
	for i := 0; i < 5; i++ {
		result, err := cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
			return "success", nil
		})

		if err != nil {
			t.Errorf("Execution %d should succeed: %v", i+1, err)
		}
		if result != "success" {
			t.Errorf("Expected result 'success', got %v", result)
		}
		if cb.GetState() != StateClosed {
			t.Errorf("Circuit breaker should remain closed")
		}
	}

	stats := cb.GetStats()
	if stats["totalSuccesses"].(int64) != 5 {
		t.Errorf("Expected 5 successes, got %d", stats["totalSuccesses"].(int64))
	}
}

func TestCircuitBreaker_OpenState(t *testing.T) {
	config := Config{
		MaxFailures:      2,
		ResetTimeout:     time.Second,
		SuccessThreshold: 1,
		Timeout:          100 * time.Millisecond,
	}

	logger := zap.NewNop()
	cb := NewCircuitBreaker("test-cb", config, logger)

	// Cause failures to open the circuit
	for i := 0; i < 2; i++ {
		_, err := cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
			return nil, fmt.Errorf("failure %d", i+1)
		})

		if err == nil {
			t.Errorf("Execution %d should fail", i+1)
		}
	}

	if cb.GetState() != StateOpen {
		t.Errorf("Circuit breaker should be open after max failures")
	}

	// Next execution should be rejected immediately
	_, err := cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		return "should not execute", nil
	})

	if err == nil {
		t.Errorf("Execution should be rejected when circuit is open")
	}
	if cb.GetState() != StateOpen {
		t.Errorf("Circuit breaker should remain open")
	}

	stats := cb.GetStats()
	if stats["totalRejected"].(int64) != 1 {
		t.Errorf("Expected 1 rejection, got %d", stats["totalRejected"].(int64))
	}
}

func TestCircuitBreaker_HalfOpenState(t *testing.T) {
	config := Config{
		MaxFailures:      2,
		ResetTimeout:     100 * time.Millisecond,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
	}

	logger := zap.NewNop()
	cb := NewCircuitBreaker("test-cb", config, logger)

	// Open the circuit
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
			return nil, fmt.Errorf("failure")
		})
	}

	if cb.GetState() != StateOpen {
		t.Errorf("Circuit breaker should be open")
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// Next execution should transition to half-open
	_, err := cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		return "success", nil
	})

	if err != nil {
		t.Errorf("First execution after timeout should succeed: %v", err)
	}
	if cb.GetState() != StateHalfOpen {
		t.Errorf("Circuit breaker should be half-open, got %s", cb.GetState().String())
	}

	// Another successful execution should close the circuit
	_, err = cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		return "success", nil
	})

	if err != nil {
		t.Errorf("Second execution should succeed: %v", err)
	}
	if cb.GetState() != StateClosed {
		t.Errorf("Circuit breaker should be closed after success threshold")
	}
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	config := Config{
		MaxFailures:      2,
		ResetTimeout:     100 * time.Millisecond,
		SuccessThreshold: 1,
		Timeout:          100 * time.Millisecond,
	}

	logger := zap.NewNop()
	cb := NewCircuitBreaker("test-cb", config, logger)

	// Open the circuit
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
			return nil, fmt.Errorf("failure")
		})
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// Execution that fails in half-open should reopen the circuit
	_, err := cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		return nil, fmt.Errorf("failure in half-open")
	})

	if err == nil {
		t.Errorf("Execution should fail")
	}
	if cb.GetState() != StateOpen {
		t.Errorf("Circuit breaker should be open again after half-open failure")
	}
}

func TestCircuitBreaker_Timeout(t *testing.T) {
	config := Config{
		MaxFailures:      3,
		ResetTimeout:     time.Second,
		SuccessThreshold: 1,
		Timeout:          50 * time.Millisecond,
	}

	logger := zap.NewNop()
	cb := NewCircuitBreaker("test-cb", config, logger)

	// Execute function that takes longer than timeout
	_, err := cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		time.Sleep(100 * time.Millisecond)
		return "too late", nil
	})

	if err == nil {
		t.Errorf("Execution should timeout")
	}

	stats := cb.GetStats()
	if stats["totalTimeouts"].(int64) != 1 {
		t.Errorf("Expected 1 timeout, got %d", stats["totalTimeouts"].(int64))
	}
}

func TestCircuitBreaker_WithFallback(t *testing.T) {
	config := Config{
		MaxFailures:      1,
		ResetTimeout:     time.Second,
		SuccessThreshold: 1,
		Timeout:          100 * time.Millisecond,
	}

	logger := zap.NewNop()
	cb := NewCircuitBreaker("test-cb", config, logger)

	fallback := func(ctx context.Context, err error) (interface{}, error) {
		return "fallback result", nil
	}

	// Open the circuit
	cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		return nil, fmt.Errorf("failure")
	})

	if cb.GetState() != StateOpen {
		t.Errorf("Circuit breaker should be open")
	}

	// Execute with fallback
	result, err := cb.ExecuteWithFallback(context.Background(), func(ctx context.Context) (interface{}, error) {
		return "should not execute", nil
	}, fallback)

	if err != nil {
		t.Errorf("Execution with fallback should not error: %v", err)
	}
	if result != "fallback result" {
		t.Errorf("Expected fallback result, got %v", result)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := Config{
		MaxFailures:      1,
		ResetTimeout:     time.Hour, // Long timeout
		SuccessThreshold: 1,
		Timeout:          100 * time.Millisecond,
	}

	logger := zap.NewNop()
	cb := NewCircuitBreaker("test-cb", config, logger)

	// Open the circuit
	cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		return nil, fmt.Errorf("failure")
	})

	if cb.GetState() != StateOpen {
		t.Errorf("Circuit breaker should be open")
	}

	// Manually reset
	cb.Reset()

	if cb.GetState() != StateClosed {
		t.Errorf("Circuit breaker should be closed after reset")
	}

	// Should allow execution now
	result, err := cb.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		return "success after reset", nil
	})

	if err != nil {
		t.Errorf("Execution should succeed after reset: %v", err)
	}
	if result != "success after reset" {
		t.Errorf("Expected success result, got %v", result)
	}
}

func TestManager(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger, true)

	config := Config{
		MaxFailures:      2,
		ResetTimeout:     100 * time.Millisecond,
		SuccessThreshold: 1,
		Timeout:          50 * time.Millisecond,
	}

	// Test Execute
	result, err := manager.Execute("test-service", config, context.Background(), func(ctx context.Context) (interface{}, error) {
		return "success", nil
	})

	if err != nil {
		t.Errorf("Execution should succeed: %v", err)
	}
	if result != "success" {
		t.Errorf("Expected success result, got %v", result)
	}

	// Check that circuit breaker was created
	cb, exists := manager.GetBreaker("test-service")
	if !exists {
		t.Errorf("Circuit breaker should be created")
	}
	if cb.GetState() != StateClosed {
		t.Errorf("Circuit breaker should be closed")
	}

	// Test with fallback
	fallback := func(ctx context.Context, err error) (interface{}, error) {
		return "fallback", nil
	}

	result, err = manager.ExecuteWithFallback("test-service-2", config, context.Background(), func(ctx context.Context) (interface{}, error) {
		return "success", nil
	}, fallback)

	if err != nil {
		t.Errorf("Execution with fallback should succeed: %v", err)
	}
	if result != "success" {
		t.Errorf("Expected success result, got %v", result)
	}
}

func TestManager_Stats(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger, true)

	config := Config{
		MaxFailures:      2,
		ResetTimeout:     time.Second,
		SuccessThreshold: 1,
		Timeout:          100 * time.Millisecond,
	}

	// Execute to create circuit breakers
	manager.Execute("service1", config, context.Background(), func(ctx context.Context) (interface{}, error) {
		return "success", nil
	})
	manager.Execute("service2", config, context.Background(), func(ctx context.Context) (interface{}, error) {
		return nil, fmt.Errorf("failure")
	})

	stats := manager.GetAllStats()
	if !stats["enabled"].(bool) {
		t.Errorf("Expected enabled to be true")
	}
	if stats["count"].(int) != 2 {
		t.Errorf("Expected 2 circuit breakers, got %d", stats["count"].(int))
	}

	breakers := stats["breakers"].(map[string]interface{})
	if len(breakers) != 2 {
		t.Errorf("Expected 2 breakers in stats, got %d", len(breakers))
	}

	// Test list breakers
	names := manager.ListBreakers()
	if len(names) != 2 {
		t.Errorf("Expected 2 breaker names, got %d", len(names))
	}

	// Test reset specific breaker
	err := manager.ResetBreaker("service1")
	if err != nil {
		t.Errorf("Reset should succeed: %v", err)
	}

	err = manager.ResetBreaker("nonexistent")
	if err == nil {
		t.Errorf("Reset should fail for nonexistent breaker")
	}

	// Test reset all
	manager.ResetAll()
}

func TestManager_Disabled(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger, false)

	config := Config{
		MaxFailures:      1,
		ResetTimeout:     time.Second,
		SuccessThreshold: 1,
		Timeout:          100 * time.Millisecond,
	}

	// When disabled, should execute directly without circuit breaker
	executorCalled := false
	result, err := manager.Execute("test-service", config, context.Background(), func(ctx context.Context) (interface{}, error) {
		executorCalled = true
		return "direct execution", nil
	})

	if err != nil {
		t.Errorf("Execution should succeed: %v", err)
	}
	if result != "direct execution" {
		t.Errorf("Expected direct execution result, got %v", result)
	}
	if !executorCalled {
		t.Errorf("Executor should be called directly when disabled")
	}

	// No circuit breaker should be created
	_, exists := manager.GetBreaker("test-service")
	if exists {
		t.Errorf("No circuit breaker should be created when disabled")
	}
}
