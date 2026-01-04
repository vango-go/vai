# Phase 1: Foundation & Core Types

**Status:** ✅ Complete
**Priority:** Critical Path
**Dependencies:** None
**Completed:** December 2025

---

## Overview

Phase 1 establishes the type system, package structure, and foundational interfaces that all subsequent phases build upon. This phase produces no runnable functionality but creates the scaffolding for everything that follows.

---

## Goals

1. Define all core types matching the API spec (Anthropic Messages API format)
2. Establish package structure for SDK and pkg/core
3. Create provider interface abstraction
4. Set up error types and handling patterns
5. Implement client skeleton with option pattern

---

## Deliverables

### 1.1 Package Structure

Create the monorepo layout:

```
vai/vango/
├── pkg/
│   └── core/
│       ├── types/           # All Vango AI API types
│       │   ├── request.go
│       │   ├── response.go
│       │   ├── content.go
│       │   ├── message.go
│       │   ├── tool.go
│       │   ├── usage.go
│       │   └── voice.go
│       ├── provider.go      # Provider interface
│       └── errors.go        # Core error types
│
├── sdk/
│   ├── client.go           # Client struct, NewClient()
│   ├── options.go          # ClientOption functions
│   ├── errors.go           # SDK-level errors
│   └── internal/
│       ├── direct/         # Direct mode implementation
│       │   └── executor.go
│       └── proxy/          # Proxy mode HTTP client
│           └── client.go
│
└── go.mod
```

### 1.2 Core Types (pkg/core/types/)

#### request.go
```go
package types

// MessageRequest is the primary request structure for the Messages API.
// Based on Anthropic Messages API with extensions for multi-provider support.
type MessageRequest struct {
    Model    string    `json:"model"`              // "provider/model-name"
    Messages []Message `json:"messages"`

    // Generation parameters
    MaxTokens     int      `json:"max_tokens,omitempty"`
    System        any      `json:"system,omitempty"`        // string or []ContentBlock
    Temperature   *float64 `json:"temperature,omitempty"`
    TopP          *float64 `json:"top_p,omitempty"`
    TopK          *int     `json:"top_k,omitempty"`
    StopSequences []string `json:"stop_sequences,omitempty"`

    // Tools
    Tools      []Tool      `json:"tools,omitempty"`
    ToolChoice *ToolChoice `json:"tool_choice,omitempty"`

    // Streaming
    Stream bool `json:"stream,omitempty"`

    // Output configuration
    OutputFormat *OutputFormat `json:"output_format,omitempty"`
    Output       *OutputConfig `json:"output,omitempty"`

    // Voice pipeline
    Voice *VoiceConfig `json:"voice,omitempty"`

    // Provider-specific extensions
    Extensions map[string]any `json:"extensions,omitempty"`

    // User metadata (passthrough)
    Metadata map[string]any `json:"metadata,omitempty"`
}

type OutputFormat struct {
    Type       string     `json:"type"`                  // "json_schema"
    JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

type OutputConfig struct {
    Modalities []string     `json:"modalities,omitempty"` // ["text", "image", "audio"]
    Image      *ImageConfig `json:"image,omitempty"`
}

type ImageConfig struct {
    Size string `json:"size,omitempty"` // "1024x1024"
}

type JSONSchema struct {
    Type        string                `json:"type"`
    Properties  map[string]JSONSchema `json:"properties,omitempty"`
    Required    []string              `json:"required,omitempty"`
    Description string                `json:"description,omitempty"`
    Enum        []string              `json:"enum,omitempty"`
    Items       *JSONSchema           `json:"items,omitempty"`
}
```

#### response.go
```go
package types

// MessageResponse is the response from the Messages API.
type MessageResponse struct {
    ID           string         `json:"id"`
    Type         string         `json:"type"`          // "message"
    Role         string         `json:"role"`          // "assistant"
    Model        string         `json:"model"`
    Content      []ContentBlock `json:"content"`
    StopReason   StopReason     `json:"stop_reason"`
    StopSequence *string        `json:"stop_sequence,omitempty"`
    Usage        Usage          `json:"usage"`
}

type StopReason string

const (
    StopReasonEndTurn      StopReason = "end_turn"
    StopReasonMaxTokens    StopReason = "max_tokens"
    StopReasonStopSequence StopReason = "stop_sequence"
    StopReasonToolUse      StopReason = "tool_use"
)

// Convenience methods
func (r *MessageResponse) TextContent() string
func (r *MessageResponse) ToolUses() []ToolUseBlock
func (r *MessageResponse) HasToolUse() bool
func (r *MessageResponse) ThinkingContent() string
func (r *MessageResponse) ImageContent() *ImageBlock
func (r *MessageResponse) AudioContent() *AudioBlock
```

#### content.go
```go
package types

// ContentBlock is the interface for all content types.
// INPUT:  text, image, audio, video, document, tool_result
// OUTPUT: text, tool_use, thinking, image, audio
type ContentBlock interface {
    blockType() string
}

// --- Input Blocks ---

type TextBlock struct {
    Type string `json:"type"` // "text"
    Text string `json:"text"`
}

type ImageBlock struct {
    Type   string      `json:"type"` // "image"
    Source ImageSource `json:"source"`
}

type ImageSource struct {
    Type      string `json:"type"`                 // "base64" or "url"
    MediaType string `json:"media_type,omitempty"` // "image/png", etc.
    Data      string `json:"data,omitempty"`       // base64 data
    URL       string `json:"url,omitempty"`        // URL reference
}

type AudioBlock struct {
    Type       string      `json:"type"` // "audio"
    Source     AudioSource `json:"source"`
    Transcript *string     `json:"transcript,omitempty"` // For output audio
}

type AudioSource struct {
    Type      string `json:"type"`       // "base64"
    MediaType string `json:"media_type"` // "audio/wav", "audio/mp3", etc.
    Data      string `json:"data"`       // base64 data
}

type VideoBlock struct {
    Type   string      `json:"type"` // "video"
    Source VideoSource `json:"source"`
}

type VideoSource struct {
    Type      string `json:"type"`       // "base64"
    MediaType string `json:"media_type"` // "video/mp4"
    Data      string `json:"data"`
}

type DocumentBlock struct {
    Type     string         `json:"type"` // "document"
    Source   DocumentSource `json:"source"`
    Filename string         `json:"filename,omitempty"`
}

type DocumentSource struct {
    Type      string `json:"type"`       // "base64"
    MediaType string `json:"media_type"` // "application/pdf"
    Data      string `json:"data"`
}

type ToolResultBlock struct {
    Type      string         `json:"type"` // "tool_result"
    ToolUseID string         `json:"tool_use_id"`
    Content   []ContentBlock `json:"content"`
    IsError   bool           `json:"is_error,omitempty"`
}

// --- Output Blocks ---

type ToolUseBlock struct {
    Type  string         `json:"type"` // "tool_use"
    ID    string         `json:"id"`
    Name  string         `json:"name"`
    Input map[string]any `json:"input"`
}

type ThinkingBlock struct {
    Type     string  `json:"type"` // "thinking"
    Thinking string  `json:"thinking"`
    Summary  *string `json:"summary,omitempty"`
}

// Implement blockType() for all types
func (t TextBlock) blockType() string       { return "text" }
func (t ImageBlock) blockType() string      { return "image" }
func (t AudioBlock) blockType() string      { return "audio" }
func (t VideoBlock) blockType() string      { return "video" }
func (t DocumentBlock) blockType() string   { return "document" }
func (t ToolResultBlock) blockType() string { return "tool_result" }
func (t ToolUseBlock) blockType() string    { return "tool_use" }
func (t ThinkingBlock) blockType() string   { return "thinking" }
```

#### message.go
```go
package types

type Message struct {
    Role    string `json:"role"`    // "user" or "assistant"
    Content any    `json:"content"` // string or []ContentBlock
}

// MarshalJSON handles the flexible Content field.
// - string -> "string"
// - ContentBlock -> [ContentBlock]
// - []ContentBlock -> [ContentBlock...]
func (m Message) MarshalJSON() ([]byte, error)

// UnmarshalJSON handles flexible Content parsing.
func (m *Message) UnmarshalJSON(data []byte) error

// ContentBlocks returns Content as []ContentBlock regardless of input type.
func (m *Message) ContentBlocks() []ContentBlock
```

#### tool.go
```go
package types

type Tool struct {
    Type        string      `json:"type"` // "function", "web_search", "code_execution", etc.
    Name        string      `json:"name,omitempty"`
    Description string      `json:"description,omitempty"`
    InputSchema *JSONSchema `json:"input_schema,omitempty"`
    Config      any         `json:"config,omitempty"`
}

type ToolChoice struct {
    Type string `json:"type"`           // "auto", "any", "none", "tool"
    Name string `json:"name,omitempty"` // Required when type="tool"
}

// Native tool types
const (
    ToolTypeFunction      = "function"
    ToolTypeWebSearch     = "web_search"
    ToolTypeCodeExecution = "code_execution"
    ToolTypeFileSearch    = "file_search"
    ToolTypeComputerUse   = "computer_use"
    ToolTypeTextEditor    = "text_editor"
)

type WebSearchConfig struct {
    MaxResults  int    `json:"max_results,omitempty"`
    SearchDepth string `json:"search_depth,omitempty"` // "basic" or "advanced"
}

type CodeExecutionConfig struct {
    Languages      []string `json:"languages,omitempty"`
    TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}

type ComputerUseConfig struct {
    DisplayWidth  int `json:"display_width"`
    DisplayHeight int `json:"display_height"`
}
```

#### usage.go
```go
package types

type Usage struct {
    InputTokens      int      `json:"input_tokens"`
    OutputTokens     int      `json:"output_tokens"`
    TotalTokens      int      `json:"total_tokens"`
    CacheReadTokens  *int     `json:"cache_read_tokens,omitempty"`
    CacheWriteTokens *int     `json:"cache_write_tokens,omitempty"`
    CostUSD          *float64 `json:"cost_usd,omitempty"`
}

// Add combines two Usage objects (for aggregation).
func (u Usage) Add(other Usage) Usage
```

#### voice.go
```go
package types

type VoiceConfig struct {
    Input  *VoiceInputConfig  `json:"input,omitempty"`
    Output *VoiceOutputConfig `json:"output,omitempty"`
}

type VoiceInputConfig struct {
    Provider string `json:"provider"` // "deepgram", "openai", "assemblyai"
    Model    string `json:"model,omitempty"`
    Language string `json:"language,omitempty"`
}

type VoiceOutputConfig struct {
    Provider string  `json:"provider"` // "elevenlabs", "openai", "cartesia"
    Voice    string  `json:"voice"`
    Speed    float64 `json:"speed,omitempty"`
    Format   string  `json:"format,omitempty"` // "mp3", "wav", "pcm"
}
```

### 1.3 Provider Interface (pkg/core/provider.go)

```go
package core

import (
    "context"
    "github.com/vango-go/vai/pkg/core/types"
)

// Provider is the interface that all LLM providers must implement.
type Provider interface {
    // Name returns the provider identifier (e.g., "anthropic", "openai").
    Name() string

    // CreateMessage sends a non-streaming request.
    CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error)

    // StreamMessage sends a streaming request.
    StreamMessage(ctx context.Context, req *types.MessageRequest) (EventStream, error)

    // Capabilities returns what this provider supports.
    Capabilities() ProviderCapabilities
}

// EventStream is an iterator over streaming events.
type EventStream interface {
    // Next returns the next event. Returns nil, io.EOF when done.
    Next() (StreamEvent, error)

    // Close releases resources.
    Close() error
}

// StreamEvent is the interface for all streaming event types.
type StreamEvent interface {
    eventType() string
}

// ProviderCapabilities describes what a provider supports.
type ProviderCapabilities struct {
    Vision          bool
    AudioInput      bool
    AudioOutput     bool
    Video           bool
    Tools           bool
    ToolStreaming   bool
    Thinking        bool
    StructuredOutput bool
    NativeTools     []string // "web_search", "code_execution", etc.
}
```

### 1.4 Streaming Events (pkg/core/types/stream.go)

```go
package types

// --- Stream Events (matching API spec section 9) ---

type MessageStartEvent struct {
    Type    string          `json:"type"` // "message_start"
    Message MessageResponse `json:"message"`
}

type ContentBlockStartEvent struct {
    Type         string       `json:"type"` // "content_block_start"
    Index        int          `json:"index"`
    ContentBlock ContentBlock `json:"content_block"`
}

type ContentBlockDeltaEvent struct {
    Type  string `json:"type"` // "content_block_delta"
    Index int    `json:"index"`
    Delta Delta  `json:"delta"`
}

// Delta types
type Delta interface {
    deltaType() string
}

type TextDelta struct {
    Type string `json:"type"` // "text_delta"
    Text string `json:"text"`
}

type InputJSONDelta struct {
    Type        string `json:"type"` // "input_json_delta"
    PartialJSON string `json:"partial_json"`
}

type ThinkingDelta struct {
    Type     string `json:"type"` // "thinking_delta"
    Thinking string `json:"thinking"`
}

type ContentBlockStopEvent struct {
    Type  string `json:"type"` // "content_block_stop"
    Index int    `json:"index"`
}

type MessageDeltaEvent struct {
    Type  string `json:"type"` // "message_delta"
    Delta struct {
        StopReason StopReason `json:"stop_reason,omitempty"`
    } `json:"delta"`
    Usage Usage `json:"usage"`
}

type MessageStopEvent struct {
    Type string `json:"type"` // "message_stop"
}

// Voice-specific streaming events
type AudioDeltaEvent struct {
    Type  string `json:"type"` // "audio_delta"
    Delta struct {
        Data   string `json:"data"`   // base64
        Format string `json:"format"` // "mp3", "wav"
    } `json:"delta"`
}

type TranscriptDeltaEvent struct {
    Type  string `json:"type"` // "transcript_delta"
    Role  string `json:"role"` // "user" or "assistant"
    Delta struct {
        Text string `json:"text"`
    } `json:"delta"`
}

type ErrorEvent struct {
    Type  string `json:"type"` // "error"
    Error Error  `json:"error"`
}

// Implement interfaces
func (e MessageStartEvent) eventType() string       { return "message_start" }
func (e ContentBlockStartEvent) eventType() string  { return "content_block_start" }
func (e ContentBlockDeltaEvent) eventType() string  { return "content_block_delta" }
func (e ContentBlockStopEvent) eventType() string   { return "content_block_stop" }
func (e MessageDeltaEvent) eventType() string       { return "message_delta" }
func (e MessageStopEvent) eventType() string        { return "message_stop" }
func (e AudioDeltaEvent) eventType() string         { return "audio_delta" }
func (e TranscriptDeltaEvent) eventType() string    { return "transcript_delta" }
func (e ErrorEvent) eventType() string              { return "error" }

func (d TextDelta) deltaType() string      { return "text_delta" }
func (d InputJSONDelta) deltaType() string { return "input_json_delta" }
func (d ThinkingDelta) deltaType() string  { return "thinking_delta" }
```

### 1.5 Error Types (pkg/core/errors.go)

```go
package core

import "fmt"

// Error types matching API spec section 15
type Error struct {
    Type          ErrorType      `json:"type"`
    Message       string         `json:"message"`
    Param         string         `json:"param,omitempty"`
    Code          string         `json:"code,omitempty"`
    RequestID     string         `json:"request_id,omitempty"`
    ProviderError any            `json:"provider_error,omitempty"`
    RetryAfter    *int           `json:"retry_after,omitempty"`
}

func (e *Error) Error() string {
    return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

type ErrorType string

const (
    ErrInvalidRequest ErrorType = "invalid_request_error"
    ErrAuthentication ErrorType = "authentication_error"
    ErrPermission     ErrorType = "permission_error"
    ErrNotFound       ErrorType = "not_found_error"
    ErrRateLimit      ErrorType = "rate_limit_error"
    ErrAPI            ErrorType = "api_error"
    ErrOverloaded     ErrorType = "overloaded_error"
    ErrProvider       ErrorType = "provider_error"
)

// Error constructors
func NewInvalidRequestError(message string) *Error
func NewAuthenticationError(message string) *Error
func NewRateLimitError(message string, retryAfter int) *Error
func NewProviderError(provider string, underlying error) *Error
```

### 1.6 SDK Client Skeleton (sdk/client.go)

```go
package vango

import (
    "log/slog"
    "net/http"
    "time"

    "go.opentelemetry.io/otel/trace"
    "github.com/vango-go/vai/pkg/core"
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
    core         *core.Engine
    providerKeys map[string]string

    // Retry configuration
    maxRetries   int
    retryBackoff time.Duration
}

// NewClient creates a new Vango AI client.
// Default is Direct Mode. Provide WithBaseURL() for Proxy Mode.
func NewClient(opts ...ClientOption) *Client {
    c := &Client{
        httpClient:   http.DefaultClient,
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
    }

    // Initialize services
    c.Messages = &MessagesService{client: c}
    c.Audio = &AudioService{client: c}
    c.Models = &ModelsService{client: c}

    return c
}
```

### 1.7 Client Options (sdk/options.go)

```go
package vango

import (
    "log/slog"
    "net/http"
    "time"

    "go.opentelemetry.io/otel/trace"
)

type ClientOption func(*Client)

// Mode selection
func WithBaseURL(url string) ClientOption {
    return func(c *Client) { c.baseURL = url }
}

func WithAPIKey(key string) ClientOption {
    return func(c *Client) { c.apiKey = key }
}

// Direct Mode provider keys
func WithProviderKey(provider, key string) ClientOption {
    return func(c *Client) { c.providerKeys[provider] = key }
}

// HTTP configuration
func WithHTTPClient(client *http.Client) ClientOption {
    return func(c *Client) { c.httpClient = client }
}

func WithTimeout(d time.Duration) ClientOption {
    return func(c *Client) {
        c.httpClient.Timeout = d
    }
}

// Observability
func WithLogger(l *slog.Logger) ClientOption {
    return func(c *Client) { c.logger = l }
}

func WithTracer(t trace.Tracer) ClientOption {
    return func(c *Client) { c.tracer = t }
}

// Resilience
func WithRetries(n int) ClientOption {
    return func(c *Client) { c.maxRetries = n }
}

func WithRetryBackoff(d time.Duration) ClientOption {
    return func(c *Client) { c.retryBackoff = d }
}
```

### 1.8 Content Block Constructors (sdk/content.go)

```go
package vango

import (
    "encoding/base64"
    "github.com/vango-go/vai/pkg/core/types"
)

// Text creates a text content block.
func Text(s string) types.ContentBlock {
    return types.TextBlock{Type: "text", Text: s}
}

// Image creates an image content block from bytes.
func Image(data []byte, mediaType string) types.ContentBlock {
    return types.ImageBlock{
        Type: "image",
        Source: types.ImageSource{
            Type:      "base64",
            MediaType: mediaType,
            Data:      base64.StdEncoding.EncodeToString(data),
        },
    }
}

// ImageURL creates an image content block from a URL.
func ImageURL(url string) types.ContentBlock {
    return types.ImageBlock{
        Type: "image",
        Source: types.ImageSource{
            Type: "url",
            URL:  url,
        },
    }
}

// Audio creates an audio content block.
func Audio(data []byte, mediaType string) types.ContentBlock {
    return types.AudioBlock{
        Type: "audio",
        Source: types.AudioSource{
            Type:      "base64",
            MediaType: mediaType,
            Data:      base64.StdEncoding.EncodeToString(data),
        },
    }
}

// Video creates a video content block.
func Video(data []byte, mediaType string) types.ContentBlock {
    return types.VideoBlock{
        Type: "video",
        Source: types.VideoSource{
            Type:      "base64",
            MediaType: mediaType,
            Data:      base64.StdEncoding.EncodeToString(data),
        },
    }
}

// Document creates a document content block.
func Document(data []byte, mediaType, filename string) types.ContentBlock {
    return types.DocumentBlock{
        Type: "document",
        Source: types.DocumentSource{
            Type:      "base64",
            MediaType: mediaType,
            Data:      base64.StdEncoding.EncodeToString(data),
        },
        Filename: filename,
    }
}

// ToolResult creates a tool result content block.
func ToolResult(toolUseID string, content []types.ContentBlock) types.ContentBlock {
    return types.ToolResultBlock{
        Type:      "tool_result",
        ToolUseID: toolUseID,
        Content:   content,
    }
}

// ToolResultError creates an error tool result.
func ToolResultError(toolUseID string, errMsg string) types.ContentBlock {
    return types.ToolResultBlock{
        Type:      "tool_result",
        ToolUseID: toolUseID,
        Content:   []types.ContentBlock{Text(errMsg)},
        IsError:   true,
    }
}
```

---

## Testing Strategy

### Unit Tests
- JSON marshaling/unmarshaling for all types
- Content block constructors
- Message.Content flexible handling
- Error type behavior
- Usage aggregation

### Test Files
```
pkg/core/types/request_test.go
pkg/core/types/response_test.go
pkg/core/types/content_test.go
pkg/core/types/message_test.go
pkg/core/errors_test.go
sdk/content_test.go
sdk/options_test.go
```

---

## Acceptance Criteria

1. [x] All types compile and match API spec
2. [x] JSON marshaling produces correct output
3. [x] JSON unmarshaling handles all valid inputs
4. [x] Message.Content handles string, ContentBlock, and []ContentBlock
5. [x] Provider interface is defined (not implemented)
6. [x] Client skeleton compiles
7. [x] All constructors create valid content blocks
8. [x] Error types implement error interface
9. [x] Test coverage on JSON handling (56.9% types, 43.4% SDK)

---

## Files to Create

```
pkg/core/types/request.go
pkg/core/types/response.go
pkg/core/types/content.go
pkg/core/types/message.go
pkg/core/types/tool.go
pkg/core/types/usage.go
pkg/core/types/voice.go
pkg/core/types/stream.go
pkg/core/provider.go
pkg/core/errors.go
sdk/client.go
sdk/options.go
sdk/content.go
sdk/messages.go      (skeleton)
sdk/audio.go         (skeleton)
sdk/models.go        (skeleton)
```

---

## Estimated Effort

- Type definitions: ~500 lines
- JSON marshal/unmarshal logic: ~200 lines
- Tests: ~400 lines
- **Total: ~1100 lines**

---

## Next Phase

Phase 2: Anthropic Provider - implements CreateMessage and StreamMessage for the Anthropic provider, enabling the first working end-to-end flow.
