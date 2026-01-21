# Phase 2: Anthropic Provider (Direct Mode)

**Status:** ✅ Complete
**Priority:** Critical Path
**Dependencies:** Phase 1 (Foundation & Core Types)
**Completed:** December 2025

---

## Overview

Phase 2 implements the Anthropic provider, which is the **canonical format** for Vango. Since the Vango AI API is based on Anthropic's Messages API, this provider is essentially a passthrough with minimal translation. This makes it the ideal first provider to implement.

By the end of this phase, you will have a working `client.Messages.Create()` and `client.Messages.Stream()` that communicates with the real Anthropic API.

---

## Goals

1. Implement the Anthropic provider (passthrough to Anthropic Messages API)
2. Enable `client.Messages.Create()` for non-streaming requests
3. Enable `client.Messages.Stream()` for streaming responses
4. Support all content block types (text, image, tool_use, tool_result)
5. Handle Anthropic-specific headers and authentication
6. Properly translate Anthropic errors to Vango AI errors

---

## Deliverables

### 2.1 Anthropic Provider Structure

```
pkg/core/providers/
└── anthropic/
    ├── provider.go      # Provider implementation
    ├── client.go        # HTTP client wrapper
    ├── request.go       # Request building
    ├── response.go      # Response parsing
    ├── stream.go        # SSE stream handling
    └── errors.go        # Error translation
```

### 2.2 Provider Implementation (pkg/core/providers/anthropic/provider.go)

```go
package anthropic

import (
    "context"
    "net/http"

    "github.com/vango-go/vai/pkg/core"
    "github.com/vango-go/vai/pkg/core/types"
)

const (
    DefaultBaseURL   = "https://api.anthropic.com"
    APIVersion       = "2023-06-01"
    BetaHeader       = "prompt-caching-2024-07-31,web-search-2025-03-05"
)

type Provider struct {
    apiKey     string
    baseURL    string
    httpClient *http.Client
}

func New(apiKey string, opts ...Option) *Provider {
    p := &Provider{
        apiKey:     apiKey,
        baseURL:    DefaultBaseURL,
        httpClient: http.DefaultClient,
    }
    for _, opt := range opts {
        opt(p)
    }
    return p
}

func (p *Provider) Name() string {
    return "anthropic"
}

func (p *Provider) Capabilities() core.ProviderCapabilities {
    return core.ProviderCapabilities{
        Vision:           true,
        AudioInput:       false,
        AudioOutput:      false,
        Video:            false,
        Tools:            true,
        ToolStreaming:    true,
        Thinking:         true,
        StructuredOutput: true,
        NativeTools:      []string{"web_search", "code_execution", "computer_use", "text_editor"},
    }
}

// CreateMessage sends a non-streaming request to Anthropic.
func (p *Provider) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
    // Build request
    anthReq := p.buildRequest(req)

    // Make HTTP call
    resp, err := p.doRequest(ctx, anthReq, false)
    if err != nil {
        return nil, err
    }

    // Parse response (minimal translation needed - same format)
    return p.parseResponse(resp)
}

// StreamMessage sends a streaming request to Anthropic.
func (p *Provider) StreamMessage(ctx context.Context, req *types.MessageRequest) (core.EventStream, error) {
    // Build request with stream=true
    anthReq := p.buildRequest(req)
    anthReq.Stream = true

    // Make HTTP call (returns SSE stream)
    resp, err := p.doStreamRequest(ctx, anthReq)
    if err != nil {
        return nil, err
    }

    return newEventStream(resp), nil
}
```

### 2.3 HTTP Client (pkg/core/providers/anthropic/client.go)

```go
package anthropic

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
)

func (p *Provider) doRequest(ctx context.Context, req *anthropicRequest, stream bool) (*http.Response, error) {
    body, err := json.Marshal(req)
    if err != nil {
        return nil, fmt.Errorf("marshal request: %w", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/messages", bytes.NewReader(body))
    if err != nil {
        return nil, err
    }

    // Set headers
    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("X-API-Key", p.apiKey)
    httpReq.Header.Set("anthropic-version", APIVersion)
    httpReq.Header.Set("anthropic-beta", BetaHeader)

    if stream {
        httpReq.Header.Set("Accept", "text/event-stream")
    }

    resp, err := p.httpClient.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("http request: %w", err)
    }

    // Check for errors
    if resp.StatusCode >= 400 {
        defer resp.Body.Close()
        return nil, p.parseError(resp)
    }

    return resp, nil
}

func (p *Provider) doStreamRequest(ctx context.Context, req *anthropicRequest) (io.ReadCloser, error) {
    resp, err := p.doRequest(ctx, req, true)
    if err != nil {
        return nil, err
    }
    return resp.Body, nil
}
```

### 2.4 Request Building (pkg/core/providers/anthropic/request.go)

```go
package anthropic

import (
    "github.com/vango-go/vai/pkg/core/types"
)

// anthropicRequest is the Anthropic API request format.
// Since Vango AI API is based on Anthropic, this is nearly identical.
type anthropicRequest struct {
    Model         string                 `json:"model"`
    Messages      []types.Message        `json:"messages"`
    MaxTokens     int                    `json:"max_tokens"`
    System        any                    `json:"system,omitempty"`
    Temperature   *float64               `json:"temperature,omitempty"`
    TopP          *float64               `json:"top_p,omitempty"`
    TopK          *int                   `json:"top_k,omitempty"`
    StopSequences []string               `json:"stop_sequences,omitempty"`
    Tools         []anthropicTool        `json:"tools,omitempty"`
    ToolChoice    *types.ToolChoice      `json:"tool_choice,omitempty"`
    Stream        bool                   `json:"stream,omitempty"`
    Metadata      map[string]any         `json:"metadata,omitempty"`
}

type anthropicTool struct {
    Type        string            `json:"type"`
    Name        string            `json:"name,omitempty"`
    Description string            `json:"description,omitempty"`
    InputSchema *types.JSONSchema `json:"input_schema,omitempty"`
    // Native tool configs
    CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type cacheControl struct {
    Type string `json:"type"` // "ephemeral"
}

func (p *Provider) buildRequest(req *types.MessageRequest) *anthropicRequest {
    anthReq := &anthropicRequest{
        Model:         stripProviderPrefix(req.Model), // "anthropic/claude-sonnet-4" -> "claude-sonnet-4"
        Messages:      req.Messages,
        MaxTokens:     req.MaxTokens,
        System:        req.System,
        Temperature:   req.Temperature,
        TopP:          req.TopP,
        TopK:          req.TopK,
        StopSequences: req.StopSequences,
        Metadata:      req.Metadata,
    }

    // Set default max_tokens if not specified
    if anthReq.MaxTokens == 0 {
        anthReq.MaxTokens = 4096
    }

    // Convert tools
    if len(req.Tools) > 0 {
        anthReq.Tools = p.convertTools(req.Tools)
    }

    // Handle tool choice
    anthReq.ToolChoice = req.ToolChoice

    // Apply extensions (Anthropic-specific options)
    p.applyExtensions(anthReq, req.Extensions)

    return anthReq
}

func (p *Provider) convertTools(tools []types.Tool) []anthropicTool {
    result := make([]anthropicTool, 0, len(tools))

    for _, tool := range tools {
        switch tool.Type {
        case types.ToolTypeFunction:
            result = append(result, anthropicTool{
                Type:        "custom", // Anthropic uses "custom" for function tools
                Name:        tool.Name,
                Description: tool.Description,
                InputSchema: tool.InputSchema,
            })

        case types.ToolTypeWebSearch:
            result = append(result, anthropicTool{
                Type: "web_search_20250305",
            })

        case types.ToolTypeCodeExecution:
            result = append(result, anthropicTool{
                Type: "code_execution_20250825",
            })

        case types.ToolTypeComputerUse:
            cfg, _ := tool.Config.(types.ComputerUseConfig)
            result = append(result, anthropicTool{
                Type: "computer_20250124",
                // Note: display dimensions passed differently
            })
            _ = cfg // Use in actual implementation

        case types.ToolTypeTextEditor:
            result = append(result, anthropicTool{
                Type: "text_editor_20250124",
            })
        }
    }

    return result
}

func (p *Provider) applyExtensions(req *anthropicRequest, ext map[string]any) {
    if ext == nil {
        return
    }

    anthExt, ok := ext["anthropic"].(map[string]any)
    if !ok {
        return
    }

    // Handle thinking extension
    if thinking, ok := anthExt["thinking"].(map[string]any); ok {
        // Add thinking configuration
        _ = thinking
    }
}

func stripProviderPrefix(model string) string {
    // "anthropic/claude-sonnet-4" -> "claude-sonnet-4"
    for i, c := range model {
        if c == '/' {
            return model[i+1:]
        }
    }
    return model
}
```

### 2.5 Response Parsing (pkg/core/providers/anthropic/response.go)

```go
package anthropic

import (
    "encoding/json"
    "io"

    "github.com/vango-go/vai/pkg/core/types"
)

// anthropicResponse matches Anthropic's response format.
// Since Vango AI uses the same format, this is nearly identical to types.MessageResponse.
type anthropicResponse struct {
    ID           string              `json:"id"`
    Type         string              `json:"type"`
    Role         string              `json:"role"`
    Model        string              `json:"model"`
    Content      []json.RawMessage   `json:"content"`
    StopReason   string              `json:"stop_reason"`
    StopSequence *string             `json:"stop_sequence,omitempty"`
    Usage        types.Usage         `json:"usage"`
}

func (p *Provider) parseResponse(resp *http.Response) (*types.MessageResponse, error) {
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("read response: %w", err)
    }

    var anthResp anthropicResponse
    if err := json.Unmarshal(body, &anthResp); err != nil {
        return nil, fmt.Errorf("unmarshal response: %w", err)
    }

    // Parse content blocks
    content, err := parseContentBlocks(anthResp.Content)
    if err != nil {
        return nil, fmt.Errorf("parse content: %w", err)
    }

    return &types.MessageResponse{
        ID:           anthResp.ID,
        Type:         anthResp.Type,
        Role:         anthResp.Role,
        Model:        "anthropic/" + anthResp.Model, // Add provider prefix back
        Content:      content,
        StopReason:   types.StopReason(anthResp.StopReason),
        StopSequence: anthResp.StopSequence,
        Usage:        anthResp.Usage,
    }, nil
}

func parseContentBlocks(raw []json.RawMessage) ([]types.ContentBlock, error) {
    result := make([]types.ContentBlock, 0, len(raw))

    for _, r := range raw {
        // First, determine the type
        var typeObj struct {
            Type string `json:"type"`
        }
        if err := json.Unmarshal(r, &typeObj); err != nil {
            return nil, err
        }

        var block types.ContentBlock
        switch typeObj.Type {
        case "text":
            var tb types.TextBlock
            if err := json.Unmarshal(r, &tb); err != nil {
                return nil, err
            }
            block = tb

        case "tool_use":
            var tub types.ToolUseBlock
            if err := json.Unmarshal(r, &tub); err != nil {
                return nil, err
            }
            block = tub

        case "thinking":
            var thb types.ThinkingBlock
            if err := json.Unmarshal(r, &thb); err != nil {
                return nil, err
            }
            block = thb

        case "image":
            var ib types.ImageBlock
            if err := json.Unmarshal(r, &ib); err != nil {
                return nil, err
            }
            block = ib

        default:
            // Unknown block type - skip or return raw
            continue
        }

        result = append(result, block)
    }

    return result, nil
}
```

### 2.6 SSE Stream Handling (pkg/core/providers/anthropic/stream.go)

```go
package anthropic

import (
    "bufio"
    "encoding/json"
    "fmt"
    "io"
    "strings"

    "github.com/vango-go/vai/pkg/core"
    "github.com/vango-go/vai/pkg/core/types"
)

type eventStream struct {
    reader  *bufio.Reader
    closer  io.Closer
    current types.StreamEvent
    err     error
}

func newEventStream(body io.ReadCloser) *eventStream {
    return &eventStream{
        reader: bufio.NewReader(body),
        closer: body,
    }
}

func (s *eventStream) Next() (types.StreamEvent, error) {
    if s.err != nil {
        return nil, s.err
    }

    for {
        line, err := s.reader.ReadString('\n')
        if err != nil {
            if err == io.EOF {
                return nil, io.EOF
            }
            s.err = err
            return nil, err
        }

        line = strings.TrimSpace(line)

        // Skip empty lines
        if line == "" {
            continue
        }

        // Parse SSE format
        if strings.HasPrefix(line, "event: ") {
            // Event type - read the data line next
            eventType := strings.TrimPrefix(line, "event: ")

            // Read data line
            dataLine, err := s.reader.ReadString('\n')
            if err != nil {
                s.err = err
                return nil, err
            }
            dataLine = strings.TrimSpace(dataLine)

            if !strings.HasPrefix(dataLine, "data: ") {
                continue
            }

            data := strings.TrimPrefix(dataLine, "data: ")

            event, err := parseStreamEvent(eventType, []byte(data))
            if err != nil {
                s.err = err
                return nil, err
            }

            return event, nil
        }

        // Handle "data:" only format (some SSE implementations)
        if strings.HasPrefix(line, "data: ") {
            data := strings.TrimPrefix(line, "data: ")

            // Try to determine event type from data
            event, err := parseStreamEventFromData([]byte(data))
            if err != nil {
                continue // Skip unparseable events
            }
            return event, nil
        }
    }
}

func (s *eventStream) Close() error {
    return s.closer.Close()
}

func parseStreamEvent(eventType string, data []byte) (types.StreamEvent, error) {
    switch eventType {
    case "message_start":
        var e types.MessageStartEvent
        if err := json.Unmarshal(data, &e); err != nil {
            return nil, err
        }
        return e, nil

    case "content_block_start":
        var e types.ContentBlockStartEvent
        if err := json.Unmarshal(data, &e); err != nil {
            return nil, err
        }
        return e, nil

    case "content_block_delta":
        var e types.ContentBlockDeltaEvent
        if err := json.Unmarshal(data, &e); err != nil {
            return nil, err
        }
        return e, nil

    case "content_block_stop":
        var e types.ContentBlockStopEvent
        if err := json.Unmarshal(data, &e); err != nil {
            return nil, err
        }
        return e, nil

    case "message_delta":
        var e types.MessageDeltaEvent
        if err := json.Unmarshal(data, &e); err != nil {
            return nil, err
        }
        return e, nil

    case "message_stop":
        var e types.MessageStopEvent
        if err := json.Unmarshal(data, &e); err != nil {
            return nil, err
        }
        return e, nil

    case "error":
        var e types.ErrorEvent
        if err := json.Unmarshal(data, &e); err != nil {
            return nil, err
        }
        return e, nil

    case "ping":
        // Ignore ping events
        return nil, nil

    default:
        return nil, fmt.Errorf("unknown event type: %s", eventType)
    }
}

func parseStreamEventFromData(data []byte) (types.StreamEvent, error) {
    // Determine type from the data itself
    var typeObj struct {
        Type string `json:"type"`
    }
    if err := json.Unmarshal(data, &typeObj); err != nil {
        return nil, err
    }
    return parseStreamEvent(typeObj.Type, data)
}
```

### 2.7 Error Handling (pkg/core/providers/anthropic/errors.go)

```go
package anthropic

import (
    "encoding/json"
    "io"
    "net/http"

    "github.com/vango-go/vai/pkg/core"
)

type anthropicError struct {
    Type    string `json:"type"`
    Error   struct {
        Type    string `json:"type"`
        Message string `json:"message"`
    } `json:"error"`
}

func (p *Provider) parseError(resp *http.Response) error {
    body, _ := io.ReadAll(resp.Body)

    var anthErr anthropicError
    if err := json.Unmarshal(body, &anthErr); err != nil {
        // Can't parse error, return generic
        return &core.Error{
            Type:    core.ErrProvider,
            Message: string(body),
        }
    }

    // Map Anthropic error types to Vango AI error types
    var errType core.ErrorType
    switch anthErr.Error.Type {
    case "invalid_request_error":
        errType = core.ErrInvalidRequest
    case "authentication_error":
        errType = core.ErrAuthentication
    case "permission_error":
        errType = core.ErrPermission
    case "not_found_error":
        errType = core.ErrNotFound
    case "rate_limit_error":
        errType = core.ErrRateLimit
    case "api_error":
        errType = core.ErrAPI
    case "overloaded_error":
        errType = core.ErrOverloaded
    default:
        errType = core.ErrProvider
    }

    return &core.Error{
        Type:    errType,
        Message: anthErr.Error.Message,
        ProviderError: anthErr.Error,
    }
}
```

### 2.8 SDK Integration (sdk/messages.go)

```go
package vango

import (
    "context"

    "github.com/vango-go/vai/pkg/core/types"
)

type MessagesService struct {
    client *Client
}

// Create sends a non-streaming message request.
func (s *MessagesService) Create(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
    switch s.client.mode {
    case modeDirect:
        return s.createDirect(ctx, req)
    case modeProxy:
        return s.createProxy(ctx, req)
    default:
        return nil, fmt.Errorf("unknown client mode")
    }
}

func (s *MessagesService) createDirect(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
    provider, err := s.client.core.GetProvider(req.Model)
    if err != nil {
        return nil, err
    }
    return provider.CreateMessage(ctx, req)
}

func (s *MessagesService) createProxy(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
    // HTTP call to proxy - implemented in Phase 8
    return nil, fmt.Errorf("proxy mode not yet implemented")
}

// Stream sends a streaming message request.
func (s *MessagesService) Stream(ctx context.Context, req *types.MessageRequest) (*Stream, error) {
    switch s.client.mode {
    case modeDirect:
        return s.streamDirect(ctx, req)
    case modeProxy:
        return s.streamProxy(ctx, req)
    default:
        return nil, fmt.Errorf("unknown client mode")
    }
}

func (s *MessagesService) streamDirect(ctx context.Context, req *types.MessageRequest) (*Stream, error) {
    provider, err := s.client.core.GetProvider(req.Model)
    if err != nil {
        return nil, err
    }

    eventStream, err := provider.StreamMessage(ctx, req)
    if err != nil {
        return nil, err
    }

    return newStream(eventStream), nil
}

func (s *MessagesService) streamProxy(ctx context.Context, req *types.MessageRequest) (*Stream, error) {
    return nil, fmt.Errorf("proxy mode not yet implemented")
}
```

### 2.9 Stream Wrapper (sdk/stream.go)

```go
package vango

import (
    "sync/atomic"

    "github.com/vango-go/vai/pkg/core"
    "github.com/vango-go/vai/pkg/core/types"
)

// Stream wraps an EventStream and provides a channel-based API.
type Stream struct {
    eventStream core.EventStream
    events      chan types.StreamEvent
    response    *types.MessageResponse
    err         error
    closed      atomic.Bool
    done        chan struct{}
}

func newStream(es core.EventStream) *Stream {
    s := &Stream{
        eventStream: es,
        events:      make(chan types.StreamEvent, 100),
        done:        make(chan struct{}),
    }
    go s.run()
    return s
}

func (s *Stream) run() {
    defer close(s.events)
    defer close(s.done)

    // Accumulator for building final response
    var accumulator responseAccumulator

    for {
        event, err := s.eventStream.Next()
        if err != nil {
            if err != io.EOF {
                s.err = err
            }
            s.response = accumulator.build()
            return
        }

        if event == nil {
            continue // Skip nil events (e.g., ping)
        }

        // Accumulate for final response
        accumulator.accumulate(event)

        // Forward event
        select {
        case s.events <- event:
        case <-s.done:
            return
        }
    }
}

// Events returns the channel of streaming events.
func (s *Stream) Events() <-chan types.StreamEvent {
    return s.events
}

// Response returns the final accumulated response after the stream ends.
func (s *Stream) Response() *types.MessageResponse {
    <-s.done
    return s.response
}

// Err returns any error that occurred during streaming.
func (s *Stream) Err() error {
    <-s.done
    return s.err
}

// Close stops the stream and releases resources.
func (s *Stream) Close() error {
    if s.closed.Swap(true) {
        return nil // Already closed
    }
    return s.eventStream.Close()
}

// responseAccumulator builds a MessageResponse from stream events.
type responseAccumulator struct {
    response types.MessageResponse
    content  []types.ContentBlock
    // Track current content block being built
    currentBlockIndex int
    currentBlockText  strings.Builder
    currentToolInput  strings.Builder
}

func (a *responseAccumulator) accumulate(event types.StreamEvent) {
    switch e := event.(type) {
    case types.MessageStartEvent:
        a.response = e.Message

    case types.ContentBlockStartEvent:
        a.currentBlockIndex = e.Index
        // Add placeholder
        for len(a.content) <= e.Index {
            a.content = append(a.content, nil)
        }
        a.content[e.Index] = e.ContentBlock
        a.currentBlockText.Reset()
        a.currentToolInput.Reset()

    case types.ContentBlockDeltaEvent:
        switch d := e.Delta.(type) {
        case types.TextDelta:
            a.currentBlockText.WriteString(d.Text)
            // Update the text block
            if tb, ok := a.content[e.Index].(types.TextBlock); ok {
                tb.Text = a.currentBlockText.String()
                a.content[e.Index] = tb
            }

        case types.InputJSONDelta:
            a.currentToolInput.WriteString(d.PartialJSON)
        }

    case types.ContentBlockStopEvent:
        // Finalize tool input if applicable
        if tub, ok := a.content[e.Index].(types.ToolUseBlock); ok {
            if a.currentToolInput.Len() > 0 {
                json.Unmarshal([]byte(a.currentToolInput.String()), &tub.Input)
                a.content[e.Index] = tub
            }
        }

    case types.MessageDeltaEvent:
        a.response.StopReason = e.Delta.StopReason
        a.response.Usage = a.response.Usage.Add(e.Usage)

    case types.MessageStopEvent:
        a.response.Content = a.content
    }
}

func (a *responseAccumulator) build() *types.MessageResponse {
    a.response.Content = a.content
    return &a.response
}
```

### 2.10 Core Engine (pkg/core/engine.go)

```go
package core

import (
    "fmt"
    "os"
    "strings"

    "github.com/vango-go/vai/pkg/core/providers/anthropic"
)

// Engine manages provider instances.
type Engine struct {
    providers    map[string]Provider
    providerKeys map[string]string
}

// NewEngine creates a new engine with the given provider keys.
func NewEngine(keys map[string]string) *Engine {
    e := &Engine{
        providers:    make(map[string]Provider),
        providerKeys: keys,
    }
    e.initProviders()
    return e
}

func (e *Engine) initProviders() {
    // Initialize Anthropic if key is available
    anthropicKey := e.getKey("anthropic", "ANTHROPIC_API_KEY")
    if anthropicKey != "" {
        e.providers["anthropic"] = anthropic.New(anthropicKey)
    }

    // Other providers will be added in later phases
}

func (e *Engine) getKey(provider, envVar string) string {
    if key, ok := e.providerKeys[provider]; ok {
        return key
    }
    return os.Getenv(envVar)
}

// GetProvider returns the provider for the given model string.
func (e *Engine) GetProvider(model string) (Provider, error) {
    // Parse "provider/model-name"
    parts := strings.SplitN(model, "/", 2)
    if len(parts) != 2 {
        return nil, &Error{
            Type:    ErrInvalidRequest,
            Message: fmt.Sprintf("invalid model format: %s (expected 'provider/model')", model),
        }
    }

    providerName := parts[0]
    provider, ok := e.providers[providerName]
    if !ok {
        return nil, &Error{
            Type:    ErrNotFound,
            Message: fmt.Sprintf("provider not found: %s", providerName),
        }
    }

    return provider, nil
}
```

---

## Testing Strategy

### Unit Tests (mock HTTP)
- Request building from Vango AI types to Anthropic format
- Response parsing from Anthropic format to Vango AI types
- SSE event parsing
- Error translation

### Integration Tests (real API)
- `TestCreate_SimpleText` - Basic text generation
- `TestCreate_WithImage` - Vision request
- `TestCreate_WithTools` - Tool definitions
- `TestStream_SimpleText` - Streaming text
- `TestStream_ToolUse` - Streaming tool calls
- `TestStream_Accumulator` - Response accumulation
- `TestError_RateLimit` - Rate limit handling
- `TestError_InvalidRequest` - Invalid request handling

### Test Files
```
pkg/core/providers/anthropic/provider_test.go
pkg/core/providers/anthropic/request_test.go
pkg/core/providers/anthropic/response_test.go
pkg/core/providers/anthropic/stream_test.go
sdk/messages_test.go
sdk/stream_test.go
```

### Real API Test Example

```go
// +build integration

package anthropic_test

import (
    "context"
    "os"
    "testing"

    "github.com/vango-go/vai"
)

func TestCreate_SimpleText(t *testing.T) {
    if os.Getenv("ANTHROPIC_API_KEY") == "" {
        t.Skip("ANTHROPIC_API_KEY not set")
    }

    client := vai.NewClient()

    resp, err := client.Messages.Create(context.Background(), &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Say 'Hello, World!' and nothing else.")},
        },
        MaxTokens: 50,
    })
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if resp.ID == "" {
        t.Error("expected non-empty ID")
    }
    if resp.Role != "assistant" {
        t.Errorf("expected role 'assistant', got %q", resp.Role)
    }
    if !strings.Contains(resp.TextContent(), "Hello") {
        t.Errorf("expected 'Hello' in response, got %q", resp.TextContent())
    }
    if resp.Usage.InputTokens == 0 {
        t.Error("expected non-zero input tokens")
    }
    if resp.Usage.OutputTokens == 0 {
        t.Error("expected non-zero output tokens")
    }
}

func TestStream_SimpleText(t *testing.T) {
    if os.Getenv("ANTHROPIC_API_KEY") == "" {
        t.Skip("ANTHROPIC_API_KEY not set")
    }

    client := vai.NewClient()

    stream, err := client.Messages.Stream(context.Background(), &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Count from 1 to 5.")},
        },
        MaxTokens: 100,
    })
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    defer stream.Close()

    var gotTextDelta bool
    var text strings.Builder

    for event := range stream.Events() {
        switch e := event.(type) {
        case *vai.ContentBlockDeltaEvent:
            if delta, ok := e.Delta.(*vai.TextDelta); ok {
                gotTextDelta = true
                text.WriteString(delta.Text)
            }
        }
    }

    if !gotTextDelta {
        t.Error("expected text delta events")
    }

    resp := stream.Response()
    if resp == nil {
        t.Fatal("expected response after stream")
    }
    if resp.TextContent() != text.String() {
        t.Error("accumulated text doesn't match final response")
    }
}
```

---

## Acceptance Criteria

1. [x] `client.Messages.Create()` works with Anthropic API
2. [x] `client.Messages.Stream()` works with Anthropic API
3. [x] All content block types (text, image) are correctly sent
4. [x] Tool definitions are correctly formatted
5. [x] Responses are correctly parsed
6. [x] Streaming events are correctly parsed
7. [x] Stream accumulator builds correct final response
8. [x] Anthropic errors are translated to Vango AI errors
9. [x] Integration tests pass with real API key

---

## Files to Create

```
pkg/core/engine.go
pkg/core/providers/anthropic/provider.go
pkg/core/providers/anthropic/client.go
pkg/core/providers/anthropic/request.go
pkg/core/providers/anthropic/response.go
pkg/core/providers/anthropic/stream.go
pkg/core/providers/anthropic/errors.go
pkg/core/providers/anthropic/options.go
sdk/messages.go
sdk/stream.go
```

---

## Estimated Effort

- Anthropic provider: ~600 lines
- SSE stream handling: ~200 lines
- SDK integration: ~300 lines
- Tests: ~500 lines
- **Total: ~1600 lines**

---

## Next Phase

Phase 3: Tool System & Run Loop - implements FuncAsTool, tool handlers, and the Messages.Run() loop for agentic workflows.
