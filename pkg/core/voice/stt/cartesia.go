package stt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	cartesiaBaseURL = "https://api.cartesia.ai"
	cartesiaVersion = "2025-04-16"
)

// CartesiaProvider implements the STT Provider interface using Cartesia's API.
type CartesiaProvider struct {
	apiKey     string
	httpClient *http.Client
}

// NewCartesia creates a new Cartesia STT provider.
func NewCartesia(apiKey string) *CartesiaProvider {
	return &CartesiaProvider{
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// NewCartesiaWithClient creates a new Cartesia STT provider with a custom HTTP client.
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

// Transcribe converts audio to text using Cartesia's STT API.
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
	ext := getExtension(opts.Format)
	fw, err := mw.CreateFormFile("file", "audio."+ext)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := fw.Write(audioData); err != nil {
		return nil, fmt.Errorf("write audio data: %w", err)
	}

	// Add model (default to ink-whisper)
	model := opts.Model
	if model == "" {
		model = "ink-whisper"
	}
	if err := mw.WriteField("model", model); err != nil {
		return nil, fmt.Errorf("write model field: %w", err)
	}

	// Add language if specified
	if opts.Language != "" {
		if err := mw.WriteField("language", opts.Language); err != nil {
			return nil, fmt.Errorf("write language field: %w", err)
		}
	}

	// Request word timestamps if enabled
	if opts.Timestamps {
		if err := mw.WriteField("timestamp_granularities[]", "word"); err != nil {
			return nil, fmt.Errorf("write timestamp field: %w", err)
		}
	}

	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	// Build URL with optional query params
	reqURL := cartesiaBaseURL + "/stt"
	if opts.Format != "" || opts.SampleRate > 0 {
		u, _ := url.Parse(reqURL)
		q := u.Query()
		if encoding := getEncoding(opts.Format); encoding != "" {
			q.Set("encoding", encoding)
		}
		if opts.SampleRate > 0 {
			q.Set("sample_rate", fmt.Sprintf("%d", opts.SampleRate))
		}
		u.RawQuery = q.Encode()
		reqURL = u.String()
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
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
	Text     string   `json:"text"`
	Language *string  `json:"language,omitempty"`
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

// TranscribeStream transcribes streaming audio via WebSocket.
// This is a one-shot streaming method where audio comes from an io.Reader.
// For live audio input, use NewStreamingSTT instead.
func (c *CartesiaProvider) TranscribeStream(ctx context.Context, audio io.Reader, opts TranscribeOptions) (<-chan TranscriptDelta, error) {
	stream, err := c.NewStreamingSTT(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Read audio and send in chunks
	go func() {
		defer stream.Close()
		buf := make([]byte, 4096) // ~85ms at 24kHz 16-bit mono
		for {
			n, err := audio.Read(buf)
			if n > 0 {
				if sendErr := stream.SendAudio(buf[:n]); sendErr != nil {
					return
				}
			}
			if err == io.EOF {
				stream.Finalize()
				return
			}
			if err != nil {
				return
			}
		}
	}()

	return stream.Transcripts(), nil
}

// StreamingSTT represents a real-time streaming transcription session.
type StreamingSTT struct {
	conn        *websocket.Conn
	transcripts chan TranscriptDelta
	done        chan struct{}
	closed      atomic.Bool
	writeMu     sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewStreamingSTT creates a new streaming STT session via WebSocket.
// Audio can be sent incrementally via SendAudio, and transcripts received via Transcripts.
func (c *CartesiaProvider) NewStreamingSTT(ctx context.Context, opts TranscribeOptions) (*StreamingSTT, error) {
	// Build WebSocket URL with query params
	u, err := url.Parse("wss://api.cartesia.ai/stt/websocket")
	if err != nil {
		return nil, fmt.Errorf("parse websocket URL: %w", err)
	}

	q := u.Query()

	// Model (required)
	model := opts.Model
	if model == "" {
		model = "ink-whisper"
	}
	q.Set("model", model)

	// Language (required, default: en)
	language := opts.Language
	if language == "" {
		language = "en"
	}
	q.Set("language", language)

	// Encoding (required) - we use pcm_s16le for best performance
	encoding := opts.Format
	if encoding == "" {
		encoding = "pcm_s16le"
	}
	q.Set("encoding", encoding)

	// Sample rate (required)
	sampleRate := opts.SampleRate
	if sampleRate == 0 {
		sampleRate = 16000 // Cartesia recommends 16kHz
	}
	q.Set("sample_rate", fmt.Sprintf("%d", sampleRate))

	// We do our own VAD with semantic checks, so don't set max_silence_duration_secs.
	// This makes Cartesia stream interim transcripts continuously without waiting for silence.
	// min_volume is still useful to filter out background noise.
	q.Set("min_volume", "0.01") // Low threshold to catch quiet speech

	// API key can be passed as query param (useful for browser) or header
	// We'll use both for maximum compatibility
	q.Set("api_key", c.apiKey)

	u.RawQuery = q.Encode()

	// Set up headers with API key and required version
	headers := http.Header{}
	headers.Set("X-API-Key", c.apiKey)
	headers.Set("Cartesia-Version", cartesiaVersion)

	// Connect WebSocket
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, resp, err := dialer.DialContext(ctx, u.String(), headers)
	if err != nil {
		// Try to get more details from the response
		if resp != nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if len(body) > 0 {
				return nil, fmt.Errorf("websocket connect (status %d): %s", resp.StatusCode, string(body))
			}
			return nil, fmt.Errorf("websocket connect: status %d: %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("websocket connect: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	s := &StreamingSTT{
		conn:        conn,
		transcripts: make(chan TranscriptDelta, 100),
		done:        make(chan struct{}),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start read loop
	go s.readLoop()

	return s, nil
}

func (s *StreamingSTT) readLoop() {
	defer func() {
		close(s.transcripts)
		close(s.done)
	}()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		_, data, err := s.conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				// Could log error here
			}
			return
		}

		var msg cartesiaSTTResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "transcript":
			delta := TranscriptDelta{
				Text:    msg.Text,
				IsFinal: msg.IsFinal,
			}
			if msg.Duration > 0 {
				delta.Timestamp = msg.Duration
			}
			select {
			case s.transcripts <- delta:
			case <-s.done:
				return
			}

		case "flush_done":
			// Acknowledgment of finalize command
			continue

		case "done":
			// Session closing
			return

		case "error":
			// Error occurred - could emit error through channel
			return
		}
	}
}

type cartesiaSTTResponse struct {
	Type      string  `json:"type"`     // "transcript", "flush_done", "done", "error"
	Text      string  `json:"text"`     // Transcribed text
	IsFinal   bool    `json:"is_final"` // Whether this is final
	Duration  float64 `json:"duration"` // Audio duration
	Language  string  `json:"language"` // Detected language
	RequestID string  `json:"request_id"`
	Error     string  `json:"error"` // Error message if type is "error"
	Words     []struct {
		Word  string  `json:"word"`
		Start float64 `json:"start"`
		End   float64 `json:"end"`
	} `json:"words"`
}

// SendAudio sends audio data to the streaming STT session.
// Audio should be in the format specified during session creation (default: pcm_s16le at 16kHz).
func (s *StreamingSTT) SendAudio(data []byte) error {
	if s.closed.Load() {
		return fmt.Errorf("session closed")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.BinaryMessage, data)
}

// SendAudioBase64 sends base64-encoded audio data.
func (s *StreamingSTT) SendAudioBase64(data string) error {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return fmt.Errorf("decode base64: %w", err)
	}
	return s.SendAudio(decoded)
}

// Finalize flushes any remaining audio and signals end of input.
// Use this when the user stops speaking but you want to keep the session open.
func (s *StreamingSTT) Finalize() error {
	if s.closed.Load() {
		return fmt.Errorf("session closed")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.TextMessage, []byte("finalize"))
}

// Transcripts returns the channel of transcript deltas.
func (s *StreamingSTT) Transcripts() <-chan TranscriptDelta {
	return s.transcripts
}

// Done returns a channel that's closed when the session ends.
func (s *StreamingSTT) Done() <-chan struct{} {
	return s.done
}

// Close closes the streaming STT session.
func (s *StreamingSTT) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	s.cancel()

	s.writeMu.Lock()
	// Send "done" command to gracefully close
	s.conn.WriteMessage(websocket.TextMessage, []byte("done"))
	s.conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	s.writeMu.Unlock()

	return s.conn.Close()
}

// getExtension returns the file extension for the given audio format.
func getExtension(format string) string {
	switch format {
	case "wav", "mp3", "webm", "ogg", "flac", "m4a", "mp4", "mpeg", "mpga", "oga":
		return format
	default:
		return "wav"
	}
}

// getEncoding returns the PCM encoding for raw audio formats.
func getEncoding(format string) string {
	switch format {
	case "pcm_s16le", "pcm_s32le", "pcm_f16le", "pcm_f32le", "pcm_mulaw", "pcm_alaw":
		return format
	default:
		return ""
	}
}
