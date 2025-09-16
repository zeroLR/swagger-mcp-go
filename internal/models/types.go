package models

import (
	"time"

	"github.com/getkin/kin-openapi/openapi3"
)

// AuthType represents the type of authentication
type AuthType string

const (
	AuthTypeBasic  AuthType = "basic"
	AuthTypeBearer AuthType = "bearer"
	AuthTypeOAuth2 AuthType = "oauth2"
	AuthTypeAPIKey AuthType = "apikey"
)

// SpecInfo holds information about a registered OpenAPI specification
type SpecInfo struct {
	ID          string            `json:"id"`
	ServiceName string            `json:"serviceName"`
	URL         string            `json:"url"`
	Spec        *openapi3.T       `json:"spec"`
	FetchedAt   time.Time         `json:"fetchedAt"`
	TTL         time.Duration     `json:"ttl"`
	Headers     map[string]string `json:"headers"`
	AuthPolicy  *AuthPolicy       `json:"authPolicy,omitempty"`
}

// ProxyRequest represents an incoming request to be proxied
type ProxyRequest struct {
	Method      string              `json:"method"`
	Path        string              `json:"path"`
	Headers     map[string]string   `json:"headers"`
	Body        []byte              `json:"body,omitempty"`
	ServiceName string              `json:"serviceName"`
	Operation   *openapi3.Operation `json:"operation"`
}

// AuthPolicy defines authentication requirements for a service
type AuthPolicy struct {
	Type     AuthType               `json:"type"`
	Config   map[string]interface{} `json:"config"`
	Required bool                   `json:"required"`
	Scopes   []string               `json:"scopes,omitempty"`
}

// RouteInfo provides information about registered routes
type RouteInfo struct {
	Path        string   `json:"path"`
	Method      string   `json:"method"`
	ServiceName string   `json:"serviceName"`
	OperationID string   `json:"operationId,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// ServiceStats contains performance and usage statistics
type ServiceStats struct {
	ServiceName    string        `json:"serviceName,omitempty"`
	RequestCount   int64         `json:"requestCount"`
	ErrorCount     int64         `json:"errorCount"`
	AverageLatency time.Duration `json:"averageLatency"`
	LastRequest    time.Time     `json:"lastRequest"`
	SpecFetchedAt  time.Time     `json:"specFetchedAt"`
	SpecURL        string        `json:"specUrl"`
	RouteCount     int           `json:"routeCount"`
}

// MCPToolRequest represents an MCP tool invocation
type MCPToolRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// MCPToolResponse represents an MCP tool response
type MCPToolResponse struct {
	Success bool                   `json:"success"`
	Result  map[string]interface{} `json:"result,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

// Config represents the application configuration
type Config struct {
	Server struct {
		Host         string        `yaml:"host"`
		Port         int           `yaml:"port"`
		ReadTimeout  time.Duration `yaml:"readTimeout"`
		WriteTimeout time.Duration `yaml:"writeTimeout"`
	} `yaml:"server"`

	MCP struct {
		Enabled bool   `yaml:"enabled"`
		Host    string `yaml:"host"`
		Port    int    `yaml:"port"`
	} `yaml:"mcp"`

	Logging struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"`
	} `yaml:"logging"`

	Metrics struct {
		Enabled bool   `yaml:"enabled"`
		Path    string `yaml:"path"`
	} `yaml:"metrics"`

	Tracing struct {
		Enabled     bool   `yaml:"enabled"`
		Endpoint    string `yaml:"endpoint"`
		ServiceName string `yaml:"serviceName"`
	} `yaml:"tracing"`

	Upstream struct {
		Timeout        time.Duration `yaml:"timeout"`
		RetryCount     int           `yaml:"retryCount"`
		RetryDelay     time.Duration `yaml:"retryDelay"`
		CircuitBreaker struct {
			Threshold int           `yaml:"threshold"`
			Timeout   time.Duration `yaml:"timeout"`
		} `yaml:"circuitBreaker"`
	} `yaml:"upstream"`

	Auth struct {
		JWT struct {
			JWKSURL  string `yaml:"jwksURL"`
			Issuer   string `yaml:"issuer"`
			Audience string `yaml:"audience"`
		} `yaml:"jwt"`
		OAuth2 struct {
			TokenURL     string `yaml:"tokenURL"`
			ClientID     string `yaml:"clientID"`
			ClientSecret string `yaml:"clientSecret"`
		} `yaml:"oauth2"`
	} `yaml:"auth"`

	Specs struct {
		DefaultTTL string `yaml:"defaultTTL"`
		MaxSize    string `yaml:"maxSize"`
	} `yaml:"specs"`

	Policies struct {
		RateLimit struct {
			Enabled           bool `yaml:"enabled"`
			RequestsPerMinute int  `yaml:"requestsPerMinute"`
		} `yaml:"rateLimit"`
		CORS struct {
			Enabled      bool     `yaml:"enabled"`
			AllowOrigins []string `yaml:"allowOrigins"`
			AllowMethods []string `yaml:"allowMethods"`
		} `yaml:"cors"`
	} `yaml:"policies"`
}
