package versioning

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"go.uber.org/zap"
)

// CompatibilityLevel represents the level of compatibility checking
type CompatibilityLevel string

const (
	CompatibilityLevelStrict CompatibilityLevel = "strict" // No breaking changes allowed
	CompatibilityLevelLoose  CompatibilityLevel = "loose"  // Some breaking changes allowed with deprecation
	CompatibilityLevelNone   CompatibilityLevel = "none"   // No compatibility checking
)

// ChangeType represents the type of schema change
type ChangeType string

const (
	ChangeTypeBreaking    ChangeType = "breaking"
	ChangeTypeDeprecation ChangeType = "deprecation"
	ChangeTypeAdditive    ChangeType = "additive"
	ChangeTypeUpdate      ChangeType = "update"
	ChangeTypeRemoval     ChangeType = "removal"
)

// SchemaChange represents a detected change between schema versions
type SchemaChange struct {
	Type        ChangeType `json:"type"`
	Severity    string     `json:"severity"`
	Path        string     `json:"path"`
	Description string     `json:"description"`
	OldValue    string     `json:"oldValue,omitempty"`
	NewValue    string     `json:"newValue,omitempty"`
}

// CompatibilityReport contains the results of a compatibility check
type CompatibilityReport struct {
	Compatible    bool           `json:"compatible"`
	Changes       []SchemaChange `json:"changes"`
	BreakingCount int            `json:"breakingCount"`
	TotalCount    int            `json:"totalCount"`
	Version       Version        `json:"version"`
	BaseVersion   Version        `json:"baseVersion"`
}

// SchemaEvolution handles schema compatibility checking and evolution
type SchemaEvolution struct {
	level  CompatibilityLevel
	logger *zap.Logger
}

// NewSchemaEvolution creates a new schema evolution checker
func NewSchemaEvolution(level CompatibilityLevel, logger *zap.Logger) *SchemaEvolution {
	return &SchemaEvolution{
		level:  level,
		logger: logger,
	}
}

// CheckCompatibility checks compatibility between two OpenAPI specifications
func (se *SchemaEvolution) CheckCompatibility(baseSpec, newSpec *openapi3.T, baseVersion, newVersion Version) *CompatibilityReport {
	report := &CompatibilityReport{
		Compatible:  true,
		Changes:     []SchemaChange{},
		Version:     newVersion,
		BaseVersion: baseVersion,
	}

	// Check paths for compatibility
	se.checkPaths(baseSpec, newSpec, report)

	// Check components for compatibility
	se.checkComponents(baseSpec, newSpec, report)

	// Check servers for compatibility
	se.checkServers(baseSpec, newSpec, report)

	// Evaluate overall compatibility
	report.TotalCount = len(report.Changes)
	report.BreakingCount = 0
	for _, change := range report.Changes {
		if change.Type == ChangeTypeBreaking {
			report.BreakingCount++
		}
	}

	// Determine compatibility based on level and breaking changes
	switch se.level {
	case CompatibilityLevelStrict:
		report.Compatible = report.BreakingCount == 0
	case CompatibilityLevelLoose:
		report.Compatible = report.BreakingCount == 0 || se.hasProperDeprecation(report)
	case CompatibilityLevelNone:
		report.Compatible = true
	}

	return report
}

// checkPaths compares API paths between versions
func (se *SchemaEvolution) checkPaths(baseSpec, newSpec *openapi3.T, report *CompatibilityReport) {
	if baseSpec.Paths == nil && newSpec.Paths == nil {
		return
	}

	basePaths := make(map[string]*openapi3.PathItem)
	if baseSpec.Paths != nil {
		basePaths = baseSpec.Paths.Map()
	}

	newPaths := make(map[string]*openapi3.PathItem)
	if newSpec.Paths != nil {
		newPaths = newSpec.Paths.Map()
	}

	// Check for removed paths
	for path := range basePaths {
		if _, exists := newPaths[path]; !exists {
			report.Changes = append(report.Changes, SchemaChange{
				Type:        ChangeTypeBreaking,
				Severity:    "error",
				Path:        path,
				Description: "Path removed",
				OldValue:    path,
			})
		}
	}

	// Check for added paths
	for path := range newPaths {
		if _, exists := basePaths[path]; !exists {
			report.Changes = append(report.Changes, SchemaChange{
				Type:        ChangeTypeAdditive,
				Severity:    "info",
				Path:        path,
				Description: "Path added",
				NewValue:    path,
			})
		}
	}

	// Check for modified paths
	for path, newPathItem := range newPaths {
		if basePathItem, exists := basePaths[path]; exists {
			se.checkPathItem(path, basePathItem, newPathItem, report)
		}
	}
}

// checkPathItem compares individual path items
func (se *SchemaEvolution) checkPathItem(path string, baseItem, newItem *openapi3.PathItem, report *CompatibilityReport) {
	// Compare operations
	operations := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS", "TRACE"}
	
	for _, method := range operations {
		baseOp := se.getOperation(baseItem, method)
		newOp := se.getOperation(newItem, method)
		
		if baseOp != nil && newOp == nil {
			// Operation removed
			report.Changes = append(report.Changes, SchemaChange{
				Type:        ChangeTypeBreaking,
				Severity:    "error",
				Path:        fmt.Sprintf("%s %s", method, path),
				Description: "Operation removed",
			})
		} else if baseOp == nil && newOp != nil {
			// Operation added
			report.Changes = append(report.Changes, SchemaChange{
				Type:        ChangeTypeAdditive,
				Severity:    "info",
				Path:        fmt.Sprintf("%s %s", method, path),
				Description: "Operation added",
			})
		} else if baseOp != nil && newOp != nil {
			// Operation modified
			se.checkOperation(fmt.Sprintf("%s %s", method, path), baseOp, newOp, report)
		}
	}
}

// getOperation gets an operation by method from a path item
func (se *SchemaEvolution) getOperation(pathItem *openapi3.PathItem, method string) *openapi3.Operation {
	switch method {
	case "GET":
		return pathItem.Get
	case "POST":
		return pathItem.Post
	case "PUT":
		return pathItem.Put
	case "DELETE":
		return pathItem.Delete
	case "PATCH":
		return pathItem.Patch
	case "HEAD":
		return pathItem.Head
	case "OPTIONS":
		return pathItem.Options
	case "TRACE":
		return pathItem.Trace
	default:
		return nil
	}
}

// checkOperation compares operations for compatibility
func (se *SchemaEvolution) checkOperation(path string, baseOp, newOp *openapi3.Operation, report *CompatibilityReport) {
	// Check if operation is deprecated
	if !baseOp.Deprecated && newOp.Deprecated {
		report.Changes = append(report.Changes, SchemaChange{
			Type:        ChangeTypeDeprecation,
			Severity:    "warning",
			Path:        path,
			Description: "Operation deprecated",
		})
	}

	// Check parameters
	se.checkParameters(path, baseOp.Parameters, newOp.Parameters, report)

	// Check request body
	se.checkRequestBody(path, baseOp.RequestBody, newOp.RequestBody, report)

	// Check responses
	se.checkResponses(path, baseOp.Responses, newOp.Responses, report)
}

// checkParameters compares operation parameters
func (se *SchemaEvolution) checkParameters(path string, baseParams, newParams openapi3.Parameters, report *CompatibilityReport) {
	baseParamMap := make(map[string]*openapi3.ParameterRef)
	for _, param := range baseParams {
		if param.Value != nil {
			key := fmt.Sprintf("%s:%s", param.Value.In, param.Value.Name)
			baseParamMap[key] = param
		}
	}

	newParamMap := make(map[string]*openapi3.ParameterRef)
	for _, param := range newParams {
		if param.Value != nil {
			key := fmt.Sprintf("%s:%s", param.Value.In, param.Value.Name)
			newParamMap[key] = param
		}
	}

	// Check for removed parameters
	for key, baseParam := range baseParamMap {
		if _, exists := newParamMap[key]; !exists && baseParam.Value.Required {
			report.Changes = append(report.Changes, SchemaChange{
				Type:        ChangeTypeBreaking,
				Severity:    "error",
				Path:        fmt.Sprintf("%s parameter %s", path, baseParam.Value.Name),
				Description: "Required parameter removed",
				OldValue:    baseParam.Value.Name,
			})
		}
	}

	// Check for added required parameters
	for key, newParam := range newParamMap {
		if _, exists := baseParamMap[key]; !exists && newParam.Value.Required {
			report.Changes = append(report.Changes, SchemaChange{
				Type:        ChangeTypeBreaking,
				Severity:    "error",
				Path:        fmt.Sprintf("%s parameter %s", path, newParam.Value.Name),
				Description: "Required parameter added",
				NewValue:    newParam.Value.Name,
			})
		}
	}
}

// checkRequestBody compares request bodies
func (se *SchemaEvolution) checkRequestBody(path string, baseBody, newBody *openapi3.RequestBodyRef, report *CompatibilityReport) {
	if baseBody != nil && newBody == nil {
		report.Changes = append(report.Changes, SchemaChange{
			Type:        ChangeTypeBreaking,
			Severity:    "error",
			Path:        fmt.Sprintf("%s request body", path),
			Description: "Request body removed",
		})
	} else if baseBody == nil && newBody != nil && newBody.Value.Required {
		report.Changes = append(report.Changes, SchemaChange{
			Type:        ChangeTypeBreaking,
			Severity:    "error",
			Path:        fmt.Sprintf("%s request body", path),
			Description: "Required request body added",
		})
	}
}

// checkResponses compares operation responses
func (se *SchemaEvolution) checkResponses(path string, baseResponses, newResponses *openapi3.Responses, report *CompatibilityReport) {
	if baseResponses == nil || newResponses == nil {
		return
	}

	baseRespMap := baseResponses.Map()
	newRespMap := newResponses.Map()

	// Check for removed success responses
	for status, baseResp := range baseRespMap {
		if strings.HasPrefix(status, "2") { // 2xx responses
			if _, exists := newRespMap[status]; !exists && baseResp != nil {
				report.Changes = append(report.Changes, SchemaChange{
					Type:        ChangeTypeBreaking,
					Severity:    "error",
					Path:        fmt.Sprintf("%s response %s", path, status),
					Description: "Success response removed",
					OldValue:    status,
				})
			}
		}
	}

	// Check for added error responses
	for status, newResp := range newRespMap {
		if strings.HasPrefix(status, "4") || strings.HasPrefix(status, "5") { // 4xx, 5xx responses
			if _, exists := baseRespMap[status]; !exists && newResp != nil {
				report.Changes = append(report.Changes, SchemaChange{
					Type:        ChangeTypeUpdate,
					Severity:    "info",
					Path:        fmt.Sprintf("%s response %s", path, status),
					Description: "Error response added",
					NewValue:    status,
				})
			}
		}
	}
}

// checkComponents compares schema components
func (se *SchemaEvolution) checkComponents(baseSpec, newSpec *openapi3.T, report *CompatibilityReport) {
	if baseSpec.Components == nil && newSpec.Components == nil {
		return
	}

	var baseSchemas, newSchemas map[string]*openapi3.SchemaRef
	if baseSpec.Components != nil {
		baseSchemas = baseSpec.Components.Schemas
	}
	if newSpec.Components != nil {
		newSchemas = newSpec.Components.Schemas
	}

	if baseSchemas == nil {
		baseSchemas = make(map[string]*openapi3.SchemaRef)
	}
	if newSchemas == nil {
		newSchemas = make(map[string]*openapi3.SchemaRef)
	}

	// Check for removed schemas
	for name := range baseSchemas {
		if _, exists := newSchemas[name]; !exists {
			report.Changes = append(report.Changes, SchemaChange{
				Type:        ChangeTypeBreaking,
				Severity:    "error",
				Path:        fmt.Sprintf("components/schemas/%s", name),
				Description: "Schema removed",
				OldValue:    name,
			})
		}
	}

	// Check for added schemas
	for name := range newSchemas {
		if _, exists := baseSchemas[name]; !exists {
			report.Changes = append(report.Changes, SchemaChange{
				Type:        ChangeTypeAdditive,
				Severity:    "info",
				Path:        fmt.Sprintf("components/schemas/%s", name),
				Description: "Schema added",
				NewValue:    name,
			})
		}
	}
}

// checkServers compares server configurations
func (se *SchemaEvolution) checkServers(baseSpec, newSpec *openapi3.T, report *CompatibilityReport) {
	if reflect.DeepEqual(baseSpec.Servers, newSpec.Servers) {
		return
	}

	report.Changes = append(report.Changes, SchemaChange{
		Type:        ChangeTypeUpdate,
		Severity:    "info",
		Path:        "servers",
		Description: "Server configuration changed",
	})
}

// hasProperDeprecation checks if breaking changes have proper deprecation notices
func (se *SchemaEvolution) hasProperDeprecation(report *CompatibilityReport) bool {
	breakingChanges := 0
	deprecations := 0

	for _, change := range report.Changes {
		if change.Type == ChangeTypeBreaking {
			breakingChanges++
		} else if change.Type == ChangeTypeDeprecation {
			deprecations++
		}
	}

	// Allow breaking changes if there are corresponding deprecations
	return deprecations > 0
}

// GetCompatibilityLevel returns the current compatibility level
func (se *SchemaEvolution) GetCompatibilityLevel() CompatibilityLevel {
	return se.level
}

// SetCompatibilityLevel sets the compatibility level
func (se *SchemaEvolution) SetCompatibilityLevel(level CompatibilityLevel) {
	se.level = level
}

// SortChangesByPath sorts schema changes by path for consistent reporting
func SortChangesByPath(changes []SchemaChange) {
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Path < changes[j].Path
	})
}