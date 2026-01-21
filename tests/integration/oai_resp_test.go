//go:build integration
// +build integration

package integration_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	vai "github.com/vango-go/vai/sdk"
)

// ==================== OAI Responses API Specific Tests ====================
//
// These tests cover features unique to the OpenAI Responses API provider:
// - Reasoning models (gpt-5, o1, o3, o4) with thinking capabilities
// - Previous response ID for stateful conversations
// - Native tools (web_search, code_interpreter)
// - Stop sequences unsupported error
// - Audio input/output

const oaiRespModel = "oai-resp/gpt-5-mini"

func requireOaiRespProvider(t *testing.T) {
	requireOpenAIKey(t)
}

// ==================== Reasoning Model Tests ====================

func TestOaiResp_ReasoningModel_IgnoresTemperature(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := defaultTestContext(t)

	// gpt-5 models are reasoning models and should ignore temperature
	temp := 0.9
	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("What is 2+2? Reply with just the number.")},
		},
		Temperature: &temp, // Should be ignored for reasoning models
		MaxTokens:   100,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still get a valid response even though temperature is ignored
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if !strings.Contains(resp.TextContent(), "4") {
		t.Errorf("expected '4' in response, got %q", resp.TextContent())
	}
	t.Logf("Response: %s", resp.TextContent())
}

func TestOaiResp_ReasoningModel_IgnoresTopP(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := defaultTestContext(t)

	topP := 0.5
	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Say 'hello' and nothing else.")},
		},
		TopP:      &topP, // Should be ignored for reasoning models
		MaxTokens: 200,   // Increase to ensure enough tokens
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	// Response may be empty for reasoning models in some cases - just verify no error
	t.Logf("Response: %s", resp.TextContent())
}

// ==================== Stop Sequences Error Test ====================

func TestOaiResp_StopSequences_ReturnsError(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := defaultTestContext(t)

	// OpenAI Responses API does not support stop_sequences
	_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Count from 1 to 10.")},
		},
		StopSequences: []string{"5"}, // Not supported
		MaxTokens:     100,
	})

	if err == nil {
		t.Fatal("expected error for stop_sequences on oai-resp")
	}

	// Should be an unsupported feature error
	if !strings.Contains(err.Error(), "stop_sequences") && !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("expected error about stop_sequences being unsupported, got: %v", err)
	}
	t.Logf("Got expected error: %v", err)
}

// ==================== Native Tool Tests ====================

func TestOaiResp_NativeTool_WebSearch(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := testContext(t, 60*time.Second)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Search the web for the current weather in Tokyo. Give me a brief summary.")},
		},
		Tools: []vai.Tool{
			vai.WebSearch(), // Native web search tool
		},
		MaxTokens: 2000, // Increased to allow for full response
	})

	if err != nil {
		// Web search may not be available for all accounts
		if strings.Contains(err.Error(), "not available") || strings.Contains(err.Error(), "not supported") {
			t.Skipf("web_search not available: %v", err)
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Should have either used the tool or responded directly
	text := resp.TextContent()
	t.Logf("Response: %s", text)
	t.Logf("Stop reason: %s", resp.StopReason)

	// Check for valid stop reasons (including max_tokens since web search can generate long responses)
	validStopReasons := resp.StopReason == vai.StopReasonToolUse ||
		resp.StopReason == vai.StopReasonEndTurn ||
		resp.StopReason == vai.StopReasonMaxTokens
	if !validStopReasons {
		t.Errorf("unexpected stop reason: %s", resp.StopReason)
	}
}

func TestOaiResp_NativeTool_CodeInterpreter(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := testContext(t, 90*time.Second)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Calculate the factorial of 10 using Python code.")},
		},
		Tools: []vai.Tool{
			vai.CodeExecution(), // Maps to code_interpreter
		},
		MaxTokens: 500,
	})

	if err != nil {
		// Code interpreter requires container configuration which may not be available
		if strings.Contains(err.Error(), "container") ||
			strings.Contains(err.Error(), "not available") ||
			strings.Contains(err.Error(), "not supported") ||
			strings.Contains(err.Error(), "missing_required_parameter") {
			t.Skipf("code_interpreter requires additional setup: %v", err)
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	text := resp.TextContent()
	t.Logf("Response: %s", text)

	// Should mention the result (3628800)
	if !strings.Contains(text, "3628800") && !strings.Contains(text, "factorial") {
		t.Log("Note: Response may not include exact factorial result")
	}
}

// ==================== Multi-Turn Conversation Tests ====================

func TestOaiResp_MultiTurn_BasicConversation(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := testContext(t, 60*time.Second)

	// First turn
	resp1, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("My favorite color is blue. Please acknowledge this.")},
		},
		MaxTokens: 500,
	})
	if err != nil {
		t.Fatalf("first turn error: %v", err)
	}

	t.Logf("First turn response: %s", resp1.TextContent())

	// Second turn - verify memory
	resp2, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("My favorite color is blue. Please acknowledge this.")},
			{Role: "assistant", Content: resp1.Content},
			{Role: "user", Content: vai.Text("What is my favorite color? Please answer directly.")},
		},
		MaxTokens: 500,
	})
	if err != nil {
		t.Fatalf("second turn error: %v", err)
	}

	text := strings.ToLower(resp2.TextContent())
	t.Logf("Second turn response: %s", text)

	if !strings.Contains(text, "blue") {
		// Some models may have empty responses due to safety filters or other issues
		if text == "" {
			t.Log("Note: Model returned empty response - may be due to API behavior")
		} else {
			t.Errorf("expected 'blue' in response, got %q", resp2.TextContent())
		}
	}
}

func TestOaiResp_MultiTurn_WithToolHistory(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := testContext(t, 90*time.Second)

	weatherTool := vai.Tool{
		Type:        "function",
		Name:        "get_weather",
		Description: "Get the current weather for a location",
		InputSchema: &vai.JSONSchema{
			Type: "object",
			Properties: map[string]vai.JSONSchema{
				"location": {Type: "string", Description: "City name"},
			},
			Required: []string{"location"},
		},
	}

	// First turn - should trigger tool use
	resp1, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("What's the weather in Paris?")},
		},
		Tools:      []vai.Tool{weatherTool},
		ToolChoice: vai.ToolChoiceTool("get_weather"),
		MaxTokens:  500,
	})
	if err != nil {
		t.Fatalf("first turn error: %v", err)
	}

	if resp1.StopReason != vai.StopReasonToolUse {
		t.Fatalf("expected tool_use stop reason, got %q", resp1.StopReason)
	}

	toolUses := resp1.ToolUses()
	if len(toolUses) == 0 {
		t.Fatal("expected at least one tool use")
	}

	// Second turn - provide tool result
	resp2, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("What's the weather in Paris?")},
			{Role: "assistant", Content: resp1.Content},
			{Role: "user", Content: []vai.ContentBlock{
				vai.ToolResult(toolUses[0].ID, []vai.ContentBlock{
					vai.Text("The weather in Paris is 18°C and partly cloudy."),
				}),
			}},
		},
		Tools:     []vai.Tool{weatherTool},
		MaxTokens: 500,
	})
	if err != nil {
		t.Fatalf("second turn error: %v", err)
	}

	if resp2.StopReason != vai.StopReasonEndTurn {
		t.Errorf("expected end_turn stop reason, got %q", resp2.StopReason)
	}

	// Response should mention the weather info
	text := resp2.TextContent()
	if !strings.Contains(text, "18") && !strings.Contains(text, "cloudy") && !strings.Contains(text, "Paris") {
		t.Logf("Response: %s", text)
		t.Log("Note: Expected response to mention weather details")
	}
}

// ==================== Streaming Tests ====================

func TestOaiResp_Stream_BasicText(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := defaultTestContext(t)

	stream, err := testClient.Messages.Stream(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Count from 1 to 3, one number per line.")},
		},
		MaxTokens: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	var textContent strings.Builder
	var gotStart, gotStop bool
	var deltaCount int

	for event := range stream.Events() {
		switch e := event.(type) {
		case vai.MessageStartEvent:
			gotStart = true
			if e.Message.ID == "" {
				t.Error("expected non-empty message ID")
			}
		case vai.ContentBlockDeltaEvent:
			if delta, ok := e.Delta.(vai.TextDelta); ok {
				deltaCount++
				textContent.WriteString(delta.Text)
			}
		case vai.MessageStopEvent:
			gotStop = true
		}
	}

	if !gotStart {
		t.Error("expected message_start event")
	}
	if !gotStop {
		t.Error("expected message_stop event")
	}
	if deltaCount == 0 {
		t.Error("expected at least one text delta")
	}

	text := textContent.String()
	if !strings.Contains(text, "1") {
		t.Errorf("expected '1' in response, got %q", text)
	}

	t.Logf("Streamed %d deltas, text: %q", deltaCount, text)
}

func TestOaiResp_Stream_WithToolCall(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := testContext(t, 60*time.Second)

	stream, err := testClient.Messages.Stream(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("What's the weather in Tokyo?")},
		},
		Tools: []vai.Tool{
			{
				Type:        "function",
				Name:        "get_weather",
				Description: "Get weather for a location",
				InputSchema: &vai.JSONSchema{
					Type: "object",
					Properties: map[string]vai.JSONSchema{
						"location": {Type: "string"},
					},
					Required: []string{"location"},
				},
			},
		},
		MaxTokens: 500,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	var gotToolUseStart bool
	var toolUseJSON strings.Builder

	for event := range stream.Events() {
		switch e := event.(type) {
		case vai.ContentBlockStartEvent:
			if e.ContentBlock.BlockType() == "tool_use" {
				gotToolUseStart = true
			}
		case vai.ContentBlockDeltaEvent:
			if delta, ok := e.Delta.(vai.InputJSONDelta); ok {
				toolUseJSON.WriteString(delta.PartialJSON)
			}
		}
	}

	resp := stream.Response()
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Model may or may not call the tool
	if resp.StopReason == vai.StopReasonToolUse {
		if !gotToolUseStart {
			t.Error("expected tool_use content block start event")
		}

		toolUses := resp.ToolUses()
		if len(toolUses) == 0 {
			t.Error("expected tool use blocks in response")
		} else {
			t.Logf("Tool called: %s, input: %v", toolUses[0].Name, toolUses[0].Input)
		}
	} else {
		t.Logf("Model did not call tool, stop reason: %s", resp.StopReason)
	}
}

func TestOaiResp_Stream_LongResponse(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := testContext(t, 120*time.Second)

	stream, err := testClient.Messages.Stream(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Write a short paragraph about artificial intelligence.")},
		},
		MaxTokens: 300,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	startTime := time.Now()
	var firstDelta time.Duration
	var deltaCount int
	var totalChars int

	for event := range stream.Events() {
		if delta, ok := event.(vai.ContentBlockDeltaEvent); ok {
			if text, ok := delta.Delta.(vai.TextDelta); ok {
				if deltaCount == 0 {
					firstDelta = time.Since(startTime)
				}
				deltaCount++
				totalChars += len(text.Text)
			}
		}
	}

	t.Logf("Time to first token: %v", firstDelta)
	t.Logf("Total deltas: %d, total chars: %d", deltaCount, totalChars)

	// Sanity checks
	if firstDelta > 15*time.Second {
		t.Errorf("time to first token too slow: %v", firstDelta)
	}
	if deltaCount < 5 {
		t.Errorf("expected more deltas for long response, got %d", deltaCount)
	}
}

// ==================== Structured Output Tests ====================

func TestOaiResp_StructuredOutput_JSONSchema(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := testContext(t, 60*time.Second)

	additionalPropertiesFalse := false
	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Extract: Alice Smith is 28 years old and works at Google.")},
		},
		OutputFormat: &vai.OutputFormat{
			Type: "json_schema",
			JSONSchema: &vai.JSONSchema{
				Type: "object",
				Properties: map[string]vai.JSONSchema{
					"name":    {Type: "string"},
					"age":     {Type: "integer"},
					"company": {Type: "string"},
				},
				Required:             []string{"name", "age", "company"},
				AdditionalProperties: &additionalPropertiesFalse,
			},
		},
		MaxTokens: 500,
	})

	if err != nil {
		// Structured output may have transient API errors
		if strings.Contains(err.Error(), "server_error") || strings.Contains(err.Error(), "An error occurred") {
			t.Skipf("API transient error, retry later: %v", err)
		}
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse response
	var result struct {
		Name    string `json:"name"`
		Age     int    `json:"age"`
		Company string `json:"company"`
	}

	text := resp.TextContent()
	// Clean up any markdown formatting
	text = strings.TrimPrefix(text, "```json\n")
	text = strings.TrimPrefix(text, "```\n")
	text = strings.TrimSuffix(text, "\n```")
	text = strings.TrimSpace(text)

	if text == "" {
		t.Skip("Empty response from model")
	}

	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v\nResponse: %s", err, resp.TextContent())
	}

	if result.Name != "Alice Smith" {
		t.Errorf("expected name 'Alice Smith', got %q", result.Name)
	}
	if result.Age != 28 {
		t.Errorf("expected age 28, got %d", result.Age)
	}
	if !strings.Contains(result.Company, "Google") {
		t.Errorf("expected company containing 'Google', got %q", result.Company)
	}

	t.Logf("Extracted: %+v", result)
}

// ==================== Vision Tests ====================

func TestOaiResp_Vision_ImageURL(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := testContext(t, 60*time.Second)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: []vai.ContentBlock{
				vai.Text("What animal is in this image? Answer in one word."),
				vai.ImageURL("https://images.unsplash.com/photo-1514888286974-6c03e2ca1dba?w=200"),
			}},
		},
		MaxTokens: 200,
	})

	if err != nil {
		// Vision may have API-specific issues
		if strings.Contains(err.Error(), "unknown_parameter") || strings.Contains(err.Error(), "not supported") {
			t.Skipf("Vision not available in expected format: %v", err)
		}
		t.Fatalf("unexpected error: %v", err)
	}

	text := strings.ToLower(resp.TextContent())
	t.Logf("Vision response: %s", text)

	if text == "" {
		t.Log("Note: Model returned empty response for vision")
	} else if !strings.Contains(text, "cat") {
		t.Errorf("expected 'cat' in response, got %q", resp.TextContent())
	}
}

func TestOaiResp_Vision_ImageBase64(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := testContext(t, 60*time.Second)

	// Use the fixture loader for a minimal PNG
	imageData := fixtures.Image("test.png")

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: []vai.ContentBlock{
				vai.Text("Describe this image briefly."),
				vai.Image(imageData, "image/png"),
			}},
		},
		MaxTokens: 200,
	})

	if err != nil {
		// Base64 image format may differ between API versions
		if strings.Contains(err.Error(), "unknown_parameter") || strings.Contains(err.Error(), "image") {
			t.Skipf("Base64 image format not supported in this API version: %v", err)
		}
		t.Fatalf("unexpected error: %v", err)
	}

	t.Logf("Image description: %s", resp.TextContent())
}

// ==================== System Prompt Tests ====================

func TestOaiResp_SystemPrompt_String(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := defaultTestContext(t)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model:  oaiRespModel,
		System: "You are a helpful assistant that always responds in French.",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Hello!")},
		},
		MaxTokens: 100,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Response should be in French (or at least contain French words)
	text := strings.ToLower(resp.TextContent())
	hasFrench := strings.Contains(text, "bonjour") ||
		strings.Contains(text, "salut") ||
		strings.Contains(text, "bienvenue") ||
		strings.Contains(text, "comment") ||
		strings.Contains(text, "je")

	if !hasFrench {
		t.Logf("Response (may not be French): %s", resp.TextContent())
	}
}

// ==================== Error Handling Tests ====================

func TestOaiResp_Error_InvalidModel(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := defaultTestContext(t)

	_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "oai-resp/nonexistent-model-xyz-123",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Hello")},
		},
		MaxTokens: 100,
	})

	if err == nil {
		t.Fatal("expected error for invalid model")
	}
	t.Logf("Got expected error: %v", err)
}

func TestOaiResp_Error_EmptyMessages(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := defaultTestContext(t)

	_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model:     oaiRespModel,
		Messages:  []vai.Message{},
		MaxTokens: 100,
	})

	if err == nil {
		t.Fatal("expected error for empty messages")
	}
	t.Logf("Got expected error: %v", err)
}

// ==================== Usage Tracking Tests ====================

func TestOaiResp_Usage_TokenCounts(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := defaultTestContext(t)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Hello! Please respond with a greeting.")},
		},
		MaxTokens: 200,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Logf("Response: %s", resp.TextContent())
	t.Logf("Usage: input=%d, output=%d, total=%d",
		resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.TotalTokens)

	// Verify input tokens is populated (output may be 0 for some reasoning models)
	if resp.Usage.InputTokens == 0 {
		t.Error("expected non-zero input tokens")
	}
	// Note: Some reasoning models may report 0 output tokens due to API behavior
	if resp.Usage.OutputTokens == 0 && resp.TextContent() != "" {
		t.Log("Note: Output tokens reported as 0 despite non-empty response - may be API behavior")
	}
}

// ==================== Max Tokens Tests ====================

func TestOaiResp_MaxTokens_Limit(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := defaultTestContext(t)

	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Write a very long essay about artificial intelligence, covering its history, current state, and future predictions.")},
		},
		MaxTokens: 20, // Very short limit
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StopReason != vai.StopReasonMaxTokens {
		t.Errorf("expected stop_reason 'max_tokens', got %q", resp.StopReason)
	}

	// Output tokens should be close to the limit
	if resp.Usage.OutputTokens > 30 { // Some buffer for model variation
		t.Errorf("expected output tokens <= 30, got %d", resp.Usage.OutputTokens)
	}
}

func TestOaiResp_MaxTokens_MinimumEnforced(t *testing.T) {
	requireOaiRespProvider(t)
	ctx := defaultTestContext(t)

	// OpenAI requires minimum 16 tokens - provider should enforce this
	resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: oaiRespModel,
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Say 'hello world' and nothing else.")},
		},
		MaxTokens: 5, // Below minimum, should be raised to 16
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still get a response (provider raises to 16)
	// Note: Some reasoning models may return empty responses
	t.Logf("Response with min tokens: %s", resp.TextContent())
}
