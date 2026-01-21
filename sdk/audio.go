package vai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"

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
	Format     string // Audio format hint (wav, mp3, webm, etc.)
	SampleRate int    // Audio sample rate in Hz
	Timestamps bool   // Include word-level timestamps
}

// Transcript is the transcription result.
type Transcript = stt.Transcript

// Word represents a transcribed word with timing.
type Word = stt.Word

// Transcribe converts audio to text using Cartesia.
func (s *AudioService) Transcribe(ctx context.Context, req *TranscribeRequest) (*Transcript, error) {
	if s.client.mode == modeProxy {
		return s.transcribeViaProxy(ctx, req)
	}
	provider := s.client.getSTTProvider()

	return provider.Transcribe(ctx, bytes.NewReader(req.Audio), stt.TranscribeOptions{
		Model:      req.Model,
		Language:   req.Language,
		Format:     req.Format,
		SampleRate: req.SampleRate,
		Timestamps: req.Timestamps,
	})
}

// SynthesizeRequest configures synthesis.
type SynthesizeRequest struct {
	Text       string  // Text to synthesize (required)
	Voice      string  // Cartesia voice ID (required)
	Speed      float64 // Speed multiplier (0.6-1.5, default 1.0)
	Volume     float64 // Volume multiplier (0.5-2.0, default 1.0)
	Emotion    string  // Emotion hint (neutral, happy, sad, etc.)
	Language   string  // Language code
	Format     string  // Output format: "wav", "mp3", or "pcm" (default: "wav")
	SampleRate int     // Sample rate in Hz (default: 24000)
}

// SynthesisResult is the synthesis result.
type SynthesisResult struct {
	Audio    []byte  // Audio data
	Format   string  // Audio format
	Duration float64 // Duration in seconds
}

// Synthesize converts text to audio using Cartesia.
func (s *AudioService) Synthesize(ctx context.Context, req *SynthesizeRequest) (*SynthesisResult, error) {
	if s.client.mode == modeProxy {
		return s.synthesizeViaProxy(ctx, req)
	}
	provider := s.client.getTTSProvider()

	synth, err := provider.Synthesize(ctx, req.Text, tts.SynthesizeOptions{
		Voice:      req.Voice,
		Speed:      req.Speed,
		Volume:     req.Volume,
		Emotion:    req.Emotion,
		Language:   req.Language,
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

// StreamSynthesize converts text to streaming audio using Cartesia WebSocket.
func (s *AudioService) StreamSynthesize(ctx context.Context, req *SynthesizeRequest) (*AudioStream, error) {
	if s.client.mode == modeProxy {
		return s.streamSynthesizeViaProxy(ctx, req)
	}
	provider := s.client.getTTSProvider()

	stream, err := provider.SynthesizeStream(ctx, req.Text, tts.SynthesizeOptions{
		Voice:      req.Voice,
		Speed:      req.Speed,
		Volume:     req.Volume,
		Emotion:    req.Emotion,
		Language:   req.Language,
		Format:     req.Format,
		SampleRate: req.SampleRate,
	})
	if err != nil {
		return nil, err
	}

	return &AudioStream{stream: stream}, nil
}

type proxyTranscribeResponse struct {
	Type            string     `json:"type"`
	Text            string     `json:"text"`
	Language        string     `json:"language"`
	DurationSeconds float64    `json:"duration_seconds"`
	Words           []stt.Word `json:"words"`
}

func (s *AudioService) transcribeViaProxy(ctx context.Context, req *TranscribeRequest) (*Transcript, error) {
	payload := map[string]any{
		"audio":    base64.StdEncoding.EncodeToString(req.Audio),
		"provider": "cartesia",
		"model":    req.Model,
		"language": req.Language,
	}

	var resp proxyTranscribeResponse
	if err := s.client.doProxyJSON(ctx, http.MethodPost, "/v1/audio", payload, &resp); err != nil {
		return nil, err
	}

	return &Transcript{
		Text:     resp.Text,
		Language: resp.Language,
		Duration: resp.DurationSeconds,
		Words:    resp.Words,
	}, nil
}

type proxySynthesisResponse struct {
	Type            string  `json:"type"`
	Audio           string  `json:"audio"`
	Format          string  `json:"format"`
	DurationSeconds float64 `json:"duration_seconds"`
}

func (s *AudioService) synthesizeViaProxy(ctx context.Context, req *SynthesizeRequest) (*SynthesisResult, error) {
	payload := map[string]any{
		"text":     req.Text,
		"provider": "cartesia",
		"voice":    req.Voice,
		"speed":    req.Speed,
		"format":   req.Format,
	}

	var resp proxySynthesisResponse
	if err := s.client.doProxyJSON(ctx, http.MethodPost, "/v1/audio", payload, &resp); err != nil {
		return nil, err
	}

	audioBytes, err := base64.StdEncoding.DecodeString(resp.Audio)
	if err != nil {
		return nil, err
	}

	return &SynthesisResult{
		Audio:    audioBytes,
		Format:   resp.Format,
		Duration: resp.DurationSeconds,
	}, nil
}

func (s *AudioService) streamSynthesizeViaProxy(ctx context.Context, req *SynthesizeRequest) (*AudioStream, error) {
	payload := map[string]any{
		"text":     req.Text,
		"provider": "cartesia",
		"voice":    req.Voice,
		"speed":    req.Speed,
		"format":   req.Format,
		"stream":   true,
	}

	resp, err := s.client.openProxyStream(ctx, "/v1/audio", payload)
	if err != nil {
		return nil, err
	}

	stream := tts.NewSynthesisStream()
	reader := newSSEReader(resp.Body)

	go func() {
		defer reader.Close()
		defer stream.FinishSending()

		for {
			event, payload, err := reader.Next()
			if err != nil {
				if err != context.Canceled {
					stream.SetError(err)
				}
				return
			}

			switch event {
			case "audio_chunk":
				var chunk struct {
					Data string `json:"data"`
				}
				if err := json.Unmarshal(payload, &chunk); err != nil {
					stream.SetError(err)
					return
				}
				audioBytes, err := base64.StdEncoding.DecodeString(chunk.Data)
				if err != nil {
					stream.SetError(err)
					return
				}
				if !stream.Send(audioBytes) {
					return
				}
			case "done":
				return
			case "error":
				stream.SetError(parseProxyError(http.StatusInternalServerError, payload))
				return
			}
		}
	}()

	return &AudioStream{stream: stream}, nil
}
