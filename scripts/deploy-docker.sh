#!/bin/bash

# Docker Compose deployment script for swagger-mcp-go
set -e

COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.yml}"
PROJECT_NAME="${PROJECT_NAME:-swagger-mcp-go}"

echo "🐳 Managing swagger-mcp-go Docker Compose deployment..."

# Function to check if docker-compose is available
check_docker_compose() {
    if command -v docker-compose &> /dev/null; then
        COMPOSE_CMD="docker-compose"
    elif docker compose version &> /dev/null; then
        COMPOSE_CMD="docker compose"
    else
        echo "❌ Docker Compose could not be found. Please install Docker Compose first."
        exit 1
    fi
}

# Function to build images
build_images() {
    echo "🔨 Building Docker images..."
    $COMPOSE_CMD -p $PROJECT_NAME -f $COMPOSE_FILE build
}

# Function to start services
start_services() {
    echo "🚀 Starting services..."
    $COMPOSE_CMD -p $PROJECT_NAME -f $COMPOSE_FILE up -d
    
    echo "⏳ Waiting for services to be healthy..."
    sleep 10
    
    show_status
}

# Function to stop services
stop_services() {
    echo "🛑 Stopping services..."
    $COMPOSE_CMD -p $PROJECT_NAME -f $COMPOSE_FILE down
}

# Function to show status
show_status() {
    echo "📊 Service status:"
    $COMPOSE_CMD -p $PROJECT_NAME -f $COMPOSE_FILE ps
    
    echo ""
    echo "🔗 Service URLs:"
    echo "  Swagger MCP Go (Main):        http://localhost:8080"
    echo "  Swagger MCP Go (Secondary):   http://localhost:8082"
    echo "  Load Balancer (Nginx):        http://localhost:8090"
    echo "  Prometheus:                   http://localhost:9090"
    echo "  Grafana:                      http://localhost:3000 (admin/admin)"
    echo "  Jaeger:                       http://localhost:16686"
    echo "  Redis:                        localhost:6379"
    echo ""
    echo "🏥 Health checks:"
    echo "  Main service:     curl http://localhost:8080/health"
    echo "  Secondary:        curl http://localhost:8082/health"
    echo "  Load balancer:    curl http://localhost:8090/health"
    echo ""
    echo "📊 Metrics:"
    echo "  Main service:     curl http://localhost:8080/metrics"
    echo "  Secondary:        curl http://localhost:8082/metrics"
}

# Function to show logs
show_logs() {
    local service="${1:-}"
    if [[ -n "$service" ]]; then
        echo "📋 Logs for service: $service"
        $COMPOSE_CMD -p $PROJECT_NAME -f $COMPOSE_FILE logs -f "$service"
    else
        echo "📋 All service logs:"
        $COMPOSE_CMD -p $PROJECT_NAME -f $COMPOSE_FILE logs -f
    fi
}

# Function to clean up
cleanup() {
    echo "🧹 Cleaning up..."
    $COMPOSE_CMD -p $PROJECT_NAME -f $COMPOSE_FILE down -v --remove-orphans
    
    # Remove unused images
    if [[ "${CLEAN_IMAGES:-false}" == "true" ]]; then
        echo "🗑️  Removing unused images..."
        docker image prune -f
    fi
}

# Function to restart services
restart_services() {
    echo "🔄 Restarting services..."
    stop_services
    start_services
}

# Function to run health checks
health_check() {
    echo "🏥 Running health checks..."
    
    services=("localhost:8080" "localhost:8082" "localhost:8090")
    
    for service in "${services[@]}"; do
        echo -n "  Checking $service: "
        if curl -f -s "http://$service/health" > /dev/null; then
            echo "✅ Healthy"
        else
            echo "❌ Unhealthy"
        fi
    done
    
    echo -n "  Checking Prometheus: "
    if curl -f -s "http://localhost:9090/-/healthy" > /dev/null; then
        echo "✅ Healthy"
    else
        echo "❌ Unhealthy"
    fi
    
    echo -n "  Checking Grafana: "
    if curl -f -s "http://localhost:3000/api/health" > /dev/null; then
        echo "✅ Healthy"
    else
        echo "❌ Unhealthy"
    fi
}

# Main execution
main() {
    check_docker_compose
    
    case "${1:-up}" in
        "up"|"start")
            if [[ "${BUILD:-true}" == "true" ]]; then
                build_images
            fi
            start_services
            ;;
        "down"|"stop")
            stop_services
            ;;
        "restart")
            restart_services
            ;;
        "build")
            build_images
            ;;
        "status"|"ps")
            show_status
            ;;
        "logs")
            show_logs "${2:-}"
            ;;
        "clean"|"cleanup")
            cleanup
            ;;
        "health")
            health_check
            ;;
        "help"|"--help"|"-h")
            echo "Usage: $0 [up|down|restart|build|status|logs|clean|health|help]"
            echo ""
            echo "Commands:"
            echo "  up       - Build and start all services (default)"
            echo "  down     - Stop all services"
            echo "  restart  - Restart all services"
            echo "  build    - Build Docker images only"
            echo "  status   - Show service status and URLs"
            echo "  logs     - Show logs (optionally for specific service)"
            echo "  clean    - Stop services and clean up volumes"
            echo "  health   - Run health checks on all services"
            echo "  help     - Show this help message"
            echo ""
            echo "Environment variables:"
            echo "  COMPOSE_FILE   - Docker Compose file to use (default: docker-compose.yml)"
            echo "  PROJECT_NAME   - Docker Compose project name (default: swagger-mcp-go)"
            echo "  BUILD          - Whether to build images (default: true)"
            echo "  CLEAN_IMAGES   - Whether to clean unused images on cleanup (default: false)"
            echo ""
            echo "Examples:"
            echo "  $0 up                    # Start all services"
            echo "  $0 logs swagger-mcp-go   # Show logs for main service"
            echo "  BUILD=false $0 up        # Start without building"
            ;;
        *)
            echo "❌ Unknown command: $1"
            echo "Use '$0 help' for usage information"
            exit 1
            ;;
    esac
}

main "$@"