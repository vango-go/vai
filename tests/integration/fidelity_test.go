//go:build integration
// +build integration

package integration_test

import (
	"strings"
	"testing"
	"time"

	vai "github.com/vango-go/vai/sdk"
)

// ==================== Response Fidelity Tests ====================
// These tests verify that every field in the Vango response is populated
// correctly from each provider.

func TestFidelity_AllResponseFields(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model:     p.Model,
			MaxTokens: 50,
			Messages:  []vai.Message{{Role: "user", Content: vai.Text("Say 'hi'")}},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify all required fields are populated
		if resp.ID == "" {
			t.Error("id field is empty")
		}
		if resp.Type != "message" {
			t.Errorf("expected type 'message', got %q", resp.Type)
		}
		if resp.Role != "assistant" {
			t.Errorf("expected role 'assistant', got %q", resp.Role)
		}
		if resp.Model == "" {
			t.Error("model field is empty")
		}
		// Model should be in provider/name format
		if !strings.Contains(resp.Model, "/") {
			t.Errorf("model should be provider/name format, got %q", resp.Model)
		}
		if len(resp.Content) == 0 {
			t.Error("content is empty")
		}
		if resp.StopReason == "" {
			t.Error("stop_reason is empty")
		}

		// Usage must have valid token counts
		if resp.Usage.InputTokens == 0 {
			t.Error("usage.input_tokens is 0")
		}
		if resp.Usage.OutputTokens == 0 {
			t.Error("usage.output_tokens is 0")
		}

		t.Logf("Response: id=%s, model=%s, stop_reason=%s, usage=%d/%d",
			resp.ID, resp.Model, resp.StopReason, resp.Usage.InputTokens, resp.Usage.OutputTokens)
	})
}

func TestFidelity_TextContent(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model:     p.Model,
			MaxTokens: 100,
			Messages:  []vai.Message{{Role: "user", Content: vai.Text("Say 'Hello World' exactly")}},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		text := resp.TextContent()
		if text == "" {
			t.Error("TextContent() returned empty string")
		}
		if !strings.Contains(strings.ToLower(text), "hello") {
			t.Logf("Response text: %s", text)
			t.Log("warning: expected 'hello' in response")
		}

		// Verify content block structure
		if len(resp.Content) == 0 {
			t.Fatal("no content blocks")
		}
		firstBlock := resp.Content[0]
		if firstBlock.BlockType() != "text" {
			t.Errorf("expected first block type 'text', got %q", firstBlock.BlockType())
		}
	})
}

func TestFidelity_ToolUseResponse(t *testing.T) {
	forEachProviderWith(t, func(p providerConfig) bool {
		return p.SupportsTools
	}, func(t *testing.T, p providerConfig) {
		ctx := testContext(t, 60*time.Second)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("What's the weather in Paris?")},
			},
			Tools: []vai.Tool{
				{
					Type:        "function",
					Name:        "get_weather",
					Description: "Get weather for a location",
					InputSchema: &vai.JSONSchema{
						Type: "object",
						Properties: map[string]vai.JSONSchema{
							"location": {Type: "string", Description: "City name"},
						},
						Required: []string{"location"},
					},
				},
			},
			ToolChoice: vai.ToolChoiceTool("get_weather"),
			MaxTokens:  8000,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify stop reason
		if resp.StopReason != vai.StopReasonToolUse {
			t.Errorf("expected stop_reason 'tool_use', got %q", resp.StopReason)
		}

		// Verify tool use blocks
		toolUses := resp.ToolUses()
		if len(toolUses) == 0 {
			t.Fatal("expected at least one tool use")
		}

		tu := toolUses[0]
		// Verify tool use block fields
		if tu.ID == "" {
			t.Error("tool_use.id is empty")
		}
		if tu.Name != "get_weather" {
			t.Errorf("expected tool name 'get_weather', got %q", tu.Name)
		}
		if tu.Input == nil {
			t.Error("tool_use.input is nil")
		}
		if _, ok := tu.Input["location"]; !ok {
			t.Error("expected 'location' in tool input")
		}

		t.Logf("Tool use: id=%s, name=%s, input=%v", tu.ID, tu.Name, tu.Input)
	})
}

func TestFidelity_StreamingAssembly(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		// First, get non-streamed response
		nonStreamResp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model:     p.Model,
			MaxTokens: 100,
			Messages:  []vai.Message{{Role: "user", Content: vai.Text("Say 'test123'")}},
		})
		if err != nil {
			t.Fatalf("non-stream error: %v", err)
		}

		// Now get streamed response
		stream, err := testClient.Messages.Stream(ctx, &vai.MessageRequest{
			Model:     p.Model,
			MaxTokens: 100,
			Messages:  []vai.Message{{Role: "user", Content: vai.Text("Say 'test123'")}},
		})
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
		defer stream.Close()

		// Consume all events
		for range stream.Events() {
		}

		streamResp := stream.Response()
		if streamResp == nil {
			t.Fatal("stream.Response() is nil")
		}

		// Compare structure (not content, as models may vary)
		if streamResp.Type != nonStreamResp.Type {
			t.Errorf("type mismatch: stream=%s, non-stream=%s", streamResp.Type, nonStreamResp.Type)
		}
		if streamResp.Role != nonStreamResp.Role {
			t.Errorf("role mismatch: stream=%s, non-stream=%s", streamResp.Role, nonStreamResp.Role)
		}
		// Both should have content
		if len(streamResp.Content) == 0 {
			t.Error("streamed response has no content")
		}
		// Both should have usage
		if streamResp.Usage.InputTokens == 0 {
			t.Error("streamed response has no input tokens")
		}
		if streamResp.Usage.OutputTokens == 0 {
			t.Error("streamed response has no output tokens")
		}
	})
}

func TestFidelity_UsageDetails(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model:     p.Model,
			MaxTokens: 100,
			System:    "You are a helpful assistant.",
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("What is 2+2? Be brief.")},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify usage is populated
		usage := resp.Usage
		if usage.InputTokens == 0 {
			t.Error("input_tokens is 0")
		}
		if usage.OutputTokens == 0 {
			t.Error("output_tokens is 0")
		}

		// Input should be higher due to system prompt
		if usage.InputTokens < 5 {
			t.Logf("warning: input_tokens seems low: %d", usage.InputTokens)
		}

		t.Logf("Usage: input=%d, output=%d", usage.InputTokens, usage.OutputTokens)
	})
}

func TestFidelity_StopReasonEndTurn(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model:     p.Model,
			MaxTokens: 1000,
			Messages:  []vai.Message{{Role: "user", Content: vai.Text("Say 'done'")}},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.StopReason != vai.StopReasonEndTurn {
			t.Errorf("expected stop_reason 'end_turn', got %q", resp.StopReason)
		}
	})
}

func TestFidelity_StopReasonMaxTokens(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model:     p.Model,
			MaxTokens: 5, // Very short to force truncation
			Messages:  []vai.Message{{Role: "user", Content: vai.Text("Write a long story about dragons.")}},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.StopReason != vai.StopReasonMaxTokens {
			t.Errorf("expected stop_reason 'max_tokens', got %q", resp.StopReason)
		}
	})
}

func TestFidelity_StopReasonStopSequence(t *testing.T) {
	forEachProviderWith(t, func(p providerConfig) bool {
		return p.SupportsStopSequences
	}, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model:         p.Model,
			MaxTokens:     100,
			Messages:      []vai.Message{{Role: "user", Content: vai.Text("Count from 1 to 10")}},
			StopSequences: []string{"5"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.StopReason != vai.StopReasonStopSequence {
			t.Errorf("expected stop_reason 'stop_sequence', got %q", resp.StopReason)
		}
		if resp.StopSequence == nil || *resp.StopSequence != "5" {
			if resp.StopSequence == nil {
				t.Error("stop_sequence is nil")
			} else {
				t.Errorf("expected stop_sequence '5', got %q", *resp.StopSequence)
			}
		}
	})
}

func TestFidelity_ModelNameFormat(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model:     p.Model,
			MaxTokens: 10,
			Messages:  []vai.Message{{Role: "user", Content: vai.Text("Hi")}},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Model should contain provider prefix
		if !strings.HasPrefix(resp.Model, p.Name+"/") {
			t.Errorf("model should start with %q/, got %q", p.Name, resp.Model)
		}
	})
}
