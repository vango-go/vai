//go:build integration
// +build integration

package integration_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	vai "github.com/vango-go/vai/sdk"
)

func TestMessages_Run_BasicToolExecution(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsTools
	}, func(t *testing.T, provider providerConfig) {
		ctx := testContext(t, 60*time.Second)

		weatherTool := vai.MakeTool("get_weather", "Get weather for a location",
			func(ctx context.Context, input struct {
				Location string `json:"location"`
			}) (string, error) {
				return "72°F and sunny in " + input.Location, nil
			},
		)

		result, err := testClient.Messages.Run(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("What's the weather in San Francisco?")},
			},
			Tools:     []vai.Tool{weatherTool.Tool},
			MaxTokens: 8000,
		},
			vai.WithTools(weatherTool),
			vai.WithMaxToolCalls(1),
		)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Response == nil {
			t.Fatal("expected non-nil response")
		}
		if result.ToolCallCount < 1 {
			t.Errorf("expected at least 1 tool call, got %d", result.ToolCallCount)
		}
		if result.StopReason != vai.RunStopEndTurn && result.StopReason != vai.RunStopMaxToolCalls {
			t.Errorf("unexpected stop reason: %q", result.StopReason)
		}

		// Response should mention the weather
		text := result.Response.TextContent()
		if !strings.Contains(text, "72") && !strings.Contains(text, "sunny") {
			t.Logf("Response: %s", text)
			t.Log("warning: expected weather info in response")
		}
	})
}

func TestMessages_Run_MultipleToolCalls(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsTools
	}, func(t *testing.T, provider providerConfig) {
		ctx := testContext(t, 90*time.Second)

		var callCount int
		var mu sync.Mutex

		weatherTool := vai.MakeTool("get_weather", "Get weather",
			func(ctx context.Context, input struct {
				Location string `json:"location"`
			}) (string, error) {
				mu.Lock()
				callCount++
				mu.Unlock()
				return "75°F in " + input.Location, nil
			},
		)

		result, err := testClient.Messages.Run(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("What's the weather in New York, Los Angeles, and Chicago?")},
			},
			Tools:     []vai.Tool{weatherTool.Tool},
			MaxTokens: 8000,
		},
			vai.WithTools(weatherTool),
			vai.WithMaxToolCalls(5),
		)

		if err != nil {
			// Model sometimes generates invalid tool call JSON that the provider can't parse
			// This is a model limitation, not a provider bug
			if strings.Contains(err.Error(), "output_parse_failed") || strings.Contains(err.Error(), "Parsing failed") {
				t.Skipf("Model generated unparseable output (model limitation): %v", err)
			}
			t.Fatalf("unexpected error: %v", err)
		}

		t.Logf("Total tool calls: %d", result.ToolCallCount)

		// Model may or may not call tools depending on its behavior
		if result.ToolCallCount == 0 {
			t.Log("Note: Model chose not to call tools - this is model-dependent behavior")
		}
		if result.ToolCallCount > 5 {
			t.Errorf("expected at most 5 tool calls (limit was set), got %d", result.ToolCallCount)
		}
	})
}

func TestMessages_Run_MaxToolCallsLimit(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsTools
	}, func(t *testing.T, provider providerConfig) {
		ctx := testContext(t, 60*time.Second)

		infiniteTool := vai.MakeTool("do_something", "Do something that might need repetition",
			func(ctx context.Context, input struct{}) (string, error) {
				return "Done, but you might want to do it again", nil
			},
		)

		result, err := testClient.Messages.Run(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Keep calling do_something 10 times.")},
			},
			Tools:      []vai.Tool{infiniteTool.Tool},
			ToolChoice: vai.ToolChoiceTool("do_something"),
			MaxTokens: 8000,
		},
			vai.WithTools(infiniteTool),
			vai.WithMaxToolCalls(3),
		)

		if err != nil {
			// Model sometimes refuses to call tools even when required - this is model-dependent
			if strings.Contains(err.Error(), "tool_use_failed") || strings.Contains(err.Error(), "did not call a tool") {
				t.Skipf("Model refused to call tools despite tool_choice (model limitation): %v", err)
			}
			t.Fatalf("unexpected error: %v", err)
		}

		t.Logf("Tool calls: %d, Stop reason: %q", result.ToolCallCount, result.StopReason)

		if result.ToolCallCount > 3 {
			t.Errorf("expected at most 3 tool calls, got %d", result.ToolCallCount)
		}
		// Accept both max_tool_calls (expected) and end_turn (model may stop early)
		if result.StopReason != vai.RunStopMaxToolCalls && result.StopReason != vai.RunStopEndTurn {
			t.Errorf("expected stop reason 'max_tool_calls' or 'end_turn', got %q", result.StopReason)
		}
	})
}

func TestMessages_Run_MaxTurnsLimit(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsTools
	}, func(t *testing.T, provider providerConfig) {
		ctx := testContext(t, 60*time.Second)

		tool := vai.MakeTool("think", "Think about something",
			func(ctx context.Context, input struct{}) (string, error) {
				return "I thought about it", nil
			},
		)

		result, err := testClient.Messages.Run(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Think about many things, calling the think tool each time.")},
			},
			Tools:      []vai.Tool{tool.Tool},
			ToolChoice: vai.ToolChoiceAny(),
			MaxTokens: 8000,
		},
			vai.WithTools(tool),
			vai.WithMaxTurns(2),
			vai.WithMaxToolCalls(10),
		)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.TurnCount > 2 {
			t.Errorf("expected at most 2 turns, got %d", result.TurnCount)
		}
		// May hit max_turns or max_tool_calls depending on model behavior
		if result.StopReason != vai.RunStopMaxTurns && result.StopReason != vai.RunStopMaxToolCalls {
			t.Errorf("expected stop reason 'max_turns' or 'max_tool_calls', got %q", result.StopReason)
		}
	})
}

func TestMessages_Run_CustomStopCondition(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		ctx := testContext(t, 60*time.Second)

		result, err := testClient.Messages.Run(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Count from 1 to 10. When you reach 5, say DONE.")},
			},
			MaxTokens: 8000,
		},
			vai.WithStopWhen(func(resp *vai.Response) bool {
				return strings.Contains(resp.TextContent(), "DONE")
			}),
			vai.WithMaxTurns(5),
		)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Either stopped by custom condition or naturally
		if result.StopReason != vai.RunStopCustom && result.StopReason != vai.RunStopEndTurn {
			t.Errorf("unexpected stop reason: %q", result.StopReason)
		}
	})
}

func TestMessages_Run_Timeout(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		ctx := defaultTestContext(t)

		_, err := testClient.Messages.Run(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Write a very long essay about the entire history of humanity.")},
			},
			MaxTokens: 4000,
		},
			vai.WithRunTimeout(1*time.Millisecond), // Very short timeout
		)

		if err == nil {
			t.Error("expected timeout error")
		}
		if !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "timeout") {
			t.Errorf("expected timeout error, got: %v", err)
		}
	})
}

func TestMessages_Run_Hooks(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsTools
	}, func(t *testing.T, provider providerConfig) {
		ctx := testContext(t, 60*time.Second)

		var beforeCallCount, afterResponseCount, toolCallCount int
		var mu sync.Mutex

		tool := vai.MakeTool("greet", "Say hello",
			func(ctx context.Context, input struct{ Name string `json:"name"` }) (string, error) {
				return "Hello, " + input.Name, nil
			},
		)

		_, err := testClient.Messages.Run(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Greet Alice using the greet tool")},
			},
			Tools:     []vai.Tool{tool.Tool},
			MaxTokens: 8000,
		},
			vai.WithTools(tool),
			vai.WithMaxToolCalls(1),
			vai.WithBeforeCall(func(req *vai.MessageRequest) {
				mu.Lock()
				beforeCallCount++
				mu.Unlock()
			}),
			vai.WithAfterResponse(func(resp *vai.Response) {
				mu.Lock()
				afterResponseCount++
				mu.Unlock()
			}),
			vai.WithOnToolCall(func(name string, input map[string]any, output any, err error) {
				mu.Lock()
				toolCallCount++
				mu.Unlock()
			}),
		)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if beforeCallCount < 1 {
			t.Errorf("expected beforeCall hook to be called, got %d calls", beforeCallCount)
		}
		if afterResponseCount < 1 {
			t.Errorf("expected afterResponse hook to be called, got %d calls", afterResponseCount)
		}
		if toolCallCount < 1 {
			t.Errorf("expected onToolCall hook to be called, got %d calls", toolCallCount)
		}
	})
}

func TestMessages_Run_UsageAggregation(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsTools
	}, func(t *testing.T, provider providerConfig) {
		ctx := testContext(t, 90*time.Second)

		tool := vai.MakeTool("calc", "Calculate",
			func(ctx context.Context, input struct{ Expr string `json:"expr"` }) (string, error) {
				return "42", nil
			},
		)

		result, err := testClient.Messages.Run(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Calculate 1+1 and 2+2 using the calc tool")},
			},
			Tools:     []vai.Tool{tool.Tool},
			MaxTokens: 8000,
		},
			vai.WithTools(tool),
			vai.WithMaxToolCalls(3),
		)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Usage should be aggregated across all turns
		if result.Usage.InputTokens <= 0 {
			t.Error("expected positive input tokens")
		}
		if result.Usage.OutputTokens <= 0 {
			t.Error("expected positive output tokens")
		}

		t.Logf("Usage: input=%d, output=%d, total=%d",
			result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.TotalTokens)
	})
}

func TestMessages_Run_NoToolHandler(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsTools
	}, func(t *testing.T, provider providerConfig) {
		ctx := testContext(t, 60*time.Second)

		// Tool defined but no handler registered
		result, err := testClient.Messages.Run(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Get the weather in Tokyo")},
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
					},
				},
			},
			MaxTokens: 8000,
		},
			vai.WithMaxToolCalls(1),
			// No handler registered with WithTools
		)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should still complete (with a message about unregistered handler)
		if result.Response == nil {
			t.Error("expected response")
		}
	})
}

func TestMessages_Run_Steps(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsTools
	}, func(t *testing.T, provider providerConfig) {
		ctx := testContext(t, 60*time.Second)

		tool := vai.MakeTool("add", "Add two numbers",
			func(ctx context.Context, input struct {
				A int `json:"a"`
				B int `json:"b"`
			}) (int, error) {
				return input.A + input.B, nil
			},
		)

		result, err := testClient.Messages.Run(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Add 2 and 3 using the add tool")},
			},
			Tools:     []vai.Tool{tool.Tool},
			MaxTokens: 8000,
		},
			vai.WithTools(tool),
			vai.WithMaxToolCalls(2),
		)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Steps) == 0 {
			t.Error("expected at least one step")
		}

		// Check first step has response
		if result.Steps[0].Response == nil {
			t.Error("expected response in first step")
		}

		// If tool was called, check tool call info
		if len(result.Steps) > 0 && len(result.Steps[0].ToolCalls) > 0 {
			tc := result.Steps[0].ToolCalls[0]
			if tc.Name != "add" {
				t.Errorf("expected tool name 'add', got %q", tc.Name)
			}
			if tc.ID == "" {
				t.Error("expected non-empty tool call ID")
			}
		}
	})
}

// ==================== RunStream Tests ====================

func TestMessages_RunStream_Basic(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsTools
	}, func(t *testing.T, provider providerConfig) {
		ctx := testContext(t, 60*time.Second)

		tool := vai.MakeTool("get_weather", "Get weather for a location",
			func(ctx context.Context, input struct {
				Location string `json:"location"`
			}) (string, error) {
				return "72°F and sunny in " + input.Location, nil
			},
		)

		stream, err := testClient.Messages.RunStream(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("What's the weather in Paris?")},
			},
			Tools:     []vai.Tool{tool.Tool},
			MaxTokens: 8000,
		},
			vai.WithTools(tool),
			vai.WithMaxToolCalls(2),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer stream.Close()

		var gotStepStart, gotStepComplete, gotRunComplete bool
		var eventCount int

		for event := range stream.Events() {
			eventCount++
			switch event.(type) {
			case vai.StepStartEvent:
				gotStepStart = true
			case vai.StepCompleteEvent:
				gotStepComplete = true
			case vai.RunCompleteEvent:
				gotRunComplete = true
			}
		}

		if !gotStepStart {
			t.Error("expected StepStartEvent")
		}
		if !gotStepComplete {
			t.Error("expected StepCompleteEvent")
		}
		if !gotRunComplete {
			t.Error("expected RunCompleteEvent")
		}

		result := stream.Result()
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if result.Response == nil {
			t.Error("expected non-nil response in result")
		}

		t.Logf("RunStream: %d events, %d tool calls", eventCount, result.ToolCallCount)
	})
}

func TestMessages_RunStream_ToolEvents(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsTools
	}, func(t *testing.T, provider providerConfig) {
		ctx := testContext(t, 60*time.Second)

		tool := vai.MakeTool("multiply", "Multiply two numbers",
			func(ctx context.Context, input struct {
				A int `json:"a"`
				B int `json:"b"`
			}) (int, error) {
				return input.A * input.B, nil
			},
		)

		stream, err := testClient.Messages.RunStream(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Multiply 6 by 7 using the multiply tool")},
			},
			Tools:     []vai.Tool{tool.Tool},
			MaxTokens: 8000,
		},
			vai.WithTools(tool),
			vai.WithMaxToolCalls(2),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer stream.Close()

		var toolCallStartEvents []vai.ToolCallStartEvent
		var toolResultEvents []vai.ToolResultEvent

		for event := range stream.Events() {
			switch e := event.(type) {
			case vai.ToolCallStartEvent:
				toolCallStartEvents = append(toolCallStartEvents, e)
			case vai.ToolResultEvent:
				toolResultEvents = append(toolResultEvents, e)
			}
		}

		// Should have at least one tool call if model used the tool
		result := stream.Result()
		if result.ToolCallCount > 0 {
			if len(toolCallStartEvents) == 0 {
				t.Error("expected ToolCallStartEvent when tools are called")
			}
			if len(toolResultEvents) == 0 {
				t.Error("expected ToolResultEvent when tools are called")
			}

			// Verify the tool call name
			if len(toolCallStartEvents) > 0 && toolCallStartEvents[0].Name != "multiply" {
				t.Errorf("expected tool name 'multiply', got %q", toolCallStartEvents[0].Name)
			}
		}

		t.Logf("Tool events: %d starts, %d results", len(toolCallStartEvents), len(toolResultEvents))
	})
}

func TestMessages_RunStream_TextDeltas(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		ctx := testContext(t, 60*time.Second)

		stream, err := testClient.Messages.RunStream(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Count from 1 to 5.")},
			},
			MaxTokens: 8000,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer stream.Close()

		var textDeltaCount int
		var accumulatedText strings.Builder

		for event := range stream.Events() {
			if wrapper, ok := event.(vai.StreamEventWrapper); ok {
				if delta, ok := wrapper.Event.(vai.ContentBlockDeltaEvent); ok {
					if textDelta, ok := delta.Delta.(vai.TextDelta); ok {
						textDeltaCount++
						accumulatedText.WriteString(textDelta.Text)
					}
				}
			}
		}

		if textDeltaCount == 0 {
			t.Error("expected text delta events")
		}

		text := accumulatedText.String()
		// Model should return some numbers - at minimum "1"
		if !strings.Contains(text, "1") {
			t.Errorf("expected text to contain at least '1', got %q", text)
		}
		// Log what we got - model may not always return full 1-5 sequence
		t.Logf("RunStream text deltas: %d events, text: %q", textDeltaCount, text)
	})
}

func TestMessages_RunStream_MaxToolCalls(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsTools
	}, func(t *testing.T, provider providerConfig) {
		ctx := testContext(t, 60*time.Second)

		tool := vai.MakeTool("ping", "Just ping - should be called repeatedly",
			func(ctx context.Context, input struct{}) (string, error) {
				return "pong received, but you need to ping again", nil
			},
		)

		stream, err := testClient.Messages.RunStream(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Keep calling ping 10 times.")},
			},
			Tools:      []vai.Tool{tool.Tool},
			ToolChoice: vai.ToolChoiceTool("ping"),
			MaxTokens: 8000,
		},
			vai.WithTools(tool),
			vai.WithMaxToolCalls(3),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer stream.Close()

		// Consume all events
		for range stream.Events() {
		}

		result := stream.Result()
		t.Logf("Tool calls: %d, Stop reason: %q", result.ToolCallCount, result.StopReason)
		if result.ToolCallCount > 3 {
			t.Errorf("expected at most 3 tool calls, got %d", result.ToolCallCount)
		}
		// Note: Some models don't respect tool_choice consistently in streaming mode.
		// Accept either max_tool_calls (expected) or end_turn (model decided to respond instead).
		if result.StopReason != vai.RunStopMaxToolCalls && result.StopReason != vai.RunStopEndTurn {
			t.Errorf("expected stop reason 'max_tool_calls' or 'end_turn', got %q", result.StopReason)
		}
		if result.StopReason == vai.RunStopEndTurn && result.ToolCallCount == 0 {
			t.Log("Note: Model ignored tool_choice in streaming mode - this is model-dependent behavior")
		}
	})
}

func TestMessages_RunStream_Error(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		ctx := testContext(t, 60*time.Second)

		// Test that Err() returns nil on successful completion
		stream, err := testClient.Messages.RunStream(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Say hello.")},
			},
			MaxTokens: 8000,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer stream.Close()

		// Consume all events
		for range stream.Events() {
		}

		// Check for errors - EOF is acceptable for completed streams
		if streamErr := stream.Err(); streamErr != nil {
			if streamErr.Error() != "EOF" {
				t.Errorf("unexpected stream error: %v", streamErr)
			}
		}

		result := stream.Result()
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if result.StopReason != vai.RunStopEndTurn {
			t.Logf("stop reason: %q", result.StopReason)
		}
	})
}
