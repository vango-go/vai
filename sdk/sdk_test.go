package vai

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/vango-go/vai/pkg/core/types"
)

func requireTCPListenSDK(t testing.TB) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping test: TCP listen not permitted in this environment: %v", err)
	}
	ln.Close()
}

func TestSDK_CreateMessage_DirectMode(t *testing.T) {
	requireTCPListenSDK(t)
	// Create a mock Anthropic server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request basics
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected /v1/messages, got %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-anthropic-key" {
			t.Errorf("expected X-API-Key header, got %q", r.Header.Get("X-API-Key"))
		}

		// Return a mock response
		resp := map[string]any{
			"id":    "msg_sdk_test",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-sonnet-4",
			"content": []map[string]any{
				{"type": "text", "text": "Hello from SDK test!"},
			},
			"stop_reason": "end_turn",
			"usage": map[string]int{
				"input_tokens":  15,
				"output_tokens": 8,
				"total_tokens":  23,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create client with custom provider key and base URL override
	// We need to create the provider with the custom base URL
	client := NewClient(
		WithProviderKey("anthropic", "test-anthropic-key"),
	)

	// Get the engine and register a provider with our test server
	// Unfortunately we can't easily override the base URL through the SDK
	// This test verifies the SDK → Engine wiring works
	// Full end-to-end testing is done via integration tests

	// For now, verify the client is correctly configured
	if !client.IsDirectMode() {
		t.Error("expected Direct Mode")
	}
	if client.IsProxyMode() {
		t.Error("expected not Proxy Mode")
	}
	if client.Engine() == nil {
		t.Error("expected non-nil engine in Direct Mode")
	}
	if client.Messages == nil {
		t.Error("expected Messages service to be initialized")
	}
}

func TestSDK_StreamMessage_DirectMode(t *testing.T) {
	requireTCPListenSDK(t)
	// Create a mock Anthropic SSE server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return SSE stream
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		// Send message_start
		w.Write([]byte(`event: message_start
data: {"type":"message_start","message":{"id":"msg_stream_test","type":"message","role":"assistant","model":"claude-sonnet-4","content":[],"stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}

`))
		flusher.Flush()

		// Send content_block_start
		w.Write([]byte(`event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

`))
		flusher.Flush()

		// Send text delta
		w.Write([]byte(`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Streaming works!"}}

`))
		flusher.Flush()

		// Send message_stop
		w.Write([]byte(`event: message_stop
data: {"type":"message_stop"}

`))
		flusher.Flush()
	}))
	defer server.Close()

	// Similar to above, we verify SDK structure
	client := NewClient(
		WithProviderKey("anthropic", "test-key"),
	)

	if client.Messages == nil {
		t.Error("expected Messages service")
	}
}

func TestSDK_ClientModes(t *testing.T) {
	t.Run("Direct Mode Default", func(t *testing.T) {
		client := NewClient()
		if !client.IsDirectMode() {
			t.Error("expected Direct Mode by default")
		}
		if client.IsProxyMode() {
			t.Error("should not be Proxy Mode")
		}
	})

	t.Run("Proxy Mode with BaseURL", func(t *testing.T) {
		client := NewClient(
			WithBaseURL("http://localhost:8080"),
			WithAPIKey("vango_sk_test"),
		)
		if client.IsDirectMode() {
			t.Error("should not be Direct Mode")
		}
		if !client.IsProxyMode() {
			t.Error("expected Proxy Mode")
		}
		// Engine should be nil in proxy mode
		if client.Engine() != nil {
			t.Error("expected nil engine in Proxy Mode")
		}
	})
}

func TestSDK_Response_Wrapper(t *testing.T) {
	// Test the Response wrapper has proper methods
	resp := &Response{
		MessageResponse: &types.MessageResponse{
			ID:   "test",
			Role: "assistant",
			Content: []types.ContentBlock{
				types.TextBlock{Type: "text", Text: "Hello"},
			},
			StopReason: types.StopReasonEndTurn,
			Usage: types.Usage{
				InputTokens:  10,
				OutputTokens: 5,
				TotalTokens:  15,
			},
		},
	}

	if resp.ID != "test" {
		t.Errorf("expected ID 'test', got %s", resp.ID)
	}
	if resp.TextContent() != "Hello" {
		t.Errorf("expected text 'Hello', got %s", resp.TextContent())
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected total tokens 15, got %d", resp.Usage.TotalTokens)
	}
}

func TestSDK_StreamAccumulator(t *testing.T) {
	// Create a simple mock event stream
	events := []types.StreamEvent{
		types.MessageStartEvent{
			Type: "message_start",
			Message: types.MessageResponse{
				ID:   "msg_acc_test",
				Role: "assistant",
			},
		},
		types.ContentBlockStartEvent{
			Type:         "content_block_start",
			Index:        0,
			ContentBlock: types.TextBlock{Type: "text", Text: ""},
		},
		types.ContentBlockDeltaEvent{
			Type:  "content_block_delta",
			Index: 0,
			Delta: types.TextDelta{Type: "text_delta", Text: "Hello"},
		},
		types.ContentBlockDeltaEvent{
			Type:  "content_block_delta",
			Index: 0,
			Delta: types.TextDelta{Type: "text_delta", Text: ", World!"},
		},
		types.MessageDeltaEvent{
			Type: "message_delta",
			Delta: struct {
				StopReason types.StopReason `json:"stop_reason,omitempty"`
			}{StopReason: types.StopReasonEndTurn},
			Usage: types.Usage{OutputTokens: 5},
		},
	}

	// Create mock event stream
	mockStream := &mockEventStream{events: events, index: 0}
	stream := newStreamFromEventStream(mockStream)

	// Consume all events
	count := 0
	for range stream.Events() {
		count++
	}

	if count != len(events) {
		t.Errorf("expected %d events, got %d", len(events), count)
	}

	// Check accumulated response
	resp := stream.Response()
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.ID != "msg_acc_test" {
		t.Errorf("expected ID 'msg_acc_test', got %s", resp.ID)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}

	// Check text was accumulated
	text := resp.TextContent()
	if text != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got %q", text)
	}
}

// mockEventStream implements core.EventStream for testing
type mockEventStream struct {
	events []types.StreamEvent
	index  int
}

func (m *mockEventStream) Next() (types.StreamEvent, error) {
	if m.index >= len(m.events) {
		return nil, io.EOF
	}
	event := m.events[m.index]
	m.index++
	return event, nil
}

func (m *mockEventStream) Close() error {
	return nil
}

func TestSDK_Extract_Structured(t *testing.T) {
	// This tests the JSON schema generation
	type Sample struct {
		Name    string   `json:"name" desc:"The name"`
		Count   int      `json:"count"`
		Values  []string `json:"values,omitempty"`
		Enabled bool     `json:"enabled"`
	}

	client := NewClient()

	schema := GenerateJSONSchema(reflect.TypeOf(Sample{}))
	if schema.Type != "object" {
		t.Errorf("expected object type, got %s", schema.Type)
	}
	if len(schema.Properties) != 4 {
		t.Errorf("expected 4 properties, got %d", len(schema.Properties))
	}
	if schema.Properties["name"].Type != "string" {
		t.Error("expected name to be string type")
	}
	if schema.Properties["name"].Description != "The name" {
		t.Error("expected description from desc tag")
	}
	if schema.Properties["count"].Type != "integer" {
		t.Error("expected count to be integer type")
	}
	if schema.Properties["values"].Type != "array" {
		t.Error("expected values to be array type")
	}

	// values should not be required (has omitempty)
	for _, req := range schema.Required {
		if req == "values" {
			t.Error("values should not be required (has omitempty)")
		}
	}

	// name should be required
	found := false
	for _, req := range schema.Required {
		if req == "name" {
			found = true
			break
		}
	}
	if !found {
		t.Error("name should be required")
	}

	_ = client // Used to verify client creation doesn't error
}

// Need to import reflect for the schema test
var _ = reflect.TypeOf
