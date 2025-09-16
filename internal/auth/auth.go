package auth

import (
	"context"
	"crypto/rsa"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zeroLR/swagger-mcp-go/internal/models"
	"go.uber.org/zap"
)

// Provider interface for authentication providers
type Provider interface {
	// Authenticate validates credentials and returns authentication context
	Authenticate(ctx context.Context, request *http.Request) (*AuthContext, error)
	// Type returns the authentication type
	Type() models.AuthType
	// Configure sets up the provider with configuration
	Configure(config map[string]interface{}) error
}

// AuthContext contains authentication information
type AuthContext struct {
	UserID   string                 `json:"userId"`
	Username string                 `json:"username"`
	Scopes   []string               `json:"scopes"`
	Claims   map[string]interface{} `json:"claims"`
	Valid    bool                   `json:"valid"`
}

// Manager manages multiple authentication providers
type Manager struct {
	providers map[models.AuthType]Provider
	logger    *zap.Logger
}

// NewManager creates a new authentication manager
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		providers: make(map[models.AuthType]Provider),
		logger:    logger,
	}
}

// RegisterProvider registers an authentication provider
func (m *Manager) RegisterProvider(authType models.AuthType, provider Provider) {
	m.providers[authType] = provider
	m.logger.Info("Registered authentication provider", zap.String("type", string(authType)))
}

// Authenticate attempts authentication using the specified policy
func (m *Manager) Authenticate(ctx context.Context, request *http.Request, policy *models.AuthPolicy) (*AuthContext, error) {
	if !policy.Required {
		// Authentication is optional, return valid context
		return &AuthContext{Valid: true}, nil
	}

	provider, exists := m.providers[policy.Type]
	if !exists {
		return nil, fmt.Errorf("authentication provider not found: %s", policy.Type)
	}

	authCtx, err := provider.Authenticate(ctx, request)
	if err != nil {
		m.logger.Debug("Authentication failed",
			zap.String("type", string(policy.Type)),
			zap.Error(err))
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	// Validate required scopes
	if len(policy.Scopes) > 0 {
		if !m.hasRequiredScopes(authCtx.Scopes, policy.Scopes) {
			return nil, fmt.Errorf("insufficient scopes: required %v, got %v", policy.Scopes, authCtx.Scopes)
		}
	}

	return authCtx, nil
}

// hasRequiredScopes checks if the user has all required scopes
func (m *Manager) hasRequiredScopes(userScopes, requiredScopes []string) bool {
	userScopeMap := make(map[string]bool)
	for _, scope := range userScopes {
		userScopeMap[scope] = true
	}

	for _, required := range requiredScopes {
		if !userScopeMap[required] {
			return false
		}
	}

	return true
}

// BasicAuthProvider implements basic authentication
type BasicAuthProvider struct {
	users  map[string]string // username -> password
	logger *zap.Logger
}

// NewBasicAuthProvider creates a new basic auth provider
func NewBasicAuthProvider(logger *zap.Logger) *BasicAuthProvider {
	return &BasicAuthProvider{
		users:  make(map[string]string),
		logger: logger,
	}
}

// Type returns the authentication type
func (p *BasicAuthProvider) Type() models.AuthType {
	return models.AuthTypeBasic
}

// Configure sets up the basic auth provider
func (p *BasicAuthProvider) Configure(config map[string]interface{}) error {
	if users, ok := config["users"].(map[string]interface{}); ok {
		for username, password := range users {
			if passwordStr, ok := password.(string); ok {
				p.users[username] = passwordStr
			}
		}
	}
	return nil
}

// Authenticate validates basic authentication credentials
func (p *BasicAuthProvider) Authenticate(ctx context.Context, request *http.Request) (*AuthContext, error) {
	username, password, ok := request.BasicAuth()
	if !ok {
		return nil, fmt.Errorf("basic auth credentials not provided")
	}

	if storedPassword, exists := p.users[username]; exists && storedPassword == password {
		return &AuthContext{
			UserID:   username,
			Username: username,
			Valid:    true,
		}, nil
	}

	return nil, fmt.Errorf("invalid credentials")
}

// BearerTokenProvider implements JWT bearer token authentication
type BearerTokenProvider struct {
	publicKey  *rsa.PublicKey
	issuer     string
	audience   string
	jwksURL    string
	httpClient *http.Client
	logger     *zap.Logger
}

// NewBearerTokenProvider creates a new bearer token provider
func NewBearerTokenProvider(logger *zap.Logger) *BearerTokenProvider {
	return &BearerTokenProvider{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		logger:     logger,
	}
}

// Type returns the authentication type
func (p *BearerTokenProvider) Type() models.AuthType {
	return models.AuthTypeBearer
}

// Configure sets up the bearer token provider
func (p *BearerTokenProvider) Configure(config map[string]interface{}) error {
	if issuer, ok := config["issuer"].(string); ok {
		p.issuer = issuer
	}
	if audience, ok := config["audience"].(string); ok {
		p.audience = audience
	}
	if jwksURL, ok := config["jwksURL"].(string); ok {
		p.jwksURL = jwksURL
	}
	return nil
}

// Authenticate validates JWT bearer tokens
func (p *BearerTokenProvider) Authenticate(ctx context.Context, request *http.Request) (*AuthContext, error) {
	authHeader := request.Header.Get("Authorization")
	if authHeader == "" {
		return nil, fmt.Errorf("authorization header not provided")
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, fmt.Errorf("invalid authorization header format")
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	// Parse the token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// TODO: Implement JWKS key resolution
		// For now, return the configured public key
		return p.publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Validate issuer and audience
	if p.issuer != "" {
		if iss, ok := claims["iss"].(string); !ok || iss != p.issuer {
			return nil, fmt.Errorf("invalid issuer")
		}
	}

	if p.audience != "" {
		if aud, ok := claims["aud"].(string); !ok || aud != p.audience {
			return nil, fmt.Errorf("invalid audience")
		}
	}

	// Extract user information
	var userID, username string
	var scopes []string

	if sub, ok := claims["sub"].(string); ok {
		userID = sub
	}
	if name, ok := claims["name"].(string); ok {
		username = name
	} else if preferred, ok := claims["preferred_username"].(string); ok {
		username = preferred
	}

	if scope, ok := claims["scope"].(string); ok {
		scopes = strings.Split(scope, " ")
	}

	return &AuthContext{
		UserID:   userID,
		Username: username,
		Scopes:   scopes,
		Claims:   claims,
		Valid:    true,
	}, nil
}

// APIKeyProvider implements API key authentication
type APIKeyProvider struct {
	keys      map[string]*APIKeyInfo // API key -> key info
	headerKey string                 // Header name for API key (default: "X-API-Key")
	queryKey  string                 // Query parameter name for API key
	logger    *zap.Logger
}

// APIKeyInfo contains information about an API key
type APIKeyInfo struct {
	UserID   string   `json:"userId"`
	Username string   `json:"username"`
	Scopes   []string `json:"scopes"`
	Active   bool     `json:"active"`
}

// NewAPIKeyProvider creates a new API key provider
func NewAPIKeyProvider(logger *zap.Logger) *APIKeyProvider {
	return &APIKeyProvider{
		keys:      make(map[string]*APIKeyInfo),
		headerKey: "X-API-Key",
		logger:    logger,
	}
}

// Type returns the authentication type
func (p *APIKeyProvider) Type() models.AuthType {
	return models.AuthTypeAPIKey
}

// Configure sets up the API key provider
func (p *APIKeyProvider) Configure(config map[string]interface{}) error {
	if headerKey, ok := config["headerKey"].(string); ok {
		p.headerKey = headerKey
	}
	if queryKey, ok := config["queryKey"].(string); ok {
		p.queryKey = queryKey
	}
	if keys, ok := config["keys"].(map[string]interface{}); ok {
		for apiKey, keyData := range keys {
			if keyInfo, ok := keyData.(map[string]interface{}); ok {
				info := &APIKeyInfo{Active: true}
				if userID, ok := keyInfo["userId"].(string); ok {
					info.UserID = userID
				}
				if username, ok := keyInfo["username"].(string); ok {
					info.Username = username
				}
				if scopes, ok := keyInfo["scopes"].([]interface{}); ok {
					info.Scopes = make([]string, len(scopes))
					for i, scope := range scopes {
						if scopeStr, ok := scope.(string); ok {
							info.Scopes[i] = scopeStr
						}
					}
				}
				if active, ok := keyInfo["active"].(bool); ok {
					info.Active = active
				}
				p.keys[apiKey] = info
			}
		}
	}
	return nil
}

// Authenticate validates API key authentication
func (p *APIKeyProvider) Authenticate(ctx context.Context, request *http.Request) (*AuthContext, error) {
	var apiKey string

	// Try header first
	if p.headerKey != "" {
		apiKey = request.Header.Get(p.headerKey)
	}

	// Try query parameter if header not found
	if apiKey == "" && p.queryKey != "" {
		apiKey = request.URL.Query().Get(p.queryKey)
	}

	if apiKey == "" {
		return nil, fmt.Errorf("API key not provided")
	}

	keyInfo, exists := p.keys[apiKey]
	if !exists || !keyInfo.Active {
		return nil, fmt.Errorf("invalid or inactive API key")
	}

	return &AuthContext{
		UserID:   keyInfo.UserID,
		Username: keyInfo.Username,
		Scopes:   keyInfo.Scopes,
		Valid:    true,
	}, nil
}

// OAuth2Provider implements OAuth2 client credentials flow
type OAuth2Provider struct {
	tokenURL     string
	clientID     string
	clientSecret string
	httpClient   *http.Client
	logger       *zap.Logger
}

// NewOAuth2Provider creates a new OAuth2 provider
func NewOAuth2Provider(logger *zap.Logger) *OAuth2Provider {
	return &OAuth2Provider{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		logger:     logger,
	}
}

// Type returns the authentication type
func (p *OAuth2Provider) Type() models.AuthType {
	return models.AuthTypeOAuth2
}

// Configure sets up the OAuth2 provider
func (p *OAuth2Provider) Configure(config map[string]interface{}) error {
	if tokenURL, ok := config["tokenURL"].(string); ok {
		p.tokenURL = tokenURL
	}
	if clientID, ok := config["clientID"].(string); ok {
		p.clientID = clientID
	}
	if clientSecret, ok := config["clientSecret"].(string); ok {
		p.clientSecret = clientSecret
	}
	return nil
}

// Authenticate validates OAuth2 tokens
func (p *OAuth2Provider) Authenticate(ctx context.Context, request *http.Request) (*AuthContext, error) {
	authHeader := request.Header.Get("Authorization")
	if authHeader == "" {
		return nil, fmt.Errorf("authorization header not provided")
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, fmt.Errorf("invalid authorization header format")
	}

	accessToken := strings.TrimPrefix(authHeader, "Bearer ")

	// TODO: Implement token introspection against OAuth2 server
	// For now, accept any non-empty token as valid
	if accessToken == "" {
		return nil, fmt.Errorf("empty access token")
	}

	return &AuthContext{
		UserID: "oauth2-user",
		Valid:  true,
	}, nil
}

// Middleware creates an HTTP middleware for authentication
func (m *Manager) Middleware(policy *models.AuthPolicy) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if policy == nil || !policy.Required {
				// No authentication required
				next.ServeHTTP(w, r)
				return
			}

			authCtx, err := m.Authenticate(r.Context(), r, policy)
			if err != nil {
				m.logger.Debug("Authentication failed", zap.Error(err))
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Add auth context to request context
			ctx := context.WithValue(r.Context(), "authContext", authCtx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetAuthContext retrieves authentication context from request context
func GetAuthContext(ctx context.Context) (*AuthContext, bool) {
	authCtx, ok := ctx.Value("authContext").(*AuthContext)
	return authCtx, ok
}
