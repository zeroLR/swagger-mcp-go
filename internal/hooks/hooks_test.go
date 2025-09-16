package hooks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

// Test hook implementation
type testHook struct {
	name     string
	hookType HookType
	priority Priority
	executed bool
	err      error
}

func (h *testHook) Execute(ctx context.Context, hookCtx *HookContext) error {
	h.executed = true
	return h.err
}

func (h *testHook) Type() HookType {
	return h.hookType
}

func (h *testHook) Priority() Priority {
	return h.priority
}

func (h *testHook) Name() string {
	return h.name
}

func TestManager_RegisterHook(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger)

	hook1 := &testHook{
		name:     "test-hook-1",
		hookType: HookTypePreRequest,
		priority: PriorityHigh,
	}
	hook2 := &testHook{
		name:     "test-hook-2",
		hookType: HookTypePreRequest,
		priority: PriorityLow,
	}

	manager.RegisterHook(hook1)
	manager.RegisterHook(hook2)

	hooks := manager.GetRegisteredHooks()
	preRequestHooks := hooks[HookTypePreRequest]

	if len(preRequestHooks) != 2 {
		t.Errorf("Expected 2 hooks, got %d", len(preRequestHooks))
	}

	// Check that hooks are sorted by priority (highest first)
	if preRequestHooks[0].Priority() != PriorityHigh {
		t.Errorf("Expected first hook to have high priority, got %d", preRequestHooks[0].Priority())
	}
	if preRequestHooks[1].Priority() != PriorityLow {
		t.Errorf("Expected second hook to have low priority, got %d", preRequestHooks[1].Priority())
	}
}

func TestManager_ExecutePreRequestHooks(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger)

	hook1 := &testHook{
		name:     "test-hook-1",
		hookType: HookTypePreRequest,
		priority: PriorityHigh,
	}
	hook2 := &testHook{
		name:     "test-hook-2",
		hookType: HookTypePreRequest,
		priority: PriorityLow,
		err:      fmt.Errorf("test error"),
	}

	manager.RegisterHook(hook1)
	manager.RegisterHook(hook2)

	hookCtx := &HookContext{
		Request: &RequestContext{
			ServiceName: "test-service",
			OperationID: "test-operation",
			Method:      "GET",
			Path:        "/test",
			StartTime:   time.Now(),
		},
		Metadata: make(map[string]interface{}),
	}

	// Test successful execution
	err := manager.ExecutePreRequestHooks(context.Background(), hookCtx)
	if err == nil {
		t.Errorf("Expected error from failing hook")
	}

	if !hook1.executed {
		t.Errorf("Expected hook1 to be executed")
	}
	if !hook2.executed {
		t.Errorf("Expected hook2 to be executed despite error")
	}
}

func TestLoggingHook(t *testing.T) {
	logger := zap.NewNop()
	hook := NewLoggingHook(logger, PriorityMedium)

	if hook.Type() != HookTypePreRequest {
		t.Errorf("Expected pre-request hook type")
	}
	if hook.Priority() != PriorityMedium {
		t.Errorf("Expected medium priority")
	}
	if hook.Name() != "logging" {
		t.Errorf("Expected name 'logging', got %s", hook.Name())
	}

	// Test pre-request execution
	hookCtx := &HookContext{
		Request: &RequestContext{
			ServiceName: "test-service",
			OperationID: "test-operation",
			Method:      "GET",
			Path:        "/test",
			Parameters:  map[string]interface{}{"param1": "value1"},
			StartTime:   time.Now(),
		},
		Metadata: make(map[string]interface{}),
	}

	err := hook.Execute(context.Background(), hookCtx)
	if err != nil {
		t.Errorf("Logging hook should not return error: %v", err)
	}

	// Test post-response execution
	hookCtx.Response = &ResponseContext{
		StatusCode:   200,
		Headers:      map[string]string{"Content-Type": "application/json"},
		ResponseTime: 100 * time.Millisecond,
		UpstreamURL:  "https://api.example.com/test",
	}

	err = hook.Execute(context.Background(), hookCtx)
	if err != nil {
		t.Errorf("Logging hook should not return error: %v", err)
	}
}

func TestMetricsHook(t *testing.T) {
	logger := zap.NewNop()
	hook := NewMetricsHook(logger, PriorityLow)

	if hook.Type() != HookTypePostResponse {
		t.Errorf("Expected post-response hook type")
	}
	if hook.Priority() != PriorityLow {
		t.Errorf("Expected low priority")
	}
	if hook.Name() != "metrics" {
		t.Errorf("Expected name 'metrics', got %s", hook.Name())
	}

	hookCtx := &HookContext{
		Request: &RequestContext{
			ServiceName: "test-service",
			OperationID: "test-operation",
			Method:      "GET",
			Path:        "/test",
			StartTime:   time.Now(),
		},
		Response: &ResponseContext{
			StatusCode:   200,
			ResponseTime: 150 * time.Millisecond,
		},
		Metadata: make(map[string]interface{}),
	}

	err := hook.Execute(context.Background(), hookCtx)
	if err != nil {
		t.Errorf("Metrics hook should not return error: %v", err)
	}
}

func TestSecurityHeadersHook(t *testing.T) {
	customHeaders := map[string]string{
		"X-Custom-Header": "custom-value",
	}
	hook := NewSecurityHeadersHook(PriorityMedium, customHeaders)

	if hook.Type() != HookTypePostResponse {
		t.Errorf("Expected post-response hook type")
	}
	if hook.Name() != "security-headers" {
		t.Errorf("Expected name 'security-headers', got %s", hook.Name())
	}

	hookCtx := &HookContext{
		Request: &RequestContext{
			ServiceName: "test-service",
			StartTime:   time.Now(),
		},
		Response: &ResponseContext{
			StatusCode: 200,
			Headers:    make(map[string]string),
		},
		Metadata: make(map[string]interface{}),
	}

	err := hook.Execute(context.Background(), hookCtx)
	if err != nil {
		t.Errorf("Security headers hook should not return error: %v", err)
	}

	// Check that security headers were added
	expectedHeaders := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"X-XSS-Protection",
		"Strict-Transport-Security",
		"X-Custom-Header",
	}

	for _, header := range expectedHeaders {
		if _, exists := hookCtx.Response.Headers[header]; !exists {
			t.Errorf("Expected security header %s to be added", header)
		}
	}

	if hookCtx.Response.Headers["X-Custom-Header"] != "custom-value" {
		t.Errorf("Expected custom header value, got %s", hookCtx.Response.Headers["X-Custom-Header"])
	}
}

func TestRequestValidationHook(t *testing.T) {
	logger := zap.NewNop()
	hook := NewRequestValidationHook(logger, PriorityHigh)

	if hook.Type() != HookTypePreRequest {
		t.Errorf("Expected pre-request hook type")
	}
	if hook.Name() != "request-validation" {
		t.Errorf("Expected name 'request-validation', got %s", hook.Name())
	}

	// Test with valid parameters
	hookCtx := &HookContext{
		Request: &RequestContext{
			ServiceName: "test-service",
			OperationID: "test-operation",
			Parameters:  map[string]interface{}{"param1": "value1"},
			StartTime:   time.Now(),
		},
		Metadata: make(map[string]interface{}),
	}

	err := hook.Execute(context.Background(), hookCtx)
	if err != nil {
		t.Errorf("Request validation hook should not return error for valid request: %v", err)
	}

	// Test with missing parameters
	hookCtx.Request.Parameters = nil
	err = hook.Execute(context.Background(), hookCtx)
	if err == nil {
		t.Errorf("Request validation hook should return error for missing parameters")
	}
}

func TestErrorHandlingHook(t *testing.T) {
	logger := zap.NewNop()
	hook := NewErrorHandlingHook(logger, PriorityMedium)

	if hook.Type() != HookTypeOnError {
		t.Errorf("Expected on-error hook type")
	}
	if hook.Name() != "error-handling" {
		t.Errorf("Expected name 'error-handling', got %s", hook.Name())
	}

	hookCtx := &HookContext{
		Request: &RequestContext{
			ServiceName: "test-service",
			OperationID: "test-operation",
			Method:      "GET",
			Path:        "/test",
			StartTime:   time.Now(),
		},
		Response: &ResponseContext{
			StatusCode:   500,
			Error:        fmt.Errorf("test error"),
			ResponseTime: 100 * time.Millisecond,
		},
		Metadata: make(map[string]interface{}),
	}

	err := hook.Execute(context.Background(), hookCtx)
	if err != nil {
		t.Errorf("Error handling hook should not return error: %v", err)
	}
}

func TestContextHelper(t *testing.T) {
	helper := &ContextHelper{}

	// Create test HTTP request
	req := httptest.NewRequest("POST", "/api/test?param1=value1&param2=value2", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token123")

	params := map[string]interface{}{
		"param1": "value1",
		"body":   map[string]interface{}{"key": "value"},
	}

	// Test creating hook context
	hookCtx := helper.NewHookContext(req, "test-service", "test-operation", params)

	if hookCtx.Request.ServiceName != "test-service" {
		t.Errorf("Expected service name 'test-service', got %s", hookCtx.Request.ServiceName)
	}
	if hookCtx.Request.OperationID != "test-operation" {
		t.Errorf("Expected operation ID 'test-operation', got %s", hookCtx.Request.OperationID)
	}
	if hookCtx.Request.Method != "POST" {
		t.Errorf("Expected method 'POST', got %s", hookCtx.Request.Method)
	}
	if hookCtx.Request.Path != "/api/test" {
		t.Errorf("Expected path '/api/test', got %s", hookCtx.Request.Path)
	}

	// Check headers
	if hookCtx.Request.Headers["Content-Type"] != "application/json" {
		t.Errorf("Expected Content-Type header")
	}
	if hookCtx.Request.Headers["Authorization"] != "Bearer token123" {
		t.Errorf("Expected Authorization header")
	}

	// Check query parameters
	if len(hookCtx.Request.QueryParams["param1"]) != 1 || hookCtx.Request.QueryParams["param1"][0] != "value1" {
		t.Errorf("Expected query parameter param1=value1")
	}
	if len(hookCtx.Request.QueryParams["param2"]) != 1 || hookCtx.Request.QueryParams["param2"][0] != "value2" {
		t.Errorf("Expected query parameter param2=value2")
	}

	// Check parameters
	if len(hookCtx.Request.Parameters) != 2 {
		t.Errorf("Expected 2 parameters, got %d", len(hookCtx.Request.Parameters))
	}

	// Test adding response context
	responseHeaders := http.Header{}
	responseHeaders.Set("Content-Type", "application/json")
	responseHeaders.Set("X-Response-Time", "100ms")

	body := []byte(`{"result": "success"}`)
	helper.AddResponseContext(hookCtx, 200, responseHeaders, body, nil, "https://api.example.com/test")

	if hookCtx.Response == nil {
		t.Fatalf("Expected response context to be added")
	}
	if hookCtx.Response.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", hookCtx.Response.StatusCode)
	}
	if hookCtx.Response.Headers["Content-Type"] != "application/json" {
		t.Errorf("Expected Content-Type header in response")
	}
	if string(hookCtx.Response.Body) != `{"result": "success"}` {
		t.Errorf("Expected response body to match")
	}
	if hookCtx.Response.UpstreamURL != "https://api.example.com/test" {
		t.Errorf("Expected upstream URL to match")
	}
	if hookCtx.Response.ResponseTime == 0 {
		t.Errorf("Expected response time to be calculated")
	}
}