# Phase 4: Voice Pipeline

**Status:** ✅ Complete (December 2025)
**Priority:** Critical Path
**Dependencies:** Phase 2 (Anthropic Provider), Phase 3 (Tool System)

---

## Overview

Phase 4 implements the voice pipeline that enables any text-based model to accept audio input and produce audio output. This is achieved through:

1. **STT (Speech-to-Text)**: Transcribe audio input before sending to the LLM
2. **TTS (Text-to-Speech)**: Convert LLM text output to audio
3. **Audio streaming**: Stream audio chunks as they're generated for low latency

The pipeline works with Anthropic (and later all providers) by intercepting audio content blocks, transcribing them, and converting text responses back to audio.

**Note:** Phase 4 uses **Cartesia** as the sole provider for both STT and TTS. This simplifies the initial implementation while maintaining an extensible architecture for adding more providers in future phases.

---

## Goals

1. Implement Cartesia STT integration (ink-whisper model)
2. Implement Cartesia TTS integration (sonic-3 model)
3. Add audio content block handling in the message flow
4. Stream audio output during streaming responses
5. Support sentence-boundary buffering for natural TTS
6. Implement standalone `Audio.Transcribe()` and `Audio.Synthesize()`

---

## Implementation Summary

### Usage Examples

#### Text Input with Audio Output (for chat UI)

```go
client := vai.NewClient()

resp, err := client.Messages.Create(ctx, &vai.MessageRequest{
    Model: "anthropic/claude-sonnet-4",
    Messages: []vai.Message{
        {Role: "user", Content: vai.Text("Hello, how are you?")},
    },
    Voice: vai.VoiceOutput("your-cartesia-voice-id"),
})

// For chat UI / message history:
assistantText := resp.TextContent()   // "I'm doing well, thank you!"
assistantAudio := resp.AudioContent() // AudioBlock for playback
```

#### Full Voice Pipeline (Audio In → LLM → Audio Out)

```go
resp, err := client.Messages.Create(ctx, &vai.MessageRequest{
    Model: "anthropic/claude-sonnet-4",
    Messages: []vai.Message{
        {Role: "user", Content: vai.Audio(audioData, "audio/wav")},
    },
    Voice: vai.VoiceFull("your-cartesia-voice-id"),
})

userTranscript := resp.UserTranscript()  // "what is two plus two"
assistantText := resp.TextContent()      // "Two plus two equals four."
assistantAudio := resp.AudioContent()    // AudioBlock for playback
```

#### Standalone Transcription

```go
transcript, err := client.Audio.Transcribe(ctx, &vai.TranscribeRequest{
    Audio:      audioData,
    Language:   "en",
    Timestamps: true,
})
fmt.Println(transcript.Text)
```

#### Standalone Synthesis

```go
result, err := client.Audio.Synthesize(ctx, &vai.SynthesizeRequest{
    Text:   "Hello, world!",
    Voice:  "your-cartesia-voice-id",
    Speed:  1.0,
    Format: "wav",
})
os.WriteFile("output.wav", result.Audio, 0644)
```

#### Streaming Synthesis

```go
stream, err := client.Audio.StreamSynthesize(ctx, &vai.SynthesizeRequest{
    Text:  longText,
    Voice: "your-cartesia-voice-id",
})

for chunk := range stream.Chunks() {
    speaker.Write(chunk)  // Play audio chunks as they arrive
}
```

---

## Deliverables

### 4.1 Package Structure

```
pkg/core/
├── voice/
│   ├── pipeline.go       # Voice pipeline orchestration
│   ├── buffer.go         # Sentence boundary buffering
│   ├── stt/
│   │   ├── provider.go   # STT provider interface
│   │   └── cartesia.go   # Cartesia implementation
│   └── tts/
│       ├── provider.go   # TTS provider interface
│       └── cartesia.go   # Cartesia implementation
│
sdk/
├── audio.go              # AudioService
└── voice.go              # VoiceConfig, helpers
```

### 4.2 STT Provider Interface (pkg/core/voice/stt/provider.go)

```go
package stt

import (
    "context"
    "io"
)

// Provider is the interface for speech-to-text services.
type Provider interface {
    // Name returns the provider identifier.
    Name() string

    // Transcribe converts audio to text.
    Transcribe(ctx context.Context, audio io.Reader, opts TranscribeOptions) (*Transcript, error)

    // TranscribeStream transcribes streaming audio.
    // Returns a channel that emits transcript updates.
    TranscribeStream(ctx context.Context, audio io.Reader, opts TranscribeOptions) (<-chan TranscriptDelta, error)
}

// TranscribeOptions configures transcription.
type TranscribeOptions struct {
    Model       string // Provider-specific model (default: "ink-whisper")
    Language    string // ISO language code (default: "en")
    Format      string // Audio format hint (wav, mp3, webm, etc.)
    SampleRate  int    // Audio sample rate in Hz
    Timestamps  bool   // Include word-level timestamps
}

// Transcript is the result of transcription.
type Transcript struct {
    Text       string  // Full transcribed text
    Language   string  // Detected or specified language
    Duration   float64 // Audio duration in seconds
    Words      []Word  // Word-level details (if timestamps requested)
}

// Word represents a single transcribed word with timing.
type Word struct {
    Word  string  // The word
    Start float64 // Start time in seconds
    End   float64 // End time in seconds
}

// TranscriptDelta is a streaming transcript update.
type TranscriptDelta struct {
    Text      string // Partial transcript
    IsFinal   bool   // True if this is a final segment
    Timestamp float64
}
```

### 4.3 Cartesia STT Implementation (pkg/core/voice/stt/cartesia.go)

```go
package stt

import (
    "bytes"
    "context"
    "fmt"
    "io"
    "mime/multipart"
    "net/http"
    "encoding/json"
)

const (
    cartesiaBaseURL = "https://api.cartesia.ai"
    cartesiaVersion = "2025-04-16"
)

type CartesiaProvider struct {
    apiKey     string
    httpClient *http.Client
}

func NewCartesia(apiKey string) *CartesiaProvider {
    return &CartesiaProvider{
        apiKey:     apiKey,
        httpClient: http.DefaultClient,
    }
}

func (c *CartesiaProvider) Name() string {
    return "cartesia"
}

func (c *CartesiaProvider) Transcribe(ctx context.Context, audio io.Reader, opts TranscribeOptions) (*Transcript, error) {
    // Read audio data
    audioData, err := io.ReadAll(audio)
    if err != nil {
        return nil, fmt.Errorf("read audio: %w", err)
    }

    // Build multipart form
    var buf bytes.Buffer
    mw := multipart.NewWriter(&buf)

    // Add audio file
    fw, err := mw.CreateFormFile("file", "audio."+getExtension(opts.Format))
    if err != nil {
        return nil, err
    }
    fw.Write(audioData)

    // Add model (default to ink-whisper)
    model := opts.Model
    if model == "" {
        model = "ink-whisper"
    }
    mw.WriteField("model", model)

    // Add language if specified
    if opts.Language != "" {
        mw.WriteField("language", opts.Language)
    }

    // Request word timestamps if enabled
    if opts.Timestamps {
        mw.WriteField("timestamp_granularities[]", "word")
    }

    mw.Close()

    // Create request
    req, err := http.NewRequestWithContext(ctx, "POST", cartesiaBaseURL+"/stt", &buf)
    if err != nil {
        return nil, err
    }

    req.Header.Set("Authorization", "Bearer "+c.apiKey)
    req.Header.Set("Cartesia-Version", cartesiaVersion)
    req.Header.Set("Content-Type", mw.FormDataContentType())

    // Execute
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("cartesia request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("cartesia error %d: %s", resp.StatusCode, string(body))
    }

    // Parse response
    var cartesiaResp cartesiaTranscriptionResponse
    if err := json.NewDecoder(resp.Body).Decode(&cartesiaResp); err != nil {
        return nil, fmt.Errorf("parse response: %w", err)
    }

    return c.convertResponse(cartesiaResp), nil
}

type cartesiaTranscriptionResponse struct {
    Text     string  `json:"text"`
    Language *string `json:"language,omitempty"`
    Duration *float64 `json:"duration,omitempty"`
    Words    []struct {
        Word  string  `json:"word"`
        Start float64 `json:"start"`
        End   float64 `json:"end"`
    } `json:"words,omitempty"`
}

func (c *CartesiaProvider) convertResponse(resp cartesiaTranscriptionResponse) *Transcript {
    t := &Transcript{
        Text: resp.Text,
    }

    if resp.Language != nil {
        t.Language = *resp.Language
    }
    if resp.Duration != nil {
        t.Duration = *resp.Duration
    }

    if len(resp.Words) > 0 {
        t.Words = make([]Word, len(resp.Words))
        for i, w := range resp.Words {
            t.Words[i] = Word{
                Word:  w.Word,
                Start: w.Start,
                End:   w.End,
            }
        }
    }

    return t
}

func (c *CartesiaProvider) TranscribeStream(ctx context.Context, audio io.Reader, opts TranscribeOptions) (<-chan TranscriptDelta, error) {
    // Future: WebSocket streaming at /stt/ws
    return nil, fmt.Errorf("streaming transcription not yet implemented")
}

func getExtension(format string) string {
    switch format {
    case "wav", "mp3", "webm", "ogg", "flac", "m4a", "mp4", "mpeg", "mpga", "oga":
        return format
    default:
        return "wav"
    }
}
```

### 4.4 TTS Provider Interface (pkg/core/voice/tts/provider.go)

```go
package tts

import (
    "context"
)

// Provider is the interface for text-to-speech services.
type Provider interface {
    // Name returns the provider identifier.
    Name() string

    // Synthesize converts text to audio.
    Synthesize(ctx context.Context, text string, opts SynthesizeOptions) (*Synthesis, error)

    // SynthesizeStream converts text to streaming audio.
    SynthesizeStream(ctx context.Context, text string, opts SynthesizeOptions) (*SynthesisStream, error)
}

// SynthesizeOptions configures synthesis.
type SynthesizeOptions struct {
    Voice      string  // Voice identifier (Cartesia voice ID)
    Speed      float64 // Speed multiplier (0.6-1.5, default 1.0)
    Volume     float64 // Volume multiplier (0.5-2.0, default 1.0)
    Emotion    string  // Emotion hint (neutral, happy, sad, angry, etc.)
    Language   string  // Language code
    Format     string  // Output format: "wav", "mp3", or "pcm"
    SampleRate int     // Sample rate: 8000, 16000, 22050, 24000, 44100, 48000
}

// Synthesis is the result of synthesis.
type Synthesis struct {
    Audio    []byte  // Audio data
    Format   string  // Audio format
    Duration float64 // Duration in seconds (if available)
}

// SynthesisStream provides streaming audio output.
type SynthesisStream struct {
    chunks chan []byte
    err    error
    done   chan struct{}
}

// Chunks returns the channel of audio chunks.
func (s *SynthesisStream) Chunks() <-chan []byte {
    return s.chunks
}

// Err returns any error that occurred.
func (s *SynthesisStream) Err() error {
    <-s.done
    return s.err
}

// Close closes the stream.
func (s *SynthesisStream) Close() error {
    close(s.done)
    return nil
}
```

### 4.5 Cartesia TTS Implementation (pkg/core/voice/tts/cartesia.go)

```go
package tts

import (
    "bytes"
    "context"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "sync/atomic"

    "github.com/gorilla/websocket"
)

const (
    cartesiaBaseURL = "https://api.cartesia.ai"
    cartesiaWSURL   = "wss://api.cartesia.ai/tts/websocket"
    cartesiaVersion = "2025-04-16"
)

// Default voices (name -> voice ID mapping)
var cartesiaVoices = map[string]string{
    "default": "a0e99841-438c-4a64-b679-ae501e7d6091", // Example voice ID
}

type CartesiaTTSProvider struct {
    apiKey     string
    httpClient *http.Client
}

func NewCartesiaTTS(apiKey string) *CartesiaTTSProvider {
    return &CartesiaTTSProvider{
        apiKey:     apiKey,
        httpClient: http.DefaultClient,
    }
}

func (c *CartesiaTTSProvider) Name() string {
    return "cartesia"
}

func (c *CartesiaTTSProvider) Synthesize(ctx context.Context, text string, opts SynthesizeOptions) (*Synthesis, error) {
    voiceID := opts.Voice
    if id, ok := cartesiaVoices[opts.Voice]; ok {
        voiceID = id
    }
    if voiceID == "" {
        voiceID = cartesiaVoices["default"]
    }

    // Build output format
    outputFormat := c.buildOutputFormat(opts)

    // Build request body
    reqBody := cartesiaTTSRequest{
        ModelID:    "sonic-3",
        Transcript: text,
        Voice: cartesiaVoiceSpec{
            Mode: "id",
            ID:   voiceID,
        },
        OutputFormat: outputFormat,
    }

    // Add generation config if speed/volume/emotion specified
    if opts.Speed != 0 || opts.Volume != 0 || opts.Emotion != "" {
        reqBody.GenerationConfig = &cartesiaGenerationConfig{
            Speed:   opts.Speed,
            Volume:  opts.Volume,
            Emotion: opts.Emotion,
        }
    }

    if opts.Language != "" {
        reqBody.Language = &opts.Language
    }

    body, _ := json.Marshal(reqBody)

    // Create request
    req, err := http.NewRequestWithContext(ctx, "POST", cartesiaBaseURL+"/tts/bytes", bytes.NewReader(body))
    if err != nil {
        return nil, err
    }

    req.Header.Set("Authorization", "Bearer "+c.apiKey)
    req.Header.Set("Cartesia-Version", cartesiaVersion)
    req.Header.Set("Content-Type", "application/json")

    // Execute
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("cartesia request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode == http.StatusNoContent {
        return &Synthesis{Audio: []byte{}, Format: opts.Format}, nil
    }

    if resp.StatusCode != http.StatusOK {
        errBody, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("cartesia error %d: %s", resp.StatusCode, string(errBody))
    }

    // Read audio
    audio, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("read audio: %w", err)
    }

    format := opts.Format
    if format == "" {
        format = "wav"
    }

    return &Synthesis{
        Audio:  audio,
        Format: format,
    }, nil
}

type cartesiaTTSRequest struct {
    ModelID          string                    `json:"model_id"`
    Transcript       string                    `json:"transcript"`
    Voice            cartesiaVoiceSpec         `json:"voice"`
    OutputFormat     cartesiaOutputFormat      `json:"output_format"`
    Language         *string                   `json:"language,omitempty"`
    GenerationConfig *cartesiaGenerationConfig `json:"generation_config,omitempty"`
}

type cartesiaVoiceSpec struct {
    Mode string `json:"mode"`
    ID   string `json:"id"`
}

type cartesiaOutputFormat struct {
    Container  string `json:"container"`
    Encoding   string `json:"encoding,omitempty"`
    SampleRate int    `json:"sample_rate,omitempty"`
    BitRate    int    `json:"bit_rate,omitempty"`
}

type cartesiaGenerationConfig struct {
    Speed   float64 `json:"speed,omitempty"`
    Volume  float64 `json:"volume,omitempty"`
    Emotion string  `json:"emotion,omitempty"`
}

func (c *CartesiaTTSProvider) buildOutputFormat(opts SynthesizeOptions) cartesiaOutputFormat {
    sampleRate := opts.SampleRate
    if sampleRate == 0 {
        sampleRate = 24000
    }

    switch opts.Format {
    case "mp3":
        bitRate := 128000 // Default 128kbps
        return cartesiaOutputFormat{
            Container:  "mp3",
            SampleRate: sampleRate,
            BitRate:    bitRate,
        }
    case "pcm", "raw":
        return cartesiaOutputFormat{
            Container:  "raw",
            Encoding:   "pcm_s16le",
            SampleRate: sampleRate,
        }
    default: // wav
        return cartesiaOutputFormat{
            Container:  "wav",
            Encoding:   "pcm_s16le",
            SampleRate: sampleRate,
        }
    }
}

func (c *CartesiaTTSProvider) SynthesizeStream(ctx context.Context, text string, opts SynthesizeOptions) (*SynthesisStream, error) {
    voiceID := opts.Voice
    if id, ok := cartesiaVoices[opts.Voice]; ok {
        voiceID = id
    }
    if voiceID == "" {
        voiceID = cartesiaVoices["default"]
    }

    // Build WebSocket URL with query params
    u, _ := url.Parse(cartesiaWSURL)
    q := u.Query()
    q.Set("api_key", c.apiKey)
    q.Set("cartesia_version", cartesiaVersion)
    u.RawQuery = q.Encode()

    // Connect WebSocket
    conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), nil)
    if err != nil {
        return nil, fmt.Errorf("websocket connect: %w", err)
    }

    // Create stream
    stream := &SynthesisStream{
        chunks: make(chan []byte, 100),
        done:   make(chan struct{}),
    }

    // Build output format
    outputFormat := c.buildOutputFormat(opts)

    // Send generation request
    wsReq := cartesiaWSRequest{
        ModelID:    "sonic-3",
        Transcript: text,
        Voice: cartesiaVoiceSpec{
            Mode: "id",
            ID:   voiceID,
        },
        OutputFormat: outputFormat,
        ContextID:    generateContextID(),
    }

    if opts.Speed != 0 || opts.Volume != 0 || opts.Emotion != "" {
        wsReq.GenerationConfig = &cartesiaGenerationConfig{
            Speed:   opts.Speed,
            Volume:  opts.Volume,
            Emotion: opts.Emotion,
        }
    }

    if err := conn.WriteJSON(wsReq); err != nil {
        conn.Close()
        return nil, fmt.Errorf("send request: %w", err)
    }

    // Read chunks in background
    go func() {
        defer close(stream.chunks)
        defer conn.Close()

        for {
            var msg cartesiaWSResponse
            if err := conn.ReadJSON(&msg); err != nil {
                if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
                    return
                }
                stream.err = err
                return
            }

            switch msg.Type {
            case "chunk":
                // Decode base64 audio data
                audioData, err := base64.StdEncoding.DecodeString(msg.Data)
                if err != nil {
                    stream.err = fmt.Errorf("decode audio: %w", err)
                    return
                }
                select {
                case stream.chunks <- audioData:
                case <-stream.done:
                    return
                }

            case "done":
                return

            case "error":
                stream.err = fmt.Errorf("cartesia error: %s", msg.Error)
                return
            }
        }
    }()

    return stream, nil
}

type cartesiaWSRequest struct {
    ModelID          string                    `json:"model_id"`
    Transcript       string                    `json:"transcript"`
    Voice            cartesiaVoiceSpec         `json:"voice"`
    OutputFormat     cartesiaOutputFormat      `json:"output_format"`
    GenerationConfig *cartesiaGenerationConfig `json:"generation_config,omitempty"`
    ContextID        string                    `json:"context_id,omitempty"`
}

type cartesiaWSResponse struct {
    Type       string `json:"type"` // "chunk", "done", "error"
    Data       string `json:"data,omitempty"` // base64 audio for "chunk"
    Done       bool   `json:"done,omitempty"`
    Error      string `json:"error,omitempty"`
    StatusCode int    `json:"status_code,omitempty"`
}

var contextCounter atomic.Uint64

func generateContextID() string {
    return fmt.Sprintf("ctx_%d", contextCounter.Add(1))
}
```

### 4.6 Voice Pipeline (pkg/core/voice/pipeline.go)

```go
package voice

import (
    "bytes"
    "context"
    "encoding/base64"
    "fmt"
    "sync"

    "github.com/vango-go/vai/pkg/core/types"
    "github.com/vango-go/vai/pkg/core/voice/stt"
    "github.com/vango-go/vai/pkg/core/voice/tts"
)

// Pipeline handles STT and TTS for message flows.
type Pipeline struct {
    sttProvider stt.Provider
    ttsProvider tts.Provider
}

// NewPipeline creates a new voice pipeline with Cartesia providers.
func NewPipeline(cartesiaAPIKey string) *Pipeline {
    return &Pipeline{
        sttProvider: stt.NewCartesia(cartesiaAPIKey),
        ttsProvider: tts.NewCartesiaTTS(cartesiaAPIKey),
    }
}

// ProcessInputAudio transcribes audio content blocks in messages.
func (p *Pipeline) ProcessInputAudio(ctx context.Context, messages []types.Message, cfg *types.VoiceConfig) ([]types.Message, string, error) {
    if cfg == nil || cfg.Input == nil {
        return messages, "", nil // No voice input config
    }

    var transcript string
    result := make([]types.Message, 0, len(messages))

    for _, msg := range messages {
        blocks := msg.ContentBlocks()
        newBlocks := make([]types.ContentBlock, 0, len(blocks))

        for _, block := range blocks {
            audioBlock, ok := block.(types.AudioBlock)
            if !ok {
                newBlocks = append(newBlocks, block)
                continue
            }

            // Decode audio
            audioData, err := base64.StdEncoding.DecodeString(audioBlock.Source.Data)
            if err != nil {
                return nil, "", fmt.Errorf("decode audio: %w", err)
            }

            // Transcribe
            trans, err := p.sttProvider.Transcribe(ctx, bytes.NewReader(audioData), stt.TranscribeOptions{
                Model:      cfg.Input.Model,
                Language:   cfg.Input.Language,
                Format:     getFormatFromMediaType(audioBlock.Source.MediaType),
                Timestamps: true,
            })
            if err != nil {
                return nil, "", fmt.Errorf("transcribe: %w", err)
            }

            transcript = trans.Text

            // Replace audio with text
            newBlocks = append(newBlocks, types.TextBlock{
                Type: "text",
                Text: trans.Text,
            })
        }

        result = append(result, types.Message{
            Role:    msg.Role,
            Content: newBlocks,
        })
    }

    return result, transcript, nil
}

// SynthesizeResponse converts text response to audio.
func (p *Pipeline) SynthesizeResponse(ctx context.Context, text string, cfg *types.VoiceConfig) ([]byte, error) {
    if cfg == nil || cfg.Output == nil {
        return nil, nil
    }

    synth, err := p.ttsProvider.Synthesize(ctx, text, tts.SynthesizeOptions{
        Voice:   cfg.Output.Voice,
        Speed:   cfg.Output.Speed,
        Volume:  cfg.Output.Volume,
        Emotion: cfg.Output.Emotion,
        Format:  cfg.Output.Format,
    })
    if err != nil {
        return nil, fmt.Errorf("synthesize: %w", err)
    }

    return synth.Audio, nil
}

// StreamingSynthesizer handles streaming TTS for chunked text.
type StreamingSynthesizer struct {
    pipeline *Pipeline
    cfg      *types.VoiceConfig
    buffer   *SentenceBuffer
    chunks   chan AudioChunk
    done     chan struct{}
    wg       sync.WaitGroup
}

// AudioChunk represents a chunk of synthesized audio.
type AudioChunk struct {
    Data   []byte
    Format string
}

// NewStreamingSynthesizer creates a synthesizer for streaming text.
func (p *Pipeline) NewStreamingSynthesizer(cfg *types.VoiceConfig) *StreamingSynthesizer {
    return &StreamingSynthesizer{
        pipeline: p,
        cfg:      cfg,
        buffer:   NewSentenceBuffer(),
        chunks:   make(chan AudioChunk, 10),
        done:     make(chan struct{}),
    }
}

// AddText adds text to the synthesizer.
// Complete sentences are immediately synthesized.
func (s *StreamingSynthesizer) AddText(ctx context.Context, text string) {
    sentences := s.buffer.Add(text)

    for _, sentence := range sentences {
        s.wg.Add(1)
        go func(sent string) {
            defer s.wg.Done()

            synth, err := s.pipeline.ttsProvider.Synthesize(ctx, sent, tts.SynthesizeOptions{
                Voice:   s.cfg.Output.Voice,
                Speed:   s.cfg.Output.Speed,
                Volume:  s.cfg.Output.Volume,
                Emotion: s.cfg.Output.Emotion,
                Format:  s.cfg.Output.Format,
            })
            if err != nil {
                return
            }

            select {
            case s.chunks <- AudioChunk{Data: synth.Audio, Format: synth.Format}:
            case <-s.done:
            }
        }(sentence)
    }
}

// Flush synthesizes any remaining text.
func (s *StreamingSynthesizer) Flush(ctx context.Context) {
    remaining := s.buffer.Flush()
    if remaining != "" {
        s.AddText(ctx, remaining+".")
    }
}

// Chunks returns the channel of audio chunks.
func (s *StreamingSynthesizer) Chunks() <-chan AudioChunk {
    return s.chunks
}

// Close waits for all synthesis to complete and closes the channel.
func (s *StreamingSynthesizer) Close() {
    close(s.done)
    s.wg.Wait()
    close(s.chunks)
}

func getFormatFromMediaType(mediaType string) string {
    switch mediaType {
    case "audio/wav":
        return "wav"
    case "audio/mpeg", "audio/mp3":
        return "mp3"
    case "audio/webm":
        return "webm"
    case "audio/ogg":
        return "ogg"
    case "audio/flac":
        return "flac"
    default:
        return "wav"
    }
}
```

### 4.7 Sentence Buffer (pkg/core/voice/buffer.go)

```go
package voice

import (
    "strings"
)

// SentenceBuffer accumulates text and extracts complete sentences.
// This enables low-latency TTS by synthesizing sentences as they complete.
type SentenceBuffer struct {
    buffer strings.Builder
}

// NewSentenceBuffer creates a new sentence buffer.
func NewSentenceBuffer() *SentenceBuffer {
    return &SentenceBuffer{}
}

// Add adds text to the buffer and returns any complete sentences.
func (b *SentenceBuffer) Add(text string) []string {
    b.buffer.WriteString(text)

    content := b.buffer.String()
    var sentences []string

    // Find sentence boundaries
    lastEnd := 0
    for i := 0; i < len(content); i++ {
        if isSentenceEnd(content, i) {
            sentences = append(sentences, strings.TrimSpace(content[lastEnd:i+1]))
            lastEnd = i + 1
        }
    }

    // Keep remainder in buffer
    if lastEnd > 0 {
        b.buffer.Reset()
        b.buffer.WriteString(content[lastEnd:])
    }

    return sentences
}

// Flush returns any remaining text and clears the buffer.
func (b *SentenceBuffer) Flush() string {
    result := strings.TrimSpace(b.buffer.String())
    b.buffer.Reset()
    return result
}

// isSentenceEnd checks if position i is a sentence boundary.
func isSentenceEnd(s string, i int) bool {
    if i >= len(s) {
        return false
    }

    c := s[i]
    if c != '.' && c != '!' && c != '?' {
        return false
    }

    // Check it's not an abbreviation (Dr., Mr., etc.)
    if c == '.' && i > 0 {
        // Simple heuristic: check if preceded by a single uppercase letter
        if i >= 2 && s[i-1] >= 'A' && s[i-1] <= 'Z' && (i < 3 || s[i-2] == ' ') {
            return false
        }
    }

    // Check there's whitespace or end of string after
    if i+1 < len(s) && s[i+1] != ' ' && s[i+1] != '\n' {
        return false
    }

    return true
}
```

### 4.8 SDK Audio Service (sdk/audio.go)

```go
package vango

import (
    "bytes"
    "context"

    "github.com/vango-go/vai/pkg/core/voice/stt"
    "github.com/vango-go/vai/pkg/core/voice/tts"
)

// AudioService provides standalone audio utilities.
type AudioService struct {
    client *Client
}

// TranscribeRequest configures transcription.
type TranscribeRequest struct {
    Audio      []byte // Audio data
    Model      string // Model to use (default: "ink-whisper")
    Language   string // ISO language code (default: "en")
    Timestamps bool   // Include word-level timestamps
}

// Transcript is the transcription result.
type Transcript = stt.Transcript

// Transcribe converts audio to text using Cartesia.
func (s *AudioService) Transcribe(ctx context.Context, req *TranscribeRequest) (*Transcript, error) {
    provider := s.client.getSTTProvider()

    return provider.Transcribe(ctx, bytes.NewReader(req.Audio), stt.TranscribeOptions{
        Model:      req.Model,
        Language:   req.Language,
        Timestamps: req.Timestamps,
    })
}

// SynthesizeRequest configures synthesis.
type SynthesizeRequest struct {
    Text       string  // Text to synthesize
    Voice      string  // Cartesia voice ID
    Speed      float64 // Speed multiplier (0.6-1.5)
    Volume     float64 // Volume multiplier (0.5-2.0)
    Emotion    string  // Emotion hint (neutral, happy, sad, etc.)
    Format     string  // Output format: "wav", "mp3", or "pcm"
    SampleRate int     // Sample rate in Hz
}

// SynthesisResult is the synthesis result.
type SynthesisResult struct {
    Audio    []byte  // Audio data
    Format   string  // Audio format
    Duration float64 // Duration in seconds
}

// Synthesize converts text to audio using Cartesia.
func (s *AudioService) Synthesize(ctx context.Context, req *SynthesizeRequest) (*SynthesisResult, error) {
    provider := s.client.getTTSProvider()

    synth, err := provider.Synthesize(ctx, req.Text, tts.SynthesizeOptions{
        Voice:      req.Voice,
        Speed:      req.Speed,
        Volume:     req.Volume,
        Emotion:    req.Emotion,
        Format:     req.Format,
        SampleRate: req.SampleRate,
    })
    if err != nil {
        return nil, err
    }

    return &SynthesisResult{
        Audio:    synth.Audio,
        Format:   synth.Format,
        Duration: synth.Duration,
    }, nil
}

// AudioStream provides streaming synthesis.
type AudioStream struct {
    stream *tts.SynthesisStream
}

// Chunks returns the channel of audio chunks.
func (s *AudioStream) Chunks() <-chan []byte {
    return s.stream.Chunks()
}

// Err returns any error.
func (s *AudioStream) Err() error {
    return s.stream.Err()
}

// Close closes the stream.
func (s *AudioStream) Close() error {
    return s.stream.Close()
}

// StreamSynthesize converts text to streaming audio using Cartesia.
func (s *AudioService) StreamSynthesize(ctx context.Context, req *SynthesizeRequest) (*AudioStream, error) {
    provider := s.client.getTTSProvider()

    stream, err := provider.SynthesizeStream(ctx, req.Text, tts.SynthesizeOptions{
        Voice:      req.Voice,
        Speed:      req.Speed,
        Volume:     req.Volume,
        Emotion:    req.Emotion,
        Format:     req.Format,
        SampleRate: req.SampleRate,
    })
    if err != nil {
        return nil, err
    }

    return &AudioStream{stream: stream}, nil
}
```

### 4.9 SDK Voice Configuration (sdk/voice.go)

```go
package vango

// VoiceConfig configures the voice pipeline.
type VoiceConfig struct {
    Input  *VoiceInputConfig  `json:"input,omitempty"`
    Output *VoiceOutputConfig `json:"output,omitempty"`
}

// VoiceInputConfig configures speech-to-text.
type VoiceInputConfig struct {
    Model    string `json:"model,omitempty"`    // Model: "ink-whisper" (default)
    Language string `json:"language,omitempty"` // ISO language code (default: "en")
}

// VoiceOutputConfig configures text-to-speech.
type VoiceOutputConfig struct {
    Voice   string  `json:"voice,omitempty"`   // Cartesia voice ID
    Speed   float64 `json:"speed,omitempty"`   // Speed: 0.6-1.5 (default: 1.0)
    Volume  float64 `json:"volume,omitempty"`  // Volume: 0.5-2.0 (default: 1.0)
    Emotion string  `json:"emotion,omitempty"` // Emotion (neutral, happy, sad, etc.)
    Format  string  `json:"format,omitempty"`  // Output format: wav, mp3, pcm
}

// VoiceInput creates a VoiceConfig with input settings.
func VoiceInput(opts ...VoiceInputOption) *VoiceConfig {
    cfg := &VoiceConfig{
        Input: &VoiceInputConfig{
            Model:    "ink-whisper",
            Language: "en",
        },
    }
    for _, opt := range opts {
        opt(cfg.Input)
    }
    return cfg
}

// VoiceOutput creates a VoiceConfig with output settings.
func VoiceOutput(voice string, opts ...VoiceOutputOption) *VoiceConfig {
    cfg := &VoiceConfig{
        Output: &VoiceOutputConfig{
            Voice:  voice,
            Speed:  1.0,
            Volume: 1.0,
            Format: "wav",
        },
    }
    for _, opt := range opts {
        opt(cfg.Output)
    }
    return cfg
}

// VoiceFull creates a VoiceConfig with both input and output.
func VoiceFull(voice string, opts ...any) *VoiceConfig {
    cfg := &VoiceConfig{
        Input: &VoiceInputConfig{
            Model:    "ink-whisper",
            Language: "en",
        },
        Output: &VoiceOutputConfig{
            Voice:  voice,
            Speed:  1.0,
            Volume: 1.0,
            Format: "wav",
        },
    }
    for _, opt := range opts {
        switch o := opt.(type) {
        case VoiceInputOption:
            o(cfg.Input)
        case VoiceOutputOption:
            o(cfg.Output)
        }
    }
    return cfg
}

// VoiceInputOption configures voice input.
type VoiceInputOption func(*VoiceInputConfig)

// VoiceOutputOption configures voice output.
type VoiceOutputOption func(*VoiceOutputConfig)

// WithLanguage sets the input language.
func WithLanguage(lang string) VoiceInputOption {
    return func(c *VoiceInputConfig) {
        c.Language = lang
    }
}

// WithSTTModel sets the STT model.
func WithSTTModel(model string) VoiceInputOption {
    return func(c *VoiceInputConfig) {
        c.Model = model
    }
}

// WithSpeed sets the TTS speed (0.6-1.5).
func WithSpeed(speed float64) VoiceOutputOption {
    return func(c *VoiceOutputConfig) {
        c.Speed = speed
    }
}

// WithVolume sets the TTS volume (0.5-2.0).
func WithVolume(volume float64) VoiceOutputOption {
    return func(c *VoiceOutputConfig) {
        c.Volume = volume
    }
}

// WithEmotion sets the TTS emotion.
func WithEmotion(emotion string) VoiceOutputOption {
    return func(c *VoiceOutputConfig) {
        c.Emotion = emotion
    }
}

// WithFormat sets the output audio format (wav, mp3, pcm).
func WithFormat(format string) VoiceOutputOption {
    return func(c *VoiceOutputConfig) {
        c.Format = format
    }
}
```

### 4.10 Integrate Voice into MessagesService

```go
// In sdk/messages.go

func (s *MessagesService) createDirect(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
    // Process audio input if voice config is set
    processedMessages := req.Messages
    var userTranscript string

    if req.Voice != nil && req.Voice.Input != nil {
        var err error
        processedMessages, userTranscript, err = s.client.voicePipeline.ProcessInputAudio(ctx, req.Messages, req.Voice)
        if err != nil {
            return nil, fmt.Errorf("process audio input: %w", err)
        }
    }

    // Create modified request with processed messages
    processedReq := *req
    processedReq.Messages = processedMessages

    // Get provider and make request
    provider, err := s.client.core.GetProvider(req.Model)
    if err != nil {
        return nil, err
    }

    resp, err := provider.CreateMessage(ctx, &processedReq)
    if err != nil {
        return nil, err
    }

    // Add user transcript to metadata
    if userTranscript != "" {
        if resp.Metadata == nil {
            resp.Metadata = make(map[string]any)
        }
        resp.Metadata["user_transcript"] = userTranscript
    }

    // Synthesize audio output if configured
    if req.Voice != nil && req.Voice.Output != nil {
        audioData, err := s.client.voicePipeline.SynthesizeResponse(ctx, resp.TextContent(), req.Voice)
        if err != nil {
            return nil, fmt.Errorf("synthesize audio: %w", err)
        }

        if audioData != nil {
            // Add audio content block to response
            resp.Content = append(resp.Content, types.AudioBlock{
                Type: "audio",
                Source: types.AudioSource{
                    Type:      "base64",
                    MediaType: "audio/wav",
                    Data:      base64.StdEncoding.EncodeToString(audioData),
                },
                Transcript: &resp.TextContent(),
            })
        }
    }

    return resp, nil
}
```

---

## Testing Strategy

### Unit Tests

- Sentence buffer logic
- Audio format detection
- Response parsing

### Integration Tests

```go
// +build integration

func TestTranscribe_Cartesia(t *testing.T) {
    if os.Getenv("CARTESIA_API_KEY") == "" {
        t.Skip("CARTESIA_API_KEY not set")
    }

    client := vai.NewClient()

    // Load test audio
    audio, _ := os.ReadFile("testdata/hello.wav")

    transcript, err := client.Audio.Transcribe(context.Background(), &vai.TranscribeRequest{
        Audio:    audio,
        Language: "en",
    })

    require.NoError(t, err)
    assert.Contains(t, strings.ToLower(transcript.Text), "hello")
}

func TestSynthesize_Cartesia(t *testing.T) {
    if os.Getenv("CARTESIA_API_KEY") == "" {
        t.Skip("CARTESIA_API_KEY not set")
    }

    client := vai.NewClient()

    result, err := client.Audio.Synthesize(context.Background(), &vai.SynthesizeRequest{
        Text:   "Hello, this is a test.",
        Voice:  "a0e99841-438c-4a64-b679-ae501e7d6091", // Example voice ID
        Format: "wav",
    })

    require.NoError(t, err)
    assert.True(t, len(result.Audio) > 1000) // Should be substantial audio
    assert.Equal(t, "wav", result.Format)
}

func TestMessages_VoicePipeline(t *testing.T) {
    if os.Getenv("ANTHROPIC_API_KEY") == "" || os.Getenv("CARTESIA_API_KEY") == "" {
        t.Skip("API keys not set")
    }

    client := vai.NewClient()

    // Load test audio asking a question
    audio, _ := os.ReadFile("testdata/what_is_two_plus_two.wav")

    resp, err := client.Messages.Create(context.Background(), &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: []vai.ContentBlock{
                vai.Audio(audio, "audio/wav"),
            }},
        },
        Voice: vai.VoiceInput(vai.WithLanguage("en")),
    })

    require.NoError(t, err)
    // The response should contain the answer to "what is 2+2"
    assert.Contains(t, strings.ToLower(resp.TextContent()), "4")
}

func TestStream_WithVoiceOutput(t *testing.T) {
    if os.Getenv("ANTHROPIC_API_KEY") == "" || os.Getenv("CARTESIA_API_KEY") == "" {
        t.Skip("API keys not set")
    }

    client := vai.NewClient()

    stream, err := client.Messages.Stream(context.Background(), &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Say hello in one sentence.")},
        },
        Voice: vai.VoiceOutput("a0e99841-438c-4a64-b679-ae501e7d6091"),
    })
    require.NoError(t, err)
    defer stream.Close()

    var gotAudioDelta bool
    var audioData []byte

    for event := range stream.Events() {
        if ad, ok := event.(vai.AudioDeltaEvent); ok {
            gotAudioDelta = true
            chunk, _ := base64.StdEncoding.DecodeString(ad.Delta.Data)
            audioData = append(audioData, chunk...)
        }
    }

    assert.True(t, gotAudioDelta, "expected audio delta events")
    assert.True(t, len(audioData) > 0, "expected audio data")
}

func TestStreamSynthesize_Cartesia(t *testing.T) {
    if os.Getenv("CARTESIA_API_KEY") == "" {
        t.Skip("CARTESIA_API_KEY not set")
    }

    client := vai.NewClient()

    stream, err := client.Audio.StreamSynthesize(context.Background(), &vai.SynthesizeRequest{
        Text:   "This is a longer text that will be streamed as audio chunks.",
        Voice:  "a0e99841-438c-4a64-b679-ae501e7d6091",
        Format: "wav",
    })
    require.NoError(t, err)
    defer stream.Close()

    var totalBytes int
    for chunk := range stream.Chunks() {
        totalBytes += len(chunk)
    }

    require.NoError(t, stream.Err())
    assert.True(t, totalBytes > 0, "expected audio data")
}
```

---

## Acceptance Criteria

1. [x] Cartesia STT integration works (ink-whisper model)
2. [x] Cartesia TTS integration works (sonic-3 model)
3. [x] Audio content blocks are transcribed before LLM call
4. [x] Text responses are synthesized to audio
5. [ ] Streaming responses include audio_delta events (deferred to Phase 5)
6. [x] Sentence buffering provides low-latency TTS
7. [x] WebSocket streaming TTS works
8. [x] `Audio.Transcribe()` works standalone
9. [x] `Audio.Synthesize()` works standalone
10. [x] `Audio.StreamSynthesize()` works standalone
11. [ ] All integration tests pass with real APIs (Phase 5)

---

## Files Created

```
pkg/core/voice/
├── pipeline.go              # Voice pipeline orchestration (STT → LLM → TTS)
├── buffer.go                # Sentence boundary buffering for low-latency TTS
├── buffer_test.go           # Unit tests for sentence buffer
├── stt/
│   ├── provider.go          # STT provider interface
│   └── cartesia.go          # Cartesia STT (ink-whisper model)
└── tts/
    ├── provider.go          # TTS provider interface
    └── cartesia.go          # Cartesia TTS (sonic-3 model, batch + WebSocket)

pkg/core/types/
├── voice.go                 # Updated: Cartesia-only VoiceConfig
└── response.go              # Updated: Added UserTranscript() method, Metadata field

sdk/
├── audio.go                 # AudioService (Transcribe, Synthesize, StreamSynthesize)
├── voice.go                 # VoiceInput(), VoiceOutput(), VoiceFull() helpers
├── client.go                # Updated: Added Audio service, voice pipeline init
└── messages.go              # Updated: Voice processing in Create()
```

---

## Environment Variables

```bash
# Single API key for both STT and TTS
CARTESIA_API_KEY=your_cartesia_api_key
```

---

## Estimated Effort

- Cartesia STT implementation: ~200 lines
- Cartesia TTS implementation: ~300 lines
- Voice pipeline: ~250 lines
- SDK integration: ~300 lines
- Tests: ~250 lines
- **Total: ~1300 lines**

---

## Future Phases

Additional providers can be added in future phases while maintaining backward compatibility:

```go
// Future: pkg/core/voice/stt/deepgram.go
// Future: pkg/core/voice/stt/whisper.go
// Future: pkg/core/voice/tts/elevenlabs.go
// Future: pkg/core/voice/tts/openai.go
```

The provider interfaces (`stt.Provider`, `tts.Provider`) are designed to support this extensibility.

---

## Next Phase

Phase 5: Integration Tests - comprehensive test suite using real API keys to validate the entire system.
