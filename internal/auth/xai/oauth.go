package xai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	log "github.com/sirupsen/logrus"
)

const (
	Provider            = "xai-oauth"
	Issuer              = "https://auth.x.ai"
	DiscoveryURL        = Issuer + "/.well-known/openid-configuration"
	ClientID            = "b1a00492-073a-47ea-816f-4c329264a828"
	Scope               = "openid profile email offline_access grok-cli:access api:access"
	DefaultBaseURL      = "https://api.x.ai/v1"
	DefaultCallbackPort = 56121
	CallbackHost        = "127.0.0.1"
	CallbackPath        = "/callback"
	Referrer            = "cliproxyapiplus"
)

// DiscoveryDocument captures the OpenID Provider metadata needed by the xAI OAuth flow.
type DiscoveryDocument struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	UserInfoEndpoint      string   `json:"userinfo_endpoint"`
	ScopesSupported       []string `json:"scopes_supported"`
	GrantTypesSupported   []string `json:"grant_types_supported"`
	PKCESupportedMethods  []string `json:"code_challenge_methods_supported"`
}

// TokenResponse represents the OAuth token response from xAI.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// UserIdentity is decoded from ID/access token claims when those tokens are JWTs.
type UserIdentity struct {
	Subject           string
	Email             string
	Name              string
	PreferredUsername string
}

// OAuth handles xAI OAuth discovery and token operations.
type OAuth struct {
	httpClient   *http.Client
	discoveryURL string
}

// NewOAuth creates a new xAI OAuth service.
func NewOAuth(cfg *config.Config, httpClient *http.Client) *OAuth {
	return NewOAuthWithDiscoveryURL(cfg, httpClient, DiscoveryURL)
}

// NewOAuthWithProxyURL creates a new xAI OAuth service with an auth-specific proxy override.
func NewOAuthWithProxyURL(cfg *config.Config, proxyURL string) *OAuth {
	if cfg == nil {
		cfg = &config.Config{}
	}
	copied := *cfg
	if strings.TrimSpace(proxyURL) != "" {
		copied.SDKConfig.ProxyURL = strings.TrimSpace(proxyURL)
	}
	return NewOAuth(&copied, nil)
}

// NewOAuthWithDiscoveryURL creates an OAuth service with an overrideable discovery URL for tests.
func NewOAuthWithDiscoveryURL(cfg *config.Config, httpClient *http.Client, discoveryURL string) *OAuth {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if httpClient == nil {
		httpClient = util.SetProxy(&cfg.SDKConfig, &http.Client{})
	}
	discoveryURL = strings.TrimSpace(discoveryURL)
	if discoveryURL == "" {
		discoveryURL = DiscoveryURL
	}
	return &OAuth{httpClient: httpClient, discoveryURL: discoveryURL}
}

// Discover loads and validates xAI's OIDC metadata.
func (o *OAuth) Discover(ctx context.Context) (*DiscoveryDocument, error) {
	if o == nil {
		return nil, fmt.Errorf("xai oauth discovery: service is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.discoveryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("xai oauth discovery: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, errDo := o.httpClient.Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("xai oauth discovery: execute request: %w", errDo)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Errorf("xai oauth discovery: close body error: %v", errClose)
		}
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, errRead := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		if errRead != nil {
			return nil, fmt.Errorf("xai oauth discovery: read error body: %w", errRead)
		}
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("xai oauth discovery: request failed: status %d: %s", resp.StatusCode, msg)
	}

	var doc DiscoveryDocument
	if errDecode := json.NewDecoder(resp.Body).Decode(&doc); errDecode != nil {
		return nil, fmt.Errorf("xai oauth discovery: decode response: %w", errDecode)
	}
	if errValidate := ValidateDiscovery(&doc); errValidate != nil {
		return nil, errValidate
	}
	return &doc, nil
}

// ValidateDiscovery enforces the expected xAI issuer and trusted OAuth endpoints.
func ValidateDiscovery(doc *DiscoveryDocument) error {
	if doc == nil {
		return fmt.Errorf("xai oauth discovery: empty document")
	}
	if strings.TrimSpace(doc.Issuer) != Issuer {
		return fmt.Errorf("xai oauth discovery: unexpected issuer %q", doc.Issuer)
	}
	if !IsTrustedEndpoint(doc.AuthorizationEndpoint) {
		return fmt.Errorf("xai oauth discovery: untrusted authorization endpoint %q", doc.AuthorizationEndpoint)
	}
	if !IsTrustedEndpoint(doc.TokenEndpoint) {
		return fmt.Errorf("xai oauth discovery: untrusted token endpoint %q", doc.TokenEndpoint)
	}
	if strings.TrimSpace(doc.UserInfoEndpoint) != "" && !IsTrustedEndpoint(doc.UserInfoEndpoint) {
		return fmt.Errorf("xai oauth discovery: untrusted userinfo endpoint %q", doc.UserInfoEndpoint)
	}
	if len(doc.PKCESupportedMethods) > 0 && !containsFold(doc.PKCESupportedMethods, "S256") {
		return fmt.Errorf("xai oauth discovery: S256 PKCE is not supported")
	}
	return nil
}

// IsTrustedEndpoint returns true for HTTPS endpoints on x.ai or an x.ai subdomain.
func IsTrustedEndpoint(endpoint string) bool {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "x.ai" || strings.HasSuffix(host, ".x.ai")
}

// BuildAuthorizeURL constructs the browser authorization URL for xAI OAuth.
func (o *OAuth) BuildAuthorizeURL(doc *DiscoveryDocument, redirectURI, state, nonce string, pkce *PKCECodes) (string, error) {
	if err := ValidateDiscovery(doc); err != nil {
		return "", err
	}
	if pkce == nil || strings.TrimSpace(pkce.CodeChallenge) == "" {
		return "", fmt.Errorf("xai oauth authorize: missing PKCE challenge")
	}
	redirectURI = strings.TrimSpace(redirectURI)
	state = strings.TrimSpace(state)
	nonce = strings.TrimSpace(nonce)
	if redirectURI == "" {
		redirectURI = fmt.Sprintf("http://%s:%d%s", CallbackHost, DefaultCallbackPort, CallbackPath)
	}
	if state == "" {
		return "", fmt.Errorf("xai oauth authorize: missing state")
	}
	if nonce == "" {
		return "", fmt.Errorf("xai oauth authorize: missing nonce")
	}

	params := url.Values{}
	params.Set("client_id", ClientID)
	params.Set("code_challenge", pkce.CodeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("nonce", nonce)
	params.Set("plan", "generic")
	params.Set("redirect_uri", redirectURI)
	params.Set("referrer", Referrer)
	params.Set("response_type", "code")
	params.Set("scope", Scope)
	params.Set("state", state)
	return strings.TrimSpace(doc.AuthorizationEndpoint) + "?" + params.Encode(), nil
}

// ExchangeCodeForTokens exchanges an authorization code for xAI OAuth tokens.
func (o *OAuth) ExchangeCodeForTokens(ctx context.Context, tokenEndpoint, code, redirectURI, verifier string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("client_id", ClientID)
	data.Set("code", strings.TrimSpace(code))
	data.Set("code_verifier", strings.TrimSpace(verifier))
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", strings.TrimSpace(redirectURI))
	return o.postTokenForm(ctx, "token exchange", tokenEndpoint, data)
}

// RefreshTokens refreshes an xAI access token using the cached refresh token.
func (o *OAuth) RefreshTokens(ctx context.Context, tokenEndpoint, refreshToken string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("client_id", ClientID)
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", strings.TrimSpace(refreshToken))
	return o.postTokenForm(ctx, "token refresh", tokenEndpoint, data)
}

func (o *OAuth) postTokenForm(ctx context.Context, operation, tokenEndpoint string, data url.Values) (*TokenResponse, error) {
	if o == nil {
		return nil, fmt.Errorf("xai oauth %s: service is nil", operation)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tokenEndpoint = strings.TrimSpace(tokenEndpoint)
	if tokenEndpoint == "" {
		tokenEndpoint = Issuer + "/oauth2/token"
	}
	if !IsTrustedEndpoint(tokenEndpoint) {
		return nil, fmt.Errorf("xai oauth %s: untrusted token endpoint %q", operation, tokenEndpoint)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("xai oauth %s: create request: %w", operation, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, errDo := o.httpClient.Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("xai oauth %s: execute request: %w", operation, errDo)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Errorf("xai oauth %s: close body error: %v", operation, errClose)
		}
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, errRead := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		if errRead != nil {
			return nil, fmt.Errorf("xai oauth %s: read error body: %w", operation, errRead)
		}
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("xai oauth %s: request failed: status %d: %s", operation, resp.StatusCode, msg)
	}

	var token TokenResponse
	if errDecode := json.NewDecoder(resp.Body).Decode(&token); errDecode != nil {
		return nil, fmt.Errorf("xai oauth %s: decode response: %w", operation, errDecode)
	}
	return &token, nil
}

// IdentityFromTokens decodes identity claims from ID token first, then access token.
func IdentityFromTokens(idToken, accessToken string) UserIdentity {
	if identity, ok := DecodeIdentityFromJWT(idToken); ok {
		return identity
	}
	identity, _ := DecodeIdentityFromJWT(accessToken)
	return identity
}

// DecodeIdentityFromJWT decodes identity-style claims from a JWT without verifying it.
func DecodeIdentityFromJWT(token string) (UserIdentity, bool) {
	token = strings.TrimSpace(token)
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return UserIdentity{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return UserIdentity{}, false
	}
	var claims map[string]any
	if err = json.Unmarshal(payload, &claims); err != nil {
		return UserIdentity{}, false
	}
	identity := UserIdentity{
		Subject:           stringClaim(claims, "sub"),
		Email:             stringClaim(claims, "email"),
		Name:              stringClaim(claims, "name"),
		PreferredUsername: stringClaim(claims, "preferred_username"),
	}
	return identity, identity.Subject != "" || identity.Email != "" || identity.Name != "" || identity.PreferredUsername != ""
}

// CredentialFileName returns the default auth filename for an xAI OAuth account.
func CredentialFileName(identifier string) string {
	identifier = sanitizeFilenamePart(identifier)
	if identifier == "" {
		return Provider + ".json"
	}
	return Provider + "-" + identifier + ".json"
}

// DisplayLabel returns a human-readable label for an xAI OAuth account.
func DisplayLabel(identity UserIdentity) string {
	if strings.TrimSpace(identity.Email) != "" {
		return strings.TrimSpace(identity.Email)
	}
	if strings.TrimSpace(identity.PreferredUsername) != "" {
		return strings.TrimSpace(identity.PreferredUsername)
	}
	if strings.TrimSpace(identity.Name) != "" {
		return strings.TrimSpace(identity.Name)
	}
	if strings.TrimSpace(identity.Subject) != "" {
		return "xAI " + strings.TrimSpace(identity.Subject)
	}
	return "xAI OAuth"
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func stringClaim(claims map[string]any, key string) string {
	if value, ok := claims[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func sanitizeFilenamePart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, ".")
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
	)
	value = replacer.Replace(value)
	value = strings.TrimSpace(value)
	value = strings.Trim(value, ".")
	return value
}

// ExpiryFromTokenResponse returns the absolute expiry time for a token response.
func ExpiryFromTokenResponse(now time.Time, token *TokenResponse) time.Time {
	if token == nil || token.ExpiresIn <= 0 {
		return time.Time{}
	}
	return now.Add(time.Duration(token.ExpiresIn) * time.Second)
}
