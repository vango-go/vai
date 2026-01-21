// Package gemini implements the Google Gemini API provider.
// It translates between the Vango API format (Anthropic-style) and Gemini's format.
package gemini

import "net/http"

// Option configures the Provider.
type Option func(*Provider)

// WithBaseURL sets the base URL for API requests.
// Default: https://generativelanguage.googleapis.com/v1beta
func WithBaseURL(url string) Option {
	return func(p *Provider) {
		p.baseURL = url
	}
}

// WithHTTPClient sets the HTTP client for API requests.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Provider) {
		p.httpClient = client
	}
}
