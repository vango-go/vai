// Package main provides a minimal CLI demo for live voice conversations.
//
// This demonstrates the intended simple DX for Vango AI live sessions
// using the unified RunStream API with WithLive option.
//
// Usage:
//
//	go run demo/live/main.go
//
// Environment variables:
//
//	ANTHROPIC_API_KEY - Required for LLM
//	CARTESIA_API_KEY  - Required for STT and TTS
//
// Controls:
//
//	/t <text>       - Send text message
//	/image <path>   - Send image file
//	/video <path>   - Send video file
//	q               - Quit the demo
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/joho/godotenv"

	vai "github.com/vango-go/vai/sdk"
)

func main() {
	_ = godotenv.Load()

	// Validate API keys
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		log.Fatal("ANTHROPIC_API_KEY required")
	}
	if os.Getenv("CARTESIA_API_KEY") == "" {
		log.Fatal("CARTESIA_API_KEY required")
	}

	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║              Vango AI Live Voice Demo                      ║")
	fmt.Println("╠════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Speak naturally - automatic turn detection is enabled.    ║")
	fmt.Println("║  Web search is enabled for real-time information.          ║")
	fmt.Println("║                                                            ║")
	fmt.Println("║  Commands:                                                 ║")
	fmt.Println("║    /t <text>       Send text message                       ║")
	fmt.Println("║    /image <path>   Send image file                         ║")
	fmt.Println("║    /video <path>   Send video file                         ║")
	fmt.Println("║    q               Quit                                    ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Create Vango client
	client := vai.NewClient()

	// Create request - same as for regular chat, just add WithLive for voice mode
	req := &vai.MessageRequest{
		Model:  "anthropic/claude-haiku-4-5-20251001",
		System: `You are in live voice mode right now (via STT <> TTS). This means you need to be more conversational in your responses. Sometimes a single word reply (e.g. "okay", "uh huh?") is appropriate. If you received the lastest user message, but you're not sure if they are actually done talking (e.g. they are trying to explain something and paused to think), you can give a short reply to help gauge. (e.g. "ya?", "uh huh?", "Are you done explaining? I have some thoughts..", etc..). You have access to web search - use it when users ask about current events, recent information, or anything that might benefit from up-to-date data.`,
		Tools:  []vai.Tool{vai.WebSearch()},
	}

	// Start live session via RunStream with WithLive option
	stream, err := client.Messages.RunStream(ctx, req,
		vai.WithLive(&vai.LiveConfig{
			SampleRate: sampleRate,
			Debug:      true,
		}),
	)
	if err != nil {
		log.Fatalf("Failed to start live session: %v", err)
	}
	defer stream.Close()

	// Initialize audio I/O
	mic, speaker, cleanup := initAudio()
	defer cleanup()

	// Send microphone audio to session
	go func() {
		buf := make([]byte, sampleRate*2/50) // 20ms chunks
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			n := mic.Read(buf)
			if n > 0 {
				stream.SendAudio(buf[:n])
			}
		}
	}()

	// Play audio using SDK's AudioOutput (handles pre-buffering and flush automatically)
	stream.AudioOutput().HandleAudio(
		func(data []byte) { speaker.Write(data) },
		func() { speaker.Flush() },
	)

	// Handle other session events
	go func() {
		for event := range stream.Events() {
			switch e := event.(type) {
			case vai.LiveErrorEvent:
				fmt.Printf("\n[ERROR] %s: %s\n", e.Code, e.Message)
			}
		}
	}()

	// Command input loop
	fmt.Println("Listening... (type commands or 'q' to quit)")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Quit command
		if strings.ToLower(input) == "q" {
			break
		}

		// Text command: /t <text>
		if strings.HasPrefix(input, "/t ") {
			text := strings.TrimPrefix(input, "/t ")
			if err := stream.SendText(text); err != nil {
				fmt.Printf("[ERROR] Failed to send text: %v\n", err)
			} else {
				fmt.Printf("[SENT] Text: %s\n", text)
			}
			continue
		}

		// Image command: /image <path>
		if strings.HasPrefix(input, "/image ") {
			path := strings.TrimSpace(strings.TrimPrefix(input, "/image "))
			if err := sendImage(stream, path); err != nil {
				fmt.Printf("[ERROR] Failed to send image: %v\n", err)
			} else {
				fmt.Printf("[SENT] Image: %s\n", path)
			}
			continue
		}

		// Video command: /video <path>
		if strings.HasPrefix(input, "/video ") {
			path := strings.TrimSpace(strings.TrimPrefix(input, "/video "))
			if err := sendVideo(stream, path); err != nil {
				fmt.Printf("[ERROR] Failed to send video: %v\n", err)
			} else {
				fmt.Printf("[SENT] Video: %s\n", path)
			}
			continue
		}

		// Unknown command
		fmt.Println("[INFO] Commands: /t <text>, /image <path>, /video <path>, q")
	}
}

// sendImage reads an image file and sends it to the session.
func sendImage(stream *vai.RunStream, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Warn for large files
	if len(data) > 10*1024*1024 {
		fmt.Printf("[WARN] Large file (%d MB) - this may take a moment\n", len(data)/1024/1024)
	}

	mediaType := inferImageMediaType(path)
	content := vai.ContentBlocks(vai.Image(data, mediaType))

	return stream.SendContent(content)
}

// sendVideo reads a video file and sends it to the session.
func sendVideo(stream *vai.RunStream, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Warn for large files
	if len(data) > 10*1024*1024 {
		fmt.Printf("[WARN] Large file (%d MB) - this may take a moment\n", len(data)/1024/1024)
	}

	mediaType := inferVideoMediaType(path)
	content := vai.ContentBlocks(vai.Video(data, mediaType))

	return stream.SendContent(content)
}

// inferImageMediaType infers MIME type from file extension.
func inferImageMediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

// inferVideoMediaType infers MIME type from file extension.
func inferVideoMediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	default:
		return "video/mp4"
	}
}
