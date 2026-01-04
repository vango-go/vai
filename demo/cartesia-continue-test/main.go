// Package main tests Cartesia TTS continue flag behavior.
//
// This demo synthesizes "Yeah?" with continue=true and continue=false
// to demonstrate the difference in prosody.
//
// Usage:
//
//	go run demo/cartesia-continue-test/main.go
//
// Environment variables:
//
//	CARTESIA_API_KEY - Required for TTS
//
// Output:
//
//	yeah_continue_true.wav  - "Yeah?" with continue=true (flat intonation, expects more)
//	yeah_continue_false.wav - "Yeah?" with continue=false (proper question intonation)
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

const (
	cartesiaWSURL   = "wss://api.cartesia.ai/tts/websocket"
	cartesiaVersion = "2025-04-16"
	voiceID         = "a0e99841-438c-4a64-b679-ae501e7d6091"
	sampleRate      = 24000
)

type cartesiaVoiceSpec struct {
	Mode string `json:"mode"`
	ID   string `json:"id"`
}

type cartesiaOutputFormat struct {
	Container  string `json:"container"`
	Encoding   string `json:"encoding,omitempty"`
	SampleRate int    `json:"sample_rate,omitempty"`
}

type cartesiaStreamingRequest struct {
	ModelID      string               `json:"model_id"`
	Transcript   string               `json:"transcript"`
	Voice        cartesiaVoiceSpec    `json:"voice"`
	OutputFormat cartesiaOutputFormat `json:"output_format"`
	ContextID    string               `json:"context_id"`
	Continue     bool                 `json:"continue"`
}

type cartesiaWSResponse struct {
	Type  string `json:"type"`
	Data  string `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func main() {
	_ = godotenv.Load()

	apiKey := os.Getenv("CARTESIA_API_KEY")
	if apiKey == "" {
		log.Fatal("CARTESIA_API_KEY required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("Testing Cartesia continue flag with 'Yeah?'")
	fmt.Println()

	// Test with continue=true
	fmt.Println("Synthesizing with continue=true...")
	audioTrue, err := synthesize(ctx, apiKey, "Yeah?", true)
	if err != nil {
		log.Fatalf("continue=true failed: %v", err)
	}
	if err := writeWAV("yeah_continue_true.wav", audioTrue); err != nil {
		log.Fatalf("write continue=true wav: %v", err)
	}
	fmt.Printf("  Saved: yeah_continue_true.wav (%d bytes)\n", len(audioTrue))

	// Test with continue=false
	fmt.Println("Synthesizing with continue=false...")
	audioFalse, err := synthesize(ctx, apiKey, "Yeah?", false)
	if err != nil {
		log.Fatalf("continue=false failed: %v", err)
	}
	if err := writeWAV("yeah_continue_false.wav", audioFalse); err != nil {
		log.Fatalf("write continue=false wav: %v", err)
	}
	fmt.Printf("  Saved: yeah_continue_false.wav (%d bytes)\n", len(audioFalse))

	fmt.Println()
	fmt.Println("Done! Play both files to hear the difference:")
	fmt.Println("  - continue=true:  Flat/incomplete intonation (expects more text)")
	fmt.Println("  - continue=false: Proper question intonation (finalized prosody)")
}

func synthesize(ctx context.Context, apiKey, text string, continueFlag bool) ([]byte, error) {
	// Build WebSocket URL
	u, err := url.Parse(cartesiaWSURL)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	q := u.Query()
	q.Set("api_key", apiKey)
	q.Set("cartesia_version", cartesiaVersion)
	u.RawQuery = q.Encode()

	// Connect
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("websocket connect: %w", err)
	}
	defer conn.Close()

	// Send request
	req := cartesiaStreamingRequest{
		ModelID:    "sonic-3",
		Transcript: text,
		Voice: cartesiaVoiceSpec{
			Mode: "id",
			ID:   voiceID,
		},
		OutputFormat: cartesiaOutputFormat{
			Container:  "raw",
			Encoding:   "pcm_s16le",
			SampleRate: sampleRate,
		},
		ContextID: fmt.Sprintf("test_%v", continueFlag),
		Continue:  continueFlag,
	}

	if err := conn.WriteJSON(req); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	// Collect audio chunks
	var audio []byte
	for {
		var msg cartesiaWSResponse
		if err := conn.ReadJSON(&msg); err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}

		switch msg.Type {
		case "chunk":
			data, err := base64.StdEncoding.DecodeString(msg.Data)
			if err != nil {
				return nil, fmt.Errorf("decode audio: %w", err)
			}
			audio = append(audio, data...)

		case "done":
			return audio, nil

		case "error":
			return nil, fmt.Errorf("cartesia error: %s", msg.Error)
		}
	}
}

func writeWAV(filename string, pcmData []byte) error {
	// Create WAV header for 24kHz mono 16-bit PCM
	var buf bytes.Buffer

	dataSize := uint32(len(pcmData))
	fileSize := 36 + dataSize

	// RIFF header
	buf.WriteString("RIFF")
	buf.Write(uint32LE(fileSize))
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	buf.Write(uint32LE(16))      // chunk size
	buf.Write(uint16LE(1))       // audio format (PCM)
	buf.Write(uint16LE(1))       // num channels
	buf.Write(uint32LE(24000))   // sample rate
	buf.Write(uint32LE(48000))   // byte rate (24000 * 1 * 2)
	buf.Write(uint16LE(2))       // block align
	buf.Write(uint16LE(16))      // bits per sample

	// data chunk
	buf.WriteString("data")
	buf.Write(uint32LE(dataSize))
	buf.Write(pcmData)

	return os.WriteFile(filename, buf.Bytes(), 0644)
}

func uint32LE(v uint32) []byte {
	return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}
}

func uint16LE(v uint16) []byte {
	return []byte{byte(v), byte(v >> 8)}
}
