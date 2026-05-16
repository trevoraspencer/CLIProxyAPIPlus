package xai

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestDiscoverParsesAndValidatesTrustedEndpoints(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"issuer":"https://auth.x.ai",
			"authorization_endpoint":"https://auth.x.ai/oauth2/authorize",
			"token_endpoint":"https://auth.x.ai/oauth2/token",
			"userinfo_endpoint":"https://auth.x.ai/oauth2/userinfo",
			"code_challenge_methods_supported":["S256"],
			"grant_types_supported":["authorization_code","refresh_token"]
		}`))
	}))
	defer server.Close()

	svc := NewOAuthWithDiscoveryURL(nil, server.Client(), server.URL)
	doc, err := svc.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if doc.Issuer != Issuer {
		t.Fatalf("issuer = %q, want %q", doc.Issuer, Issuer)
	}
	if doc.TokenEndpoint != "https://auth.x.ai/oauth2/token" {
		t.Fatalf("token endpoint = %q", doc.TokenEndpoint)
	}
}

func TestValidateDiscoveryRejectsAccountsAndUntrustedEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		doc  *DiscoveryDocument
	}{
		{
			name: "accounts issuer",
			doc: &DiscoveryDocument{
				Issuer:                "https://accounts.x.ai",
				AuthorizationEndpoint: "https://auth.x.ai/oauth2/authorize",
				TokenEndpoint:         "https://auth.x.ai/oauth2/token",
				PKCESupportedMethods:  []string{"S256"},
			},
		},
		{
			name: "untrusted token",
			doc: &DiscoveryDocument{
				Issuer:                Issuer,
				AuthorizationEndpoint: "https://auth.x.ai/oauth2/authorize",
				TokenEndpoint:         "https://evil.example/token",
				PKCESupportedMethods:  []string{"S256"},
			},
		},
		{
			name: "plain http",
			doc: &DiscoveryDocument{
				Issuer:                Issuer,
				AuthorizationEndpoint: "http://auth.x.ai/oauth2/authorize",
				TokenEndpoint:         "https://auth.x.ai/oauth2/token",
				PKCESupportedMethods:  []string{"S256"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateDiscovery(tt.doc); err == nil {
				t.Fatal("ValidateDiscovery succeeded, want error")
			}
		})
	}
}

func TestBuildAuthorizeURLUsesExpectedOAuthParameters(t *testing.T) {
	t.Parallel()

	svc := NewOAuth(nil, nil)
	authURL, err := svc.BuildAuthorizeURL(validDiscoveryDocument(), "http://127.0.0.1:56121/callback", "state-1", "nonce-1", &PKCECodes{CodeChallenge: "challenge-1"})
	if err != nil {
		t.Fatalf("BuildAuthorizeURL error: %v", err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse authorize URL: %v", err)
	}
	if got := parsed.Scheme + "://" + parsed.Host + parsed.Path; got != "https://auth.x.ai/oauth2/authorize" {
		t.Fatalf("authorize endpoint = %q", got)
	}
	query := parsed.Query()
	assertQueryValue(t, query, "client_id", ClientID)
	assertQueryValue(t, query, "scope", Scope)
	assertQueryValue(t, query, "redirect_uri", "http://127.0.0.1:56121/callback")
	assertQueryValue(t, query, "response_type", "code")
	assertQueryValue(t, query, "code_challenge", "challenge-1")
	assertQueryValue(t, query, "code_challenge_method", "S256")
	assertQueryValue(t, query, "state", "state-1")
	assertQueryValue(t, query, "nonce", "nonce-1")
	assertQueryValue(t, query, "plan", "generic")
	assertQueryValue(t, query, "referrer", Referrer)
}

func TestGeneratePKCECodesUsesS256Challenge(t *testing.T) {
	t.Parallel()

	codes, err := GeneratePKCECodes()
	if err != nil {
		t.Fatalf("GeneratePKCECodes error: %v", err)
	}
	if len(codes.CodeVerifier) < 43 {
		t.Fatalf("code verifier too short: %d", len(codes.CodeVerifier))
	}
	hash := sha256.Sum256([]byte(codes.CodeVerifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(hash[:])
	if codes.CodeChallenge != wantChallenge {
		t.Fatalf("code challenge mismatch")
	}
}

func TestRefreshTokensPostsPublicClientRefreshGrant(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", req.Method)
		}
		if req.URL.String() != "https://auth.x.ai/oauth2/token" {
			t.Fatalf("URL = %s", req.URL.String())
		}
		if got := req.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Fatalf("Content-Type = %q", got)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse form: %v", err)
		}
		assertQueryValue(t, form, "grant_type", "refresh_token")
		assertQueryValue(t, form, "client_id", ClientID)
		assertQueryValue(t, form, "refresh_token", "refresh-1")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"access_token":"access-2","refresh_token":"refresh-2","expires_in":3600,"token_type":"Bearer"}`)),
		}, nil
	})}

	svc := NewOAuthWithDiscoveryURL(nil, client, DiscoveryURL)
	token, err := svc.RefreshTokens(context.Background(), "https://auth.x.ai/oauth2/token", "refresh-1")
	if err != nil {
		t.Fatalf("RefreshTokens error: %v", err)
	}
	if token.AccessToken != "access-2" || token.RefreshToken != "refresh-2" || token.ExpiresIn != 3600 {
		t.Fatalf("unexpected token response: %+v", token)
	}
}

func TestIdentityFromTokensDecodesIDTokenClaims(t *testing.T) {
	t.Parallel()

	idToken := unsignedJWT(`{"sub":"sub-1","email":"user@example.com","name":"User Name","preferred_username":"user"}`)
	identity := IdentityFromTokens(idToken, "")
	if identity.Subject != "sub-1" || identity.Email != "user@example.com" || identity.Name != "User Name" || identity.PreferredUsername != "user" {
		t.Fatalf("identity mismatch: %+v", identity)
	}
	if got := CredentialFileName(identity.Email); got != "xai-oauth-user@example.com.json" {
		t.Fatalf("CredentialFileName = %q", got)
	}
}

func validDiscoveryDocument() *DiscoveryDocument {
	return &DiscoveryDocument{
		Issuer:                Issuer,
		AuthorizationEndpoint: "https://auth.x.ai/oauth2/authorize",
		TokenEndpoint:         "https://auth.x.ai/oauth2/token",
		UserInfoEndpoint:      "https://auth.x.ai/oauth2/userinfo",
		PKCESupportedMethods:  []string{"S256"},
	}
}

func assertQueryValue(t *testing.T, values url.Values, key, want string) {
	t.Helper()
	if got := values.Get(key); got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}

func unsignedJWT(payload string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	body := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return header + "." + body + "."
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
