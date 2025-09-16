# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o swagger-mcp-go ./cmd/server

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/swagger-mcp-go .

# Copy example configs and specs
COPY --from=builder /app/configs/ ./configs/
COPY --from=builder /app/examples/ ./examples/

# Create a non-root user
RUN adduser -D -s /bin/sh appuser
USER appuser

# Default command
ENTRYPOINT ["./swagger-mcp-go"]
CMD ["--help"]