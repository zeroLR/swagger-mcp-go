# OpenAPI-driven Proxy Server with MCP Integration

A dynamic OpenAPI-driven proxy server that operates as a Model Context Protocol (MCP) server. The system dynamically imports remote Swagger/OpenAPI specifications by URL, registers proxied routes with customizable headers, supports Server-Sent Events (SSE) pass-through, and exposes management capabilities through both HTTP Admin APIs and MCP tools.

## Features

- 🚀 **Dynamic OpenAPI Import**: Fetch and register specs from remote URLs
- 🔄 **Intelligent Proxying**: Route requests based on OpenAPI specifications  
- 🔗 **MCP Integration**: Remote management via Model Context Protocol
- 📊 **Observability**: Prometheus metrics, structured logging, tracing support
- 🔐 **Security**: Pluggable authentication (Basic, Bearer JWT, OAuth2)
- 🔧 **Extensibility**: Hook system for custom processing
- ⚡ **Performance**: In-memory caching with TTL, circuit breakers, retries

## Quick Start

### Prerequisites
- Go 1.22+
- Optional: Prometheus for metrics collection
- Optional: Jaeger for tracing

### Installation

```bash
# Clone the repository
git clone https://github.com/zeroLR/swagger-mcp-go.git
cd swagger-mcp-go

# Build the application
go build -o bin/swagger-mcp-go ./cmd/server

# Run the server
./bin/swagger-mcp-go
```

### Configuration

Create a `config.yaml` file or use the provided example in `configs/config.yaml`:

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
```

## Usage

### Health Check

```bash
curl http://localhost:8080/health
```

### Admin API

#### List Specifications
```bash
curl http://localhost:8080/admin/specs
```

#### Get Statistics
```bash
curl http://localhost:8080/admin/stats
```

#### Remove Specification
```bash
curl -X DELETE http://localhost:8080/admin/specs/my-service
```

### Metrics

Prometheus metrics are available at:
```bash
curl http://localhost:8080/metrics
```

### MCP Integration

The server exposes MCP tools for remote management:

- `listSpecs` - List all registered OpenAPI specifications
- `addSpec` - Add a new specification from URL
- `refreshSpec` - Force refresh of an existing specification  
- `removeSpec` - Remove a specification
- `inspectRoute` - Inspect route configuration
- `getStats` - Get performance statistics
- `enableAuthPolicy` - Enable authentication for a service
- `disableAuthPolicy` - Disable authentication for a service

## Project Structure

```
.
├── cmd/server/           # Main application entry point
├── internal/
│   ├── auth/            # Authentication providers
│   ├── config/          # Configuration management
│   ├── handlers/        # HTTP handlers
│   ├── mcp/             # MCP server implementation
│   ├── middleware/      # HTTP middleware
│   ├── models/          # Data models
│   ├── proxy/           # Proxy engine
│   ├── registry/        # Specification registry
│   ├── specs/           # Specification fetcher
│   └── upstream/        # Upstream client
├── pkg/
│   ├── health/          # Health checks
│   ├── hooks/           # Extension hooks
│   ├── metrics/         # Metrics collection
│   └── tracing/         # Distributed tracing
├── configs/             # Configuration files
├── docs/                # Documentation
└── examples/            # Usage examples
```

## Architecture

The system is built with a modular architecture:

- **SpecFetcher**: Fetches and validates OpenAPI specs from URLs
- **SpecRegistry**: In-memory cache with TTL management
- **RouteBinder**: Dynamic route registration from specs
- **ProxyEngine**: Request forwarding with customization
- **MCPServerAdapter**: MCP protocol implementation
- **AdminAPI**: HTTP management interface

For detailed architecture information, see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Development

### Building

```bash
go build -o bin/swagger-mcp-go ./cmd/server
```

### Testing

```bash
go test ./...
```

### Running with Development Config

```bash
# Copy example config
cp configs/config.yaml config.yaml

# Edit configuration as needed
vim config.yaml

# Run server
go run ./cmd/server
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

- [ ] Complete MCP tool implementations
- [ ] Add dynamic route binding from OpenAPI specs
- [ ] Implement authentication providers
- [ ] Add request/response transformation hooks
- [ ] Circuit breaker and retry mechanisms
- [ ] OpenTelemetry tracing integration
- [ ] Rate limiting support
- [ ] WebSocket and SSE pass-through
- [ ] Plugin system for custom extensions
- [ ] Kubernetes deployment manifests