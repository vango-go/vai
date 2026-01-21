package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// LoginResult contains the outcome of the login flow.
type LoginResult struct {
	Credentials *Credentials
	Error       error
}

// StartLogin initiates the OAuth flow and returns a channel for the result.
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
		return nil, "", fmt.Errorf("failed to start callback server: %w", err)
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

		creds, err := ExchangeCode(code, verifier)
		if err != nil {
			http.Error(w, "Token exchange failed", http.StatusInternalServerError)
			resultChan <- LoginResult{Error: err}
			return
		}

		creds.ProjectID = projectID

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Gemini Proxy - Authenticated</title></head>
<body style="font-family: sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #1a1a2e; color: #eee;">
<div style="text-align: center; padding: 2rem; background: #16213e; border-radius: 12px;">
<h1>✓ Authentication Complete</h1>
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
