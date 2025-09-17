package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zeroLR/swagger-mcp-go/internal/models"
	"go.uber.org/zap"
)

func TestBasicAuthProvider(t *testing.T) {
	logger := zap.NewNop()
	provider := NewBasicAuthProvider(logger)

	// Configure with test users
	config := map[string]interface{}{
		"users": map[string]interface{}{
			"testuser": "testpass",
			"admin":    "secret",
		},
	}
	err := provider.Configure(config)
	if err != nil {
		t.Fatalf("Failed to configure provider: %v", err)
	}

	tests := []struct {
		name     string
		username string
		password string
		wantErr  bool
	}{
		{"valid credentials", "testuser", "testpass", false},
		{"valid admin", "admin", "secret", false},
		{"invalid password", "testuser", "wrongpass", true},
		{"invalid user", "nonexistent", "testpass", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.SetBasicAuth(tt.username, tt.password)

			authCtx, err := provider.Authenticate(context.Background(), req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Authenticate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if authCtx == nil || !authCtx.Valid {
					t.Errorf("Expected valid auth context")
				}
				if authCtx.Username != tt.username {
					t.Errorf("Expected username %s, got %s", tt.username, authCtx.Username)
				}
			}
		})
	}
}

func TestAPIKeyProvider(t *testing.T) {
	logger := zap.NewNop()
	provider := NewAPIKeyProvider(logger)

	// Configure with test API keys
	config := map[string]interface{}{
		"headerKey": "X-API-Key",
		"queryKey":  "api_key",
		"keys": map[string]interface{}{
			"test-key-123": map[string]interface{}{
				"userId":   "user1",
				"username": "testuser",
				"scopes":   []interface{}{"read", "write"},
				"active":   true,
			},
			"inactive-key": map[string]interface{}{
				"userId":   "user2",
				"username": "inactive",
				"active":   false,
			},
		},
	}
	err := provider.Configure(config)
	if err != nil {
		t.Fatalf("Failed to configure provider: %v", err)
	}

	tests := []struct {
		name    string
		apiKey  string
		inQuery bool
		wantErr bool
	}{
		{"valid header key", "test-key-123", false, false},
		{"valid query key", "test-key-123", true, false},
		{"inactive key", "inactive-key", false, true},
		{"invalid key", "invalid-key", false, true},
		{"empty key", "", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)

			if tt.inQuery {
				q := req.URL.Query()
				q.Add("api_key", tt.apiKey)
				req.URL.RawQuery = q.Encode()
			} else {
				req.Header.Set("X-API-Key", tt.apiKey)
			}

			authCtx, err := provider.Authenticate(context.Background(), req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Authenticate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if authCtx == nil || !authCtx.Valid {
					t.Errorf("Expected valid auth context")
				}
				if tt.apiKey == "test-key-123" {
					if authCtx.Username != "testuser" {
						t.Errorf("Expected username testuser, got %s", authCtx.Username)
					}
					if len(authCtx.Scopes) != 2 {
						t.Errorf("Expected 2 scopes, got %d", len(authCtx.Scopes))
					}
				}
			}
		})
	}
}

func TestManager(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger)

	// Register providers
	basicProvider := NewBasicAuthProvider(logger)
	basicProvider.Configure(map[string]interface{}{
		"users": map[string]interface{}{
			"testuser": "testpass",
		},
	})
	manager.RegisterProvider(models.AuthTypeBasic, basicProvider)

	apiKeyProvider := NewAPIKeyProvider(logger)
	apiKeyProvider.Configure(map[string]interface{}{
		"keys": map[string]interface{}{
			"test-key": map[string]interface{}{
				"userId":   "user1",
				"username": "testuser",
				"active":   true,
			},
		},
	})
	manager.RegisterProvider(models.AuthTypeAPIKey, apiKeyProvider)

	tests := []struct {
		name     string
		policy   *models.AuthPolicy
		setupReq func(*http.Request)
		wantErr  bool
	}{
		{
			name: "optional auth",
			policy: &models.AuthPolicy{
				Type:     models.AuthTypeBasic,
				Required: false,
			},
			setupReq: func(req *http.Request) {},
			wantErr:  false,
		},
		{
			name: "valid basic auth",
			policy: &models.AuthPolicy{
				Type:     models.AuthTypeBasic,
				Required: true,
			},
			setupReq: func(req *http.Request) {
				req.SetBasicAuth("testuser", "testpass")
			},
			wantErr: false,
		},
		{
			name: "invalid basic auth",
			policy: &models.AuthPolicy{
				Type:     models.AuthTypeBasic,
				Required: true,
			},
			setupReq: func(req *http.Request) {
				req.SetBasicAuth("testuser", "wrongpass")
			},
			wantErr: true,
		},
		{
			name: "valid api key",
			policy: &models.AuthPolicy{
				Type:     models.AuthTypeAPIKey,
				Required: true,
			},
			setupReq: func(req *http.Request) {
				req.Header.Set("X-API-Key", "test-key")
			},
			wantErr: false,
		},
		{
			name: "missing provider",
			policy: &models.AuthPolicy{
				Type:     models.AuthTypeOAuth2,
				Required: true,
			},
			setupReq: func(req *http.Request) {},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			tt.setupReq(req)

			authCtx, err := manager.Authenticate(context.Background(), req, tt.policy)
			if (err != nil) != tt.wantErr {
				t.Errorf("Authenticate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && (authCtx == nil || !authCtx.Valid) {
				t.Errorf("Expected valid auth context")
			}
		})
	}
}

func TestAuthMiddleware(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(logger)

	// Register basic auth provider
	basicProvider := NewBasicAuthProvider(logger)
	basicProvider.Configure(map[string]interface{}{
		"users": map[string]interface{}{
			"testuser": "testpass",
		},
	})
	manager.RegisterProvider(models.AuthTypeBasic, basicProvider)

	policy := &models.AuthPolicy{
		Type:     models.AuthTypeBasic,
		Required: true,
	}

	middleware := manager.Middleware(policy)

	// Test handler that checks auth context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCtx, ok := GetAuthContext(r.Context())
		if !ok || !authCtx.Valid {
			http.Error(w, "No auth context", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("authenticated"))
	})

	wrappedHandler := middleware(handler)

	tests := []struct {
		name           string
		setupReq       func(*http.Request)
		expectedStatus int
	}{
		{
			name: "valid auth",
			setupReq: func(req *http.Request) {
				req.SetBasicAuth("testuser", "testpass")
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "invalid auth",
			setupReq: func(req *http.Request) {
				req.SetBasicAuth("testuser", "wrongpass")
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "no auth",
			setupReq:       func(req *http.Request) {},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			tt.setupReq(req)

			recorder := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(recorder, req)

			if recorder.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, recorder.Code)
			}
		})
	}
}

func TestOAuth2Provider(t *testing.T) {
	logger := zap.NewNop()
	provider := NewOAuth2Provider(logger)

	// Configure OAuth2 provider
	config := map[string]interface{}{
		"tokenURL":     "https://auth.example.com/token",
		"clientID":     "test-client",
		"clientSecret": "test-secret",
		"scopes":       []interface{}{"read", "write"},
	}
	err := provider.Configure(config)
	if err != nil {
		t.Fatalf("Failed to configure OAuth2 provider: %v", err)
	}

	// Test type
	if provider.Type() != models.AuthTypeOAuth2 {
		t.Errorf("Expected type %s, got %s", models.AuthTypeOAuth2, provider.Type())
	}

	// Test basic token validation (without introspection URL)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer valid-token")

	authCtx, err := provider.Authenticate(context.Background(), req)
	if err != nil {
		t.Errorf("Authenticate should not fail with valid token: %v", err)
	}
	if authCtx == nil || !authCtx.Valid {
		t.Errorf("Expected valid auth context")
	}

	// Test invalid token format
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Invalid token")

	_, err = provider.Authenticate(context.Background(), req)
	if err == nil {
		t.Errorf("Expected error for invalid token format")
	}

	// Test missing authorization header
	req = httptest.NewRequest("GET", "/", nil)
	_, err = provider.Authenticate(context.Background(), req)
	if err == nil {
		t.Errorf("Expected error for missing authorization header")
	}
}

func TestOAuth2AuthorizationURL(t *testing.T) {
	logger := zap.NewNop()
	provider := NewOAuth2Provider(logger)

	config := map[string]interface{}{
		"authorizationURL": "https://auth.example.com/authorize",
		"clientID":         "test-client",
		"scopes":           []interface{}{"read", "write"},
	}
	err := provider.Configure(config)
	if err != nil {
		t.Fatalf("Failed to configure OAuth2 provider: %v", err)
	}

	authURL, err := provider.GetAuthorizationURL("https://app.example.com/callback", "test-state", "")
	if err != nil {
		t.Fatalf("Failed to get authorization URL: %v", err)
	}

	if !strings.Contains(authURL, "https://auth.example.com/authorize") {
		t.Errorf("Authorization URL should contain base URL")
	}
	if !strings.Contains(authURL, "client_id=test-client") {
		t.Errorf("Authorization URL should contain client ID")
	}
	if !strings.Contains(authURL, "redirect_uri=") {
		t.Errorf("Authorization URL should contain redirect URI")
	}
	if !strings.Contains(authURL, "state=test-state") {
		t.Errorf("Authorization URL should contain state")
	}
}
