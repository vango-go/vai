// Package vai provides the Vai SDK for Go.
//
// The SDK operates in two modes:
//
// # Direct Mode (Default)
//
// Runs the translation engine directly in your process. No external proxy required.
// Provider API keys come from environment variables.
//
//	client := vai.NewClient()
//
// # Proxy Mode
//
// Connects to a Vango AI Proxy instance via HTTP. Ideal for production environments
// requiring centralized governance, observability, and secret management.
//
//	client := vai.NewClient(
//	    vai.WithBaseURL("http://vango-proxy.internal:8080"),
//	    vai.WithAPIKey("vango_sk_..."),
//	)
package vai

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/vango-go/vai/pkg/core"
	"github.com/vango-go/vai/pkg/core/providers/anthropic"
	"github.com/vango-go/vai/pkg/core/providers/cerebras"
	"github.com/vango-go/vai/pkg/core/providers/gemini"
	"github.com/vango-go/vai/pkg/core/providers/gemini_oauth"
	"github.com/vango-go/vai/pkg/core/providers/groq"
	"github.com/vango-go/vai/pkg/core/providers/oai_resp"
	"github.com/vango-go/vai/pkg/core/providers/openai"
	"github.com/vango-go/vai/pkg/core/voice"
	"github.com/vango-go/vai/pkg/core/voice/stt"
	"github.com/vango-go/vai/pkg/core/voice/tts"
)

type clientMode int

const (
	modeDirect clientMode = iota
	modeProxy
)

// Client is the main entry point for the Vango AI SDK.
type Client struct {
	Messages *MessagesService
	Audio    *AudioService
	Models   *ModelsService

	// Internal
	mode       clientMode
	baseURL    string
	apiKey     string
	httpClient *http.Client
	logger     *slog.Logger
	tracer     trace.Tracer

	// Direct mode only
	core          *core.Engine
	providerKeys  map[string]string
	voicePipeline *voice.Pipeline

	// Retry configuration
	maxRetries   int
	retryBackoff time.Duration
}

// NewClient creates a new Vango AI client.
// Default is Direct Mode. Provide WithBaseURL() for Proxy Mode.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		httpClient:   &http.Client{Timeout: 60 * time.Second},
		logger:       slog.Default(),
		providerKeys: make(map[string]string),
		maxRetries:   0,
		retryBackoff: time.Second,
	}

	for _, opt := range opts {
		opt(c)
	}

	// Determine mode
	if c.baseURL != "" {
		c.mode = modeProxy
	} else {
		c.mode = modeDirect
		c.core = core.NewEngine(c.providerKeys)
		c.initProviders()
		c.initVoicePipeline()
	}

	// Initialize services
	c.Messages = &MessagesService{client: c}
	c.Audio = &AudioService{client: c}
	c.Models = &ModelsService{client: c}

	return c
}

// initProviders registers all available providers with the engine.
func (c *Client) initProviders() {
	// Register Anthropic if API key is available
	anthropicKey := c.core.GetAPIKey("anthropic")
	if anthropicKey != "" {
		c.core.RegisterProvider(newAnthropicAdapter(anthropic.New(anthropicKey)))
	}

	// Register OpenAI Chat Completions if API key is available
	openaiKey := c.core.GetAPIKey("openai")
	if openaiKey != "" {
		c.core.RegisterProvider(newOpenAIAdapter(openai.New(openaiKey)))
	}

	// Register OpenAI Responses API (oai-resp) if API key is available
	// Uses the same API key as OpenAI but with different endpoint
	if openaiKey != "" {
		c.core.RegisterProvider(newOaiRespAdapter(oai_resp.New(openaiKey)))
	}

	// Register Groq if API key is available
	groqKey := c.core.GetAPIKey("groq")
	if groqKey != "" {
		c.core.RegisterProvider(newGroqAdapter(groq.New(groqKey)))
	}

	// Register Cerebras if API key is available
	cerebrasKey := c.core.GetAPIKey("cerebras")
	if cerebrasKey != "" {
		c.core.RegisterProvider(newCerebrasAdapter(cerebras.New(cerebrasKey)))
	}

	// Register Gemini OAuth if credentials are available
	// Credentials are stored at ~/.config/vango/gemini-oauth-credentials.json
	// Project ID can be overridden via GEMINI_OAUTH_PROJECT_ID env var
	var geminiOAuthOpts []gemini_oauth.Option
	if projectID := os.Getenv("GEMINI_OAUTH_PROJECT_ID"); projectID != "" {
		geminiOAuthOpts = append(geminiOAuthOpts, gemini_oauth.WithProjectID(projectID))
	}
	if provider, err := gemini_oauth.New(geminiOAuthOpts...); err == nil {
		c.core.RegisterProvider(newGeminiOAuthAdapter(provider))
	} else {
		c.logger.Debug("gemini-oauth provider not initialized", "error", err)
	}

	// Register Gemini API key provider if available
	geminiKey := c.core.GetAPIKey("gemini")
	if geminiKey == "" {
		geminiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if geminiKey != "" {
		c.core.RegisterProvider(newGeminiAdapter(gemini.New(geminiKey)))
	}
}

// initVoicePipeline initializes the voice pipeline if Cartesia API key is available.
func (c *Client) initVoicePipeline() {
	cartesiaKey := c.getCartesiaAPIKey()
	if cartesiaKey != "" {
		c.voicePipeline = voice.NewPipeline(cartesiaKey)
	}
}

// getCartesiaAPIKey returns the Cartesia API key from provider keys or environment.
func (c *Client) getCartesiaAPIKey() string {
	if key, ok := c.providerKeys["cartesia"]; ok && key != "" {
		return key
	}
	return os.Getenv("CARTESIA_API_KEY")
}

// getSTTProvider returns the STT provider (Cartesia).
func (c *Client) getSTTProvider() stt.Provider {
	if c.voicePipeline != nil {
		return c.voicePipeline.STTProvider()
	}
	// Create a standalone provider
	cartesiaKey := c.getCartesiaAPIKey()
	if cartesiaKey == "" {
		return nil
	}
	return stt.NewCartesia(cartesiaKey)
}

// getTTSProvider returns the TTS provider (Cartesia).
func (c *Client) getTTSProvider() tts.Provider {
	if c.voicePipeline != nil {
		return c.voicePipeline.TTSProvider()
	}
	// Create a standalone provider
	cartesiaKey := c.getCartesiaAPIKey()
	if cartesiaKey == "" {
		return nil
	}
	return tts.NewCartesia(cartesiaKey)
}

// VoicePipeline returns the voice pipeline (only available in Direct Mode with Cartesia key).
func (c *Client) VoicePipeline() *voice.Pipeline {
	return c.voicePipeline
}

// IsDirectMode returns true if the client is operating in Direct Mode.
func (c *Client) IsDirectMode() bool {
	return c.mode == modeDirect
}

// IsProxyMode returns true if the client is operating in Proxy Mode.
func (c *Client) IsProxyMode() bool {
	return c.mode == modeProxy
}

// Engine returns the core engine (only available in Direct Mode).
func (c *Client) Engine() *core.Engine {
	return c.core
}

// Live creates a new live voice conversation session.
// This enables real-time bidirectional voice conversations with automatic
// turn detection, interruption handling, and text-to-speech.
//
// Deprecated: Use client.Messages.RunStream(ctx, req, WithLive(&LiveConfig{})) instead.
// This method will be removed in a future version.
//
// Direct Mode only. Requires CARTESIA_API_KEY for STT/TTS.
func (c *Client) Live(ctx context.Context, config LiveConfig) (*LiveSession, error) {
	if c.mode != modeDirect {
		return nil, fmt.Errorf("live sessions only supported in direct mode")
	}

	// Get STT provider
	sttProvider := c.getSTTProvider()
	if sttProvider == nil {
		return nil, fmt.Errorf("STT provider not available - ensure CARTESIA_API_KEY is set")
	}

	// Get TTS provider
	ttsProvider := c.getTTSProvider()
	if ttsProvider == nil {
		return nil, fmt.Errorf("TTS provider not available - ensure CARTESIA_API_KEY is set")
	}

	// Create adapters
	llmAdapter := &llmClientAdapter{client: c}
	ttsAdapter := &ttsClientAdapter{provider: ttsProvider}
	sttAdapter := &sttClientAdapter{provider: sttProvider}

	// Create and start session
	session := newLiveSession(config, llmAdapter, ttsAdapter, sttAdapter)
	if err := session.Start(ctx); err != nil {
		return nil, fmt.Errorf("start live session: %w", err)
	}

	return session, nil
}
