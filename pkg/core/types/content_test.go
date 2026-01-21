package types

import (
	"encoding/json"
	"testing"
)

func TestTextBlock_MarshalJSON(t *testing.T) {
	block := TextBlock{Type: "text", Text: "Hello, world!"}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	expected := `{"type":"text","text":"Hello, world!"}`
	if string(data) != expected {
		t.Errorf("JSON mismatch: got %s, want %s", string(data), expected)
	}
}

func TestImageBlock_MarshalJSON(t *testing.T) {
	block := ImageBlock{
		Type: "image",
		Source: ImageSource{
			Type:      "base64",
			MediaType: "image/png",
			Data:      "iVBORw0KGgo=",
		},
	}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to unmarshal as map: %v", err)
	}

	if m["type"] != "image" {
		t.Errorf("Type mismatch: got %v", m["type"])
	}
	source := m["source"].(map[string]any)
	if source["type"] != "base64" {
		t.Errorf("Source type mismatch: got %v", source["type"])
	}
}

func TestImageBlock_URL(t *testing.T) {
	block := ImageBlock{
		Type: "image",
		Source: ImageSource{
			Type: "url",
			URL:  "https://example.com/image.png",
		},
	}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to unmarshal as map: %v", err)
	}

	source := m["source"].(map[string]any)
	if source["url"] != "https://example.com/image.png" {
		t.Errorf("URL mismatch: got %v", source["url"])
	}
}

func TestToolUseBlock_MarshalJSON(t *testing.T) {
	block := ToolUseBlock{
		Type:  "tool_use",
		ID:    "call_123",
		Name:  "get_weather",
		Input: map[string]any{"location": "Tokyo"},
	}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var unmarshaled ToolUseBlock
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if unmarshaled.ID != "call_123" {
		t.Errorf("ID mismatch: got %q, want %q", unmarshaled.ID, "call_123")
	}
	if unmarshaled.Name != "get_weather" {
		t.Errorf("Name mismatch: got %q, want %q", unmarshaled.Name, "get_weather")
	}
}

func TestToolResultBlock_MarshalJSON(t *testing.T) {
	block := ToolResultBlock{
		Type:      "tool_result",
		ToolUseID: "call_123",
		Content: []ContentBlock{
			TextBlock{Type: "text", Text: "The weather is sunny."},
		},
	}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to unmarshal as map: %v", err)
	}

	if m["type"] != "tool_result" {
		t.Errorf("Type mismatch: got %v", m["type"])
	}
	if m["tool_use_id"] != "call_123" {
		t.Errorf("ToolUseID mismatch: got %v", m["tool_use_id"])
	}
}

func TestToolResultBlock_WithError(t *testing.T) {
	block := ToolResultBlock{
		Type:      "tool_result",
		ToolUseID: "call_123",
		Content: []ContentBlock{
			TextBlock{Type: "text", Text: "Error: location not found"},
		},
		IsError: true,
	}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to unmarshal as map: %v", err)
	}

	if m["is_error"] != true {
		t.Errorf("is_error should be true, got %v", m["is_error"])
	}
}

func TestUnmarshalContentBlock(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		wantType string
	}{
		{
			name:     "text block",
			json:     `{"type":"text","text":"Hello"}`,
			wantType: "text",
		},
		{
			name:     "image block",
			json:     `{"type":"image","source":{"type":"url","url":"https://example.com/img.png"}}`,
			wantType: "image",
		},
		{
			name:     "tool use block",
			json:     `{"type":"tool_use","id":"call_123","name":"test","input":{}}`,
			wantType: "tool_use",
		},
		{
			name:     "thinking block",
			json:     `{"type":"thinking","thinking":"Let me think..."}`,
			wantType: "thinking",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block, err := UnmarshalContentBlock([]byte(tt.json))
			if err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}
			if block.BlockType() != tt.wantType {
				t.Errorf("Type mismatch: got %q, want %q", block.BlockType(), tt.wantType)
			}
		})
	}
}

func TestUnmarshalContentBlocks(t *testing.T) {
	json := `[
		{"type":"text","text":"Hello"},
		{"type":"tool_use","id":"call_123","name":"test","input":{}}
	]`

	blocks, err := UnmarshalContentBlocks([]byte(json))
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(blocks) != 2 {
		t.Fatalf("Expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].BlockType() != "text" {
		t.Errorf("First block type mismatch: got %q", blocks[0].BlockType())
	}
	if blocks[1].BlockType() != "tool_use" {
		t.Errorf("Second block type mismatch: got %q", blocks[1].BlockType())
	}
}

func TestContentBlock_Interface(t *testing.T) {
	// Verify all types implement ContentBlock
	var _ ContentBlock = TextBlock{}
	var _ ContentBlock = ImageBlock{}
	var _ ContentBlock = AudioBlock{}
	var _ ContentBlock = VideoBlock{}
	var _ ContentBlock = DocumentBlock{}
	var _ ContentBlock = ToolResultBlock{}
	var _ ContentBlock = ToolUseBlock{}
	var _ ContentBlock = ThinkingBlock{}
}
