package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zeroLR/swagger-mcp-go/internal/parser"
	"go.uber.org/zap"
)

// Engine handles proxying requests to upstream APIs
type Engine struct {
	client  *http.Client
	logger  *zap.Logger
	baseURL string
	headers map[string]string
}

// Response represents a proxy response
type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// New creates a new proxy engine
func New(logger *zap.Logger, timeout time.Duration) *Engine {
	return &Engine{
		client: &http.Client{
			Timeout: timeout,
		},
		logger:  logger,
		headers: make(map[string]string),
	}
}

// SetBaseURL sets the base URL for upstream requests
func (e *Engine) SetBaseURL(baseURL string) {
	e.baseURL = strings.TrimSuffix(baseURL, "/")
}

// SetHeaders sets default headers for upstream requests
func (e *Engine) SetHeaders(headers map[string]string) {
	e.headers = headers
}

// ExecuteRoute executes a route with the given parameters
func (e *Engine) ExecuteRoute(ctx context.Context, route *parser.RouteConfig, params map[string]interface{}) (*Response, error) {
	// Build the URL with path parameters
	reqURL, err := e.buildURL(route.Path, params)
	if err != nil {
		return nil, fmt.Errorf("failed to build URL: %w", err)
	}

	// Create request
	req, err := e.createRequest(ctx, route, reqURL, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request
	e.logger.Debug("Executing proxy request",
		zap.String("method", req.Method),
		zap.String("url", req.URL.String()),
		zap.String("operationID", route.OperationID))

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	response := &Response{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       body,
	}

	e.logger.Debug("Proxy request completed",
		zap.String("operationID", route.OperationID),
		zap.Int("statusCode", resp.StatusCode),
		zap.Int("bodySize", len(body)))

	return response, nil
}

// buildURL constructs the full URL with path parameters
func (e *Engine) buildURL(path string, params map[string]interface{}) (string, error) {
	fullPath := path

	// Replace path parameters
	for paramName, paramValue := range params {
		placeholder := "{" + paramName + "}"
		if strings.Contains(fullPath, placeholder) {
			fullPath = strings.ReplaceAll(fullPath, placeholder, fmt.Sprintf("%v", paramValue))
		}
	}

	// Build full URL
	fullURL := e.baseURL + fullPath

	// Add query parameters
	queryParams := make(map[string][]string)
	for paramName, paramValue := range params {
		// Skip path parameters and body
		if strings.Contains(path, "{"+paramName+"}") || paramName == "body" {
			continue
		}
		if queryParams[paramName] == nil {
			queryParams[paramName] = make([]string, 0)
		}
		queryParams[paramName] = append(queryParams[paramName], fmt.Sprintf("%v", paramValue))
	}

	if len(queryParams) > 0 {
		values := url.Values(queryParams)
		fullURL += "?" + values.Encode()
	}

	return fullURL, nil
}

// createRequest creates an HTTP request from route config and parameters
func (e *Engine) createRequest(ctx context.Context, route *parser.RouteConfig, reqURL string, params map[string]interface{}) (*http.Request, error) {
	var body io.Reader
	var contentType string

	if route.RequestBody != nil {
		if bodyData, ok := params["body"]; ok {
			b, ct, err := buildRequestBody(route, bodyData)
			if err != nil {
				return nil, err
			}
			body, contentType = b, ct
		}
	}

	req, err := http.NewRequestWithContext(ctx, route.Method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	addDefaultHeaders(req, e.headers)
	addParameterHeaders(req, route.Parameters, params)

	return req, nil
}

// buildRequestBody constructs the request body and content type according to route config
func buildRequestBody(route *parser.RouteConfig, bodyData interface{}) (io.Reader, string, error) {
	switch route.RequestBody.ContentType {
	case "application/json":
		jsonData, err := json.Marshal(bodyData)
		if err != nil {
			return nil, "", fmt.Errorf("failed to marshal JSON body: %w", err)
		}
		return bytes.NewReader(jsonData), "application/json", nil
	case "application/x-www-form-urlencoded":
		values := make(map[string][]string)
		if bodyMap, ok := bodyData.(map[string]interface{}); ok {
			for key, value := range bodyMap {
				if values[key] == nil {
					values[key] = make([]string, 0)
				}
				values[key] = append(values[key], fmt.Sprintf("%v", value))
			}
		}
		urlValues := url.Values(values)
		return strings.NewReader(urlValues.Encode()), "application/x-www-form-urlencoded", nil
	case "text/plain":
		return strings.NewReader(fmt.Sprintf("%v", bodyData)), "text/plain", nil
	default:
		jsonData, err := json.Marshal(bodyData)
		if err != nil {
			return strings.NewReader(fmt.Sprintf("%v", bodyData)), "text/plain", nil
		}
		return bytes.NewReader(jsonData), "application/json", nil
	}
}

// addDefaultHeaders applies default engine headers
func addDefaultHeaders(req *http.Request, headers map[string]string) {
	for key, value := range headers {
		req.Header.Set(key, value)
	}
}

// addParameterHeaders applies header parameters from the route configuration
func addParameterHeaders(req *http.Request, parameters []parser.ParameterConfig, params map[string]interface{}) {
	for _, param := range parameters {
		if param.In == "header" {
			if value, exists := params[param.Name]; exists {
				req.Header.Set(param.Name, fmt.Sprintf("%v", value))
			}
		}
	}
}

// GetExecutor returns a function that can execute a specific route
func (e *Engine) GetExecutor(route *parser.RouteConfig) func(context.Context, map[string]interface{}) (*Response, error) {
	return func(ctx context.Context, params map[string]interface{}) (*Response, error) {
		return e.ExecuteRoute(ctx, route, params)
	}
}
