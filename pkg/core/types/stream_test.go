package types

import (
	"encoding/json"
	"testing"
)

func TestStreamEvent_Types(t *testing.T) {
	tests := []struct {
		event    StreamEvent
		wantType string
	}{
		{MessageStartEvent{Type: "message_start"}, "message_start"},
		{ContentBlockStartEvent{Type: "content_block_start"}, "content_block_start"},
		{ContentBlockDeltaEvent{Type: "content_block_delta"}, "content_block_delta"},
		{ContentBlockStopEvent{Type: "content_block_stop"}, "content_block_stop"},
		{MessageDeltaEvent{Type: "message_delta"}, "message_delta"},
		{MessageStopEvent{Type: "message_stop"}, "message_stop"},
		{PingEvent{Type: "ping"}, "ping"},
		{AudioDeltaEvent{Type: "audio_delta"}, "audio_delta"},
		{TranscriptDeltaEvent{Type: "transcript_delta"}, "transcript_delta"},
		{ErrorEvent{Type: "error"}, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.wantType, func(t *testing.T) {
			if got := tt.event.EventType(); got != tt.wantType {
				t.Errorf("EventType() = %q, want %q", got, tt.wantType)
			}
		})
	}
}

func TestDelta_Types(t *testing.T) {
	tests := []struct {
		delta    Delta
		wantType string
	}{
		{TextDelta{Type: "text_delta"}, "text_delta"},
		{InputJSONDelta{Type: "input_json_delta"}, "input_json_delta"},
		{ThinkingDelta{Type: "thinking_delta"}, "thinking_delta"},
	}

	for _, tt := range tests {
		t.Run(tt.wantType, func(t *testing.T) {
			if got := tt.delta.DeltaType(); got != tt.wantType {
				t.Errorf("DeltaType() = %q, want %q", got, tt.wantType)
			}
		})
	}
}

func TestUnmarshalStreamEvent_MessageStart(t *testing.T) {
	jsonData := `{
		"type": "message_start",
		"message": {
			"id": "msg_123",
			"type": "message",
			"role": "assistant",
			"content": [],
			"stop_reason": ""
		}
	}`

	event, err := UnmarshalStreamEvent([]byte(jsonData))
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if event.EventType() != "message_start" {
		t.Errorf("EventType() = %q, want %q", event.EventType(), "message_start")
	}

	ms, ok := event.(MessageStartEvent)
	if !ok {
		t.Fatalf("Expected MessageStartEvent, got %T", event)
	}
	if ms.Message.ID != "msg_123" {
		t.Errorf("Message.ID = %q, want %q", ms.Message.ID, "msg_123")
	}
}

func TestUnmarshalStreamEvent_ContentBlockStart(t *testing.T) {
	jsonData := `{
		"type": "content_block_start",
		"index": 0,
		"content_block": {
			"type": "text",
			"text": ""
		}
	}`

	event, err := UnmarshalStreamEvent([]byte(jsonData))
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	cbs, ok := event.(ContentBlockStartEvent)
	if !ok {
		t.Fatalf("Expected ContentBlockStartEvent, got %T", event)
	}
	if cbs.Index != 0 {
		t.Errorf("Index = %d, want 0", cbs.Index)
	}
	if cbs.ContentBlock.BlockType() != "text" {
		t.Errorf("ContentBlock type = %q, want %q", cbs.ContentBlock.BlockType(), "text")
	}
}

func TestUnmarshalStreamEvent_ContentBlockDelta(t *testing.T) {
	jsonData := `{
		"type": "content_block_delta",
		"index": 0,
		"delta": {
			"type": "text_delta",
			"text": "Hello"
		}
	}`

	event, err := UnmarshalStreamEvent([]byte(jsonData))
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	cbd, ok := event.(ContentBlockDeltaEvent)
	if !ok {
		t.Fatalf("Expected ContentBlockDeltaEvent, got %T", event)
	}
	if cbd.Index != 0 {
		t.Errorf("Index = %d, want 0", cbd.Index)
	}

	textDelta, ok := cbd.Delta.(TextDelta)
	if !ok {
		t.Fatalf("Expected TextDelta, got %T", cbd.Delta)
	}
	if textDelta.Text != "Hello" {
		t.Errorf("Text = %q, want %q", textDelta.Text, "Hello")
	}
}

func TestUnmarshalStreamEvent_MessageDelta(t *testing.T) {
	jsonData := `{
		"type": "message_delta",
		"delta": {
			"stop_reason": "end_turn"
		},
		"usage": {
			"input_tokens": 100,
			"output_tokens": 50,
			"total_tokens": 150
		}
	}`

	event, err := UnmarshalStreamEvent([]byte(jsonData))
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	md, ok := event.(MessageDeltaEvent)
	if !ok {
		t.Fatalf("Expected MessageDeltaEvent, got %T", event)
	}
	if md.Delta.StopReason != StopReasonEndTurn {
		t.Errorf("StopReason = %q, want %q", md.Delta.StopReason, StopReasonEndTurn)
	}
	if md.Usage.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150", md.Usage.TotalTokens)
	}
}

func TestUnmarshalDelta(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		wantType string
	}{
		{
			name:     "text_delta",
			json:     `{"type":"text_delta","text":"Hello"}`,
			wantType: "text_delta",
		},
		{
			name:     "input_json_delta",
			json:     `{"type":"input_json_delta","partial_json":"{\"foo\":"}`,
			wantType: "input_json_delta",
		},
		{
			name:     "thinking_delta",
			json:     `{"type":"thinking_delta","thinking":"Let me think..."}`,
			wantType: "thinking_delta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delta, err := UnmarshalDelta([]byte(tt.json))
			if err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}
			if delta.DeltaType() != tt.wantType {
				t.Errorf("DeltaType() = %q, want %q", delta.DeltaType(), tt.wantType)
			}
		})
	}
}

func TestTextDelta_MarshalJSON(t *testing.T) {
	delta := TextDelta{Type: "text_delta", Text: "Hello, world!"}
	data, err := json.Marshal(delta)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var unmarshaled TextDelta
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if unmarshaled.Text != "Hello, world!" {
		t.Errorf("Text = %q, want %q", unmarshaled.Text, "Hello, world!")
	}
}
