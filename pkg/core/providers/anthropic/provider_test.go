package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vango-go/vai/pkg/core/types"
)

func requireTCPListen(t testing.TB) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping test: TCP listen not permitted in this environment: %v", err)
	}
	ln.Close()
}

func TestProvider_Name(t *testing.T) {
	p := New("test-key")
	if got := p.Name(); got != "anthropic" {
		t.Errorf("Name() = %q, want %q", got, "anthropic")
	}
}

func TestProvider_Capabilities(t *testing.T) {
	p := New("test-key")
	caps := p.Capabilities()

	if !caps.Vision {
		t.Error("expected Vision to be true")
	}
	if !caps.Tools {
		t.Error("expected Tools to be true")
	}
	if !caps.ToolStreaming {
		t.Error("expected ToolStreaming to be true")
	}
	if !caps.Thinking {
		t.Error("expected Thinking to be true")
	}
	if !caps.StructuredOutput {
		t.Error("expected StructuredOutput to be true")
	}
	if caps.AudioInput {
		t.Error("expected AudioInput to be false")
	}
	if caps.AudioOutput {
		t.Error("expected AudioOutput to be false")
	}
	if caps.Video {
		t.Error("expected Video to be false")
	}
	if len(caps.NativeTools) != 4 {
		t.Errorf("expected 4 native tools, got %d", len(caps.NativeTools))
	}
}

func TestProvider_CreateMessage(t *testing.T) {
	requireTCPListen(t)
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected /v1/messages, got %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("expected X-API-Key header")
		}
		if r.Header.Get("anthropic-version") != APIVersion {
			t.Errorf("expected anthropic-version header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type: application/json")
		}

		// Verify request body
		var reqBody anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if reqBody.Model != "claude-sonnet-4" {
			t.Errorf("expected model claude-sonnet-4, got %s", reqBody.Model)
		}
		if reqBody.MaxTokens != 1024 {
			t.Errorf("expected max_tokens 1024, got %d", reqBody.MaxTokens)
		}

		// Return a mock response
		resp := map[string]any{
			"id":    "msg_123",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-sonnet-4",
			"content": []map[string]any{
				{"type": "text", "text": "Hello, World!"},
			},
			"stop_reason": "end_turn",
			"usage": map[string]int{
				"input_tokens":  10,
				"output_tokens": 5,
				"total_tokens":  15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create provider with test server
	p := New("test-key", WithBaseURL(server.URL))

	// Create request
	req := &types.MessageRequest{
		Model:     "claude-sonnet-4",
		MaxTokens: 1024,
		Messages: []types.Message{
			{Role: "user", Content: "Hello"},
		},
	}

	// Execute
	resp, err := p.CreateMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateMessage failed: %v", err)
	}

	// Verify response
	if resp.ID != "msg_123" {
		t.Errorf("expected ID msg_123, got %s", resp.ID)
	}
	if resp.Role != "assistant" {
		t.Errorf("expected role assistant, got %s", resp.Role)
	}
	if resp.Model != "anthropic/claude-sonnet-4" {
		t.Errorf("expected model anthropic/claude-sonnet-4, got %s", resp.Model)
	}
	if resp.StopReason != types.StopReasonEndTurn {
		t.Errorf("expected stop_reason end_turn, got %s", resp.StopReason)
	}
	if resp.TextContent() != "Hello, World!" {
		t.Errorf("expected text 'Hello, World!', got %s", resp.TextContent())
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("expected input_tokens 10, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 5 {
		t.Errorf("expected output_tokens 5, got %d", resp.Usage.OutputTokens)
	}
}

func TestProvider_CreateMessage_WithTools(t *testing.T) {
	requireTCPListen(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody anthropicRequest
		json.NewDecoder(r.Body).Decode(&reqBody)

		// Verify tools in request
		if len(reqBody.Tools) != 2 {
			t.Errorf("expected 2 tools, got %d", len(reqBody.Tools))
		}
		if reqBody.Tools[0].Type != "custom" {
			t.Errorf("expected tool type 'custom', got %s", reqBody.Tools[0].Type)
		}
		if reqBody.Tools[1].Type != "web_search_20250305" {
			t.Errorf("expected tool type 'web_search_20250305', got %s", reqBody.Tools[1].Type)
		}

		// Return tool_use response
		resp := map[string]any{
			"id":    "msg_456",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-sonnet-4",
			"content": []map[string]any{
				{
					"type":  "tool_use",
					"id":    "call_123",
					"name":  "get_weather",
					"input": map[string]any{"location": "Tokyo"},
				},
			},
			"stop_reason": "tool_use",
			"usage": map[string]int{
				"input_tokens":  20,
				"output_tokens": 10,
				"total_tokens":  30,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New("test-key", WithBaseURL(server.URL))

	req := &types.MessageRequest{
		Model:     "claude-sonnet-4",
		MaxTokens: 1024,
		Messages: []types.Message{
			{Role: "user", Content: "What's the weather in Tokyo?"},
		},
		Tools: []types.Tool{
			types.NewFunctionTool("get_weather", "Get the weather", &types.JSONSchema{
				Type: "object",
				Properties: map[string]types.JSONSchema{
					"location": {Type: "string"},
				},
			}),
			types.NewWebSearchTool(nil),
		},
	}

	resp, err := p.CreateMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateMessage failed: %v", err)
	}

	if resp.StopReason != types.StopReasonToolUse {
		t.Errorf("expected stop_reason tool_use, got %s", resp.StopReason)
	}
	if !resp.HasToolUse() {
		t.Error("expected HasToolUse to be true")
	}

	toolUses := resp.ToolUses()
	if len(toolUses) != 1 {
		t.Fatalf("expected 1 tool use, got %d", len(toolUses))
	}
	if toolUses[0].Name != "get_weather" {
		t.Errorf("expected tool name 'get_weather', got %s", toolUses[0].Name)
	}
}

func TestProvider_CreateMessage_Error(t *testing.T) {
	requireTCPListen(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    "invalid_request_error",
				"message": "max_tokens is required",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New("test-key", WithBaseURL(server.URL))

	req := &types.MessageRequest{
		Model: "claude-sonnet-4",
		Messages: []types.Message{
			{Role: "user", Content: "Hello"},
		},
		// Missing MaxTokens
	}

	_, err := p.CreateMessage(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if apiErr.Type != ErrInvalidRequest {
		t.Errorf("expected ErrInvalidRequest, got %s", apiErr.Type)
	}
	if apiErr.Message != "max_tokens is required" {
		t.Errorf("expected message 'max_tokens is required', got %s", apiErr.Message)
	}
}

func TestProvider_StreamMessage(t *testing.T) {
	requireTCPListen(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("expected Accept: text/event-stream")
		}

		var reqBody anthropicRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		if !reqBody.Stream {
			t.Error("expected stream=true in request")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		// Send message_start
		w.Write([]byte(`event: message_start
data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","model":"claude-sonnet-4","content":[],"stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}

`))
		flusher.Flush()

		// Send content_block_start
		w.Write([]byte(`event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

`))
		flusher.Flush()

		// Send text deltas
		w.Write([]byte(`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

`))
		flusher.Flush()

		w.Write([]byte(`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":", World!"}}

`))
		flusher.Flush()

		// Send content_block_stop
		w.Write([]byte(`event: content_block_stop
data: {"type":"content_block_stop","index":0}

`))
		flusher.Flush()

		// Send message_delta
		w.Write([]byte(`event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}

`))
		flusher.Flush()

		// Send message_stop
		w.Write([]byte(`event: message_stop
data: {"type":"message_stop"}

`))
		flusher.Flush()
	}))
	defer server.Close()

	p := New("test-key", WithBaseURL(server.URL))

	req := &types.MessageRequest{
		Model:     "claude-sonnet-4",
		MaxTokens: 1024,
		Messages: []types.Message{
			{Role: "user", Content: "Hello"},
		},
	}

	stream, err := p.StreamMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("StreamMessage failed: %v", err)
	}
	defer stream.Close()

	var events []types.StreamEvent
	for {
		event, err := stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("stream.Next() failed: %v", err)
		}
		if event != nil {
			events = append(events, event)
		}
	}

	// Verify we got expected events
	if len(events) < 4 {
		t.Errorf("expected at least 4 events, got %d", len(events))
	}

	// Check for message_start
	if _, ok := events[0].(types.MessageStartEvent); !ok {
		t.Errorf("expected MessageStartEvent first, got %T", events[0])
	}

	// Check for content_block_start
	if _, ok := events[1].(types.ContentBlockStartEvent); !ok {
		t.Errorf("expected ContentBlockStartEvent second, got %T", events[1])
	}
}

func TestBuildRequest_DefaultMaxTokens(t *testing.T) {
	req := &types.MessageRequest{
		Model: "claude-sonnet-4",
		Messages: []types.Message{
			{Role: "user", Content: "Hello"},
		},
		// MaxTokens not set
	}

	anthReq := buildRequest(req)
	if anthReq.MaxTokens != DefaultMaxTokens {
		t.Errorf("expected MaxTokens %d, got %d", DefaultMaxTokens, anthReq.MaxTokens)
	}
}

func TestBuildRequest_PreservesTemperature(t *testing.T) {
	temp := 0.5
	req := &types.MessageRequest{
		Model:       "claude-sonnet-4",
		MaxTokens:   100,
		Temperature: &temp,
		Messages: []types.Message{
			{Role: "user", Content: "Hello"},
		},
	}

	anthReq := buildRequest(req)
	if anthReq.Temperature == nil || *anthReq.Temperature != 0.5 {
		t.Error("expected temperature to be preserved")
	}
}

func TestConvertTools_Function(t *testing.T) {
	tools := []types.Tool{
		types.NewFunctionTool("test", "description", &types.JSONSchema{Type: "object"}),
	}

	result := convertTools(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Type != "custom" {
		t.Errorf("expected type 'custom', got %s", result[0].Type)
	}
	if result[0].Name != "test" {
		t.Errorf("expected name 'test', got %s", result[0].Name)
	}
}

func TestConvertTools_NativeTools(t *testing.T) {
	tests := []struct {
		toolType     string
		expectedType string
	}{
		{types.ToolTypeWebSearch, "web_search_20250305"},
		{types.ToolTypeCodeExecution, "code_execution_20250522"},
		{types.ToolTypeComputerUse, "computer_20250124"},
		{types.ToolTypeTextEditor, "text_editor_20250124"},
	}

	for _, tt := range tests {
		t.Run(tt.toolType, func(t *testing.T) {
			tools := []types.Tool{
				{Type: tt.toolType},
			}
			result := convertTools(tools)
			if len(result) != 1 {
				t.Fatalf("expected 1 tool, got %d", len(result))
			}
			if result[0].Type != tt.expectedType {
				t.Errorf("expected type %q, got %q", tt.expectedType, result[0].Type)
			}
		})
	}
}

func TestStripProviderPrefix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"anthropic/claude-sonnet-4", "claude-sonnet-4"},
		{"openai/gpt-4o", "gpt-4o"},
		{"claude-sonnet-4", "claude-sonnet-4"},
		{"", ""},
	}

	for _, tt := range tests {
		result := stripProviderPrefix(tt.input)
		if result != tt.expected {
			t.Errorf("stripProviderPrefix(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestError_Error(t *testing.T) {
	err := &Error{
		Type:    ErrInvalidRequest,
		Message: "Invalid model",
	}
	expected := "anthropic: invalid_request_error: Invalid model"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

func TestError_ErrorWithCode(t *testing.T) {
	err := &Error{
		Type:    ErrRateLimit,
		Message: "Too many requests",
		Code:    "rate_limit_exceeded",
	}
	expected := "anthropic: rate_limit_error: Too many requests (code: rate_limit_exceeded)"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

func TestError_IsRetryable(t *testing.T) {
	tests := []struct {
		errType   ErrorType
		retryable bool
	}{
		{ErrRateLimit, true},
		{ErrOverloaded, true},
		{ErrAPI, true},
		{ErrInvalidRequest, false},
		{ErrAuthentication, false},
		{ErrPermission, false},
		{ErrNotFound, false},
		{ErrProvider, false},
	}

	for _, tt := range tests {
		err := &Error{Type: tt.errType, Message: "test"}
		if err.IsRetryable() != tt.retryable {
			t.Errorf("%s IsRetryable() = %v, want %v", tt.errType, err.IsRetryable(), tt.retryable)
		}
	}
}
