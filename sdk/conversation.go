package vai

import (
	"github.com/vango-go/vai/pkg/core/types"
)

// Conversation is a helper for managing multi-turn conversations.
type Conversation struct {
	Messages []types.Message
	System   any // string or []types.ContentBlock
}

// NewConversation creates a new conversation.
func NewConversation() *Conversation {
	return &Conversation{
		Messages: []types.Message{},
	}
}

// WithSystem sets the system prompt for the conversation.
func (c *Conversation) WithSystem(system string) *Conversation {
	c.System = system
	return c
}

// WithSystemBlocks sets the system prompt as content blocks.
func (c *Conversation) WithSystemBlocks(blocks []types.ContentBlock) *Conversation {
	c.System = blocks
	return c
}

// AddUserMessage adds a user message to the conversation.
func (c *Conversation) AddUserMessage(content any) *Conversation {
	c.Messages = append(c.Messages, types.Message{
		Role:    "user",
		Content: content,
	})
	return c
}

// AddAssistantMessage adds an assistant message to the conversation.
func (c *Conversation) AddAssistantMessage(content any) *Conversation {
	c.Messages = append(c.Messages, types.Message{
		Role:    "assistant",
		Content: content,
	})
	return c
}

// AddMessage adds a message to the conversation.
func (c *Conversation) AddMessage(role string, content any) *Conversation {
	c.Messages = append(c.Messages, types.Message{
		Role:    role,
		Content: content,
	})
	return c
}

// AddResponse adds an assistant response to the conversation.
func (c *Conversation) AddResponse(resp *Response) *Conversation {
	if resp != nil && resp.MessageResponse != nil {
		c.Messages = append(c.Messages, types.Message{
			Role:    "assistant",
			Content: resp.Content,
		})
	}
	return c
}

// AddToolResult adds a tool result to the conversation.
func (c *Conversation) AddToolResult(toolUseID string, result []types.ContentBlock) *Conversation {
	// Tool results are added as a user message with the tool_result block
	c.Messages = append(c.Messages, types.Message{
		Role: "user",
		Content: []types.ContentBlock{
			types.ToolResultBlock{
				Type:      "tool_result",
				ToolUseID: toolUseID,
				Content:   result,
			},
		},
	})
	return c
}

// ToRequest creates a MessageRequest from the conversation.
func (c *Conversation) ToRequest(model string) *types.MessageRequest {
	return &types.MessageRequest{
		Model:    model,
		Messages: c.Messages,
		System:   c.System,
	}
}

// Clone creates a copy of the conversation.
func (c *Conversation) Clone() *Conversation {
	clone := &Conversation{
		Messages: make([]types.Message, len(c.Messages)),
		System:   c.System,
	}
	copy(clone.Messages, c.Messages)
	return clone
}

// Clear removes all messages from the conversation.
func (c *Conversation) Clear() *Conversation {
	c.Messages = []types.Message{}
	return c
}

// LastMessage returns the last message in the conversation.
func (c *Conversation) LastMessage() *types.Message {
	if len(c.Messages) == 0 {
		return nil
	}
	return &c.Messages[len(c.Messages)-1]
}

// Len returns the number of messages in the conversation.
func (c *Conversation) Len() int {
	return len(c.Messages)
}
