package specs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"go.uber.org/zap"
	"github.com/zeroLR/swagger-mcp-go/internal/models"
)

// Fetcher handles fetching and validating OpenAPI specifications
type Fetcher struct {
	client  *http.Client
	logger  *zap.Logger
	maxSize int64
}

// New creates a new spec fetcher
func New(logger *zap.Logger, timeout time.Duration, maxSize int64) *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: timeout,
		},
		logger:  logger,
		maxSize: maxSize,
	}
}

// FetchSpec fetches and validates an OpenAPI specification from a URL
func (f *Fetcher) FetchSpec(ctx context.Context, specURL, serviceName string, headers map[string]string, ttl time.Duration) (*models.SpecInfo, error) {
	// Validate URL
	if _, err := url.Parse(specURL); err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	f.logger.Info("Fetching OpenAPI spec",
		zap.String("url", specURL),
		zap.String("serviceName", serviceName))

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", specURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Set Accept header for content negotiation
	req.Header.Set("Accept", "application/json, application/yaml, text/yaml")

	// Make request
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch spec: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Read response with size limit
	body, err := f.readLimitedBody(resp.Body, f.maxSize)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse OpenAPI spec
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false // Security: disable external refs

	spec, err := loader.LoadFromData(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}

	// Validate spec
	if err := spec.Validate(ctx); err != nil {
		return nil, fmt.Errorf("invalid OpenAPI spec: %w", err)
	}

	var pathCount int
	if spec.Paths != nil {
		pathCount = len(spec.Paths.Map())
	}

	f.logger.Info("Successfully fetched and validated OpenAPI spec",
		zap.String("url", specURL),
		zap.String("serviceName", serviceName),
		zap.String("title", spec.Info.Title),
		zap.String("version", spec.Info.Version),
		zap.Int("pathCount", pathCount))

	return &models.SpecInfo{
		ID:          generateSpecID(serviceName, specURL),
		ServiceName: serviceName,
		URL:         specURL,
		Spec:        spec,
		FetchedAt:   time.Now(),
		TTL:         ttl,
		Headers:     headers,
	}, nil
}

// ValidateSpec validates an OpenAPI specification without fetching
func (f *Fetcher) ValidateSpec(ctx context.Context, spec *openapi3.T) error {
	return spec.Validate(ctx)
}

// readLimitedBody reads response body with size limit
func (f *Fetcher) readLimitedBody(body io.Reader, maxSize int64) ([]byte, error) {
	if maxSize <= 0 {
		maxSize = 10 * 1024 * 1024 // 10MB default
	}

	limitedReader := io.LimitReader(body, maxSize+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
	}

	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("response too large: %d bytes (max: %d)", len(data), maxSize)
	}

	return data, nil
}

// generateSpecID creates a unique identifier for a spec
func generateSpecID(serviceName, specURL string) string {
	return fmt.Sprintf("%s:%s", serviceName, specURL)
}