package vai

import (
	"context"
	"fmt"
	"time"

	"github.com/vango-go/vai/pkg/core/live"
	"github.com/vango-go/vai/pkg/core/types"
	"github.com/vango-go/vai/pkg/core/voice/stt"
	"github.com/vango-go/vai/pkg/core/voice/tts"
)

// LiveSession represents an active real-time voice conversation session.
// It wraps the core live.Session and provides a clean SDK interface.
type LiveSession struct {
	session     *live.Session
	events      chan LiveEvent
	done        chan struct{}
	closed      bool
	audioOutput *AudioOutput
}

// LiveConfig contains configuration for a live session.
type LiveConfig struct {
	// Model is the LLM model to use for responses.
	// Example: "anthropic/claude-haiku-4-5-20251001"
	Model string

	// System is the system prompt for the agent.
	System string

	// Tools are the available tools for the agent.
	Tools []types.Tool

	// Messages are any pre-existing conversation history.
	Messages []types.Message

	// Voice configures STT and TTS.
	Voice *types.VoiceConfig

	// VAD configures voice activity detection.
	VAD *LiveVADConfig

	// GracePeriod configures the post-VAD continuation window.
	GracePeriod *LiveGracePeriodConfig

	// Interrupt configures interrupt detection during bot speech.
	Interrupt *LiveInterruptConfig

	// SampleRate is the audio sample rate in Hz. Default: 24000.
	SampleRate int

	// Channels is the number of audio channels. Default: 1 (mono).
	Channels int

	// MaxTokens is the maximum tokens for LLM responses.
	MaxTokens int

	// Temperature controls LLM response randomness.
	Temperature *float64

	// AudioOutput configures audio output buffering.
	// If nil, uses DefaultAudioOutputConfig().
	AudioOutput *AudioOutputConfig

	// Debug enables debug event emission.
	Debug bool
}

// LiveVADConfig configures voice activity detection.
// Uses punctuation-based turn detection with semantic confirmation.
type LiveVADConfig struct {
	// Model is the LLM model for semantic turn completion checks.
	Model string

	// PunctuationTrigger defines characters that trigger semantic check.
	// When transcript ends with one of these, semantic check runs immediately.
	// Default: ".!?"
	PunctuationTrigger string

	// NoActivityTimeoutMs is how long to wait before forcing semantic check.
	// Fallback for when user doesn't use punctuation.
	// Default: 3000
	NoActivityTimeoutMs int

	// SemanticCheck enables LLM-based turn completion analysis.
	// Default: true
	SemanticCheck bool

	// MinWordsForCheck is minimum words before semantic check.
	// Default: 1
	MinWordsForCheck int

	// EnergyThreshold is the RMS energy below which audio is silence.
	// Used for STT silence filtering and interrupt detection.
	// Range: 0.0-1.0. Default: 0.02
	EnergyThreshold float64
}

// LiveGracePeriodConfig configures the post-VAD continuation window.
type LiveGracePeriodConfig struct {
	// Enabled turns the grace period on or off.
	// Default: true
	Enabled bool

	// DurationMs is how long to wait for user continuation.
	// Default: 5000
	DurationMs int
}

// LiveInterruptConfig configures interrupt detection during bot speech.
type LiveInterruptConfig struct {
	// Mode controls interrupt detection: "auto", "always", "never"
	Mode string

	// EnergyThreshold is the RMS energy to trigger detection.
	// Default: 0.05
	EnergyThreshold float64

	// CaptureDurationMs is capture window before analysis.
	// Default: 600
	CaptureDurationMs int

	// SemanticCheck enables interrupt vs backchannel detection.
	// Default: true
	SemanticCheck bool

	// SemanticModel is the LLM for interrupt checks.
	SemanticModel string

	// SavePartial controls partial message handling: "none", "marked", "full"
	SavePartial string
}

// LiveEvent is the interface for all live session events.
type LiveEvent interface {
	liveEvent()
}

// LiveSessionCreatedEvent is emitted when session is created.
type LiveSessionCreatedEvent struct {
	SessionID  string
	SampleRate int
	Channels   int
}

func (*LiveSessionCreatedEvent) liveEvent()                  {}
func (e LiveSessionCreatedEvent) runStreamEventType() string { return "live_session_created" }

// LiveStateChangedEvent is emitted when session state changes.
type LiveStateChangedEvent struct {
	From string
	To   string
}

func (*LiveStateChangedEvent) liveEvent()                  {}
func (e LiveStateChangedEvent) runStreamEventType() string { return "live_state_changed" }

// LiveTranscriptDeltaEvent is emitted for real-time transcription.
type LiveTranscriptDeltaEvent struct {
	Delta   string
	IsFinal bool
}

func (*LiveTranscriptDeltaEvent) liveEvent()                  {}
func (e LiveTranscriptDeltaEvent) runStreamEventType() string { return "live_transcript_delta" }

// LiveVADCommittedEvent is emitted when VAD commits a turn.
type LiveVADCommittedEvent struct {
	Transcript string
	Forced     bool
}

func (*LiveVADCommittedEvent) liveEvent()                  {}
func (e LiveVADCommittedEvent) runStreamEventType() string { return "live_vad_committed" }

// LiveInputCommittedEvent is emitted when input is sent to LLM.
type LiveInputCommittedEvent struct {
	Transcript string
}

func (*LiveInputCommittedEvent) liveEvent()                  {}
func (e LiveInputCommittedEvent) runStreamEventType() string { return "live_input_committed" }

// LiveDiscreteInputEvent is emitted when a discrete text/content input is received.
// Discrete inputs bypass VAD and grace period - they are submitted as complete user turns.
type LiveDiscreteInputEvent struct {
	Content []types.ContentBlock
}

func (*LiveDiscreteInputEvent) liveEvent()                  {}
func (e LiveDiscreteInputEvent) runStreamEventType() string { return "live_discrete_input" }

// LiveResponseStartEvent is emitted when LLM starts responding.
type LiveResponseStartEvent struct{}

func (*LiveResponseStartEvent) liveEvent()                  {}
func (e LiveResponseStartEvent) runStreamEventType() string { return "live_response_start" }

// LiveResponseDeltaEvent is emitted for streaming LLM text.
type LiveResponseDeltaEvent struct {
	Delta string
}

func (*LiveResponseDeltaEvent) liveEvent()                  {}
func (e LiveResponseDeltaEvent) runStreamEventType() string { return "live_response_delta" }

// LiveResponseDoneEvent is emitted when LLM response is complete.
type LiveResponseDoneEvent struct{}

func (*LiveResponseDoneEvent) liveEvent()                  {}
func (e LiveResponseDoneEvent) runStreamEventType() string { return "live_response_done" }

// LiveAudioDeltaEvent is emitted for TTS audio chunks.
type LiveAudioDeltaEvent struct {
	Data   []byte
	Format string
}

func (*LiveAudioDeltaEvent) liveEvent()                  {}
func (e LiveAudioDeltaEvent) runStreamEventType() string { return "live_audio_delta" }

// LiveAudioDoneEvent is emitted when TTS is complete.
type LiveAudioDoneEvent struct {
	DurationMs int
}

func (*LiveAudioDoneEvent) liveEvent()                  {}
func (e LiveAudioDoneEvent) runStreamEventType() string { return "live_audio_done" }

// LiveAudioFlushEvent signals that all pending/buffered audio should be discarded.
// This is emitted when the user speaks during grace period (extending their turn)
// or when a real interrupt is confirmed. Clients should clear their audio buffers.
type LiveAudioFlushEvent struct{}

func (*LiveAudioFlushEvent) liveEvent()                  {}
func (e LiveAudioFlushEvent) runStreamEventType() string { return "live_audio_flush" }

// LiveGracePeriodStartedEvent is emitted when grace period starts.
type LiveGracePeriodStartedEvent struct {
	Transcript string
	DurationMs int
	ExpiresAt  time.Time
}

func (*LiveGracePeriodStartedEvent) liveEvent()                  {}
func (e LiveGracePeriodStartedEvent) runStreamEventType() string { return "live_grace_period_started" }

// LiveGracePeriodExtendedEvent is emitted when user speaks during grace.
type LiveGracePeriodExtendedEvent struct {
	PreviousTranscript string
	NewTranscript      string
}

func (*LiveGracePeriodExtendedEvent) liveEvent()                  {}
func (e LiveGracePeriodExtendedEvent) runStreamEventType() string { return "live_grace_period_extended" }

// LiveGracePeriodExpiredEvent is emitted when grace period expires.
type LiveGracePeriodExpiredEvent struct {
	Transcript string
}

func (*LiveGracePeriodExpiredEvent) liveEvent()                  {}
func (e LiveGracePeriodExpiredEvent) runStreamEventType() string { return "live_grace_period_expired" }

// LiveInterruptDetectingEvent is emitted when detecting potential interrupt.
type LiveInterruptDetectingEvent struct{}

func (*LiveInterruptDetectingEvent) liveEvent()                  {}
func (e LiveInterruptDetectingEvent) runStreamEventType() string { return "live_interrupt_detecting" }

// LiveInterruptCapturedEvent is emitted when interrupt audio captured.
type LiveInterruptCapturedEvent struct {
	Transcript string
}

func (*LiveInterruptCapturedEvent) liveEvent()                  {}
func (e LiveInterruptCapturedEvent) runStreamEventType() string { return "live_interrupt_captured" }

// LiveInterruptDismissedEvent is emitted when interrupt is dismissed.
type LiveInterruptDismissedEvent struct {
	Transcript string
	Reason     string
}

func (*LiveInterruptDismissedEvent) liveEvent()                  {}
func (e LiveInterruptDismissedEvent) runStreamEventType() string { return "live_interrupt_dismissed" }

// LiveResponseInterruptedEvent is emitted when response is interrupted.
type LiveResponseInterruptedEvent struct {
	PartialText         string
	InterruptTranscript string
	AudioPositionMs     int
}

func (*LiveResponseInterruptedEvent) liveEvent()                  {}
func (e LiveResponseInterruptedEvent) runStreamEventType() string { return "live_response_interrupted" }

// LiveErrorEvent is emitted on errors.
type LiveErrorEvent struct {
	Code    string
	Message string
}

func (*LiveErrorEvent) liveEvent()                  {}
func (e LiveErrorEvent) runStreamEventType() string { return "live_error" }

// LiveClosedEvent is emitted when session closes.
type LiveClosedEvent struct {
	Reason string
}

func (*LiveClosedEvent) liveEvent()                  {}
func (e LiveClosedEvent) runStreamEventType() string { return "live_closed" }

// LiveDebugEvent is emitted for debug information.
type LiveDebugEvent struct {
	Category string
	Message  string
}

func (*LiveDebugEvent) liveEvent()                  {}
func (e LiveDebugEvent) runStreamEventType() string { return "live_debug" }

// newLiveSession creates a new live session from the SDK config.
func newLiveSession(
	config LiveConfig,
	llmClient live.LLMClient,
	ttsClient live.TTSClient,
	sttClient live.STTClient,
) *LiveSession {
	// Convert SDK config to core config
	coreConfig := live.SessionConfig{
		Model:      config.Model,
		System:     config.System,
		Tools:      config.Tools,
		Messages:   config.Messages,
		Voice:      config.Voice,
		SampleRate: config.SampleRate,
		Channels:   config.Channels,
		MaxTokens:  config.MaxTokens,
	}

	if config.Temperature != nil {
		coreConfig.Temperature = config.Temperature
	}

	// Apply VAD config
	if config.VAD != nil {
		coreConfig.VAD = live.VADConfig{
			Model:               config.VAD.Model,
			PunctuationTrigger:  config.VAD.PunctuationTrigger,
			NoActivityTimeoutMs: config.VAD.NoActivityTimeoutMs,
			SemanticCheck:       config.VAD.SemanticCheck,
			MinWordsForCheck:    config.VAD.MinWordsForCheck,
			EnergyThreshold:     config.VAD.EnergyThreshold,
		}
	} else {
		coreConfig.VAD = live.DefaultVADConfig()
	}

	// Apply grace period config
	if config.GracePeriod != nil {
		coreConfig.GracePeriod = live.GracePeriodConfig{
			Enabled:    config.GracePeriod.Enabled,
			DurationMs: config.GracePeriod.DurationMs,
		}
	} else {
		coreConfig.GracePeriod = live.DefaultGracePeriodConfig()
	}

	// Apply interrupt config
	if config.Interrupt != nil {
		mode := live.InterruptModeAuto
		switch config.Interrupt.Mode {
		case "always":
			mode = live.InterruptModeAlways
		case "never":
			mode = live.InterruptModeNever
		}

		savePartial := live.PartialSaveMarked
		switch config.Interrupt.SavePartial {
		case "none":
			savePartial = live.PartialSaveNone
		case "full":
			savePartial = live.PartialSaveFull
		}

		coreConfig.Interrupt = live.InterruptConfig{
			Mode:              mode,
			EnergyThreshold:   config.Interrupt.EnergyThreshold,
			CaptureDurationMs: config.Interrupt.CaptureDurationMs,
			SemanticCheck:     config.Interrupt.SemanticCheck,
			SemanticModel:     config.Interrupt.SemanticModel,
			SavePartial:       savePartial,
		}
	} else {
		coreConfig.Interrupt = live.DefaultInterruptConfig()
	}

	// Apply defaults for sample rate and channels
	if coreConfig.SampleRate == 0 {
		coreConfig.SampleRate = 24000
	}
	if coreConfig.Channels == 0 {
		coreConfig.Channels = 1
	}

	// Create core session
	session := live.NewSession(coreConfig, llmClient, ttsClient, sttClient)
	if config.Debug {
		session.EnableDebug()
	}

	// Create audio output with buffering
	audioOutputConfig := DefaultAudioOutputConfig()
	if config.AudioOutput != nil {
		audioOutputConfig = *config.AudioOutput
	}

	ls := &LiveSession{
		session:     session,
		events:      make(chan LiveEvent, 100),
		done:        make(chan struct{}),
		audioOutput: NewAudioOutput(coreConfig.SampleRate, audioOutputConfig),
	}

	return ls
}

// Start begins the live session.
func (ls *LiveSession) Start(ctx context.Context) error {
	if err := ls.session.Start(ctx); err != nil {
		return fmt.Errorf("start session: %w", err)
	}

	// Start event translation goroutine
	go ls.translateEvents()

	return nil
}

// SendAudio sends audio data to the session for processing.
// Audio should be 16-bit PCM, mono, at the configured sample rate.
func (ls *LiveSession) SendAudio(data []byte) error {
	return ls.session.SendAudio(data)
}

// Commit forces the VAD to commit the current turn.
// Useful for push-to-talk style interaction.
func (ls *LiveSession) Commit() error {
	return ls.session.Commit()
}

// Interrupt forces an interrupt of the current response.
func (ls *LiveSession) Interrupt(transcript string) error {
	return ls.session.Interrupt(transcript)
}

// SendText sends a discrete text message as a complete user turn.
// This bypasses VAD and grace period - the text is processed as a complete turn.
// If the session is speaking or processing, it waits for the response to complete.
func (ls *LiveSession) SendText(text string) error {
	return ls.session.SendText(text)
}

// SendContent sends discrete content blocks (text, image, video) as a complete user turn.
// This bypasses VAD and grace period - the content is processed as a complete turn.
// If the session is speaking or processing, it waits for the response to complete.
func (ls *LiveSession) SendContent(content []types.ContentBlock) error {
	return ls.session.SendContent(content)
}

// Events returns a channel for receiving session events.
// Note: If you're using AudioOutput(), you don't need to handle
// LiveAudioDeltaEvent and LiveAudioFlushEvent from this channel.
func (ls *LiveSession) Events() <-chan LiveEvent {
	return ls.events
}

// AudioOutput returns the audio output manager.
// This provides a simpler interface for playing audio with built-in buffering
// and flush handling. Use this instead of manually handling LiveAudioDeltaEvent
// and LiveAudioFlushEvent from Events().
//
// Example:
//
//	audio := session.AudioOutput()
//	for {
//	    select {
//	    case chunk := <-audio.Chunks():
//	        player.Write(chunk)
//	    case <-audio.Flush():
//	        player.Clear()
//	    }
//	}
//
// Or use the convenience method:
//
//	session.AudioOutput().HandleAudio(
//	    func(data []byte) { player.Write(data) },
//	    func() { player.Clear() },
//	)
func (ls *LiveSession) AudioOutput() *AudioOutput {
	return ls.audioOutput
}

// SessionID returns the session identifier.
func (ls *LiveSession) SessionID() string {
	return ls.session.SessionID()
}

// State returns the current session state as a string.
func (ls *LiveSession) State() string {
	return ls.session.State().String()
}

// Close shuts down the session.
func (ls *LiveSession) Close() error {
	if ls.closed {
		return nil
	}
	ls.closed = true

	err := ls.session.Close()
	close(ls.done)
	if ls.audioOutput != nil {
		ls.audioOutput.Close()
	}
	return err
}

// translateEvents converts core events to SDK events.
func (ls *LiveSession) translateEvents() {
	coreEvents := ls.session.Events()

	for {
		select {
		case <-ls.done:
			close(ls.events)
			return
		case event, ok := <-coreEvents:
			if !ok {
				close(ls.events)
				return
			}
			if sdkEvent := ls.convertEvent(event); sdkEvent != nil {
				select {
				case ls.events <- sdkEvent:
				case <-ls.done:
					return
				default:
					// Channel full, drop event
				}
			}
		}
	}
}

// convertEvent converts a core event to an SDK event.
func (ls *LiveSession) convertEvent(event live.Event) LiveEvent {
	switch e := event.(type) {
	case *live.SessionCreatedEvent:
		return &LiveSessionCreatedEvent{
			SessionID:  e.SessionID,
			SampleRate: e.SampleRate,
			Channels:   e.Channels,
		}
	case *live.StateChangedEvent:
		return &LiveStateChangedEvent{
			From: e.From.String(),
			To:   e.To.String(),
		}
	case *live.TranscriptDeltaEvent:
		return &LiveTranscriptDeltaEvent{
			Delta:   e.Delta,
			IsFinal: e.IsFinal,
		}
	case *live.VADCommittedEvent:
		return &LiveVADCommittedEvent{
			Transcript: e.Transcript,
			Forced:     e.Forced,
		}
	case *live.InputCommittedEvent:
		return &LiveInputCommittedEvent{
			Transcript: e.Transcript,
		}
	case *live.DiscreteInputReceivedEvent:
		return &LiveDiscreteInputEvent{
			Content: e.Content,
		}
	case *live.MessageStartEvent:
		return &LiveResponseStartEvent{}
	case *live.ContentBlockDeltaEvent:
		return &LiveResponseDeltaEvent{
			Delta: e.Delta,
		}
	case *live.MessageStopEvent:
		return &LiveResponseDoneEvent{}
	case *live.AudioDeltaEvent:
		// Push to AudioOutput for buffered playback
		if ls.audioOutput != nil {
			ls.audioOutput.pushAudio(e.Data)
		}
		return &LiveAudioDeltaEvent{
			Data:   e.Data,
			Format: e.Format,
		}
	case *live.AudioCommittedEvent:
		return &LiveAudioDoneEvent{
			DurationMs: e.DurationMs,
		}
	case *live.AudioFlushEvent:
		// Flush AudioOutput buffer
		if ls.audioOutput != nil {
			ls.audioOutput.doFlush()
		}
		return &LiveAudioFlushEvent{}
	case *live.GracePeriodStartedEvent:
		return &LiveGracePeriodStartedEvent{
			Transcript: e.Transcript,
			DurationMs: e.DurationMs,
			ExpiresAt:  e.ExpiresAt,
		}
	case *live.GracePeriodExtendedEvent:
		return &LiveGracePeriodExtendedEvent{
			PreviousTranscript: e.PreviousTranscript,
			NewTranscript:      e.NewTranscript,
		}
	case *live.GracePeriodExpiredEvent:
		return &LiveGracePeriodExpiredEvent{
			Transcript: e.Transcript,
		}
	case *live.InterruptDetectingEvent:
		return &LiveInterruptDetectingEvent{}
	case *live.InterruptCapturedEvent:
		return &LiveInterruptCapturedEvent{
			Transcript: e.Transcript,
		}
	case *live.InterruptDismissedEvent:
		return &LiveInterruptDismissedEvent{
			Transcript: e.Transcript,
			Reason:     e.Reason,
		}
	case *live.ResponseInterruptedEvent:
		return &LiveResponseInterruptedEvent{
			PartialText:         e.PartialText,
			InterruptTranscript: e.InterruptTranscript,
			AudioPositionMs:     e.AudioPositionMs,
		}
	case *live.ErrorEvent:
		return &LiveErrorEvent{
			Code:    e.Code,
			Message: e.Message,
		}
	case *live.SessionClosedEvent:
		return &LiveClosedEvent{
			Reason: e.Reason,
		}
	case *live.DebugEvent:
		return &LiveDebugEvent{
			Category: e.Category,
			Message:  e.Message,
		}
	default:
		// Unknown event type, ignore
		return nil
	}
}

// llmClientAdapter wraps the SDK client to implement live.LLMClient.
type llmClientAdapter struct {
	client *Client
}

func (a *llmClientAdapter) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
	return a.client.core.CreateMessage(ctx, req)
}

func (a *llmClientAdapter) StreamMessage(ctx context.Context, req *types.MessageRequest) (live.EventStream, error) {
	coreStream, err := a.client.core.StreamMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	// Wrap core.EventStream to implement live.EventStream
	return &liveEventStreamAdapter{stream: coreStream}, nil
}

// liveEventStreamAdapter adapts core.EventStream to live.EventStream.
type liveEventStreamAdapter struct {
	stream interface {
		Next() (types.StreamEvent, error)
		Close() error
	}
}

func (a *liveEventStreamAdapter) Next() (types.StreamEvent, error) {
	return a.stream.Next()
}

func (a *liveEventStreamAdapter) Close() error {
	return a.stream.Close()
}

// ttsClientAdapter wraps the TTS provider to implement live.TTSClient.
type ttsClientAdapter struct {
	provider tts.Provider
}

func (a *ttsClientAdapter) NewStreamingContext(ctx context.Context, opts tts.StreamingContextOptions) (*tts.StreamingContext, error) {
	return a.provider.NewStreamingContext(ctx, opts)
}

// sttClientAdapter wraps the STT provider to implement live.STTClient.
type sttClientAdapter struct {
	provider stt.Provider
}

func (a *sttClientAdapter) NewStreamingSTT(ctx context.Context, opts stt.TranscribeOptions) (*stt.StreamingSTT, error) {
	return a.provider.NewStreamingSTT(ctx, opts)
}
