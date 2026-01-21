// Package openai implements the OpenAI Chat Completions API provider.
// It translates between the Vango API format (Anthropic-style) and OpenAI's format.
package openai

import (
	"context"
	"net/http"

	"github.com/vango-go/vai/pkg/core/types"
)

const (
	// DefaultBaseURL is the default OpenAI API endpoint.
	DefaultBaseURL = "https://api.openai.com/v1"

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

// Provider implements the OpenAI Chat Completions API.
type Provider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// New creates a new OpenAI provider.
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
	return "openai"
}

// Capabilities returns what this provider supports.
func (p *Provider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Vision:           true,
		AudioInput:       true,  // GPT-4o supports audio
		AudioOutput:      true,  // GPT-4o supports audio
		Video:            false,
		Tools:            true,
		ToolStreaming:    true,
		Thinking:         false, // o1 has reasoning but different format
		StructuredOutput: true,
		NativeTools:      []string{"web_search", "code_interpreter", "file_search"},
	}
}

// CreateMessage sends a non-streaming request to OpenAI.
func (p *Provider) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
	// Build the OpenAI request
	openaiReq := p.buildRequest(req)

	// Make HTTP call
	respBody, err := p.doRequest(ctx, openaiReq)
	if err != nil {
		return nil, err
	}

	// Parse and translate response
	return p.parseResponse(respBody)
}

// StreamMessage sends a streaming request to OpenAI.
func (p *Provider) StreamMessage(ctx context.Context, req *types.MessageRequest) (EventStream, error) {
	// Build request with stream=true
	openaiReq := p.buildRequest(req)
	openaiReq.Stream = true
	openaiReq.StreamOptions = &streamOptions{IncludeUsage: true}

	// Make HTTP call (returns SSE stream)
	body, err := p.doStreamRequest(ctx, openaiReq)
	if err != nil {
		return nil, err
	}

	return newEventStream(body), nil
}
