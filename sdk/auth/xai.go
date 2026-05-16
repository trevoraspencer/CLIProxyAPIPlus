package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/auth/xai"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/browser"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/misc"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

// XAIOAuthAuthenticator implements browser OAuth login for xAI Grok CLI accounts.
type XAIOAuthAuthenticator struct{}

// NewXAIOAuthAuthenticator constructs a new xAI OAuth authenticator.
func NewXAIOAuthAuthenticator() Authenticator { return &XAIOAuthAuthenticator{} }

// Provider returns the provider key for xAI OAuth.
func (XAIOAuthAuthenticator) Provider() string { return xai.Provider }

// RefreshLead instructs the manager to refresh five minutes before expiry.
func (XAIOAuthAuthenticator) RefreshLead() *time.Duration {
	return new(5 * time.Minute)
}

// Login launches a local OAuth code+PKCE flow to obtain xAI tokens and persists them.
func (XAIOAuthAuthenticator) Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cliproxy auth: configuration is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if opts == nil {
		opts = &LoginOptions{}
	}

	callbackPort := xai.DefaultCallbackPort
	if opts.CallbackPort > 0 {
		callbackPort = opts.CallbackPort
	}

	authSvc := xai.NewOAuth(cfg, nil)
	discovery, err := authSvc.Discover(ctx)
	if err != nil {
		return nil, err
	}

	pkceCodes, err := xai.GeneratePKCECodes()
	if err != nil {
		return nil, fmt.Errorf("xai oauth: PKCE generation failed: %w", err)
	}
	state, err := misc.GenerateRandomState()
	if err != nil {
		return nil, fmt.Errorf("xai oauth: state generation failed: %w", err)
	}
	nonce, err := misc.GenerateRandomState()
	if err != nil {
		return nil, fmt.Errorf("xai oauth: nonce generation failed: %w", err)
	}

	srv, port, cbChan, errServer := startXAICallbackServer(callbackPort)
	if errServer != nil {
		return nil, fmt.Errorf("xai oauth: failed to start callback server: %w", errServer)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if errShutdown := srv.Shutdown(shutdownCtx); errShutdown != nil {
			log.Warnf("xai oauth callback server shutdown error: %v", errShutdown)
		}
	}()

	redirectURI := fmt.Sprintf("http://%s:%d%s", xai.CallbackHost, port, xai.CallbackPath)
	authURL, err := authSvc.BuildAuthorizeURL(discovery, redirectURI, state, nonce, pkceCodes)
	if err != nil {
		return nil, err
	}

	if !opts.NoBrowser {
		fmt.Println("Opening browser for xAI authentication")
		if !browser.IsAvailable() {
			log.Warn("No browser available; please open the URL manually")
			util.PrintSSHTunnelInstructions(port)
			fmt.Printf("Visit the following URL to continue authentication:\n%s\n", authURL)
		} else if errOpen := browser.OpenURL(authURL); errOpen != nil {
			log.Warnf("Failed to open browser automatically: %v", errOpen)
			util.PrintSSHTunnelInstructions(port)
			fmt.Printf("Visit the following URL to continue authentication:\n%s\n", authURL)
		}
	} else {
		util.PrintSSHTunnelInstructions(port)
		fmt.Printf("Visit the following URL to continue authentication:\n%s\n", authURL)
	}

	fmt.Println("Waiting for xAI authentication callback...")

	cbRes, err := waitForXAICallback(cbChan, opts)
	if err != nil {
		return nil, err
	}
	if cbRes.Error != "" {
		return nil, fmt.Errorf("xai oauth: authentication failed: %s", cbRes.Error)
	}
	if cbRes.State != state {
		return nil, fmt.Errorf("xai oauth: invalid state")
	}
	if cbRes.Code == "" {
		return nil, fmt.Errorf("xai oauth: missing authorization code")
	}

	tokenResp, errToken := authSvc.ExchangeCodeForTokens(ctx, discovery.TokenEndpoint, cbRes.Code, redirectURI, pkceCodes.CodeVerifier)
	if errToken != nil {
		return nil, fmt.Errorf("xai oauth: token exchange failed: %w", errToken)
	}
	accessToken := strings.TrimSpace(tokenResp.AccessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("xai oauth: token exchange returned empty access token")
	}

	identity := xai.IdentityFromTokens(tokenResp.IDToken, tokenResp.AccessToken)
	metadata := xaiAuthMetadata(tokenResp, discovery, identity)
	fileName := xai.CredentialFileName(xaiIdentityFileID(identity))
	label := xai.DisplayLabel(identity)

	fmt.Println("xAI authentication successful")
	return &coreauth.Auth{
		ID:       fileName,
		Provider: xai.Provider,
		FileName: fileName,
		Label:    label,
		Attributes: map[string]string{
			"auth_kind": "oauth",
			"base_url":  xai.DefaultBaseURL,
		},
		Metadata: metadata,
	}, nil
}

type xaiCallbackResult struct {
	Code  string
	Error string
	State string
}

func startXAICallbackServer(port int) (*http.Server, int, <-chan xaiCallbackResult, error) {
	if port <= 0 {
		port = xai.DefaultCallbackPort
	}
	addr := fmt.Sprintf("%s:%d", xai.CallbackHost, port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, 0, nil, err
	}
	port = listener.Addr().(*net.TCPAddr).Port
	resultCh := make(chan xaiCallbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(xai.CallbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		res := xaiCallbackResult{
			Code:  strings.TrimSpace(q.Get("code")),
			Error: strings.TrimSpace(q.Get("error")),
			State: strings.TrimSpace(q.Get("state")),
		}
		resultCh <- res
		if res.Code != "" && res.Error == "" {
			_, _ = w.Write([]byte("<h1>Login successful</h1><p>You can close this window.</p>"))
		} else {
			_, _ = w.Write([]byte("<h1>Login failed</h1><p>Please check the CLI output.</p>"))
		}
	})

	srv := &http.Server{Handler: mux}
	go func() {
		if errServe := srv.Serve(listener); errServe != nil && !strings.Contains(errServe.Error(), "Server closed") {
			log.Warnf("xai oauth callback server error: %v", errServe)
		}
	}()
	return srv, port, resultCh, nil
}

func waitForXAICallback(cbChan <-chan xaiCallbackResult, opts *LoginOptions) (xaiCallbackResult, error) {
	timeoutTimer := time.NewTimer(5 * time.Minute)
	defer timeoutTimer.Stop()

	var manualPromptTimer *time.Timer
	var manualPromptC <-chan time.Time
	if opts != nil && opts.Prompt != nil {
		manualPromptTimer = time.NewTimer(15 * time.Second)
		manualPromptC = manualPromptTimer.C
		defer manualPromptTimer.Stop()
	}

	var manualInputCh <-chan string
	var manualInputErrCh <-chan error

	for {
		select {
		case res := <-cbChan:
			return res, nil
		case <-manualPromptC:
			manualPromptC = nil
			if manualPromptTimer != nil {
				manualPromptTimer.Stop()
			}
			select {
			case res := <-cbChan:
				return res, nil
			default:
			}
			manualInputCh, manualInputErrCh = misc.AsyncPrompt(opts.Prompt, "Paste the xAI callback URL (or press Enter to keep waiting): ")
			continue
		case input := <-manualInputCh:
			manualInputCh = nil
			manualInputErrCh = nil
			parsed, errParse := misc.ParseOAuthCallback(input)
			if errParse != nil {
				return xaiCallbackResult{}, errParse
			}
			if parsed == nil {
				continue
			}
			return xaiCallbackResult{
				Code:  parsed.Code,
				State: parsed.State,
				Error: parsed.Error,
			}, nil
		case errManual := <-manualInputErrCh:
			return xaiCallbackResult{}, errManual
		case <-timeoutTimer.C:
			return xaiCallbackResult{}, fmt.Errorf("xai oauth: authentication timed out")
		}
	}
}

func xaiAuthMetadata(token *xai.TokenResponse, discovery *xai.DiscoveryDocument, identity xai.UserIdentity) map[string]any {
	now := time.Now()
	metadata := map[string]any{
		"type":                   xai.Provider,
		"auth_method":            "oauth",
		"access_token":           token.AccessToken,
		"refresh_token":          token.RefreshToken,
		"id_token":               token.IDToken,
		"token_type":             token.TokenType,
		"expires_in":             token.ExpiresIn,
		"timestamp":              now.UnixMilli(),
		"issuer":                 xai.Issuer,
		"authorization_endpoint": discovery.AuthorizationEndpoint,
		"token_endpoint":         discovery.TokenEndpoint,
		"base_url":               xai.DefaultBaseURL,
		"client_id":              xai.ClientID,
		"scope":                  xai.Scope,
	}
	if scope := strings.TrimSpace(token.Scope); scope != "" {
		metadata["granted_scope"] = scope
	}
	if expiry := xai.ExpiryFromTokenResponse(now, token); !expiry.IsZero() {
		metadata["expired"] = expiry.Format(time.RFC3339)
		metadata["expires_at"] = expiry.Format(time.RFC3339)
	}
	if identity.Subject != "" {
		metadata["sub"] = identity.Subject
		metadata["account_id"] = identity.Subject
	}
	if identity.Email != "" {
		metadata["email"] = identity.Email
	}
	if identity.Name != "" {
		metadata["name"] = identity.Name
	}
	if identity.PreferredUsername != "" {
		metadata["preferred_username"] = identity.PreferredUsername
	}
	return metadata
}

func xaiIdentityFileID(identity xai.UserIdentity) string {
	switch {
	case strings.TrimSpace(identity.Email) != "":
		return identity.Email
	case strings.TrimSpace(identity.PreferredUsername) != "":
		return identity.PreferredUsername
	case strings.TrimSpace(identity.Subject) != "":
		return identity.Subject
	default:
		return ""
	}
}
