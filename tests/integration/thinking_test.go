//go:build integration
// +build integration

package integration_test

import (
	"strings"
	"testing"
	"time"

	vai "github.com/vango-go/vai/sdk"
)

// ==================== Thinking/Reasoning Tests ====================
// These tests verify that thinking/reasoning output works correctly.

func TestThinking_Basic_Anthropic(t *testing.T) {
	requireAnthropicKey(t)
	ctx := testContext(t, 60*time.Second)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "anthropic/claude-haiku-4-5-20251001",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("What is 17 * 23? Think through this step by step.")},
		},
		Extensions: map[string]any{
			"anthropic": map[string]any{
				"thinking": map[string]any{
					"type":          "enabled",
					"budget_tokens": 1000,
				},
			},
		},
		MaxTokens: 2000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check for thinking content
	thinking := resp.ThinkingContent()
	if thinking != "" {
		t.Logf("Thinking content: %s", truncate(thinking, 200))
	} else {
		t.Log("No thinking content in response (may be in content blocks)")
	}

	// Should have the answer: 391
	text := resp.TextContent()
	if !strings.Contains(text, "391") {
		t.Logf("Response: %s", text)
		t.Log("warning: expected answer 391")
	}
}

func TestThinking_Streaming_Anthropic(t *testing.T) {
	requireAnthropicKey(t)
	ctx := testContext(t, 60*time.Second)

	stream, err := testClient.Messages.Stream(ctx, &vai.MessageRequest{
		Model: "anthropic/claude-haiku-4-5-20251001",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Solve: What is 42 divided by 6?")},
		},
		Extensions: map[string]any{
			"anthropic": map[string]any{
				"thinking": map[string]any{
					"type":          "enabled",
					"budget_tokens": 500,
				},
			},
		},
		MaxTokens: 1000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	var hasThinkingDelta bool
	for event := range stream.Events() {
		if delta, ok := event.(vai.ContentBlockDeltaEvent); ok {
			if _, ok := delta.Delta.(vai.ThinkingDelta); ok {
				hasThinkingDelta = true
			}
		}
	}

	if hasThinkingDelta {
		t.Log("Received thinking deltas in stream")
	} else {
		t.Log("No thinking deltas in stream (may be provider-specific)")
	}

	resp := stream.Response()
	if resp == nil {
		t.Fatal("stream.Response() is nil")
	}

	// Should have the answer: 7
	if !strings.Contains(resp.TextContent(), "7") {
		t.Logf("Response: %s", resp.TextContent())
		t.Log("warning: expected answer 7")
	}
}

func TestThinking_OaiResp_ReasoningEffort(t *testing.T) {
	requireOpenAIKey(t)
	ctx := testContext(t, 90*time.Second)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "oai-resp/gpt-5-mini",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("What is the sum of the first 10 prime numbers?")},
		},
		Extensions: map[string]any{
			"openai": map[string]any{
				"reasoning": map[string]any{
					"effort": "medium",
				},
			},
		},
		MaxTokens: 2000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The sum is 129 (2+3+5+7+11+13+17+19+23+29)
	text := resp.TextContent()
	if !strings.Contains(text, "129") {
		t.Logf("Response: %s", text)
		t.Log("warning: expected answer 129")
	}
}

func TestThinking_Gemini_ThinkingLevel(t *testing.T) {
	requireGeminiKey(t)
	ctx := testContext(t, 90*time.Second)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "gemini/gemini-3-flash-preview",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("What is 15 squared?")},
		},
		Extensions: map[string]any{
			"gemini": map[string]any{
				"thinking_level": "high",
			},
		},
		MaxTokens: 1000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have the answer: 225
	text := resp.TextContent()
	if !strings.Contains(text, "225") {
		t.Logf("Response: %s", text)
		t.Log("warning: expected answer 225")
	}

	// Check for thinking content
	thinking := resp.ThinkingContent()
	if thinking != "" {
		t.Logf("Thinking content: %s", truncate(thinking, 200))
	}
}

func TestThinking_Groq_ReasoningEffort(t *testing.T) {
	requireGroqKey(t)
	ctx := testContext(t, 60*time.Second)

	// Use GPT-OSS model for reasoning
	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "groq/openai/gpt-oss-120b",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("What is 8 cubed?")},
		},
		Extensions: map[string]any{
			"groq": map[string]any{
				"reasoning_effort":  "high",
				"include_reasoning": true,
			},
		},
		MaxTokens: 1000,
	})
	if err != nil {
		// Model may not be available, skip
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not available") || strings.Contains(err.Error(), "does not exist") {
			t.Skip("GPT-OSS model not available on Groq")
		}
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have the answer: 512
	text := resp.TextContent()
	if !strings.Contains(text, "512") {
		t.Logf("Response: %s", text)
		t.Log("warning: expected answer 512")
	}
}

func TestThinking_WithTools(t *testing.T) {
	requireAnthropicKey(t)
	ctx := testContext(t, 90*time.Second)

	// Combine thinking with tool use
	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "anthropic/claude-haiku-4-5-20251001",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("I need to calculate the area of a circle with radius 5. Use the calculator tool.")},
		},
		Tools: []vai.Tool{
			{
				Type:        "function",
				Name:        "calculator",
				Description: "Calculate mathematical expressions",
				InputSchema: &vai.JSONSchema{
					Type: "object",
					Properties: map[string]vai.JSONSchema{
						"expression": {Type: "string", Description: "Math expression to evaluate"},
					},
					Required: []string{"expression"},
				},
			},
		},
		Extensions: map[string]any{
			"anthropic": map[string]any{
				"thinking": map[string]any{
					"type":          "enabled",
					"budget_tokens": 500,
				},
			},
		},
		MaxTokens: 2000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check what content blocks we got
	hasThinking := resp.ThinkingContent() != ""
	hasToolUse := len(resp.ToolUses()) > 0
	hasText := resp.TextContent() != ""

	t.Logf("Has thinking: %v, Has tool use: %v, Has text: %v", hasThinking, hasToolUse, hasText)

	if hasToolUse {
		for _, tu := range resp.ToolUses() {
			t.Logf("Tool use: %s with input %v", tu.Name, tu.Input)
		}
	}
}

func TestThinking_InterleavedWithText(t *testing.T) {
	requireAnthropicKey(t)
	ctx := testContext(t, 60*time.Second)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "anthropic/claude-haiku-4-5-20251001",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Explain your reasoning, then give me the answer: What is 100 - 37?")},
		},
		Extensions: map[string]any{
			"anthropic": map[string]any{
				"thinking": map[string]any{
					"type":          "enabled",
					"budget_tokens": 500,
				},
			},
		},
		MaxTokens: 1000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have the answer: 63
	text := resp.TextContent()
	if !strings.Contains(text, "63") {
		t.Logf("Response: %s", text)
		t.Log("warning: expected answer 63")
	}

	// Log content block types
	for i, block := range resp.Content {
		t.Logf("Block %d: type=%s", i, block.BlockType())
	}
}

func TestThinking_ThinkingContent_Helper(t *testing.T) {
	requireAnthropicKey(t)
	ctx := testContext(t, 60*time.Second)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "anthropic/claude-haiku-4-5-20251001",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("What is 2 + 2?")},
		},
		Extensions: map[string]any{
			"anthropic": map[string]any{
				"thinking": map[string]any{
					"type":          "enabled",
					"budget_tokens": 200,
				},
			},
		},
		MaxTokens: 500,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test the ThinkingContent() helper method
	thinking := resp.ThinkingContent()
	t.Logf("ThinkingContent() returned: %q", truncate(thinking, 100))

	// TextContent should return only the text, not thinking
	text := resp.TextContent()
	if !strings.Contains(text, "4") {
		t.Logf("TextContent: %s", text)
		t.Log("warning: expected answer 4 in text content")
	}
}
