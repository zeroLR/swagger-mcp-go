package hooks

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// HookType represents the type of hook
type HookType string

const (
	HookTypePreRequest   HookType = "pre-request"
	HookTypePostResponse HookType = "post-response"
	HookTypeOnError      HookType = "on-error"
)

// Priority defines the execution priority of hooks
type Priority int

const (
	PriorityHigh   Priority = 100
	PriorityMedium Priority = 50
	PriorityLow    Priority = 10
)

// RequestContext contains information about the incoming request
type RequestContext struct {
	ServiceName string                 `json:"serviceName"`
	OperationID string                 `json:"operationId"`
	Method      string                 `json:"method"`
	Path        string                 `json:"path"`
	Headers     map[string]string      `json:"headers"`
	QueryParams map[string][]string    `json:"queryParams"`
	Body        []byte                 `json:"body,omitempty"`
	Parameters  map[string]interface{} `json:"parameters"`
	StartTime   time.Time              `json:"startTime"`
}

// ResponseContext contains information about the response
type ResponseContext struct {
	StatusCode    int               `json:"statusCode"`
	Headers       map[string]string `json:"headers"`
	Body          []byte            `json:"body,omitempty"`
	ResponseTime  time.Duration     `json:"responseTime"`
	Error         error             `json:"error,omitempty"`
	UpstreamURL   string            `json:"upstreamUrl"`
}

// HookContext contains both request and response context for hooks
type HookContext struct {
	Request  *RequestContext  `json:"request"`
	Response *ResponseContext `json:"response,omitempty"`
	Metadata map[string]interface{} `json:"metadata"`
}

// Hook represents a function that can modify requests or responses
type Hook interface {
	// Execute runs the hook with the given context
	Execute(ctx context.Context, hookCtx *HookContext) error
	// Type returns the hook type
	Type() HookType
	// Priority returns the execution priority
	Priority() Priority
	// Name returns a unique name for the hook
	Name() string
}

// Manager manages request/response hooks
type Manager struct {
	hooks  map[HookType][]Hook
	logger *zap.Logger
}

// NewManager creates a new hook manager
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		hooks: map[HookType][]Hook{
			HookTypePreRequest:   make([]Hook, 0),
			HookTypePostResponse: make([]Hook, 0),
			HookTypeOnError:      make([]Hook, 0),
		},
		logger: logger,
	}
}

// RegisterHook registers a hook with the manager
func (m *Manager) RegisterHook(hook Hook) {
	hookType := hook.Type()
	m.hooks[hookType] = append(m.hooks[hookType], hook)
	
	// Sort hooks by priority (highest first)
	m.sortHooksByPriority(hookType)
	
	m.logger.Info("Registered hook",
		zap.String("name", hook.Name()),
		zap.String("type", string(hookType)),
		zap.Int("priority", int(hook.Priority())))
}

// ExecutePreRequestHooks executes all pre-request hooks
func (m *Manager) ExecutePreRequestHooks(ctx context.Context, hookCtx *HookContext) error {
	return m.executeHooks(ctx, HookTypePreRequest, hookCtx)
}

// ExecutePostResponseHooks executes all post-response hooks
func (m *Manager) ExecutePostResponseHooks(ctx context.Context, hookCtx *HookContext) error {
	return m.executeHooks(ctx, HookTypePostResponse, hookCtx)
}

// ExecuteErrorHooks executes all error hooks
func (m *Manager) ExecuteErrorHooks(ctx context.Context, hookCtx *HookContext) error {
	return m.executeHooks(ctx, HookTypeOnError, hookCtx)
}

// executeHooks executes all hooks of a given type
func (m *Manager) executeHooks(ctx context.Context, hookType HookType, hookCtx *HookContext) error {
	hooks := m.hooks[hookType]
	
	for _, hook := range hooks {
		start := time.Now()
		err := hook.Execute(ctx, hookCtx)
		duration := time.Since(start)
		
		if err != nil {
			m.logger.Error("Hook execution failed",
				zap.String("hook", hook.Name()),
				zap.String("type", string(hookType)),
				zap.Duration("duration", duration),
				zap.Error(err))
			return fmt.Errorf("hook %s failed: %w", hook.Name(), err)
		}
		
		m.logger.Debug("Hook executed successfully",
			zap.String("hook", hook.Name()),
			zap.String("type", string(hookType)),
			zap.Duration("duration", duration))
	}
	
	return nil
}

// sortHooksByPriority sorts hooks by priority (highest first)
func (m *Manager) sortHooksByPriority(hookType HookType) {
	hooks := m.hooks[hookType]
	
	// Simple bubble sort by priority
	for i := 0; i < len(hooks)-1; i++ {
		for j := 0; j < len(hooks)-i-1; j++ {
			if hooks[j].Priority() < hooks[j+1].Priority() {
				hooks[j], hooks[j+1] = hooks[j+1], hooks[j]
			}
		}
	}
}

// GetRegisteredHooks returns all registered hooks
func (m *Manager) GetRegisteredHooks() map[HookType][]Hook {
	result := make(map[HookType][]Hook)
	for hookType, hooks := range m.hooks {
		result[hookType] = make([]Hook, len(hooks))
		copy(result[hookType], hooks)
	}
	return result
}

// Built-in hooks

// LoggingHook logs request and response information
type LoggingHook struct {
	logger   *zap.Logger
	priority Priority
}

// NewLoggingHook creates a new logging hook
func NewLoggingHook(logger *zap.Logger, priority Priority) *LoggingHook {
	return &LoggingHook{
		logger:   logger,
		priority: priority,
	}
}

func (h *LoggingHook) Execute(ctx context.Context, hookCtx *HookContext) error {
	if hookCtx.Response != nil {
		// Post-response logging
		h.logger.Info("Request completed",
			zap.String("service", hookCtx.Request.ServiceName),
			zap.String("operation", hookCtx.Request.OperationID),
			zap.String("method", hookCtx.Request.Method),
			zap.String("path", hookCtx.Request.Path),
			zap.Int("statusCode", hookCtx.Response.StatusCode),
			zap.Duration("responseTime", hookCtx.Response.ResponseTime),
			zap.String("upstreamUrl", hookCtx.Response.UpstreamURL))
	} else {
		// Pre-request logging
		h.logger.Info("Processing request",
			zap.String("service", hookCtx.Request.ServiceName),
			zap.String("operation", hookCtx.Request.OperationID),
			zap.String("method", hookCtx.Request.Method),
			zap.String("path", hookCtx.Request.Path),
			zap.Int("paramCount", len(hookCtx.Request.Parameters)))
	}
	return nil
}

func (h *LoggingHook) Type() HookType {
	return HookTypePreRequest // This hook can be registered for multiple types
}

func (h *LoggingHook) Priority() Priority {
	return h.priority
}

func (h *LoggingHook) Name() string {
	return "logging"
}

// MetricsHook collects metrics about requests and responses
type MetricsHook struct {
	priority Priority
	logger   *zap.Logger
}

// NewMetricsHook creates a new metrics hook
func NewMetricsHook(logger *zap.Logger, priority Priority) *MetricsHook {
	return &MetricsHook{
		priority: priority,
		logger:   logger,
	}
}

func (h *MetricsHook) Execute(ctx context.Context, hookCtx *HookContext) error {
	if hookCtx.Response != nil {
		// Collect response metrics
		h.logger.Debug("Collecting response metrics",
			zap.String("service", hookCtx.Request.ServiceName),
			zap.String("operation", hookCtx.Request.OperationID),
			zap.Int("statusCode", hookCtx.Response.StatusCode),
			zap.Duration("responseTime", hookCtx.Response.ResponseTime))
		
		// TODO: Integrate with Prometheus metrics
		// This would increment counters, update histograms, etc.
	}
	return nil
}

func (h *MetricsHook) Type() HookType {
	return HookTypePostResponse
}

func (h *MetricsHook) Priority() Priority {
	return h.priority
}

func (h *MetricsHook) Name() string {
	return "metrics"
}

// SecurityHeadersHook adds security headers to responses
type SecurityHeadersHook struct {
	priority Priority
	headers  map[string]string
}

// NewSecurityHeadersHook creates a new security headers hook
func NewSecurityHeadersHook(priority Priority, headers map[string]string) *SecurityHeadersHook {
	defaultHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "1; mode=block",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
	}
	
	// Merge custom headers with defaults
	for k, v := range headers {
		defaultHeaders[k] = v
	}
	
	return &SecurityHeadersHook{
		priority: priority,
		headers:  defaultHeaders,
	}
}

func (h *SecurityHeadersHook) Execute(ctx context.Context, hookCtx *HookContext) error {
	if hookCtx.Response != nil {
		// Add security headers to response
		for key, value := range h.headers {
			hookCtx.Response.Headers[key] = value
		}
	}
	return nil
}

func (h *SecurityHeadersHook) Type() HookType {
	return HookTypePostResponse
}

func (h *SecurityHeadersHook) Priority() Priority {
	return h.priority
}

func (h *SecurityHeadersHook) Name() string {
	return "security-headers"
}

// RequestValidationHook validates request parameters and body
type RequestValidationHook struct {
	priority Priority
	logger   *zap.Logger
}

// NewRequestValidationHook creates a new request validation hook
func NewRequestValidationHook(logger *zap.Logger, priority Priority) *RequestValidationHook {
	return &RequestValidationHook{
		priority: priority,
		logger:   logger,
	}
}

func (h *RequestValidationHook) Execute(ctx context.Context, hookCtx *HookContext) error {
	// Validate request parameters
	if hookCtx.Request.Parameters == nil {
		return fmt.Errorf("missing request parameters")
	}
	
	// Basic validation - check for required parameters
	// TODO: Implement proper OpenAPI schema validation
	h.logger.Debug("Validating request",
		zap.String("service", hookCtx.Request.ServiceName),
		zap.String("operation", hookCtx.Request.OperationID),
		zap.Int("paramCount", len(hookCtx.Request.Parameters)))
	
	return nil
}

func (h *RequestValidationHook) Type() HookType {
	return HookTypePreRequest
}

func (h *RequestValidationHook) Priority() Priority {
	return h.priority
}

func (h *RequestValidationHook) Name() string {
	return "request-validation"
}

// ErrorHandlingHook handles and formats errors
type ErrorHandlingHook struct {
	priority Priority
	logger   *zap.Logger
}

// NewErrorHandlingHook creates a new error handling hook
func NewErrorHandlingHook(logger *zap.Logger, priority Priority) *ErrorHandlingHook {
	return &ErrorHandlingHook{
		priority: priority,
		logger:   logger,
	}
}

func (h *ErrorHandlingHook) Execute(ctx context.Context, hookCtx *HookContext) error {
	if hookCtx.Response != nil && hookCtx.Response.Error != nil {
		h.logger.Error("Request failed",
			zap.String("service", hookCtx.Request.ServiceName),
			zap.String("operation", hookCtx.Request.OperationID),
			zap.String("method", hookCtx.Request.Method),
			zap.String("path", hookCtx.Request.Path),
			zap.Error(hookCtx.Response.Error),
			zap.Duration("responseTime", hookCtx.Response.ResponseTime))
		
		// TODO: Transform error response format
		// This could standardize error responses across services
	}
	return nil
}

func (h *ErrorHandlingHook) Type() HookType {
	return HookTypeOnError
}

func (h *ErrorHandlingHook) Priority() Priority {
	return h.priority
}

func (h *ErrorHandlingHook) Name() string {
	return "error-handling"
}

// ContextHelper provides helper functions for working with hook contexts
type ContextHelper struct{}

// NewHookContext creates a new hook context from HTTP request
func (h *ContextHelper) NewHookContext(req *http.Request, serviceName, operationID string, params map[string]interface{}) *HookContext {
	// Extract headers
	headers := make(map[string]string)
	for key, values := range req.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	
	// Extract query parameters
	queryParams := make(map[string][]string)
	for key, values := range req.URL.Query() {
		queryParams[key] = values
	}
	
	return &HookContext{
		Request: &RequestContext{
			ServiceName: serviceName,
			OperationID: operationID,
			Method:      req.Method,
			Path:        req.URL.Path,
			Headers:     headers,
			QueryParams: queryParams,
			Parameters:  params,
			StartTime:   time.Now(),
		},
		Metadata: make(map[string]interface{}),
	}
}

// AddResponseContext adds response information to the hook context
func (h *ContextHelper) AddResponseContext(hookCtx *HookContext, statusCode int, responseHeaders http.Header, body []byte, err error, upstreamURL string) {
	headers := make(map[string]string)
	for key, values := range responseHeaders {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	
	hookCtx.Response = &ResponseContext{
		StatusCode:   statusCode,
		Headers:      headers,
		Body:         body,
		ResponseTime: time.Since(hookCtx.Request.StartTime),
		Error:        err,
		UpstreamURL:  upstreamURL,
	}
}