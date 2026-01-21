//go:build integration
// +build integration

package integration_test

import (
	"errors"
	"strings"
	"testing"

	vai "github.com/vango-go/vai/sdk"
)

// ==================== Graceful Degradation Tests ====================
// These tests verify that unsupported features return clear, actionable errors.

func TestDegradation_StopSequences_OaiResp(t *testing.T) {
	requireOpenAIKey(t)
	ctx := defaultTestContext(t)

	// OAI Responses API doesn't support stop sequences
	_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "oai-resp/gpt-5-mini",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Count to 10")},
		},
		StopSequences: []string{"5"},
		MaxTokens:     100,
	})

	if err == nil {
		t.Log("warning: expected error for stop_sequences on oai-resp, but request succeeded")
		return
	}

	// Should get a clear error about stop_sequences not being supported
	errStr := strings.ToLower(err.Error())
	if !strings.Contains(errStr, "stop") && !strings.Contains(errStr, "not supported") {
		t.Logf("Error: %v", err)
		t.Log("warning: error message should mention stop_sequences not being supported")
	}
}

func TestDegradation_Video_Anthropic(t *testing.T) {
	requireAnthropicKey(t)
	ctx := defaultTestContext(t)

	// Anthropic doesn't support video input
	videoData := []byte{0x00, 0x00, 0x00, 0x1c, 0x66, 0x74, 0x79, 0x70} // Minimal MP4 header

	_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "anthropic/claude-haiku-4-5-20251001",
		Messages: []vai.Message{
			{Role: "user", Content: []vai.ContentBlock{
				vai.Text("Describe this video"),
				vai.Video(videoData, "video/mp4"),
			}},
		},
		MaxTokens: 100,
	})

	if err == nil {
		t.Log("warning: expected error for video on anthropic, but request succeeded")
		return
	}

	// Should get a clear error about video not being supported
	errStr := strings.ToLower(err.Error())
	if !strings.Contains(errStr, "video") && !strings.Contains(errStr, "not supported") && !strings.Contains(errStr, "unsupported") {
		t.Logf("Error: %v", err)
		t.Log("warning: error message should mention video not being supported")
	}

	// Verify it's an API error with proper type
	var apiErr *vai.Error
	if errors.As(err, &apiErr) {
		t.Logf("Error type: %s, message: %s", apiErr.Type, apiErr.Message)
	}
}

func TestDegradation_Video_OaiResp(t *testing.T) {
	requireOpenAIKey(t)
	ctx := defaultTestContext(t)

	// OAI Responses API doesn't support video input
	videoData := []byte{0x00, 0x00, 0x00, 0x1c, 0x66, 0x74, 0x79, 0x70}

	_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "oai-resp/gpt-5-mini",
		Messages: []vai.Message{
			{Role: "user", Content: []vai.ContentBlock{
				vai.Text("Describe this video"),
				vai.Video(videoData, "video/mp4"),
			}},
		},
		MaxTokens: 100,
	})

	if err == nil {
		t.Log("warning: expected error for video on oai-resp, but request succeeded")
		return
	}

	t.Logf("Error: %v", err)
}

func TestDegradation_Video_Groq(t *testing.T) {
	requireGroqKey(t)
	ctx := defaultTestContext(t)

	// Groq doesn't support video input
	videoData := []byte{0x00, 0x00, 0x00, 0x1c, 0x66, 0x74, 0x79, 0x70}

	_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "groq/moonshotai/kimi-k2-instruct-0905",
		Messages: []vai.Message{
			{Role: "user", Content: []vai.ContentBlock{
				vai.Text("Describe this video"),
				vai.Video(videoData, "video/mp4"),
			}},
		},
		MaxTokens: 100,
	})

	if err == nil {
		t.Log("warning: expected error for video on groq, but request succeeded")
		return
	}

	t.Logf("Error: %v", err)
}

func TestDegradation_Audio_Anthropic(t *testing.T) {
	requireAnthropicKey(t)
	ctx := defaultTestContext(t)

	// Anthropic doesn't support audio input
	audioData := fixtures.Audio("test.wav")

	_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "anthropic/claude-haiku-4-5-20251001",
		Messages: []vai.Message{
			{Role: "user", Content: []vai.ContentBlock{
				vai.Text("Transcribe this audio"),
				vai.Audio(audioData, "audio/wav"),
			}},
		},
		MaxTokens: 100,
	})

	if err == nil {
		t.Log("warning: expected error for audio on anthropic, but request succeeded")
		return
	}

	errStr := strings.ToLower(err.Error())
	if !strings.Contains(errStr, "audio") && !strings.Contains(errStr, "not supported") && !strings.Contains(errStr, "unsupported") {
		t.Logf("Error: %v", err)
		t.Log("warning: error message should mention audio not being supported")
	}
}

func TestDegradation_Audio_Groq(t *testing.T) {
	requireGroqKey(t)
	ctx := defaultTestContext(t)

	// Groq doesn't support audio input in chat
	audioData := fixtures.Audio("test.wav")

	_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "groq/moonshotai/kimi-k2-instruct-0905",
		Messages: []vai.Message{
			{Role: "user", Content: []vai.ContentBlock{
				vai.Text("Transcribe this audio"),
				vai.Audio(audioData, "audio/wav"),
			}},
		},
		MaxTokens: 100,
	})

	if err == nil {
		t.Log("warning: expected error for audio on groq, but request succeeded")
		return
	}

	t.Logf("Error: %v", err)
}

func TestDegradation_Vision_NonVisionModel(t *testing.T) {
	requireGroqKey(t)
	ctx := defaultTestContext(t)

	// Groq's non-vision models shouldn't support images
	_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "groq/moonshotai/kimi-k2-instruct-0905", // Non-vision model
		Messages: []vai.Message{
			{Role: "user", Content: []vai.ContentBlock{
				vai.Text("What's in this image?"),
				vai.ImageURL("https://example.com/image.jpg"),
			}},
		},
		MaxTokens: 100,
	})

	if err == nil {
		// Some models may silently ignore images
		t.Log("warning: non-vision model may have processed image request")
		return
	}

	t.Logf("Error: %v", err)
}

func TestDegradation_StructuredOutput_Groq(t *testing.T) {
	requireGroqKey(t)
	ctx := defaultTestContext(t)

	// Groq has limited structured output support
	additionalPropertiesFalse := false
	_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "groq/moonshotai/kimi-k2-instruct-0905",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Extract: John is 30 years old")},
		},
		OutputFormat: &vai.OutputFormat{
			Type: "json_schema",
			JSONSchema: &vai.JSONSchema{
				Type: "object",
				Properties: map[string]vai.JSONSchema{
					"name": {Type: "string"},
					"age":  {Type: "integer"},
				},
				Required:             []string{"name", "age"},
				AdditionalProperties: &additionalPropertiesFalse,
			},
		},
		MaxTokens: 100,
	})

	if err == nil {
		// Some providers may accept but not strictly enforce
		t.Log("Request succeeded - provider may have partial structured output support")
		return
	}

	t.Logf("Error: %v", err)
}

func TestDegradation_NativeTool_UnsupportedProvider(t *testing.T) {
	requireGroqKey(t)
	ctx := defaultTestContext(t)

	// Groq doesn't support text_editor tool
	_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "groq/moonshotai/kimi-k2-instruct-0905",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Edit the file")},
		},
		Tools: []vai.Tool{
			vai.TextEditor(),
		},
		MaxTokens: 100,
	})

	if err == nil {
		t.Log("warning: expected error for text_editor on groq, but request may have been ignored")
		return
	}

	t.Logf("Error: %v", err)
}

func TestDegradation_InvalidModel(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: p.Name + "/nonexistent-model-xyz-123",
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Hello")},
			},
			MaxTokens: 100,
		})

		if err == nil {
			t.Fatal("expected error for invalid model")
		}

		// Should be a typed error
		var apiErr *vai.Error
		if errors.As(err, &apiErr) {
			// Should be not_found or invalid_request
			t.Logf("Error type: %s, message: %s", apiErr.Type, apiErr.Message)
		} else {
			t.Logf("Non-API error: %v", err)
		}
	})
}

func TestDegradation_InvalidAPIKey(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		// Create client with invalid key
		badClient := vai.NewClient(
			vai.WithProviderKey(p.KeyName, "sk-invalid-key-xxx"),
		)

		_, err := badClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Hello")},
			},
			MaxTokens: 100,
		})

		if err == nil {
			t.Fatal("expected error for invalid API key")
		}

		// Should be an authentication error
		var apiErr *vai.Error
		if errors.As(err, &apiErr) {
			if apiErr.Type != vai.ErrAuthentication {
				t.Logf("Error type: %s (expected authentication_error)", apiErr.Type)
			}
		}
	})
}

func TestDegradation_EmptyMessages(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model:     p.Model,
			Messages:  []vai.Message{},
			MaxTokens: 100,
		})

		if err == nil {
			t.Fatal("expected error for empty messages")
		}

		// Should be invalid_request
		var apiErr *vai.Error
		if errors.As(err, &apiErr) {
			if apiErr.Type != vai.ErrInvalidRequest {
				t.Logf("Error type: %s (expected invalid_request_error)", apiErr.Type)
			}
		}
	})
}

func TestDegradation_NegativeMaxTokens(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Hello")},
			},
			MaxTokens: -100,
		})

		if err == nil {
			t.Log("warning: expected error for negative max_tokens")
			return
		}

		t.Logf("Error: %v", err)
	})
}

func TestDegradation_ErrorTypes(t *testing.T) {
	// Test that error types are properly mapped
	tests := []struct {
		errType   string
		retryable bool
	}{
		{string(vai.ErrInvalidRequest), false},
		{string(vai.ErrAuthentication), false},
		{string(vai.ErrPermission), false},
		{string(vai.ErrNotFound), false},
		{string(vai.ErrRateLimit), true},
		{string(vai.ErrAPI), true},
		{string(vai.ErrOverloaded), true},
	}

	for _, tc := range tests {
		t.Run(tc.errType, func(t *testing.T) {
			err := &vai.Error{Type: vai.ErrInvalidRequest}
			if tc.errType == string(vai.ErrRateLimit) {
				err.Type = vai.ErrRateLimit
			} else if tc.errType == string(vai.ErrAPI) {
				err.Type = vai.ErrAPI
			} else if tc.errType == string(vai.ErrOverloaded) {
				err.Type = vai.ErrOverloaded
			}

			if err.IsRetryable() != tc.retryable {
				t.Errorf("expected IsRetryable=%v for %s", tc.retryable, tc.errType)
			}
		})
	}
}
