package types

import (
	"encoding/json"
)

// Message represents a single message in a conversation.
type Message struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content any    `json:"content"` // string or []ContentBlock
}

// MarshalJSON handles the flexible Content field.
// - string -> "string"
// - ContentBlock -> [ContentBlock]
// - []ContentBlock -> [ContentBlock...]
func (m Message) MarshalJSON() ([]byte, error) {
	type rawMessage struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}

	var content any

	switch c := m.Content.(type) {
	case string:
		// String content is passed as-is
		content = c
	case ContentBlock:
		// Single ContentBlock is wrapped in array
		content = []ContentBlock{c}
	case []ContentBlock:
		// Array of ContentBlocks is passed as-is
		content = c
	case []any:
		// Handle slice of any (e.g., from ContentBlocks helper)
		blocks := make([]ContentBlock, 0, len(c))
		for _, item := range c {
			if block, ok := item.(ContentBlock); ok {
				blocks = append(blocks, block)
			}
		}
		content = blocks
	default:
		// Try to marshal as-is (for pre-marshaled content)
		content = m.Content
	}

	return json.Marshal(rawMessage{
		Role:    m.Role,
		Content: content,
	})
}

// UnmarshalJSON handles flexible Content parsing.
func (m *Message) UnmarshalJSON(data []byte) error {
	type rawMessage struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}

	var raw rawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	m.Role = raw.Role

	// Try to parse as string first
	var str string
	if err := json.Unmarshal(raw.Content, &str); err == nil {
		m.Content = str
		return nil
	}

	// Try to parse as array of content blocks
	blocks, err := UnmarshalContentBlocks(raw.Content)
	if err != nil {
		return err
	}
	m.Content = blocks
	return nil
}

// ContentBlocks returns Content as []ContentBlock regardless of input type.
func (m *Message) ContentBlocks() []ContentBlock {
	switch c := m.Content.(type) {
	case string:
		return []ContentBlock{TextBlock{Type: "text", Text: c}}
	case ContentBlock:
		return []ContentBlock{c}
	case []ContentBlock:
		return c
	default:
		return nil
	}
}

// TextContent returns the text content of the message if it's a simple string,
// or concatenates all text blocks if it's an array.
func (m *Message) TextContent() string {
	switch c := m.Content.(type) {
	case string:
		return c
	case []ContentBlock:
		var text string
		for _, block := range c {
			if tb, ok := block.(TextBlock); ok {
				text += tb.Text
			}
			if tb, ok := block.(*TextBlock); ok {
				text += tb.Text
			}
		}
		return text
	default:
		return ""
	}
}
