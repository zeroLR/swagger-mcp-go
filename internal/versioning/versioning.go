package versioning

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/zeroLR/swagger-mcp-go/internal/models"
	"go.uber.org/zap"
)

// VersioningStrategy represents different versioning approaches
type VersioningStrategy string

const (
	VersioningStrategyPath    VersioningStrategy = "path"      // /v1/users, /v2/users
	VersioningStrategyHeader  VersioningStrategy = "header"    // Accept: application/vnd.api.v1+json
	VersioningStrategyQuery   VersioningStrategy = "query"     // /users?version=1
	VersioningStrategyContent VersioningStrategy = "content"   // Content-Type: application/vnd.api.v1+json
)

// Version represents an API version
type Version struct {
	Major int    `json:"major"`
	Minor int    `json:"minor"`
	Patch int    `json:"patch"`
	Label string `json:"label,omitempty"` // alpha, beta, rc
}

// String returns the semantic version string
func (v Version) String() string {
	version := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Label != "" {
		version += "-" + v.Label
	}
	return version
}

// ShortString returns the major.minor version string
func (v Version) ShortString() string {
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

// Compare compares two versions (-1 if less, 0 if equal, 1 if greater)
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}
	return strings.Compare(v.Label, other.Label)
}

// IsCompatible checks if this version is backwards compatible with another
func (v Version) IsCompatible(other Version) bool {
	// Major version changes are breaking
	if v.Major != other.Major {
		return false
	}
	// Minor version increases are backwards compatible
	return v.Minor >= other.Minor
}

// VersionedSpec represents a versioned OpenAPI specification
type VersionedSpec struct {
	Version       Version             `json:"version"`
	Spec          *openapi3.T         `json:"spec"`
	SpecInfo      *models.SpecInfo    `json:"specInfo"`
	Strategy      VersioningStrategy  `json:"strategy"`
	Compatibility []Version           `json:"compatibility"` // Compatible versions
	Deprecated    bool                `json:"deprecated"`
}

// VersionManager manages multiple versions of API specifications
type VersionManager struct {
	specs    map[string]map[Version]*VersionedSpec // serviceName -> version -> spec
	strategy VersioningStrategy
	logger   *zap.Logger
}

// NewVersionManager creates a new version manager
func NewVersionManager(strategy VersioningStrategy, logger *zap.Logger) *VersionManager {
	return &VersionManager{
		specs:    make(map[string]map[Version]*VersionedSpec),
		strategy: strategy,
		logger:   logger,
	}
}

// AddVersion adds a versioned specification
func (vm *VersionManager) AddVersion(serviceName string, versionedSpec *VersionedSpec) error {
	if vm.specs[serviceName] == nil {
		vm.specs[serviceName] = make(map[Version]*VersionedSpec)
	}

	vm.specs[serviceName][versionedSpec.Version] = versionedSpec
	vm.logger.Info("Added versioned spec",
		zap.String("service", serviceName),
		zap.String("version", versionedSpec.Version.String()),
		zap.String("strategy", string(versionedSpec.Strategy)))

	return nil
}

// GetVersion retrieves a specific version of a specification
func (vm *VersionManager) GetVersion(serviceName string, version Version) (*VersionedSpec, error) {
	serviceSpecs, exists := vm.specs[serviceName]
	if !exists {
		return nil, fmt.Errorf("service %s not found", serviceName)
	}

	spec, exists := serviceSpecs[version]
	if !exists {
		return nil, fmt.Errorf("version %s not found for service %s", version.String(), serviceName)
	}

	return spec, nil
}

// GetLatestVersion returns the latest version of a service
func (vm *VersionManager) GetLatestVersion(serviceName string) (*VersionedSpec, error) {
	serviceSpecs, exists := vm.specs[serviceName]
	if !exists {
		return nil, fmt.Errorf("service %s not found", serviceName)
	}

	var latest *VersionedSpec
	var latestVersion Version

	for version, spec := range serviceSpecs {
		if latest == nil || version.Compare(latestVersion) > 0 {
			latest = spec
			latestVersion = version
		}
	}

	return latest, nil
}

// GetCompatibleVersion finds a compatible version for the requested version
func (vm *VersionManager) GetCompatibleVersion(serviceName string, requestedVersion Version) (*VersionedSpec, error) {
	serviceSpecs, exists := vm.specs[serviceName]
	if !exists {
		return nil, fmt.Errorf("service %s not found", serviceName)
	}

	// Try exact match first
	if spec, exists := serviceSpecs[requestedVersion]; exists {
		return spec, nil
	}

	// Find compatible version
	var bestMatch *VersionedSpec
	for _, spec := range serviceSpecs {
		if spec.Version.IsCompatible(requestedVersion) {
			if bestMatch == nil || spec.Version.Compare(bestMatch.Version) > 0 {
				bestMatch = spec
			}
		}
	}

	if bestMatch == nil {
		return nil, fmt.Errorf("no compatible version found for %s", requestedVersion.String())
	}

	return bestMatch, nil
}

// ListVersions returns all versions for a service
func (vm *VersionManager) ListVersions(serviceName string) ([]Version, error) {
	serviceSpecs, exists := vm.specs[serviceName]
	if !exists {
		return nil, fmt.Errorf("service %s not found", serviceName)
	}

	versions := make([]Version, 0, len(serviceSpecs))
	for version := range serviceSpecs {
		versions = append(versions, version)
	}

	return versions, nil
}

// ResolveVersionFromRequest extracts version information from an HTTP request
func (vm *VersionManager) ResolveVersionFromRequest(r *http.Request, serviceName string) (*VersionedSpec, error) {
	switch vm.strategy {
	case VersioningStrategyPath:
		return vm.resolveVersionFromPath(r, serviceName)
	case VersioningStrategyHeader:
		return vm.resolveVersionFromHeader(r, serviceName)
	case VersioningStrategyQuery:
		return vm.resolveVersionFromQuery(r, serviceName)
	case VersioningStrategyContent:
		return vm.resolveVersionFromContentType(r, serviceName)
	default:
		return vm.GetLatestVersion(serviceName)
	}
}

// resolveVersionFromPath extracts version from URL path (/v1/users -> v1)
func (vm *VersionManager) resolveVersionFromPath(r *http.Request, serviceName string) (*VersionedSpec, error) {
	// Pattern: /v{major}.{minor}/... or /v{major}/...
	pathVersionRegex := regexp.MustCompile(`^/v(\d+)(?:\.(\d+))?(?:/|$)`)
	matches := pathVersionRegex.FindStringSubmatch(r.URL.Path)
	
	if len(matches) < 2 {
		return vm.GetLatestVersion(serviceName)
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return vm.GetLatestVersion(serviceName)
	}

	minor := 0
	if len(matches) > 2 && matches[2] != "" {
		minor, _ = strconv.Atoi(matches[2])
	}

	requestedVersion := Version{Major: major, Minor: minor}
	return vm.GetCompatibleVersion(serviceName, requestedVersion)
}

// resolveVersionFromHeader extracts version from Accept header
func (vm *VersionManager) resolveVersionFromHeader(r *http.Request, serviceName string) (*VersionedSpec, error) {
	accept := r.Header.Get("Accept")
	if accept == "" {
		return vm.GetLatestVersion(serviceName)
	}

	// Pattern: application/vnd.{service}.v{major}+json
	headerVersionRegex := regexp.MustCompile(`application/vnd\.[^.]+\.v(\d+)(?:\.(\d+))?`)
	matches := headerVersionRegex.FindStringSubmatch(accept)
	
	if len(matches) < 2 {
		return vm.GetLatestVersion(serviceName)
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return vm.GetLatestVersion(serviceName)
	}

	minor := 0
	if len(matches) > 2 && matches[2] != "" {
		minor, _ = strconv.Atoi(matches[2])
	}

	requestedVersion := Version{Major: major, Minor: minor}
	return vm.GetCompatibleVersion(serviceName, requestedVersion)
}

// resolveVersionFromQuery extracts version from query parameter
func (vm *VersionManager) resolveVersionFromQuery(r *http.Request, serviceName string) (*VersionedSpec, error) {
	versionStr := r.URL.Query().Get("version")
	if versionStr == "" {
		versionStr = r.URL.Query().Get("v")
	}
	
	if versionStr == "" {
		return vm.GetLatestVersion(serviceName)
	}

	version, err := ParseVersion(versionStr)
	if err != nil {
		return vm.GetLatestVersion(serviceName)
	}

	return vm.GetCompatibleVersion(serviceName, version)
}

// resolveVersionFromContentType extracts version from Content-Type header
func (vm *VersionManager) resolveVersionFromContentType(r *http.Request, serviceName string) (*VersionedSpec, error) {
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		return vm.GetLatestVersion(serviceName)
	}

	// Pattern: application/vnd.{service}.v{major}+json
	contentVersionRegex := regexp.MustCompile(`application/vnd\.[^.]+\.v(\d+)(?:\.(\d+))?`)
	matches := contentVersionRegex.FindStringSubmatch(contentType)
	
	if len(matches) < 2 {
		return vm.GetLatestVersion(serviceName)
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return vm.GetLatestVersion(serviceName)
	}

	minor := 0
	if len(matches) > 2 && matches[2] != "" {
		minor, _ = strconv.Atoi(matches[2])
	}

	requestedVersion := Version{Major: major, Minor: minor}
	return vm.GetCompatibleVersion(serviceName, requestedVersion)
}

// ParseVersion parses a version string into a Version struct
func ParseVersion(versionStr string) (Version, error) {
	versionStr = strings.TrimPrefix(versionStr, "v")
	
	// Split version and label (e.g., "1.2.3-beta")
	parts := strings.Split(versionStr, "-")
	versionPart := parts[0]
	
	var label string
	if len(parts) > 1 {
		label = parts[1]
	}

	// Parse version numbers
	versionNumbers := strings.Split(versionPart, ".")
	if len(versionNumbers) == 0 {
		return Version{}, fmt.Errorf("invalid version format: %s", versionStr)
	}

	major, err := strconv.Atoi(versionNumbers[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version: %s", versionNumbers[0])
	}

	minor := 0
	if len(versionNumbers) > 1 {
		minor, err = strconv.Atoi(versionNumbers[1])
		if err != nil {
			return Version{}, fmt.Errorf("invalid minor version: %s", versionNumbers[1])
		}
	}

	patch := 0
	if len(versionNumbers) > 2 {
		patch, err = strconv.Atoi(versionNumbers[2])
		if err != nil {
			return Version{}, fmt.Errorf("invalid patch version: %s", versionNumbers[2])
		}
	}

	return Version{
		Major: major,
		Minor: minor,
		Patch: patch,
		Label: label,
	}, nil
}

// VersioningMiddleware creates middleware for version resolution
func (vm *VersionManager) VersioningMiddleware(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			versionedSpec, err := vm.ResolveVersionFromRequest(r, serviceName)
			if err != nil {
				vm.logger.Error("Failed to resolve version", 
					zap.String("service", serviceName),
					zap.Error(err))
				http.Error(w, "Version resolution failed", http.StatusBadRequest)
				return
			}

			// Add version information to request context
			ctx := r.Context()
			ctx = withVersionedSpec(ctx, versionedSpec)
			
			// Add version headers to response
			w.Header().Set("API-Version", versionedSpec.Version.String())
			w.Header().Set("API-Version-Strategy", string(versionedSpec.Strategy))
			
			if versionedSpec.Deprecated {
				w.Header().Set("API-Deprecated", "true")
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}