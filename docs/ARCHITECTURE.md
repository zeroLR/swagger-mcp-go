# OpenAPI-driven Proxy Server with MCP Integration

## Executive Summary

This project implements a dynamic OpenAPI-driven proxy server that operates as a Model Context Protocol (MCP) server. The system dynamically imports remote Swagger/OpenAPI specifications by URL, registers proxied routes with customizable headers, supports Server-Sent Events (SSE) pass-through, and exposes management capabilities through both HTTP Admin APIs and MCP tools.

### Key Capabilities
- **Dynamic Spec Import**: Fetch and register OpenAPI specs from remote URLs
- **Intelligent Proxying**: Route requests to upstream services based on spec definitions
- **MCP Integration**: Expose management tools via mark3labs/mcp-go for remote administration
- **Observability**: Comprehensive metrics, tracing, and structured logging
- **Security**: Pluggable authentication with support for Basic, Bearer (JWT), and OAuth2
- **Extensibility**: Hook system for custom pre/post-request processing

## Architecture Overview

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   MCP Client    │    │  Admin Client   │    │  Proxy Client   │
│   (Remote)      │    │   (HTTP API)    │    │   (HTTP API)    │
└─────────┬───────┘    └─────────┬───────┘    └─────────┬───────┘
          │                      │                      │
          │ MCP Protocol         │ HTTP                 │ HTTP
          │                      │                      │
┌─────────▼──────────────────────▼──────────────────────▼───────┐
│                     Swagger MCP Go Server                     │
│ ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐ │
│ │  MCP Server     │ │   Admin API     │ │  Proxy Engine   │ │
│ │   Adapter       │ │                 │ │                 │ │
│ └─────────────────┘ └─────────────────┘ └─────────────────┘ │
│ ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐ │
│ │  Spec Registry  │ │  Auth Provider  │ │ Upstream Client │ │
│ │   (In-Memory)   │ │      Set        │ │ (Retry/Circuit) │ │
│ └─────────────────┘ └─────────────────┘ └─────────────────┘ │
│ ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐ │
│ │ Metrics Emitter │ │    Tracer       │ │  Hook Manager   │ │
│ │  (Prometheus)   │ │ (OpenTelemetry) │ │                 │ │
│ └─────────────────┘ └─────────────────┘ └─────────────────┘ │
└─────────────────────────────┬─────────────────────────────────┘
                              │
                              │ HTTP Requests
                              │
┌─────────────────────────────▼─────────────────────────────────┐
│                    Upstream Services                          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐           │
│  │ Service A   │  │ Service B   │  │ Service C   │    ...    │
│  │(OpenAPI 3.0)│  │(Swagger 2.0)│  │(OpenAPI 3.1)│           │
│  └─────────────┘  └─────────────┘  └─────────────┘           │
└───────────────────────────────────────────────────────────────┘
```

## Core Components

### 1. SpecFetcher
**Purpose**: Fetch and validate OpenAPI specifications from remote URLs
- HTTP client with timeout and retry logic
- Support for OpenAPI 3.0/3.1 and Swagger 2.0
- JSON Schema validation
- Content negotiation (JSON/YAML)

### 2. SpecRegistry
**Purpose**: In-memory cache for OpenAPI specifications with TTL management
- Thread-safe access with RWMutex
- Configurable TTL per spec
- Lazy refresh on expiry
- Event emission for spec changes

### 3. RouteBinder
**Purpose**: Dynamic route registration from OpenAPI specifications
- Parse operation paths and methods
- Generate Gin route handlers
- Namespace support (/apis/{serviceName}/...)
- Parameter validation and binding

### 4. ProxyEngine
**Purpose**: Core request forwarding with customizable processing
- Request/response transformation
- Header manipulation via policies
- Content-type aware proxying
- SSE pass-through support

### 5. UpstreamClient
**Purpose**: Resilient HTTP client for upstream communication
- Configurable timeouts and retries
- Circuit breaker pattern
- Connection pooling
- Request/response tracing

### 6. AuthProviderSet
**Purpose**: Pluggable authentication system
- **Basic Auth**: Username/password validation
- **Bearer Token**: JWT validation (RS256 via JWKS, optional HS256)
- **OAuth2**: client_credentials and authorization_code + PKCE flows
- Policy-based auth enforcement per route

### 7. HeaderPolicyEngine
**Purpose**: Customizable header management
- Request header injection/modification
- Response header filtering
- Security headers enforcement
- Custom header policies per service

### 8. MCPServerAdapter
**Purpose**: MCP server implementation using mark3labs/mcp-go
- Tool registration and execution
- Resource streaming for events
- Bidirectional communication with MCP clients

### 9. AdminAPI
**Purpose**: HTTP-based management interface
- Spec management endpoints
- Runtime configuration
- Health checks and diagnostics
- Metrics exposure

### 10. MetricsEmitter
**Purpose**: Prometheus metrics collection
- Request/response metrics
- Upstream service health
- Cache hit/miss ratios
- Custom business metrics

### 11. Tracer
**Purpose**: OpenTelemetry distributed tracing
- HTTP server instrumentation
- Upstream client tracing
- Custom span creation
- Trace correlation

### 12. HookManager
**Purpose**: Extension point system
- Pre-request hooks for validation/transformation
- Post-response hooks for monitoring/logging
- Plugin architecture for custom logic

## Data Models

### SpecInfo
```go
type SpecInfo struct {
    ID          string                 `json:"id"`
    ServiceName string                 `json:"serviceName"`
    URL         string                 `json:"url"`
    Spec        *openapi3.T           `json:"spec"`
    FetchedAt   time.Time             `json:"fetchedAt"`
    TTL         time.Duration         `json:"ttl"`
    Headers     map[string]string     `json:"headers"`
    AuthPolicy  *AuthPolicy           `json:"authPolicy,omitempty"`
}
```

### ProxyRequest
```go
type ProxyRequest struct {
    Method      string            `json:"method"`
    Path        string            `json:"path"`
    Headers     map[string]string `json:"headers"`
    Body        []byte            `json:"body,omitempty"`
    ServiceName string            `json:"serviceName"`
    Operation   *openapi3.Operation `json:"operation"`
}
```

### AuthPolicy
```go
type AuthPolicy struct {
    Type     AuthType          `json:"type"`     // basic, bearer, oauth2
    Config   AuthConfig        `json:"config"`
    Required bool              `json:"required"`
    Scopes   []string          `json:"scopes,omitempty"`
}
```

## MCP Integration

### Tools Available

1. **listSpecs**
   - **Input**: `{}`
   - **Output**: `{specs: SpecInfo[]}`
   - **Purpose**: List all registered OpenAPI specifications

2. **addSpec**
   - **Input**: `{url: string, serviceName: string, ttl?: duration, headers?: map[string]string}`
   - **Output**: `{success: boolean, spec?: SpecInfo, error?: string}`
   - **Purpose**: Add a new OpenAPI specification from URL

3. **refreshSpec**
   - **Input**: `{serviceName: string}`
   - **Output**: `{success: boolean, spec?: SpecInfo, error?: string}`
   - **Purpose**: Force refresh of an existing specification

4. **removeSpec**
   - **Input**: `{serviceName: string}`
   - **Output**: `{success: boolean, error?: string}`
   - **Purpose**: Remove a specification and its routes

5. **inspectRoute**
   - **Input**: `{path: string, method?: string}`
   - **Output**: `{routes: RouteInfo[], serviceName?: string}`
   - **Purpose**: Inspect route configuration and mapping

6. **getStats**
   - **Input**: `{serviceName?: string}`
   - **Output**: `{stats: ServiceStats}`
   - **Purpose**: Retrieve performance and usage statistics

7. **enableAuthPolicy**
   - **Input**: `{serviceName: string, policy: AuthPolicy}`
   - **Output**: `{success: boolean, error?: string}`
   - **Purpose**: Enable authentication for a service

8. **disableAuthPolicy**
   - **Input**: `{serviceName: string}`
   - **Output**: `{success: boolean, error?: string}`
   - **Purpose**: Disable authentication for a service

### Resources

1. **resource:openapi.spec/{serviceName}**
   - **Purpose**: Stream spec change events
   - **Events**: spec.added, spec.updated, spec.removed, spec.error

## Configuration

### Example Configuration (config.yaml)
```yaml
server:
  host: "0.0.0.0"
  port: 8080
  readTimeout: 30s
  writeTimeout: 30s

mcp:
  enabled: true
  host: "0.0.0.0"
  port: 8081

logging:
  level: "info"
  format: "json"

metrics:
  enabled: true
  path: "/metrics"

tracing:
  enabled: true
  endpoint: "http://jaeger:14268/api/traces"
  serviceName: "swagger-mcp-go"

upstream:
  timeout: 30s
  retryCount: 3
  retryDelay: 1s
  circuitBreaker:
    threshold: 5
    timeout: 60s

auth:
  jwt:
    jwksURL: "https://example.com/.well-known/jwks.json"
    issuer: "https://example.com"
    audience: "api"
  oauth2:
    tokenURL: "https://example.com/oauth/token"
    clientID: "${OAUTH2_CLIENT_ID}"
    clientSecret: "${OAUTH2_CLIENT_SECRET}"

specs:
  defaultTTL: "1h"
  maxSize: "10MB"
  
policies:
  rateLimit:
    enabled: false
    requestsPerMinute: 100
  cors:
    enabled: true
    allowOrigins: ["*"]
    allowMethods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"]
```

## Security Considerations

1. **Input Validation**: All incoming requests validated against OpenAPI schemas
2. **Size Limits**: Configurable limits on spec size and request bodies
3. **Timeouts**: Prevent resource exhaustion with configurable timeouts
4. **Authentication**: Multi-tier auth with optional enforcement
5. **Headers**: Security headers automatically added
6. **Circuit Breaker**: Prevent cascade failures to upstream services
7. **Panic Recovery**: Graceful error handling and recovery

## Performance & Scalability

1. **Caching**: In-memory spec caching with TTL
2. **Connection Pooling**: HTTP client reuse
3. **Concurrent Safety**: RWMutex protection for shared state
4. **Metrics**: Performance monitoring and alerting
5. **Graceful Shutdown**: Clean resource cleanup
6. **Resource Limits**: Configurable limits on memory and connections

## Extensibility

1. **Hook System**: Pre/post-request extension points
2. **Plugin Architecture**: Dynamic loading of custom logic
3. **Custom Auth Providers**: Pluggable authentication
4. **Custom Metrics**: Business-specific metric collection
5. **Custom Headers**: Service-specific header policies