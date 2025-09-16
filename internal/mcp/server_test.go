package mcp

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/zeroLR/swagger-mcp-go/internal/models"
	"github.com/zeroLR/swagger-mcp-go/internal/registry"
	"github.com/zeroLR/swagger-mcp-go/internal/specs"
)

func createTestServer(t *testing.T) *Server {
	logger := zap.NewNop()
	reg := registry.New(logger)
	fetcher := specs.New(logger, 30*time.Second, 10*1024*1024)
	
	return NewServer(logger, reg, fetcher)
}

func TestNewServer(t *testing.T) {
	server := createTestServer(t)
	
	if server == nil {
		t.Fatal("Expected server to be created")
	}
	
	if server.mcpServer == nil {
		t.Fatal("Expected MCP server to be initialized")
	}
	
	if server.registry == nil {
		t.Fatal("Expected registry to be set")
	}
	
	if server.fetcher == nil {
		t.Fatal("Expected fetcher to be set")
	}
}

func TestListSpecsEmpty(t *testing.T) {
	server := createTestServer(t)
	ctx := context.Background()
	
	// Create a request with empty arguments
	request := mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name:      "listSpecs",
			Arguments: map[string]interface{}{},
		},
	}
	
	result, err := server.handleListSpecs(ctx, request)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}
	
	// Check structured content
	if result.StructuredContent == nil {
		t.Fatal("Expected structured content to be non-nil")
	}
	
	structuredResult, ok := result.StructuredContent.(map[string]interface{})
	if !ok {
		t.Fatal("Expected structured content to be a map")
	}
	
	count, ok := structuredResult["count"].(int)
	if !ok {
		t.Fatal("Expected count to be an integer")
	}
	
	if count != 0 {
		t.Fatalf("Expected count to be 0, got: %d", count)
	}
	
	specs, ok := structuredResult["specs"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected specs to be an array")
	}
	
	if len(specs) != 0 {
		t.Fatalf("Expected specs array to be empty, got length: %d", len(specs))
	}
}

func TestGetStatsEmpty(t *testing.T) {
	server := createTestServer(t)
	ctx := context.Background()
	
	request := mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name:      "getStats",
			Arguments: map[string]interface{}{},
		},
	}
	
	result, err := server.handleGetStats(ctx, request)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}
	
	// Check structured content
	if result.StructuredContent == nil {
		t.Fatal("Expected structured content to be non-nil")
	}
	
	structuredResult, ok := result.StructuredContent.(map[string]interface{})
	if !ok {
		t.Fatal("Expected structured content to be a map")
	}
	
	global, ok := structuredResult["global"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected global stats to be an object")
	}
	
	totalSpecs, ok := global["totalSpecs"].(int)
	if !ok {
		t.Fatal("Expected totalSpecs to be a number")
	}
	
	if totalSpecs != 0 {
		t.Fatalf("Expected totalSpecs to be 0, got: %d", totalSpecs)
	}
	
	services, ok := structuredResult["services"].([]models.ServiceStats)
	if !ok {
		t.Fatal("Expected services to be an array")
	}
	
	if len(services) != 0 {
		t.Fatalf("Expected services array to be empty, got length: %d", len(services))
	}
}

func TestRemoveSpecNotFound(t *testing.T) {
	server := createTestServer(t)
	ctx := context.Background()
	
	request := mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "removeSpec",
			Arguments: map[string]interface{}{
				"serviceName": "nonexistent",
			},
		},
	}
	
	result, err := server.handleRemoveSpec(ctx, request)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	// For MCP tools, errors are returned as tool results, not as Go errors
	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}
	
	// The result should indicate an error through the content
	if len(result.Content) == 0 {
		t.Fatal("Expected result to have content")
	}
	
	textContent, ok := mcpgo.AsTextContent(result.Content[0])
	if !ok {
		t.Fatal("Expected content to be text content")
	}
	
	if textContent.Text != "Service 'nonexistent' not found" {
		t.Fatalf("Expected error message about service not found, got: %s", textContent.Text)
	}
}

func TestInspectRouteNotFound(t *testing.T) {
	server := createTestServer(t)
	ctx := context.Background()
	
	request := mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "inspectRoute",
			Arguments: map[string]interface{}{
				"serviceName": "nonexistent",
			},
		},
	}
	
	result, err := server.handleInspectRoute(ctx, request)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}
	
	if len(result.Content) == 0 {
		t.Fatal("Expected result to have content")
	}
	
	textContent, ok := mcpgo.AsTextContent(result.Content[0])
	if !ok {
		t.Fatal("Expected content to be text content")
	}
	
	if textContent.Text != "Service 'nonexistent' not found" {
		t.Fatalf("Expected error message about service not found, got: %s", textContent.Text)
	}
}

func TestEnableAuthPolicyNotFound(t *testing.T) {
	server := createTestServer(t)
	ctx := context.Background()
	
	request := mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "enableAuthPolicy",
			Arguments: map[string]interface{}{
				"serviceName": "nonexistent",
				"authType":    "basic",
			},
		},
	}
	
	result, err := server.handleEnableAuthPolicy(ctx, request)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}
	
	if len(result.Content) == 0 {
		t.Fatal("Expected result to have content")
	}
	
	textContent, ok := mcpgo.AsTextContent(result.Content[0])
	if !ok {
		t.Fatal("Expected content to be text content")
	}
	
	if textContent.Text != "Service 'nonexistent' not found" {
		t.Fatalf("Expected error message about service not found, got: %s", textContent.Text)
	}
}

func TestEnableAuthPolicyInvalidType(t *testing.T) {
	server := createTestServer(t)
	ctx := context.Background()
	
	request := mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "enableAuthPolicy",
			Arguments: map[string]interface{}{
				"serviceName": "test",
				"authType":    "invalid",
			},
		},
	}
	
	result, err := server.handleEnableAuthPolicy(ctx, request)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}
	
	if len(result.Content) == 0 {
		t.Fatal("Expected result to have content")
	}
	
	textContent, ok := mcpgo.AsTextContent(result.Content[0])
	if !ok {
		t.Fatal("Expected content to be text content")
	}
	
	if textContent.Text != "Invalid authType. Must be one of: basic, bearer, oauth2" {
		t.Fatalf("Expected error message about invalid auth type, got: %s", textContent.Text)
	}
}

func TestMissingRequiredParameters(t *testing.T) {
	server := createTestServer(t)
	ctx := context.Background()
	
	testCases := []struct {
		toolName string
		args     map[string]interface{}
		expected string
	}{
		{
			toolName: "addSpec",
			args:     map[string]interface{}{},
			expected: "URL is required",
		},
		{
			toolName: "addSpec",
			args:     map[string]interface{}{"url": "http://example.com"},
			expected: "serviceName is required",
		},
		{
			toolName: "removeSpec",
			args:     map[string]interface{}{},
			expected: "serviceName is required",
		},
		{
			toolName: "refreshSpec",
			args:     map[string]interface{}{},
			expected: "serviceName is required",
		},
		{
			toolName: "inspectRoute",
			args:     map[string]interface{}{},
			expected: "serviceName is required",
		},
		{
			toolName: "enableAuthPolicy",
			args:     map[string]interface{}{},
			expected: "serviceName is required",
		},
		{
			toolName: "enableAuthPolicy",
			args:     map[string]interface{}{"serviceName": "test"},
			expected: "authType is required",
		},
		{
			toolName: "disableAuthPolicy",
			args:     map[string]interface{}{},
			expected: "serviceName is required",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.toolName, func(t *testing.T) {
			request := mcpgo.CallToolRequest{
				Params: mcpgo.CallToolParams{
					Name:      tc.toolName,
					Arguments: tc.args,
				},
			}
			
			var result *mcpgo.CallToolResult
			var err error
			
			switch tc.toolName {
			case "addSpec":
				result, err = server.handleAddSpec(ctx, request)
			case "removeSpec":
				result, err = server.handleRemoveSpec(ctx, request)
			case "refreshSpec":
				result, err = server.handleRefreshSpec(ctx, request)
			case "inspectRoute":
				result, err = server.handleInspectRoute(ctx, request)
			case "enableAuthPolicy":
				result, err = server.handleEnableAuthPolicy(ctx, request)
			case "disableAuthPolicy":
				result, err = server.handleDisableAuthPolicy(ctx, request)
			}
			
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}
			
			if result == nil {
				t.Fatal("Expected result to be non-nil")
			}
			
			if len(result.Content) == 0 {
				t.Fatal("Expected result to have content")
			}
			
			textContent, ok := mcpgo.AsTextContent(result.Content[0])
			if !ok {
				t.Fatal("Expected content to be text content")
			}
			
			if textContent.Text != tc.expected {
				t.Fatalf("Expected error message '%s', got: '%s'", tc.expected, textContent.Text)
			}
		})
	}
}