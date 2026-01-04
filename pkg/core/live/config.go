package live

import (
	"github.com/vango-go/vai/pkg/core/types"
)

// SessionState represents the current state of the live session.
type SessionState int

const (
	// StateConfiguring is the initial state before session is started.
	StateConfiguring SessionState = iota
	// StateListening is when VAD is active and capturing user speech.
	StateListening
	// StateGracePeriod is the window after VAD commits where user can continue.
	StateGracePeriod
	// StateProcessing is when the agent is generating a response.
	StateProcessing
	// StateSpeaking is when TTS audio is being played.
	StateSpeaking
	// StateInterruptCapturing is when potential interrupt is being analyzed.
	StateInterruptCapturing
	// StateClosed is when the session has been closed.
	StateClosed
)

// String returns a human-readable state name.
func (s SessionState) String() string {
	switch s {
	case StateConfiguring:
		return "CONFIGURING"
	case StateListening:
		return "LISTENING"
	case StateGracePeriod:
		return "GRACE_PERIOD"
	case StateProcessing:
		return "PROCESSING"
	case StateSpeaking:
		return "SPEAKING"
	case StateInterruptCapturing:
		return "INTERRUPT_CAPTURING"
	case StateClosed:
		return "CLOSED"
	default:
		return "UNKNOWN"
	}
}

// SessionConfig holds all configuration for a live session.
type SessionConfig struct {
	// Model is the main LLM model to use for responses.
	Model string `json:"model"`

	// System is the system prompt for the agent.
	System string `json:"system,omitempty"`

	// Tools are the available tools for the agent.
	Tools []types.Tool `json:"tools,omitempty"`

	// Messages are any pre-existing conversation history.
	Messages []types.Message `json:"messages,omitempty"`

	// Voice configures STT and TTS.
	Voice *types.VoiceConfig `json:"voice,omitempty"`

	// VAD configures voice activity detection.
	VAD VADConfig `json:"vad"`

	// GracePeriod configures the post-VAD continuation window.
	GracePeriod GracePeriodConfig `json:"grace_period"`

	// Interrupt configures interrupt detection during bot speech.
	Interrupt InterruptConfig `json:"interrupt"`

	// SampleRate is the audio sample rate in Hz. Default: 24000.
	SampleRate int `json:"sample_rate"`

	// Channels is the number of audio channels. Default: 1 (mono).
	Channels int `json:"channels"`

	// MaxTokens is the maximum tokens for LLM responses.
	MaxTokens int `json:"max_tokens,omitempty"`

	// Temperature controls LLM response randomness.
	Temperature *float64 `json:"temperature,omitempty"`
}

// DefaultSessionConfig returns a SessionConfig with sensible defaults.
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		Model:       "anthropic/claude-haiku-4-5-20251001",
		VAD:         DefaultVADConfig(),
		GracePeriod: DefaultGracePeriodConfig(),
		Interrupt:   DefaultInterruptConfig(),
		SampleRate:  24000,
		Channels:    1,
		MaxTokens:   1024,
	}
}

// VADConfig configures hybrid voice activity detection.
// Uses punctuation-based turn detection with semantic confirmation.
type VADConfig struct {
	// Model is the LLM model for semantic turn completion checks.
	// If empty, uses the session's main Model.
	Model string `json:"model,omitempty"`

	// PunctuationTrigger defines characters that trigger immediate semantic check.
	// When transcript ends with one of these, semantic check runs immediately.
	// Default: ".!?"
	PunctuationTrigger string `json:"punctuation_trigger"`

	// NoActivityTimeoutMs is how long to wait for new transcript before forcing semantic check.
	// This is a fallback for when user doesn't use punctuation.
	// Default: 3000 (3 seconds)
	NoActivityTimeoutMs int `json:"no_activity_timeout_ms"`

	// SemanticCheck enables LLM-based turn completion analysis.
	// If false, punctuation alone determines turn completion.
	// Default: true
	SemanticCheck bool `json:"semantic_check"`

	// MinWordsForCheck is the minimum word count before semantic check runs.
	// Default: 1 (allows "Hello" to trigger semantic check)
	MinWordsForCheck int `json:"min_words_for_check"`

	// EnergyThreshold is the RMS energy level below which audio is considered silence.
	// Used for: (1) filtering silence before sending to STT to reduce costs,
	// (2) interrupt detection during bot speech.
	// Range: 0.0 to 1.0. Default: 0.02
	EnergyThreshold float64 `json:"energy_threshold"`

	// PrefixPadding is audio to keep before speech detection (for context).
	// Default: 300ms
	PrefixPaddingMs int `json:"prefix_padding_ms"`
}

// DefaultVADConfig returns a VADConfig with sensible defaults.
// Uses Groq's fast Llama model for low-latency semantic checks.
func DefaultVADConfig() VADConfig {
	return VADConfig{
		Model:               "groq/moonshotai/kimi-k2-instruct", // Fast model for semantic checks
		PunctuationTrigger:  ".!?",                              // Trigger semantic check on sentence-ending punctuation
		NoActivityTimeoutMs: 3000,                               // 3s fallback timeout for no new transcript
		SemanticCheck:       true,
		MinWordsForCheck:    1,    // Allow semantic check on single words like "Hello"
		EnergyThreshold:     0.02, // Used for STT silence filtering and interrupt detection
		PrefixPaddingMs:     300,
	}
}

// GracePeriodConfig configures the post-VAD continuation window.
type GracePeriodConfig struct {
	// Enabled turns the grace period on or off.
	// If disabled, VAD commit immediately starts agent processing.
	// Default: true
	Enabled bool `json:"enabled"`

	// DurationMs is how long to wait for user continuation after VAD commits.
	// Default: 5000 (5 seconds)
	DurationMs int `json:"duration_ms"`
}

// DefaultGracePeriodConfig returns a GracePeriodConfig with sensible defaults.
func DefaultGracePeriodConfig() GracePeriodConfig {
	return GracePeriodConfig{
		Enabled:    true,
		DurationMs: 5000, // 5 seconds - cancellation window runs in parallel with LLM
	}
}

// InterruptMode specifies how interrupts are handled.
type InterruptMode string

const (
	// InterruptModeAuto uses semantic analysis to distinguish interrupts from backchannels.
	InterruptModeAuto InterruptMode = "auto"
	// InterruptModeAlways treats any user speech during bot output as an interrupt.
	InterruptModeAlways InterruptMode = "always"
	// InterruptModeNever ignores user speech during bot output.
	InterruptModeNever InterruptMode = "never"
)

// PartialSaveMode specifies how partial assistant messages are handled on interrupt.
type PartialSaveMode string

const (
	// PartialSaveNone discards partial messages on interrupt.
	PartialSaveNone PartialSaveMode = "none"
	// PartialSaveMarked saves partial messages with an [interrupted] marker.
	PartialSaveMarked PartialSaveMode = "marked"
	// PartialSaveFull saves partial messages as-is.
	PartialSaveFull PartialSaveMode = "full"
)

// InterruptConfig configures interrupt detection during bot speech.
type InterruptConfig struct {
	// Mode controls how interrupts are detected.
	// Default: "auto"
	Mode InterruptMode `json:"mode"`

	// EnergyThreshold is the RMS energy level to trigger interrupt detection.
	// This is typically higher than VAD threshold to avoid false positives.
	// Default: 0.05
	EnergyThreshold float64 `json:"energy_threshold"`

	// CaptureDurationMs is how long to capture audio before semantic analysis.
	// This allows capturing "Actually wait—" rather than just "Act—".
	// Default: 600
	CaptureDurationMs int `json:"capture_duration_ms"`

	// SemanticCheck enables LLM-based interrupt vs backchannel detection.
	// Only used when Mode is "auto".
	// Default: true
	SemanticCheck bool `json:"semantic_check"`

	// SemanticModel is the LLM model for interrupt semantic checks.
	// If empty, uses the session's main Model.
	SemanticModel string `json:"semantic_model,omitempty"`

	// SavePartial controls how partial assistant messages are saved on interrupt.
	// Default: "marked"
	SavePartial PartialSaveMode `json:"save_partial"`
}

// DefaultInterruptConfig returns an InterruptConfig with sensible defaults.
// Uses Groq's fast Llama model for low-latency interrupt detection.
func DefaultInterruptConfig() InterruptConfig {
	return InterruptConfig{
		Mode:              InterruptModeAuto,
		EnergyThreshold:   0.05,
		CaptureDurationMs: 600,
		SemanticCheck:     true,
		SemanticModel:     "groq/llama-3.3-70b-versatile", // Fast model for interrupt checks
		SavePartial:       PartialSaveMarked,
	}
}

// AudioConfig specifies audio format parameters.
type AudioConfig struct {
	// SampleRate in Hz. Common values: 16000, 24000, 44100, 48000.
	SampleRate int `json:"sample_rate"`

	// Channels: 1 for mono, 2 for stereo.
	Channels int `json:"channels"`

	// BitsPerSample: typically 16 for PCM.
	BitsPerSample int `json:"bits_per_sample"`
}

// DefaultAudioConfig returns the standard audio configuration.
func DefaultAudioConfig() AudioConfig {
	return AudioConfig{
		SampleRate:    24000,
		Channels:      1,
		BitsPerSample: 16,
	}
}

// BytesPerSecond returns the audio byte rate.
func (c AudioConfig) BytesPerSecond() int {
	return c.SampleRate * c.Channels * (c.BitsPerSample / 8)
}

// DurationMs returns the duration in milliseconds for the given byte count.
func (c AudioConfig) DurationMs(bytes int) int {
	if c.BytesPerSecond() == 0 {
		return 0
	}
	return (bytes * 1000) / c.BytesPerSecond()
}

// BytesForDurationMs returns the byte count for the given duration in milliseconds.
func (c AudioConfig) BytesForDurationMs(ms int) int {
	return (c.BytesPerSecond() * ms) / 1000
}
