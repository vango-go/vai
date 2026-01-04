package vai

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
