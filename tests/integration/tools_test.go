//go:build integration
// +build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	vai "github.com/vango-go/vai/sdk"
)

func TestMessages_Create_WithToolDefinition(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsTools
	}, func(t *testing.T, provider providerConfig) {
		ctx := defaultTestContext(t)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("What's the weather in San Francisco?")},
			},
			Tools: []vai.Tool{
				{
					Type:        "function",
					Name:        "get_weather",
					Description: "Get current weather for a location",
					InputSchema: &vai.JSONSchema{
						Type: "object",
						Properties: map[string]vai.JSONSchema{
							"location": {Type: "string", Description: "City name"},
						},
						Required: []string{"location"},
					},
				},
			},
			MaxTokens: 8000,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.StopReason != vai.StopReasonToolUse {
			t.Errorf("expected stop_reason 'tool_use', got %q", resp.StopReason)
		}

		toolUses := resp.ToolUses()
		if len(toolUses) == 0 {
			t.Fatal("expected at least one tool use")
		}

		if toolUses[0].Name != "get_weather" {
			t.Errorf("expected tool 'get_weather', got %q", toolUses[0].Name)
		}

		if _, ok := toolUses[0].Input["location"]; !ok {
			t.Error("expected 'location' in tool input")
		}
	})
}

func TestMessages_Create_ToolResult(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsTools
	}, func(t *testing.T, provider providerConfig) {
		ctx := testContext(t, 60*time.Second)

		// First turn: Get tool call
		resp1, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
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
							"location": {Type: "string"},
						},
						Required: []string{"location"},
					},
				},
			},
			ToolChoice: vai.ToolChoiceTool("get_weather"),
			MaxTokens: 8000,
		})
		if err != nil {
			t.Fatalf("first turn error: %v", err)
		}

		if resp1.StopReason != vai.StopReasonToolUse {
			t.Fatalf("expected stop_reason 'tool_use', got %q", resp1.StopReason)
		}

		toolUses := resp1.ToolUses()
		if len(toolUses) == 0 {
			t.Fatal("expected at least one tool use")
		}

		// Second turn: Provide tool result
		resp2, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("What's the weather in Tokyo?")},
				{Role: "assistant", Content: resp1.Content},
				{Role: "user", Content: []vai.ContentBlock{
					vai.ToolResult(toolUses[0].ID, []vai.ContentBlock{
						vai.Text("Weather in Tokyo: 22°C, sunny"),
					}),
				}},
			},
			Tools: []vai.Tool{
				{
					Type:        "function",
					Name:        "get_weather",
					Description: "Get weather",
					InputSchema: &vai.JSONSchema{Type: "object"},
				},
			},
			MaxTokens: 8000,
		})

		if err != nil {
			t.Fatalf("second turn error: %v", err)
		}

		if resp2.StopReason != vai.StopReasonEndTurn {
			t.Errorf("expected stop_reason 'end_turn', got %q", resp2.StopReason)
		}

		text := resp2.TextContent()
		if text == "" {
			t.Error("expected text content in response")
		}

		// Should mention the temperature
		if !contains(text, "22") && !contains(text, "sunny") {
			t.Logf("Response: %s", text)
			t.Log("warning: expected response to mention weather details")
		}
	})
}

func TestMessages_Create_ToolChoice_Auto(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsTools
	}, func(t *testing.T, provider providerConfig) {
		ctx := defaultTestContext(t)

		// With tool_choice auto, model decides whether to use tools
		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("What is 2+2?")},
			},
			Tools: []vai.Tool{
				{
					Type:        "function",
					Name:        "calculator",
					Description: "Perform calculations",
					InputSchema: &vai.JSONSchema{
						Type: "object",
						Properties: map[string]vai.JSONSchema{
							"expression": {Type: "string"},
						},
					},
				},
			},
			ToolChoice: vai.ToolChoiceAuto(),
			MaxTokens: 8000,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Model can either answer directly or use tool
		if resp.StopReason != vai.StopReasonEndTurn && resp.StopReason != vai.StopReasonToolUse {
			t.Errorf("unexpected stop_reason: %q", resp.StopReason)
		}
	})
}

func TestMessages_Create_ToolChoice_None(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsTools
	}, func(t *testing.T, provider providerConfig) {
		ctx := defaultTestContext(t)

		// With tool_choice none, model cannot use tools
		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("What's the weather? Use a tool if needed.")},
			},
			Tools: []vai.Tool{
				{
					Type:        "function",
					Name:        "get_weather",
					Description: "Get weather",
					InputSchema: &vai.JSONSchema{Type: "object"},
				},
			},
			ToolChoice: vai.ToolChoiceNone(),
			MaxTokens: 8000,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Model should not use tools
		if resp.StopReason == vai.StopReasonToolUse {
			t.Error("expected no tool use with tool_choice none")
		}

		if len(resp.ToolUses()) > 0 {
			t.Error("expected empty tool uses")
		}
	})
}

func TestFuncAsTool_SchemaGeneration(t *testing.T) {
	type TestInput struct {
		Name     string   `json:"name" desc:"User name"`
		Age      int      `json:"age" desc:"User age"`
		Tags     []string `json:"tags,omitempty" desc:"Optional tags"`
		IsActive bool     `json:"is_active"`
	}

	tool, handler := vai.FuncAsTool("test_tool", "A test tool",
		func(ctx context.Context, input TestInput) (any, error) {
			return "ok", nil
		},
	)

	if tool.Type != "function" {
		t.Errorf("expected type 'function', got %q", tool.Type)
	}
	if tool.Name != "test_tool" {
		t.Errorf("expected name 'test_tool', got %q", tool.Name)
	}
	if tool.InputSchema == nil {
		t.Fatal("expected non-nil input schema")
	}
	if tool.InputSchema.Type != "object" {
		t.Errorf("expected object type, got %q", tool.InputSchema.Type)
	}

	// Check properties
	if _, ok := tool.InputSchema.Properties["name"]; !ok {
		t.Error("expected 'name' property")
	}
	if _, ok := tool.InputSchema.Properties["age"]; !ok {
		t.Error("expected 'age' property")
	}

	// Check required (tags has omitempty so should not be required)
	hasName := false
	hasTags := false
	for _, r := range tool.InputSchema.Required {
		if r == "name" {
			hasName = true
		}
		if r == "tags" {
			hasTags = true
		}
	}
	if !hasName {
		t.Error("expected 'name' to be required")
	}
	if hasTags {
		t.Error("expected 'tags' to not be required (has omitempty)")
	}

	// Verify handler is not nil
	if handler == nil {
		t.Error("expected non-nil handler")
	}
}

func TestMakeTool_Integration(t *testing.T) {
	tool := vai.MakeTool("greet", "Greet a person",
		func(ctx context.Context, input struct {
			Name string `json:"name"`
		}) (string, error) {
			return "Hello, " + input.Name + "!", nil
		},
	)

	if tool.Name != "greet" {
		t.Errorf("expected name 'greet', got %q", tool.Name)
	}
	if tool.Handler == nil {
		t.Error("expected non-nil handler")
	}
	if tool.Type != "function" {
		t.Errorf("expected type 'function', got %q", tool.Type)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsHelper(s, substr)
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
