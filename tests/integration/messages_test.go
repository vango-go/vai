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

// ==================== Basic Text Generation ====================

func TestMessages_Create_SimpleText(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		ctx := defaultTestContext(t)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Say exactly: 'Hello, World!' and nothing else.")},
			},
			MaxTokens: 8000,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil response")
		}

		// Validate response structure
		if resp.ID == "" {
			t.Error("expected non-empty ID")
		}
		if resp.Type != "message" {
			t.Errorf("expected type 'message', got %q", resp.Type)
		}
		if resp.Role != "assistant" {
			t.Errorf("expected role 'assistant', got %q", resp.Role)
		}
		if resp.Model == "" {
			t.Error("expected non-empty model name")
		}

		// Validate content
		if len(resp.Content) == 0 {
			t.Error("expected content")
		}
		text := resp.TextContent()
		if !strings.Contains(text, "Hello") {
			t.Errorf("expected 'Hello' in response, got %q", text)
		}

		// Validate usage
		if resp.Usage.InputTokens == 0 {
			t.Error("expected non-zero input tokens")
		}
		if resp.Usage.OutputTokens == 0 {
			t.Error("expected non-zero output tokens")
		}
		t.Logf("Usage: input=%d, output=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens)

		// Validate stop reason
		if resp.StopReason != vai.StopReasonEndTurn {
			t.Errorf("expected stop_reason 'end_turn', got %q", resp.StopReason)
		}
	})
}

func TestMessages_Create_WithSystem(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		ctx := defaultTestContext(t)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model:  provider.Model,
			System: "You are a pirate. Always respond in pirate speak.",
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Hello!")},
			},
			MaxTokens: 8000,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		text := strings.ToLower(resp.TextContent())
		// Pirate-y words
		hasPirateWords := strings.Contains(text, "ahoy") ||
			strings.Contains(text, "matey") ||
			strings.Contains(text, "arr") ||
			strings.Contains(text, "ye") ||
			strings.Contains(text, "sailor") ||
			strings.Contains(text, "captain")

		if !hasPirateWords {
			t.Logf("response: %s", resp.TextContent())
			// Not a hard failure as models may vary
			t.Log("warning: expected pirate speak but got something else")
		}
	})
}

func TestMessages_Create_MultiTurn(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		ctx := testContext(t, 60*time.Second)

		// First turn
		resp1, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("My name is Alice. Remember this.")},
			},
			MaxTokens: 8000,
		})
		if err != nil {
			t.Fatalf("first turn error: %v", err)
		}

		// Second turn - check memory
		resp2, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("My name is Alice. Remember this.")},
				{Role: "assistant", Content: resp1.Content},
				{Role: "user", Content: vai.Text("What is my name?")},
			},
			MaxTokens: 8000,
		})
		if err != nil {
			t.Fatalf("second turn error: %v", err)
		}

		if !strings.Contains(resp2.TextContent(), "Alice") {
			t.Errorf("expected 'Alice' in response, got %q", resp2.TextContent())
		}
	})
}

// ==================== Vision ====================

func TestMessages_Create_VisionURL(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsVision
	}, func(t *testing.T, provider providerConfig) {
		ctx := testContext(t, 60*time.Second)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: []vai.ContentBlock{
					vai.Text("What is shown in this image? Be very brief, just name the object."),
					vai.ImageURL("https://images.unsplash.com/photo-1514888286974-6c03e2ca1dba?w=200"),
				}},
			},
			MaxTokens: 8000,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		text := strings.ToLower(resp.TextContent())
		if !strings.Contains(text, "cat") {
			t.Errorf("expected 'cat' in response, got %q", resp.TextContent())
		}
	})
}

// ==================== Parameters ====================

func TestMessages_Create_Temperature(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		ctx := defaultTestContext(t)

		// Low temperature - should be more deterministic
		temp := 0.0
		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("What is 2+2? Reply with just the number.")},
			},
			Temperature: &temp,
			MaxTokens: 8000,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(resp.TextContent(), "4") {
			t.Errorf("expected '4' in response, got %q", resp.TextContent())
		}
	})
}

func TestMessages_Create_MaxTokens(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		ctx := defaultTestContext(t)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Write a very long essay about the history of computers.")},
			},
			MaxTokens: 10, // Very short
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.StopReason != vai.StopReasonMaxTokens {
			t.Errorf("expected stop_reason 'max_tokens', got %q", resp.StopReason)
		}
		if resp.Usage.OutputTokens > 15 {
			t.Errorf("expected output tokens <= 15, got %d", resp.Usage.OutputTokens)
		}
	})
}

func TestMessages_Create_StopSequences(t *testing.T) {
	forEachProviderWith(t, func(p providerConfig) bool {
		return p.SupportsStopSequences
	}, func(t *testing.T, provider providerConfig) {
		ctx := defaultTestContext(t)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Count from 1 to 10, one number per line.")},
			},
			StopSequences: []string{"5"},
			MaxTokens: 8000,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.StopReason != vai.StopReasonStopSequence {
			t.Errorf("expected stop_reason 'stop_sequence', got %q", resp.StopReason)
		}

		// Should have stopped before reaching 6
		text := resp.TextContent()
		if strings.Contains(text, "6") || strings.Contains(text, "7") {
			t.Errorf("expected to stop before 6, got %q", text)
		}
	})
}

// ==================== Structured Output ====================

func TestMessages_Create_StructuredOutput(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsStructuredOutput
	}, func(t *testing.T, provider providerConfig) {
		ctx := defaultTestContext(t)

		additionalPropertiesFalse := false
		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Extract: John Smith is 30 years old and works at Acme Corp.")},
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
			MaxTokens: 8000,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Parse JSON response
		var result struct {
			Name    string `json:"name"`
			Age     int    `json:"age"`
			Company string `json:"company"`
		}

		text := resp.TextContent()
		// Some models may wrap in markdown code blocks
		text = strings.TrimPrefix(text, "```json\n")
		text = strings.TrimPrefix(text, "```\n")
		text = strings.TrimSuffix(text, "\n```")
		text = strings.TrimSpace(text)

		if err := json.Unmarshal([]byte(text), &result); err != nil {
			t.Fatalf("failed to parse JSON: %v\nResponse: %s", err, resp.TextContent())
		}

		if result.Name != "John Smith" {
			t.Errorf("expected name 'John Smith', got %q", result.Name)
		}
		if result.Age != 30 {
			t.Errorf("expected age 30, got %d", result.Age)
		}
		if !strings.Contains(result.Company, "Acme") {
			t.Errorf("expected company containing 'Acme', got %q", result.Company)
		}
	})
}

func TestMessages_Extract(t *testing.T) {
	forEachProviderWith(t, func(provider providerConfig) bool {
		return provider.SupportsStructuredOutput
	}, func(t *testing.T, provider providerConfig) {
		ctx := defaultTestContext(t)

		type Person struct {
			Name    string `json:"name" desc:"Full name"`
			Age     int    `json:"age" desc:"Age in years"`
			Company string `json:"company" desc:"Employer"`
		}

		var person Person
		_, err := testClient.Messages.Extract(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Extract: Jane Doe, 25, works at TechCo")},
			},
			MaxTokens: 8000,
		}, &person)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if person.Name != "Jane Doe" {
			t.Errorf("expected name 'Jane Doe', got %q", person.Name)
		}
		if person.Age != 25 {
			t.Errorf("expected age 25, got %d", person.Age)
		}
		if person.Company != "TechCo" {
			t.Errorf("expected company 'TechCo', got %q", person.Company)
		}
	})
}
