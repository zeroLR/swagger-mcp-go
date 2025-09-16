package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/zeroLR/swagger-mcp-go/internal/config"
	"github.com/zeroLR/swagger-mcp-go/internal/mcp"
	"github.com/zeroLR/swagger-mcp-go/internal/registry"
	"github.com/zeroLR/swagger-mcp-go/internal/specs"
)

func main() {
	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize logger
	logger, err := initLogger(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	logger.Info("Starting swagger-mcp-go server",
		zap.String("version", "1.0.0"),
		zap.Int("port", cfg.Server.Port),
		zap.Bool("mcpEnabled", cfg.MCP.Enabled))

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize components
	reg := registry.New(logger.Named("registry"))
	
	// Parse max size for fetcher
	maxSize := int64(10 * 1024 * 1024) // 10MB default
	fetcher := specs.New(logger.Named("specs"), cfg.Upstream.Timeout, maxSize)

	// Start registry cleanup
	reg.StartCleanup(ctx, 5*time.Minute)

	// Initialize Gin router
	router := setupRouter(cfg, logger.Named("http"), reg)

	// Create HTTP server
	httpServer := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start MCP server if enabled
	var mcpServer *mcp.Server
	if cfg.MCP.Enabled {
		mcpServer = mcp.NewServer(logger.Named("mcp"), reg, fetcher)
		go func() {
			if err := mcpServer.Start(ctx); err != nil {
				logger.Error("MCP server error", zap.Error(err))
			}
		}()
	}

	// Start HTTP server
	go func() {
		logger.Info("Starting HTTP server", zap.String("addr", httpServer.Addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP server error", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down server...")

	// Cancel context to stop background processes
	cancel()

	// Shutdown HTTP server with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", zap.Error(err))
	}

	// Stop MCP server
	if mcpServer != nil {
		if err := mcpServer.Stop(); err != nil {
			logger.Error("MCP server stop error", zap.Error(err))
		}
	}

	logger.Info("Server stopped")
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
			"status": "healthy",
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