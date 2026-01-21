//go:build integration
// +build integration

package integration_test

import (
	"strings"
	"testing"
	"time"

	vai "github.com/vango-go/vai/sdk"
)

// ==================== Native Tools Tests ====================
// These tests verify that native/built-in tools work correctly across providers.
// Native tools are executed server-side by the provider, not by the client.

// supportsNativeTool checks if a provider supports a specific native tool.
func supportsNativeTool(p providerConfig, tool string) bool {
	for _, t := range p.NativeTools {
		if t == tool {
			return true
		}
	}
	return false
}

func TestNativeTool_WebSearch_Anthropic(t *testing.T) {
	requireAnthropicKey(t)
	ctx := testContext(t, 60*time.Second)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "anthropic/claude-haiku-4-5-20251001",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Search the web for the current weather in San Francisco")},
		},
		Tools:     []vai.Tool{vai.WebSearch()},
		MaxTokens: 1000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have used web search and returned results
	text := resp.TextContent()
	if text == "" {
		t.Error("expected text content in response")
	}

	// Look for web search related content blocks
	for _, block := range resp.Content {
		t.Logf("Content block type: %s", block.BlockType())
	}

	t.Logf("Response: %s", truncate(text, 200))
}

func TestNativeTool_WebSearch_OaiResp(t *testing.T) {
	requireOpenAIKey(t)
	ctx := testContext(t, 60*time.Second)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "oai-resp/gpt-5-mini",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Search the web for the current weather in London")},
		},
		Tools:     []vai.Tool{vai.WebSearch()},
		MaxTokens: 1000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resp.TextContent()
	if text == "" {
		t.Error("expected text content in response")
	}

	t.Logf("Response: %s", truncate(text, 200))
}

func TestNativeTool_WebSearch_Gemini(t *testing.T) {
	requireGeminiKey(t)
	ctx := testContext(t, 60*time.Second)

	// Gemini uses "google_search" instead of generic "web_search"
	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "gemini/gemini-3-flash-preview",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Search the web for today's news headlines")},
		},
		Tools:     []vai.Tool{vai.WebSearch()},
		MaxTokens: 1000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resp.TextContent()
	if text == "" {
		t.Error("expected text content in response")
	}

	t.Logf("Response: %s", truncate(text, 200))
}

func TestNativeTool_WebSearch_Groq(t *testing.T) {
	requireGroqKey(t)
	ctx := testContext(t, 60*time.Second)

	// Groq uses Tavily for web search
	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "groq/moonshotai/kimi-k2-instruct-0905",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Search the web for recent tech news")},
		},
		Tools:     []vai.Tool{vai.WebSearch()},
		MaxTokens: 1000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resp.TextContent()
	if text == "" {
		t.Error("expected text content in response")
	}

	t.Logf("Response: %s", truncate(text, 200))
}

func TestNativeTool_WebSearch_WithConfig(t *testing.T) {
	requireAnthropicKey(t)
	ctx := testContext(t, 60*time.Second)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "anthropic/claude-haiku-4-5-20251001",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Search for Python documentation")},
		},
		Tools: []vai.Tool{
			vai.WebSearch(vai.WebSearchConfig{
				MaxUses:        3,
				AllowedDomains: []string{"python.org", "docs.python.org"},
			}),
		},
		MaxTokens: 1000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resp.TextContent()
	if text == "" {
		t.Error("expected text content in response")
	}

	// Results should be from python.org
	if !strings.Contains(strings.ToLower(text), "python") {
		t.Logf("Response: %s", text)
		t.Log("warning: expected Python-related content")
	}
}

func TestNativeTool_CodeExecution_Anthropic(t *testing.T) {
	requireAnthropicKey(t)
	ctx := testContext(t, 120*time.Second) // Code execution may take longer

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "anthropic/claude-haiku-4-5-20251001",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Calculate the factorial of 10 using Python code")},
		},
		Tools:     []vai.Tool{vai.CodeExecution()},
		MaxTokens: 2000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resp.TextContent()
	// Should contain the result: 3628800
	if !strings.Contains(text, "3628800") {
		t.Logf("Response: %s", text)
		t.Log("warning: expected factorial result 3628800")
	}
}

func TestNativeTool_CodeExecution_Gemini(t *testing.T) {
	requireGeminiKey(t)
	ctx := testContext(t, 120*time.Second)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "gemini/gemini-3-flash-preview",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Calculate the sum of numbers from 1 to 100 using code")},
		},
		Tools:     []vai.Tool{vai.CodeExecution()},
		MaxTokens: 2000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resp.TextContent()
	// Should contain the result: 5050
	if !strings.Contains(text, "5050") {
		t.Logf("Response: %s", text)
		t.Log("warning: expected sum result 5050")
	}
}

func TestNativeTool_CodeExecution_OaiResp(t *testing.T) {
	requireOpenAIKey(t)
	ctx := testContext(t, 120*time.Second)

	// OAI uses "code_interpreter"
	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "oai-resp/gpt-5-mini",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Calculate 2^10 using code")},
		},
		Tools:     []vai.Tool{vai.CodeExecution()},
		MaxTokens: 2000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resp.TextContent()
	// Should contain the result: 1024
	if !strings.Contains(text, "1024") {
		t.Logf("Response: %s", text)
		t.Log("warning: expected result 1024")
	}
}

func TestNativeTool_ComputerUse_Anthropic(t *testing.T) {
	requireAnthropicKey(t)
	t.Skip("skipping computer use test - requires special setup")

	/*
		ctx := testContext(t, 60*time.Second)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: "anthropic/claude-sonnet-4-20250514",
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Take a screenshot")},
			},
			Tools: []vai.Tool{
				vai.ComputerUse(1920, 1080),
			},
			MaxTokens: 1000,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Check for computer_use tool call
		toolUses := resp.ToolUses()
		if len(toolUses) == 0 {
			t.Error("expected computer_use tool call")
		}
	*/
}

func TestNativeTool_TextEditor_Anthropic(t *testing.T) {
	requireAnthropicKey(t)
	t.Skip("skipping text editor test - requires special setup")

	/*
		ctx := testContext(t, 60*time.Second)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: "anthropic/claude-sonnet-4-20250514",
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("View the current file")},
			},
			Tools: []vai.Tool{
				vai.TextEditor(),
			},
			MaxTokens: 1000,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Check for text_editor tool call
		toolUses := resp.ToolUses()
		if len(toolUses) == 0 {
			t.Error("expected text_editor tool call")
		}
	*/
}

func TestNativeTool_FileSearch_OaiResp(t *testing.T) {
	requireOpenAIKey(t)
	t.Skip("skipping file search test - requires uploaded files")

	/*
		ctx := testContext(t, 60*time.Second)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: "oai-resp/gpt-5-mini",
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Search my files for information about quarterly revenue")},
			},
			Tools: []vai.Tool{
				vai.FileSearch(),
			},
			MaxTokens: 1000,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		text := resp.TextContent()
		if text == "" {
			t.Error("expected response text")
		}
	*/
}

func TestNativeTool_MultipleNativeTools(t *testing.T) {
	requireAnthropicKey(t)
	ctx := testContext(t, 120*time.Second)

	// Use multiple native tools together
	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "anthropic/claude-haiku-4-5-20251001",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Search the web for the population of France, then calculate what 1% of that number is")},
		},
		Tools: []vai.Tool{
			vai.WebSearch(),
			vai.CodeExecution(),
		},
		MaxTokens: 2000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resp.TextContent()
	if text == "" {
		t.Error("expected text content in response")
	}

	// Log what tools were used
	for _, block := range resp.Content {
		t.Logf("Content block type: %s", block.BlockType())
	}

	t.Logf("Response: %s", truncate(text, 300))
}

func TestNativeTool_WithFunctionTool(t *testing.T) {
	requireAnthropicKey(t)
	ctx := testContext(t, 60*time.Second)

	// Combine native web search with custom function tool
	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "anthropic/claude-haiku-4-5-20251001",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Search for the current Bitcoin price, then convert it to euros")},
		},
		Tools: []vai.Tool{
			vai.WebSearch(),
			{
				Type:        "function",
				Name:        "convert_currency",
				Description: "Convert USD to EUR",
				InputSchema: &vai.JSONSchema{
					Type: "object",
					Properties: map[string]vai.JSONSchema{
						"usd_amount": {Type: "number"},
					},
					Required: []string{"usd_amount"},
				},
			},
		},
		MaxTokens: 1000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have used tools
	for _, block := range resp.Content {
		t.Logf("Content block type: %s", block.BlockType())
	}

	t.Logf("Response: %s", truncate(resp.TextContent(), 200))
}

// truncate truncates a string to the specified length.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
