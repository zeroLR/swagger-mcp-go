# Docker Deployment Guide

This guide covers deploying swagger-mcp-go using Docker and Docker Compose.

## Quick Start

### Using Docker Compose (Recommended)

1. **Start all services**:
   ```bash
   ./scripts/deploy-docker.sh up
   ```

2. **Access the application**:
   - Main service: http://localhost:8080
   - Load balancer: http://localhost:8090
   - Grafana: http://localhost:3000 (admin/admin)
   - Prometheus: http://localhost:9090

3. **Stop services**:
   ```bash
   ./scripts/deploy-docker.sh down
   ```

### Using Docker Directly

1. **Build the image**:
   ```bash
   docker build -t swagger-mcp-go:latest .
   ```

2. **Run the container**:
   ```bash
   docker run -p 8080:8080 \
     -v $(pwd)/examples:/examples:ro \
     swagger-mcp-go:latest \
     --swagger-file=/examples/petstore.json \
     --mode=http \
     --base-url=https://petstore.swagger.io/v2
   ```

## Docker Compose Services

The `docker-compose.yml` includes several services:

### Core Services

- **swagger-mcp-go**: Main application instance
- **swagger-mcp-go-jsonplaceholder**: Secondary instance with different API
- **nginx**: Load balancer for multiple instances

### Monitoring Stack

- **prometheus**: Metrics collection
- **grafana**: Metrics visualization
- **jaeger**: Distributed tracing

### Infrastructure

- **redis**: Caching (for rate limiting)

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LOG_LEVEL` | Logging level | `info` |
| `SWAGGER_MCP_GO_LOGGING_FORMAT` | Log format | `json` |

### Volumes

- `./examples:/examples:ro` - OpenAPI specification files
- `./configs:/configs:ro` - Configuration files

## Health Checks

All services include health checks:

```bash
# Check main service
curl http://localhost:8080/health

# Check via load balancer
curl http://localhost:8090/health

# Run all health checks
./scripts/deploy-docker.sh health
```

## Monitoring

### Prometheus Metrics

Access Prometheus at http://localhost:9090

Available metrics:
- HTTP request metrics
- Circuit breaker status
- Rate limiting metrics
- Application-specific metrics

### Grafana Dashboards

Access Grafana at http://localhost:3000 (admin/admin)

Pre-configured with Prometheus datasource.

### Distributed Tracing

Access Jaeger at http://localhost:16686

Traces include:
- HTTP requests
- Upstream API calls
- Circuit breaker operations

## Scaling

### Manual Scaling

Scale services using Docker Compose:

```bash
docker-compose up -d --scale swagger-mcp-go=3
```

### Load Balancing

Nginx automatically load balances between instances:

```bash
# Check load balancer status
curl http://localhost:8090/health
```

## Security

### Image Security

The Docker image uses:
- Distroless base image for minimal attack surface
- Non-root user (65534)
- Read-only root filesystem
- No unnecessary packages

### Network Security

- Services communicate via internal network
- Only necessary ports are exposed
- Health checks use internal endpoints

## Troubleshooting

### Common Issues

1. **Port conflicts**:
   ```bash
   # Check what's using the ports
   lsof -i :8080
   lsof -i :8090
   ```

2. **Service not starting**:
   ```bash
   # Check logs
   ./scripts/deploy-docker.sh logs swagger-mcp-go
   ```

3. **Health check failures**:
   ```bash
   # Run health checks
   ./scripts/deploy-docker.sh health
   ```

### Debug Mode

Run with debug logging:

```yaml
# In docker-compose.yml
environment:
  - LOG_LEVEL=debug
```

### Resource Issues

Monitor resource usage:

```bash
# Check container resources
docker stats

# Check system resources
docker system df
```

## Production Considerations

### Resource Limits

Configure appropriate limits in `docker-compose.yml`:

```yaml
services:
  swagger-mcp-go:
    deploy:
      resources:
        limits:
          memory: 512M
          cpus: '0.5'
        reservations:
          memory: 256M
          cpus: '0.25'
```

### Persistent Storage

For production, use named volumes:

```yaml
volumes:
  redis_data:
    driver: local
  prometheus_data:
    driver: local
```

### Secrets Management

Use Docker secrets for sensitive data:

```yaml
secrets:
  oauth_client_secret:
    file: ./secrets/oauth_client_secret.txt

services:
  swagger-mcp-go:
    secrets:
      - oauth_client_secret
```

### Backup and Recovery

1. **Backup volumes**:
   ```bash
   docker run --rm -v swagger-mcp-go_redis_data:/data -v $(pwd):/backup alpine tar czf /backup/redis_backup.tar.gz /data
   ```

2. **Restore volumes**:
   ```bash
   docker run --rm -v swagger-mcp-go_redis_data:/data -v $(pwd):/backup alpine tar xzf /backup/redis_backup.tar.gz -C /
   ```

## Script Reference

The `./scripts/deploy-docker.sh` script supports these commands:

| Command | Description |
|---------|-------------|
| `up` | Build and start all services |
| `down` | Stop all services |
| `restart` | Restart all services |
| `build` | Build Docker images only |
| `status` | Show service status and URLs |
| `logs [service]` | Show logs |
| `clean` | Stop services and clean up |
| `health` | Run health checks |

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `COMPOSE_FILE` | Docker Compose file | `docker-compose.yml` |
| `PROJECT_NAME` | Project name | `swagger-mcp-go` |
| `BUILD` | Build images | `true` |
| `CLEAN_IMAGES` | Clean unused images | `false` |