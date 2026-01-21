package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vango-go/vai/pkg/core"
	"github.com/vango-go/vai/pkg/core/live"
	"github.com/vango-go/vai/pkg/core/providers/anthropic"
	"github.com/vango-go/vai/pkg/core/providers/cerebras"
	"github.com/vango-go/vai/pkg/core/providers/gemini"
	"github.com/vango-go/vai/pkg/core/providers/gemini_oauth"
	"github.com/vango-go/vai/pkg/core/providers/groq"
	"github.com/vango-go/vai/pkg/core/providers/oai_resp"
	"github.com/vango-go/vai/pkg/core/providers/openai"
	"github.com/vango-go/vai/pkg/core/types"
	"github.com/vango-go/vai/pkg/core/voice"
	"github.com/vango-go/vai/pkg/core/voice/stt"
	"github.com/vango-go/vai/pkg/core/voice/tts"
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
	bodyLimiter *RequestBodyLimitMiddleware

	// Metrics
	metrics *Metrics

	// WebSocket upgrader
	upgrader websocket.Upgrader

	// Lifecycle
	done     chan struct{}
	shutdown atomic.Bool

	// Live session tracking
	liveSessions atomic.Int64
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

	registerProviders(engine, config.ProviderKeys, logger)

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
	s.auth = NewAuthMiddleware(config.AuthMode, config.APIKeys, logger, metrics)
	s.rateLimiter = NewRateLimiter(config.RateLimit, logger, metrics)
	s.logging = NewLoggingMiddleware(logger)
	s.recovery = NewRecoveryMiddleware(logger, metrics)
	s.cors = NewCORSMiddleware(config.AllowedOrigins)
	s.bodyLimiter = NewRequestBodyLimitMiddleware(config.MaxRequestBodyBytes)

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
			writeJSONErrorWithStatus(w, http.StatusNotFound, core.NewNotFoundError("Endpoint not found"), requestIDFromContext(r.Context()))
		}
	}))
	s.mux.Handle("/v1/", apiHandler)

	// WebSocket endpoint for live sessions
	s.mux.HandleFunc("GET /v1/messages/live", s.handleLive)
}

// withMiddleware wraps a handler with all middleware.
func (s *Server) withMiddleware(handler http.Handler) http.Handler {
	// Apply middleware in reverse order (innermost first)
	handler = s.recovery.Recover(handler)
	handler = s.rateLimiter.RateLimit(handler)
	handler = s.auth.Authenticate(handler)
	handler = s.bodyLimiter.Limit(handler)
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
	var req proxyMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONErrorWithStatus(w, http.StatusRequestEntityTooLarge, core.NewInvalidRequestError("Request body too large"), requestIDFromContext(r.Context()))
			return
		}
		writeJSONErrorWithStatus(w, http.StatusBadRequest, core.NewInvalidRequestError("Invalid JSON: "+err.Error()), requestIDFromContext(r.Context()))
		return
	}

	providerKeys := extractProviderKeys(r, req.ProviderKeys)
	if s.config.AuthMode == "passthrough" && len(providerKeys) == 0 {
		writeJSONErrorWithStatus(w, http.StatusUnauthorized, core.NewAuthenticationError("Missing provider keys"), requestIDFromContext(r.Context()))
		return
	}

	if req.MessageRequest.Stream {
		s.handleMessagesStream(w, r, &req.MessageRequest, providerKeys, start)
		return
	}

	// Extract provider and model
	provider, model, err := core.ParseModelString(req.MessageRequest.Model)
	if err != nil {
		writeJSONErrorWithStatus(w, http.StatusBadRequest, core.NewInvalidRequestError(err.Error()), requestIDFromContext(r.Context()))
		return
	}

	engine := s.engineForRequest(providerKeys)
	voicePipeline := s.voicePipelineForRequest(providerKeys)

	processedReq := &req.MessageRequest
	var userTranscript string
	if req.MessageRequest.Voice != nil && req.MessageRequest.Voice.Input != nil && voicePipeline != nil {
		processedMessages, transcript, err := voicePipeline.ProcessInputAudio(r.Context(), req.MessageRequest.Messages, req.MessageRequest.Voice)
		if err != nil {
			writeJSONErrorWithStatus(w, http.StatusBadRequest, core.NewInvalidRequestError("Process audio input: "+err.Error()), requestIDFromContext(r.Context()))
			return
		}
		userTranscript = transcript
		reqCopy := req.MessageRequest
		reqCopy.Messages = processedMessages
		processedReq = &reqCopy
	}

	// Process request
	resp, err := engine.CreateMessage(r.Context(), processedReq)
	if err != nil {
		apiErr := normalizeCoreError(err)
		s.metrics.RecordError(provider, string(apiErr.Type))
		writeJSONError(w, apiErr, requestIDFromContext(r.Context()))
		return
	}

	if userTranscript != "" {
		if resp.Metadata == nil {
			resp.Metadata = make(map[string]any)
		}
		resp.Metadata["user_transcript"] = userTranscript
	}

	if req.MessageRequest.Voice != nil && req.MessageRequest.Voice.Output != nil && voicePipeline != nil {
		textContent := resp.TextContent()
		if textContent != "" {
			audioData, err := voicePipeline.SynthesizeResponse(r.Context(), textContent, req.MessageRequest.Voice)
			if err != nil {
				writeJSONError(w, normalizeCoreError(fmt.Errorf("synthesize audio: %w", err)), requestIDFromContext(r.Context()))
				return
			}
			if len(audioData) > 0 {
				format := req.MessageRequest.Voice.Output.Format
				if format == "" {
					format = "wav"
				}
				mediaType := "audio/" + format
				resp.Content = append(resp.Content, types.AudioBlock{
					Type: "audio",
					Source: types.AudioSource{
						Type:      "base64",
						MediaType: mediaType,
						Data:      base64.StdEncoding.EncodeToString(audioData),
					},
					Transcript: &textContent,
				})
			}
		}
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

func (s *Server) handleMessagesStream(w http.ResponseWriter, r *http.Request, req *MessageRequest, providerKeys map[string]string, start time.Time) {
	// Extract provider and model
	provider, model, err := core.ParseModelString(req.Model)
	if err != nil {
		writeJSONErrorWithStatus(w, http.StatusBadRequest, core.NewInvalidRequestError(err.Error()), requestIDFromContext(r.Context()))
		return
	}

	engine := s.engineForRequest(providerKeys)
	voicePipeline := s.voicePipelineForRequest(providerKeys)

	processedReq := req
	if req.Voice != nil && req.Voice.Input != nil && voicePipeline != nil {
		processedMessages, _, err := voicePipeline.ProcessInputAudio(r.Context(), req.Messages, req.Voice)
		if err != nil {
			writeJSONErrorWithStatus(w, http.StatusBadRequest, core.NewInvalidRequestError("Process audio input: "+err.Error()), requestIDFromContext(r.Context()))
			return
		}
		reqCopy := *req
		reqCopy.Messages = processedMessages
		processedReq = &reqCopy
	}

	stream, err := engine.StreamMessage(r.Context(), processedReq)
	if err != nil {
		apiErr := normalizeCoreError(err)
		s.metrics.RecordError(provider, string(apiErr.Type))
		writeJSONError(w, apiErr, requestIDFromContext(r.Context()))
		return
	}
	defer stream.Close()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONErrorWithStatus(w, http.StatusInternalServerError, core.NewAPIError("Streaming not supported"), requestIDFromContext(r.Context()))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Model", req.Model)

	var usage types.Usage
	var haveUsage bool

	for {
		event, err := stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			s.writeSSEError(w, normalizeCoreError(err))
			flusher.Flush()
			break
		}

		switch e := event.(type) {
		case types.MessageStartEvent:
			usage = e.Message.Usage
			haveUsage = true
		case *types.MessageStartEvent:
			usage = e.Message.Usage
			haveUsage = true
		case types.MessageDeltaEvent:
			usage = e.Usage
			haveUsage = true
		case *types.MessageDeltaEvent:
			usage = e.Usage
			haveUsage = true
		}

		payload, err := encodeStreamEvent(event)
		if err != nil {
			s.writeSSEError(w, normalizeCoreError(err))
			flusher.Flush()
			break
		}
		writeSSE(w, event.EventType(), payload)
		flusher.Flush()
	}

	duration := time.Since(start)
	s.metrics.RecordRequest(provider, model, "/v1/messages", "success", duration)
	if haveUsage && (usage.InputTokens > 0 || usage.OutputTokens > 0) {
		s.metrics.RecordTokens(provider, model, usage.InputTokens, usage.OutputTokens)
	}
}

// handleModels handles /v1/models requests.
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	providerNames := s.engine.ProviderNames()
	providers := make([]map[string]any, 0, len(providerNames))

	for _, name := range providerNames {
		provider, ok := s.engine.GetProvider(name)
		if !ok {
			continue
		}
		caps := provider.Capabilities()
		providers = append(providers, map[string]any{
			"id":           provider.Name(),
			"name":         providerDisplayName(provider.Name()),
			"capabilities": caps,
			"models":       []map[string]any{},
		})
	}

	models := map[string]any{
		"providers": providers,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models)
}

func providerDisplayName(provider string) string {
	switch provider {
	case "oai-resp":
		return "OpenAI Responses"
	case "gemini-oauth":
		return "Gemini OAuth"
	case "groq":
		return "Groq"
	case "cerebras":
		return "Cerebras"
	case "gemini":
		return "Google Gemini"
	case "openai":
		return "OpenAI"
	case "anthropic":
		return "Anthropic"
	default:
		return provider
	}
}

// handleAudio handles /v1/audio requests.
func (s *Server) handleAudio(w http.ResponseWriter, r *http.Request) {
	if s.voicePipeline == nil {
		writeJSONErrorWithStatus(w, http.StatusServiceUnavailable, core.NewAPIError("Voice pipeline not configured"), requestIDFromContext(r.Context()))
		return
	}

	// Parse request
	var req AudioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONErrorWithStatus(w, http.StatusRequestEntityTooLarge, core.NewInvalidRequestError("Request body too large"), requestIDFromContext(r.Context()))
			return
		}
		writeJSONErrorWithStatus(w, http.StatusBadRequest, core.NewInvalidRequestError("Invalid JSON: "+err.Error()), requestIDFromContext(r.Context()))
		return
	}

	providerKeys := extractProviderKeys(r, req.ProviderKeys)
	if s.config.AuthMode == "passthrough" && len(providerKeys) == 0 {
		writeJSONErrorWithStatus(w, http.StatusUnauthorized, core.NewAuthenticationError("Missing provider keys"), requestIDFromContext(r.Context()))
		return
	}

	pipeline := s.voicePipelineForRequest(providerKeys)
	if pipeline == nil {
		writeJSONErrorWithStatus(w, http.StatusServiceUnavailable, core.NewAPIError("Voice pipeline not configured"), requestIDFromContext(r.Context()))
		return
	}

	// Determine operation based on fields
	if req.Audio != "" {
		s.handleTranscribe(w, r, pipeline, &req)
	} else if req.Text != "" {
		s.handleSynthesize(w, r, pipeline, &req)
	} else {
		writeJSONErrorWithStatus(w, http.StatusBadRequest, core.NewInvalidRequestError("Either 'audio' or 'text' field is required"), requestIDFromContext(r.Context()))
	}
}

func (s *Server) handleTranscribe(w http.ResponseWriter, r *http.Request, pipeline *voice.Pipeline, req *AudioRequest) {
	if req.Provider != "" && req.Provider != "cartesia" {
		writeJSONErrorWithStatus(w, http.StatusBadRequest, core.NewInvalidRequestError("Unsupported provider: "+req.Provider), requestIDFromContext(r.Context()))
		return
	}

	audioData, err := base64.StdEncoding.DecodeString(req.Audio)
	if err != nil {
		writeJSONErrorWithStatus(w, http.StatusBadRequest, core.NewInvalidRequestError("Invalid audio data: "+err.Error()), requestIDFromContext(r.Context()))
		return
	}

	provider := pipeline.STTProvider()
	transcript, err := provider.Transcribe(r.Context(), bytes.NewReader(audioData), voiceTranscribeOptions(req))
	if err != nil {
		writeJSONError(w, normalizeCoreError(fmt.Errorf("transcription failed: %w", err)), requestIDFromContext(r.Context()))
		return
	}

	resp := map[string]any{
		"type":             "transcription",
		"text":             transcript.Text,
		"language":         transcript.Language,
		"duration_seconds": transcript.Duration,
	}
	if len(transcript.Words) > 0 {
		words := make([]map[string]any, 0, len(transcript.Words))
		for _, word := range transcript.Words {
			words = append(words, map[string]any{
				"word":  word.Word,
				"start": word.Start,
				"end":   word.End,
			})
		}
		resp["words"] = words
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleSynthesize(w http.ResponseWriter, r *http.Request, pipeline *voice.Pipeline, req *AudioRequest) {
	if req.Provider != "" && req.Provider != "cartesia" {
		writeJSONErrorWithStatus(w, http.StatusBadRequest, core.NewInvalidRequestError("Unsupported provider: "+req.Provider), requestIDFromContext(r.Context()))
		return
	}
	if req.Text == "" {
		writeJSONErrorWithStatus(w, http.StatusBadRequest, core.NewInvalidRequestError("Text is required"), requestIDFromContext(r.Context()))
		return
	}

	provider := pipeline.TTSProvider()

	if req.Stream {
		s.handleSynthesizeStream(w, r, provider, req)
		return
	}

	synth, err := provider.Synthesize(r.Context(), req.Text, voiceSynthesizeOptions(req))
	if err != nil {
		writeJSONError(w, normalizeCoreError(fmt.Errorf("synthesis failed: %w", err)), requestIDFromContext(r.Context()))
		return
	}

	resp := map[string]any{
		"type":             "synthesis",
		"audio":            base64.StdEncoding.EncodeToString(synth.Audio),
		"format":           synth.Format,
		"duration_seconds": synth.Duration,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleSynthesizeStream(w http.ResponseWriter, r *http.Request, provider tts.Provider, req *AudioRequest) {
	stream, err := provider.SynthesizeStream(r.Context(), req.Text, voiceSynthesizeOptions(req))
	if err != nil {
		writeJSONError(w, normalizeCoreError(fmt.Errorf("synthesis failed: %w", err)), requestIDFromContext(r.Context()))
		return
	}
	defer stream.Close()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONErrorWithStatus(w, http.StatusInternalServerError, core.NewAPIError("Streaming not supported"), requestIDFromContext(r.Context()))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	index := 0
	for chunk := range stream.Chunks() {
		payload, err := json.Marshal(map[string]any{
			"data":  base64.StdEncoding.EncodeToString(chunk),
			"index": index,
		})
		if err != nil {
			s.writeSSEError(w, normalizeCoreError(err))
			flusher.Flush()
			return
		}
		writeSSE(w, "audio_chunk", payload)
		flusher.Flush()
		index++
	}

	stream.Close()
	if err := stream.Err(); err != nil {
		s.writeSSEError(w, normalizeCoreError(err))
		flusher.Flush()
		return
	}

	donePayload, _ := json.Marshal(map[string]any{"duration_seconds": 0})
	writeSSE(w, "done", donePayload)
	flusher.Flush()
}

// handleLive handles WebSocket live sessions.
func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	if s.voicePipeline == nil {
		writeJSONErrorWithStatus(w, http.StatusServiceUnavailable, core.NewAPIError("Voice pipeline not configured"), requestIDFromContext(r.Context()))
		return
	}

	if _, ok := s.auth.AuthenticateWebSocket(r); !ok {
		writeJSONErrorWithStatus(w, http.StatusUnauthorized, core.NewAuthenticationError("Invalid API key"), requestIDFromContext(r.Context()))
		return
	}

	if !s.rateLimiter.CheckLiveSessionLimit(int(s.liveSessions.Load())) {
		writeJSONErrorWithStatus(w, http.StatusTooManyRequests, core.NewRateLimitError("Too many live sessions", 0), requestIDFromContext(r.Context()))
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	providerKeys := extractProviderKeys(r, nil)
	if s.config.AuthMode == "passthrough" && len(providerKeys) == 0 {
		writeWSError(conn, core.NewAuthenticationError("Missing provider keys"))
		return
	}

	engine := s.engineForRequest(providerKeys)
	voicePipeline := s.voicePipelineForRequest(providerKeys)
	if voicePipeline == nil {
		writeWSError(conn, core.NewAPIError("Voice pipeline not configured"))
		return
	}

	idleTimeout := s.config.RateLimit.SessionIdleTimeout
	if idleTimeout > 0 {
		conn.SetReadDeadline(time.Now().Add(idleTimeout))
	}

	s.liveSessions.Add(1)
	s.metrics.RecordLiveSessionStart()

	sessionStart := time.Now()
	providerName := ""
	modelName := ""
	defer func() {
		s.liveSessions.Add(-1)
		s.metrics.RecordLiveSessionEnd(providerName, modelName, "success", time.Since(sessionStart))
	}()

	_, raw, err := conn.ReadMessage()
	if err != nil {
		return
	}
	if idleTimeout > 0 {
		conn.SetReadDeadline(time.Now().Add(idleTimeout))
	}

	var configure liveSessionConfigureMessage
	if err := json.Unmarshal(raw, &configure); err != nil || configure.Type != "session.configure" {
		writeWSError(conn, core.NewInvalidRequestError("First message must be session.configure"))
		return
	}

	config := mergeSessionConfig(configure.Config)
	if provider, model, err := core.ParseModelString(config.Model); err == nil {
		providerName = provider
		modelName = model
	}

	llmAdapter := &liveLLMAdapter{engine: engine}
	session := live.NewSession(config, llmAdapter, voicePipeline.TTSProvider(), voicePipeline.STTProvider())
	if err := session.Start(r.Context()); err != nil {
		writeWSError(conn, normalizeCoreError(fmt.Errorf("failed to start session: %w", err)))
		return
	}
	defer session.Close()

	// Forward events to client
	done := make(chan struct{})
	go func() {
		defer close(done)
		for event := range session.Events() {
			payload, err := encodeLiveEvent(event)
			if err != nil {
				continue
			}
			if audioEvent, ok := event.(*live.AudioDeltaEvent); ok {
				s.metrics.RecordLiveAudio("output", len(audioEvent.Data))
			}
			if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		}
	}()

	for {
		msgType, message, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if idleTimeout > 0 {
			conn.SetReadDeadline(time.Now().Add(idleTimeout))
		}

		if msgType == websocket.BinaryMessage {
			s.metrics.RecordLiveAudio("input", len(message))
			_ = session.SendAudio(message)
			continue
		}

		var clientMsg liveClientMessage
		if err := json.Unmarshal(message, &clientMsg); err != nil {
			writeWSError(conn, core.NewInvalidRequestError("Invalid JSON: "+err.Error()))
			continue
		}

		switch clientMsg.Type {
		case "input.commit":
			if err := session.Commit(); err != nil {
				writeWSError(conn, core.NewInvalidRequestError(err.Error()))
			}
		case "input.interrupt":
			if err := session.Interrupt(clientMsg.Transcript); err != nil {
				writeWSError(conn, core.NewInvalidRequestError(err.Error()))
			}
		case "input.text":
			if err := session.SendText(clientMsg.Text); err != nil {
				writeWSError(conn, core.NewInvalidRequestError(err.Error()))
			}
		case "input.content":
			blocks, err := decodeContentBlocks(clientMsg.Content)
			if err != nil {
				writeWSError(conn, core.NewInvalidRequestError(err.Error()))
				continue
			}
			if err := session.SendContent(blocks); err != nil {
				writeWSError(conn, core.NewInvalidRequestError(err.Error()))
			}
		case "tool_result":
			blocks, err := decodeContentBlocks(clientMsg.Content)
			if err != nil {
				writeWSError(conn, core.NewInvalidRequestError(err.Error()))
				continue
			}
			toolBlock := types.ToolResultBlock{
				Type:      "tool_result",
				ToolUseID: clientMsg.ToolUseID,
				Content:   blocks,
				IsError:   clientMsg.IsError,
			}
			if err := session.SendContent([]types.ContentBlock{toolBlock}); err != nil {
				writeWSError(conn, core.NewInvalidRequestError(err.Error()))
			}
		case "session.update":
			writeWSError(conn, core.NewInvalidRequestError("session.update not supported"))
		default:
			writeWSError(conn, core.NewInvalidRequestError("Unknown message type: "+clientMsg.Type))
		}
	}

	<-done
}

type liveSessionConfigureMessage struct {
	Type   string             `json:"type"`
	Config live.SessionConfig `json:"config"`
}

type liveClientMessage struct {
	Type       string            `json:"type"`
	Text       string            `json:"text,omitempty"`
	Transcript string            `json:"transcript,omitempty"`
	Content    []json.RawMessage `json:"content,omitempty"`
	ToolUseID  string            `json:"tool_use_id,omitempty"`
	IsError    bool              `json:"is_error,omitempty"`
}

func mergeSessionConfig(incoming live.SessionConfig) live.SessionConfig {
	cfg := live.DefaultSessionConfig()

	if incoming.Model != "" {
		cfg.Model = incoming.Model
	}
	if incoming.System != "" {
		cfg.System = incoming.System
	}
	if incoming.Tools != nil {
		cfg.Tools = incoming.Tools
	}
	if incoming.Messages != nil {
		cfg.Messages = incoming.Messages
	}
	if incoming.Voice != nil {
		cfg.Voice = incoming.Voice
	}
	if incoming.SampleRate != 0 {
		cfg.SampleRate = incoming.SampleRate
	}
	if incoming.Channels != 0 {
		cfg.Channels = incoming.Channels
	}
	if incoming.MaxTokens != 0 {
		cfg.MaxTokens = incoming.MaxTokens
	}
	if incoming.Temperature != nil {
		cfg.Temperature = incoming.Temperature
	}
	if incoming.VAD != (live.VADConfig{}) {
		cfg.VAD = incoming.VAD
	}
	if incoming.GracePeriod != (live.GracePeriodConfig{}) {
		cfg.GracePeriod = incoming.GracePeriod
	}
	if incoming.Interrupt != (live.InterruptConfig{}) {
		cfg.Interrupt = incoming.Interrupt
	}

	return cfg
}

func encodeLiveEvent(event live.Event) ([]byte, error) {
	payload, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return nil, err
	}
	obj["type"] = event.EventType()
	return json.Marshal(obj)
}

func decodeContentBlocks(raw []json.RawMessage) ([]types.ContentBlock, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("content cannot be empty")
	}
	blocks := make([]types.ContentBlock, 0, len(raw))
	for _, item := range raw {
		block, err := types.UnmarshalContentBlock(item)
		if err != nil {
			return nil, fmt.Errorf("invalid content block: %w", err)
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func encodeStreamEvent(event types.StreamEvent) ([]byte, error) {
	payload, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return nil, err
	}
	obj["type"] = event.EventType()
	return json.Marshal(obj)
}

func writeSSE(w http.ResponseWriter, event string, payload []byte) {
	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", payload)
}

func (s *Server) writeSSEError(w http.ResponseWriter, err *core.Error) {
	event := streamErrorEvent(err)
	payload, _ := json.Marshal(event)
	writeSSE(w, "error", payload)
}

func writeWSError(conn *websocket.Conn, err *core.Error) {
	if err == nil {
		err = core.NewAPIError("unknown error")
	}
	payload, _ := json.Marshal(map[string]any{
		"type":  "error",
		"error": err,
	})
	_ = conn.WriteMessage(websocket.TextMessage, payload)
}

func voiceSynthesizeOptions(req *AudioRequest) tts.SynthesizeOptions {
	return tts.SynthesizeOptions{
		Voice:  req.Voice,
		Speed:  req.Speed,
		Format: req.Format,
	}
}

func voiceTranscribeOptions(req *AudioRequest) stt.TranscribeOptions {
	return stt.TranscribeOptions{
		Model:    req.Model,
		Language: req.Language,
	}
}

func (s *Server) engineForRequest(providerKeys map[string]string) *core.Engine {
	if len(providerKeys) == 0 {
		return s.engine
	}

	var merged map[string]string
	if s.config.AuthMode == "passthrough" {
		merged = copyProviderKeys(providerKeys)
	} else {
		merged = copyProviderKeys(s.config.ProviderKeys)
		for key, value := range providerKeys {
			merged[key] = value
		}
	}

	engine := core.NewEngine(merged)
	registerProviders(engine, merged, s.logger)
	return engine
}

func (s *Server) voicePipelineForRequest(providerKeys map[string]string) *voice.Pipeline {
	if key := providerKeys["cartesia"]; key != "" {
		return voice.NewPipeline(key)
	}
	if s.config.AuthMode == "passthrough" {
		return nil
	}
	return s.voicePipeline
}

func extractProviderKeys(r *http.Request, bodyKeys map[string]string) map[string]string {
	keys := make(map[string]string)
	for name, value := range bodyKeys {
		normalized := normalizeProviderKey(name)
		if normalized != "" && value != "" {
			keys[normalized] = value
		}
	}
	for header, values := range r.Header {
		if !strings.HasPrefix(strings.ToLower(header), "x-provider-key-") {
			continue
		}
		raw := strings.TrimPrefix(header, "X-Provider-Key-")
		raw = strings.TrimPrefix(raw, "x-provider-key-")
		normalized := normalizeProviderKey(raw)
		if normalized == "" || len(values) == 0 {
			continue
		}
		if value := strings.TrimSpace(values[0]); value != "" {
			keys[normalized] = value
		}
	}
	return keys
}

func normalizeProviderKey(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	switch normalized {
	case "google":
		return "gemini"
	default:
		return normalized
	}
}

func copyProviderKeys(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func registerProviders(engine *core.Engine, keys map[string]string, logger *slog.Logger) {
	anthropicKey := keys["anthropic"]
	if anthropicKey == "" {
		anthropicKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if anthropicKey != "" {
		engine.RegisterProvider(newAnthropicAdapter(anthropic.New(anthropicKey)))
	}

	openaiKey := keys["openai"]
	if openaiKey == "" {
		openaiKey = os.Getenv("OPENAI_API_KEY")
	}
	if openaiKey != "" {
		engine.RegisterProvider(newOpenAIAdapter(openai.New(openaiKey)))
		engine.RegisterProvider(newOaiRespAdapter(oai_resp.New(openaiKey)))
	}

	groqKey := keys["groq"]
	if groqKey == "" {
		groqKey = os.Getenv("GROQ_API_KEY")
	}
	if groqKey != "" {
		engine.RegisterProvider(newGroqAdapter(groq.New(groqKey)))
	}

	cerebrasKey := keys["cerebras"]
	if cerebrasKey == "" {
		cerebrasKey = os.Getenv("CEREBRAS_API_KEY")
	}
	if cerebrasKey != "" {
		engine.RegisterProvider(newCerebrasAdapter(cerebras.New(cerebrasKey)))
	}

	var geminiOAuthOpts []gemini_oauth.Option
	if projectID := os.Getenv("GEMINI_OAUTH_PROJECT_ID"); projectID != "" {
		geminiOAuthOpts = append(geminiOAuthOpts, gemini_oauth.WithProjectID(projectID))
	}
	if provider, err := gemini_oauth.New(geminiOAuthOpts...); err == nil {
		engine.RegisterProvider(newGeminiOAuthAdapter(provider))
	} else if logger != nil {
		logger.Debug("gemini-oauth provider not initialized", "error", err)
	}

	geminiKey := keys["gemini"]
	if geminiKey == "" {
		geminiKey = keys["google"]
	}
	if geminiKey == "" {
		geminiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if geminiKey != "" {
		engine.RegisterProvider(newGeminiAdapter(gemini.New(geminiKey)))
	}
}

type liveLLMAdapter struct {
	engine *core.Engine
}

func (a *liveLLMAdapter) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
	return a.engine.CreateMessage(ctx, req)
}

func (a *liveLLMAdapter) StreamMessage(ctx context.Context, req *types.MessageRequest) (live.EventStream, error) {
	stream, err := a.engine.StreamMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	return &liveEventStreamAdapter{stream: stream}, nil
}

type liveEventStreamAdapter struct {
	stream core.EventStream
}

func (a *liveEventStreamAdapter) Next() (types.StreamEvent, error) {
	return a.stream.Next()
}

func (a *liveEventStreamAdapter) Close() error {
	return a.stream.Close()
}

// MessageRequest is the request body for /v1/messages.
type MessageRequest = types.MessageRequest

type proxyMessageRequest struct {
	types.MessageRequest
	ProviderKeys map[string]string `json:"provider_keys,omitempty"`
}

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

	ProviderKeys map[string]string `json:"provider_keys,omitempty"`
}
