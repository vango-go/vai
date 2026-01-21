# Phase 8: Proxy Server

**Status:** Not Started
**Priority:** Medium
**Dependencies:** Phase 7 (Live Sessions)

---

## Overview

Phase 8 implements the Vango Proxy server (`cmd/vango-proxy`), which wraps `pkg/core` in an HTTP server. This enables:

1. **Non-Go clients**: Python, Node, Rust, curl can use Vango via HTTP
2. **Centralized governance**: API keys, rate limiting, usage tracking
3. **Observability**: Metrics, logging, tracing
4. **Secret management**: Provider keys stay on server

The Proxy exposes the same API as defined in `API_SPEC.md`, using `pkg/core` for all the heavy lifting.

---

## Goals

1. Implement HTTP server with all API endpoints
2. Add authentication (API keys)
3. Add rate limiting
4. Add observability (Prometheus metrics, structured logging, OpenTelemetry)
5. Add configuration file support
6. Create Docker image
7. Update SDK to support Proxy Mode

---

## Deliverables

### 8.1 Package Structure

```
cmd/
└── vango-proxy/
    └── main.go           # Entry point

pkg/
└── proxy/
    ├── server.go         # HTTP server
    ├── handlers/
    │   ├── messages.go   # POST /v1/messages
    │   ├── audio.go      # POST /v1/audio
    │   ├── models.go     # GET /v1/models
    │   ├── live.go       # WebSocket /v1/messages/live
    │   └── health.go     # GET /health
    ├── middleware/
    │   ├── auth.go       # API key authentication
    │   ├── ratelimit.go  # Rate limiting
    │   ├── logging.go    # Request logging
    │   ├── metrics.go    # Prometheus metrics
    │   └── tracing.go    # OpenTelemetry tracing
    ├── config/
    │   ├── config.go     # Configuration types
    │   └── loader.go     # Load from file/env
    └── errors.go         # Error response helpers
```

### 8.2 Main Entry Point (cmd/vango-proxy/main.go)

```go
package main

import (
    "context"
    "flag"
    "log/slog"
    "os"
    "os/signal"
    "syscall"

    "github.com/vango-go/vai/pkg/proxy"
    "github.com/vango-go/vai/pkg/proxy/config"
)

func main() {
    configPath := flag.String("config", "", "Path to config file")
    flag.Parse()

    // Load configuration
    cfg, err := config.Load(*configPath)
    if err != nil {
        slog.Error("failed to load config", "error", err)
        os.Exit(1)
    }

    // Setup logging
    logger := setupLogger(cfg.Observability.Logging)
    slog.SetDefault(logger)

    // Create server
    server, err := proxy.NewServer(cfg)
    if err != nil {
        slog.Error("failed to create server", "error", err)
        os.Exit(1)
    }

    // Start server
    go func() {
        slog.Info("starting vango proxy",
            "host", cfg.Server.Host,
            "port", cfg.Server.Port,
        )
        if err := server.Start(); err != nil {
            slog.Error("server error", "error", err)
            os.Exit(1)
        }
    }()

    // Wait for shutdown signal
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    slog.Info("shutting down server")
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := server.Shutdown(ctx); err != nil {
        slog.Error("shutdown error", "error", err)
    }
}

func setupLogger(cfg config.LoggingConfig) *slog.Logger {
    var level slog.Level
    switch cfg.Level {
    case "debug":
        level = slog.LevelDebug
    case "info":
        level = slog.LevelInfo
    case "warn":
        level = slog.LevelWarn
    case "error":
        level = slog.LevelError
    default:
        level = slog.LevelInfo
    }

    opts := &slog.HandlerOptions{Level: level}

    var handler slog.Handler
    if cfg.Format == "json" {
        handler = slog.NewJSONHandler(os.Stdout, opts)
    } else {
        handler = slog.NewTextHandler(os.Stdout, opts)
    }

    return slog.New(handler)
}
```

### 8.3 Configuration (pkg/proxy/config/config.go)

```go
package config

import (
    "os"
    "time"

    "gopkg.in/yaml.v3"
)

// Config is the proxy configuration.
type Config struct {
    Server        ServerConfig        `yaml:"server"`
    Auth          AuthConfig          `yaml:"auth"`
    Providers     ProvidersConfig     `yaml:"providers"`
    Voice         VoiceConfig         `yaml:"voice"`
    Aliases       map[string]string   `yaml:"aliases"`
    Defaults      DefaultsConfig      `yaml:"defaults"`
    RateLimits    RateLimitsConfig    `yaml:"rate_limits"`
    Observability ObservabilityConfig `yaml:"observability"`
}

type ServerConfig struct {
    Host string    `yaml:"host"`
    Port int       `yaml:"port"`
    TLS  TLSConfig `yaml:"tls"`
}

type TLSConfig struct {
    Enabled  bool   `yaml:"enabled"`
    CertFile string `yaml:"cert_file"`
    KeyFile  string `yaml:"key_file"`
}

type AuthConfig struct {
    Mode    string       `yaml:"mode"` // "api_key", "passthrough", "none"
    APIKeys []APIKeyConfig `yaml:"api_keys"`
}

type APIKeyConfig struct {
    Key       string `yaml:"key"`
    Name      string `yaml:"name"`
    UserID    string `yaml:"user_id"`
    RateLimit int    `yaml:"rate_limit"`
}

type ProvidersConfig struct {
    Anthropic ProviderConfig `yaml:"anthropic"`
    OpenAI    ProviderConfig `yaml:"openai"`
    Gemini    ProviderConfig `yaml:"gemini"`
    Groq      ProviderConfig `yaml:"groq"`
}

type ProviderConfig struct {
    APIKey  string `yaml:"api_key"`
    BaseURL string `yaml:"base_url"`
}

type VoiceConfig struct {
    STT map[string]ProviderConfig `yaml:"stt"`
    TTS map[string]ProviderConfig `yaml:"tts"`
}

type DefaultsConfig struct {
    MaxTokens   int     `yaml:"max_tokens"`
    Temperature float64 `yaml:"temperature"`
}

type RateLimitsConfig struct {
    Global  RateLimitConfig `yaml:"global"`
    PerUser RateLimitConfig `yaml:"per_user"`
}

type RateLimitConfig struct {
    RequestsPerMinute int `yaml:"requests_per_minute"`
    TokensPerMinute   int `yaml:"tokens_per_minute"`
}

type ObservabilityConfig struct {
    Metrics MetricsConfig `yaml:"metrics"`
    Logging LoggingConfig `yaml:"logging"`
    Tracing TracingConfig `yaml:"tracing"`
}

type MetricsConfig struct {
    Enabled bool   `yaml:"enabled"`
    Path    string `yaml:"path"`
}

type LoggingConfig struct {
    Level  string `yaml:"level"`
    Format string `yaml:"format"`
}

type TracingConfig struct {
    Enabled  bool   `yaml:"enabled"`
    Exporter string `yaml:"exporter"`
    Endpoint string `yaml:"endpoint"`
}

// Load loads configuration from file and environment.
func Load(path string) (*Config, error) {
    cfg := &Config{
        Server: ServerConfig{
            Host: "0.0.0.0",
            Port: 8080,
        },
        Auth: AuthConfig{
            Mode: "none",
        },
        Defaults: DefaultsConfig{
            MaxTokens:   4096,
            Temperature: 1.0,
        },
        Observability: ObservabilityConfig{
            Metrics: MetricsConfig{
                Enabled: true,
                Path:    "/metrics",
            },
            Logging: LoggingConfig{
                Level:  "info",
                Format: "json",
            },
        },
    }

    // Load from file if provided
    if path != "" {
        data, err := os.ReadFile(path)
        if err != nil {
            return nil, err
        }
        if err := yaml.Unmarshal(data, cfg); err != nil {
            return nil, err
        }
    }

    // Override from environment
    cfg.loadEnv()

    return cfg, nil
}

func (c *Config) loadEnv() {
    // Server
    if port := os.Getenv("VANGO_PORT"); port != "" {
        fmt.Sscanf(port, "%d", &c.Server.Port)
    }

    // Provider keys
    if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
        c.Providers.Anthropic.APIKey = key
    }
    if key := os.Getenv("OPENAI_API_KEY"); key != "" {
        c.Providers.OpenAI.APIKey = key
    }
    if key := os.Getenv("GOOGLE_API_KEY"); key != "" {
        c.Providers.Gemini.APIKey = key
    }
    if key := os.Getenv("GROQ_API_KEY"); key != "" {
        c.Providers.Groq.APIKey = key
    }

    // Voice provider keys
    if key := os.Getenv("DEEPGRAM_API_KEY"); key != "" {
        if c.Voice.STT == nil {
            c.Voice.STT = make(map[string]ProviderConfig)
        }
        c.Voice.STT["deepgram"] = ProviderConfig{APIKey: key}
    }
    if key := os.Getenv("ELEVENLABS_API_KEY"); key != "" {
        if c.Voice.TTS == nil {
            c.Voice.TTS = make(map[string]ProviderConfig)
        }
        c.Voice.TTS["elevenlabs"] = ProviderConfig{APIKey: key}
    }
}
```

### 8.4 Server (pkg/proxy/server.go)

```go
package proxy

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/prometheus/client_golang/prometheus/promhttp"

    "github.com/vango-go/vai/pkg/core"
    "github.com/vango-go/vai/pkg/proxy/config"
    "github.com/vango-go/vai/pkg/proxy/handlers"
    "github.com/vango-go/vai/pkg/proxy/middleware"
)

// Server is the Vango proxy HTTP server.
type Server struct {
    config     *config.Config
    httpServer *http.Server
    engine     *core.Engine
    router     *chi.Mux
}

// NewServer creates a new proxy server.
func NewServer(cfg *config.Config) (*Server, error) {
    // Initialize core engine with provider keys
    providerKeys := map[string]string{
        "anthropic": cfg.Providers.Anthropic.APIKey,
        "openai":    cfg.Providers.OpenAI.APIKey,
        "gemini":    cfg.Providers.Gemini.APIKey,
        "groq":      cfg.Providers.Groq.APIKey,
    }
    engine := core.NewEngine(providerKeys)

    // Create router
    r := chi.NewRouter()

    s := &Server{
        config: cfg,
        engine: engine,
        router: r,
    }

    s.setupMiddleware()
    s.setupRoutes()

    s.httpServer = &http.Server{
        Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
        Handler:      r,
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 120 * time.Second,
        IdleTimeout:  120 * time.Second,
    }

    return s, nil
}

func (s *Server) setupMiddleware() {
    // Recovery
    s.router.Use(middleware.Recoverer)

    // Request ID
    s.router.Use(middleware.RequestID)

    // Logging
    s.router.Use(middleware.Logger(s.config.Observability.Logging))

    // Metrics
    if s.config.Observability.Metrics.Enabled {
        s.router.Use(middleware.Metrics)
    }

    // Tracing
    if s.config.Observability.Tracing.Enabled {
        s.router.Use(middleware.Tracing(s.config.Observability.Tracing))
    }

    // Authentication
    if s.config.Auth.Mode != "none" {
        s.router.Use(middleware.Auth(s.config.Auth))
    }

    // Rate limiting
    if s.config.RateLimits.Global.RequestsPerMinute > 0 {
        s.router.Use(middleware.RateLimit(s.config.RateLimits))
    }
}

func (s *Server) setupRoutes() {
    // Health check
    s.router.Get("/health", handlers.Health(s.engine))

    // Metrics endpoint
    if s.config.Observability.Metrics.Enabled {
        s.router.Handle(s.config.Observability.Metrics.Path, promhttp.Handler())
    }

    // API v1
    s.router.Route("/v1", func(r chi.Router) {
        // Messages
        r.Post("/messages", handlers.CreateMessage(s.engine, s.config))

        // Audio
        r.Post("/audio", handlers.Audio(s.engine, s.config))

        // Models
        r.Get("/models", handlers.ListModels(s.engine))

        // Live sessions (WebSocket)
        r.Get("/messages/live", handlers.Live(s.engine, s.config))
    })
}

// Start starts the server.
func (s *Server) Start() error {
    if s.config.Server.TLS.Enabled {
        return s.httpServer.ListenAndServeTLS(
            s.config.Server.TLS.CertFile,
            s.config.Server.TLS.KeyFile,
        )
    }
    return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
    return s.httpServer.Shutdown(ctx)
}
```

### 8.5 Messages Handler (pkg/proxy/handlers/messages.go)

```go
package handlers

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/vango-go/vai/pkg/core"
    "github.com/vango-go/vai/pkg/core/types"
    "github.com/vango-go/vai/pkg/proxy/config"
)

// CreateMessage handles POST /v1/messages
func CreateMessage(engine *core.Engine, cfg *config.Config) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()

        // Parse request
        var req types.MessageRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
            return
        }

        // Apply defaults
        if req.MaxTokens == 0 {
            req.MaxTokens = cfg.Defaults.MaxTokens
        }

        // Resolve aliases
        if alias, ok := cfg.Aliases[req.Model]; ok {
            req.Model = alias
        }

        // Get provider
        provider, err := engine.GetProvider(req.Model)
        if err != nil {
            writeError(w, http.StatusNotFound, "not_found_error", err.Error())
            return
        }

        // Handle streaming vs non-streaming
        if req.Stream {
            handleStreamingResponse(ctx, w, provider, &req)
        } else {
            handleNonStreamingResponse(ctx, w, provider, &req)
        }
    }
}

func handleNonStreamingResponse(ctx context.Context, w http.ResponseWriter, provider core.Provider, req *types.MessageRequest) {
    startTime := time.Now()

    resp, err := provider.CreateMessage(ctx, req)
    if err != nil {
        handleProviderError(w, err)
        return
    }

    duration := time.Since(startTime)

    // Set response headers
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("X-Request-ID", getRequestID(ctx))
    w.Header().Set("X-Model", resp.Model)
    w.Header().Set("X-Input-Tokens", fmt.Sprintf("%d", resp.Usage.InputTokens))
    w.Header().Set("X-Output-Tokens", fmt.Sprintf("%d", resp.Usage.OutputTokens))
    w.Header().Set("X-Duration-Ms", fmt.Sprintf("%d", duration.Milliseconds()))

    if resp.Usage.CostUSD != nil {
        w.Header().Set("X-Cost-USD", fmt.Sprintf("%.6f", *resp.Usage.CostUSD))
    }

    json.NewEncoder(w).Encode(resp)
}

func handleStreamingResponse(ctx context.Context, w http.ResponseWriter, provider core.Provider, req *types.MessageRequest) {
    // Set SSE headers
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("X-Request-ID", getRequestID(ctx))

    flusher, ok := w.(http.Flusher)
    if !ok {
        writeError(w, http.StatusInternalServerError, "api_error", "streaming not supported")
        return
    }

    stream, err := provider.StreamMessage(ctx, req)
    if err != nil {
        writeSSEError(w, flusher, err)
        return
    }
    defer stream.Close()

    for {
        event, err := stream.Next()
        if err != nil {
            if err == io.EOF {
                break
            }
            writeSSEError(w, flusher, err)
            return
        }

        if event == nil {
            continue
        }

        // Write SSE event
        eventType := event.eventType()
        eventData, _ := json.Marshal(event)

        fmt.Fprintf(w, "event: %s\n", eventType)
        fmt.Fprintf(w, "data: %s\n\n", eventData)
        flusher.Flush()
    }
}

func writeSSEError(w http.ResponseWriter, flusher http.Flusher, err error) {
    errEvent := types.ErrorEvent{
        Type: "error",
        Error: core.Error{
            Type:    core.ErrAPI,
            Message: err.Error(),
        },
    }
    data, _ := json.Marshal(errEvent)
    fmt.Fprintf(w, "event: error\n")
    fmt.Fprintf(w, "data: %s\n\n", data)
    flusher.Flush()
}

func handleProviderError(w http.ResponseWriter, err error) {
    if apiErr, ok := err.(*core.Error); ok {
        status := mapErrorToStatus(apiErr.Type)
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(status)
        json.NewEncoder(w).Encode(map[string]any{
            "type":  "error",
            "error": apiErr,
        })
        return
    }

    writeError(w, http.StatusInternalServerError, "api_error", err.Error())
}

func mapErrorToStatus(errType core.ErrorType) int {
    switch errType {
    case core.ErrInvalidRequest:
        return http.StatusBadRequest
    case core.ErrAuthentication:
        return http.StatusUnauthorized
    case core.ErrPermission:
        return http.StatusForbidden
    case core.ErrNotFound:
        return http.StatusNotFound
    case core.ErrRateLimit:
        return http.StatusTooManyRequests
    case core.ErrOverloaded:
        return http.StatusServiceUnavailable
    case core.ErrProvider:
        return http.StatusBadGateway
    default:
        return http.StatusInternalServerError
    }
}

func writeError(w http.ResponseWriter, status int, errType, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(map[string]any{
        "type": "error",
        "error": map[string]any{
            "type":    errType,
            "message": message,
        },
    })
}

func getRequestID(ctx context.Context) string {
    if id, ok := ctx.Value("request_id").(string); ok {
        return id
    }
    return ""
}
```

### 8.6 Authentication Middleware (pkg/proxy/middleware/auth.go)

```go
package middleware

import (
    "context"
    "net/http"
    "strings"

    "github.com/vango-go/vai/pkg/proxy/config"
)

type contextKey string

const (
    UserIDKey contextKey = "user_id"
    APIKeyKey contextKey = "api_key"
)

// Auth returns authentication middleware.
func Auth(cfg config.AuthConfig) func(http.Handler) http.Handler {
    // Build API key lookup
    apiKeys := make(map[string]*config.APIKeyConfig)
    for i := range cfg.APIKeys {
        apiKeys[cfg.APIKeys[i].Key] = &cfg.APIKeys[i]
    }

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Skip auth for health check
            if r.URL.Path == "/health" || r.URL.Path == "/metrics" {
                next.ServeHTTP(w, r)
                return
            }

            switch cfg.Mode {
            case "api_key":
                key := extractAPIKey(r)
                if key == "" {
                    writeAuthError(w, "Missing API key")
                    return
                }

                keyConfig, ok := apiKeys[key]
                if !ok {
                    writeAuthError(w, "Invalid API key")
                    return
                }

                // Add user context
                ctx := context.WithValue(r.Context(), UserIDKey, keyConfig.UserID)
                ctx = context.WithValue(ctx, APIKeyKey, key)
                r = r.WithContext(ctx)

            case "passthrough":
                // Just extract and pass through
                key := extractAPIKey(r)
                if key != "" {
                    ctx := context.WithValue(r.Context(), APIKeyKey, key)
                    r = r.WithContext(ctx)
                }

            case "none":
                // No auth required
            }

            next.ServeHTTP(w, r)
        })
    }
}

func extractAPIKey(r *http.Request) string {
    // Check Authorization header
    auth := r.Header.Get("Authorization")
    if strings.HasPrefix(auth, "Bearer ") {
        return strings.TrimPrefix(auth, "Bearer ")
    }

    // Check X-API-Key header
    if key := r.Header.Get("X-API-Key"); key != "" {
        return key
    }

    return ""
}

func writeAuthError(w http.ResponseWriter, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusUnauthorized)
    json.NewEncoder(w).Encode(map[string]any{
        "type": "error",
        "error": map[string]any{
            "type":    "authentication_error",
            "message": message,
        },
    })
}
```

### 8.7 Rate Limiting Middleware (pkg/proxy/middleware/ratelimit.go)

```go
package middleware

import (
    "net/http"
    "sync"
    "time"

    "github.com/vango-go/vai/pkg/proxy/config"
    "golang.org/x/time/rate"
)

// RateLimit returns rate limiting middleware.
func RateLimit(cfg config.RateLimitsConfig) func(http.Handler) http.Handler {
    // Global limiter
    globalLimiter := rate.NewLimiter(
        rate.Limit(cfg.Global.RequestsPerMinute)/60,
        cfg.Global.RequestsPerMinute,
    )

    // Per-user limiters
    var mu sync.Mutex
    userLimiters := make(map[string]*rate.Limiter)

    getUserLimiter := func(userID string) *rate.Limiter {
        mu.Lock()
        defer mu.Unlock()

        if limiter, ok := userLimiters[userID]; ok {
            return limiter
        }

        limiter := rate.NewLimiter(
            rate.Limit(cfg.PerUser.RequestsPerMinute)/60,
            cfg.PerUser.RequestsPerMinute,
        )
        userLimiters[userID] = limiter
        return limiter
    }

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Check global limit
            if !globalLimiter.Allow() {
                writeRateLimitError(w, 30)
                return
            }

            // Check per-user limit
            if userID, ok := r.Context().Value(UserIDKey).(string); ok && userID != "" {
                limiter := getUserLimiter(userID)
                if !limiter.Allow() {
                    writeRateLimitError(w, 30)
                    return
                }
            }

            next.ServeHTTP(w, r)
        })
    }
}

func writeRateLimitError(w http.ResponseWriter, retryAfter int) {
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
    w.Header().Set("X-RateLimit-Remaining", "0")
    w.WriteHeader(http.StatusTooManyRequests)
    json.NewEncoder(w).Encode(map[string]any{
        "type": "error",
        "error": map[string]any{
            "type":        "rate_limit_error",
            "message":     "Rate limit exceeded. Please retry after " + fmt.Sprintf("%d", retryAfter) + " seconds.",
            "retry_after": retryAfter,
        },
    })
}
```

### 8.8 Metrics Middleware (pkg/proxy/middleware/metrics.go)

```go
package middleware

import (
    "net/http"
    "strconv"
    "time"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    requestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "vango_requests_total",
            Help: "Total number of API requests",
        },
        []string{"method", "path", "status", "provider", "model"},
    )

    requestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "vango_request_duration_seconds",
            Help:    "Request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "path", "provider"},
    )

    tokensTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "vango_tokens_total",
            Help: "Total tokens processed",
        },
        []string{"provider", "direction"},
    )

    costTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "vango_cost_usd_total",
            Help: "Total cost in USD",
        },
        []string{"provider"},
    )
)

// Metrics returns metrics middleware.
func Metrics(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()

        // Wrap response writer to capture status
        ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}

        next.ServeHTTP(ww, r)

        duration := time.Since(start).Seconds()

        // Extract provider and model from context if available
        provider := "unknown"
        model := "unknown"
        if p, ok := r.Context().Value("provider").(string); ok {
            provider = p
        }
        if m, ok := r.Context().Value("model").(string); ok {
            model = m
        }

        requestsTotal.WithLabelValues(
            r.Method,
            r.URL.Path,
            strconv.Itoa(ww.status),
            provider,
            model,
        ).Inc()

        requestDuration.WithLabelValues(
            r.Method,
            r.URL.Path,
            provider,
        ).Observe(duration)
    })
}

// RecordTokens records token usage metrics.
func RecordTokens(provider string, input, output int) {
    tokensTotal.WithLabelValues(provider, "input").Add(float64(input))
    tokensTotal.WithLabelValues(provider, "output").Add(float64(output))
}

// RecordCost records cost metrics.
func RecordCost(provider string, cost float64) {
    costTotal.WithLabelValues(provider).Add(cost)
}

type responseWriter struct {
    http.ResponseWriter
    status int
}

func (w *responseWriter) WriteHeader(status int) {
    w.status = status
    w.ResponseWriter.WriteHeader(status)
}
```

### 8.9 SDK Proxy Mode (sdk/internal/proxy/client.go)

```go
package proxy

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"

    "github.com/vango-go/vai/pkg/core/types"
)

// Client is an HTTP client for the Vango Proxy.
type Client struct {
    baseURL    string
    apiKey     string
    httpClient *http.Client
}

// NewClient creates a new proxy client.
func NewClient(baseURL, apiKey string, httpClient *http.Client) *Client {
    return &Client{
        baseURL:    baseURL,
        apiKey:     apiKey,
        httpClient: httpClient,
    }
}

// CreateMessage sends a message request to the proxy.
func (c *Client) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
    body, err := json.Marshal(req)
    if err != nil {
        return nil, err
    }

    httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(body))
    if err != nil {
        return nil, err
    }

    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 400 {
        return nil, c.parseError(resp)
    }

    var result types.MessageResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }

    return &result, nil
}

// StreamMessage sends a streaming request to the proxy.
func (c *Client) StreamMessage(ctx context.Context, req *types.MessageRequest) (io.ReadCloser, error) {
    req.Stream = true

    body, err := json.Marshal(req)
    if err != nil {
        return nil, err
    }

    httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(body))
    if err != nil {
        return nil, err
    }

    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
    httpReq.Header.Set("Accept", "text/event-stream")

    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, err
    }

    if resp.StatusCode >= 400 {
        defer resp.Body.Close()
        return nil, c.parseError(resp)
    }

    return resp.Body, nil
}

func (c *Client) parseError(resp *http.Response) error {
    body, _ := io.ReadAll(resp.Body)

    var errResp struct {
        Error struct {
            Type    string `json:"type"`
            Message string `json:"message"`
        } `json:"error"`
    }

    if err := json.Unmarshal(body, &errResp); err != nil {
        return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
    }

    return &core.Error{
        Type:    core.ErrorType(errResp.Error.Type),
        Message: errResp.Error.Message,
    }
}
```

### 8.10 Docker Configuration

**Dockerfile:**
```dockerfile
# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o vango-proxy ./cmd/vango-proxy

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/vango-proxy .
COPY --from=builder /app/config.example.yaml ./config.yaml

EXPOSE 8080

ENTRYPOINT ["./vango-proxy"]
CMD ["-config", "config.yaml"]
```

**docker-compose.yml:**
```yaml
version: '3.8'

services:
  vango-proxy:
    build: .
    ports:
      - "8080:8080"
    environment:
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - GOOGLE_API_KEY=${GOOGLE_API_KEY}
      - DEEPGRAM_API_KEY=${DEEPGRAM_API_KEY}
      - ELEVENLABS_API_KEY=${ELEVENLABS_API_KEY}
    volumes:
      - ./vai.yaml:/app/config.yaml:ro
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
```

---

## Testing Strategy

### Unit Tests
- Configuration loading
- Authentication middleware
- Rate limiting logic
- Error mapping

### Integration Tests

```go
// +build integration

func TestProxy_Messages(t *testing.T) {
    // Start test server
    cfg := &config.Config{
        Server: config.ServerConfig{Port: 0}, // Random port
        Auth:   config.AuthConfig{Mode: "none"},
    }
    server, _ := proxy.NewServer(cfg)
    go server.Start()
    defer server.Shutdown(context.Background())

    // Wait for server to start
    time.Sleep(100 * time.Millisecond)

    // Test via HTTP client
    client := vai.NewClient(
        vai.WithBaseURL("http://localhost:"+server.Port()),
    )

    resp, err := client.Messages.Create(context.Background(), &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Hello")},
        },
    })

    require.NoError(t, err)
    assert.NotEmpty(t, resp.TextContent())
}

func TestProxy_Auth(t *testing.T) {
    cfg := &config.Config{
        Auth: config.AuthConfig{
            Mode: "api_key",
            APIKeys: []config.APIKeyConfig{
                {Key: "test_key", Name: "Test", UserID: "user1"},
            },
        },
    }
    server, _ := proxy.NewServer(cfg)
    go server.Start()
    defer server.Shutdown(context.Background())

    // Without key - should fail
    client := vai.NewClient(
        vai.WithBaseURL("http://localhost:"+server.Port()),
    )
    _, err := client.Messages.Create(context.Background(), &vai.MessageRequest{...})
    assert.Error(t, err)

    // With valid key - should work
    client = vai.NewClient(
        vai.WithBaseURL("http://localhost:"+server.Port()),
        vai.WithAPIKey("test_key"),
    )
    _, err = client.Messages.Create(context.Background(), &vai.MessageRequest{...})
    assert.NoError(t, err)
}

func TestProxy_RateLimit(t *testing.T) {
    cfg := &config.Config{
        RateLimits: config.RateLimitsConfig{
            Global: config.RateLimitConfig{RequestsPerMinute: 2},
        },
    }
    server, _ := proxy.NewServer(cfg)
    go server.Start()
    defer server.Shutdown(context.Background())

    client := vai.NewClient(
        vai.WithBaseURL("http://localhost:"+server.Port()),
    )

    // First two should succeed
    _, err1 := client.Messages.Create(ctx, &req)
    _, err2 := client.Messages.Create(ctx, &req)

    // Third should fail with rate limit
    _, err3 := client.Messages.Create(ctx, &req)

    assert.NoError(t, err1)
    assert.NoError(t, err2)

    var apiErr *vai.Error
    assert.True(t, errors.As(err3, &apiErr))
    assert.Equal(t, vai.ErrRateLimit, apiErr.Type)
}
```

---

## Acceptance Criteria

1. [ ] Server starts and serves /health endpoint
2. [ ] POST /v1/messages works (non-streaming)
3. [ ] POST /v1/messages works (streaming)
4. [ ] POST /v1/audio works
5. [ ] GET /v1/models works
6. [ ] WebSocket /v1/messages/live works
7. [ ] API key authentication works
8. [ ] Rate limiting works
9. [ ] Prometheus metrics are exposed
10. [ ] Structured logging works
11. [ ] Configuration file loading works
12. [ ] Environment variable overrides work
13. [ ] Docker image builds and runs
14. [ ] SDK Proxy Mode works end-to-end

---

## Files to Create

```
cmd/vango-proxy/main.go
pkg/proxy/server.go
pkg/proxy/handlers/messages.go
pkg/proxy/handlers/audio.go
pkg/proxy/handlers/models.go
pkg/proxy/handlers/live.go
pkg/proxy/handlers/health.go
pkg/proxy/middleware/auth.go
pkg/proxy/middleware/ratelimit.go
pkg/proxy/middleware/logging.go
pkg/proxy/middleware/metrics.go
pkg/proxy/middleware/tracing.go
pkg/proxy/middleware/recovery.go
pkg/proxy/middleware/requestid.go
pkg/proxy/config/config.go
pkg/proxy/config/loader.go
pkg/proxy/errors.go
sdk/internal/proxy/client.go
docker/Dockerfile
docker/docker-compose.yml
config.example.yaml
```

---

## Estimated Effort

- Server and handlers: ~600 lines
- Middleware: ~400 lines
- Configuration: ~200 lines
- SDK proxy client: ~200 lines
- Docker: ~50 lines
- Tests: ~400 lines
- **Total: ~1850 lines**

---

## Summary

With Phase 8 complete, Vango will be a fully functional AI gateway:

| Mode | Use Case | Setup |
|------|----------|-------|
| **Direct Mode** | Go apps, CLI tools | `go get` + env vars |
| **Proxy Mode** | Production, polyglot | Docker deploy |

The shared `pkg/core` ensures both modes behave identically.
