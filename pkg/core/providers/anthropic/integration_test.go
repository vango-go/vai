//go:build integration
// +build integration

package anthropic_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	vai "github.com/vango-go/vai/sdk"
)

// Integration tests that hit the real Anthropic API.
// Run with: go test -tags=integration ./pkg/core/providers/anthropic/... -v

func TestIntegration_CreateMessage_SimpleText(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	client := vai.NewClient(vai.WithProviderKey("anthropic", apiKey))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.Messages.Create(ctx, &vai.MessageRequest{
		Model: "anthropic/claude-sonnet-4",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Say 'Hello, World!' and nothing else.")},
		},
		MaxTokens: 50,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID == "" {
		t.Error("expected non-empty ID")
	}
	if resp.Role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", resp.Role)
	}
	if !strings.Contains(resp.TextContent(), "Hello") {
		t.Errorf("expected 'Hello' in response, got %q", resp.TextContent())
	}
	if resp.Usage.InputTokens == 0 {
		t.Error("expected non-zero input tokens")
	}
	if resp.Usage.OutputTokens == 0 {
		t.Error("expected non-zero output tokens")
	}
}

func TestIntegration_Stream_SimpleText(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	client := vai.NewClient(vai.WithProviderKey("anthropic", apiKey))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stream, err := client.Messages.Stream(ctx, &vai.MessageRequest{
		Model: "anthropic/claude-sonnet-4",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Count from 1 to 5.")},
		},
		MaxTokens: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	var gotTextDelta bool
	var textParts []string

	for event := range stream.Events() {
		if event == nil {
			continue
		}
		switch e := event.(type) {
		case vai.ContentBlockDeltaEvent:
			if delta, ok := e.Delta.(vai.TextDelta); ok {
				gotTextDelta = true
				textParts = append(textParts, delta.Text)
			}
		}
	}

	if !gotTextDelta {
		t.Error("expected text delta events")
	}

	fullText := strings.Join(textParts, "")
	t.Logf("Streamed text: %s", fullText)

	// Also verify final response
	resp := stream.Response()
	if resp == nil {
		t.Fatal("expected response after stream")
	}
	if resp.TextContent() == "" {
		t.Error("expected non-empty text content in response")
	}
}

func TestIntegration_CreateMessage_WithImage(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	client := vai.NewClient(vai.WithProviderKey("anthropic", apiKey))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Use a simple public image URL
	resp, err := client.Messages.Create(ctx, &vai.MessageRequest{
		Model: "anthropic/claude-sonnet-4",
		Messages: []vai.Message{
			{Role: "user", Content: []vai.ContentBlock{
				vai.Text("What color is this image? Reply with just the color name."),
				// Use a tiny image URL that Anthropic can fetch
				vai.ImageURL("https://upload.wikimedia.org/wikipedia/commons/thumb/e/e8/Red_square.svg/50px-Red_square.svg.png"),
			}},
		},
		MaxTokens: 50,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := strings.ToLower(resp.TextContent())
	if !strings.Contains(text, "red") {
		t.Logf("Response: %s", resp.TextContent())
		t.Errorf("expected 'red' in response for red square image")
	}
}
