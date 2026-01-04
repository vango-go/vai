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
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

const (
	cartesiaBaseURL = "https://api.cartesia.ai"
	cartesiaWSURL   = "wss://api.cartesia.ai/tts/websocket"
	cartesiaVersion = "2025-04-16"
)

// Default voice ID - users should provide their own voice IDs
const defaultVoiceID = "a0e99841-438c-4a64-b679-ae501e7d6091"

// CartesiaProvider implements the TTS Provider interface using Cartesia's API.
type CartesiaProvider struct {
	apiKey     string
	httpClient *http.Client
}

// NewCartesia creates a new Cartesia TTS provider.
func NewCartesia(apiKey string) *CartesiaProvider {
	return &CartesiaProvider{
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// NewCartesiaWithClient creates a new Cartesia TTS provider with a custom HTTP client.
func NewCartesiaWithClient(apiKey string, client *http.Client) *CartesiaProvider {
	return &CartesiaProvider{
		apiKey:     apiKey,
		httpClient: client,
	}
}

// Name returns the provider identifier.
func (c *CartesiaProvider) Name() string {
	return "cartesia"
}

// Synthesize converts text to audio using Cartesia's TTS API.
func (c *CartesiaProvider) Synthesize(ctx context.Context, text string, opts SynthesizeOptions) (*Synthesis, error) {
	voiceID := opts.Voice
	if voiceID == "" {
		voiceID = defaultVoiceID
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
		genConfig := &cartesiaGenerationConfig{}
		if opts.Speed != 0 {
			genConfig.Speed = opts.Speed
		}
		if opts.Volume != 0 {
			genConfig.Volume = opts.Volume
		}
		if opts.Emotion != "" {
			genConfig.Emotion = opts.Emotion
		}
		reqBody.GenerationConfig = genConfig
	}

	if opts.Language != "" {
		reqBody.Language = &opts.Language
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", cartesiaBaseURL+"/tts/bytes", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
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
		return &Synthesis{Audio: []byte{}, Format: getFormat(opts.Format)}, nil
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

	return &Synthesis{
		Audio:  audio,
		Format: getFormat(opts.Format),
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

func (c *CartesiaProvider) buildOutputFormat(opts SynthesizeOptions) cartesiaOutputFormat {
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

func getFormat(format string) string {
	switch format {
	case "mp3", "pcm", "raw", "wav":
		return format
	default:
		return "wav"
	}
}

// SynthesizeStream converts text to streaming audio using Cartesia's WebSocket API.
func (c *CartesiaProvider) SynthesizeStream(ctx context.Context, text string, opts SynthesizeOptions) (*SynthesisStream, error) {
	voiceID := opts.Voice
	if voiceID == "" {
		voiceID = defaultVoiceID
	}

	// Build WebSocket URL with query params
	u, err := url.Parse(cartesiaWSURL)
	if err != nil {
		return nil, fmt.Errorf("parse websocket URL: %w", err)
	}
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
	stream := NewSynthesisStream()

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
		genConfig := &cartesiaGenerationConfig{}
		if opts.Speed != 0 {
			genConfig.Speed = opts.Speed
		}
		if opts.Volume != 0 {
			genConfig.Volume = opts.Volume
		}
		if opts.Emotion != "" {
			genConfig.Emotion = opts.Emotion
		}
		wsReq.GenerationConfig = genConfig
	}

	if opts.Language != "" {
		wsReq.Language = &opts.Language
	}

	if err := conn.WriteJSON(wsReq); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send request: %w", err)
	}

	// Read chunks in background
	go func() {
		defer stream.FinishSending()
		defer conn.Close()

		for {
			select {
			case <-ctx.Done():
				stream.SetError(ctx.Err())
				return
			case <-stream.done:
				return
			default:
			}

			var msg cartesiaWSResponse
			if err := conn.ReadJSON(&msg); err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					return
				}
				stream.SetError(err)
				return
			}

			switch msg.Type {
			case "chunk":
				// Decode base64 audio data
				audioData, err := base64.StdEncoding.DecodeString(msg.Data)
				if err != nil {
					stream.SetError(fmt.Errorf("decode audio: %w", err))
					return
				}
				if !stream.Send(audioData) {
					return
				}

			case "done":
				return

			case "error":
				stream.SetError(fmt.Errorf("cartesia error: %s", msg.Error))
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
	Language         *string                   `json:"language,omitempty"`
	ContextID        string                    `json:"context_id,omitempty"`
}

type cartesiaWSResponse struct {
	Type       string `json:"type"` // "chunk", "done", "error"
	Data       string `json:"data,omitempty"`
	Done       bool   `json:"done,omitempty"`
	Error      string `json:"error,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
}

var contextCounter atomic.Uint64

func generateContextID() string {
	return fmt.Sprintf("ctx_%d", contextCounter.Add(1))
}

// NewStreamingContext creates a streaming context for incremental text-to-speech.
// Text chunks can be sent via SendText(), and audio chunks are received via Audio().
func (c *CartesiaProvider) NewStreamingContext(ctx context.Context, opts StreamingContextOptions) (*StreamingContext, error) {
	voiceID := opts.Voice
	if voiceID == "" {
		voiceID = defaultVoiceID
	}

	// Build WebSocket URL with query params
	u, err := url.Parse(cartesiaWSURL)
	if err != nil {
		return nil, fmt.Errorf("parse websocket URL: %w", err)
	}
	q := u.Query()
	q.Set("api_key", c.apiKey)
	q.Set("cartesia_version", cartesiaVersion)
	u.RawQuery = q.Encode()

	// Connect WebSocket
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("websocket connect: %w", err)
	}

	// Create streaming context
	sc := NewStreamingContext()
	contextID := generateContextID()

	// Build output format from options
	sampleRate := opts.SampleRate
	if sampleRate == 0 {
		sampleRate = 24000
	}

	var outputFormat cartesiaOutputFormat
	switch opts.Format {
	case "mp3":
		outputFormat = cartesiaOutputFormat{
			Container:  "mp3",
			SampleRate: sampleRate,
			BitRate:    128000,
		}
	case "pcm", "raw":
		outputFormat = cartesiaOutputFormat{
			Container:  "raw",
			Encoding:   "pcm_s16le",
			SampleRate: sampleRate,
		}
	default: // wav
		outputFormat = cartesiaOutputFormat{
			Container:  "wav",
			Encoding:   "pcm_s16le",
			SampleRate: sampleRate,
		}
	}

	// Default buffer delay for good quality with reasonable latency
	maxBufferDelay := opts.MaxBufferDelayMs
	if maxBufferDelay == 0 {
		maxBufferDelay = 500 // 500ms default
	}

	// Build base request template
	baseReq := cartesiaStreamingRequest{
		ModelID: "sonic-3",
		Voice: cartesiaVoiceSpec{
			Mode: "id",
			ID:   voiceID,
		},
		OutputFormat:     outputFormat,
		ContextID:        contextID,
		MaxBufferDelayMs: maxBufferDelay,
	}

	if opts.Speed != 0 || opts.Volume != 0 || opts.Emotion != "" {
		genConfig := &cartesiaGenerationConfig{}
		if opts.Speed != 0 {
			genConfig.Speed = opts.Speed
		}
		if opts.Volume != 0 {
			genConfig.Volume = opts.Volume
		}
		if opts.Emotion != "" {
			genConfig.Emotion = opts.Emotion
		}
		baseReq.GenerationConfig = genConfig
	}

	if opts.Language != "" {
		baseReq.Language = &opts.Language
	}

	// Mutex for WebSocket writes
	var writeMu sync.Mutex

	// SendFunc sends text chunks to WebSocket
	sc.SendFunc = func(text string, isFinal bool) error {
		writeMu.Lock()
		defer writeMu.Unlock()

		req := baseReq
		req.Transcript = text

		// Continue=true means more text is coming.
		// Continue=false tells Cartesia this is the final chunk and closes the context.
		// We must keep Continue=true until isFinal, otherwise Cartesia closes the context
		// and rejects subsequent chunks with "Context has closed" error.
		req.Continue = !isFinal

		if isFinal && text == "" {
			// No text to send - signal end of stream
			req.Transcript = ""
			req.Continue = false
			return conn.WriteJSON(req)
		}

		return conn.WriteJSON(req)
	}

	// CloseFunc closes the WebSocket
	sc.CloseFunc = func() error {
		return conn.Close()
	}

	// Read audio chunks in background
	go func() {
		defer sc.FinishAudio()
		defer conn.Close()

		for {
			select {
			case <-ctx.Done():
				sc.SetError(ctx.Err())
				return
			case <-sc.Done():
				return
			default:
			}

			var msg cartesiaWSResponse
			if err := conn.ReadJSON(&msg); err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					return
				}
				sc.SetError(err)
				return
			}

			switch msg.Type {
			case "chunk":
				// Decode base64 audio data
				audioData, err := base64.StdEncoding.DecodeString(msg.Data)
				if err != nil {
					sc.SetError(fmt.Errorf("decode audio: %w", err))
					return
				}
				if !sc.PushAudio(audioData) {
					return
				}

			case "done":
				return

			case "flush_done":
				// Flush acknowledged, continue reading until "done" with all audio
				continue

			case "error":
				sc.SetError(fmt.Errorf("cartesia error: %s", msg.Error))
				return
			}
		}
	}()

	return sc, nil
}

// cartesiaStreamingRequest is the request format for streaming TTS with continuation.
type cartesiaStreamingRequest struct {
	ModelID          string                    `json:"model_id"`
	Transcript       string                    `json:"transcript"`
	Voice            cartesiaVoiceSpec         `json:"voice"`
	OutputFormat     cartesiaOutputFormat      `json:"output_format"`
	ContextID        string                    `json:"context_id"`
	Continue         bool                      `json:"continue"`
	Flush            bool                      `json:"flush,omitempty"`
	MaxBufferDelayMs int                       `json:"max_buffer_delay_ms,omitempty"`
	GenerationConfig *cartesiaGenerationConfig `json:"generation_config,omitempty"`
	Language         *string                   `json:"language,omitempty"`
}

