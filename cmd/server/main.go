package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/zeroLR/swagger-mcp-go/internal/config"
	"github.com/zeroLR/swagger-mcp-go/internal/mcp"
	"github.com/zeroLR/swagger-mcp-go/internal/registry"
	"github.com/zeroLR/swagger-mcp-go/internal/specs"
)

var (
	// CLI flags
	swaggerFile = flag.String("swagger-file", "", "Path to OpenAPI/Swagger specification file")
	configFile  = flag.String("config", "", "Path to configuration file")
	mode        = flag.String("mode", "stdio", "Server mode: stdio, http, or sse")
	baseURL     = flag.String("base-url", "", "Base URL for upstream API (overrides spec servers)")
	showVersion = flag.Bool("version", false, "Show version information")
	showHelp    = flag.Bool("help", false, "Show help information")
)

const version = "1.0.0"

func main() {
	flag.Parse()

	handleBasicFlags()

	cfg := mustLoadConfig()
	normalizeMode(cfg)

	logger := mustInitLogger(cfg)
	defer logger.Sync()

	logger.Info("Starting swagger-mcp-go",
		zap.String("version", version),
		zap.String("mode", *mode),
		zap.String("swaggerFile", *swaggerFile))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg, fetcher := initCoreComponents(ctx, cfg, logger)
	mcpServer := initMCPServer(ctx, cfg, reg, fetcher, logger)

	httpServer := maybeStartHTTPServer(cfg, logger, reg)

	waitForShutdownSignal(logger)
	performShutdown(cancel, httpServer, mcpServer, logger)
}

// handleBasicFlags processes help, version and required flags
func handleBasicFlags() {
	if *showHelp {
		printHelp()
		os.Exit(0)
	}
	if *showVersion {
		fmt.Printf("swagger-mcp-go version %s\n", version)
		os.Exit(0)
	}
	if *swaggerFile == "" {
		fmt.Fprintf(os.Stderr, "Error: --swagger-file is required\n")
		printHelp()
		os.Exit(1)
	}
}

// mustLoadConfig loads configuration or exits on failure
func mustLoadConfig() *config.Config {
	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	return cfg
}

// normalizeMode validates and applies mode specific adjustments
func normalizeMode(cfg *config.Config) {
	if *mode != "stdio" {
		switch *mode {
		case "http", "sse":
		default:
			log.Fatalf("Invalid mode: %s. Must be one of: stdio, http, sse", *mode)
		}
	}
	if *mode == "stdio" { // minimize logging
		cfg.Logging.Level = "error"
	}
}

// mustInitLogger initializes the logger or exits on failure
func mustInitLogger(cfg *config.Config) *zap.Logger {
	logger, err := initLogger(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	return logger
}

// initCoreComponents creates registry and spec fetcher and starts cleanup
func initCoreComponents(ctx context.Context, cfg *config.Config, logger *zap.Logger) (*registry.Registry, *specs.Fetcher) {
	reg := registry.New(logger.Named("registry"))
	maxSize := int64(10 * 1024 * 1024)
	fetcher := specs.New(logger.Named("specs"), cfg.Upstream.Timeout, maxSize)
	reg.StartCleanup(ctx, 5*time.Minute)
	return reg, fetcher
}

// initMCPServer loads spec and starts MCP server
func initMCPServer(ctx context.Context, cfg *config.Config, reg *registry.Registry, fetcher *specs.Fetcher, logger *zap.Logger) *mcp.Server {
	mcpServer := mcp.NewServer(logger.Named("mcp"), cfg, reg, fetcher)
	mcpServer.SetMode(mcp.ServerMode(*mode))
	headers := make(map[string]string)
	if err := mcpServer.LoadSpecFromFile(*swaggerFile, *baseURL, headers); err != nil {
		logger.Fatal("Failed to load OpenAPI spec", zap.Error(err))
	}
	go func() {
		if err := mcpServer.Start(ctx); err != nil {
			logger.Error("MCP server error", zap.Error(err))
		}
	}()
	return mcpServer
}

// maybeStartHTTPServer starts HTTP server if mode requires it
func maybeStartHTTPServer(cfg *config.Config, logger *zap.Logger, reg *registry.Registry) *http.Server {
	if *mode == "stdio" {
		return nil
	}
	router := setupRouter(cfg, logger.Named("http"), reg)
	httpServer := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}
	go func() {
		logger.Info("Starting HTTP server", zap.String("addr", httpServer.Addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP server error", zap.Error(err))
		}
	}()
	return httpServer
}

// waitForShutdownSignal blocks until an interrupt signal is received
func waitForShutdownSignal(logger *zap.Logger) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	logger.Info("Shutting down server...")
}

// performShutdown gracefully stops servers and background processes
func performShutdown(cancel context.CancelFunc, httpServer *http.Server, mcpServer *mcp.Server, logger *zap.Logger) {
	cancel()
	if httpServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("HTTP server shutdown error", zap.Error(err))
		}
	}
	if err := mcpServer.Stop(); err != nil {
		logger.Error("MCP server stop error", zap.Error(err))
	}
	logger.Info("Server stopped")
}

func printHelp() {
	fmt.Printf(`swagger-mcp-go - Transform OpenAPI/Swagger specs into MCP servers

Usage: swagger-mcp-go [OPTIONS]

OPTIONS:
  --swagger-file=FILE    Path to OpenAPI/Swagger specification file (required)
  --config=FILE          Path to configuration file (optional)
  --mode=MODE            Server mode: stdio, http, or sse (default: stdio)
  --base-url=URL         Base URL for upstream API (overrides spec servers)
  --version              Show version information
  --help                 Show this help message

MODES:
  stdio                  Communicate via stdin/stdout (for Claude Desktop)
  http                   Run HTTP server for MCP over HTTP
  sse                    Run SSE server for MCP over Server-Sent Events

EXAMPLES:
  # Run in stdio mode (default, for Claude Desktop)
  swagger-mcp-go --swagger-file=petstore.json

  # Run HTTP server mode
  swagger-mcp-go --swagger-file=petstore.json --mode=http

  # Use custom base URL
  swagger-mcp-go --swagger-file=petstore.json --base-url=https://api.example.com

  # Use custom config
  swagger-mcp-go --swagger-file=petstore.json --config=myconfig.yaml
`)
}

func initLogger(cfg *config.Config) (*zap.Logger, error) {
	var zapConfig zap.Config

	if cfg.Logging.Format == "json" {
		zapConfig = zap.NewProductionConfig()
	} else {
		zapConfig = zap.NewDevelopmentConfig()
	}

	// Set log level
	switch cfg.Logging.Level {
	case "debug":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	return zapConfig.Build()
}

func setupRouter(cfg *config.Config, logger *zap.Logger, reg *registry.Registry) *gin.Engine {
	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()

	// Middleware
	router.Use(gin.Recovery())
	router.Use(ginLogger(logger))

	// CORS middleware
	if cfg.Policies.CORS.Enabled {
		router.Use(corsMiddleware(cfg))
	}

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now(),
		})
	})

	// Metrics endpoint
	if cfg.Metrics.Enabled {
		router.GET(cfg.Metrics.Path, gin.WrapH(promhttp.Handler()))
	}

	// Admin API
	admin := router.Group("/admin")
	{
		admin.GET("/specs", listSpecsHandler(reg))
		admin.POST("/specs", addSpecHandler(reg, logger))
		admin.PUT("/specs/:service/refresh", refreshSpecHandler(reg, logger))
		admin.DELETE("/specs/:service", removeSpecHandler(reg))
		admin.GET("/stats", statsHandler(reg))
	}

	// Proxy routes will be dynamically registered here
	apis := router.Group("/apis")
	{
		// Dynamic routes will be added by the route binder
		apis.Any("/*path", func(c *gin.Context) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "No service registered for this path",
			})
		})
	}

	return router
}

func ginLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()

		if raw != "" {
			path = path + "?" + raw
		}

		logger.Info("HTTP request",
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("status", statusCode),
			zap.Duration("latency", latency),
			zap.String("clientIP", clientIP),
		)
	}
}

func corsMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Simple CORS implementation
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// Admin API handlers

func listSpecsHandler(reg *registry.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		specs := reg.List()
		c.JSON(http.StatusOK, gin.H{
			"specs": specs,
		})
	}
}

func addSpecHandler(reg *registry.Registry, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			URL         string            `json:"url" binding:"required"`
			ServiceName string            `json:"serviceName" binding:"required"`
			TTL         string            `json:"ttl"`
			Headers     map[string]string `json:"headers"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// TODO: Implement spec fetching and registration
		c.JSON(http.StatusNotImplemented, gin.H{
			"error": "Spec registration not yet implemented in HTTP API",
		})
	}
}

func refreshSpecHandler(reg *registry.Registry, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		_ = c.Param("service")

		// TODO: Implement spec refresh
		c.JSON(http.StatusNotImplemented, gin.H{
			"error": "Spec refresh not yet implemented in HTTP API",
		})
	}
}

func removeSpecHandler(reg *registry.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		serviceName := c.Param("service")

		removed := reg.Remove(serviceName)
		if !removed {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Service not found",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
		})
	}
}

func statsHandler(reg *registry.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		stats := reg.Stats()
		c.JSON(http.StatusOK, stats)
	}
}
