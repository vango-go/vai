package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Credentials holds the OAuth tokens and project info.
type Credentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
	ProjectID    string    `json:"project_id"`
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
	q.Set("client_id", ClientID)
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
func ExchangeCode(code, verifier string) (*Credentials, error) {
	data := url.Values{}
	data.Set("client_id", ClientID)
	data.Set("client_secret", ClientSecret)
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

	return &Credentials{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
	}, nil
}

// Refresh uses the refresh token to get a new access token.
func Refresh(creds *Credentials) error {
	data := url.Values{}
	data.Set("client_id", ClientID)
	data.Set("client_secret", ClientSecret)
	data.Set("refresh_token", creds.RefreshToken)
	data.Set("grant_type", "refresh_token")

	resp, err := http.PostForm(TokenURL, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
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

// GetValidToken returns a valid access token, refreshing if needed.
func GetValidToken(creds *Credentials) (string, error) {
	if time.Now().After(creds.Expiry.Add(-time.Minute)) {
		if err := Refresh(creds); err != nil {
			return "", err
		}
	}
	return creds.AccessToken, nil
}

// credentialsPath returns the path to the credentials file.
func credentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "gemini-proxy")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

// SaveCredentials saves credentials to disk.
func SaveCredentials(creds *Credentials) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// LoadCredentials loads credentials from disk.
func LoadCredentials() (*Credentials, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}
	return &creds, nil
}
