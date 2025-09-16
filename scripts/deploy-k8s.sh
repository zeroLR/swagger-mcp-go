#!/bin/bash

# Kubernetes deployment script for swagger-mcp-go
set -e

NAMESPACE="swagger-mcp-go"
IMAGE_TAG="${IMAGE_TAG:-latest}"
KUBECTL_CMD="${KUBECTL_CMD:-kubectl}"

echo "🚀 Deploying swagger-mcp-go to Kubernetes..."

# Function to check if kubectl is available
check_kubectl() {
    if ! command -v $KUBECTL_CMD &> /dev/null; then
        echo "❌ kubectl could not be found. Please install kubectl first."
        exit 1
    fi
}

# Function to check if namespace exists
check_namespace() {
    if $KUBECTL_CMD get namespace $NAMESPACE &> /dev/null; then
        echo "✅ Namespace $NAMESPACE exists"
    else
        echo "📦 Creating namespace $NAMESPACE..."
        $KUBECTL_CMD apply -f k8s/namespace.yaml
    fi
}

# Function to build and load Docker image (for local development)
build_image() {
    if [[ "${BUILD_IMAGE:-false}" == "true" ]]; then
        echo "🔨 Building Docker image..."
        docker build -t swagger-mcp-go:$IMAGE_TAG .
        
        # Load image into kind/minikube if available
        if command -v kind &> /dev/null && kind get clusters | grep -q "kind"; then
            echo "📦 Loading image into kind cluster..."
            kind load docker-image swagger-mcp-go:$IMAGE_TAG
        elif command -v minikube &> /dev/null && minikube status | grep -q "Running"; then
            echo "📦 Loading image into minikube..."
            minikube image load swagger-mcp-go:$IMAGE_TAG
        fi
    fi
}

# Function to apply Kubernetes manifests
deploy_manifests() {
    echo "📋 Applying Kubernetes manifests..."
    
    # Apply in order
    $KUBECTL_CMD apply -f k8s/namespace.yaml
    $KUBECTL_CMD apply -f k8s/security.yaml
    $KUBECTL_CMD apply -f k8s/configmap.yaml
    $KUBECTL_CMD apply -f k8s/deployment.yaml
    $KUBECTL_CMD apply -f k8s/scaling.yaml
    
    # Apply monitoring if Prometheus operator is available
    if $KUBECTL_CMD get crd servicemonitors.monitoring.coreos.com &> /dev/null; then
        echo "📊 Applying monitoring manifests..."
        $KUBECTL_CMD apply -f k8s/monitoring.yaml
    else
        echo "⚠️  Prometheus operator not found, skipping monitoring manifests"
    fi
}

# Function to wait for deployment
wait_for_deployment() {
    echo "⏳ Waiting for deployment to be ready..."
    $KUBECTL_CMD wait --for=condition=available --timeout=300s deployment/swagger-mcp-go -n $NAMESPACE
    
    echo "🔍 Checking pod status..."
    $KUBECTL_CMD get pods -n $NAMESPACE -l app=swagger-mcp-go
}

# Function to show access information
show_access_info() {
    echo "🎉 Deployment completed successfully!"
    echo ""
    echo "📝 Access information:"
    echo "  Namespace: $NAMESPACE"
    echo "  Service: swagger-mcp-go-service"
    echo ""
    
    # Check if ingress is available
    if $KUBECTL_CMD get ingress swagger-mcp-go-ingress -n $NAMESPACE &> /dev/null; then
        echo "🌐 Ingress URL: http://swagger-mcp-go.local"
        echo "   (Add '127.0.0.1 swagger-mcp-go.local' to /etc/hosts for local access)"
    fi
    
    echo ""
    echo "🔧 Useful commands:"
    echo "  View logs: $KUBECTL_CMD logs -f deployment/swagger-mcp-go -n $NAMESPACE"
    echo "  Port forward: $KUBECTL_CMD port-forward svc/swagger-mcp-go-service 8080:8080 -n $NAMESPACE"
    echo "  Delete deployment: $KUBECTL_CMD delete namespace $NAMESPACE"
    echo ""
    echo "🏥 Health check: curl http://localhost:8080/health (after port-forward)"
    echo "📊 Metrics: curl http://localhost:8080/metrics (after port-forward)"
}

# Main execution
main() {
    echo "🔍 Checking prerequisites..."
    check_kubectl
    check_namespace
    
    if [[ "${BUILD_IMAGE:-false}" == "true" ]]; then
        build_image
    fi
    
    deploy_manifests
    wait_for_deployment
    show_access_info
}

# Handle command line arguments
case "${1:-deploy}" in
    "deploy")
        main
        ;;
    "delete")
        echo "🗑️  Deleting swagger-mcp-go deployment..."
        $KUBECTL_CMD delete namespace $NAMESPACE
        echo "✅ Deployment deleted"
        ;;
    "status")
        echo "📊 Deployment status:"
        $KUBECTL_CMD get all -n $NAMESPACE
        ;;
    "logs")
        echo "📋 Application logs:"
        $KUBECTL_CMD logs -f deployment/swagger-mcp-go -n $NAMESPACE
        ;;
    "help"|"--help"|"-h")
        echo "Usage: $0 [deploy|delete|status|logs|help]"
        echo ""
        echo "Commands:"
        echo "  deploy  - Deploy swagger-mcp-go to Kubernetes (default)"
        echo "  delete  - Delete the deployment"
        echo "  status  - Show deployment status"
        echo "  logs    - Show application logs"
        echo "  help    - Show this help message"
        echo ""
        echo "Environment variables:"
        echo "  IMAGE_TAG     - Docker image tag (default: latest)"
        echo "  BUILD_IMAGE   - Build Docker image locally (default: false)"
        echo "  KUBECTL_CMD   - kubectl command to use (default: kubectl)"
        ;;
    *)
        echo "❌ Unknown command: $1"
        echo "Use '$0 help' for usage information"
        exit 1
        ;;
esac