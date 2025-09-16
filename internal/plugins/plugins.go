package plugins

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
	"github.com/zeroLR/swagger-mcp-go/internal/hooks"
)

// PluginType represents the type of plugin
type PluginType string

const (
	PluginTypeAuth        PluginType = "auth"
	PluginTypeTransform   PluginType = "transform"
	PluginTypeValidation  PluginType = "validation"
	PluginTypeMiddleware  PluginType = "middleware"
	PluginTypeProcessor   PluginType = "processor"
	PluginTypeIntegration PluginType = "integration"
)

// Plugin represents a plugin interface
type Plugin interface {
	// Name returns the plugin name
	Name() string
	// Type returns the plugin type
	Type() PluginType
	// Version returns the plugin version
	Version() string
	// Description returns the plugin description
	Description() string
	// Initialize initializes the plugin with configuration
	Initialize(config map[string]interface{}) error
	// Start starts the plugin (called when the application starts)
	Start(ctx context.Context) error
	// Stop stops the plugin (called when the application stops)
	Stop() error
	// Health returns the plugin health status
	Health() HealthStatus
}

// HealthStatus represents plugin health status
type HealthStatus struct {
	Healthy bool   `json:"healthy"`
	Message string `json:"message"`
}

// AuthPlugin extends Plugin for authentication plugins
type AuthPlugin interface {
	Plugin
	// Authenticate validates credentials
	Authenticate(ctx context.Context, request *http.Request) (*AuthResult, error)
}

// AuthResult represents authentication result
type AuthResult struct {
	Authenticated bool                   `json:"authenticated"`
	UserID        string                 `json:"userId"`
	Username      string                 `json:"username"`
	Roles         []string               `json:"roles"`
	Attributes    map[string]interface{} `json:"attributes"`
}

// TransformPlugin extends Plugin for request/response transformation
type TransformPlugin interface {
	Plugin
	// TransformRequest transforms an incoming request
	TransformRequest(ctx context.Context, req *TransformRequest) (*TransformRequest, error)
	// TransformResponse transforms an outgoing response
	TransformResponse(ctx context.Context, resp *TransformResponse) (*TransformResponse, error)
}

// TransformRequest represents a request to be transformed
type TransformRequest struct {
	Method      string                 `json:"method"`
	URL         string                 `json:"url"`
	Headers     map[string]string      `json:"headers"`
	Body        []byte                 `json:"body,omitempty"`
	Parameters  map[string]interface{} `json:"parameters"`
	ServiceName string                 `json:"serviceName"`
}

// TransformResponse represents a response to be transformed
type TransformResponse struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body,omitempty"`
}

// ValidationPlugin extends Plugin for request validation
type ValidationPlugin interface {
	Plugin
	// ValidateRequest validates an incoming request
	ValidateRequest(ctx context.Context, req *ValidationRequest) error
}

// ValidationRequest represents a request to be validated
type ValidationRequest struct {
	Method      string                 `json:"method"`
	Path        string                 `json:"path"`
	Headers     map[string]string      `json:"headers"`
	Parameters  map[string]interface{} `json:"parameters"`
	Body        []byte                 `json:"body,omitempty"`
	ServiceName string                 `json:"serviceName"`
	OperationID string                 `json:"operationId"`
}

// MiddlewarePlugin extends Plugin for HTTP middleware
type MiddlewarePlugin interface {
	Plugin
	// Middleware returns an HTTP middleware function
	Middleware() func(http.Handler) http.Handler
}

// ProcessorPlugin extends Plugin for custom processing logic
type ProcessorPlugin interface {
	Plugin
	// Process executes custom processing logic
	Process(ctx context.Context, input *ProcessorInput) (*ProcessorOutput, error)
}

// ProcessorInput represents input for processor plugins
type ProcessorInput struct {
	Type    string                 `json:"type"`
	Data    map[string]interface{} `json:"data"`
	Context map[string]interface{} `json:"context"`
}

// ProcessorOutput represents output from processor plugins
type ProcessorOutput struct {
	Result map[string]interface{} `json:"result"`
	Events []Event                `json:"events,omitempty"`
}

// Event represents an event emitted by a plugin
type Event struct {
	Type      string                 `json:"type"`
	Source    string                 `json:"source"`
	Data      map[string]interface{} `json:"data"`
	Timestamp int64                  `json:"timestamp"`
}

// IntegrationPlugin extends Plugin for third-party integrations
type IntegrationPlugin interface {
	Plugin
	// Connect establishes connection to external service
	Connect(ctx context.Context) error
	// Disconnect closes connection to external service
	Disconnect() error
	// Send sends data to external service
	Send(ctx context.Context, data interface{}) error
	// Receive receives data from external service
	Receive(ctx context.Context) (interface{}, error)
}

// Registry manages plugins
type Registry struct {
	plugins     map[string]Plugin
	pluginsByType map[PluginType][]Plugin
	mutex       sync.RWMutex
	logger      *zap.Logger
	hookManager *hooks.Manager
}

// NewRegistry creates a new plugin registry
func NewRegistry(logger *zap.Logger, hookManager *hooks.Manager) *Registry {
	return &Registry{
		plugins:       make(map[string]Plugin),
		pluginsByType: make(map[PluginType][]Plugin),
		logger:        logger,
		hookManager:   hookManager,
	}
}

// Register registers a plugin
func (r *Registry) Register(plugin Plugin) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	name := plugin.Name()
	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin with name '%s' already registered", name)
	}

	r.plugins[name] = plugin
	pluginType := plugin.Type()
	r.pluginsByType[pluginType] = append(r.pluginsByType[pluginType], plugin)

	r.logger.Info("Registered plugin",
		zap.String("name", name),
		zap.String("type", string(pluginType)),
		zap.String("version", plugin.Version()))

	// Register hooks if plugin supports them
	r.registerPluginHooks(plugin)

	return nil
}

// registerPluginHooks registers hooks for supported plugin types
func (r *Registry) registerPluginHooks(plugin Plugin) {
	switch p := plugin.(type) {
	case ValidationPlugin:
		hook := &validationPluginHook{plugin: p, logger: r.logger}
		r.hookManager.RegisterHook(hook)
	case TransformPlugin:
		hook := &transformPluginHook{plugin: p, logger: r.logger}
		r.hookManager.RegisterHook(hook)
	}
}

// Get retrieves a plugin by name
func (r *Registry) Get(name string) (Plugin, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	plugin, exists := r.plugins[name]
	return plugin, exists
}

// GetByType retrieves all plugins of a specific type
func (r *Registry) GetByType(pluginType PluginType) []Plugin {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	plugins := make([]Plugin, len(r.pluginsByType[pluginType]))
	copy(plugins, r.pluginsByType[pluginType])
	return plugins
}

// List returns all registered plugins
func (r *Registry) List() []Plugin {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	plugins := make([]Plugin, 0, len(r.plugins))
	for _, plugin := range r.plugins {
		plugins = append(plugins, plugin)
	}
	return plugins
}

// Initialize initializes all plugins
func (r *Registry) Initialize(configs map[string]map[string]interface{}) error {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for name, plugin := range r.plugins {
		config := configs[name]
		if config == nil {
			config = make(map[string]interface{})
		}

		if err := plugin.Initialize(config); err != nil {
			return fmt.Errorf("failed to initialize plugin '%s': %w", name, err)
		}

		r.logger.Info("Initialized plugin", zap.String("name", name))
	}

	return nil
}

// Start starts all plugins
func (r *Registry) Start(ctx context.Context) error {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for name, plugin := range r.plugins {
		if err := plugin.Start(ctx); err != nil {
			return fmt.Errorf("failed to start plugin '%s': %w", name, err)
		}

		r.logger.Info("Started plugin", zap.String("name", name))
	}

	return nil
}

// Stop stops all plugins
func (r *Registry) Stop() error {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var errors []error
	for name, plugin := range r.plugins {
		if err := plugin.Stop(); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop plugin '%s': %w", name, err))
		} else {
			r.logger.Info("Stopped plugin", zap.String("name", name))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors stopping plugins: %v", errors)
	}

	return nil
}

// Health returns health status of all plugins
func (r *Registry) Health() map[string]HealthStatus {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	health := make(map[string]HealthStatus)
	for name, plugin := range r.plugins {
		health[name] = plugin.Health()
	}

	return health
}

// Hook implementations for plugin integration

// validationPluginHook integrates validation plugins with the hook system
type validationPluginHook struct {
	plugin ValidationPlugin
	logger *zap.Logger
}

func (h *validationPluginHook) Execute(ctx context.Context, hookCtx *hooks.HookContext) error {
	if hookCtx.Request == nil {
		return nil
	}

	validationReq := &ValidationRequest{
		Method:      hookCtx.Request.Method,
		Path:        hookCtx.Request.Path,
		Headers:     hookCtx.Request.Headers,
		Parameters:  hookCtx.Request.Parameters,
		ServiceName: hookCtx.Request.ServiceName,
		OperationID: hookCtx.Request.OperationID,
	}

	return h.plugin.ValidateRequest(ctx, validationReq)
}

func (h *validationPluginHook) Type() hooks.HookType {
	return hooks.HookTypePreRequest
}

func (h *validationPluginHook) Priority() hooks.Priority {
	return hooks.PriorityHigh
}

func (h *validationPluginHook) Name() string {
	return fmt.Sprintf("validation-plugin-%s", h.plugin.Name())
}

// transformPluginHook integrates transform plugins with the hook system
type transformPluginHook struct {
	plugin TransformPlugin
	logger *zap.Logger
}

func (h *transformPluginHook) Execute(ctx context.Context, hookCtx *hooks.HookContext) error {
	if hookCtx.Response != nil {
		// Post-response transformation
		transformResp := &TransformResponse{
			StatusCode: hookCtx.Response.StatusCode,
			Headers:    hookCtx.Response.Headers,
			Body:       hookCtx.Response.Body,
		}

		transformed, err := h.plugin.TransformResponse(ctx, transformResp)
		if err != nil {
			return err
		}

		// Update response
		hookCtx.Response.StatusCode = transformed.StatusCode
		hookCtx.Response.Headers = transformed.Headers
		hookCtx.Response.Body = transformed.Body
	} else if hookCtx.Request != nil {
		// Pre-request transformation
		transformReq := &TransformRequest{
			Method:      hookCtx.Request.Method,
			URL:         hookCtx.Request.Path,
			Headers:     hookCtx.Request.Headers,
			Parameters:  hookCtx.Request.Parameters,
			ServiceName: hookCtx.Request.ServiceName,
		}

		transformed, err := h.plugin.TransformRequest(ctx, transformReq)
		if err != nil {
			return err
		}

		// Update request
		hookCtx.Request.Method = transformed.Method
		hookCtx.Request.Path = transformed.URL
		hookCtx.Request.Headers = transformed.Headers
		hookCtx.Request.Parameters = transformed.Parameters
	}

	return nil
}

func (h *transformPluginHook) Type() hooks.HookType {
	return hooks.HookTypePreRequest // This hook can be registered for both types
}

func (h *transformPluginHook) Priority() hooks.Priority {
	return hooks.PriorityMedium
}

func (h *transformPluginHook) Name() string {
	return fmt.Sprintf("transform-plugin-%s", h.plugin.Name())
}

// Built-in example plugins

// ExampleAuthPlugin demonstrates an authentication plugin
type ExampleAuthPlugin struct {
	name        string
	version     string
	description string
	config      map[string]interface{}
	users       map[string]string
	logger      *zap.Logger
}

// NewExampleAuthPlugin creates a new example auth plugin
func NewExampleAuthPlugin(logger *zap.Logger) *ExampleAuthPlugin {
	return &ExampleAuthPlugin{
		name:        "example-auth",
		version:     "1.0.0",
		description: "Example authentication plugin with static users",
		users:       make(map[string]string),
		logger:      logger.Named("example-auth-plugin"),
	}
}

func (p *ExampleAuthPlugin) Name() string        { return p.name }
func (p *ExampleAuthPlugin) Type() PluginType    { return PluginTypeAuth }
func (p *ExampleAuthPlugin) Version() string     { return p.version }
func (p *ExampleAuthPlugin) Description() string { return p.description }

func (p *ExampleAuthPlugin) Initialize(config map[string]interface{}) error {
	p.config = config
	
	// Load users from config
	if users, ok := config["users"].(map[string]interface{}); ok {
		for username, password := range users {
			if passwordStr, ok := password.(string); ok {
				p.users[username] = passwordStr
			}
		}
	}
	
	p.logger.Info("Initialized with users", zap.Int("userCount", len(p.users)))
	return nil
}

func (p *ExampleAuthPlugin) Start(ctx context.Context) error {
	p.logger.Info("Started example auth plugin")
	return nil
}

func (p *ExampleAuthPlugin) Stop() error {
	p.logger.Info("Stopped example auth plugin")
	return nil
}

func (p *ExampleAuthPlugin) Health() HealthStatus {
	return HealthStatus{
		Healthy: true,
		Message: "Example auth plugin is healthy",
	}
}

func (p *ExampleAuthPlugin) Authenticate(ctx context.Context, request *http.Request) (*AuthResult, error) {
	username, password, ok := request.BasicAuth()
	if !ok {
		return &AuthResult{Authenticated: false}, nil
	}

	if storedPassword, exists := p.users[username]; exists && storedPassword == password {
		return &AuthResult{
			Authenticated: true,
			UserID:        username,
			Username:      username,
			Roles:         []string{"user"},
			Attributes:    map[string]interface{}{"source": "example-auth-plugin"},
		}, nil
	}

	return &AuthResult{Authenticated: false}, nil
}

// ExampleTransformPlugin demonstrates a transformation plugin
type ExampleTransformPlugin struct {
	name        string
	version     string
	description string
	config      map[string]interface{}
	logger      *zap.Logger
}

// NewExampleTransformPlugin creates a new example transform plugin
func NewExampleTransformPlugin(logger *zap.Logger) *ExampleTransformPlugin {
	return &ExampleTransformPlugin{
		name:        "example-transform",
		version:     "1.0.0",
		description: "Example transformation plugin that adds headers",
		logger:      logger.Named("example-transform-plugin"),
	}
}

func (p *ExampleTransformPlugin) Name() string        { return p.name }
func (p *ExampleTransformPlugin) Type() PluginType    { return PluginTypeTransform }
func (p *ExampleTransformPlugin) Version() string     { return p.version }
func (p *ExampleTransformPlugin) Description() string { return p.description }

func (p *ExampleTransformPlugin) Initialize(config map[string]interface{}) error {
	p.config = config
	p.logger.Info("Initialized example transform plugin")
	return nil
}

func (p *ExampleTransformPlugin) Start(ctx context.Context) error {
	p.logger.Info("Started example transform plugin")
	return nil
}

func (p *ExampleTransformPlugin) Stop() error {
	p.logger.Info("Stopped example transform plugin")
	return nil
}

func (p *ExampleTransformPlugin) Health() HealthStatus {
	return HealthStatus{
		Healthy: true,
		Message: "Example transform plugin is healthy",
	}
}

func (p *ExampleTransformPlugin) TransformRequest(ctx context.Context, req *TransformRequest) (*TransformRequest, error) {
	// Add custom header to request
	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	req.Headers["X-Transform-Plugin"] = "example-transform"
	req.Headers["X-Request-ID"] = fmt.Sprintf("req-%d", time.Now().UnixNano())
	
	p.logger.Debug("Transformed request", zap.String("service", req.ServiceName))
	return req, nil
}

func (p *ExampleTransformPlugin) TransformResponse(ctx context.Context, resp *TransformResponse) (*TransformResponse, error) {
	// Add custom header to response
	if resp.Headers == nil {
		resp.Headers = make(map[string]string)
	}
	resp.Headers["X-Response-Transformed"] = "true"
	resp.Headers["X-Transform-Time"] = fmt.Sprintf("%d", time.Now().Unix())
	
	p.logger.Debug("Transformed response", zap.Int("statusCode", resp.StatusCode))
	return resp, nil
}

// Manager manages the plugin system
type Manager struct {
	registry *Registry
	logger   *zap.Logger
}

// NewManager creates a new plugin manager
func NewManager(logger *zap.Logger, hookManager *hooks.Manager) *Manager {
	return &Manager{
		registry: NewRegistry(logger, hookManager),
		logger:   logger,
	}
}

// Registry returns the plugin registry
func (m *Manager) Registry() *Registry {
	return m.registry
}

// LoadBuiltinPlugins loads built-in example plugins
func (m *Manager) LoadBuiltinPlugins() error {
	// Register example plugins
	authPlugin := NewExampleAuthPlugin(m.logger)
	if err := m.registry.Register(authPlugin); err != nil {
		return fmt.Errorf("failed to register example auth plugin: %w", err)
	}

	transformPlugin := NewExampleTransformPlugin(m.logger)
	if err := m.registry.Register(transformPlugin); err != nil {
		return fmt.Errorf("failed to register example transform plugin: %w", err)
	}

	return nil
}