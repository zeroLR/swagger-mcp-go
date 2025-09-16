package plugins

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/zeroLR/swagger-mcp-go/internal/hooks"
	"go.uber.org/zap"
)

func TestRegistry_BasicOperations(t *testing.T) {
	logger := zap.NewNop()
	hookManager := hooks.NewManager(logger)
	registry := NewRegistry(logger, hookManager)

	// Test empty registry
	if len(registry.List()) != 0 {
		t.Errorf("Expected empty registry")
	}

	// Create test plugin
	plugin := &testPlugin{
		name:        "test-plugin",
		pluginType:  PluginTypeAuth,
		version:     "1.0.0",
		description: "Test plugin",
	}

	// Test registration
	err := registry.Register(plugin)
	if err != nil {
		t.Errorf("Failed to register plugin: %v", err)
	}

	// Test duplicate registration
	err = registry.Register(plugin)
	if err == nil {
		t.Errorf("Expected error for duplicate registration")
	}

	// Test retrieval by name
	retrieved, exists := registry.Get("test-plugin")
	if !exists {
		t.Errorf("Plugin should exist")
	}
	if retrieved.Name() != "test-plugin" {
		t.Errorf("Retrieved wrong plugin")
	}

	// Test retrieval by type
	authPlugins := registry.GetByType(PluginTypeAuth)
	if len(authPlugins) != 1 {
		t.Errorf("Expected 1 auth plugin, got %d", len(authPlugins))
	}

	// Test list all
	allPlugins := registry.List()
	if len(allPlugins) != 1 {
		t.Errorf("Expected 1 plugin in list, got %d", len(allPlugins))
	}
}

func TestRegistry_Lifecycle(t *testing.T) {
	logger := zap.NewNop()
	hookManager := hooks.NewManager(logger)
	registry := NewRegistry(logger, hookManager)

	plugin := &testPlugin{
		name:        "lifecycle-plugin",
		pluginType:  PluginTypeValidation,
		version:     "1.0.0",
		description: "Lifecycle test plugin",
	}

	registry.Register(plugin)

	// Test initialization
	configs := map[string]map[string]interface{}{
		"lifecycle-plugin": {
			"testConfig": "testValue",
		},
	}

	err := registry.Initialize(configs)
	if err != nil {
		t.Errorf("Failed to initialize plugins: %v", err)
	}

	if !plugin.initialized {
		t.Errorf("Plugin should be initialized")
	}

	// Test start
	ctx := context.Background()
	err = registry.Start(ctx)
	if err != nil {
		t.Errorf("Failed to start plugins: %v", err)
	}

	if !plugin.started {
		t.Errorf("Plugin should be started")
	}

	// Test health
	health := registry.Health()
	pluginHealth, exists := health["lifecycle-plugin"]
	if !exists {
		t.Errorf("Plugin health should exist")
	}
	if !pluginHealth.Healthy {
		t.Errorf("Plugin should be healthy")
	}

	// Test stop
	err = registry.Stop()
	if err != nil {
		t.Errorf("Failed to stop plugins: %v", err)
	}

	if !plugin.stopped {
		t.Errorf("Plugin should be stopped")
	}
}

func TestExampleAuthPlugin(t *testing.T) {
	logger := zap.NewNop()
	plugin := NewExampleAuthPlugin(logger)

	// Test plugin metadata
	if plugin.Name() != "example-auth" {
		t.Errorf("Expected name 'example-auth', got %s", plugin.Name())
	}
	if plugin.Type() != PluginTypeAuth {
		t.Errorf("Expected auth type")
	}
	if plugin.Version() != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got %s", plugin.Version())
	}

	// Test initialization
	config := map[string]interface{}{
		"users": map[string]interface{}{
			"testuser": "testpass",
			"admin":    "secret",
		},
	}

	err := plugin.Initialize(config)
	if err != nil {
		t.Errorf("Failed to initialize plugin: %v", err)
	}

	// Test lifecycle
	ctx := context.Background()
	err = plugin.Start(ctx)
	if err != nil {
		t.Errorf("Failed to start plugin: %v", err)
	}

	health := plugin.Health()
	if !health.Healthy {
		t.Errorf("Plugin should be healthy")
	}

	// Test authentication
	tests := []struct {
		username string
		password string
		expected bool
	}{
		{"testuser", "testpass", true},
		{"admin", "secret", true},
		{"testuser", "wrongpass", false},
		{"nonexistent", "testpass", false},
	}

	for _, test := range tests {
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth(test.username, test.password)

		result, err := plugin.Authenticate(context.Background(), req)
		if err != nil {
			t.Errorf("Authenticate should not error: %v", err)
		}

		if result.Authenticated != test.expected {
			t.Errorf("Expected authentication result %v for %s:%s, got %v",
				test.expected, test.username, test.password, result.Authenticated)
		}

		if test.expected && result.Username != test.username {
			t.Errorf("Expected username %s, got %s", test.username, result.Username)
		}
	}

	// Test authentication without basic auth
	req := httptest.NewRequest("GET", "/", nil)
	result, err := plugin.Authenticate(context.Background(), req)
	if err != nil {
		t.Errorf("Authenticate should not error: %v", err)
	}
	if result.Authenticated {
		t.Errorf("Should not authenticate without basic auth")
	}

	err = plugin.Stop()
	if err != nil {
		t.Errorf("Failed to stop plugin: %v", err)
	}
}

func TestExampleTransformPlugin(t *testing.T) {
	logger := zap.NewNop()
	plugin := NewExampleTransformPlugin(logger)

	// Test plugin metadata
	if plugin.Name() != "example-transform" {
		t.Errorf("Expected name 'example-transform', got %s", plugin.Name())
	}
	if plugin.Type() != PluginTypeTransform {
		t.Errorf("Expected transform type")
	}

	// Test initialization and lifecycle
	err := plugin.Initialize(map[string]interface{}{})
	if err != nil {
		t.Errorf("Failed to initialize plugin: %v", err)
	}

	ctx := context.Background()
	err = plugin.Start(ctx)
	if err != nil {
		t.Errorf("Failed to start plugin: %v", err)
	}

	// Test request transformation
	originalReq := &TransformRequest{
		Method:      "GET",
		URL:         "/test",
		Headers:     map[string]string{"Original": "header"},
		Parameters:  map[string]interface{}{"param": "value"},
		ServiceName: "test-service",
	}

	transformedReq, err := plugin.TransformRequest(context.Background(), originalReq)
	if err != nil {
		t.Errorf("Failed to transform request: %v", err)
	}

	// Check that transform plugin header was added
	if transformedReq.Headers["X-Transform-Plugin"] != "example-transform" {
		t.Errorf("Expected X-Transform-Plugin header")
	}

	if transformedReq.Headers["X-Request-ID"] == "" {
		t.Errorf("Expected X-Request-ID header")
	}

	// Original headers should still be there
	if transformedReq.Headers["Original"] != "header" {
		t.Errorf("Original headers should be preserved")
	}

	// Test response transformation
	originalResp := &TransformResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       []byte(`{"test": "data"}`),
	}

	transformedResp, err := plugin.TransformResponse(context.Background(), originalResp)
	if err != nil {
		t.Errorf("Failed to transform response: %v", err)
	}

	// Check that transform headers were added
	if transformedResp.Headers["X-Response-Transformed"] != "true" {
		t.Errorf("Expected X-Response-Transformed header")
	}

	if transformedResp.Headers["X-Transform-Time"] == "" {
		t.Errorf("Expected X-Transform-Time header")
	}

	// Original headers should still be there
	if transformedResp.Headers["Content-Type"] != "application/json" {
		t.Errorf("Original headers should be preserved")
	}

	err = plugin.Stop()
	if err != nil {
		t.Errorf("Failed to stop plugin: %v", err)
	}
}

func TestManager(t *testing.T) {
	logger := zap.NewNop()
	hookManager := hooks.NewManager(logger)
	manager := NewManager(logger, hookManager)

	// Test registry access
	registry := manager.Registry()
	if registry == nil {
		t.Errorf("Registry should not be nil")
	}

	// Test loading builtin plugins
	err := manager.LoadBuiltinPlugins()
	if err != nil {
		t.Errorf("Failed to load builtin plugins: %v", err)
	}

	// Check that builtin plugins were loaded
	plugins := registry.List()
	if len(plugins) < 2 {
		t.Errorf("Expected at least 2 builtin plugins, got %d", len(plugins))
	}

	// Check specific plugins
	authPlugin, exists := registry.Get("example-auth")
	if !exists {
		t.Errorf("Example auth plugin should be loaded")
	}
	if authPlugin.Type() != PluginTypeAuth {
		t.Errorf("Expected auth plugin type")
	}

	transformPlugin, exists := registry.Get("example-transform")
	if !exists {
		t.Errorf("Example transform plugin should be loaded")
	}
	if transformPlugin.Type() != PluginTypeTransform {
		t.Errorf("Expected transform plugin type")
	}
}

func TestPluginHookIntegration(t *testing.T) {
	logger := zap.NewNop()
	hookManager := hooks.NewManager(logger)
	registry := NewRegistry(logger, hookManager)

	// Create validation plugin
	validationPlugin := &testValidationPlugin{
		testPlugin: testPlugin{
			name:        "test-validation",
			pluginType:  PluginTypeValidation,
			version:     "1.0.0",
			description: "Test validation plugin",
		},
	}

	// Register plugin (should automatically register hook)
	err := registry.Register(validationPlugin)
	if err != nil {
		t.Errorf("Failed to register validation plugin: %v", err)
	}

	// Create hook context
	hookCtx := &hooks.HookContext{
		Request: &hooks.RequestContext{
			ServiceName: "test-service",
			OperationID: "test-operation",
			Method:      "GET",
			Path:        "/test",
			Headers:     map[string]string{"Test": "header"},
			Parameters:  map[string]interface{}{"param": "value"},
		},
		Metadata: make(map[string]interface{}),
	}

	// Execute pre-request hooks
	err = hookManager.ExecutePreRequestHooks(context.Background(), hookCtx)
	if err != nil {
		t.Errorf("Hook execution should not fail: %v", err)
	}

	if !validationPlugin.validateCalled {
		t.Errorf("Validation plugin should have been called")
	}
}

// Test helper structs

type testPlugin struct {
	name        string
	pluginType  PluginType
	version     string
	description string
	initialized bool
	started     bool
	stopped     bool
}

func (p *testPlugin) Name() string        { return p.name }
func (p *testPlugin) Type() PluginType    { return p.pluginType }
func (p *testPlugin) Version() string     { return p.version }
func (p *testPlugin) Description() string { return p.description }

func (p *testPlugin) Initialize(config map[string]interface{}) error {
	p.initialized = true
	return nil
}

func (p *testPlugin) Start(ctx context.Context) error {
	p.started = true
	return nil
}

func (p *testPlugin) Stop() error {
	p.stopped = true
	return nil
}

func (p *testPlugin) Health() HealthStatus {
	return HealthStatus{
		Healthy: true,
		Message: "Test plugin is healthy",
	}
}

type testValidationPlugin struct {
	testPlugin
	validateCalled bool
}

func (p *testValidationPlugin) ValidateRequest(ctx context.Context, req *ValidationRequest) error {
	p.validateCalled = true
	return nil
}

type testTransformPlugin struct {
	testPlugin
	transformRequestCalled  bool
	transformResponseCalled bool
}

func (p *testTransformPlugin) TransformRequest(ctx context.Context, req *TransformRequest) (*TransformRequest, error) {
	p.transformRequestCalled = true
	return req, nil
}

func (p *testTransformPlugin) TransformResponse(ctx context.Context, resp *TransformResponse) (*TransformResponse, error) {
	p.transformResponseCalled = true
	return resp, nil
}
