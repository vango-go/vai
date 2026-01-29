package gemini_oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// TokenSource provides valid access tokens with automatic refresh.
type TokenSource interface {
	// Token returns a valid access token, refreshing if needed.
	Token() (string, error)
}

// OAuthTokenSource implements TokenSource with auto-refresh capability.
type OAuthTokenSource struct {
	creds *Credentials
	store CredentialsStore
	mu    sync.Mutex
}

// NewOAuthTokenSource creates a token source from existing credentials.
func NewOAuthTokenSource(creds *Credentials, store CredentialsStore) *OAuthTokenSource {
	return &OAuthTokenSource{
		creds: creds,
		store: store,
	}
}

// Token returns a valid access token, refreshing if needed.
func (s *OAuthTokenSource) Token() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.creds.NeedsRefresh() {
		if err := Refresh(s.creds); err != nil {
			return "", err
		}
		// Save refreshed credentials if we have a store
		if s.store != nil {
			if err := s.store.Save(s.creds); err != nil {
				// Log but don't fail - token is still valid
				// TODO: Add logging
			}
		}
	}

	return s.creds.AccessToken, nil
}

// Credentials returns the current credentials.
func (s *OAuthTokenSource) Credentials() *Credentials {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creds
}

// generatePKCE creates a code verifier and challenge for OAuth PKCE.
func generatePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

// AuthorizeURL builds the Google OAuth authorization URL.
func AuthorizeURL(state, challenge string) string {
	u, _ := url.Parse(AuthURL)
	q := u.Query()
	q.Set("client_id", GetClientID())
	q.Set("response_type", "code")
	q.Set("redirect_uri", RedirectURI)
	q.Set("scope", strings.Join(Scopes, " "))
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("access_type", "offline")
	q.Set("prompt", "consent")
	u.RawQuery = q.Encode()
	return u.String()
}

// ExchangeCode exchanges an authorization code for tokens.
// If projectID is empty, it will be auto-discovered using the Cloud Resource Manager API.
func ExchangeCode(code, verifier, projectID string) (*Credentials, error) {
	data := url.Values{}
	data.Set("client_id", GetClientID())
	data.Set("client_secret", GetClientSecret())
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", RedirectURI)
	data.Set("code_verifier", verifier)

	resp, err := http.PostForm(TokenURL, data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed: %s", resp.Status)
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token received - ensure 'offline' access was requested")
	}

	// Auto-discover project ID if not provided
	if projectID == "" {
		projectID, err = discoverProjectID(result.AccessToken)
		if err != nil {
			return nil, fmt.Errorf("failed to discover project ID: %w", err)
		}
	}

	return &Credentials{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
		ProjectID:    projectID,
	}, nil
}

// discoverProjectID fetches the user's first available GCP project.
func discoverProjectID(accessToken string) (string, error) {
	req, err := http.NewRequest("GET", "https://cloudresourcemanager.googleapis.com/v1/projects?filter=lifecycleState:ACTIVE", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to list projects: %s", resp.Status)
	}

	var result struct {
		Projects []struct {
			ProjectID string `json:"projectId"`
			Name      string `json:"name"`
		} `json:"projects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Projects) == 0 {
		return "", fmt.Errorf("no GCP projects found for this account")
	}

	// Return the first project
	return result.Projects[0].ProjectID, nil
}

// Refresh uses the refresh token to get a new access token.
func Refresh(creds *Credentials) error {
	if creds.RefreshToken == "" {
		return ErrCredentialsExpired
	}

	data := url.Values{}
	data.Set("client_id", GetClientID())
	data.Set("client_secret", GetClientSecret())
	data.Set("refresh_token", creds.RefreshToken)
	data.Set("grant_type", "refresh_token")

	resp, err := http.PostForm(TokenURL, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Check for invalid_grant error (refresh token revoked)
		var errResp struct {
			Error string `json:"error"`
		}
		if json.NewDecoder(resp.Body).Decode(&errResp) == nil {
			if errResp.Error == "invalid_grant" {
				return ErrCredentialsExpired
			}
		}
		return fmt.Errorf("token refresh failed: %s", resp.Status)
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	creds.AccessToken = result.AccessToken
	creds.Expiry = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	return nil
}

// LoginResult contains the outcome of the login flow.
type LoginResult struct {
	Credentials *Credentials
	Error       error
}

// StartLogin initiates the OAuth flow and returns a channel for the result.
// It starts a local HTTP server to receive the OAuth callback.
func StartLogin(projectID string) (<-chan LoginResult, string, error) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate PKCE: %w", err)
	}

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, "", fmt.Errorf("failed to generate state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	authURL := AuthorizeURL(state, challenge)

	resultChan := make(chan LoginResult, 1)

	// Parse redirect URI to get port
	redirectURL, _ := url.Parse(RedirectURI)
	port := redirectURL.Port()
	if port == "" {
		port = "80"
	}

	listener, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		return nil, "", fmt.Errorf("failed to start callback server on port %s: %w", port, err)
	}

	server := &http.Server{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != redirectURL.Path {
			http.NotFound(w, r)
			return
		}

		receivedState := r.URL.Query().Get("state")
		if receivedState != state {
			http.Error(w, "Invalid state", http.StatusBadRequest)
			resultChan <- LoginResult{Error: fmt.Errorf("state mismatch")}
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "No code received", http.StatusBadRequest)
			resultChan <- LoginResult{Error: fmt.Errorf("no code received")}
			return
		}

		creds, err := ExchangeCode(code, verifier, projectID)
		if err != nil {
			http.Error(w, "Token exchange failed", http.StatusInternalServerError)
			resultChan <- LoginResult{Error: err}
			return
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Vango AI - Gemini OAuth</title></head>
<body style="font-family: sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #1a1a2e; color: #eee;">
<div style="text-align: center; padding: 2rem; background: #16213e; border-radius: 12px;">
<h1>Authentication Complete</h1>
<p>You can close this window and return to the terminal.</p>
</div>
</body>
</html>`)

		resultChan <- LoginResult{Credentials: creds}

		go func() {
			time.Sleep(500 * time.Millisecond)
			server.Shutdown(context.Background())
		}()
	})

	go func() {
		if err := server.Serve(listener); err != http.ErrServerClosed {
			resultChan <- LoginResult{Error: err}
		}
	}()

	return resultChan, authURL, nil
}
