// Package voice provides voice pipeline functionality for STT and TTS.
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
		ttsProvider: tts.NewCartesia(cartesiaAPIKey),
	}
}

// NewPipelineWithProviders creates a new voice pipeline with custom providers.
func NewPipelineWithProviders(sttProvider stt.Provider, ttsProvider tts.Provider) *Pipeline {
	return &Pipeline{
		sttProvider: sttProvider,
		ttsProvider: ttsProvider,
	}
}

// STTProvider returns the current STT provider.
func (p *Pipeline) STTProvider() stt.Provider {
	return p.sttProvider
}

// TTSProvider returns the current TTS provider.
func (p *Pipeline) TTSProvider() tts.Provider {
	return p.ttsProvider
}

// ProcessInputAudio transcribes audio content blocks in messages.
// It returns the processed messages with audio blocks replaced by text,
// and the concatenated transcript of all audio blocks.
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

	if text == "" {
		return nil, nil
	}

	// Use configured sample rate or default to 44100 Hz
	sampleRate := cfg.Output.SampleRate
	if sampleRate == 0 {
		sampleRate = 44100
	}

	synth, err := p.ttsProvider.Synthesize(ctx, text, tts.SynthesizeOptions{
		Voice:      cfg.Output.Voice,
		Speed:      cfg.Output.Speed,
		Volume:     cfg.Output.Volume,
		Emotion:    cfg.Output.Emotion,
		Format:     cfg.Output.Format,
		SampleRate: sampleRate,
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
	ctx      context.Context
}

// AudioChunk represents a chunk of synthesized audio.
type AudioChunk struct {
	Data   []byte
	Format string
}

// NewStreamingSynthesizer creates a synthesizer for streaming text.
func (p *Pipeline) NewStreamingSynthesizer(ctx context.Context, cfg *types.VoiceConfig) *StreamingSynthesizer {
	return &StreamingSynthesizer{
		pipeline: p,
		cfg:      cfg,
		buffer:   NewSentenceBuffer(),
		chunks:   make(chan AudioChunk, 10),
		done:     make(chan struct{}),
		ctx:      ctx,
	}
}

// AddText adds text to the synthesizer.
// Complete sentences are immediately synthesized.
func (s *StreamingSynthesizer) AddText(text string) {
	sentences := s.buffer.Add(text)

	// Use configured sample rate or default to 44100 Hz
	sampleRate := s.cfg.Output.SampleRate
	if sampleRate == 0 {
		sampleRate = 44100
	}

	for _, sentence := range sentences {
		s.wg.Add(1)
		go func(sent string) {
			defer s.wg.Done()

			synth, err := s.pipeline.ttsProvider.Synthesize(s.ctx, sent, tts.SynthesizeOptions{
				Voice:      s.cfg.Output.Voice,
				Speed:      s.cfg.Output.Speed,
				Volume:     s.cfg.Output.Volume,
				Emotion:    s.cfg.Output.Emotion,
				Format:     s.cfg.Output.Format,
				SampleRate: sampleRate,
			})
			if err != nil {
				return
			}

			select {
			case s.chunks <- AudioChunk{Data: synth.Audio, Format: synth.Format}:
			case <-s.done:
			case <-s.ctx.Done():
			}
		}(sentence)
	}
}

// Flush synthesizes any remaining text.
func (s *StreamingSynthesizer) Flush() {
	remaining := s.buffer.Flush()
	if remaining != "" {
		s.AddText(remaining + ".")
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
	case "audio/wav", "audio/wave", "audio/x-wav":
		return "wav"
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	case "audio/webm":
		return "webm"
	case "audio/ogg":
		return "ogg"
	case "audio/flac":
		return "flac"
	case "audio/m4a":
		return "m4a"
	default:
		return "wav"
	}
}

// NewStreamingTTSContext creates a streaming TTS context for incremental synthesis.
// Text can be sent incrementally via SendText(), and audio is streamed back via Audio().
func (p *Pipeline) NewStreamingTTSContext(ctx context.Context, cfg *types.VoiceConfig) (*tts.StreamingContext, error) {
	if cfg == nil || cfg.Output == nil {
		return nil, fmt.Errorf("voice output config required")
	}

	opts := tts.StreamingContextOptions{
		Voice:   cfg.Output.Voice,
		Speed:   cfg.Output.Speed,
		Volume:  cfg.Output.Volume,
		Emotion: cfg.Output.Emotion,
		Format:  cfg.Output.Format,
	}

	return p.ttsProvider.NewStreamingContext(ctx, opts)
}
