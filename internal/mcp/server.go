package mcp

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/zeroLR/swagger-mcp-go/internal/models"
	"github.com/zeroLR/swagger-mcp-go/internal/registry"
	"github.com/zeroLR/swagger-mcp-go/internal/specs"
)

// Server represents the MCP server adapter
type Server struct {
	registry  *registry.Registry
	fetcher   *specs.Fetcher
	logger    *zap.Logger
	mcpServer *server.MCPServer
	stdioSrv  *server.StdioServer
}

// NewServer creates a new MCP server instance
func NewServer(logger *zap.Logger, reg *registry.Registry, fetcher *specs.Fetcher) *Server {
	s := &Server{
		registry: reg,
		fetcher:  fetcher,
		logger:   logger,
	}

	// Create the MCP server
	s.mcpServer = server.NewMCPServer("swagger-mcp-go", "1.0.0",
		server.WithToolCapabilities(false), // No tool list change notifications
		server.WithLogging(),
	)

	// Register tools
	s.registerTools()

	return s
}

// Start starts the MCP server using stdio transport
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("Starting MCP server with stdio transport")
	
	// Use stdio server for MCP communication
	return server.ServeStdio(s.mcpServer)
}

// Stop stops the MCP server
func (s *Server) Stop() error {
	s.logger.Info("Stopping MCP server")
	return nil
}

// registerTools registers all MCP tools with the server
func (s *Server) registerTools() {
	// Register listSpecs tool
	s.mcpServer.AddTool(
		mcpgo.NewTool("listSpecs",
			mcpgo.WithDescription("List all registered OpenAPI specifications"),
		),
		s.handleListSpecs,
	)

	// Register addSpec tool
	s.mcpServer.AddTool(
		mcpgo.NewTool("addSpec",
			mcpgo.WithDescription("Add a new OpenAPI specification from URL"),
			mcpgo.WithString("url", mcpgo.Description("URL to fetch the OpenAPI specification from"), mcpgo.Required()),
			mcpgo.WithString("serviceName", mcpgo.Description("Name for the service"), mcpgo.Required()),
			mcpgo.WithString("ttl", mcpgo.Description("Time-to-live for the specification cache (e.g., '1h', '30m')"), mcpgo.DefaultString("1h")),
			mcpgo.WithObject("headers", mcpgo.Description("Additional headers to send with the request")),
		),
		s.handleAddSpec,
	)

	// Register refreshSpec tool
	s.mcpServer.AddTool(
		mcpgo.NewTool("refreshSpec",
			mcpgo.WithDescription("Force refresh of an existing OpenAPI specification"),
			mcpgo.WithString("serviceName", mcpgo.Description("Name of the service to refresh"), mcpgo.Required()),
		),
		s.handleRefreshSpec,
	)

	// Register removeSpec tool
	s.mcpServer.AddTool(
		mcpgo.NewTool("removeSpec",
			mcpgo.WithDescription("Remove an OpenAPI specification"),
			mcpgo.WithString("serviceName", mcpgo.Description("Name of the service to remove"), mcpgo.Required()),
		),
		s.handleRemoveSpec,
	)

	// Register inspectRoute tool
	s.mcpServer.AddTool(
		mcpgo.NewTool("inspectRoute",
			mcpgo.WithDescription("Inspect route configuration for a service"),
			mcpgo.WithString("serviceName", mcpgo.Description("Name of the service to inspect"), mcpgo.Required()),
		),
		s.handleInspectRoute,
	)

	// Register getStats tool
	s.mcpServer.AddTool(
		mcpgo.NewTool("getStats",
			mcpgo.WithDescription("Get performance statistics for all services or a specific service"),
			mcpgo.WithString("serviceName", mcpgo.Description("Optional: Name of specific service to get stats for")),
		),
		s.handleGetStats,
	)

	// Register enableAuthPolicy tool
	s.mcpServer.AddTool(
		mcpgo.NewTool("enableAuthPolicy",
			mcpgo.WithDescription("Enable authentication policy for a service"),
			mcpgo.WithString("serviceName", mcpgo.Description("Name of the service"), mcpgo.Required()),
			mcpgo.WithString("authType", mcpgo.Description("Type of authentication (basic, bearer, oauth2)"), mcpgo.Required()),
			mcpgo.WithObject("config", mcpgo.Description("Authentication configuration parameters")),
			mcpgo.WithBoolean("required", mcpgo.Description("Whether authentication is required"), mcpgo.DefaultBool(true)),
		),
		s.handleEnableAuthPolicy,
	)

	// Register disableAuthPolicy tool
	s.mcpServer.AddTool(
		mcpgo.NewTool("disableAuthPolicy",
			mcpgo.WithDescription("Disable authentication policy for a service"),
			mcpgo.WithString("serviceName", mcpgo.Description("Name of the service"), mcpgo.Required()),
		),
		s.handleDisableAuthPolicy,
	)
}

// Legacy methods for compatibility
func (s *Server) ListSpecs() []*models.SpecInfo {
	return s.registry.List()
}

func (s *Server) AddSpec(ctx context.Context, url, serviceName string, headers map[string]string, ttl time.Duration) (*models.SpecInfo, error) {
	spec, err := s.fetcher.FetchSpec(ctx, url, serviceName, headers, ttl)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch spec: %w", err)
	}

	if err := s.registry.Add(spec); err != nil {
		return nil, fmt.Errorf("failed to add spec to registry: %w", err)
	}

	return spec, nil
}

func (s *Server) RemoveSpec(serviceName string) bool {
	return s.registry.Remove(serviceName)
}

func (s *Server) GetStats() map[string]interface{} {
	return s.registry.Stats()
}

// handleListSpecs handles the listSpecs tool call
func (s *Server) handleListSpecs(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	s.logger.Debug("Handling listSpecs tool call")

	specs := s.registry.List()
	
	// Format specs for response
	result := make([]map[string]interface{}, len(specs))
	for i, spec := range specs {
		result[i] = map[string]interface{}{
			"id":          spec.ID,
			"serviceName": spec.ServiceName,
			"url":         spec.URL,
			"fetchedAt":   spec.FetchedAt.Format(time.RFC3339),
			"ttl":         spec.TTL.String(),
			"title":       "",
			"version":     "",
			"pathCount":   0,
		}
		
		if spec.Spec != nil && spec.Spec.Info != nil {
			result[i]["title"] = spec.Spec.Info.Title
			result[i]["version"] = spec.Spec.Info.Version
		}
		
		if spec.Spec != nil && spec.Spec.Paths != nil {
			result[i]["pathCount"] = len(spec.Spec.Paths.Map())
		}
	}

	return mcpgo.NewToolResultStructured(map[string]interface{}{
		"specs": result,
		"count": len(specs),
	}, fmt.Sprintf("Found %d registered OpenAPI specifications", len(specs))), nil
}

// handleAddSpec handles the addSpec tool call
func (s *Server) handleAddSpec(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	s.logger.Debug("Handling addSpec tool call")

	// Parse arguments
	url := mcpgo.ParseString(request, "url", "")
	serviceName := mcpgo.ParseString(request, "serviceName", "")
	ttlStr := mcpgo.ParseString(request, "ttl", "1h")
	headers := mcpgo.ParseStringMap(request, "headers", make(map[string]any))

	if url == "" {
		return mcpgo.NewToolResultError("URL is required"), nil
	}
	if serviceName == "" {
		return mcpgo.NewToolResultError("serviceName is required"), nil
	}

	// Parse TTL
	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		return mcpgo.NewToolResultErrorFromErr("Invalid TTL format", err), nil
	}

	// Convert headers to string map
	stringHeaders := make(map[string]string)
	for k, v := range headers {
		if str, ok := v.(string); ok {
			stringHeaders[k] = str
		}
	}

	// Add the spec
	spec, err := s.AddSpec(ctx, url, serviceName, stringHeaders, ttl)
	if err != nil {
		return mcpgo.NewToolResultErrorFromErr("Failed to add specification", err), nil
	}

	result := map[string]interface{}{
		"success":     true,
		"serviceName": spec.ServiceName,
		"url":         spec.URL,
		"fetchedAt":   spec.FetchedAt.Format(time.RFC3339),
		"ttl":         spec.TTL.String(),
	}

	if spec.Spec != nil && spec.Spec.Info != nil {
		result["title"] = spec.Spec.Info.Title
		result["version"] = spec.Spec.Info.Version
	}

	if spec.Spec != nil && spec.Spec.Paths != nil {
		result["pathCount"] = len(spec.Spec.Paths.Map())
	}

	return mcpgo.NewToolResultStructured(result, 
		fmt.Sprintf("Successfully added OpenAPI specification for service '%s'", serviceName)), nil
}

// handleRefreshSpec handles the refreshSpec tool call
func (s *Server) handleRefreshSpec(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	s.logger.Debug("Handling refreshSpec tool call")

	serviceName := mcpgo.ParseString(request, "serviceName", "")
	if serviceName == "" {
		return mcpgo.NewToolResultError("serviceName is required"), nil
	}

	// Get existing spec
	existingSpec, exists := s.registry.Get(serviceName)
	if !exists {
		return mcpgo.NewToolResultError(fmt.Sprintf("Service '%s' not found", serviceName)), nil
	}

	// Refresh the spec
	refreshedSpec, err := s.fetcher.FetchSpec(ctx, existingSpec.URL, serviceName, existingSpec.Headers, existingSpec.TTL)
	if err != nil {
		return mcpgo.NewToolResultErrorFromErr("Failed to refresh specification", err), nil
	}

	// Update in registry
	if err := s.registry.Add(refreshedSpec); err != nil {
		return mcpgo.NewToolResultErrorFromErr("Failed to update specification in registry", err), nil
	}

	result := map[string]interface{}{
		"success":     true,
		"serviceName": refreshedSpec.ServiceName,
		"url":         refreshedSpec.URL,
		"fetchedAt":   refreshedSpec.FetchedAt.Format(time.RFC3339),
		"ttl":         refreshedSpec.TTL.String(),
		"previousFetchedAt": existingSpec.FetchedAt.Format(time.RFC3339),
	}

	if refreshedSpec.Spec != nil && refreshedSpec.Spec.Info != nil {
		result["title"] = refreshedSpec.Spec.Info.Title
		result["version"] = refreshedSpec.Spec.Info.Version
	}

	return mcpgo.NewToolResultStructured(result,
		fmt.Sprintf("Successfully refreshed OpenAPI specification for service '%s'", serviceName)), nil
}

// handleRemoveSpec handles the removeSpec tool call
func (s *Server) handleRemoveSpec(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	s.logger.Debug("Handling removeSpec tool call")

	serviceName := mcpgo.ParseString(request, "serviceName", "")
	if serviceName == "" {
		return mcpgo.NewToolResultError("serviceName is required"), nil
	}

	removed := s.registry.Remove(serviceName)
	if !removed {
		return mcpgo.NewToolResultError(fmt.Sprintf("Service '%s' not found", serviceName)), nil
	}

	result := map[string]interface{}{
		"success":     true,
		"serviceName": serviceName,
		"removed":     true,
	}

	return mcpgo.NewToolResultStructured(result,
		fmt.Sprintf("Successfully removed OpenAPI specification for service '%s'", serviceName)), nil
}

// handleInspectRoute handles the inspectRoute tool call
func (s *Server) handleInspectRoute(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	s.logger.Debug("Handling inspectRoute tool call")

	serviceName := mcpgo.ParseString(request, "serviceName", "")
	if serviceName == "" {
		return mcpgo.NewToolResultError("serviceName is required"), nil
	}

	spec, exists := s.registry.Get(serviceName)
	if !exists {
		return mcpgo.NewToolResultError(fmt.Sprintf("Service '%s' not found", serviceName)), nil
	}

	// Extract route information
	routes := make([]models.RouteInfo, 0)
	if spec.Spec != nil && spec.Spec.Paths != nil {
		for path, pathItem := range spec.Spec.Paths.Map() {
			for method, operation := range pathItem.Operations() {
				route := models.RouteInfo{
					Path:        path,
					Method:      method,
					ServiceName: serviceName,
				}
				
				if operation.OperationID != "" {
					route.OperationID = operation.OperationID
				}
				if operation.Summary != "" {
					route.Summary = operation.Summary
				}
				if len(operation.Tags) > 0 {
					route.Tags = operation.Tags
				}
				
				routes = append(routes, route)
			}
		}
	}

	result := map[string]interface{}{
		"serviceName": serviceName,
		"url":         spec.URL,
		"routeCount":  len(routes),
		"routes":      routes,
	}

	if spec.AuthPolicy != nil {
		result["authPolicy"] = map[string]interface{}{
			"type":     string(spec.AuthPolicy.Type),
			"required": spec.AuthPolicy.Required,
			"scopes":   spec.AuthPolicy.Scopes,
		}
	}

	return mcpgo.NewToolResultStructured(result,
		fmt.Sprintf("Service '%s' has %d routes", serviceName, len(routes))), nil
}

// handleGetStats handles the getStats tool call
func (s *Server) handleGetStats(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	s.logger.Debug("Handling getStats tool call")

	serviceName := mcpgo.ParseString(request, "serviceName", "")
	
	if serviceName != "" {
		// Get stats for specific service
		spec, exists := s.registry.Get(serviceName)
		if !exists {
			return mcpgo.NewToolResultError(fmt.Sprintf("Service '%s' not found", serviceName)), nil
		}

		stats := models.ServiceStats{
			ServiceName:   serviceName,
			SpecFetchedAt: spec.FetchedAt,
			SpecURL:       spec.URL,
		}

		if spec.Spec != nil && spec.Spec.Paths != nil {
			stats.RouteCount = len(spec.Spec.Paths.Map())
		}

		result := map[string]interface{}{
			"serviceName": serviceName,
			"stats":       stats,
		}

		return mcpgo.NewToolResultStructured(result,
			fmt.Sprintf("Statistics for service '%s'", serviceName)), nil
	}

	// Get global stats
	globalStats := s.registry.Stats()
	
	// Get individual service stats
	serviceStats := make([]models.ServiceStats, 0)
	for _, spec := range s.registry.List() {
		stats := models.ServiceStats{
			ServiceName:   spec.ServiceName,
			SpecFetchedAt: spec.FetchedAt,
			SpecURL:       spec.URL,
		}

		if spec.Spec != nil && spec.Spec.Paths != nil {
			stats.RouteCount = len(spec.Spec.Paths.Map())
		}

		serviceStats = append(serviceStats, stats)
	}

	result := map[string]interface{}{
		"global":   globalStats,
		"services": serviceStats,
	}

	return mcpgo.NewToolResultStructured(result,
		fmt.Sprintf("Statistics for %d services", len(serviceStats))), nil
}

// handleEnableAuthPolicy handles the enableAuthPolicy tool call
func (s *Server) handleEnableAuthPolicy(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	s.logger.Debug("Handling enableAuthPolicy tool call")

	serviceName := mcpgo.ParseString(request, "serviceName", "")
	authTypeStr := mcpgo.ParseString(request, "authType", "")
	config := mcpgo.ParseStringMap(request, "config", make(map[string]any))
	required := mcpgo.ParseBoolean(request, "required", true)

	if serviceName == "" {
		return mcpgo.NewToolResultError("serviceName is required"), nil
	}
	if authTypeStr == "" {
		return mcpgo.NewToolResultError("authType is required"), nil
	}

	// Validate auth type
	var authType models.AuthType
	switch authTypeStr {
	case "basic":
		authType = models.AuthTypeBasic
	case "bearer":
		authType = models.AuthTypeBearer
	case "oauth2":
		authType = models.AuthTypeOAuth2
	default:
		return mcpgo.NewToolResultError("Invalid authType. Must be one of: basic, bearer, oauth2"), nil
	}

	// Get existing spec
	spec, exists := s.registry.Get(serviceName)
	if !exists {
		return mcpgo.NewToolResultError(fmt.Sprintf("Service '%s' not found", serviceName)), nil
	}

	// Create and set auth policy
	authPolicy := &models.AuthPolicy{
		Type:     authType,
		Config:   config,
		Required: required,
	}

	// Update spec with auth policy
	spec.AuthPolicy = authPolicy
	if err := s.registry.Add(spec); err != nil {
		return mcpgo.NewToolResultErrorFromErr("Failed to update authentication policy", err), nil
	}

	result := map[string]interface{}{
		"success":     true,
		"serviceName": serviceName,
		"authPolicy": map[string]interface{}{
			"type":     string(authType),
			"required": required,
		},
	}

	return mcpgo.NewToolResultStructured(result,
		fmt.Sprintf("Successfully enabled %s authentication for service '%s'", authType, serviceName)), nil
}

// handleDisableAuthPolicy handles the disableAuthPolicy tool call
func (s *Server) handleDisableAuthPolicy(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	s.logger.Debug("Handling disableAuthPolicy tool call")

	serviceName := mcpgo.ParseString(request, "serviceName", "")
	if serviceName == "" {
		return mcpgo.NewToolResultError("serviceName is required"), nil
	}

	// Get existing spec
	spec, exists := s.registry.Get(serviceName)
	if !exists {
		return mcpgo.NewToolResultError(fmt.Sprintf("Service '%s' not found", serviceName)), nil
	}

	// Remove auth policy
	spec.AuthPolicy = nil
	if err := s.registry.Add(spec); err != nil {
		return mcpgo.NewToolResultErrorFromErr("Failed to disable authentication policy", err), nil
	}

	result := map[string]interface{}{
		"success":     true,
		"serviceName": serviceName,
		"authPolicy":  nil,
	}

	return mcpgo.NewToolResultStructured(result,
		fmt.Sprintf("Successfully disabled authentication for service '%s'", serviceName)), nil
}