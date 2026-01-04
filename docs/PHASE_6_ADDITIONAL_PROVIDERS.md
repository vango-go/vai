# Phase 6: Additional Providers

**Status:** Not Started
**Priority:** High
**Dependencies:** Phase 5 (Integration Tests)

---

## Overview

Phase 6 adds support for additional LLM providers beyond Anthropic. Each provider must translate between the Vango API format (Anthropic-style) and the provider's native format. The integration test suite from Phase 5 validates each provider implementation.

This phase implements:
1. **OpenAI** - Chat Completions API (primary) + Responses API (for file search)
2. **Google Gemini** - Gemini API
3. **Groq** - OpenAI-compatible API

---

## Goals

1. Implement OpenAI provider with full translation layer
2. Implement Gemini provider with full translation layer
3. Implement Groq provider (OpenAI-compatible)
4. Register all providers in the core engine
5. Run existing integration tests against each provider
6. Handle provider-specific capabilities and limitations

---

## Provider Implementation Strategy

Each provider needs:
1. **Request Translation**: Vango format → Provider format
2. **Response Translation**: Provider format → Vango format
3. **Streaming Translation**: Provider SSE events → Vango SSE events
4. **Tool Normalization**: Vango tools → Provider-specific tools
5. **Error Mapping**: Provider errors → Vango errors

---

## Deliverables

### 6.1 Package Structure

```
pkg/core/providers/
├── anthropic/        # (Already done in Phase 2)
├── openai/
│   ├── provider.go   # Provider implementation
│   ├── client.go     # HTTP client
│   ├── request.go    # Vango → OpenAI translation
│   ├── response.go   # OpenAI → Vango translation
│   ├── stream.go     # SSE stream handling
│   ├── tools.go      # Tool translation
│   └── errors.go     # Error mapping
├── gemini/
│   ├── provider.go
│   ├── client.go
│   ├── request.go
│   ├── response.go
│   ├── stream.go
│   └── errors.go
└── groq/
    ├── provider.go   # Extends OpenAI (compatible)
    └── options.go
```

### 6.2 OpenAI Provider (pkg/core/providers/openai/provider.go)

```go
package openai

import (
    "context"
    "net/http"

    "github.com/vango-go/vai/pkg/core"
    "github.com/vango-go/vai/pkg/core/types"
)

const (
    DefaultBaseURL = "https://api.openai.com/v1"
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
    return "openai"
}

func (p *Provider) Capabilities() core.ProviderCapabilities {
    return core.ProviderCapabilities{
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

func (p *Provider) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
    // Translate Vango request to OpenAI format
    openaiReq := p.translateRequest(req)

    // Make HTTP call
    resp, err := p.doRequest(ctx, openaiReq)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    // Parse and translate response
    return p.parseResponse(resp)
}

func (p *Provider) StreamMessage(ctx context.Context, req *types.MessageRequest) (core.EventStream, error) {
    openaiReq := p.translateRequest(req)
    openaiReq.Stream = true
    openaiReq.StreamOptions = &streamOptions{IncludeUsage: true}

    resp, err := p.doStreamRequest(ctx, openaiReq)
    if err != nil {
        return nil, err
    }

    return newEventStream(resp), nil
}
```

### 6.3 OpenAI Request Translation (pkg/core/providers/openai/request.go)

```go
package openai

import (
    "encoding/base64"
    "encoding/json"
    "fmt"

    "github.com/vango-go/vai/pkg/core/types"
)

// OpenAI Chat Completions request format
type chatRequest struct {
    Model          string          `json:"model"`
    Messages       []chatMessage   `json:"messages"`
    MaxTokens      *int            `json:"max_completion_tokens,omitempty"`
    Temperature    *float64        `json:"temperature,omitempty"`
    TopP           *float64        `json:"top_p,omitempty"`
    Stop           []string        `json:"stop,omitempty"`
    Tools          []chatTool      `json:"tools,omitempty"`
    ToolChoice     any             `json:"tool_choice,omitempty"`
    ResponseFormat *responseFormat `json:"response_format,omitempty"`
    Stream         bool            `json:"stream,omitempty"`
    StreamOptions  *streamOptions  `json:"stream_options,omitempty"`
}

type chatMessage struct {
    Role       string      `json:"role"`
    Content    any         `json:"content"` // string or []contentPart
    ToolCalls  []toolCall  `json:"tool_calls,omitempty"`
    ToolCallID string      `json:"tool_call_id,omitempty"`
}

type contentPart struct {
    Type     string    `json:"type"`
    Text     string    `json:"text,omitempty"`
    ImageURL *imageURL `json:"image_url,omitempty"`
    InputAudio *inputAudio `json:"input_audio,omitempty"`
}

type imageURL struct {
    URL    string `json:"url"`
    Detail string `json:"detail,omitempty"`
}

type inputAudio struct {
    Data   string `json:"data"`   // base64
    Format string `json:"format"` // "wav", "mp3"
}

type chatTool struct {
    Type     string       `json:"type"` // "function"
    Function toolFunction `json:"function"`
}

type toolFunction struct {
    Name        string          `json:"name"`
    Description string          `json:"description,omitempty"`
    Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type toolCall struct {
    ID       string `json:"id"`
    Type     string `json:"type"`
    Function struct {
        Name      string `json:"name"`
        Arguments string `json:"arguments"`
    } `json:"function"`
}

type responseFormat struct {
    Type       string      `json:"type"` // "json_schema", "json_object", "text"
    JSONSchema *jsonSchema `json:"json_schema,omitempty"`
}

type jsonSchema struct {
    Name   string          `json:"name"`
    Schema json.RawMessage `json:"schema"`
    Strict bool            `json:"strict"`
}

type streamOptions struct {
    IncludeUsage bool `json:"include_usage"`
}

func (p *Provider) translateRequest(req *types.MessageRequest) *chatRequest {
    openaiReq := &chatRequest{
        Model:       stripProviderPrefix(req.Model),
        Temperature: req.Temperature,
        TopP:        req.TopP,
        Stop:        req.StopSequences,
    }

    if req.MaxTokens > 0 {
        openaiReq.MaxTokens = &req.MaxTokens
    }

    // Translate messages
    openaiReq.Messages = p.translateMessages(req.Messages, req.System)

    // Translate tools
    if len(req.Tools) > 0 {
        openaiReq.Tools = p.translateTools(req.Tools)
    }

    // Translate tool choice
    if req.ToolChoice != nil {
        openaiReq.ToolChoice = p.translateToolChoice(req.ToolChoice)
    }

    // Translate output format
    if req.OutputFormat != nil && req.OutputFormat.Type == "json_schema" {
        schemaBytes, _ := json.Marshal(req.OutputFormat.JSONSchema)
        openaiReq.ResponseFormat = &responseFormat{
            Type: "json_schema",
            JSONSchema: &jsonSchema{
                Name:   "response",
                Schema: schemaBytes,
                Strict: true,
            },
        }
    }

    return openaiReq
}

func (p *Provider) translateMessages(messages []types.Message, system any) []chatMessage {
    result := make([]chatMessage, 0, len(messages)+1)

    // Add system message if present
    if system != nil {
        switch s := system.(type) {
        case string:
            result = append(result, chatMessage{Role: "system", Content: s})
        case []types.ContentBlock:
            // Convert content blocks to text for system
            var text string
            for _, block := range s {
                if tb, ok := block.(types.TextBlock); ok {
                    text += tb.Text + "\n"
                }
            }
            result = append(result, chatMessage{Role: "system", Content: text})
        }
    }

    for _, msg := range messages {
        openaiMsg := chatMessage{Role: msg.Role}

        blocks := msg.ContentBlocks()

        // Check if this is a tool result message
        if len(blocks) > 0 {
            if tr, ok := blocks[0].(types.ToolResultBlock); ok {
                // OpenAI uses "tool" role for tool results
                openaiMsg.Role = "tool"
                openaiMsg.ToolCallID = tr.ToolUseID
                openaiMsg.Content = p.toolResultToText(tr.Content)
                result = append(result, openaiMsg)
                continue
            }
        }

        // Check if assistant message has tool calls
        if msg.Role == "assistant" {
            for _, block := range blocks {
                if tu, ok := block.(types.ToolUseBlock); ok {
                    inputJSON, _ := json.Marshal(tu.Input)
                    openaiMsg.ToolCalls = append(openaiMsg.ToolCalls, toolCall{
                        ID:   tu.ID,
                        Type: "function",
                        Function: struct {
                            Name      string `json:"name"`
                            Arguments string `json:"arguments"`
                        }{
                            Name:      tu.Name,
                            Arguments: string(inputJSON),
                        },
                    })
                }
            }
        }

        // Convert content blocks
        openaiMsg.Content = p.translateContentBlocks(blocks)

        result = append(result, openaiMsg)
    }

    return result
}

func (p *Provider) translateContentBlocks(blocks []types.ContentBlock) any {
    // If only text, return as string
    if len(blocks) == 1 {
        if tb, ok := blocks[0].(types.TextBlock); ok {
            return tb.Text
        }
    }

    // Multiple blocks or non-text: use content parts array
    parts := make([]contentPart, 0, len(blocks))

    for _, block := range blocks {
        switch b := block.(type) {
        case types.TextBlock:
            parts = append(parts, contentPart{Type: "text", Text: b.Text})

        case types.ImageBlock:
            var url string
            if b.Source.Type == "url" {
                url = b.Source.URL
            } else {
                // Convert base64 to data URL
                url = fmt.Sprintf("data:%s;base64,%s", b.Source.MediaType, b.Source.Data)
            }
            parts = append(parts, contentPart{
                Type:     "image_url",
                ImageURL: &imageURL{URL: url},
            })

        case types.AudioBlock:
            parts = append(parts, contentPart{
                Type: "input_audio",
                InputAudio: &inputAudio{
                    Data:   b.Source.Data,
                    Format: getAudioFormat(b.Source.MediaType),
                },
            })

        case types.ToolUseBlock:
            // Skip tool use blocks - they're handled separately
            continue
        }
    }

    return parts
}

func (p *Provider) translateTools(tools []types.Tool) []chatTool {
    result := make([]chatTool, 0, len(tools))

    for _, tool := range tools {
        switch tool.Type {
        case types.ToolTypeFunction:
            schemaBytes, _ := json.Marshal(tool.InputSchema)
            result = append(result, chatTool{
                Type: "function",
                Function: toolFunction{
                    Name:        tool.Name,
                    Description: tool.Description,
                    Parameters:  schemaBytes,
                },
            })

        case types.ToolTypeWebSearch:
            result = append(result, chatTool{
                Type: "web_search",
            })

        case types.ToolTypeCodeExecution:
            result = append(result, chatTool{
                Type: "code_interpreter",
            })
        }
    }

    return result
}

func (p *Provider) translateToolChoice(tc *types.ToolChoice) any {
    switch tc.Type {
    case "auto":
        return "auto"
    case "none":
        return "none"
    case "any":
        return "required"
    case "tool":
        return map[string]any{
            "type": "function",
            "function": map[string]string{
                "name": tc.Name,
            },
        }
    }
    return "auto"
}

func (p *Provider) toolResultToText(content []types.ContentBlock) string {
    var result string
    for _, block := range content {
        if tb, ok := block.(types.TextBlock); ok {
            result += tb.Text
        }
    }
    return result
}

func getAudioFormat(mediaType string) string {
    switch mediaType {
    case "audio/wav":
        return "wav"
    case "audio/mpeg", "audio/mp3":
        return "mp3"
    default:
        return "wav"
    }
}

func stripProviderPrefix(model string) string {
    for i, c := range model {
        if c == '/' {
            return model[i+1:]
        }
    }
    return model
}
```

### 6.4 OpenAI Response Translation (pkg/core/providers/openai/response.go)

```go
package openai

import (
    "encoding/json"
    "io"
    "net/http"

    "github.com/vango-go/vai/pkg/core/types"
)

// OpenAI Chat Completions response
type chatResponse struct {
    ID      string `json:"id"`
    Object  string `json:"object"`
    Created int64  `json:"created"`
    Model   string `json:"model"`
    Choices []struct {
        Index        int         `json:"index"`
        Message      chatMessage `json:"message"`
        FinishReason string      `json:"finish_reason"`
    } `json:"choices"`
    Usage struct {
        PromptTokens     int `json:"prompt_tokens"`
        CompletionTokens int `json:"completion_tokens"`
        TotalTokens      int `json:"total_tokens"`
    } `json:"usage"`
}

func (p *Provider) parseResponse(resp *http.Response) (*types.MessageResponse, error) {
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, err
    }

    var openaiResp chatResponse
    if err := json.Unmarshal(body, &openaiResp); err != nil {
        return nil, err
    }

    if len(openaiResp.Choices) == 0 {
        return nil, fmt.Errorf("no choices in response")
    }

    choice := openaiResp.Choices[0]

    // Build content blocks
    content := make([]types.ContentBlock, 0)

    // Add text content
    if choice.Message.Content != nil {
        switch c := choice.Message.Content.(type) {
        case string:
            if c != "" {
                content = append(content, types.TextBlock{Type: "text", Text: c})
            }
        }
    }

    // Add tool calls
    for _, tc := range choice.Message.ToolCalls {
        var input map[string]any
        json.Unmarshal([]byte(tc.Function.Arguments), &input)

        content = append(content, types.ToolUseBlock{
            Type:  "tool_use",
            ID:    tc.ID,
            Name:  tc.Function.Name,
            Input: input,
        })
    }

    // Map finish reason
    stopReason := p.mapFinishReason(choice.FinishReason)

    return &types.MessageResponse{
        ID:         openaiResp.ID,
        Type:       "message",
        Role:       "assistant",
        Model:      "openai/" + openaiResp.Model,
        Content:    content,
        StopReason: stopReason,
        Usage: types.Usage{
            InputTokens:  openaiResp.Usage.PromptTokens,
            OutputTokens: openaiResp.Usage.CompletionTokens,
            TotalTokens:  openaiResp.Usage.TotalTokens,
        },
    }, nil
}

func (p *Provider) mapFinishReason(reason string) types.StopReason {
    switch reason {
    case "stop":
        return types.StopReasonEndTurn
    case "length":
        return types.StopReasonMaxTokens
    case "tool_calls":
        return types.StopReasonToolUse
    case "content_filter":
        return types.StopReasonEndTurn
    default:
        return types.StopReasonEndTurn
    }
}
```

### 6.5 OpenAI Streaming (pkg/core/providers/openai/stream.go)

```go
package openai

import (
    "bufio"
    "encoding/json"
    "io"
    "strings"

    "github.com/vango-go/vai/pkg/core/types"
)

type eventStream struct {
    reader      *bufio.Reader
    closer      io.Closer
    responseID  string
    model       string
    accumulator streamAccumulator
}

type streamAccumulator struct {
    textContent    strings.Builder
    toolCalls      map[int]*toolCallAccumulator
    finishReason   string
    inputTokens    int
    outputTokens   int
}

type toolCallAccumulator struct {
    ID            string
    Name          string
    ArgumentsJSON strings.Builder
}

// OpenAI stream chunk format
type chatChunk struct {
    ID      string `json:"id"`
    Object  string `json:"object"`
    Model   string `json:"model"`
    Choices []struct {
        Index int `json:"index"`
        Delta struct {
            Content   string     `json:"content,omitempty"`
            ToolCalls []toolCall `json:"tool_calls,omitempty"`
        } `json:"delta"`
        FinishReason string `json:"finish_reason,omitempty"`
    } `json:"choices"`
    Usage *struct {
        PromptTokens     int `json:"prompt_tokens"`
        CompletionTokens int `json:"completion_tokens"`
    } `json:"usage,omitempty"`
}

func newEventStream(body io.ReadCloser) *eventStream {
    return &eventStream{
        reader: bufio.NewReader(body),
        closer: body,
        accumulator: streamAccumulator{
            toolCalls: make(map[int]*toolCallAccumulator),
        },
    }
}

func (s *eventStream) Next() (types.StreamEvent, error) {
    for {
        line, err := s.reader.ReadString('\n')
        if err != nil {
            if err == io.EOF {
                return s.buildFinalEvents()
            }
            return nil, err
        }

        line = strings.TrimSpace(line)
        if line == "" || !strings.HasPrefix(line, "data: ") {
            continue
        }

        data := strings.TrimPrefix(line, "data: ")
        if data == "[DONE]" {
            return s.buildFinalEvents()
        }

        var chunk chatChunk
        if err := json.Unmarshal([]byte(data), &chunk); err != nil {
            continue
        }

        // Store response ID and model
        if chunk.ID != "" {
            s.responseID = chunk.ID
        }
        if chunk.Model != "" {
            s.model = chunk.Model
        }

        // Handle usage (comes at end with stream_options)
        if chunk.Usage != nil {
            s.accumulator.inputTokens = chunk.Usage.PromptTokens
            s.accumulator.outputTokens = chunk.Usage.CompletionTokens
        }

        if len(chunk.Choices) == 0 {
            continue
        }

        choice := chunk.Choices[0]

        // Handle finish reason
        if choice.FinishReason != "" {
            s.accumulator.finishReason = choice.FinishReason
        }

        // Handle text delta
        if choice.Delta.Content != "" {
            s.accumulator.textContent.WriteString(choice.Delta.Content)
            return types.ContentBlockDeltaEvent{
                Type:  "content_block_delta",
                Index: 0,
                Delta: types.TextDelta{
                    Type: "text_delta",
                    Text: choice.Delta.Content,
                },
            }, nil
        }

        // Handle tool call deltas
        for _, tc := range choice.Delta.ToolCalls {
            acc, exists := s.accumulator.toolCalls[tc.Index]
            if !exists {
                acc = &toolCallAccumulator{
                    ID:   tc.ID,
                    Name: tc.Function.Name,
                }
                s.accumulator.toolCalls[tc.Index] = acc

                // Emit tool call start
                return types.ContentBlockStartEvent{
                    Type:  "content_block_start",
                    Index: tc.Index + 1, // Text is index 0
                    ContentBlock: types.ToolUseBlock{
                        Type: "tool_use",
                        ID:   tc.ID,
                        Name: tc.Function.Name,
                    },
                }, nil
            }

            if tc.Function.Arguments != "" {
                acc.ArgumentsJSON.WriteString(tc.Function.Arguments)
                return types.ContentBlockDeltaEvent{
                    Type:  "content_block_delta",
                    Index: tc.Index + 1,
                    Delta: types.InputJSONDelta{
                        Type:        "input_json_delta",
                        PartialJSON: tc.Function.Arguments,
                    },
                }, nil
            }
        }
    }
}

func (s *eventStream) buildFinalEvents() (types.StreamEvent, error) {
    // Return message_delta with stop reason and usage
    return types.MessageDeltaEvent{
        Type: "message_delta",
        Delta: struct {
            StopReason types.StopReason `json:"stop_reason,omitempty"`
        }{
            StopReason: s.mapFinishReason(s.accumulator.finishReason),
        },
        Usage: types.Usage{
            InputTokens:  s.accumulator.inputTokens,
            OutputTokens: s.accumulator.outputTokens,
            TotalTokens:  s.accumulator.inputTokens + s.accumulator.outputTokens,
        },
    }, io.EOF
}

func (s *eventStream) mapFinishReason(reason string) types.StopReason {
    switch reason {
    case "stop":
        return types.StopReasonEndTurn
    case "length":
        return types.StopReasonMaxTokens
    case "tool_calls":
        return types.StopReasonToolUse
    default:
        return types.StopReasonEndTurn
    }
}

func (s *eventStream) Close() error {
    return s.closer.Close()
}
```

### 6.6 Gemini Provider (pkg/core/providers/gemini/provider.go)

```go
package gemini

import (
    "context"
    "net/http"

    "github.com/vango-go/vai/pkg/core"
    "github.com/vango-go/vai/pkg/core/types"
)

const (
    DefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
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
    return "gemini"
}

func (p *Provider) Capabilities() core.ProviderCapabilities {
    return core.ProviderCapabilities{
        Vision:           true,
        AudioInput:       true,
        AudioOutput:      true,
        Video:            true, // Gemini supports video
        Tools:            true,
        ToolStreaming:    true,
        Thinking:         true, // Gemini 2.0 has thinking
        StructuredOutput: true,
        NativeTools:      []string{"web_search", "code_execution"},
    }
}

func (p *Provider) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
    geminiReq := p.translateRequest(req)
    resp, err := p.doRequest(ctx, req.Model, geminiReq)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    return p.parseResponse(resp)
}

func (p *Provider) StreamMessage(ctx context.Context, req *types.MessageRequest) (core.EventStream, error) {
    geminiReq := p.translateRequest(req)
    resp, err := p.doStreamRequest(ctx, req.Model, geminiReq)
    if err != nil {
        return nil, err
    }
    return newEventStream(resp), nil
}
```

### 6.7 Gemini Request Translation (pkg/core/providers/gemini/request.go)

```go
package gemini

import (
    "encoding/base64"

    "github.com/vango-go/vai/pkg/core/types"
)

// Gemini generateContent request
type generateContentRequest struct {
    Contents          []content            `json:"contents"`
    SystemInstruction *content             `json:"systemInstruction,omitempty"`
    Tools             []tool               `json:"tools,omitempty"`
    ToolConfig        *toolConfig          `json:"toolConfig,omitempty"`
    GenerationConfig  *generationConfig    `json:"generationConfig,omitempty"`
}

type content struct {
    Role  string `json:"role"` // "user" or "model"
    Parts []part `json:"parts"`
}

type part struct {
    Text         string        `json:"text,omitempty"`
    InlineData   *inlineData   `json:"inlineData,omitempty"`
    FunctionCall *functionCall `json:"functionCall,omitempty"`
    FunctionResponse *functionResponse `json:"functionResponse,omitempty"`
}

type inlineData struct {
    MimeType string `json:"mimeType"`
    Data     string `json:"data"` // base64
}

type functionCall struct {
    Name string         `json:"name"`
    Args map[string]any `json:"args"`
}

type functionResponse struct {
    Name     string `json:"name"`
    Response any    `json:"response"`
}

type tool struct {
    FunctionDeclarations []functionDeclaration `json:"functionDeclarations,omitempty"`
    GoogleSearch         *googleSearch         `json:"googleSearch,omitempty"`
    CodeExecution        *codeExecution        `json:"codeExecution,omitempty"`
}

type functionDeclaration struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type googleSearch struct{}
type codeExecution struct{}

type toolConfig struct {
    FunctionCallingConfig *functionCallingConfig `json:"functionCallingConfig,omitempty"`
}

type functionCallingConfig struct {
    Mode string `json:"mode"` // "AUTO", "ANY", "NONE"
}

type generationConfig struct {
    MaxOutputTokens  *int     `json:"maxOutputTokens,omitempty"`
    Temperature      *float64 `json:"temperature,omitempty"`
    TopP             *float64 `json:"topP,omitempty"`
    TopK             *int     `json:"topK,omitempty"`
    StopSequences    []string `json:"stopSequences,omitempty"`
    ResponseMimeType string   `json:"responseMimeType,omitempty"`
    ResponseSchema   any      `json:"responseSchema,omitempty"`
}

func (p *Provider) translateRequest(req *types.MessageRequest) *generateContentRequest {
    geminiReq := &generateContentRequest{
        GenerationConfig: &generationConfig{
            Temperature:   req.Temperature,
            TopP:          req.TopP,
            TopK:          req.TopK,
            StopSequences: req.StopSequences,
        },
    }

    if req.MaxTokens > 0 {
        geminiReq.GenerationConfig.MaxOutputTokens = &req.MaxTokens
    }

    // System instruction
    if req.System != nil {
        switch s := req.System.(type) {
        case string:
            geminiReq.SystemInstruction = &content{
                Parts: []part{{Text: s}},
            }
        }
    }

    // Translate messages
    geminiReq.Contents = p.translateMessages(req.Messages)

    // Translate tools
    if len(req.Tools) > 0 {
        geminiReq.Tools = p.translateTools(req.Tools)
    }

    // Tool choice
    if req.ToolChoice != nil {
        geminiReq.ToolConfig = p.translateToolChoice(req.ToolChoice)
    }

    // Structured output
    if req.OutputFormat != nil && req.OutputFormat.Type == "json_schema" {
        geminiReq.GenerationConfig.ResponseMimeType = "application/json"
        geminiReq.GenerationConfig.ResponseSchema = req.OutputFormat.JSONSchema
    }

    return geminiReq
}

func (p *Provider) translateMessages(messages []types.Message) []content {
    result := make([]content, 0, len(messages))

    for _, msg := range messages {
        role := "user"
        if msg.Role == "assistant" {
            role = "model"
        }

        c := content{Role: role}

        for _, block := range msg.ContentBlocks() {
            switch b := block.(type) {
            case types.TextBlock:
                c.Parts = append(c.Parts, part{Text: b.Text})

            case types.ImageBlock:
                c.Parts = append(c.Parts, part{
                    InlineData: &inlineData{
                        MimeType: b.Source.MediaType,
                        Data:     b.Source.Data,
                    },
                })

            case types.AudioBlock:
                c.Parts = append(c.Parts, part{
                    InlineData: &inlineData{
                        MimeType: b.Source.MediaType,
                        Data:     b.Source.Data,
                    },
                })

            case types.VideoBlock:
                c.Parts = append(c.Parts, part{
                    InlineData: &inlineData{
                        MimeType: b.Source.MediaType,
                        Data:     b.Source.Data,
                    },
                })

            case types.ToolUseBlock:
                c.Parts = append(c.Parts, part{
                    FunctionCall: &functionCall{
                        Name: b.Name,
                        Args: b.Input,
                    },
                })

            case types.ToolResultBlock:
                c.Parts = append(c.Parts, part{
                    FunctionResponse: &functionResponse{
                        Name:     b.ToolUseID, // Note: Gemini uses name, not ID
                        Response: p.toolResultToMap(b.Content),
                    },
                })
            }
        }

        result = append(result, c)
    }

    return result
}

func (p *Provider) translateTools(tools []types.Tool) []tool {
    var functionDecls []functionDeclaration
    var geminiTools []tool

    for _, t := range tools {
        switch t.Type {
        case types.ToolTypeFunction:
            schemaBytes, _ := json.Marshal(t.InputSchema)
            functionDecls = append(functionDecls, functionDeclaration{
                Name:        t.Name,
                Description: t.Description,
                Parameters:  schemaBytes,
            })

        case types.ToolTypeWebSearch:
            geminiTools = append(geminiTools, tool{
                GoogleSearch: &googleSearch{},
            })

        case types.ToolTypeCodeExecution:
            geminiTools = append(geminiTools, tool{
                CodeExecution: &codeExecution{},
            })
        }
    }

    if len(functionDecls) > 0 {
        geminiTools = append(geminiTools, tool{
            FunctionDeclarations: functionDecls,
        })
    }

    return geminiTools
}

func (p *Provider) translateToolChoice(tc *types.ToolChoice) *toolConfig {
    mode := "AUTO"
    switch tc.Type {
    case "none":
        mode = "NONE"
    case "any":
        mode = "ANY"
    case "tool":
        mode = "ANY" // Gemini doesn't support forcing specific tool
    }

    return &toolConfig{
        FunctionCallingConfig: &functionCallingConfig{
            Mode: mode,
        },
    }
}

func (p *Provider) toolResultToMap(content []types.ContentBlock) map[string]any {
    result := make(map[string]any)
    for _, block := range content {
        if tb, ok := block.(types.TextBlock); ok {
            result["result"] = tb.Text
        }
    }
    return result
}
```

### 6.8 Groq Provider (pkg/core/providers/groq/provider.go)

```go
package groq

import (
    "github.com/vango-go/vai/pkg/core/providers/openai"
)

const (
    DefaultBaseURL = "https://api.groq.com/openai/v1"
)

// Provider is an OpenAI-compatible provider for Groq.
type Provider struct {
    *openai.Provider
}

func New(apiKey string, opts ...Option) *Provider {
    // Create base OpenAI provider with Groq URL
    base := openai.New(apiKey, openai.WithBaseURL(DefaultBaseURL))

    return &Provider{Provider: base}
}

func (p *Provider) Name() string {
    return "groq"
}

func (p *Provider) Capabilities() core.ProviderCapabilities {
    return core.ProviderCapabilities{
        Vision:           true, // Some Groq models support vision
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
```

### 6.9 Register Providers in Engine (pkg/core/engine.go)

```go
package core

import (
    "os"

    "github.com/vango-go/vai/pkg/core/providers/anthropic"
    "github.com/vango-go/vai/pkg/core/providers/gemini"
    "github.com/vango-go/vai/pkg/core/providers/groq"
    "github.com/vango-go/vai/pkg/core/providers/openai"
)

func (e *Engine) initProviders() {
    // Anthropic
    if key := e.getKey("anthropic", "ANTHROPIC_API_KEY"); key != "" {
        e.providers["anthropic"] = anthropic.New(key)
    }

    // OpenAI
    if key := e.getKey("openai", "OPENAI_API_KEY"); key != "" {
        e.providers["openai"] = openai.New(key)
    }

    // Gemini
    if key := e.getKey("gemini", "GOOGLE_API_KEY"); key != "" {
        e.providers["gemini"] = gemini.New(key)
    }
    // Also check GEMINI_API_KEY
    if key := e.getKey("gemini", "GEMINI_API_KEY"); key != "" {
        e.providers["gemini"] = gemini.New(key)
    }

    // Groq
    if key := e.getKey("groq", "GROQ_API_KEY"); key != "" {
        e.providers["groq"] = groq.New(key)
    }
}
```

---

## Testing Strategy

### Run Existing Tests Against All Providers

```go
// tests/integration/providers_test.go

// +build integration

package integration_test

var testProviders = []struct {
    name        string
    model       string
    envKey      string
}{
    {"anthropic", "anthropic/claude-sonnet-4", "ANTHROPIC_API_KEY"},
    {"openai", "openai/gpt-4o", "OPENAI_API_KEY"},
    {"gemini", "gemini/gemini-2.0-flash", "GOOGLE_API_KEY"},
    {"groq", "groq/llama-3.3-70b", "GROQ_API_KEY"},
}

func TestAllProviders_SimpleText(t *testing.T) {
    for _, p := range testProviders {
        t.Run(p.name, func(t *testing.T) {
            if os.Getenv(p.envKey) == "" {
                t.Skipf("%s not set", p.envKey)
            }

            resp, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
                Model: p.model,
                Messages: []vai.Message{
                    {Role: "user", Content: vai.Text("Say 'Hello' and nothing else.")},
                },
                MaxTokens: 10,
            })

            require.NoError(t, err)
            assert.Contains(t, resp.TextContent(), "Hello")
        })
    }
}

func TestAllProviders_Streaming(t *testing.T) {
    for _, p := range testProviders {
        t.Run(p.name, func(t *testing.T) {
            if os.Getenv(p.envKey) == "" {
                t.Skipf("%s not set", p.envKey)
            }

            stream, err := testClient.Messages.Stream(testCtx, &vai.MessageRequest{
                Model: p.model,
                Messages: []vai.Message{
                    {Role: "user", Content: vai.Text("Count from 1 to 3.")},
                },
                MaxTokens: 50,
            })
            require.NoError(t, err)
            defer stream.Close()

            var gotDelta bool
            for event := range stream.Events() {
                if _, ok := event.(vai.ContentBlockDeltaEvent); ok {
                    gotDelta = true
                }
            }

            assert.True(t, gotDelta)
            assert.NoError(t, stream.Err())
        })
    }
}

func TestAllProviders_Tools(t *testing.T) {
    // Test tool definitions work across providers
    for _, p := range testProviders {
        t.Run(p.name, func(t *testing.T) {
            if os.Getenv(p.envKey) == "" {
                t.Skipf("%s not set", p.envKey)
            }

            resp, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
                Model: p.model,
                Messages: []vai.Message{
                    {Role: "user", Content: vai.Text("Get the weather in Paris")},
                },
                Tools: []vai.Tool{{
                    Type:        "function",
                    Name:        "get_weather",
                    Description: "Get weather",
                    InputSchema: &vai.JSONSchema{
                        Type: "object",
                        Properties: map[string]vai.JSONSchema{
                            "location": {Type: "string"},
                        },
                        Required: []string{"location"},
                    },
                }},
                ToolChoice: &vai.ToolChoice{Type: "tool", Name: "get_weather"},
                MaxTokens:  200,
            })

            require.NoError(t, err)
            assert.Equal(t, vai.StopReasonToolUse, resp.StopReason)
            assert.NotEmpty(t, resp.ToolUses())
        })
    }
}
```

---

## Acceptance Criteria

1. [ ] OpenAI provider passes all integration tests
2. [ ] Gemini provider passes all integration tests
3. [ ] Groq provider passes all integration tests
4. [ ] Request translation is correct for each provider
5. [ ] Response translation produces valid Vango format
6. [ ] Streaming works correctly for each provider
7. [ ] Tool definitions work across providers
8. [ ] Error mapping is correct for each provider
9. [ ] Provider-specific capabilities are reported correctly

---

## Files to Create

```
pkg/core/providers/openai/provider.go
pkg/core/providers/openai/client.go
pkg/core/providers/openai/request.go
pkg/core/providers/openai/response.go
pkg/core/providers/openai/stream.go
pkg/core/providers/openai/tools.go
pkg/core/providers/openai/errors.go
pkg/core/providers/openai/options.go
pkg/core/providers/gemini/provider.go
pkg/core/providers/gemini/client.go
pkg/core/providers/gemini/request.go
pkg/core/providers/gemini/response.go
pkg/core/providers/gemini/stream.go
pkg/core/providers/gemini/errors.go
pkg/core/providers/groq/provider.go
pkg/core/providers/groq/options.go
tests/integration/providers_test.go
```

---

## Estimated Effort

- OpenAI provider: ~800 lines
- Gemini provider: ~700 lines
- Groq provider: ~100 lines (extends OpenAI)
- Tests: ~400 lines
- **Total: ~2000 lines**

---

## Next Phase

Phase 7: Live Sessions - implements WebSocket-based real-time bidirectional voice and text.
