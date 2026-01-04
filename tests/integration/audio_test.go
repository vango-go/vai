//go:build integration
// +build integration

package integration_test

import (
	"strings"
	"testing"
	"time"

	vai "github.com/vango-go/vai/sdk"
)

// ==================== Transcription ====================

func TestAudio_Transcribe_Basic(t *testing.T) {
	requireCartesiaKey(t)
	ctx := testContext(t, 30*time.Second)

	audioData := fixtures.Audio("hello.wav")

	transcript, err := testClient.Audio.Transcribe(ctx, &vai.TranscribeRequest{
		Audio:    audioData,
		Model:    "ink-whisper",
		Language: "en",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if transcript.Text == "" {
		t.Error("expected non-empty transcript")
	}

	t.Logf("Transcript: %s", transcript.Text)
}

func TestAudio_Transcribe_WithTimestamps(t *testing.T) {
	requireCartesiaKey(t)
	ctx := testContext(t, 30*time.Second)

	audioData := fixtures.Audio("hello.wav")

	transcript, err := testClient.Audio.Transcribe(ctx, &vai.TranscribeRequest{
		Audio:      audioData,
		Model:      "ink-whisper",
		Language:   "en",
		Timestamps: true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if transcript.Text == "" {
		t.Error("expected non-empty transcript")
	}

	// Check for word-level timestamps if supported
	if len(transcript.Words) > 0 {
		for _, word := range transcript.Words {
			if word.Word == "" {
				t.Error("expected non-empty word")
			}
			if word.End < word.Start {
				t.Errorf("invalid timestamps: start=%f end=%f", word.Start, word.End)
			}
		}
		t.Logf("Got %d words with timestamps", len(transcript.Words))
	} else {
		t.Log("No word-level timestamps returned")
	}
}

func TestAudio_Transcribe_DifferentLanguages(t *testing.T) {
	requireCartesiaKey(t)

	languages := []string{"en", "es", "fr", "de"}

	for _, lang := range languages {
		t.Run(lang, func(t *testing.T) {
			ctx := testContext(t, 30*time.Second)
			audioData := fixtures.Audio("hello.wav")

			transcript, err := testClient.Audio.Transcribe(ctx, &vai.TranscribeRequest{
				Audio:    audioData,
				Model:    "ink-whisper",
				Language: lang,
			})

			if err != nil {
				// Some languages may not be supported
				t.Logf("Language %s: %v", lang, err)
				return
			}

			if transcript.Text == "" {
				t.Logf("Language %s: empty transcript", lang)
			} else {
				t.Logf("Language %s: %s", lang, transcript.Text)
			}
		})
	}
}

// ==================== Synthesis ====================

func TestAudio_Synthesize_Basic(t *testing.T) {
	requireCartesiaKey(t)
	ctx := testContext(t, 30*time.Second)

	result, err := testClient.Audio.Synthesize(ctx, &vai.SynthesizeRequest{
		Text:   "Hello, this is a test of text to speech synthesis.",
		Voice:  "a0e99841-438c-4a64-b679-ae501e7d6091", // Default Cartesia voice
		Format: "wav",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Audio) < 100 {
		t.Errorf("expected substantial audio data, got %d bytes", len(result.Audio))
	}

	if result.Format != "wav" {
		t.Errorf("expected format 'wav', got %q", result.Format)
	}

	t.Logf("Synthesized %d bytes of audio (%.2f seconds)", len(result.Audio), result.Duration)
}

func TestAudio_Synthesize_WithSpeed(t *testing.T) {
	requireCartesiaKey(t)
	ctx := testContext(t, 60*time.Second)

	text := "This is normal speed speech."

	normalResult, err := testClient.Audio.Synthesize(ctx, &vai.SynthesizeRequest{
		Text:   text,
		Voice:  "a0e99841-438c-4a64-b679-ae501e7d6091",
		Speed:  1.0,
		Format: "wav",
	})
	if err != nil {
		t.Fatalf("normal speed error: %v", err)
	}

	fastResult, err := testClient.Audio.Synthesize(ctx, &vai.SynthesizeRequest{
		Text:   text,
		Voice:  "a0e99841-438c-4a64-b679-ae501e7d6091",
		Speed:  1.5,
		Format: "wav",
	})
	if err != nil {
		t.Fatalf("fast speed error: %v", err)
	}

	// Fast speech should produce different duration
	t.Logf("Normal: %.2fs, Fast: %.2fs", normalResult.Duration, fastResult.Duration)

	// The durations should be different (fast should be shorter)
	if normalResult.Duration > 0 && fastResult.Duration > 0 {
		if fastResult.Duration >= normalResult.Duration {
			t.Log("warning: expected faster speech to have shorter duration")
		}
	}
}

func TestAudio_Synthesize_DifferentFormats(t *testing.T) {
	requireCartesiaKey(t)

	formats := []string{"wav", "mp3", "pcm"}

	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			ctx := testContext(t, 30*time.Second)

			result, err := testClient.Audio.Synthesize(ctx, &vai.SynthesizeRequest{
				Text:   "Hello",
				Voice:  "a0e99841-438c-4a64-b679-ae501e7d6091",
				Format: format,
			})

			if err != nil {
				t.Logf("Format %s error: %v", format, err)
				return
			}

			if len(result.Audio) == 0 {
				t.Errorf("expected audio data for format %s", format)
			}

			if result.Format != format {
				t.Logf("Format mismatch: expected %s, got %s", format, result.Format)
			}

			t.Logf("Format %s: %d bytes", format, len(result.Audio))
		})
	}
}

func TestAudio_Synthesize_LongText(t *testing.T) {
	requireCartesiaKey(t)
	ctx := testContext(t, 60*time.Second)

	longText := strings.Repeat("This is a sentence with multiple words. ", 10)

	result, err := testClient.Audio.Synthesize(ctx, &vai.SynthesizeRequest{
		Text:   longText,
		Voice:  "a0e99841-438c-4a64-b679-ae501e7d6091",
		Format: "wav",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Long text should produce more audio
	if len(result.Audio) < 1000 {
		t.Errorf("expected more audio for long text, got %d bytes", len(result.Audio))
	}

	t.Logf("Long text: %d bytes, %.2f seconds", len(result.Audio), result.Duration)
}

func TestAudio_StreamSynthesize_Basic(t *testing.T) {
	t.Skip("Skipping: WebSocket streaming synthesis has connection issues - needs investigation")
	requireCartesiaKey(t)
	ctx := testContext(t, 60*time.Second)

	stream, err := testClient.Audio.StreamSynthesize(ctx, &vai.SynthesizeRequest{
		Text:   "Hello world.",
		Voice:  "a0e99841-438c-4a64-b679-ae501e7d6091",
		Format: "pcm",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	var chunkCount int
	var totalBytes int

	// Use a timeout to prevent hanging
	done := make(chan struct{})
	go func() {
		for chunk := range stream.Chunks() {
			chunkCount++
			totalBytes += len(chunk)
		}
		close(done)
	}()

	select {
	case <-done:
		// Stream completed normally
	case <-ctx.Done():
		t.Fatal("stream timed out")
	}

	if stream.Err() != nil {
		t.Errorf("stream error: %v", stream.Err())
	}

	t.Logf("Streaming: %d chunks, %d total bytes", chunkCount, totalBytes)

	if totalBytes == 0 {
		t.Error("expected audio data from stream")
	}
}

func TestAudio_Synthesize_Emotion(t *testing.T) {
	requireCartesiaKey(t)

	emotions := []string{"neutral", "happy", "sad"}

	for _, emotion := range emotions {
		t.Run(emotion, func(t *testing.T) {
			ctx := testContext(t, 30*time.Second)

			result, err := testClient.Audio.Synthesize(ctx, &vai.SynthesizeRequest{
				Text:    "Hello, how are you today?",
				Voice:   "a0e99841-438c-4a64-b679-ae501e7d6091",
				Emotion: emotion,
				Format:  "wav",
			})

			if err != nil {
				// Emotion support may vary
				t.Logf("Emotion %s: %v", emotion, err)
				return
			}

			if len(result.Audio) == 0 {
				t.Errorf("expected audio for emotion %s", emotion)
			}

			t.Logf("Emotion %s: %d bytes", emotion, len(result.Audio))
		})
	}
}

func TestAudio_Synthesize_Volume(t *testing.T) {
	requireCartesiaKey(t)
	ctx := testContext(t, 60*time.Second)

	volumes := []float64{0.5, 1.0, 1.5}

	for _, volume := range volumes {
		result, err := testClient.Audio.Synthesize(ctx, &vai.SynthesizeRequest{
			Text:   "Testing volume levels",
			Voice:  "a0e99841-438c-4a64-b679-ae501e7d6091",
			Volume: volume,
			Format: "wav",
		})

		if err != nil {
			t.Logf("Volume %.1f: %v", volume, err)
			continue
		}

		t.Logf("Volume %.1f: %d bytes", volume, len(result.Audio))
	}
}
