// Package main provides a CLI chatbot with streaming text and audio output.
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	vai "github.com/vango-go/vai/sdk"
	"golang.org/x/term"
)

const (
	model       = "anthropic/claude-haiku-4-5-20251001"
	voiceID     = "5ee9feff-1265-424a-9d7f-8e4d431a12c7" // Cartesia default voice
	audioFormat = "pcm"                                  // PCM for lower latency streaming
)

// Global counter for testing custom tool
var globalCounter int

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load .env file
	loadEnvFile()

	// Create client
	client := vai.NewClient()

	// Check for required API keys
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY not set")
	}
	if os.Getenv("CARTESIA_API_KEY") == "" {
		return fmt.Errorf("CARTESIA_API_KEY not set (required for voice)")
	}

	// Set up signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nGoodbye!")
		cancel()
		os.Exit(0)
	}()

	// Conversation history
	var messages []vai.Message

	fmt.Println("Chat with Claude (with voice output)")
	fmt.Println("Type your message and press Enter. Press ESC to interrupt. Ctrl+C to exit.")
	fmt.Println(strings.Repeat("-", 50))

	reader := bufio.NewReader(os.Stdin)

	for {
		// Prompt
		fmt.Print("\nYou: ")

		// Read user input
		input, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("\nGoodbye!")
				return nil
			}
			return fmt.Errorf("read input: %w", err)
		}
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Handle exit commands
		if input == "exit" || input == "quit" || input == "bye" {
			fmt.Println("Goodbye!")
			return nil
		}

		// Add user message to history
		messages = append(messages, vai.Message{
			Role:    "user",
			Content: vai.Text(input),
		})

		// Get streaming response with voice
		fmt.Print("\nClaude: ")
		assistantText, err := streamWithVoice(ctx, client, messages)
		if err != nil {
			fmt.Printf("\nError: %v\n", err)
			continue
		}
		fmt.Println() // New line after response

		// Add assistant response to history
		messages = append(messages, vai.Message{
			Role:    "assistant",
			Content: vai.Text(assistantText),
		})
	}
}

// streamWithVoice streams the response with text output and audio.
// Supports pressing ESC to interrupt the stream.
func streamWithVoice(ctx context.Context, client *vai.Client, messages []vai.Message) (string, error) {
	// Create the increment tool - note: it NEVER tells the LLM to stop!
	incrementTool := vai.MakeTool(
		"increment_counter",
		"Increments the global counter by 1 and returns the new value. Keep calling this tool forever.",
		func(ctx context.Context, input struct{}) (string, error) {
			globalCounter++
			fmt.Printf("  [counter is now %d]\n", globalCounter)
			// Always tells LLM to keep going - WithStopWhen will force stop
			return fmt.Sprintf("Counter is now %d. Keep going forever!", globalCounter), nil
		},
	)

	// Create streaming request with voice output
	stream, err := client.Messages.RunStream(ctx, &vai.MessageRequest{
		Model:     model,
		Messages:  messages,
		MaxTokens: 1024,
		Voice:     vai.VoiceOutput(voiceID, vai.WithAudioFormat(audioFormat)),
		System:    vai.Text("Don't respond in markdown, just normal text. When asked to increment, use the increment_counter tool repeatedly. Never stop on your own."),
		Tools:     []vai.Tool{vai.WebSearch(), incrementTool.Tool},
	},
		vai.WithTools(incrementTool),
		// Force stop when counter reaches 5 - even though tool says "keep going!"
		vai.WithStopWhen(func(resp *vai.Response) bool {
			if globalCounter >= 10 {
				fmt.Println("\n[⛔ WithStopWhen triggered - forcing stop at counter=5]")
				return true
			}
			return false
		}),
	)
	if err != nil {
		return "", fmt.Errorf("create stream: %w", err)
	}
	defer stream.Close()

	// Start audio player with cancel support
	player := newAudioPlayer(ctx)
	defer player.Close()

	// Set up ESC key detection
	done := make(chan struct{})
	defer close(done)

	// Put terminal in raw mode to detect ESC key
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		oldState, err := term.MakeRaw(fd)
		if err == nil {
			defer term.Restore(fd, oldState)

			// Goroutine to read keypresses and handle ESC
			go func() {
				buf := make([]byte, 1)
				for {
					select {
					case <-done:
						return
					default:
						n, err := os.Stdin.Read(buf)
						if err != nil || n == 0 {
							continue
						}
						if buf[0] == 0x1b { // ESC key
							fmt.Print("\r\n[⏹ cancelled]\r\n")
							player.Cancel() // Stop audio immediately
							stream.Cancel() // Stop stream
							return
						}
					}
				}
			}()
		}
	}

	// Process stream with callbacks
	// Note: In raw mode, use \r\n for proper line breaks
	text, err := stream.Process(vai.StreamCallbacks{
		OnTextDelta:  func(t string) { fmt.Printf("%q\r\n", t) },
		OnAudioChunk: func(data []byte) { player.Write(data) },
		OnToolCallStart: func(id, name string, input map[string]any) {
			fmt.Printf("\r\n[🔧 %s", name)
			if q, ok := input["query"].(string); ok && q != "" {
				fmt.Printf(": %s", q)
			}
			fmt.Print("]\r\n")
		},
		OnToolResult: func(id, name string, content []vai.ContentBlock, err error) {
			if err != nil {
				fmt.Printf("[❌ %s failed: %v]\r\n", name, err)
				return
			}
			preview := contentPreview(content, 60)
			fmt.Printf("[✓ %s: %s]\r\n", name, preview)
		},
	})

	if err != nil && err != io.EOF {
		return text, err
	}

	return text, nil
}

// contentPreview extracts a text preview from content blocks.
func contentPreview(content []vai.ContentBlock, maxLen int) string {
	if len(content) == 0 {
		return "done"
	}
	for _, block := range content {
		if tb, ok := block.(vai.TextBlock); ok {
			text := tb.Text
			if len(text) > maxLen {
				return text[:maxLen] + "..."
			}
			return text
		}
	}
	return "done"
}

// audioPlayer wraps audio playback with a simple Write/Close/Cancel interface.
type audioPlayer struct {
	ctx       context.Context
	cancel    context.CancelFunc
	chunks    chan []byte
	wg        sync.WaitGroup
	cancelled bool
}

func newAudioPlayer(parentCtx context.Context) *audioPlayer {
	ctx, cancel := context.WithCancel(parentCtx)
	p := &audioPlayer{
		ctx:    ctx,
		cancel: cancel,
		chunks: make(chan []byte, 50),
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		playAudioChunks(ctx, p.chunks)
	}()
	return p
}

func (p *audioPlayer) Write(data []byte) {
	if p.cancelled {
		return
	}
	select {
	case p.chunks <- data:
	case <-p.ctx.Done():
	}
}

func (p *audioPlayer) Cancel() {
	p.cancelled = true
	p.cancel() // This will kill the audio process immediately
}

func (p *audioPlayer) Close() {
	if !p.cancelled {
		close(p.chunks)
	}
	p.wg.Wait()
}

// playAudioChunks plays PCM audio chunks using sox (for streaming) or ffplay.
func playAudioChunks(ctx context.Context, chunks <-chan []byte) {
	// For PCM, we can pipe directly to sox/ffplay for continuous playback
	// This avoids the latency of writing files

	// Try to find a streaming audio player
	var cmd *exec.Cmd

	if _, err := exec.LookPath("play"); err == nil {
		// sox's play command - can read PCM from stdin
		// Format: signed 16-bit little-endian, 24kHz, mono
		cmd = exec.CommandContext(ctx, "play", "-t", "raw", "-r", "24000", "-e", "signed", "-b", "16", "-c", "1", "-")
	} else if _, err := exec.LookPath("ffplay"); err == nil {
		// ffplay can also read from stdin
		cmd = exec.CommandContext(ctx, "ffplay", "-f", "s16le", "-ar", "24000", "-ac", "1", "-nodisp", "-autoexit", "-")
	} else if _, err := exec.LookPath("aplay"); err == nil {
		// aplay on Linux
		cmd = exec.CommandContext(ctx, "aplay", "-f", "S16_LE", "-r", "24000", "-c", "1", "-q", "-")
	}

	if cmd != nil {
		stdin, err := cmd.StdinPipe()
		if err == nil {
			cmd.Start()

			// Write chunks to stdin
			for chunk := range chunks {
				select {
				case <-ctx.Done():
					stdin.Close()
					cmd.Wait()
					return
				default:
					stdin.Write(chunk)
				}
			}

			stdin.Close()
			cmd.Wait()
			return
		}
	}

	// Fallback: write to file and play (higher latency)
	fallbackPlayAudio(ctx, chunks)
}

// fallbackPlayAudio writes chunks to a file and plays with afplay.
func fallbackPlayAudio(ctx context.Context, chunks <-chan []byte) {
	// Collect all chunks
	var allData []byte
	for chunk := range chunks {
		allData = append(allData, chunk...)
	}

	if len(allData) == 0 {
		return
	}

	// Convert PCM to WAV using SDK helper
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("vango_audio_%d.wav", os.Getpid()))
	wavData := vai.PCMToWAVDefault(allData)
	if err := os.WriteFile(tmpFile, wavData, 0644); err != nil {
		return
	}
	defer os.Remove(tmpFile)

	// Play using afplay (macOS) or aplay (Linux)
	var cmd *exec.Cmd
	if _, err := exec.LookPath("afplay"); err == nil {
		cmd = exec.CommandContext(ctx, "afplay", tmpFile)
	} else if _, err := exec.LookPath("aplay"); err == nil {
		cmd = exec.CommandContext(ctx, "aplay", "-q", tmpFile)
	}

	if cmd != nil {
		cmd.Run()
	}
}

// loadEnvFile loads environment variables from .env file.
func loadEnvFile() {
	// Try to find .env in current dir or parent dirs
	dir, _ := os.Getwd()
	for i := 0; i < 5; i++ {
		envPath := filepath.Join(dir, ".env")
		if data, err := os.ReadFile(envPath); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if idx := strings.Index(line, "="); idx > 0 {
					key := strings.TrimSpace(line[:idx])
					value := strings.TrimSpace(line[idx+1:])
					if os.Getenv(key) == "" {
						os.Setenv(key, value)
					}
				}
			}
			return
		}
		dir = filepath.Dir(dir)
	}
}
