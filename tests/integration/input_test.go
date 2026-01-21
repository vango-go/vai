//go:build integration
// +build integration

package integration_test

import (
	"strings"
	"testing"
	"time"

	vai "github.com/vango-go/vai/sdk"
)

// ==================== Input Content Type Tests ====================
// These tests verify that different input content types work correctly.

func TestInput_ImageURL(t *testing.T) {
	forEachProviderWith(t, func(p providerConfig) bool {
		return p.SupportsVision
	}, func(t *testing.T, p providerConfig) {
		ctx := testContext(t, 60*time.Second)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: []vai.ContentBlock{
					vai.Text("What animal is in this image? Answer with just the animal name."),
					vai.ImageURL("https://images.unsplash.com/photo-1514888286974-6c03e2ca1dba?w=200"),
				}},
			},
			MaxTokens: 100,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		text := strings.ToLower(resp.TextContent())
		if !strings.Contains(text, "cat") {
			t.Logf("Response: %s", resp.TextContent())
			t.Log("warning: expected 'cat' in response")
		}
	})
}

func TestInput_ImageBase64(t *testing.T) {
	forEachProviderWith(t, func(p providerConfig) bool {
		// Skip Gemini as it rejects the minimal 1x1 PNG - use TestInput_ImageURL instead
		return p.SupportsVision && p.Name != "gemini"
	}, func(t *testing.T, p providerConfig) {
		ctx := testContext(t, 60*time.Second)

		// Use fixture image (minimal PNG)
		imageData := fixtures.Image("test.png")

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: []vai.ContentBlock{
					vai.Text("Is there an image attached? Reply 'yes' or 'no'."),
					vai.Image(imageData, "image/png"),
				}},
			},
			MaxTokens: 50,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		text := strings.ToLower(resp.TextContent())
		if !strings.Contains(text, "yes") {
			t.Logf("Response: %s", resp.TextContent())
			// The minimal fixture may not be recognized as a meaningful image
			t.Log("warning: model may not have recognized the image")
		}
	})
}

func TestInput_MultipleImages(t *testing.T) {
	forEachProviderWith(t, func(p providerConfig) bool {
		return p.SupportsVision
	}, func(t *testing.T, p providerConfig) {
		ctx := testContext(t, 60*time.Second)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: []vai.ContentBlock{
					vai.Text("I'm showing you two images. Just say 'two images received'."),
					vai.ImageURL("https://images.unsplash.com/photo-1514888286974-6c03e2ca1dba?w=100"),
					vai.ImageURL("https://images.unsplash.com/photo-1518791841217-8f162f1e1131?w=100"),
				}},
			},
			MaxTokens: 50,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		text := strings.ToLower(resp.TextContent())
		if !strings.Contains(text, "two") && !strings.Contains(text, "2") {
			t.Logf("Response: %s", resp.TextContent())
			t.Log("warning: expected acknowledgment of two images")
		}
	})
}

func TestInput_AudioTranscription(t *testing.T) {
	forEachProviderWith(t, func(p providerConfig) bool {
		return p.SupportsAudioInput
	}, func(t *testing.T, p providerConfig) {
		ctx := testContext(t, 60*time.Second)

		// Use fixture audio
		audioData := fixtures.Audio("test.wav")

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: []vai.ContentBlock{
					vai.Text("What does the audio say? If it's silent or unclear, just say 'unclear'."),
					vai.Audio(audioData, "audio/wav"),
				}},
			},
			MaxTokens: 100,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Just verify we got a response
		if resp.TextContent() == "" {
			t.Error("expected text response for audio input")
		}
		t.Logf("Response: %s", resp.TextContent())
	})
}

func TestInput_VideoMP4(t *testing.T) {
	// Only Gemini supports video input
	forEachProviderWith(t, func(p providerConfig) bool {
		return p.SupportsVideoInput
	}, func(t *testing.T, p providerConfig) {
		ctx := testContext(t, 120*time.Second) // Videos may take longer

		// Skip if no video fixture available
		// For now, we'll just test that the request doesn't error
		t.Skip("skipping video test - requires video fixture")

		/*
			videoData := fixtures.Video("test.mp4")
			resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
				Model: p.Model,
				Messages: []vai.Message{
					{Role: "user", Content: []vai.ContentBlock{
						vai.Text("Describe what happens in this video briefly."),
						vai.Video(videoData, "video/mp4"),
					}},
				},
				MaxTokens: 200,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.TextContent() == "" {
				t.Error("expected text response for video input")
			}
		*/
		_ = ctx
	})
}

func TestInput_Document(t *testing.T) {
	// Test document input (PDF)
	forEachProviderWith(t, func(p providerConfig) bool {
		// Document support varies - test on providers known to support it
		return p.SupportsVision // Generally, vision-capable models can handle documents
	}, func(t *testing.T, p providerConfig) {
		t.Skip("skipping document test - requires PDF fixture")
		/*
			ctx := testContext(t, 60*time.Second)
			docData := fixtures.Document("test.pdf")

			resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
				Model: p.Model,
				Messages: []vai.Message{
					{Role: "user", Content: []vai.ContentBlock{
						vai.Text("What is this document about? Be brief."),
						vai.Document(docData, "application/pdf", "test.pdf"),
					}},
				},
				MaxTokens: 100,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.TextContent() == "" {
				t.Error("expected text response for document input")
			}
		*/
	})
}

func TestInput_MixedContent(t *testing.T) {
	forEachProviderWith(t, func(p providerConfig) bool {
		return p.SupportsVision
	}, func(t *testing.T, p providerConfig) {
		ctx := testContext(t, 60*time.Second)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: []vai.ContentBlock{
					vai.Text("Look at this cat image and tell me its likely color:"),
					vai.ImageURL("https://images.unsplash.com/photo-1514888286974-6c03e2ca1dba?w=200"),
					vai.Text("Just say the color in one word."),
				}},
			},
			MaxTokens: 50,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		text := resp.TextContent()
		if text == "" {
			t.Error("expected non-empty response")
		}
		t.Logf("Response: %s", text)
	})
}

func TestInput_LongText(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		ctx := testContext(t, 60*time.Second)

		// Create a long input text
		var longText strings.Builder
		for i := 0; i < 100; i++ {
			longText.WriteString("This is sentence number ")
			longText.WriteString(string(rune('0' + (i % 10))))
			longText.WriteString(". ")
		}
		longText.WriteString("How many sentences did I write? Just give the number.")

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text(longText.String())},
			},
			MaxTokens: 50,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		text := resp.TextContent()
		if text == "" {
			t.Error("expected non-empty response")
		}
		// Should mention 100 or hundred
		if !strings.Contains(text, "100") && !strings.Contains(strings.ToLower(text), "hundred") {
			t.Logf("Response: %s", text)
			t.Log("warning: expected response to mention 100")
		}
	})
}

func TestInput_SystemPrompt(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		ctx := defaultTestContext(t)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model:  p.Model,
			System: "You are a robot. You must start every response with 'BEEP BOOP'.",
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Hello!")},
			},
			MaxTokens: 100,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		text := strings.ToLower(resp.TextContent())
		if !strings.Contains(text, "beep") {
			t.Logf("Response: %s", resp.TextContent())
			t.Log("warning: expected 'beep' in response (system prompt may not be followed)")
		}
	})
}

func TestInput_MultiTurnConversation(t *testing.T) {
	forEachProvider(t, func(t *testing.T, p providerConfig) {
		ctx := testContext(t, 60*time.Second)

		// First turn
		resp1, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Remember this number: 42. Just say 'remembered'.")},
			},
			MaxTokens: 50,
		})
		if err != nil {
			t.Fatalf("first turn error: %v", err)
		}

		// Second turn - verify context is maintained
		resp2, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: p.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Remember this number: 42. Just say 'remembered'.")},
				{Role: "assistant", Content: resp1.Content},
				{Role: "user", Content: vai.Text("What number did I ask you to remember?")},
			},
			MaxTokens: 50,
		})
		if err != nil {
			t.Fatalf("second turn error: %v", err)
		}

		text := resp2.TextContent()
		if !strings.Contains(text, "42") && !strings.Contains(text, "forty-two") {
			t.Logf("warning: expected '42' in response, got %q (model may not maintain context)", text)
			// Don't fail - this tests context maintenance which can vary by model
		}
	})
}
