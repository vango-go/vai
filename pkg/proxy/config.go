// Package proxy provides the Vango AI HTTP/WebSocket proxy server.
package proxy

import (
	"log/slog"
	"os"
	"time"
)

// Config holds all proxy server configuration.
type Config struct {
	// Server settings
	Host string `json:"host" yaml:"host"`
	Port int    `json:"port" yaml:"port"`

	// TLS settings
	TLSEnabled  bool   `json:"tls_enabled" yaml:"tls_enabled"`
	TLSCertFile string `json:"tls_cert_file" yaml:"tls_cert_file"`
	TLSKeyFile  string `json:"tls_key_file" yaml:"tls_key_file"`

	// Authentication
	AuthMode string         `json:"auth_mode" yaml:"auth_mode"` // api_key, passthrough, none
	APIKeys  []APIKeyConfig `json:"api_keys" yaml:"api_keys"`

	// Provider keys
	ProviderKeys map[string]string `json:"provider_keys" yaml:"provider_keys"`

	// Rate limiting
	RateLimit RateLimitConfig `json:"rate_limit" yaml:"rate_limit"`

	// Observability
	Observability ObservabilityConfig `json:"observability" yaml:"observability"`

	// CORS
	AllowedOrigins []string `json:"allowed_origins" yaml:"allowed_origins"`

	// Request limits
	MaxRequestBodyBytes int64 `json:"max_request_body_bytes" yaml:"max_request_body_bytes"`

	// Timeouts
	ReadTimeout     time.Duration `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout" yaml:"write_timeout"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout" yaml:"shutdown_timeout"`

	// Logger
	Logger *slog.Logger `json:"-" yaml:"-"`
}

// APIKeyConfig defines an API key with associated metadata.
type APIKeyConfig struct {
	Key       string `json:"key" yaml:"key"`
	Name      string `json:"name" yaml:"name"`
	UserID    string `json:"user_id" yaml:"user_id"`
	RateLimit int    `json:"rate_limit" yaml:"rate_limit"` // per-key rate limit (requests/min)
}

// RateLimitConfig configures rate limiting.
type RateLimitConfig struct {
	// Enabled toggles rate limiting on or off.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Global limits
	GlobalRequestsPerMinute int `json:"global_requests_per_minute" yaml:"global_requests_per_minute"`
	GlobalTokensPerMinute   int `json:"global_tokens_per_minute" yaml:"global_tokens_per_minute"`

	// Per-user defaults (can be overridden per API key)
	UserRequestsPerMinute int `json:"user_requests_per_minute" yaml:"user_requests_per_minute"`
	UserTokensPerMinute   int `json:"user_tokens_per_minute" yaml:"user_tokens_per_minute"`

	// Live session limits
	MaxConcurrentSessions int           `json:"max_concurrent_sessions" yaml:"max_concurrent_sessions"`
	SessionIdleTimeout    time.Duration `json:"session_idle_timeout" yaml:"session_idle_timeout"`
}

// ObservabilityConfig configures metrics, logging, and tracing.
type ObservabilityConfig struct {
	// Metrics
	MetricsEnabled bool   `json:"metrics_enabled" yaml:"metrics_enabled"`
	MetricsPath    string `json:"metrics_path" yaml:"metrics_path"`

	// Logging
	LogLevel  string `json:"log_level" yaml:"log_level"`
	LogFormat string `json:"log_format" yaml:"log_format"` // "json" or "text"

	// Tracing
	TracingEnabled  bool   `json:"tracing_enabled" yaml:"tracing_enabled"`
	TracingExporter string `json:"tracing_exporter" yaml:"tracing_exporter"` // "otlp", "jaeger"
	TracingEndpoint string `json:"tracing_endpoint" yaml:"tracing_endpoint"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Host: "0.0.0.0",
		Port: 8080,

		AuthMode: "api_key",

		ProviderKeys: make(map[string]string),

		RateLimit: RateLimitConfig{
			Enabled:                 true,
			GlobalRequestsPerMinute: 1000000,
			GlobalTokensPerMinute:   0,
			UserRequestsPerMinute:   100000,
			UserTokensPerMinute:     0,
			MaxConcurrentSessions:   1000,
			SessionIdleTimeout:      5 * time.Minute,
		},

		Observability: ObservabilityConfig{
			MetricsEnabled: true,
			MetricsPath:    "/metrics",
			LogLevel:       "info",
			LogFormat:      "json",
			TracingEnabled: false,
		},

		ReadTimeout:     60 * time.Second,
		WriteTimeout:    60 * time.Second,
		ShutdownTimeout: 30 * time.Second,

		AllowedOrigins:      []string{"*"},
		MaxRequestBodyBytes: 10 << 20,

		Logger: slog.Default(),
	}
}

// LoadProviderKeysFromEnv loads provider API keys from environment variables.
func (c *Config) LoadProviderKeysFromEnv() {
	providers := []string{"anthropic", "openai", "google", "groq", "mistral", "cartesia", "deepgram", "elevenlabs"}
	for _, provider := range providers {
		envKey := toEnvKey(provider) + "_API_KEY"
		if key := os.Getenv(envKey); key != "" {
			c.ProviderKeys[provider] = key
		}
	}
}

// toEnvKey converts a provider name to environment variable format.
func toEnvKey(provider string) string {
	result := make([]byte, 0, len(provider))
	for i := 0; i < len(provider); i++ {
		c := provider[i]
		if c >= 'a' && c <= 'z' {
			result = append(result, c-32) // to uppercase
		} else {
			result = append(result, c)
		}
	}
	return string(result)
}

// ConfigOption is a functional option for Config.
type ConfigOption func(*Config)

// WithHost sets the server host.
func WithHost(host string) ConfigOption {
	return func(c *Config) {
		c.Host = host
	}
}

// WithPort sets the server port.
func WithPort(port int) ConfigOption {
	return func(c *Config) {
		c.Port = port
	}
}

// WithTLS enables TLS with the given certificate and key files.
func WithTLS(certFile, keyFile string) ConfigOption {
	return func(c *Config) {
		c.TLSEnabled = true
		c.TLSCertFile = certFile
		c.TLSKeyFile = keyFile
	}
}

// WithAPIKey adds an API key.
func WithAPIKey(key, name, userID string, rateLimit int) ConfigOption {
	return func(c *Config) {
		c.APIKeys = append(c.APIKeys, APIKeyConfig{
			Key:       key,
			Name:      name,
			UserID:    userID,
			RateLimit: rateLimit,
		})
	}
}

// WithProviderKey sets a provider API key.
func WithProviderKey(provider, key string) ConfigOption {
	return func(c *Config) {
		c.ProviderKeys[provider] = key
	}
}

// WithLogger sets the logger.
func WithLogger(logger *slog.Logger) ConfigOption {
	return func(c *Config) {
		c.Logger = logger
	}
}

// WithAuthMode sets the authentication mode.
func WithAuthMode(mode string) ConfigOption {
	return func(c *Config) {
		c.AuthMode = mode
	}
}

// WithAPIKeys sets the configured API keys.
func WithAPIKeys(keys []APIKeyConfig) ConfigOption {
	return func(c *Config) {
		c.APIKeys = keys
	}
}

// WithProviderKeys sets provider API keys.
func WithProviderKeys(keys map[string]string) ConfigOption {
	return func(c *Config) {
		c.ProviderKeys = keys
	}
}

// WithRateLimitConfig sets rate limiting configuration.
func WithRateLimitConfig(cfg RateLimitConfig) ConfigOption {
	return func(c *Config) {
		c.RateLimit = cfg
	}
}

// WithObservability sets observability configuration.
func WithObservability(cfg ObservabilityConfig) ConfigOption {
	return func(c *Config) {
		c.Observability = cfg
	}
}

// WithAllowedOrigins sets allowed CORS origins.
func WithAllowedOrigins(origins []string) ConfigOption {
	return func(c *Config) {
		c.AllowedOrigins = origins
	}
}

// WithRequestBodyLimit sets max request body size in bytes.
func WithRequestBodyLimit(limit int64) ConfigOption {
	return func(c *Config) {
		c.MaxRequestBodyBytes = limit
	}
}

// WithTimeouts sets server timeouts.
func WithTimeouts(read, write, shutdown time.Duration) ConfigOption {
	return func(c *Config) {
		if read > 0 {
			c.ReadTimeout = read
		}
		if write > 0 {
			c.WriteTimeout = write
		}
		if shutdown > 0 {
			c.ShutdownTimeout = shutdown
		}
	}
}

// WithRateLimit configures rate limiting.
func WithRateLimit(globalRPM, userRPM int) ConfigOption {
	return func(c *Config) {
		c.RateLimit.GlobalRequestsPerMinute = globalRPM
		c.RateLimit.UserRequestsPerMinute = userRPM
	}
}

// WithMetrics enables or disables metrics.
func WithMetrics(enabled bool) ConfigOption {
	return func(c *Config) {
		c.Observability.MetricsEnabled = enabled
	}
}
