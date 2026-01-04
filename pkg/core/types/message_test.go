package types

import (
	"encoding/json"
	"testing"
)

func TestMessage_StringContent(t *testing.T) {
	msg := Message{Role: "user", Content: "Hello!"}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Should produce {"role":"user","content":"Hello!"}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to unmarshal as map: %v", err)
	}

	if m["role"] != "user" {
		t.Errorf("Role mismatch: got %v", m["role"])
	}
	if m["content"] != "Hello!" {
		t.Errorf("Content mismatch: got %v", m["content"])
	}
}

func TestMessage_ContentBlocksArray(t *testing.T) {
	msg := Message{
		Role: "user",
		Content: []ContentBlock{
			TextBlock{Type: "text", Text: "What's in this image?"},
			ImageBlock{
				Type: "image",
				Source: ImageSource{
					Type: "url",
					URL:  "https://example.com/img.png",
				},
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to unmarshal as map: %v", err)
	}

	content, ok := m["content"].([]any)
	if !ok {
		t.Fatalf("Content should be array, got %T", m["content"])
	}
	if len(content) != 2 {
		t.Errorf("Expected 2 content blocks, got %d", len(content))
	}
}

func TestMessage_SingleContentBlock(t *testing.T) {
	msg := Message{
		Role:    "user",
		Content: TextBlock{Type: "text", Text: "Hello!"},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to unmarshal as map: %v", err)
	}

	// Single content block should be wrapped in array
	content, ok := m["content"].([]any)
	if !ok {
		t.Fatalf("Content should be array, got %T", m["content"])
	}
	if len(content) != 1 {
		t.Errorf("Expected 1 content block, got %d", len(content))
	}
}

func TestMessage_UnmarshalString(t *testing.T) {
	jsonData := `{"role":"user","content":"Hello!"}`

	var msg Message
	if err := json.Unmarshal([]byte(jsonData), &msg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if msg.Role != "user" {
		t.Errorf("Role mismatch: got %q", msg.Role)
	}
	if msg.Content != "Hello!" {
		t.Errorf("Content mismatch: got %v", msg.Content)
	}
}

func TestMessage_UnmarshalContentBlocks(t *testing.T) {
	jsonData := `{
		"role": "user",
		"content": [
			{"type": "text", "text": "What's in this image?"},
			{"type": "image", "source": {"type": "url", "url": "https://example.com/img.png"}}
		]
	}`

	var msg Message
	if err := json.Unmarshal([]byte(jsonData), &msg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if msg.Role != "user" {
		t.Errorf("Role mismatch: got %q", msg.Role)
	}

	blocks, ok := msg.Content.([]ContentBlock)
	if !ok {
		t.Fatalf("Content should be []ContentBlock, got %T", msg.Content)
	}
	if len(blocks) != 2 {
		t.Errorf("Expected 2 content blocks, got %d", len(blocks))
	}
}

func TestMessage_ContentBlocks_Method(t *testing.T) {
	tests := []struct {
		name    string
		content any
		want    int
	}{
		{
			name:    "string content",
			content: "Hello!",
			want:    1,
		},
		{
			name:    "single block",
			content: TextBlock{Type: "text", Text: "Hello!"},
			want:    1,
		},
		{
			name: "multiple blocks",
			content: []ContentBlock{
				TextBlock{Type: "text", Text: "A"},
				TextBlock{Type: "text", Text: "B"},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &Message{Role: "user", Content: tt.content}
			blocks := msg.ContentBlocks()
			if len(blocks) != tt.want {
				t.Errorf("Expected %d blocks, got %d", tt.want, len(blocks))
			}
		})
	}
}

func TestMessage_TextContent(t *testing.T) {
	tests := []struct {
		name    string
		content any
		want    string
	}{
		{
			name:    "string content",
			content: "Hello!",
			want:    "Hello!",
		},
		{
			name: "text blocks",
			content: []ContentBlock{
				TextBlock{Type: "text", Text: "Hello, "},
				TextBlock{Type: "text", Text: "world!"},
			},
			want: "Hello, world!",
		},
		{
			name: "mixed blocks",
			content: []ContentBlock{
				TextBlock{Type: "text", Text: "Check this: "},
				ImageBlock{Type: "image", Source: ImageSource{Type: "url", URL: "https://example.com"}},
			},
			want: "Check this: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &Message{Role: "user", Content: tt.content}
			if got := msg.TextContent(); got != tt.want {
				t.Errorf("TextContent() = %q, want %q", got, tt.want)
			}
		})
	}
}
