// Package cerebras implements the Cerebras API provider.
// Cerebras uses an OpenAI-compatible API, so this provider wraps the OpenAI provider
// with a different base URL and adjusted capabilities.
package cerebras

import (
	"context"
	"net/http"

	"github.com/vango-go/vai/pkg/core/providers/openai"
	"github.com/vango-go/vai/pkg/core/types"
)

const (
	// DefaultBaseURL is the Cerebras API endpoint.
	DefaultBaseURL = "https://api.cerebras.ai/v1"
)

// ProviderCapabilities describes what a provider supports.
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

// Provider implements the Cerebras API using OpenAI-compatible interface.
type Provider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	inner      *openai.Provider
}

// New creates a new Cerebras provider.
func New(apiKey string, opts ...Option) *Provider {
	p := &Provider{
		apiKey:     apiKey,
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{},
	}
	for _, opt := range opts {
		opt(p)
	}

	// Create inner OpenAI provider with configured settings
	p.inner = openai.New(apiKey,
		openai.WithBaseURL(p.baseURL),
		openai.WithHTTPClient(p.httpClient),
	)

	return p
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	return "cerebras"
}

// Capabilities returns what this provider supports.
func (p *Provider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Vision:           false, // Cerebras focuses on LLM inference
		AudioInput:       false,
		AudioOutput:      false,
		Video:            false,
		Tools:            true,
		ToolStreaming:    true,
		Thinking:         false,
		StructuredOutput: true,
		NativeTools:      []string{}, // No native tools
	}
}

// CreateMessage sends a non-streaming request to Cerebras.
func (p *Provider) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
	// Use the inner OpenAI provider
	resp, err := p.inner.CreateMessage(ctx, req)
	if err != nil {
		return nil, err
	}

	// Update model prefix from openai/ to cerebras/
	if resp != nil {
		resp.Model = "cerebras/" + stripOpenAIPrefix(resp.Model)
	}

	return resp, nil
}

// StreamMessage sends a streaming request to Cerebras.
func (p *Provider) StreamMessage(ctx context.Context, req *types.MessageRequest) (EventStream, error) {
	// Use the inner OpenAI provider
	stream, err := p.inner.StreamMessage(ctx, req)
	if err != nil {
		return nil, err
	}

	return &cerebrasEventStream{inner: stream}, nil
}

// cerebrasEventStream wraps the OpenAI event stream to fix model names.
type cerebrasEventStream struct {
	inner openai.EventStream
}

// Next returns the next event from the stream.
func (s *cerebrasEventStream) Next() (types.StreamEvent, error) {
	event, err := s.inner.Next()
	if err != nil {
		return nil, err
	}

	// Fix model name in message_start event
	if mse, ok := event.(types.MessageStartEvent); ok {
		mse.Message.Model = "cerebras/" + stripOpenAIPrefix(mse.Message.Model)
		return mse, nil
	}

	return event, nil
}

// Close releases resources associated with the stream.
func (s *cerebrasEventStream) Close() error {
	return s.inner.Close()
}

// stripOpenAIPrefix removes the openai/ prefix from a model string.
func stripOpenAIPrefix(model string) string {
	if len(model) > 7 && model[:7] == "openai/" {
		return model[7:]
	}
	return model
}
