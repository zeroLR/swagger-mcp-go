#!/bin/bash

# Example script demonstrating basic usage of swagger-mcp-go

echo "Starting swagger-mcp-go server..."
./bin/swagger-mcp-go &
SERVER_PID=$!

# Wait for server to start
sleep 3

echo "Checking server health..."
curl -s http://localhost:8080/health | jq .

echo -e "\nGetting initial specs (should be empty)..."
curl -s http://localhost:8080/admin/specs | jq .

echo -e "\nGetting server statistics..."
curl -s http://localhost:8080/admin/stats | jq .

echo -e "\nChecking metrics endpoint..."
curl -s http://localhost:8080/metrics | head -5

echo -e "\nTesting 404 for unknown API path..."
curl -s http://localhost:8080/apis/unknown/test | jq .

echo -e "\nServer is running on http://localhost:8080"
echo "Admin API available at http://localhost:8080/admin/*"
echo "Metrics available at http://localhost:8080/metrics"
echo "Health check at http://localhost:8080/health"

echo -e "\nStopping server..."
kill $SERVER_PID

echo "Demo complete!"