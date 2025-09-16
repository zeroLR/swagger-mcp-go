# OpenAPI-driven MCP Server

Transform any OpenAPI/Swagger definition into a fully-featured Model Context Protocol (MCP) server. The system dynamically parses OpenAPI specifications and converts endpoints into MCP tools that can be used with AI assistants like Claude Desktop.

## Features

- 🚀 **Dynamic OpenAPI Import**: Parse local OpenAPI/Swagger files and convert to MCP tools
- 🔄 **Intelligent Proxying**: Route requests based on OpenAPI specifications  
- 🔗 **MCP Integration**: Full Model Context Protocol implementation
- 📊 **Multiple Transport Modes**: Support for stdio, HTTP, and SSE
- 🔧 **Command Line Interface**: Easy-to-use CLI with flexible configuration
- ⚡ **Real-time Processing**: Live conversion of API endpoints to MCP tools

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

upstream:
  timeout: 30s
  retryCount: 3
  retryDelay: 1s
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

## Project Structure

```
.
├── cmd/server/           # Main application entry point
├── internal/
│   ├── config/          # Configuration management
│   ├── mcp/             # MCP server implementation
│   ├── models/          # Data models
│   ├── parser/          # OpenAPI specification parser
│   ├── proxy/           # HTTP proxy engine
│   ├── registry/        # Specification registry
│   └── specs/           # Specification fetcher
├── examples/            # Example OpenAPI specifications
└── configs/             # Configuration files
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
- [ ] Authentication support (Bearer, API Key, OAuth2)
- [ ] Request/response transformation hooks
- [ ] Rate limiting and circuit breakers
- [ ] WebSocket support
- [ ] Plugin system for custom extensions
- [ ] Docker containerization
- [ ] Kubernetes deployment manifests