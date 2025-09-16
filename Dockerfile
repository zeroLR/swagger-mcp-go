# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the application with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o swagger-mcp-go ./cmd/server

# Verify the binary
RUN ./swagger-mcp-go --version

# Final stage - distroless for better security and smaller size
FROM gcr.io/distroless/static-debian12:nonroot

# Copy the binary from builder stage
COPY --from=builder /app/swagger-mcp-go /usr/local/bin/swagger-mcp-go

# Copy configuration and example files
COPY --from=builder /app/configs/ /configs/
COPY --from=builder /app/examples/ /examples/

# Expose ports
EXPOSE 8080 8081

# Health check (distroless doesn't support HEALTHCHECK)

# Default command
ENTRYPOINT ["/usr/local/bin/swagger-mcp-go"]
CMD ["--help"]