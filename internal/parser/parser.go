package parser

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"
)

// Parser handles parsing OpenAPI specifications into MCP tools
type Parser struct {
	logger  *zap.Logger
	baseURL string
	spec    *openapi3.T
	routes  []RouteConfig
}

// RouteConfig represents a parsed route from OpenAPI spec
type RouteConfig struct {
	Path        string
	Method      string
	OperationID string
	Summary     string
	Description string
	Parameters  []ParameterConfig
	RequestBody *RequestBodyConfig
	Tool        mcp.Tool
}

// ParameterConfig represents an OpenAPI parameter
type ParameterConfig struct {
	Name        string
	In          string // query, path, header
	Required    bool
	Type        string
	Description string
	Default     interface{}
	Enum        []interface{}
}

// RequestBodyConfig represents an OpenAPI request body
type RequestBodyConfig struct {
	Required    bool
	ContentType string
	Schema      *openapi3.SchemaRef
	Description string
}

// New creates a new parser instance
func New(logger *zap.Logger, baseURL string) *Parser {
	return &Parser{
		logger:  logger,
		baseURL: baseURL,
		routes:  make([]RouteConfig, 0),
	}
}

// ParseSpec parses an OpenAPI specification into routes and tools
func (p *Parser) ParseSpec(spec *openapi3.T) error {
	p.spec = spec
	p.routes = make([]RouteConfig, 0)

	if spec.Paths == nil {
		return fmt.Errorf("no paths found in OpenAPI specification")
	}

	for path, pathItem := range spec.Paths.Map() {
		if err := p.parsePath(path, pathItem); err != nil {
			p.logger.Error("Failed to parse path", zap.String("path", path), zap.Error(err))
			continue
		}
	}

	p.logger.Info("Parsed OpenAPI specification",
		zap.Int("routeCount", len(p.routes)),
		zap.String("title", spec.Info.Title),
		zap.String("version", spec.Info.Version))

	return nil
}

// parsePath processes a single path and its operations
func (p *Parser) parsePath(path string, pathItem *openapi3.PathItem) error {
	operations := map[string]*openapi3.Operation{
		"GET":     pathItem.Get,
		"POST":    pathItem.Post,
		"PUT":     pathItem.Put,
		"DELETE":  pathItem.Delete,
		"PATCH":   pathItem.Patch,
		"HEAD":    pathItem.Head,
		"OPTIONS": pathItem.Options,
	}

	for method, operation := range operations {
		if operation == nil {
			continue
		}

		route, err := p.parseOperation(path, method, operation, pathItem.Parameters)
		if err != nil {
			p.logger.Error("Failed to parse operation",
				zap.String("path", path),
				zap.String("method", method),
				zap.Error(err))
			continue
		}

		p.routes = append(p.routes, route)
	}

	return nil
}

// parseOperation converts an OpenAPI operation to a route config
func (p *Parser) parseOperation(path, method string, operation *openapi3.Operation, pathParams openapi3.Parameters) (RouteConfig, error) {
	route := RouteConfig{
		Path:        path,
		Method:      method,
		OperationID: operation.OperationID,
		Summary:     operation.Summary,
		Description: operation.Description,
		Parameters:  make([]ParameterConfig, 0),
	}

	// Generate operation ID if not provided
	if route.OperationID == "" {
		route.OperationID = p.generateOperationID(method, path)
	}

	// Parse parameters (path-level and operation-level)
	allParams := append(pathParams, operation.Parameters...)
	for _, paramRef := range allParams {
		if paramRef.Value == nil {
			continue
		}
		param := p.parseParameter(paramRef.Value)
		route.Parameters = append(route.Parameters, param)
	}

	// Parse request body
	if operation.RequestBody != nil && operation.RequestBody.Value != nil {
		route.RequestBody = p.parseRequestBody(operation.RequestBody.Value)
	}

	// Generate MCP tool
	tool, err := p.generateMCPTool(route)
	if err != nil {
		return route, fmt.Errorf("failed to generate MCP tool: %w", err)
	}
	route.Tool = tool

	return route, nil
}

// parseParameter converts an OpenAPI parameter to our parameter config
func (p *Parser) parseParameter(param *openapi3.Parameter) ParameterConfig {
	paramConfig := ParameterConfig{
		Name:        param.Name,
		In:          param.In,
		Required:    param.Required,
		Description: param.Description,
	}

	if param.Schema != nil && param.Schema.Value != nil {
		schema := param.Schema.Value
		// Handle the Types field properly - it's a slice, get the first type
		if schema.Type != nil && len(*schema.Type) > 0 {
			paramConfig.Type = (*schema.Type)[0]
		} else {
			paramConfig.Type = "string" // Default type
		}
		paramConfig.Default = schema.Default
		if schema.Enum != nil {
			paramConfig.Enum = schema.Enum
		}
	}

	return paramConfig
}

// parseRequestBody converts an OpenAPI request body to our config
func (p *Parser) parseRequestBody(requestBody *openapi3.RequestBody) *RequestBodyConfig {
	config := &RequestBodyConfig{
		Required:    requestBody.Required,
		Description: requestBody.Description,
	}

	// Find the first supported content type
	supportedTypes := []string{"application/json", "application/x-www-form-urlencoded", "text/plain"}
	for _, contentType := range supportedTypes {
		if content, exists := requestBody.Content[contentType]; exists {
			config.ContentType = contentType
			config.Schema = content.Schema
			break
		}
	}

	// Fallback to first available content type
	if config.ContentType == "" && len(requestBody.Content) > 0 {
		for contentType, content := range requestBody.Content {
			config.ContentType = contentType
			config.Schema = content.Schema
			break
		}
	}

	return config
}

// generateMCPTool creates an MCP tool definition from a route config
func (p *Parser) generateMCPTool(route RouteConfig) (mcp.Tool, error) {
	// Create tool name from operation ID or method+path
	toolName := route.OperationID
	if toolName == "" {
		toolName = p.generateOperationID(route.Method, route.Path)
	}

	// Create description
	description := route.Summary
	if description == "" {
		description = route.Description
	}
	if description == "" {
		description = fmt.Sprintf("%s %s", route.Method, route.Path)
	}

	// Create input schema for tool parameters
	inputSchema := p.createInputSchema(route)

	tool := mcp.Tool{
		Name:        toolName,
		Description: description,
		InputSchema: inputSchema,
	}

	return tool, nil
}

// createInputSchema creates a JSON schema for the tool parameters
func (p *Parser) createInputSchema(route RouteConfig) mcp.ToolInputSchema {
	properties := make(map[string]interface{})
	required := make([]string, 0)

	// Add parameters to schema
	for _, param := range route.Parameters {
		properties[param.Name] = p.parameterToSchema(param)
		if param.Required {
			required = append(required, param.Name)
		}
	}

	// Add request body as 'body' parameter if present
	if route.RequestBody != nil {
		properties["body"] = p.requestBodyToSchema(route.RequestBody)
		if route.RequestBody.Required {
			required = append(required, "body")
		}
	}

	schema := mcp.ToolInputSchema{
		Type:       "object",
		Properties: properties,
	}

	if len(required) > 0 {
		schema.Required = required
	}

	return schema
}

// parameterToSchema converts a parameter to JSON schema format
func (p *Parser) parameterToSchema(param ParameterConfig) map[string]interface{} {
	schema := map[string]interface{}{
		"type": param.Type,
	}

	if param.Description != "" {
		schema["description"] = param.Description
	}

	if param.Default != nil {
		schema["default"] = param.Default
	}

	if len(param.Enum) > 0 {
		schema["enum"] = param.Enum
	}

	return schema
}

// requestBodyToSchema converts a request body to JSON schema format
func (p *Parser) requestBodyToSchema(requestBody *RequestBodyConfig) map[string]interface{} {
	schema := map[string]interface{}{
		"type": "object",
	}

	if requestBody.Description != "" {
		schema["description"] = requestBody.Description
	}

	// Add content type information
	if requestBody.ContentType != "" {
		schema["contentType"] = requestBody.ContentType
	}

	return schema
}

// generateOperationID creates an operation ID from method and path
func (p *Parser) generateOperationID(method, path string) string {
	// Convert path to camelCase and remove special characters
	parts := strings.Split(strings.Trim(path, "/"), "/")
	var cleanParts []string

	for _, part := range parts {
		// Remove path parameters (e.g., {id} -> id)
		clean := strings.Trim(part, "{}")
		// Convert to camelCase
		if len(clean) > 0 {
			if len(cleanParts) == 0 {
				cleanParts = append(cleanParts, strings.ToLower(clean))
			} else {
				cleanParts = append(cleanParts, strings.Title(clean))
			}
		}
	}

	operationID := strings.ToLower(method)
	if len(cleanParts) > 0 {
		operationID += strings.Title(strings.Join(cleanParts, ""))
	}

	return operationID
}

// GetRoutes returns all parsed routes
func (p *Parser) GetRoutes() []RouteConfig {
	return p.routes
}

// GetTools returns all MCP tools generated from the routes
func (p *Parser) GetTools() []mcp.Tool {
	tools := make([]mcp.Tool, len(p.routes))
	for i, route := range p.routes {
		tools[i] = route.Tool
	}
	return tools
}

// GetRouteByOperationID finds a route by its operation ID
func (p *Parser) GetRouteByOperationID(operationID string) *RouteConfig {
	for _, route := range p.routes {
		if route.OperationID == operationID {
			return &route
		}
	}
	return nil
}
