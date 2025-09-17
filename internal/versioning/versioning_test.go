package versioning

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"go.uber.org/zap"
)

func TestVersion(t *testing.T) {
	tests := []struct {
		name     string
		version  Version
		expected string
	}{
		{
			name:     "basic version",
			version:  Version{Major: 1, Minor: 2, Patch: 3},
			expected: "1.2.3",
		},
		{
			name:     "version with label",
			version:  Version{Major: 2, Minor: 0, Patch: 0, Label: "beta"},
			expected: "2.0.0-beta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.version.String(); got != tt.expected {
				t.Errorf("Version.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		name     string
		v1       Version
		v2       Version
		expected int
	}{
		{
			name:     "equal versions",
			v1:       Version{Major: 1, Minor: 2, Patch: 3},
			v2:       Version{Major: 1, Minor: 2, Patch: 3},
			expected: 0,
		},
		{
			name:     "v1 greater major",
			v1:       Version{Major: 2, Minor: 0, Patch: 0},
			v2:       Version{Major: 1, Minor: 9, Patch: 9},
			expected: 1,
		},
		{
			name:     "v1 lesser minor",
			v1:       Version{Major: 1, Minor: 1, Patch: 0},
			v2:       Version{Major: 1, Minor: 2, Patch: 0},
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v1.Compare(tt.v2); got != tt.expected {
				t.Errorf("Version.Compare() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestVersionCompatibility(t *testing.T) {
	tests := []struct {
		name       string
		v1         Version
		v2         Version
		compatible bool
	}{
		{
			name:       "same major version",
			v1:         Version{Major: 1, Minor: 2, Patch: 0},
			v2:         Version{Major: 1, Minor: 1, Patch: 0},
			compatible: true,
		},
		{
			name:       "different major version",
			v1:         Version{Major: 2, Minor: 0, Patch: 0},
			v2:         Version{Major: 1, Minor: 9, Patch: 9},
			compatible: false,
		},
		{
			name:       "backward compatible minor",
			v1:         Version{Major: 1, Minor: 3, Patch: 0},
			v2:         Version{Major: 1, Minor: 2, Patch: 0},
			compatible: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v1.IsCompatible(tt.v2); got != tt.compatible {
				t.Errorf("Version.IsCompatible() = %v, want %v", got, tt.compatible)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name        string
		versionStr  string
		expected    Version
		expectError bool
	}{
		{
			name:       "basic version",
			versionStr: "1.2.3",
			expected:   Version{Major: 1, Minor: 2, Patch: 3},
		},
		{
			name:       "version with v prefix",
			versionStr: "v2.0.0",
			expected:   Version{Major: 2, Minor: 0, Patch: 0},
		},
		{
			name:       "version with label",
			versionStr: "1.0.0-alpha",
			expected:   Version{Major: 1, Minor: 0, Patch: 0, Label: "alpha"},
		},
		{
			name:       "major only",
			versionStr: "3",
			expected:   Version{Major: 3, Minor: 0, Patch: 0},
		},
		{
			name:        "invalid version",
			versionStr:  "invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVersion(tt.versionStr)
			if tt.expectError {
				if err == nil {
					t.Errorf("ParseVersion() expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("ParseVersion() error = %v", err)
				return
			}
			if got != tt.expected {
				t.Errorf("ParseVersion() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestVersionManager(t *testing.T) {
	logger := zap.NewNop()
	vm := NewVersionManager(VersioningStrategyPath, logger)

	// Create test specs
	spec1 := &openapi3.T{OpenAPI: "3.0.0"}
	spec2 := &openapi3.T{OpenAPI: "3.0.0"}

	v1 := Version{Major: 1, Minor: 0, Patch: 0}
	v2 := Version{Major: 1, Minor: 1, Patch: 0}

	versionedSpec1 := &VersionedSpec{
		Version:  v1,
		Spec:     spec1,
		Strategy: VersioningStrategyPath,
	}

	versionedSpec2 := &VersionedSpec{
		Version:  v2,
		Spec:     spec2,
		Strategy: VersioningStrategyPath,
	}

	// Test adding versions
	err := vm.AddVersion("test-service", versionedSpec1)
	if err != nil {
		t.Errorf("AddVersion() error = %v", err)
	}

	err = vm.AddVersion("test-service", versionedSpec2)
	if err != nil {
		t.Errorf("AddVersion() error = %v", err)
	}

	// Test getting specific version
	retrieved, err := vm.GetVersion("test-service", v1)
	if err != nil {
		t.Errorf("GetVersion() error = %v", err)
	}
	if retrieved.Version != v1 {
		t.Errorf("GetVersion() version = %v, want %v", retrieved.Version, v1)
	}

	// Test getting latest version
	latest, err := vm.GetLatestVersion("test-service")
	if err != nil {
		t.Errorf("GetLatestVersion() error = %v", err)
	}
	if latest.Version != v2 {
		t.Errorf("GetLatestVersion() version = %v, want %v", latest.Version, v2)
	}

	// Test compatibility
	compatible, err := vm.GetCompatibleVersion("test-service", v1)
	if err != nil {
		t.Errorf("GetCompatibleVersion() error = %v", err)
	}
	if compatible.Version != v1 {
		t.Errorf("GetCompatibleVersion() version = %v, want %v", compatible.Version, v1)
	}

	// Test listing versions
	versions, err := vm.ListVersions("test-service")
	if err != nil {
		t.Errorf("ListVersions() error = %v", err)
	}
	if len(versions) != 2 {
		t.Errorf("ListVersions() count = %v, want %v", len(versions), 2)
	}
}

func TestVersionResolution(t *testing.T) {
	logger := zap.NewNop()
	vm := NewVersionManager(VersioningStrategyPath, logger)

	// Add test version
	spec := &openapi3.T{OpenAPI: "3.0.0"}
	version := Version{Major: 1, Minor: 0, Patch: 0}
	versionedSpec := &VersionedSpec{
		Version:  version,
		Spec:     spec,
		Strategy: VersioningStrategyPath,
	}
	vm.AddVersion("test-service", versionedSpec)

	tests := []struct {
		name        string
		method      string
		url         string
		headers     map[string]string
		strategy    VersioningStrategy
		expectError bool
	}{
		{
			name:     "path versioning",
			method:   "GET",
			url:      "/v1/users",
			strategy: VersioningStrategyPath,
		},
		{
			name:     "header versioning",
			method:   "GET",
			url:      "/users",
			headers:  map[string]string{"Accept": "application/vnd.api.v1+json"},
			strategy: VersioningStrategyHeader,
		},
		{
			name:     "query versioning",
			method:   "GET",
			url:      "/users?version=1",
			strategy: VersioningStrategyQuery,
		},
		{
			name:     "content type versioning",
			method:   "POST",
			url:      "/users",
			headers:  map[string]string{"Content-Type": "application/vnd.api.v1+json"},
			strategy: VersioningStrategyContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm.strategy = tt.strategy

			parsedURL, _ := url.Parse(tt.url)
			req := &http.Request{
				Method: tt.method,
				URL:    parsedURL,
				Header: make(http.Header),
			}

			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			result, err := vm.ResolveVersionFromRequest(req, "test-service")
			if tt.expectError {
				if err == nil {
					t.Errorf("ResolveVersionFromRequest() expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("ResolveVersionFromRequest() error = %v", err)
				return
			}
			if result == nil {
				t.Errorf("ResolveVersionFromRequest() returned nil result")
			}
		})
	}
}

func TestVersioningMiddleware(t *testing.T) {
	logger := zap.NewNop()
	vm := NewVersionManager(VersioningStrategyPath, logger)

	// Add test version
	spec := &openapi3.T{OpenAPI: "3.0.0"}
	version := Version{Major: 1, Minor: 0, Patch: 0}
	versionedSpec := &VersionedSpec{
		Version:  version,
		Spec:     spec,
		Strategy: VersioningStrategyPath,
	}
	vm.AddVersion("test-service", versionedSpec)

	// Create test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		versionedSpec, ok := GetVersionedSpecFromContext(r.Context())
		if !ok {
			http.Error(w, "No versioned spec in context", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Test-Version", versionedSpec.Version.String())
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with versioning middleware
	middleware := vm.VersioningMiddleware("test-service")
	wrappedHandler := middleware(testHandler)

	// Test the middleware
	req := httptest.NewRequest("GET", "/v1/users", nil)
	recorder := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	apiVersion := recorder.Header().Get("API-Version")
	if apiVersion != "1.0.0" {
		t.Errorf("Expected API-Version header '1.0.0', got '%s'", apiVersion)
	}

	testVersion := recorder.Header().Get("Test-Version")
	if testVersion != "1.0.0" {
		t.Errorf("Expected Test-Version header '1.0.0', got '%s'", testVersion)
	}
}