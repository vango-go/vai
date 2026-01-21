package vai

import (
	"context"

	"github.com/vango-go/vai/pkg/core"
	"github.com/vango-go/vai/pkg/core/providers/anthropic"
	"github.com/vango-go/vai/pkg/core/providers/cerebras"
	"github.com/vango-go/vai/pkg/core/providers/gemini"
	"github.com/vango-go/vai/pkg/core/providers/gemini_oauth"
	"github.com/vango-go/vai/pkg/core/providers/groq"
	"github.com/vango-go/vai/pkg/core/providers/oai_resp"
	"github.com/vango-go/vai/pkg/core/providers/openai"
	"github.com/vango-go/vai/pkg/core/types"
)

// anthropicAdapter wraps the anthropic.Provider to implement core.Provider.
type anthropicAdapter struct {
	provider *anthropic.Provider
}

func newAnthropicAdapter(p *anthropic.Provider) *anthropicAdapter {
	return &anthropicAdapter{provider: p}
}

func (a *anthropicAdapter) Name() string {
	return a.provider.Name()
}

func (a *anthropicAdapter) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
	return a.provider.CreateMessage(ctx, req)
}

func (a *anthropicAdapter) StreamMessage(ctx context.Context, req *types.MessageRequest) (core.EventStream, error) {
	stream, err := a.provider.StreamMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	return &eventStreamAdapter{stream: stream}, nil
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

// openaiAdapter wraps the openai.Provider to implement core.Provider.
type openaiAdapter struct {
	provider *openai.Provider
}

func newOpenAIAdapter(p *openai.Provider) *openaiAdapter {
	return &openaiAdapter{provider: p}
}

func (a *openaiAdapter) Name() string {
	return a.provider.Name()
}

func (a *openaiAdapter) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
	return a.provider.CreateMessage(ctx, req)
}

func (a *openaiAdapter) StreamMessage(ctx context.Context, req *types.MessageRequest) (core.EventStream, error) {
	stream, err := a.provider.StreamMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	return &openaiEventStreamAdapter{stream: stream}, nil
}

func (a *openaiAdapter) Capabilities() core.ProviderCapabilities {
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

// oaiRespAdapter wraps the oai_resp.Provider to implement core.Provider.
type oaiRespAdapter struct {
	provider *oai_resp.Provider
}

func newOaiRespAdapter(p *oai_resp.Provider) *oaiRespAdapter {
	return &oaiRespAdapter{provider: p}
}

func (a *oaiRespAdapter) Name() string {
	return a.provider.Name()
}

func (a *oaiRespAdapter) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
	return a.provider.CreateMessage(ctx, req)
}

func (a *oaiRespAdapter) StreamMessage(ctx context.Context, req *types.MessageRequest) (core.EventStream, error) {
	stream, err := a.provider.StreamMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	return &oaiRespEventStreamAdapter{stream: stream}, nil
}

func (a *oaiRespAdapter) Capabilities() core.ProviderCapabilities {
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

// groqAdapter wraps the groq.Provider to implement core.Provider.
type groqAdapter struct {
	provider *groq.Provider
}

func newGroqAdapter(p *groq.Provider) *groqAdapter {
	return &groqAdapter{provider: p}
}

func (a *groqAdapter) Name() string {
	return a.provider.Name()
}

func (a *groqAdapter) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
	return a.provider.CreateMessage(ctx, req)
}

func (a *groqAdapter) StreamMessage(ctx context.Context, req *types.MessageRequest) (core.EventStream, error) {
	stream, err := a.provider.StreamMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	return &groqEventStreamAdapter{stream: stream}, nil
}

func (a *groqAdapter) Capabilities() core.ProviderCapabilities {
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

// cerebrasAdapter wraps the cerebras.Provider to implement core.Provider.
type cerebrasAdapter struct {
	provider *cerebras.Provider
}

func newCerebrasAdapter(p *cerebras.Provider) *cerebrasAdapter {
	return &cerebrasAdapter{provider: p}
}

func (a *cerebrasAdapter) Name() string {
	return a.provider.Name()
}

func (a *cerebrasAdapter) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
	return a.provider.CreateMessage(ctx, req)
}

func (a *cerebrasAdapter) StreamMessage(ctx context.Context, req *types.MessageRequest) (core.EventStream, error) {
	stream, err := a.provider.StreamMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	return &cerebrasEventStreamAdapter{stream: stream}, nil
}

func (a *cerebrasAdapter) Capabilities() core.ProviderCapabilities {
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

// geminiAdapter wraps the gemini.Provider to implement core.Provider.
type geminiAdapter struct {
	provider *gemini.Provider
}

func newGeminiAdapter(p *gemini.Provider) *geminiAdapter {
	return &geminiAdapter{provider: p}
}

func (a *geminiAdapter) Name() string {
	return a.provider.Name()
}

func (a *geminiAdapter) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
	return a.provider.CreateMessage(ctx, req)
}

func (a *geminiAdapter) StreamMessage(ctx context.Context, req *types.MessageRequest) (core.EventStream, error) {
	stream, err := a.provider.StreamMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	return &geminiEventStreamAdapter{stream: stream}, nil
}

func (a *geminiAdapter) Capabilities() core.ProviderCapabilities {
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

// eventStreamAdapter wraps anthropic.EventStream to implement core.EventStream.
type eventStreamAdapter struct {
	stream anthropic.EventStream
}

func (a *eventStreamAdapter) Next() (types.StreamEvent, error) {
	return a.stream.Next()
}

func (a *eventStreamAdapter) Close() error {
	return a.stream.Close()
}

// openaiEventStreamAdapter wraps openai.EventStream to implement core.EventStream.
type openaiEventStreamAdapter struct {
	stream openai.EventStream
}

func (a *openaiEventStreamAdapter) Next() (types.StreamEvent, error) {
	return a.stream.Next()
}

func (a *openaiEventStreamAdapter) Close() error {
	return a.stream.Close()
}

// groqEventStreamAdapter wraps groq.EventStream to implement core.EventStream.
type groqEventStreamAdapter struct {
	stream groq.EventStream
}

func (a *groqEventStreamAdapter) Next() (types.StreamEvent, error) {
	return a.stream.Next()
}

func (a *groqEventStreamAdapter) Close() error {
	return a.stream.Close()
}

// cerebrasEventStreamAdapter wraps cerebras.EventStream to implement core.EventStream.
type cerebrasEventStreamAdapter struct {
	stream cerebras.EventStream
}

func (a *cerebrasEventStreamAdapter) Next() (types.StreamEvent, error) {
	return a.stream.Next()
}

func (a *cerebrasEventStreamAdapter) Close() error {
	return a.stream.Close()
}

// geminiEventStreamAdapter wraps gemini.EventStream to implement core.EventStream.
type geminiEventStreamAdapter struct {
	stream gemini.EventStream
}

func (a *geminiEventStreamAdapter) Next() (types.StreamEvent, error) {
	return a.stream.Next()
}

func (a *geminiEventStreamAdapter) Close() error {
	return a.stream.Close()
}

// oaiRespEventStreamAdapter wraps oai_resp.EventStream to implement core.EventStream.
type oaiRespEventStreamAdapter struct {
	stream oai_resp.EventStream
}

func (a *oaiRespEventStreamAdapter) Next() (types.StreamEvent, error) {
	return a.stream.Next()
}

func (a *oaiRespEventStreamAdapter) Close() error {
	return a.stream.Close()
}

// geminiOAuthAdapter wraps the gemini_oauth.Provider to implement core.Provider.
type geminiOAuthAdapter struct {
	provider *gemini_oauth.Provider
}

func newGeminiOAuthAdapter(p *gemini_oauth.Provider) *geminiOAuthAdapter {
	return &geminiOAuthAdapter{provider: p}
}

func (a *geminiOAuthAdapter) Name() string {
	return a.provider.Name()
}

func (a *geminiOAuthAdapter) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
	return a.provider.CreateMessage(ctx, req)
}

func (a *geminiOAuthAdapter) StreamMessage(ctx context.Context, req *types.MessageRequest) (core.EventStream, error) {
	stream, err := a.provider.StreamMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	return &geminiOAuthEventStreamAdapter{stream: stream}, nil
}

func (a *geminiOAuthAdapter) Capabilities() core.ProviderCapabilities {
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

// geminiOAuthEventStreamAdapter wraps gemini_oauth.EventStream to implement core.EventStream.
type geminiOAuthEventStreamAdapter struct {
	stream gemini_oauth.EventStream
}

func (a *geminiOAuthEventStreamAdapter) Next() (types.StreamEvent, error) {
	return a.stream.Next()
}

func (a *geminiOAuthEventStreamAdapter) Close() error {
	return a.stream.Close()
}
