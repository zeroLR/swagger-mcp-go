# Examples and Usage Guide

This directory contains example OpenAPI specifications and usage examples for swagger-mcp-go.

## Example APIs

### 1. Pet Store API (`petstore.json`)
A simple pet management API demonstrating basic CRUD operations.

**Generated MCP Tools:**
- `addPet` - Add a new pet to the store
- `findPetsByStatus` - Find pets by status
- `getPetById` - Get a pet by ID
- `deletePet` - Delete a pet

**Usage:**
```bash
./bin/swagger-mcp-go --swagger-file=examples/petstore.json --base-url=https://petstore.swagger.io/v2
```

### 2. JSON Placeholder API (`jsonplaceholder.json`)
A comprehensive fake REST API for testing and prototyping.

**Generated MCP Tools:**
- `getPosts` - Get all posts (with optional userId filter)
- `createPost` - Create a new post
- `getPostById` - Get post by ID
- `updatePost` - Update an existing post
- `deletePost` - Delete a post
- `getUsers` - Get all users
- `getUserById` - Get user by ID
- `getComments` - Get comments (with optional postId filter)

**Usage:**
```bash
./bin/swagger-mcp-go --swagger-file=examples/jsonplaceholder.json
```

## Claude Desktop Integration Examples

### Basic Pet Store Integration

Add to your Claude Desktop MCP configuration (`~/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "petstore": {
      "command": "/path/to/swagger-mcp-go",
      "args": [
        "--swagger-file=/path/to/examples/petstore.json",
        "--base-url=https://petstore.swagger.io/v2"
      ]
    }
  }
}
```

### JSON Placeholder Integration

```json
{
  "mcpServers": {
    "jsonplaceholder": {
      "command": "/path/to/swagger-mcp-go", 
      "args": [
        "--swagger-file=/path/to/examples/jsonplaceholder.json"
      ]
    }
  }
}
```

### Docker Integration

```json
{
  "mcpServers": {
    "my-api": {
      "command": "docker",
      "args": [
        "run",
        "-i", 
        "--rm",
        "-v", "/path/to/your-api.json:/api.json",
        "swagger-mcp-go:latest",
        "--swagger-file=/api.json",
        "--base-url=https://your-api.com"
      ]
    }
  }
}
```

## Testing MCP Integration

### Test with MCP Inspector

1. Install MCP Inspector:
```bash
npm install -g @anthropic-ai/mcp-inspector
```

2. Run with swagger-mcp-go:
```bash
mcp-inspector ./bin/swagger-mcp-go --swagger-file=examples/petstore.json
```

### Manual JSON-RPC Testing

Test the STDIO mode directly:

```bash
echo '{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": {}}' | ./bin/swagger-mcp-go --swagger-file=examples/petstore.json
```

### HTTP Mode Testing

1. Start in HTTP mode:
```bash
./bin/swagger-mcp-go --swagger-file=examples/petstore.json --mode=http
```

2. Test with curl:
```bash
# List available tools
curl -X POST http://localhost:8081 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": {}}'

# Call a tool
curl -X POST http://localhost:8081 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0", 
    "id": 1, 
    "method": "tools/call", 
    "params": {
      "name": "getPetById", 
      "arguments": {"petId": 1}
    }
  }'
```

## Creating Your Own OpenAPI Integration

### Step 1: Prepare Your OpenAPI Spec

Ensure your OpenAPI specification:
- Has valid `operationId` fields (or they'll be auto-generated)
- Includes proper parameter definitions
- Has server URLs defined (or use `--base-url` flag)

### Step 2: Test the Integration

```bash
./bin/swagger-mcp-go --swagger-file=your-api.json --mode=http --base-url=https://your-api.com
```

### Step 3: Configure Authentication (if needed)

Add custom headers via configuration file:

```yaml
# config.yaml
upstream:
  timeout: 30s
  headers:
    Authorization: "Bearer your-token"
    X-API-Key: "your-api-key"
```

Then run:
```bash
./bin/swagger-mcp-go --swagger-file=your-api.json --config=config.yaml
```

### Step 4: Deploy to Claude Desktop

Add to your Claude Desktop configuration and restart Claude.

## Troubleshooting

### Common Issues

1. **"No paths found in OpenAPI specification"**
   - Check that your OpenAPI file has a `paths` section
   - Verify the file is valid JSON/YAML

2. **"Failed to parse OpenAPI spec"**
   - Validate your OpenAPI spec using online validators
   - Check for syntax errors in JSON/YAML

3. **"Tool execution failed"**
   - Verify the base URL is correct
   - Check that the API is accessible
   - Review authentication requirements

### Debug Mode

Run with detailed logging:
```bash
./bin/swagger-mcp-go --swagger-file=your-api.json --mode=http 2>&1 | grep -v "level.*debug"
```

### Validate OpenAPI Specs

Use online validators:
- [Swagger Editor](https://editor.swagger.io/)
- [OpenAPI Validator](https://openapi-generator.tech/docs/installation)

## Advanced Usage

### Custom Configuration

Create a configuration file for advanced settings:

```yaml
# advanced-config.yaml
server:
  host: "0.0.0.0"
  port: 8080

mcp:
  enabled: true
  host: "0.0.0.0" 
  port: 8081

logging:
  level: "debug"
  format: "json"

upstream:
  timeout: 60s
  retryCount: 3
  retryDelay: 2s
  headers:
    User-Agent: "swagger-mcp-go/1.0.0"
    Accept: "application/json"

metrics:
  enabled: true
  path: "/metrics"
```

### Environment Variables

Override configuration with environment variables:

```bash
export LOG_LEVEL=debug
export MCP_PORT=9000
./bin/swagger-mcp-go --swagger-file=examples/petstore.json
```

### Docker Deployment

Build and run with Docker:

```bash
# Build
docker build -t swagger-mcp-go .

# Run
docker run -p 8080:8080 -p 8081:8081 \
  -v $(pwd)/examples:/examples \
  swagger-mcp-go \
  --swagger-file=/examples/petstore.json \
  --mode=http
```

Use docker-compose:
```bash
docker-compose up
```