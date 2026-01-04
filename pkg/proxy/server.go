package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vango-go/vai/pkg/core"
	"github.com/vango-go/vai/pkg/core/providers/anthropic"
	"github.com/vango-go/vai/pkg/core/types"
	"github.com/vango-go/vai/pkg/core/voice"
)

// Server is the Vango AI proxy server.
type Server struct {
	config *Config
	logger *slog.Logger

	// Core components
	engine        *core.Engine
	voicePipeline *voice.Pipeline

	// HTTP server
	httpServer *http.Server
	mux        *http.ServeMux

	// Middleware
	auth        *AuthMiddleware
	rateLimiter *RateLimiter
	logging     *LoggingMiddleware
	recovery    *RecoveryMiddleware
	cors        *CORSMiddleware

	// Metrics
	metrics *Metrics

	// WebSocket upgrader
	upgrader websocket.Upgrader

	// Lifecycle
	done     chan struct{}
	shutdown atomic.Bool
}

// NewServer creates a new proxy server.
func NewServer(opts ...ConfigOption) (*Server, error) {
	config := DefaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	// Load provider keys from environment if not set
	config.LoadProviderKeysFromEnv()

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Initialize metrics
	metrics := NewMetrics("vango")

	// Initialize core engine
	engine := core.NewEngine(config.ProviderKeys)

	// Register providers
	if anthropicKey := config.ProviderKeys["anthropic"]; anthropicKey != "" {
		provider := anthropic.New(anthropicKey)
		engine.RegisterProvider(newAnthropicAdapter(provider))
	} else if envKey := os.Getenv("ANTHROPIC_API_KEY"); envKey != "" {
		provider := anthropic.New(envKey)
		engine.RegisterProvider(newAnthropicAdapter(provider))
	}

	// Initialize voice pipeline
	var voicePipeline *voice.Pipeline
	cartesiaKey := config.ProviderKeys["cartesia"]
	if cartesiaKey == "" {
		cartesiaKey = os.Getenv("CARTESIA_API_KEY")
	}
	if cartesiaKey != "" {
		voicePipeline = voice.NewPipeline(cartesiaKey)
	}

	s := &Server{
		config:        config,
		logger:        logger,
		engine:        engine,
		voicePipeline: voicePipeline,
		metrics:       metrics,
		done:          make(chan struct{}),
		upgrader: websocket.Upgrader{
			HandshakeTimeout: 10 * time.Second,
			ReadBufferSize:   4096,
			WriteBufferSize:  4096,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for now; configure per deployment
			},
		},
	}

	// Initialize middleware
	s.auth = NewAuthMiddleware(config.APIKeys, logger, metrics)
	s.rateLimiter = NewRateLimiter(config.RateLimit, logger, metrics)
	s.logging = NewLoggingMiddleware(logger)
	s.recovery = NewRecoveryMiddleware(logger, metrics)
	s.cors = NewCORSMiddleware(nil)

	// Set up routes
	s.setupRoutes()

	return s, nil
}

// setupRoutes configures the HTTP router.
func (s *Server) setupRoutes() {
	s.mux = http.NewServeMux()

	// Health check (no auth required)
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// Metrics (no auth required by default)
	if s.config.Observability.MetricsEnabled {
		s.mux.Handle("GET "+s.config.Observability.MetricsPath, s.metrics.Handler())
	}

	// API endpoints (with auth and rate limiting)
	apiHandler := s.withMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/messages":
			s.handleMessages(w, r)
		case r.Method == "GET" && r.URL.Path == "/v1/models":
			s.handleModels(w, r)
		case r.Method == "POST" && r.URL.Path == "/v1/audio":
			s.handleAudio(w, r)
		default:
			s.writeError(w, http.StatusNotFound, "not_found_error", "Endpoint not found")
		}
	}))
	s.mux.Handle("/v1/", apiHandler)

	// WebSocket endpoint for live sessions
	//s.mux.HandleFunc("GET /v1/messages/live", s.handleLive)
}

// withMiddleware wraps a handler with all middleware.
func (s *Server) withMiddleware(handler http.Handler) http.Handler {
	// Apply middleware in reverse order (innermost first)
	handler = s.recovery.Recover(handler)
	handler = s.rateLimiter.RateLimit(handler)
	handler = s.auth.Authenticate(handler)
	handler = s.cors.Handle(handler)
	handler = s.logging.Log(handler)
	return handler
}

// Start starts the server.
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.mux,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	s.logger.Info("server starting",
		"addr", addr,
		"tls", s.config.TLSEnabled,
	)

	// Start cleanup goroutine
	go s.cleanupLoop()

	if s.config.TLSEnabled {
		return s.httpServer.ServeTLS(listener, s.config.TLSCertFile, s.config.TLSKeyFile)
	}
	return s.httpServer.Serve(listener)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.shutdown.Swap(true) {
		return nil
	}
	close(s.done)

	s.logger.Info("server shutting down")

	// Shutdown HTTP server if started
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// cleanupLoop periodically cleans up stale data.
func (s *Server) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.rateLimiter.Cleanup()
		}
	}
}

// handleHealth handles health check requests.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]any{
		"status":  "healthy",
		"version": "1.0.0",
		"providers": map[string]any{
			"anthropic": map[string]any{
				"status": "healthy",
			},
		},
	}

	if s.voicePipeline != nil {
		health["voice"] = map[string]any{
			"status": "available",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

// handleMessages handles /v1/messages requests.
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Parse request
	var req MessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "Invalid JSON: "+err.Error())
		return
	}

	// Extract provider and model
	provider, model, err := core.ParseModelString(req.Model)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	// Process request
	resp, err := s.engine.CreateMessage(r.Context(), &req)
	if err != nil {
		s.metrics.RecordError(provider, "request_error")
		s.writeError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}

	// Record metrics
	duration := time.Since(start)
	s.metrics.RecordRequest(provider, model, "/v1/messages", "success", duration)
	if resp.Usage.InputTokens > 0 || resp.Usage.OutputTokens > 0 {
		s.metrics.RecordTokens(provider, model, resp.Usage.InputTokens, resp.Usage.OutputTokens)
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", r.Context().Value(ContextKeyRequestID).(string))
	w.Header().Set("X-Model", req.Model)
	w.Header().Set("X-Input-Tokens", fmt.Sprint(resp.Usage.InputTokens))
	w.Header().Set("X-Output-Tokens", fmt.Sprint(resp.Usage.OutputTokens))
	w.Header().Set("X-Duration-Ms", fmt.Sprint(duration.Milliseconds()))

	json.NewEncoder(w).Encode(resp)
}

// handleModels handles /v1/models requests.
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	// Return available models
	models := map[string]any{
		"providers": []map[string]any{
			{
				"id":   "anthropic",
				"name": "Anthropic",
				"models": []map[string]any{
					{
						"id":      "claude-sonnet-4",
						"name":    "Claude Sonnet 4",
						"full_id": "anthropic/claude-sonnet-4",
					},
					{
						"id":      "claude-opus-4",
						"name":    "Claude Opus 4",
						"full_id": "anthropic/claude-opus-4",
					},
					{
						"id":      "claude-haiku-4-5-20251001",
						"name":    "Claude Haiku 4.5",
						"full_id": "anthropic/claude-haiku-4-5-20251001",
					},
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models)
}

// handleAudio handles /v1/audio requests.
func (s *Server) handleAudio(w http.ResponseWriter, r *http.Request) {
	if s.voicePipeline == nil {
		s.writeError(w, http.StatusServiceUnavailable, "api_error", "Voice pipeline not configured")
		return
	}

	// Parse request
	var req AudioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "Invalid JSON: "+err.Error())
		return
	}

	// Determine operation based on fields
	if req.Audio != "" {
		s.handleTranscribe(w, r, &req)
	} else if req.Text != "" {
		s.handleSynthesize(w, r, &req)
	} else {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "Either 'audio' or 'text' field is required")
	}
}

func (s *Server) handleTranscribe(w http.ResponseWriter, r *http.Request, req *AudioRequest) {
	// TODO: Implement transcription via voice pipeline
	s.writeError(w, http.StatusNotImplemented, "api_error", "Transcription not yet implemented")
}

func (s *Server) handleSynthesize(w http.ResponseWriter, r *http.Request, req *AudioRequest) {
	// TODO: Implement synthesis via voice pipeline
	s.writeError(w, http.StatusNotImplemented, "api_error", "Synthesis not yet implemented")
}

// writeError writes an error response.
func (s *Server) writeError(w http.ResponseWriter, status int, errType, message string) {
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

// MessageRequest is the request body for /v1/messages.
type MessageRequest = types.MessageRequest

// AudioRequest is the request body for /v1/audio.
type AudioRequest struct {
	// Transcription fields
	Audio    string `json:"audio,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Language string `json:"language,omitempty"`

	// Synthesis fields
	Text   string  `json:"text,omitempty"`
	Voice  string  `json:"voice,omitempty"`
	Speed  float64 `json:"speed,omitempty"`
	Format string  `json:"format,omitempty"`
	Stream bool    `json:"stream,omitempty"`
}

// Anthropic adapter - wraps the anthropic provider to implement core.Provider
type anthropicAdapter struct {
	provider *anthropic.Provider
}

func newAnthropicAdapter(provider *anthropic.Provider) *anthropicAdapter {
	return &anthropicAdapter{provider: provider}
}

func (a *anthropicAdapter) Name() string {
	return "anthropic"
}

func (a *anthropicAdapter) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
	return a.provider.CreateMessage(ctx, req)
}

func (a *anthropicAdapter) StreamMessage(ctx context.Context, req *types.MessageRequest) (core.EventStream, error) {
	return a.provider.StreamMessage(ctx, req)
}

func (a *anthropicAdapter) Capabilities() core.ProviderCapabilities {
	caps := a.provider.Capabilities()
	return core.ProviderCapabilities{
		Vision:           caps.Vision,
		AudioInput:       caps.AudioInput,
		AudioOutput:      caps.AudioOutput,
		Video:            caps.Video,
		Tools:            caps.Tools,
		ToolStreaming:    caps.ToolStreaming,
		Thinking:         caps.Thinking,
		StructuredOutput: caps.StructuredOutput,
		NativeTools:      caps.NativeTools,
	}
}
