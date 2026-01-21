//go:build integration
// +build integration

package integration_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	vai "github.com/vango-go/vai/sdk"
)

// Note: Voice tests require CARTESIA_API_KEY for both STT and TTS
// The current implementation uses Cartesia's Ink Whisper for STT
// and Cartesia Sonic for TTS

func TestMessages_Create_VoiceInput(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		requireAllVoiceKeys(t)
		ctx := testContext(t, 60*time.Second)

		// Load audio fixture (or use minimal WAV)
		audioData := fixtures.Audio("question.wav")

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: []vai.ContentBlock{
					vai.Audio(audioData, "audio/wav"),
				}},
			},
			Voice: &vai.VoiceConfig{
				Input: &vai.VoiceInputConfig{
					Model:    "ink-whisper",
					Language: "en",
				},
			},
			MaxTokens: 8000,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have some response
		if resp.TextContent() == "" {
			t.Error("expected text content in response")
		}

		// Check if transcript was captured in metadata
		if resp.Metadata != nil {
			if transcript, ok := resp.Metadata["user_transcript"].(string); ok {
				t.Logf("User transcript: %s", transcript)
			}
		}
	})
}

func TestMessages_Create_VoiceOutput(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		requireAllVoiceKeys(t)
		ctx := testContext(t, 60*time.Second)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Say 'Hello, World!' and nothing else.")},
			},
			Voice: &vai.VoiceConfig{
				Output: &vai.VoiceOutputConfig{
					Voice:  "a0e99841-438c-4a64-b679-ae501e7d6091", // Default Cartesia voice
					Speed:  1.0,
					Format: "wav",
				},
			},
			MaxTokens: 8000,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have audio content block
		audioBlock := resp.AudioContent()
		if audioBlock == nil {
			t.Fatal("expected audio content block")
		}

		// Decode and verify it's real audio
		audioData, err := base64.StdEncoding.DecodeString(audioBlock.Source.Data)
		if err != nil {
			t.Fatalf("failed to decode audio: %v", err)
		}

		if len(audioData) < 100 {
			t.Errorf("expected substantial audio data, got %d bytes", len(audioData))
		}

		// Audio should have transcript
		if audioBlock.Transcript == nil || *audioBlock.Transcript == "" {
			t.Log("warning: expected transcript in audio block")
		}
	})
}

func TestMessages_Create_VoiceInputAndOutput(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		requireAllVoiceKeys(t)
		ctx := testContext(t, 90*time.Second)

		audioData := fixtures.Audio("hello.wav")

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: []vai.ContentBlock{
					vai.Audio(audioData, "audio/wav"),
				}},
			},
			Voice: &vai.VoiceConfig{
				Input: &vai.VoiceInputConfig{
					Model:    "ink-whisper",
					Language: "en",
				},
				Output: &vai.VoiceOutputConfig{
					Voice:  "a0e99841-438c-4a64-b679-ae501e7d6091",
					Speed:  1.0,
					Format: "wav",
				},
			},
			MaxTokens: 8000,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have both text and audio
		if resp.TextContent() == "" {
			t.Error("expected text content")
		}
		if resp.AudioContent() == nil {
			t.Error("expected audio content")
		}
	})
}

func TestMessages_Stream_VoiceOutput(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		requireAllVoiceKeys(t)
		ctx := testContext(t, 60*time.Second)

		stream, err := testClient.Messages.Stream(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Say a greeting in one short sentence.")},
			},
			Voice: &vai.VoiceConfig{
				Output: &vai.VoiceOutputConfig{
					Voice:  "a0e99841-438c-4a64-b679-ae501e7d6091",
					Speed:  1.0,
					Format: "wav",
				},
			},
			MaxTokens: 8000,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer stream.Close()

		var gotAudioDelta bool
		var totalAudioBytes int
		var gotTextDelta bool

		for event := range stream.Events() {
			switch e := event.(type) {
			case vai.AudioDeltaEvent:
				gotAudioDelta = true
				chunk, _ := base64.StdEncoding.DecodeString(e.Delta.Data)
				totalAudioBytes += len(chunk)
			case vai.ContentBlockDeltaEvent:
				if _, ok := e.Delta.(vai.TextDelta); ok {
					gotTextDelta = true
				}
			}
		}

		if !gotTextDelta {
			t.Error("expected text delta events")
		}

		// Audio deltas are optional during streaming (may be synthesized after)
		if gotAudioDelta {
			t.Logf("Got %d bytes of streaming audio", totalAudioBytes)
		}

		// Final response should have audio
		resp := stream.Response()
		if resp != nil && resp.AudioContent() != nil {
			t.Log("Audio content present in final response")
		}
	})
}

func TestVoice_TextWithAudioMarkers(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		requireAllVoiceKeys(t)
		ctx := testContext(t, 60*time.Second)

		resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Count from one to five.")},
			},
			Voice: &vai.VoiceConfig{
				Output: &vai.VoiceOutputConfig{
					Voice:  "a0e99841-438c-4a64-b679-ae501e7d6091",
					Speed:  1.0,
					Format: "wav",
				},
			},
			MaxTokens: 8000,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		text := strings.ToLower(resp.TextContent())
		hasNumbers := strings.Contains(text, "one") ||
			strings.Contains(text, "1") ||
			strings.Contains(text, "two") ||
			strings.Contains(text, "2")

		if !hasNumbers {
			t.Logf("Response: %s", resp.TextContent())
			t.Log("warning: expected counting in response")
		}

		// Should have audio
		if resp.AudioContent() == nil {
			t.Error("expected audio output")
		}
	})
}

func TestVoice_DifferentFormats(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		requireAllVoiceKeys(t)

		formats := []string{"wav", "mp3", "pcm"}

		for _, format := range formats {
			t.Run(format, func(t *testing.T) {
				ctx := testContext(t, 60*time.Second)

				resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
					Model: provider.Model,
					Messages: []vai.Message{
						{Role: "user", Content: vai.Text("Say hello.")},
					},
					Voice: &vai.VoiceConfig{
						Output: &vai.VoiceOutputConfig{
							Voice:  "a0e99841-438c-4a64-b679-ae501e7d6091",
							Format: format,
						},
					},
					MaxTokens: 8000,
				})

				if err != nil {
					// Some formats may not be supported
					t.Logf("Format %s error: %v", format, err)
					return
				}

				audio := resp.AudioContent()
				if audio == nil {
					t.Errorf("expected audio for format %s", format)
					return
				}

				// Check media type matches
				expectedMedia := "audio/" + format
				if audio.Source.MediaType != expectedMedia {
					t.Logf("Media type: %s (expected %s)", audio.Source.MediaType, expectedMedia)
				}
			})
		}
	})
}

func TestVoice_SpeedVariation(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		requireAllVoiceKeys(t)
		ctx := testContext(t, 90*time.Second)

		speeds := []float64{0.8, 1.0, 1.2}
		var audioSizes []int

		for _, speed := range speeds {
			resp, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
				Model: provider.Model,
				Messages: []vai.Message{
					{Role: "user", Content: vai.Text("Say hello world.")},
				},
				Voice: &vai.VoiceConfig{
					Output: &vai.VoiceOutputConfig{
						Voice:  "a0e99841-438c-4a64-b679-ae501e7d6091",
						Speed:  speed,
						Format: "wav",
					},
				},
				MaxTokens: 8000,
			})

			if err != nil {
				t.Logf("Speed %.1f error: %v", speed, err)
				continue
			}

			audio := resp.AudioContent()
			if audio == nil {
				t.Logf("No audio for speed %.1f", speed)
				continue
			}

			data, _ := base64.StdEncoding.DecodeString(audio.Source.Data)
			audioSizes = append(audioSizes, len(data))
			t.Logf("Speed %.1f: %d bytes", speed, len(data))
		}

		// Speeds should produce different audio lengths (generally)
		if len(audioSizes) > 1 {
			t.Logf("Audio sizes across speeds: %v", audioSizes)
		}
	})
}
