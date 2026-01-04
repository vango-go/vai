// Package gemini implements the Google Gemini API provider.
// It translates between the Vango API format (Anthropic-style) and Gemini's format.
package gemini

import (
	"context"
	"net/http"

	"github.com/vango-go/vai/pkg/core/types"
)

const (
	// DefaultBaseURL is the default Gemini API endpoint.
	DefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

	// DefaultMaxTokens is the default max tokens if not specified.
	DefaultMaxTokens = 4096
)

// ProviderCapabilities describes what a provider supports.
// This is a local copy to avoid import cycles.
type ProviderCapabilities struct {
	Vision           bool
	AudioInput       bool
	AudioOutput      bool
	Video            bool
	Tools            bool
	ToolStreaming    bool
	Thinking         bool
	StructuredOutput bool
	NativeTools      []string
}

// EventStream is an iterator over streaming events.
type EventStream interface {
	Next() (types.StreamEvent, error)
	Close() error
}

// Provider implements the Google Gemini API.
type Provider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// New creates a new Gemini provider.
func New(apiKey string, opts ...Option) *Provider {
	p := &Provider{
		apiKey:     apiKey,
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	return "gemini"
}

// Capabilities returns what this provider supports.
func (p *Provider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Vision:           true,  // All Gemini models support vision
		AudioInput:       true,  // Gemini 2.0+ supports audio
		AudioOutput:      false, // Only via Live API
		Video:            true,  // Gemini supports video!
		Tools:            true,
		ToolStreaming:    true,
		Thinking:         true,  // Gemini 2.5+ supports thinking
		StructuredOutput: true,
		NativeTools:      []string{"web_search", "code_execution"},
	}
}

// CreateMessage sends a non-streaming request to Gemini.
func (p *Provider) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
	// Get the model name (strip provider prefix if present)
	model := stripProviderPrefix(req.Model)

	// Build the Gemini request
	geminiReq := p.buildRequest(req)

	// Make HTTP call
	respBody, err := p.doRequest(ctx, model, geminiReq)
	if err != nil {
		return nil, err
	}

	// Parse and translate response
	return p.parseResponse(respBody, model)
}

// StreamMessage sends a streaming request to Gemini.
func (p *Provider) StreamMessage(ctx context.Context, req *types.MessageRequest) (EventStream, error) {
	// Get the model name (strip provider prefix if present)
	model := stripProviderPrefix(req.Model)

	// Build the Gemini request
	geminiReq := p.buildRequest(req)

	// Make HTTP call (returns SSE stream)
	body, err := p.doStreamRequest(ctx, model, geminiReq)
	if err != nil {
		return nil, err
	}

	return newEventStream(body, model), nil
}
