package groq

import "net/http"

// Option configures the Groq provider.
type Option func(*Provider)

// WithBaseURL sets a custom base URL (for testing or proxying).
func WithBaseURL(url string) Option {
	return func(p *Provider) {
		p.baseURL = url
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Provider) {
		p.httpClient = client
	}
}
