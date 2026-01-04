package types

import (
	"encoding/json"
	"testing"
)

func TestMessageResponse_TextContent(t *testing.T) {
	resp := &MessageResponse{
		ID:   "msg_123",
		Type: "message",
		Role: "assistant",
		Content: []ContentBlock{
			TextBlock{Type: "text", Text: "Hello, "},
			TextBlock{Type: "text", Text: "world!"},
		},
		StopReason: StopReasonEndTurn,
	}

	text := resp.TextContent()
	if text != "Hello, world!" {
		t.Errorf("TextContent mismatch: got %q, want %q", text, "Hello, world!")
	}
}

func TestMessageResponse_ToolUses(t *testing.T) {
	resp := &MessageResponse{
		ID:   "msg_123",
		Type: "message",
		Role: "assistant",
		Content: []ContentBlock{
			TextBlock{Type: "text", Text: "I'll search for that."},
			ToolUseBlock{
				Type:  "tool_use",
				ID:    "call_123",
				Name:  "web_search",
				Input: map[string]any{"query": "weather in Tokyo"},
			},
		},
		StopReason: StopReasonToolUse,
	}

	uses := resp.ToolUses()
	if len(uses) != 1 {
		t.Fatalf("Expected 1 tool use, got %d", len(uses))
	}
	if uses[0].Name != "web_search" {
		t.Errorf("Tool name mismatch: got %q, want %q", uses[0].Name, "web_search")
	}
}

func TestMessageResponse_HasToolUse(t *testing.T) {
	withTool := &MessageResponse{
		Content: []ContentBlock{
			ToolUseBlock{Type: "tool_use", ID: "call_123", Name: "test", Input: nil},
		},
	}
	if !withTool.HasToolUse() {
		t.Error("Expected HasToolUse to return true")
	}

	withoutTool := &MessageResponse{
		Content: []ContentBlock{
			TextBlock{Type: "text", Text: "Hello"},
		},
	}
	if withoutTool.HasToolUse() {
		t.Error("Expected HasToolUse to return false")
	}
}

func TestMessageResponse_MarshalJSON(t *testing.T) {
	resp := &MessageResponse{
		ID:    "msg_123",
		Type:  "message",
		Role:  "assistant",
		Model: "anthropic/claude-sonnet-4",
		Content: []ContentBlock{
			TextBlock{Type: "text", Text: "Hello!"},
		},
		StopReason: StopReasonEndTurn,
		Usage: Usage{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Check that it contains expected fields
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to unmarshal as map: %v", err)
	}

	if m["id"] != "msg_123" {
		t.Errorf("ID mismatch in JSON: got %v", m["id"])
	}
	if m["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason mismatch in JSON: got %v", m["stop_reason"])
	}
}

func TestStopReason_Values(t *testing.T) {
	tests := []struct {
		reason StopReason
		want   string
	}{
		{StopReasonEndTurn, "end_turn"},
		{StopReasonMaxTokens, "max_tokens"},
		{StopReasonStopSequence, "stop_sequence"},
		{StopReasonToolUse, "tool_use"},
	}

	for _, tt := range tests {
		if string(tt.reason) != tt.want {
			t.Errorf("StopReason %v: got %q, want %q", tt.reason, string(tt.reason), tt.want)
		}
	}
}
