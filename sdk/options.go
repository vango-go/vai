package vai

import (
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// ClientOption is a function that configures a Client.
type ClientOption func(*Client)

// WithBaseURL sets the base URL for Proxy Mode.
// If provided, the client operates in Proxy Mode.
func WithBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithAPIKey sets the Vango AI API key.
// Used for authentication in Proxy Mode.
func WithAPIKey(key string) ClientOption {
	return func(c *Client) {
		c.apiKey = key
	}
}

// WithProviderKey sets an API key for a specific provider.
// Used in Direct Mode to configure provider credentials.
func WithProviderKey(provider, key string) ClientOption {
	return func(c *Client) {
		c.providerKeys[provider] = key
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = client
	}
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// WithLogger sets the logger for the client.
func WithLogger(l *slog.Logger) ClientOption {
	return func(c *Client) {
		c.logger = l
	}
}

// WithTracer sets the OpenTelemetry tracer for the client.
func WithTracer(t trace.Tracer) ClientOption {
	return func(c *Client) {
		c.tracer = t
	}
}

// WithRetries sets the maximum number of retries for failed requests.
func WithRetries(n int) ClientOption {
	return func(c *Client) {
		c.maxRetries = n
	}
}

// WithRetryBackoff sets the initial backoff duration between retries.
func WithRetryBackoff(d time.Duration) ClientOption {
	return func(c *Client) {
		c.retryBackoff = d
	}
}
