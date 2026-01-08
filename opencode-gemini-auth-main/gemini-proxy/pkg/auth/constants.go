package auth

const (
	ClientID     = "" // Load from GEMINI_OAUTH_CLIENT_ID environment variable
	ClientSecret = "" // Load from GEMINI_OAUTH_CLIENT_SECRET environment variable
	RedirectURI  = "http://localhost:8085/oauth2callback"

	AuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	TokenURL = "https://oauth2.googleapis.com/token"
)

var Scopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
}
