package mcp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/zeroLR/swagger-mcp-go/internal/config"
	"github.com/zeroLR/swagger-mcp-go/internal/models"
	"github.com/zeroLR/swagger-mcp-go/internal/parser"
	"github.com/zeroLR/swagger-mcp-go/internal/proxy"
	"github.com/zeroLR/swagger-mcp-go/internal/registry"
	"github.com/zeroLR/swagger-mcp-go/internal/specs"
)

// ServerMode represents the MCP server mode
type ServerMode string

const (
	ServerModeSTDIO ServerMode = "stdio"
	ServerModeHTTP  ServerMode = "http"
	ServerModeSSE   ServerMode = "sse"
)

// Server represents the MCP server implementation
type Server struct {
	registry    *registry.Registry
	fetcher     *specs.Fetcher
	logger      *zap.Logger
	config      *config.Config
	mcpServer   *mcpserver.MCPServer
	parser      *parser.Parser
	proxyEngine *proxy.Engine
	mode        ServerMode
}

// NewServer creates a new MCP server instance
func NewServer(logger *zap.Logger, cfg *config.Config, reg *registry.Registry, fetcher *specs.Fetcher) *Server {
	mcpServer := mcpserver.NewMCPServer(
		"swagger-mcp-go",
		"1.0.0",
	)

	proxyEngine := proxy.New(logger.Named("proxy"), cfg.Upstream.Timeout)
	parser := parser.New(logger.Named("parser"), "")

	return &Server{
		registry:    reg,
		fetcher:     fetcher,
		logger:      logger,
		config:      cfg,
		mcpServer:   mcpServer,
		parser:      parser,
		proxyEngine: proxyEngine,
		mode:        ServerModeSTDIO, // Default mode
	}
}

// SetMode sets the server mode
func (s *Server) SetMode(mode ServerMode) {
	s.mode = mode
}

// LoadSpecFromURL loads an OpenAPI spec from URL and registers tools
func (s *Server) LoadSpecFromURL(ctx context.Context, url, serviceName string, headers map[string]string, baseURL string) error {
	// Fetch the spec
	specInfo, err := s.fetcher.FetchSpec(ctx, url, serviceName, headers, 1*time.Hour)
	if err != nil {
		return fmt.Errorf("failed to fetch spec: %w", err)
	}

	// Add to registry
	if err := s.registry.Add(specInfo); err != nil {
		return fmt.Errorf("failed to add spec to registry: %w", err)
	}

	// Parse and register tools
	return s.registerToolsFromSpec(specInfo, baseURL, headers)
}

// LoadSpecFromFile loads an OpenAPI spec from file and registers tools
func (s *Server) LoadSpecFromFile(specFile, baseURL string, headers map[string]string) error {
	// Read and parse spec file
	spec, err := s.loadSpecFile(specFile)
	if err != nil {
		return fmt.Errorf("failed to load spec file: %w", err)
	}

	// Create spec info
	specInfo := &models.SpecInfo{
		ID:          fmt.Sprintf("file:%s", specFile),
		ServiceName: "local",
		URL:         specFile,
		Spec:        spec,
		FetchedAt:   time.Now(),
		TTL:         0, // No expiration for file-based specs
		Headers:     headers,
	}

	// Add to registry
	if err := s.registry.Add(specInfo); err != nil {
		return fmt.Errorf("failed to add spec to registry: %w", err)
	}

	// Parse and register tools
	return s.registerToolsFromSpec(specInfo, baseURL, headers)
}

// registerToolsFromSpec parses a spec and registers MCP tools
func (s *Server) registerToolsFromSpec(specInfo *models.SpecInfo, baseURL string, headers map[string]string) error {
	// Set up proxy engine
	if baseURL != "" {
		s.proxyEngine.SetBaseURL(baseURL)
	} else if len(specInfo.Spec.Servers) > 0 {
		s.proxyEngine.SetBaseURL(specInfo.Spec.Servers[0].URL)
	}
	s.proxyEngine.SetHeaders(headers)

	// Parse the OpenAPI spec
	if err := s.parser.ParseSpec(specInfo.Spec); err != nil {
		return fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}

	// Register tools
	routes := s.parser.GetRoutes()
	for _, route := range routes {
		executor := s.proxyEngine.GetExecutor(&route)
		handler := s.createToolHandler(&route, executor)
		
		s.mcpServer.AddTool(route.Tool, handler)
		s.logger.Info("Registered MCP tool",
			zap.String("name", route.Tool.Name),
			zap.String("method", route.Method),
			zap.String("path", route.Path))
	}

	s.logger.Info("Successfully registered OpenAPI spec as MCP tools",
		zap.String("serviceName", specInfo.ServiceName),
		zap.Int("toolCount", len(routes)))

	return nil
}

// createToolHandler creates an MCP tool handler for a route
func (s *Server) createToolHandler(route *parser.RouteConfig, executor func(context.Context, map[string]interface{}) (*proxy.Response, error)) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		s.logger.Debug("Executing tool",
			zap.String("tool", route.Tool.Name),
			zap.String("operationID", route.OperationID))

		// Get parameters from request
		params := request.GetArguments()

		// Execute the request
		resp, err := executor(ctx, params)
		if err != nil {
			s.logger.Error("Tool execution failed",
				zap.String("tool", route.Tool.Name),
				zap.Error(err))
			return mcp.NewToolResultError(fmt.Sprintf("Request failed: %v", err)), nil
		}

		// Handle error responses
		if resp.StatusCode >= http.StatusBadRequest {
			errorMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(resp.Body))
			s.logger.Warn("Tool returned error response",
				zap.String("tool", route.Tool.Name),
				zap.Int("statusCode", resp.StatusCode))
			return mcp.NewToolResultError(errorMsg), nil
		}

		s.logger.Debug("Tool execution successful",
			zap.String("tool", route.Tool.Name),
			zap.Int("statusCode", resp.StatusCode))

		return mcp.NewToolResultText(string(resp.Body)), nil
	}
}

// Start starts the MCP server in the configured mode
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("Starting MCP server", zap.String("mode", string(s.mode)))

	switch s.mode {
	case ServerModeSTDIO:
		return s.startSTDIO(ctx)
	case ServerModeHTTP:
		return s.startHTTP(ctx)
	case ServerModeSSE:
		return s.startSSE(ctx)
	default:
		return fmt.Errorf("unsupported server mode: %s", s.mode)
	}
}

// startSTDIO starts the server in STDIO mode
func (s *Server) startSTDIO(ctx context.Context) error {
	s.logger.Info("Starting MCP server in STDIO mode")
	stdioServer := mcpserver.NewStdioServer(s.mcpServer)
	return stdioServer.Listen(ctx, os.Stdin, os.Stdout)
}

// startHTTP starts the server in HTTP mode
func (s *Server) startHTTP(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.config.MCP.Host, s.config.MCP.Port)
	s.logger.Info("Starting MCP server in HTTP mode", zap.String("address", addr))

	httpServer := mcpserver.NewStreamableHTTPServer(s.mcpServer)
	server := &http.Server{
		Addr:    addr,
		Handler: httpServer,
	}

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		s.logger.Info("Shutting down HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errChan:
		return err
	}
}

// startSSE starts the server in SSE mode
func (s *Server) startSSE(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.config.MCP.Host, s.config.MCP.Port)
	s.logger.Info("Starting MCP server in SSE mode", zap.String("address", addr))

	sseServer := mcpserver.NewSSEServer(s.mcpServer)
	server := &http.Server{
		Addr:    addr,
		Handler: sseServer,
	}

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		s.logger.Info("Shutting down SSE server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errChan:
		return err
	}
}

// Stop stops the MCP server
func (s *Server) Stop() error {
	s.logger.Info("Stopping MCP server")
	return nil
}

// loadSpecFile loads an OpenAPI specification from a file
func (s *Server) loadSpecFile(specFile string) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false // Security: disable external refs
	
	spec, err := loader.LoadFromFile(specFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI spec from file: %w", err)
	}

	// Validate the spec
	ctx := context.Background()
	if err := spec.Validate(ctx); err != nil {
		return nil, fmt.Errorf("invalid OpenAPI spec: %w", err)
	}

	return spec, nil
}

// Legacy methods for compatibility

// ListSpecs returns all registered specifications
func (s *Server) ListSpecs() []*models.SpecInfo {
	return s.registry.List()
}

// AddSpec adds a new specification
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

// RemoveSpec removes a specification
func (s *Server) RemoveSpec(serviceName string) bool {
	return s.registry.Remove(serviceName)
}

// GetStats returns statistics
func (s *Server) GetStats() map[string]interface{} {
	return s.registry.Stats()
}