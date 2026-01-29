// Package gemini_oauth provides a Gemini provider that uses Google OAuth authentication
// via the Cloud Code Assist endpoint, allowing users to use their existing Gemini plan
// quota instead of API billing.
package gemini_oauth

import "os"

// GetClientID returns the OAuth client ID from GEMINI_OAUTH_CLIENT_ID env var.
func GetClientID() string {
	return os.Getenv("GEMINI_OAUTH_CLIENT_ID")
}

// GetClientSecret returns the OAuth client secret from GEMINI_OAUTH_CLIENT_SECRET env var.
func GetClientSecret() string {
	return os.Getenv("GEMINI_OAUTH_CLIENT_SECRET")
}

// OAuth constants for Google Gemini CLI authentication.
const (
	// ClientID is deprecated, use GetClientID() instead.
	ClientID = ""

	// ClientSecret is deprecated, use GetClientSecret() instead.
	ClientSecret = ""

	// RedirectURI is the OAuth callback URI.
	RedirectURI = "http://localhost:8085/oauth2callback"

	// AuthURL is the Google OAuth authorization endpoint.
	AuthURL = "https://accounts.google.com/o/oauth2/v2/auth"

	// TokenURL is the Google OAuth token endpoint.
	TokenURL = "https://oauth2.googleapis.com/token"

	// CloudCodeEndpoint is the Cloud Code Assist API endpoint.
	CloudCodeEndpoint = "https://cloudcode-pa.googleapis.com"

	// DefaultCredentialsPath is the default path for storing credentials.
	DefaultCredentialsPath = ".config/vango/gemini-oauth-credentials.json"

	// DefaultMaxTokens is the default max output tokens if not specified.
	DefaultMaxTokens = 4096
)

// Scopes are the OAuth scopes required for Gemini access.
var Scopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
}

// SpoofHeaders are headers that mimic the Google API client for compatibility.
var SpoofHeaders = map[string]string{
	"User-Agent":        "google-api-nodejs-client/9.15.1",
	"X-Goog-Api-Client": "gl-node/22.17.0",
	"Client-Metadata":   "ideType=IDE_UNSPECIFIED,platform=PLATFORM_UNSPECIFIED,pluginType=GEMINI",
}

// ModelFallbacks maps unsupported model names to supported alternatives.
var ModelFallbacks = map[string]string{
	"gemini-2.5-flash-image": "gemini-2.5-flash",
}
