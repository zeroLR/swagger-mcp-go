# Kubernetes Deployment Guide

This guide covers deploying swagger-mcp-go to Kubernetes clusters.

## Quick Start

### Prerequisites

- Kubernetes cluster (1.19+)
- kubectl configured
- Docker (for building images)

### Deploy

1. **Deploy to cluster**:
   ```bash
   ./scripts/deploy-k8s.sh deploy
   ```

2. **Access the application**:
   ```bash
   kubectl port-forward svc/swagger-mcp-go-service 8080:8080 -n swagger-mcp-go
   ```

3. **Check status**:
   ```bash
   ./scripts/deploy-k8s.sh status
   ```

4. **Clean up**:
   ```bash
   ./scripts/deploy-k8s.sh delete
   ```

### Local Development

For local development with kind/minikube:

```bash
# Build and load image locally
BUILD_IMAGE=true ./scripts/deploy-k8s.sh deploy
```

## Architecture

### Kubernetes Resources

The deployment includes:

- **Namespace**: Isolated environment
- **ConfigMaps**: Configuration and examples
- **Deployment**: Application pods with replicas
- **Service**: Internal load balancing
- **Ingress**: External access
- **HPA**: Horizontal Pod Autoscaler
- **PDB**: Pod Disruption Budget
- **ServiceAccount**: RBAC permissions
- **NetworkPolicy**: Network security
- **ServiceMonitor**: Prometheus integration

### Security Features

- **Non-root containers**: Runs as user 65534
- **Read-only filesystem**: Enhanced security
- **Resource limits**: Prevents resource exhaustion
- **Network policies**: Restricts network access
- **RBAC**: Least privilege access
- **Security context**: Additional hardening

## Configuration

### ConfigMaps

#### Application Configuration

```yaml
# k8s/configmap.yaml
data:
  config.yaml: |
    server:
      host: "0.0.0.0"
      port: 8080
    policies:
      rateLimit:
        enabled: true
        requestsPerMinute: 100
```

#### OpenAPI Specifications

Example specifications are stored in ConfigMaps and mounted as volumes.

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LOG_LEVEL` | Logging level | `info` |
| `SWAGGER_MCP_GO_LOGGING_FORMAT` | Log format | `json` |

### Resource Requests and Limits

```yaml
resources:
  requests:
    memory: "64Mi"
    cpu: "250m"
  limits:
    memory: "128Mi"
    cpu: "500m"
```

## Scaling

### Horizontal Pod Autoscaler

Automatically scales based on CPU and memory:

```yaml
spec:
  minReplicas: 3
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

### Manual Scaling

```bash
kubectl scale deployment swagger-mcp-go --replicas=5 -n swagger-mcp-go
```

### Pod Disruption Budget

Ensures availability during updates:

```yaml
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: swagger-mcp-go
```

## Networking

### Service

ClusterIP service for internal load balancing:

```yaml
spec:
  type: ClusterIP
  ports:
  - name: http
    port: 8080
    targetPort: http
  - name: mcp
    port: 8081
    targetPort: mcp
```

### Ingress

NGINX ingress for external access:

```yaml
spec:
  rules:
  - host: swagger-mcp-go.local
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: swagger-mcp-go-service
            port:
              number: 8080
```

### Network Policies

Restricts network access:

```yaml
spec:
  podSelector:
    matchLabels:
      app: swagger-mcp-go
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: monitoring
    ports:
    - protocol: TCP
      port: 8080
```

## Monitoring

### Prometheus Integration

ServiceMonitor for automatic discovery:

```yaml
spec:
  selector:
    matchLabels:
      app: swagger-mcp-go
  endpoints:
  - port: http
    path: /metrics
    interval: 30s
```

### Alerts

PrometheusRule with common alerts:

- Service down
- High error rate
- High latency
- Circuit breaker open

### Health Checks

#### Liveness Probe

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: http
  initialDelaySeconds: 30
  periodSeconds: 10
```

#### Readiness Probe

```yaml
readinessProbe:
  httpGet:
    path: /health
    port: http
  initialDelaySeconds: 5
  periodSeconds: 5
```

## Security

### RBAC

Minimal permissions:

```yaml
rules:
- apiGroups: [""]
  resources: ["configmaps", "secrets"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch"]
```

### Security Context

Container security:

```yaml
securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop:
    - ALL
```

Pod security:

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 65534
  runAsGroup: 65534
  fsGroup: 65534
```

### Image Security

- Distroless base image
- Non-root user
- No shell access
- Minimal dependencies

## Updates and Rollouts

### Rolling Updates

Default strategy ensures zero downtime:

```yaml
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxUnavailable: 1
    maxSurge: 1
```

### Rollback

```bash
# View rollout history
kubectl rollout history deployment/swagger-mcp-go -n swagger-mcp-go

# Rollback to previous version
kubectl rollout undo deployment/swagger-mcp-go -n swagger-mcp-go
```

## Troubleshooting

### Common Issues

1. **Image pull errors**:
   ```bash
   # Check if image exists
   kubectl describe pod <pod-name> -n swagger-mcp-go
   
   # For local development, ensure image is loaded
   BUILD_IMAGE=true ./scripts/deploy-k8s.sh deploy
   ```

2. **Configuration issues**:
   ```bash
   # Check ConfigMap
   kubectl get configmap swagger-mcp-go-config -n swagger-mcp-go -o yaml
   
   # Check mounted volumes
   kubectl describe pod <pod-name> -n swagger-mcp-go
   ```

3. **Network issues**:
   ```bash
   # Check service endpoints
   kubectl get endpoints -n swagger-mcp-go
   
   # Test internal connectivity
   kubectl run test-pod --image=curlimages/curl -it --rm -- /bin/sh
   ```

### Debug Commands

```bash
# Get all resources
kubectl get all -n swagger-mcp-go

# Describe deployment
kubectl describe deployment swagger-mcp-go -n swagger-mcp-go

# View logs
kubectl logs -f deployment/swagger-mcp-go -n swagger-mcp-go

# Execute into pod
kubectl exec -it deployment/swagger-mcp-go -n swagger-mcp-go -- /bin/sh

# Port forward for testing
kubectl port-forward svc/swagger-mcp-go-service 8080:8080 -n swagger-mcp-go
```

### Resource Issues

```bash
# Check resource usage
kubectl top pods -n swagger-mcp-go
kubectl top nodes

# Check HPA status
kubectl get hpa -n swagger-mcp-go

# Check PDB status
kubectl get pdb -n swagger-mcp-go
```

## Production Considerations

### Cluster Requirements

- **Node resources**: At least 2 CPU cores, 4GB RAM
- **Storage**: Persistent volumes if needed
- **Networking**: CNI plugin supporting NetworkPolicies
- **Monitoring**: Prometheus operator for metrics

### Multi-Environment Setup

Use Kustomize for environment-specific configurations:

```bash
# Base configuration
k8s/
├── base/
│   ├── kustomization.yaml
│   ├── deployment.yaml
│   └── service.yaml
└── overlays/
    ├── staging/
    │   ├── kustomization.yaml
    │   └── replica-count.yaml
    └── production/
        ├── kustomization.yaml
        ├── replica-count.yaml
        └── resource-limits.yaml
```

### Backup and Recovery

1. **Backup configurations**:
   ```bash
   kubectl get all,configmaps,secrets -n swagger-mcp-go -o yaml > backup.yaml
   ```

2. **Restore from backup**:
   ```bash
   kubectl apply -f backup.yaml
   ```

### CI/CD Integration

Example GitHub Actions workflow:

```yaml
- name: Deploy to Kubernetes
  run: |
    kubectl set image deployment/swagger-mcp-go \
      swagger-mcp-go=swagger-mcp-go:${{ github.sha }} \
      -n swagger-mcp-go
    kubectl rollout status deployment/swagger-mcp-go -n swagger-mcp-go
```

## Script Reference

The `./scripts/deploy-k8s.sh` script supports:

| Command | Description |
|---------|-------------|
| `deploy` | Deploy to Kubernetes |
| `delete` | Delete deployment |
| `status` | Show deployment status |
| `logs` | Show application logs |

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `IMAGE_TAG` | Docker image tag | `latest` |
| `BUILD_IMAGE` | Build image locally | `false` |
| `KUBECTL_CMD` | kubectl command | `kubectl` |