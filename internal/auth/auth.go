package auth

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

// JWKSResponse represents a JWKS response
type JWKSResponse struct {
	Keys []JWK `json:"keys"`
}

// JWK represents a JSON Web Key
type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
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

		// Try to get key from JWKS if configured
		if p.jwksURL != "" {
			return p.getJWKSKey(token)
		}

		// Fallback to configured public key
		if p.publicKey == nil {
			return nil, fmt.Errorf("no public key or JWKS URL configured")
		}
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

// getJWKSKey retrieves the public key from JWKS endpoint
func (p *BearerTokenProvider) getJWKSKey(token *jwt.Token) (interface{}, error) {
	// Get the key ID from token header
	kid, ok := token.Header["kid"].(string)
	if !ok {
		return nil, fmt.Errorf("token missing key ID")
	}

	// Fetch JWKS
	resp, err := p.httpClient.Get(p.jwksURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	var jwks JWKSResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("failed to decode JWKS response: %w", err)
	}

	// Find the key with matching kid
	for _, jwk := range jwks.Keys {
		if jwk.Kid == kid && jwk.Kty == "RSA" {
			return p.parseRSAKey(&jwk)
		}
	}

	return nil, fmt.Errorf("key with ID %s not found in JWKS", kid)
}

// parseRSAKey converts a JWK to an RSA public key
func (p *BearerTokenProvider) parseRSAKey(jwk *JWK) (*rsa.PublicKey, error) {
	// This is a simplified implementation
	// In production, you'd want to use a proper JWK library
	// like github.com/lestrrat-go/jwx or similar
	return nil, fmt.Errorf("JWK parsing not implemented - use proper JWK library in production")
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
	tokenURL           string
	introspectionURL   string
	authorizationURL   string
	clientID           string
	clientSecret       string
	scopes             []string
	httpClient         *http.Client
	logger             *zap.Logger
}

// OAuth2TokenResponse represents the response from token endpoint
type OAuth2TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// OAuth2IntrospectionResponse represents the response from introspection endpoint
type OAuth2IntrospectionResponse struct {
	Active    bool   `json:"active"`
	ClientID  string `json:"client_id,omitempty"`
	Username  string `json:"username,omitempty"`
	Subject   string `json:"sub,omitempty"`
	Scope     string `json:"scope,omitempty"`
	ExpiresAt int64  `json:"exp,omitempty"`
	IssuedAt  int64  `json:"iat,omitempty"`
	TokenType string `json:"token_type,omitempty"`
}

// OAuth2AuthorizationCodeRequest represents authorization code flow request
type OAuth2AuthorizationCodeRequest struct {
	ClientID     string `json:"client_id"`
	RedirectURI  string `json:"redirect_uri"`
	Scope        string `json:"scope"`
	State        string `json:"state"`
	CodeVerifier string `json:"code_verifier,omitempty"` // PKCE
}

// NewOAuth2Provider creates a new OAuth2 provider
func NewOAuth2Provider(logger *zap.Logger) *OAuth2Provider {
	return &OAuth2Provider{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		scopes:     []string{},
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
	if introspectionURL, ok := config["introspectionURL"].(string); ok {
		p.introspectionURL = introspectionURL
	}
	if authorizationURL, ok := config["authorizationURL"].(string); ok {
		p.authorizationURL = authorizationURL
	}
	if clientID, ok := config["clientID"].(string); ok {
		p.clientID = clientID
	}
	if clientSecret, ok := config["clientSecret"].(string); ok {
		p.clientSecret = clientSecret
	}
	if scopes, ok := config["scopes"].([]interface{}); ok {
		p.scopes = make([]string, len(scopes))
		for i, scope := range scopes {
			if scopeStr, ok := scope.(string); ok {
				p.scopes[i] = scopeStr
			}
		}
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
	if accessToken == "" {
		return nil, fmt.Errorf("empty access token")
	}

	// If introspection URL is configured, validate token via introspection
	if p.introspectionURL != "" {
		return p.introspectToken(ctx, accessToken)
	}

	// Fallback: basic token validation (just check if token is not empty)
	p.logger.Warn("OAuth2 introspection URL not configured, using basic validation")
	return &AuthContext{
		UserID: "oauth2-user",
		Valid:  true,
	}, nil
}

// introspectToken validates token using OAuth2 introspection endpoint
func (p *OAuth2Provider) introspectToken(ctx context.Context, token string) (*AuthContext, error) {
	// Prepare introspection request
	data := url.Values{}
	data.Set("token", token)
	data.Set("token_type_hint", "access_token")

	req, err := http.NewRequestWithContext(ctx, "POST", p.introspectionURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create introspection request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(p.clientID, p.clientSecret)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("introspection request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("introspection failed with status %d", resp.StatusCode)
	}

	var introspectionResp OAuth2IntrospectionResponse
	if err := json.NewDecoder(resp.Body).Decode(&introspectionResp); err != nil {
		return nil, fmt.Errorf("failed to decode introspection response: %w", err)
	}

	if !introspectionResp.Active {
		return nil, fmt.Errorf("token is not active")
	}

	// Check token expiration
	if introspectionResp.ExpiresAt > 0 && time.Now().Unix() > introspectionResp.ExpiresAt {
		return nil, fmt.Errorf("token has expired")
	}

	// Extract scopes
	var scopes []string
	if introspectionResp.Scope != "" {
		scopes = strings.Split(introspectionResp.Scope, " ")
	}

	username := introspectionResp.Username
	if username == "" {
		username = introspectionResp.Subject
	}

	return &AuthContext{
		UserID:   introspectionResp.Subject,
		Username: username,
		Scopes:   scopes,
		Claims: map[string]interface{}{
			"client_id":  introspectionResp.ClientID,
			"token_type": introspectionResp.TokenType,
			"exp":        introspectionResp.ExpiresAt,
			"iat":        introspectionResp.IssuedAt,
		},
		Valid: true,
	}, nil
}

// GetClientCredentialsToken obtains a token using client credentials flow
func (p *OAuth2Provider) GetClientCredentialsToken(ctx context.Context) (*OAuth2TokenResponse, error) {
	if p.tokenURL == "" {
		return nil, fmt.Errorf("token URL not configured")
	}

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	if len(p.scopes) > 0 {
		data.Set("scope", strings.Join(p.scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(p.clientID, p.clientSecret)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed with status %d", resp.StatusCode)
	}

	var tokenResp OAuth2TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokenResp, nil
}

// ExchangeAuthorizationCode exchanges an authorization code for tokens
func (p *OAuth2Provider) ExchangeAuthorizationCode(ctx context.Context, code, redirectURI, codeVerifier string) (*OAuth2TokenResponse, error) {
	if p.tokenURL == "" {
		return nil, fmt.Errorf("token URL not configured")
	}

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", p.clientID)
	
	// PKCE support
	if codeVerifier != "" {
		data.Set("code_verifier", codeVerifier)
	} else {
		// Use client secret if no PKCE
		data.Set("client_secret", p.clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token exchange request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d", resp.StatusCode)
	}

	var tokenResp OAuth2TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokenResp, nil
}

// GetAuthorizationURL generates an authorization URL for the authorization code flow
func (p *OAuth2Provider) GetAuthorizationURL(redirectURI, state, codeChallenge string) (string, error) {
	if p.authorizationURL == "" {
		return "", fmt.Errorf("authorization URL not configured")
	}

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", p.clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("state", state)
	
	if len(p.scopes) > 0 {
		params.Set("scope", strings.Join(p.scopes, " "))
	}
	
	// PKCE support
	if codeChallenge != "" {
		params.Set("code_challenge", codeChallenge)
		params.Set("code_challenge_method", "S256")
	}

	authURL, err := url.Parse(p.authorizationURL)
	if err != nil {
		return "", fmt.Errorf("invalid authorization URL: %w", err)
	}

	authURL.RawQuery = params.Encode()
	return authURL.String(), nil
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
