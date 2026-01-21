package vai

import "github.com/vango-go/vai/pkg/core/types"

// VoiceInput creates a VoiceConfig with input settings for STT.
func VoiceInput(opts ...VoiceInputOption) *VoiceConfig {
	cfg := &VoiceConfig{
		Input: &types.VoiceInputConfig{
			Model:    "ink-whisper",
			Language: "en",
		},
	}
	for _, opt := range opts {
		opt(cfg.Input)
	}
	return cfg
}

// VoiceOutput creates a VoiceConfig with output settings for TTS.
func VoiceOutput(voice string, opts ...VoiceOutputOption) *VoiceConfig {
	cfg := &VoiceConfig{
		Output: &types.VoiceOutputConfig{
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

// VoiceFull creates a VoiceConfig with both input and output settings.
func VoiceFull(voice string, opts ...any) *VoiceConfig {
	cfg := &VoiceConfig{
		Input: &types.VoiceInputConfig{
			Model:    "ink-whisper",
			Language: "en",
		},
		Output: &types.VoiceOutputConfig{
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
type VoiceInputOption func(*types.VoiceInputConfig)

// VoiceOutputOption configures voice output.
type VoiceOutputOption func(*types.VoiceOutputConfig)

// WithLanguage sets the input language.
func WithLanguage(lang string) VoiceInputOption {
	return func(c *types.VoiceInputConfig) {
		c.Language = lang
	}
}

// WithSTTModel sets the STT model.
func WithSTTModel(model string) VoiceInputOption {
	return func(c *types.VoiceInputConfig) {
		c.Model = model
	}
}

// WithSpeed sets the TTS speed (0.6-1.5).
func WithSpeed(speed float64) VoiceOutputOption {
	return func(c *types.VoiceOutputConfig) {
		c.Speed = speed
	}
}

// WithVolume sets the TTS volume (0.5-2.0).
func WithVolume(volume float64) VoiceOutputOption {
	return func(c *types.VoiceOutputConfig) {
		c.Volume = volume
	}
}

// WithEmotion sets the TTS emotion.
func WithEmotion(emotion string) VoiceOutputOption {
	return func(c *types.VoiceOutputConfig) {
		c.Emotion = emotion
	}
}

// WithAudioFormat sets the output audio format (wav, mp3, pcm).
func WithAudioFormat(format string) VoiceOutputOption {
	return func(c *types.VoiceOutputConfig) {
		c.Format = format
	}
}

// Emotion constants for convenience
const (
	EmotionNeutral      = types.EmotionNeutral
	EmotionHappy        = types.EmotionHappy
	EmotionExcited      = types.EmotionExcited
	EmotionEnthusiastic = types.EmotionEnthusiastic
	EmotionCalm         = types.EmotionCalm
	EmotionPeaceful     = types.EmotionPeaceful
	EmotionAngry        = types.EmotionAngry
	EmotionFrustrated   = types.EmotionFrustrated
	EmotionSad          = types.EmotionSad
	EmotionMelancholic  = types.EmotionMelancholic
	EmotionScared       = types.EmotionScared
	EmotionAnxious      = types.EmotionAnxious
	EmotionConfident    = types.EmotionConfident
	EmotionCurious      = types.EmotionCurious
)

// Audio format constants
const (
	AudioFormatWAV = types.VoiceFormatWAV
	AudioFormatMP3 = types.VoiceFormatMP3
	AudioFormatPCM = types.VoiceFormatPCM
)
