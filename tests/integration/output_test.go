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

// ==================== Output Content Type Tests ====================
// These tests verify that different output content types are generated correctly.

func TestOutput_TextOnly(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model:     p.Model,
			MaxTokens: 100,
			Messages:  []vai.Message{{Role: "user", Content: vai.Text("Say hello")}},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should only have text content
		text := resp.TextContent()
		if text == "" {
			t.Error("expected text content")
		}

		// Verify no tool uses
		if len(resp.ToolUses()) > 0 {
			t.Error("unexpected tool uses in text-only response")
		}
	})
}

func TestOutput_ToolUseBlock(t *testing.T) {
	forEachProviderWith(t, func(p providerConfig) bool {
		return p.SupportsTools
	}, func(t *testing.T, p providerConfig) {
		ctx := testContext(t, 60*time.Second)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Calculate 15 * 7")},
			},
			Tools: []vai.Tool{
				{
					Type:        "function",
					Name:        "multiply",
					Description: "Multiply two numbers",
					InputSchema: &vai.JSONSchema{
						Type: "object",
						Properties: map[string]vai.JSONSchema{
							"a": {Type: "number", Description: "First number"},
							"b": {Type: "number", Description: "Second number"},
						},
						Required: []string{"a", "b"},
					},
				},
			},
			ToolChoice: vai.ToolChoiceTool("multiply"),
			MaxTokens:  8000,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		toolUses := resp.ToolUses()
		if len(toolUses) == 0 {
			t.Fatal("expected tool use in response")
		}

		tu := toolUses[0]
		if tu.Name != "multiply" {
			t.Errorf("expected tool name 'multiply', got %q", tu.Name)
		}

		// Check input values
		a, hasA := tu.Input["a"]
		b, hasB := tu.Input["b"]
		if !hasA || !hasB {
			t.Errorf("expected inputs a and b, got %v", tu.Input)
		}

		// Values should be numbers (15 and 7)
		t.Logf("Tool input: a=%v, b=%v", a, b)
	})
}

func TestOutput_MultipleToolUses(t *testing.T) {
	forEachProviderWith(t, func(p providerConfig) bool {
		return p.SupportsTools
	}, func(t *testing.T, p providerConfig) {
		ctx := testContext(t, 60*time.Second)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Get the weather for both Paris and London")},
			},
			Tools: []vai.Tool{
				{
					Type:        "function",
					Name:        "get_weather",
					Description: "Get weather for a city",
					InputSchema: &vai.JSONSchema{
						Type: "object",
						Properties: map[string]vai.JSONSchema{
							"city": {Type: "string"},
						},
						Required: []string{"city"},
					},
				},
			},
			ToolChoice: vai.ToolChoiceAny(),
			MaxTokens:  8000,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		toolUses := resp.ToolUses()
		// Model might use one or two tool calls depending on implementation
		if len(toolUses) == 0 {
			t.Fatal("expected at least one tool use")
		}

		// Log what we got
		for i, tu := range toolUses {
			t.Logf("Tool use %d: name=%s, input=%v", i+1, tu.Name, tu.Input)
		}

		// If we got two calls, verify they're for different cities
		if len(toolUses) >= 2 {
			cities := make(map[string]bool)
			for _, tu := range toolUses {
				if city, ok := tu.Input["city"].(string); ok {
					cities[strings.ToLower(city)] = true
				}
			}
			if !cities["paris"] && !cities["london"] {
				t.Log("warning: expected Paris and/or London in tool calls")
			}
		}
	})
}

func TestOutput_StructuredJSON(t *testing.T) {
	forEachProviderWith(t, func(p providerConfig) bool {
		return p.SupportsStructuredOutput
	}, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		additionalPropertiesFalse := false
		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Extract: Alice is 30 years old and lives in NYC")},
			},
			OutputFormat: &vai.OutputFormat{
				Type: "json_schema",
				JSONSchema: &vai.JSONSchema{
					Type: "object",
					Properties: map[string]vai.JSONSchema{
						"name": {Type: "string"},
						"age":  {Type: "integer"},
						"city": {Type: "string"},
					},
					Required:             []string{"name", "age", "city"},
					AdditionalProperties: &additionalPropertiesFalse,
				},
			},
			MaxTokens: 200,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		text := resp.TextContent()
		// Strip markdown code blocks if present
		text = strings.TrimPrefix(text, "```json\n")
		text = strings.TrimPrefix(text, "```\n")
		text = strings.TrimSuffix(text, "\n```")
		text = strings.TrimSpace(text)

		var result struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
			City string `json:"city"`
		}
		if err := json.Unmarshal([]byte(text), &result); err != nil {
			t.Fatalf("failed to parse JSON: %v\nResponse: %s", err, resp.TextContent())
		}

		if result.Name != "Alice" {
			t.Errorf("expected name 'Alice', got %q", result.Name)
		}
		if result.Age != 30 {
			t.Errorf("expected age 30, got %d", result.Age)
		}
		if !strings.Contains(result.City, "NYC") && !strings.Contains(result.City, "New York") {
			t.Errorf("expected city containing 'NYC' or 'New York', got %q", result.City)
		}
	})
}

func TestOutput_MixedTextAndToolUse(t *testing.T) {
	forEachProviderWith(t, func(p providerConfig) bool {
		return p.SupportsTools
	}, func(t *testing.T, p providerConfig) {
		ctx := testContext(t, 60*time.Second)

		// Some models will include thinking/text before tool use
		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("First explain what you're going to do, then call get_weather for Paris")},
			},
			Tools: []vai.Tool{
				{
					Type:        "function",
					Name:        "get_weather",
					Description: "Get weather for a city",
					InputSchema: &vai.JSONSchema{
						Type: "object",
						Properties: map[string]vai.JSONSchema{
							"city": {Type: "string"},
						},
						Required: []string{"city"},
					},
				},
			},
			MaxTokens: 8000,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have tool use
		if len(resp.ToolUses()) == 0 {
			t.Error("expected tool use in response")
		}

		// May or may not have text depending on model
		text := resp.TextContent()
		hasText := text != ""
		hasToolUse := len(resp.ToolUses()) > 0

		t.Logf("Has text: %v, Has tool use: %v", hasText, hasToolUse)
		if hasText {
			t.Logf("Text content: %s", text)
		}
	})
}

func TestOutput_StreamedText(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		stream, err := testClient.Messages.Stream(ctx, &vai.MessageRequest{
			Model:     p.Model,
			MaxTokens: 100,
			Messages:  []vai.Message{{Role: "user", Content: vai.Text("Count 1 2 3 4 5")}},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer stream.Close()

		var textDeltas []string
		for event := range stream.Events() {
			if delta, ok := event.(vai.ContentBlockDeltaEvent); ok {
				if text, ok := delta.Delta.(vai.TextDelta); ok {
					textDeltas = append(textDeltas, text.Text)
				}
			}
		}

		if len(textDeltas) == 0 {
			t.Error("expected text deltas in stream")
		}

		// Final response should match accumulated deltas
		resp := stream.Response()
		if resp == nil {
			t.Fatal("stream.Response() is nil")
		}

		accumulated := strings.Join(textDeltas, "")
		if accumulated != resp.TextContent() {
			t.Errorf("accumulated text mismatch:\nAccumulated: %q\nResponse: %q",
				accumulated, resp.TextContent())
		}
	})
}

func TestOutput_StreamedToolUse(t *testing.T) {
	forEachProviderWith(t, func(p providerConfig) bool {
		return p.SupportsToolStreaming
	}, func(t *testing.T, p providerConfig) {
		ctx := testContext(t, 60*time.Second)

		stream, err := testClient.Messages.Stream(ctx, &vai.MessageRequest{
			Model: p.Model,
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
			ToolChoice: vai.ToolChoiceTool("get_weather"),
			MaxTokens:  8000,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer stream.Close()

		var hasToolBlockStart, hasInputDelta bool
		for event := range stream.Events() {
			switch e := event.(type) {
			case vai.ContentBlockStartEvent:
				if e.ContentBlock.BlockType() == "tool_use" {
					hasToolBlockStart = true
				}
			case vai.ContentBlockDeltaEvent:
				if _, ok := e.Delta.(vai.InputJSONDelta); ok {
					hasInputDelta = true
				}
			}
		}

		if !hasToolBlockStart {
			t.Error("expected tool_use content_block_start event")
		}
		if !hasInputDelta {
			t.Log("warning: expected input_json_delta events (may be provider-specific)")
		}

		// Verify final response has tool use
		resp := stream.Response()
		if resp == nil {
			t.Fatal("stream.Response() is nil")
		}
		if len(resp.ToolUses()) == 0 {
			t.Error("expected tool use in final response")
		}
	})
}

func TestOutput_Extract(t *testing.T) {
	forEachProviderWith(t, func(p providerConfig) bool {
		return p.SupportsStructuredOutput
	}, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		type Person struct {
			Name string `json:"name" desc:"Person's name"`
			Age  int    `json:"age" desc:"Person's age"`
		}

		var person Person
		_, err := testClient.Messages.Extract(ctx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Extract: Bob is 25 years old")},
			},
			MaxTokens: 100,
		}, &person)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if person.Name != "Bob" {
			t.Errorf("expected name 'Bob', got %q", person.Name)
		}
		if person.Age != 25 {
			t.Errorf("expected age 25, got %d", person.Age)
		}
	})
}
