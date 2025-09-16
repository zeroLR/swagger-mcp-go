# MCP Tool Demonstration

This document demonstrates the MCP (Model Context Protocol) tools implemented in the swagger-mcp-go server.

## Available Tools

The MCP server provides 8 tools for managing OpenAPI specifications:

### 1. listSpecs
Lists all registered OpenAPI specifications.

**Parameters:** None

**Example Response:**
```json
{
  "specs": [],
  "count": 0
}
```

### 2. addSpec
Adds a new OpenAPI specification from a URL.

**Parameters:**
- `url` (required): URL to fetch the OpenAPI specification from
- `serviceName` (required): Name for the service
- `ttl` (optional): Time-to-live for the specification cache (default: "1h")
- `headers` (optional): Additional headers to send with the request

**Example Request:**
```json
{
  "url": "https://petstore.swagger.io/v2/swagger.json",
  "serviceName": "petstore",
  "ttl": "2h",
  "headers": {
    "Authorization": "Bearer token123"
  }
}
```

### 3. refreshSpec
Force refreshes an existing OpenAPI specification.

**Parameters:**
- `serviceName` (required): Name of the service to refresh

### 4. removeSpec
Removes an OpenAPI specification.

**Parameters:**
- `serviceName` (required): Name of the service to remove

### 5. inspectRoute
Inspects route configuration for a service.

**Parameters:**
- `serviceName` (required): Name of the service to inspect

**Example Response:**
```json
{
  "serviceName": "petstore",
  "url": "https://petstore.swagger.io/v2/swagger.json",
  "routeCount": 20,
  "routes": [
    {
      "path": "/pet",
      "method": "POST",
      "serviceName": "petstore",
      "operationId": "addPet",
      "summary": "Add a new pet to the store"
    }
  ]
}
```

### 6. getStats
Gets performance statistics for all services or a specific service.

**Parameters:**
- `serviceName` (optional): Name of specific service to get stats for

**Example Response (global):**
```json
{
  "global": {
    "totalSpecs": 1,
    "expiredSpecs": 0,
    "services": ["petstore"]
  },
  "services": [
    {
      "serviceName": "petstore",
      "specFetchedAt": "2024-01-01T12:00:00Z",
      "specUrl": "https://petstore.swagger.io/v2/swagger.json",
      "routeCount": 20
    }
  ]
}
```

### 7. enableAuthPolicy
Enables authentication policy for a service.

**Parameters:**
- `serviceName` (required): Name of the service
- `authType` (required): Type of authentication (basic, bearer, oauth2)
- `config` (optional): Authentication configuration parameters
- `required` (optional): Whether authentication is required (default: true)

**Example Request:**
```json
{
  "serviceName": "petstore",
  "authType": "bearer",
  "config": {
    "jwksUrl": "https://example.com/.well-known/jwks.json"
  },
  "required": true
}
```

### 8. disableAuthPolicy
Disables authentication policy for a service.

**Parameters:**
- `serviceName` (required): Name of the service

## Error Handling

All tools return structured error responses when validation fails or resources are not found:

```json
{
  "error": "Service 'nonexistent' not found"
}
```

## Integration

The MCP server integrates with:
- **Registry**: For in-memory specification storage with TTL
- **Fetcher**: For downloading and validating OpenAPI specifications
- **Models**: For data structures and authentication policies

## Testing

The implementation includes comprehensive tests covering:
- Tool parameter validation
- Error handling for missing services
- Empty state handling
- Authentication policy management
- Structured response format validation

All tests pass and the server builds successfully.