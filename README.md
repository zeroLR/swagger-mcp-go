# Swagger MCP Go

`swagger-mcp-go` is a Go-based server that transforms OpenAPI/Swagger specifications into Model Context Protocol (MCP) tools. It enables AI assistants like Claude to interact with any RESTful API defined by an OpenAPI spec, allowing dynamic and intelligent API calls.

## Features

- 🚀 **Dynamic OpenAPI Import**: Parse local OpenAPI/Swagger files and convert to MCP tools
- 🔄 **Intelligent Proxying**: Route requests based on OpenAPI specifications  
- 🔗 **MCP Integration**: Full Model Context Protocol implementation
- 📊 **Multiple Transport Modes**: Support for stdio, HTTP, and SSE
- 🔧 **Command Line Interface**: Easy-to-use CLI with flexible configuration
- ⚡ **Real-time Processing**: Live conversion of API endpoints to MCP tools
- 🔐 **Authentication Framework**: JWT, OAuth2 configuration support (providers in development)
- 🛡️ **Rate Limiting**: Token bucket and sliding window algorithms 
- ⚡ **Circuit Breakers**: Fault tolerance with configurable failure thresholds
- 🔌 **Plugin System**: Extensible architecture with hooks for custom logic
- 🐳 **Docker Ready**: Multi-stage builds with distroless images and health checks
- ☸️ **Kubernetes Native**: Complete manifests with HPA, monitoring, and security policies
- 📈 **Monitoring**: Prometheus metrics, Grafana dashboards, and distributed tracing
- 🔄 **Request Transformation**: Pre/post processing hooks for request/response modification
- 🌐 **WebSocket Ready**: WebSocket implementation available (CLI integration pending)

## Quick Start

### Prerequisites
- Go 1.24+
- OpenAPI/Swagger specification file (JSON or YAML)

### Installation

```bash
# Clone the repository
git clone https://github.com/zeroLR/swagger-mcp-go.git
cd swagger-mcp-go

# Build the application
go build -o bin/swagger-mcp-go ./cmd/server
```

### Basic Usage

```bash
# Show help
./bin/swagger-mcp-go --help

# Run with OpenAPI spec (stdio mode for Claude Desktop)
./bin/swagger-mcp-go --swagger-file=examples/petstore.json

# Run HTTP server mode
./bin/swagger-mcp-go --swagger-file=examples/petstore.json --mode=http

# Use custom base URL for API calls
./bin/swagger-mcp-go --swagger-file=examples/petstore.json --base-url=https://api.example.com
```

## Transport Modes

### STDIO Mode (Default)
Perfect for Claude Desktop integration. Communicates via stdin/stdout using JSON-RPC.

```bash
./bin/swagger-mcp-go --swagger-file=petstore.json
```

### HTTP Mode
Runs an HTTP server for MCP over HTTP transport.

```bash
./bin/swagger-mcp-go --swagger-file=petstore.json --mode=http
```

### SSE Mode  
Runs a Server-Sent Events server for real-time MCP communication.

```bash
./bin/swagger-mcp-go --swagger-file=petstore.json --mode=sse
```

### WebSocket Mode
WebSocket support is available through configuration files. Create a config file with WebSocket settings:

```yaml
# config.yaml
websocket:
  enabled: true
  readBufferSize: 1024
  writeBufferSize: 1024
  pingInterval: 30s
```

```bash
./bin/swagger-mcp-go --swagger-file=petstore.json --config=config.yaml
```

## Claude Desktop Integration

Add the following to your Claude Desktop MCP configuration:

```json
{
  "mcpServers": {
    "petstore-api": {
      "command": "/path/to/swagger-mcp-go",
      "args": [
        "--swagger-file=/path/to/petstore.json",
        "--base-url=https://petstore.swagger.io/v2"
      ]
    }
  }
}
```

## How It Works

1. **Parse OpenAPI Spec**: Reads your OpenAPI/Swagger specification
2. **Generate MCP Tools**: Converts each API endpoint into an MCP tool
3. **Handle Requests**: Proxies tool calls to your actual API endpoints
4. **Return Results**: Formats responses for the AI assistant

### Example Transformation

Given this OpenAPI endpoint:
```yaml
paths:
  /pet/{petId}:
    get:
      operationId: getPetById
      summary: Find pet by ID
      parameters:
        - name: petId
          in: path
          required: true
          schema:
            type: integer
```

Creates this MCP tool:
- **Name**: `getPetById`
- **Description**: "Find pet by ID"
- **Parameters**: `petId` (integer, required)

## Configuration

Create a `config.yaml` file for advanced configuration:

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

# Metrics and monitoring
metrics:
  enabled: true
  path: "/metrics"

# Distributed tracing
tracing:
  enabled: false
  endpoint: "http://jaeger:14268/api/traces"
  serviceName: "swagger-mcp-go"

# Upstream API configuration
upstream:
  timeout: 30s
  retryCount: 3
  retryDelay: 1s
  circuitBreaker:
    threshold: 5
    timeout: "60s"

# Authentication configuration
auth:
  jwt:
    jwksURL: "https://your-auth-provider.com/.well-known/jwks.json"
    issuer: "your-issuer"
    audience: "your-audience"
  oauth2:
    tokenURL: "https://auth.example.com/oauth/token"
    clientID: "${OAUTH_CLIENT_ID}"
    clientSecret: "${OAUTH_CLIENT_SECRET}"

# OpenAPI specification settings
specs:
  defaultTTL: "1h"
  maxSize: "10MB"

# Policies configuration
policies:
  rateLimit:
    enabled: false
    requestsPerMinute: 100
  cors:
    enabled: true
    allowOrigins: ["*"]
    allowMethods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"]

# WebSocket configuration (when implemented)
websocket:
  enabled: true
  readBufferSize: 1024
  writeBufferSize: 1024
  pingInterval: 30s
  maxMessageSize: 1048576
```

## Command Line Options

```
Usage: swagger-mcp-go [OPTIONS]

OPTIONS:
  --swagger-file=FILE    Path to OpenAPI/Swagger specification file (required)
  --config=FILE          Path to configuration file (optional)
  --mode=MODE            Server mode: stdio, http, or sse (default: stdio)
  --base-url=URL         Base URL for upstream API (overrides spec servers)
  --version              Show version information
  --help                 Show this help message
```

**Note**: Advanced features like authentication, rate limiting, WebSocket support, and plugins are configured via the configuration file (see Configuration section).

## Examples

### Pet Store API
The repository includes a sample Pet Store OpenAPI specification:

```bash
./bin/swagger-mcp-go --swagger-file=examples/petstore.json --base-url=https://petstore.swagger.io/v2
```

This creates the following MCP tools:
- `addPet` - Add a new pet to the store
- `findPetsByStatus` - Find pets by status
- `getPetById` - Get a pet by ID
- `deletePet` - Delete a pet

### Custom API
Use your own OpenAPI specification:

```bash
./bin/swagger-mcp-go --swagger-file=my-api.json --base-url=https://my-api.com/v1
```

## Advanced Features

### Authentication

swagger-mcp-go supports multiple authentication methods via configuration:

#### JWT/Bearer Token Authentication
```yaml
# config.yaml
auth:
  jwt:
    jwksURL: "https://your-auth-provider.com/.well-known/jwks.json"
    issuer: "your-issuer"
    audience: "your-audience"
```

#### OAuth2 Client Credentials
```yaml
# config.yaml  
auth:
  oauth2:
    tokenURL: "https://auth.example.com/oauth/token"
    clientID: "${OAUTH_CLIENT_ID}"
    clientSecret: "${OAUTH_CLIENT_SECRET}"
```

### Rate Limiting

Configure rate limiting to protect your APIs:

```yaml
# config.yaml
policies:
  rateLimit:
    enabled: true
    requestsPerMinute: 100
```

### Circuit Breakers

Built-in fault tolerance with configurable circuit breakers:

```yaml
# config.yaml
upstream:
  circuitBreaker:
    threshold: 5
    timeout: "60s"
```

### WebSocket Support

Enable WebSocket transport for real-time communication:

```yaml
# config.yaml
websocket:
  enabled: true
  readBufferSize: 1024
  writeBufferSize: 1024
  pingInterval: 30s
  maxMessageSize: 1048576
```

### Plugin System

The plugin system is implemented and supports various plugin types. Plugins are configured via the configuration file and loaded from a specified directory.

## Docker Deployment

### Quick Start with Docker

```bash
# Build the image
docker build -t swagger-mcp-go:latest .

# Run with HTTP mode
docker run -p 8080:8080 \
  -v $(pwd)/examples:/examples:ro \
  swagger-mcp-go:latest \
  --swagger-file=/examples/petstore.json \
  --mode=http \
  --base-url=https://petstore.swagger.io/v2
```

### Docker Compose

Deploy with full monitoring stack:

```bash
# Start all services
./scripts/deploy-docker.sh up

# Check status
./scripts/deploy-docker.sh status

# View logs
./scripts/deploy-docker.sh logs

# Stop services
./scripts/deploy-docker.sh down
```

Includes:
- Multiple swagger-mcp-go instances
- NGINX load balancer  
- Prometheus monitoring
- Grafana dashboards
- Jaeger tracing
- Redis for rate limiting

## Kubernetes Deployment

### Quick Start with Kubernetes

```bash
# Deploy to cluster
./scripts/deploy-k8s.sh deploy

# Check status
kubectl get pods -n swagger-mcp-go

# Access via port-forward
kubectl port-forward svc/swagger-mcp-go-service 8080:8080 -n swagger-mcp-go

# Clean up
./scripts/deploy-k8s.sh delete
```

### Features

- **Horizontal Pod Autoscaling**: Automatic scaling based on CPU/memory
- **Pod Disruption Budgets**: High availability during updates
- **Network Policies**: Security isolation
- **Service Monitoring**: Prometheus integration
- **Health Checks**: Liveness and readiness probes
- **Resource Limits**: Memory and CPU constraints
- **Security Context**: Non-root containers with read-only filesystem

## Monitoring and Observability

### Prometheus Metrics

Available metrics:
- HTTP request duration and count
- Circuit breaker status
- Rate limiting metrics
- Plugin execution metrics
- WebSocket connection metrics

### Grafana Dashboards

Pre-configured dashboards for:
- Application overview
- Request performance
- Error rates and latencies
- Circuit breaker status
- Rate limiting statistics

### Distributed Tracing

Jaeger integration provides:
- Request flow visualization
- Performance bottleneck identification
- Error propagation tracking
- Cross-service dependencies

## Project Structure

```
.
├── cmd/server/           # Main application entry point
├── internal/
│   ├── auth/            # Authentication providers
│   ├── circuitbreaker/  # Circuit breaker implementation
│   ├── config/          # Configuration management
│   ├── hooks/           # Request/response transformation hooks
│   ├── mcp/             # MCP server implementation
│   ├── models/          # Data models
│   ├── parser/          # OpenAPI specification parser
│   ├── plugins/         # Plugin system
│   ├── proxy/           # HTTP proxy engine
│   ├── ratelimit/       # Rate limiting implementation
│   ├── registry/        # Specification registry
│   ├── specs/           # Specification fetcher
│   └── websocket/       # WebSocket server
├── examples/            # Example OpenAPI specifications
├── configs/             # Configuration files
├── docs/                # Documentation
│   ├── DOCKER.md        # Docker deployment guide
│   └── KUBERNETES.md    # Kubernetes deployment guide
├── k8s/                 # Kubernetes manifests
├── scripts/             # Deployment scripts
├── docker-compose.yml   # Docker Compose configuration
└── Dockerfile          # Docker image definition
```

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Roadmap

- [x] Core OpenAPI to MCP transformation
- [x] Multiple transport modes (stdio, http, sse)
- [x] Command-line interface
- [x] Basic proxy functionality
- [x] Request/response transformation hooks
- [x] Rate limiting and circuit breakers
- [x] Plugin system architecture
- [x] Docker containerization
- [x] Kubernetes deployment manifests
- [x] Monitoring and observability (Prometheus, Grafana, Jaeger)
- [x] Security features (RBAC, Network Policies, Security Context)
- [x] High availability features (HPA, PDB, Health Checks)
- [x] Configuration management with environment variable support
- [~] Authentication framework (JWT, OAuth2 config structure ready)
- [~] WebSocket transport (implementation ready, integration pending)
- [ ] Complete authentication provider implementations (Bearer, API Key, Basic Auth)
- [ ] OAuth2 flows (Authorization Code, Client Credentials)
- [ ] API versioning and schema evolution