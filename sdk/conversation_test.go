package vai

import (
	"testing"

	"github.com/vango-go/vai/pkg/core/types"
)

func TestConversation_Basic(t *testing.T) {
	conv := NewConversation().
		WithSystem("You are a helpful assistant.").
		AddUserMessage("Hello!").
		AddAssistantMessage("Hi there!")

	if conv.Len() != 2 {
		t.Errorf("Len() = %d, want 2", conv.Len())
	}

	if conv.System != "You are a helpful assistant." {
		t.Errorf("System = %q, want %q", conv.System, "You are a helpful assistant.")
	}
}

func TestConversation_ToRequest(t *testing.T) {
	conv := NewConversation().
		WithSystem("You are helpful").
		AddUserMessage("Hello")

	req := conv.ToRequest("anthropic/claude-sonnet-4")

	if req.Model != "anthropic/claude-sonnet-4" {
		t.Errorf("Model = %q, want %q", req.Model, "anthropic/claude-sonnet-4")
	}
	if len(req.Messages) != 1 {
		t.Errorf("Messages length = %d, want 1", len(req.Messages))
	}
	if req.System != "You are helpful" {
		t.Errorf("System = %v, want %q", req.System, "You are helpful")
	}
}

func TestConversation_Clone(t *testing.T) {
	conv := NewConversation().AddUserMessage("Hello")
	clone := conv.Clone()

	// Modify clone
	clone.AddUserMessage("Another message")

	if conv.Len() != 1 {
		t.Error("Original conversation should not be modified")
	}
	if clone.Len() != 2 {
		t.Error("Clone should have 2 messages")
	}
}

func TestConversation_Clear(t *testing.T) {
	conv := NewConversation().
		AddUserMessage("Hello").
		AddAssistantMessage("Hi")

	conv.Clear()

	if conv.Len() != 0 {
		t.Errorf("After Clear(), Len() = %d, want 0", conv.Len())
	}
}

func TestConversation_LastMessage(t *testing.T) {
	conv := NewConversation().
		AddUserMessage("Hello").
		AddAssistantMessage("Hi")

	last := conv.LastMessage()
	if last == nil {
		t.Fatal("LastMessage() returned nil")
	}
	if last.Role != "assistant" {
		t.Errorf("LastMessage().Role = %q, want %q", last.Role, "assistant")
	}
}

func TestConversation_LastMessage_Empty(t *testing.T) {
	conv := NewConversation()
	if conv.LastMessage() != nil {
		t.Error("LastMessage() should return nil for empty conversation")
	}
}

func TestConversation_AddResponse(t *testing.T) {
	conv := NewConversation().AddUserMessage("Hello")

	resp := &Response{
		MessageResponse: &types.MessageResponse{
			Content: []types.ContentBlock{
				types.TextBlock{Type: "text", Text: "Hi there!"},
			},
		},
	}

	conv.AddResponse(resp)

	if conv.Len() != 2 {
		t.Errorf("Len() = %d, want 2", conv.Len())
	}

	last := conv.LastMessage()
	if last.Role != "assistant" {
		t.Errorf("LastMessage().Role = %q, want %q", last.Role, "assistant")
	}
}

func TestConversation_AddToolResult(t *testing.T) {
	conv := NewConversation().
		AddUserMessage("What's the weather?")

	conv.AddToolResult("call_123", []types.ContentBlock{
		types.TextBlock{Type: "text", Text: "It's sunny"},
	})

	if conv.Len() != 2 {
		t.Errorf("Len() = %d, want 2", conv.Len())
	}

	last := conv.LastMessage()
	if last.Role != "user" {
		t.Errorf("Tool result should be user message, got %q", last.Role)
	}
}
