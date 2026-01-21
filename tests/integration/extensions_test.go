//go:build integration
// +build integration

package integration_test

import (
	"strings"
	"testing"
	"time"

	vai "github.com/vango-go/vai/sdk"
)

// ==================== Provider Extensions Tests ====================
// These tests verify provider-specific extensions work correctly.

// --- Anthropic Extensions ---

func TestExtension_Anthropic_ThinkingBudget(t *testing.T) {
	requireAnthropicKey(t)
	ctx := testContext(t, 60*time.Second)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "anthropic/claude-haiku-4-5-20251001",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("What is 5 + 7?")},
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

	// Verify response
	text := resp.TextContent()
	if !strings.Contains(text, "12") {
		t.Logf("Response: %s", text)
		t.Log("warning: expected answer 12")
	}

	// Check for thinking content
	thinking := resp.ThinkingContent()
	t.Logf("Thinking content length: %d", len(thinking))
}

func TestExtension_Anthropic_PromptCaching(t *testing.T) {
	requireAnthropicKey(t)
	ctx := testContext(t, 60*time.Second)

	// Long system prompt to test caching
	longSystemPrompt := strings.Repeat("You are a helpful assistant. ", 100)

	// First request - should create cache
	resp1, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model:  "anthropic/claude-haiku-4-5-20251001",
		System: longSystemPrompt,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Say hi")},
		},
		Extensions: map[string]any{
			"anthropic": map[string]any{
				"cache_control": map[string]any{
					"type": "ephemeral",
				},
			},
		},
		MaxTokens: 50,
	})
	if err != nil {
		t.Fatalf("first request error: %v", err)
	}

	// Second request - may use cache
	resp2, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model:  "anthropic/claude-haiku-4-5-20251001",
		System: longSystemPrompt,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Say hello")},
		},
		Extensions: map[string]any{
			"anthropic": map[string]any{
				"cache_control": map[string]any{
					"type": "ephemeral",
				},
			},
		},
		MaxTokens: 50,
	})
	if err != nil {
		t.Fatalf("second request error: %v", err)
	}

	t.Logf("First request usage: input=%d, output=%d",
		resp1.Usage.InputTokens, resp1.Usage.OutputTokens)
	t.Logf("Second request usage: input=%d, output=%d",
		resp2.Usage.InputTokens, resp2.Usage.OutputTokens)
}

// --- OpenAI Extensions ---

func TestExtension_OaiResp_ReasoningEffort(t *testing.T) {
	requireOpenAIKey(t)
	ctx := testContext(t, 90*time.Second)

	levels := []string{"low", "medium", "high"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
				Model: "oai-resp/gpt-5-mini",
				Messages: []vai.Message{
					{Role: "user", Content: vai.Text("What is 8 * 9?")},
				},
				Extensions: map[string]any{
					"openai": map[string]any{
						"reasoning": map[string]any{
							"effort": level,
						},
					},
				},
				MaxTokens: 500,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			text := resp.TextContent()
			if !strings.Contains(text, "72") {
				t.Logf("Response: %s", text)
				t.Log("warning: expected answer 72")
			}
		})
	}
}

func TestExtension_OaiResp_ReasoningSummary(t *testing.T) {
	requireOpenAIKey(t)
	ctx := testContext(t, 90*time.Second)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "oai-resp/gpt-5-mini",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("What is the square root of 144?")},
		},
		Extensions: map[string]any{
			"openai": map[string]any{
				"reasoning": map[string]any{
					"effort":  "medium",
					"summary": "concise",
				},
			},
		},
		MaxTokens: 500,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resp.TextContent()
	if !strings.Contains(text, "12") {
		t.Logf("Response: %s", text)
		t.Log("warning: expected answer 12")
	}
}

func TestExtension_OaiResp_Store(t *testing.T) {
	requireOpenAIKey(t)
	ctx := testContext(t, 60*time.Second)

	// Test the store parameter for conversation persistence
	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "oai-resp/gpt-5-mini",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Remember this: my favorite color is blue")},
		},
		Extensions: map[string]any{
			"openai": map[string]any{
				"store": true,
			},
		},
		MaxTokens: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Logf("Response ID: %s", resp.ID)
	t.Logf("Response: %s", resp.TextContent())
}

// --- Gemini Extensions ---

func TestExtension_Gemini_ThinkingLevel(t *testing.T) {
	requireGeminiKey(t)
	ctx := testContext(t, 90*time.Second)

	levels := []string{"low", "high"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
				Model: "gemini/gemini-3-flash-preview",
				Messages: []vai.Message{
					{Role: "user", Content: vai.Text("What is 7 * 8?")},
				},
				Extensions: map[string]any{
					"gemini": map[string]any{
						"thinking_level": level,
					},
				},
				MaxTokens: 500,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			text := resp.TextContent()
			if !strings.Contains(text, "56") {
				t.Logf("Response: %s", text)
				t.Log("warning: expected answer 56")
			}
		})
	}
}

func TestExtension_Gemini_ThinkingBudget(t *testing.T) {
	requireGeminiKey(t)
	ctx := testContext(t, 90*time.Second)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "gemini/gemini-3-flash-preview",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("What is 9 + 3?")},
		},
		Extensions: map[string]any{
			"gemini": map[string]any{
				"thinking_budget": 1000,
			},
		},
		MaxTokens: 500,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resp.TextContent()
	if !strings.Contains(text, "12") {
		t.Logf("Response: %s", text)
		t.Log("warning: expected answer 12")
	}
}

func TestExtension_Gemini_MediaResolution(t *testing.T) {
	requireGeminiKey(t)
	ctx := testContext(t, 60*time.Second)

	// Test media_resolution parameter for image processing
	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "gemini/gemini-3-flash-preview",
		Messages: []vai.Message{
			{Role: "user", Content: []vai.ContentBlock{
				vai.Text("What do you see in this image?"),
				vai.ImageURL("https://images.unsplash.com/photo-1514888286974-6c03e2ca1dba?w=200"),
			}},
		},
		Extensions: map[string]any{
			"gemini": map[string]any{
				"media_resolution": "low",
			},
		},
		MaxTokens: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resp.TextContent()
	if text == "" {
		t.Error("expected non-empty response")
	}
	t.Logf("Response: %s", truncate(text, 100))
}

func TestExtension_Gemini_ThoughtSignatures(t *testing.T) {
	requireGeminiKey(t)
	t.Skip("skipping thought signatures test - requires multi-turn implementation")

	/*
		// First turn with tool calling
		resp1, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: "gemini/gemini-3-flash-preview",
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("What's the weather in Tokyo?")},
			},
			Tools: []vai.Tool{
				{
					Type:        "function",
					Name:        "get_weather",
					Description: "Get weather",
					InputSchema: &vai.JSONSchema{
						Type: "object",
						Properties: map[string]vai.JSONSchema{
							"city": {Type: "string"},
						},
						Required: []string{"city"},
					},
				},
			},
			MaxTokens: 500,
		})
		if err != nil {
			t.Fatalf("first turn error: %v", err)
		}

		// Extract thought signature from response metadata
		var thoughtSig string
		if resp1.Metadata != nil {
			if sig, ok := resp1.Metadata["thought_signature"].(string); ok {
				thoughtSig = sig
			}
		}

		if thoughtSig == "" {
			t.Log("No thought signature in response (may be provider-specific)")
			return
		}

		// Second turn must include the signature
		resp2, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: "gemini/gemini-3-flash-preview",
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("What's the weather in Tokyo?")},
				{Role: "assistant", Content: resp1.Content},
				{Role: "user", Content: []vai.ContentBlock{
					vai.ToolResult(resp1.ToolUses()[0].ID, []vai.ContentBlock{
						vai.Text("Weather in Tokyo: 18°C, cloudy"),
					}),
				}},
			},
			Extensions: map[string]any{
				"gemini": map[string]any{
					"thought_signature": thoughtSig,
				},
			},
			MaxTokens: 500,
		})
		if err != nil {
			t.Fatalf("second turn error: %v", err)
		}

		if resp2.TextContent() == "" {
			t.Error("expected text response")
		}
	*/
}

// --- Groq Extensions ---

func TestExtension_Groq_ReasoningEffort(t *testing.T) {
	requireGroqKey(t)
	ctx := testContext(t, 60*time.Second)

	// Use GPT-OSS model for reasoning
	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "groq/openai/gpt-oss-120b",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("What is 6 * 7?")},
		},
		Extensions: map[string]any{
			"groq": map[string]any{
				"reasoning_effort": "high",
			},
		},
		MaxTokens: 500,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not available") || strings.Contains(err.Error(), "does not exist") {
			t.Skip("GPT-OSS model not available")
		}
		t.Fatalf("unexpected error: %v", err)
	}

	text := resp.TextContent()
	if !strings.Contains(text, "42") {
		t.Logf("Response: %s", text)
		t.Log("warning: expected answer 42")
	}
}

func TestExtension_Groq_IncludeReasoning(t *testing.T) {
	requireGroqKey(t)
	ctx := testContext(t, 60*time.Second)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "groq/openai/gpt-oss-120b",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("What is 10 / 2?")},
		},
		Extensions: map[string]any{
			"groq": map[string]any{
				"reasoning_effort":  "medium",
				"include_reasoning": true,
			},
		},
		MaxTokens: 500,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not available") || strings.Contains(err.Error(), "does not exist") {
			t.Skip("GPT-OSS model not available")
		}
		t.Fatalf("unexpected error: %v", err)
	}

	text := resp.TextContent()
	if !strings.Contains(text, "5") {
		t.Logf("Response: %s", text)
		t.Log("warning: expected answer 5")
	}

	// Check for reasoning content
	reasoning := resp.ThinkingContent()
	if reasoning != "" {
		t.Logf("Reasoning content: %s", truncate(reasoning, 200))
	}
}

// --- Cross-Provider Extension Tests ---

func TestExtension_UnknownExtension(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		testCtx := defaultTestContext(t)

		// Unknown extensions should be ignored or cause an error
		resp, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Say hi")},
			},
			Extensions: map[string]any{
				"unknown_provider": map[string]any{
					"unknown_option": true,
				},
			},
			MaxTokens: 50,
		})

		if err != nil {
			// Error is acceptable - provider may reject unknown extensions
			t.Logf("Error for unknown extension: %v", err)
			return
		}

		// If no error, request should have completed normally
		if resp.TextContent() == "" {
			t.Error("expected text response")
		}
	})
}

func TestExtension_EmptyExtensions(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		// Empty extensions map should work fine
		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Say hello")},
			},
			Extensions: map[string]any{},
			MaxTokens:  50,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.TextContent() == "" {
			t.Error("expected text response")
		}
	})
}
