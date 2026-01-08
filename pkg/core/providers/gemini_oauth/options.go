package gemini_oauth

import "net/http"

// Option configures a Provider.
type Option func(*Provider)

// WithProjectID sets the Google Cloud project ID.
func WithProjectID(projectID string) Option {
	return func(p *Provider) {
		p.projectID = projectID
	}
}

// WithCredentialsPath sets the path to the credentials file.
func WithCredentialsPath(path string) Option {
	return func(p *Provider) {
		p.credsPath = path
	}
}

// WithCredentials provides credentials directly.
func WithCredentials(creds *Credentials) Option {
	return func(p *Provider) {
		p.creds = creds
	}
}

// WithTokenSource provides a custom token source.
func WithTokenSource(ts TokenSource) Option {
	return func(p *Provider) {
		p.tokenSource = ts
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Provider) {
		p.httpClient = client
	}
}
