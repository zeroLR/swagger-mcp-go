package registry_test

import (
	"testing"
	"time"

	"go.uber.org/zap"
	"github.com/getkin/kin-openapi/openapi3"
	
	"github.com/zeroLR/swagger-mcp-go/internal/models"
	"github.com/zeroLR/swagger-mcp-go/internal/registry"
)

func TestRegistry_AddAndGet(t *testing.T) {
	logger := zap.NewNop()
	reg := registry.New(logger)

	// Create a test spec
	spec := &openapi3.T{
		OpenAPI: "3.0.0",
		Info: &openapi3.Info{
			Title:   "Test API",
			Version: "1.0.0",
		},
	}

	specInfo := &models.SpecInfo{
		ID:          "test-service:http://example.com/api.json",
		ServiceName: "test-service",
		URL:         "http://example.com/api.json",
		Spec:        spec,
		FetchedAt:   time.Now(),
		TTL:         time.Hour,
		Headers:     map[string]string{"Authorization": "Bearer token"},
	}

	// Test adding spec
	err := reg.Add(specInfo)
	if err != nil {
		t.Fatalf("Failed to add spec: %v", err)
	}

	// Test getting spec
	retrieved, exists := reg.Get("test-service")
	if !exists {
		t.Fatal("Spec should exist")
	}

	if retrieved.ServiceName != "test-service" {
		t.Errorf("Expected service name 'test-service', got '%s'", retrieved.ServiceName)
	}

	if retrieved.URL != "http://example.com/api.json" {
		t.Errorf("Expected URL 'http://example.com/api.json', got '%s'", retrieved.URL)
	}
}

func TestRegistry_Remove(t *testing.T) {
	logger := zap.NewNop()
	reg := registry.New(logger)

	// Add a test spec
	spec := &models.SpecInfo{
		ID:          "test-service:http://example.com/api.json",
		ServiceName: "test-service",
		URL:         "http://example.com/api.json",
		Spec:        &openapi3.T{OpenAPI: "3.0.0"},
		FetchedAt:   time.Now(),
		TTL:         time.Hour,
	}

	reg.Add(spec)

	// Test removal
	removed := reg.Remove("test-service")
	if !removed {
		t.Fatal("Should have removed spec")
	}

	// Test that spec is gone
	_, exists := reg.Get("test-service")
	if exists {
		t.Fatal("Spec should not exist after removal")
	}

	// Test removing non-existent spec
	removed = reg.Remove("non-existent")
	if removed {
		t.Fatal("Should not have removed non-existent spec")
	}
}

func TestRegistry_List(t *testing.T) {
	logger := zap.NewNop()
	reg := registry.New(logger)

	// Initially empty
	specs := reg.List()
	if len(specs) != 0 {
		t.Fatalf("Expected 0 specs, got %d", len(specs))
	}

	// Add some specs
	spec1 := &models.SpecInfo{
		ServiceName: "service1",
		URL:         "http://example.com/api1.json",
		Spec:        &openapi3.T{OpenAPI: "3.0.0"},
		FetchedAt:   time.Now(),
		TTL:         time.Hour,
	}

	spec2 := &models.SpecInfo{
		ServiceName: "service2",
		URL:         "http://example.com/api2.json",
		Spec:        &openapi3.T{OpenAPI: "3.0.0"},
		FetchedAt:   time.Now(),
		TTL:         time.Hour,
	}

	reg.Add(spec1)
	reg.Add(spec2)

	// Test listing
	specs = reg.List()
	if len(specs) != 2 {
		t.Fatalf("Expected 2 specs, got %d", len(specs))
	}

	// Check that both specs are present
	serviceNames := make(map[string]bool)
	for _, spec := range specs {
		serviceNames[spec.ServiceName] = true
	}

	if !serviceNames["service1"] || !serviceNames["service2"] {
		t.Fatal("Both service1 and service2 should be present")
	}
}

func TestRegistry_Stats(t *testing.T) {
	logger := zap.NewNop()
	reg := registry.New(logger)

	// Test empty stats
	stats := reg.Stats()
	if stats["totalSpecs"] != 0 {
		t.Errorf("Expected 0 total specs, got %v", stats["totalSpecs"])
	}

	// Add a spec
	spec := &models.SpecInfo{
		ServiceName: "test-service",
		URL:         "http://example.com/api.json",
		Spec:        &openapi3.T{OpenAPI: "3.0.0"},
		FetchedAt:   time.Now(),
		TTL:         time.Hour,
	}

	reg.Add(spec)

	// Test stats with one spec
	stats = reg.Stats()
	if stats["totalSpecs"] != 1 {
		t.Errorf("Expected 1 total spec, got %v", stats["totalSpecs"])
	}

	services, ok := stats["services"].([]string)
	if !ok {
		t.Fatal("services should be a slice of strings")
	}

	if len(services) != 1 || services[0] != "test-service" {
		t.Errorf("Expected services ['test-service'], got %v", services)
	}
}