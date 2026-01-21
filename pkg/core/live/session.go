package live

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vango-go/vai/pkg/core/types"
	"github.com/vango-go/vai/pkg/core/voice/stt"
	"github.com/vango-go/vai/pkg/core/voice/tts"
)

// LLMClient is the interface for making LLM requests.
type LLMClient interface {
	// CreateMessage sends a non-streaming message request.
	CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error)

	// StreamMessage sends a streaming message request.
	// Returns an EventStream that yields streaming events.
	StreamMessage(ctx context.Context, req *types.MessageRequest) (EventStream, error)
}

// EventStream is an iterator over streaming events from LLM.
type EventStream interface {
	// Next returns the next event. Returns nil, io.EOF when done.
	Next() (types.StreamEvent, error)

	// Close releases resources.
	Close() error
}

// TTSClient is the interface for text-to-speech synthesis.
type TTSClient interface {
	// NewStreamingContext creates a new streaming TTS context.
	NewStreamingContext(ctx context.Context, opts tts.StreamingContextOptions) (*tts.StreamingContext, error)
}

// STTClient is the interface for speech-to-text transcription.
type STTClient interface {
	// NewStreamingSTT creates a new streaming STT session.
	NewStreamingSTT(ctx context.Context, opts stt.TranscribeOptions) (*stt.StreamingSTT, error)
}

// Session is the main orchestrator for a live voice conversation.
// It coordinates STT, VAD, grace period, LLM, TTS, and interrupt detection.
type Session struct {
	config      SessionConfig
	audioConfig AudioConfig

	// Clients
	llmClient LLMClient
	ttsClient TTSClient
	sttClient STTClient

	// Components
	vad         *HybridVAD
	gracePeriod *GracePeriodManager
	interrupt   *InterruptDetector

	// State
	mu                sync.RWMutex
	state             SessionState
	sessionID         string
	messages          []types.Message
	currentTranscript string
	partialResponse   string

	// STT session
	sttSession *stt.StreamingSTT
	sttMu      sync.Mutex

	// TTS context
	ttsContext  *tts.StreamingContext
	ttsMu       sync.Mutex
	ttsPaused   bool
	ttsPosition int

	// Channels
	events chan Event
	audio  chan []byte
	done   chan struct{}
	closed atomic.Bool

	// Context for cancellation
	ctx         context.Context
	cancel      context.CancelFunc
	agentCancel context.CancelFunc

	// Debug logging
	debugEnabled bool
}

// NewSession creates a new live session.
func NewSession(
	config SessionConfig,
	llmClient LLMClient,
	ttsClient TTSClient,
	sttClient STTClient,
) *Session {
	audioConfig := AudioConfig{
		SampleRate:    config.SampleRate,
		Channels:      config.Channels,
		BitsPerSample: 16,
	}
	if audioConfig.SampleRate == 0 {
		audioConfig.SampleRate = 24000
	}
	if audioConfig.Channels == 0 {
		audioConfig.Channels = 1
	}

	s := &Session{
		config:      config,
		audioConfig: audioConfig,
		llmClient:   llmClient,
		ttsClient:   ttsClient,
		sttClient:   sttClient,
		state:       StateConfiguring,
		sessionID:   generateSessionID(),
		messages:    make([]types.Message, 0),
		events:      make(chan Event, 100),
		audio:       make(chan []byte, 100),
		done:        make(chan struct{}),
	}

	// Copy initial messages if provided
	if len(config.Messages) > 0 {
		s.messages = append(s.messages, config.Messages...)
	}

	return s
}

// EnableDebug enables debug event emission.
func (s *Session) EnableDebug() {
	s.debugEnabled = true
}

// SessionID returns the session identifier.
func (s *Session) SessionID() string {
	return s.sessionID
}

// State returns the current session state.
func (s *Session) State() SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// Events returns the channel for receiving session events.
func (s *Session) Events() <-chan Event {
	return s.events
}

// Start begins the live session.
func (s *Session) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.state != StateConfiguring {
		s.mu.Unlock()
		return fmt.Errorf("session already started")
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	// Initialize components
	if err := s.initComponents(); err != nil {
		return fmt.Errorf("init components: %w", err)
	}

	// Start STT session
	if err := s.startSTT(); err != nil {
		return fmt.Errorf("start STT: %w", err)
	}

	// Start processing loops
	go s.audioLoop()
	go s.sttLoop()

	// Transition to listening state
	s.setState(StateListening)

	// Emit session created event
	s.emit(&SessionCreatedEvent{
		SessionID:  s.sessionID,
		Config:     &s.config,
		SampleRate: s.audioConfig.SampleRate,
		Channels:   s.audioConfig.Channels,
	})

	return nil
}

// initComponents initializes VAD, grace period, and interrupt detector.
func (s *Session) initComponents() error {
	// Create semantic checker for VAD
	vadChecker := NewDefaultSemanticChecker(func(ctx context.Context, transcript string) (bool, error) {
		return s.checkTurnComplete(ctx, transcript)
	})

	// Create VAD
	s.vad = NewHybridVAD(s.config.VAD, s.audioConfig, vadChecker)
	s.vad.SetCallbacks(
		nil, // onSilence - not used in punctuation-based VAD
		func(transcript string) { s.emit(&VADAnalyzingEvent{Transcript: transcript}) },
		func(transcript string, forced bool) { s.onVADCommit(transcript, forced) },
		func(category, message string) { s.debug(category, message) },
	)
	// Start VAD timeout checker goroutine
	s.vad.Start(s.ctx)

	// Create grace period manager
	s.gracePeriod = NewGracePeriodManager(s.config.GracePeriod)
	s.gracePeriod.SetCallbacks(
		func(transcript string) { s.onGracePeriodExpired(transcript) },
		func(combined string) { s.onGracePeriodContinuation(combined) },
		func(category, message string) { s.debug(category, message) },
	)

	// Create interrupt checker
	interruptChecker := NewDefaultInterruptChecker(func(ctx context.Context, transcript string) (bool, error) {
		return s.checkInterrupt(ctx, transcript)
	})

	// Create interrupt detector
	s.interrupt = NewInterruptDetector(s.config.Interrupt, s.audioConfig, interruptChecker)
	s.interrupt.SetCallbacks(
		func() { s.emit(&InterruptDetectingEvent{}) },
		func(transcript string) { s.emit(&InterruptCapturedEvent{Transcript: transcript}) },
		func(transcript, reason string) { s.onInterruptDismissed(transcript, reason) },
		nil, // Removed: onConfirmed - logic moved to handleInterruptResult
		func(category, message string) { s.debug(category, message) },
	)

	return nil
}

// startSTT starts the STT streaming session.
func (s *Session) startSTT() error {
	s.sttMu.Lock()
	defer s.sttMu.Unlock()

	opts := stt.TranscribeOptions{
		Model:      "ink-whisper",
		Language:   "en",
		Format:     "pcm_s16le",
		SampleRate: s.audioConfig.SampleRate,
	}

	if s.config.Voice != nil && s.config.Voice.Input != nil {
		if s.config.Voice.Input.Language != "" {
			opts.Language = s.config.Voice.Input.Language
		}
		if s.config.Voice.Input.Model != "" {
			opts.Model = s.config.Voice.Input.Model
		}
	}

	session, err := s.sttClient.NewStreamingSTT(s.ctx, opts)
	if err != nil {
		return err
	}

	s.sttSession = session
	return nil
}

// SendAudio sends audio data to the session for processing.
func (s *Session) SendAudio(data []byte) error {
	if s.closed.Load() {
		return fmt.Errorf("session closed")
	}

	select {
	case s.audio <- data:
		return nil
	case <-s.done:
		return fmt.Errorf("session closed")
	default:
		// Buffer full, drop audio
		s.debug("AUDIO", "Buffer full, dropping audio chunk")
		return nil
	}
}

// Commit forces the VAD to commit the current turn.
// Useful for push-to-talk style interaction.
func (s *Session) Commit() error {
	s.mu.Lock()
	state := s.state
	s.mu.Unlock()

	if state != StateListening {
		return fmt.Errorf("not in listening state")
	}

	transcript := s.vad.GetTranscript()
	if transcript == "" {
		return fmt.Errorf("no transcript to commit")
	}

	s.onVADCommit(transcript, false)
	return nil
}

// Interrupt forces an interrupt of the current response.
func (s *Session) Interrupt(transcript string) error {
	s.mu.Lock()
	state := s.state
	partial := s.partialResponse
	s.mu.Unlock()

	if state != StateSpeaking && state != StateProcessing {
		return fmt.Errorf("nothing to interrupt")
	}

	// Save partial assistant response to conversation history
	s.mu.Lock()
	if partial != "" {
		s.messages = append(s.messages, types.Message{
			Role:    "assistant",
			Content: partial,
		})
	}
	s.mu.Unlock()

	// Emit interrupt event
	s.emit(&ResponseInterruptedEvent{
		PartialText:         partial,
		InterruptTranscript: transcript,
		AudioPositionMs:     s.ttsPosition,
	})

	// Cancel agent/TTS and flush audio
	s.cancelAgent()
	s.cancelTTS()
	s.emit(&AudioFlushEvent{})
	s.setState(StateListening)

	// Process interrupt through normal VAD commit flow
	s.vad.SetTranscript(transcript)
	s.onVADCommit(transcript, false)

	return nil
}

// SendText sends a discrete text message as a complete user turn.
// This bypasses VAD and grace period - the text is processed as a complete turn.
// If the session is speaking or processing, it waits for the response to complete.
func (s *Session) SendText(text string) error {
	if s.closed.Load() {
		return fmt.Errorf("session closed")
	}

	content := []types.ContentBlock{
		types.TextBlock{Type: "text", Text: text},
	}
	return s.processDiscreteInput(content)
}

// SendContent sends discrete content blocks (text, image, video) as a complete user turn.
// This bypasses VAD and grace period - the content is processed as a complete turn.
// If the session is speaking or processing, it waits for the response to complete.
func (s *Session) SendContent(content []types.ContentBlock) error {
	if s.closed.Load() {
		return fmt.Errorf("session closed")
	}
	if len(content) == 0 {
		return fmt.Errorf("content cannot be empty")
	}
	return s.processDiscreteInput(content)
}

// processDiscreteInput handles a discrete input (text/image/video).
// It waits for any in-flight response to complete, then processes the input.
func (s *Session) processDiscreteInput(content []types.ContentBlock) error {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()

	s.debug("INPUT", fmt.Sprintf("Discrete input received: %d blocks (state: %s)", len(content), state))

	// Emit event
	s.emit(&DiscreteInputReceivedEvent{Content: content})

	switch state {
	case StateListening:
		// Process immediately
		s.processDiscreteInputNow(content)

	case StateGracePeriod:
		// Cancel grace period (not an interrupt) and process
		s.gracePeriod.Cancel()
		s.cancelAgent()
		s.cancelTTS()
		s.emit(&AudioFlushEvent{})
		s.processDiscreteInputNow(content)

	case StateProcessing, StateSpeaking:
		// Wait for current response to complete, then process
		go func() {
			s.waitForResponseComplete()
			s.processDiscreteInputNow(content)
		}()

	case StateInterruptCapturing:
		// Wait for interrupt resolution, then process
		go func() {
			s.waitForResponseComplete()
			s.processDiscreteInputNow(content)
		}()

	default:
		return fmt.Errorf("cannot send input in state: %s", state)
	}

	return nil
}

// processDiscreteInputNow immediately processes a discrete input as a user turn.
func (s *Session) processDiscreteInputNow(content []types.ContentBlock) {
	s.debug("INPUT", "Processing discrete input")

	// Reset VAD to discard any in-progress audio transcript
	s.vad.Reset()

	// Add to conversation and trigger agent
	s.mu.Lock()
	s.messages = append(s.messages, types.Message{
		Role:    "user",
		Content: content,
	})
	messages := make([]types.Message, len(s.messages))
	copy(messages, s.messages)
	s.mu.Unlock()

	// Create agent context
	agentCtx, agentCancel := context.WithCancel(s.ctx)
	s.agentCancel = agentCancel

	s.setState(StateProcessing)
	s.emit(&InputCommittedEvent{
		Transcript: contentToString(content),
	})

	go s.runAgent(agentCtx, messages)
}

// waitForResponseComplete blocks until current response finishes.
func (s *Session) waitForResponseComplete() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.RLock()
			state := s.state
			s.mu.RUnlock()

			if state == StateListening {
				return
			}
		}
	}
}

// contentToString extracts text representation from content blocks for logging/events.
func contentToString(content []types.ContentBlock) string {
	var parts []string
	for _, block := range content {
		switch b := block.(type) {
		case types.TextBlock:
			parts = append(parts, b.Text)
		case types.ImageBlock:
			parts = append(parts, "[image]")
		case types.VideoBlock:
			parts = append(parts, "[video]")
		case types.AudioBlock:
			parts = append(parts, "[audio]")
		case types.DocumentBlock:
			parts = append(parts, "[document]")
		default:
			parts = append(parts, "[content]")
		}
	}
	return strings.Join(parts, " ")
}

// Close shuts down the session.
func (s *Session) Close() error {
	if s.closed.Swap(true) {
		return nil // Already closed
	}

	s.debug("SESSION", "Closing session")

	// Cancel context
	if s.cancel != nil {
		s.cancel()
	}

	// Stop VAD timeout checker
	if s.vad != nil {
		s.vad.Stop()
	}

	// Cancel agent if running
	if s.agentCancel != nil {
		s.agentCancel()
	}

	// Close STT session
	s.sttMu.Lock()
	if s.sttSession != nil {
		s.sttSession.Close()
	}
	s.sttMu.Unlock()

	// Close TTS context
	s.ttsMu.Lock()
	if s.ttsContext != nil {
		s.ttsContext.Close()
	}
	s.ttsMu.Unlock()

	// Close done channel
	close(s.done)

	// Set state
	s.setState(StateClosed)

	// Emit close event
	s.emit(&SessionClosedEvent{Reason: "closed"})

	// Close events channel
	close(s.events)

	return nil
}

// audioLoop processes incoming audio chunks.
func (s *Session) audioLoop() {
	for {
		select {
		case <-s.done:
			return
		case <-s.ctx.Done():
			return
		case data := <-s.audio:
			s.processAudio(data)
		}
	}
}

// processAudio handles a single audio chunk based on current state.
func (s *Session) processAudio(data []byte) {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()

	switch state {
	case StateListening:
		// Send audio to STT for transcription
		// Note: Cartesia's min_volume setting handles silence filtering server-side
		s.sttMu.Lock()
		if s.sttSession != nil {
			s.sttSession.SendAudio(data)
		}
		s.sttMu.Unlock()

		// Note: VAD turn detection is now handled via punctuation triggers in processTranscriptDelta
		// No need to call vad.ProcessAudio - the timeout checker runs in background

	case StateProcessing:
		// During processing (e.g., waiting for server-side tool execution like web_search),
		// we should still detect user speech for potential interrupts.
		// This is important because the LLM might call a tool before emitting any text,
		// keeping the state in StateProcessing for several seconds.
		energy := CalculateRMSEnergy(data)
		if energy > s.config.Interrupt.EnergyThreshold {
			s.debug("INTERRUPT", "Speech detected during PROCESSING state")
			// Cancel the agent immediately - user wants to interrupt before response starts
			s.cancelAgent()
			s.emit(&AudioFlushEvent{})
			s.setState(StateListening)

			// Send audio to STT to capture what the user said
			s.sttMu.Lock()
			if s.sttSession != nil {
				s.sttSession.SendAudio(data)
			}
			s.sttMu.Unlock()
		}

	case StateGracePeriod:
		// Detect user speech immediately via energy - cancel agent before it generates more
		energy := CalculateRMSEnergy(data)
		if energy > s.config.VAD.EnergyThreshold {
			// Check if we haven't already cancelled the agent
			if s.agentCancel != nil {
				s.debug("GRACE", "User speech detected (energy) during GRACE_PERIOD, cancelling agent")
				s.cancelAgent()
				// Also cancel TTS if it somehow started
				s.cancelTTS()
				s.emit(&AudioFlushEvent{})
			}
		}

		// Send audio to STT during grace period to capture user continuation
		s.sttMu.Lock()
		if s.sttSession != nil {
			s.sttSession.SendAudio(data)
		}
		s.sttMu.Unlock()

	case StateSpeaking:
		// Check if grace period is still active - if so, user speech is continuation, not interrupt
		if s.gracePeriod.IsActive() {
			// Detect user speech immediately via energy - don't wait for STT
			energy := CalculateRMSEnergy(data)

			// Check if TTS is still running (not already cancelled)
			s.ttsMu.Lock()
			ttsRunning := s.ttsContext != nil
			s.ttsMu.Unlock()

			if energy > s.config.VAD.EnergyThreshold && ttsRunning {
				// User is speaking! Cancel TTS immediately, don't wait for transcript
				s.debug("GRACE", "User speech detected (energy), cancelling TTS immediately")
				s.cancelAgent()
				s.cancelTTS()
				s.emit(&AudioFlushEvent{})
			}

			// Send audio to STT to capture the transcript
			s.sttMu.Lock()
			if s.sttSession != nil {
				s.sttSession.SendAudio(data)
			}
			s.sttMu.Unlock()
			return
		}

		// Grace period expired - check for potential interrupt using energy detection
		energy := CalculateRMSEnergy(data)
		if energy > s.config.Interrupt.EnergyThreshold {
			s.handlePotentialInterrupt(data)
		}

		// Always send audio to STT during speaking, even if below interrupt threshold.
		// This keeps STT "warm" and provides context so it can transcribe quickly
		// when an interrupt is detected.
		s.sttMu.Lock()
		if s.sttSession != nil {
			s.sttSession.SendAudio(data)
		}
		s.sttMu.Unlock()

	case StateInterruptCapturing:
		// Add to interrupt buffer
		s.interrupt.AddAudio(data)
		// Also send to STT for transcription
		s.sttMu.Lock()
		if s.sttSession != nil {
			s.sttSession.SendAudio(data)
		}
		s.sttMu.Unlock()

		// Check if capture is complete
		if s.interrupt.CaptureComplete() {
			result := s.interrupt.Analyze(s.ctx)
			s.handleInterruptResult(result)
		}
	}
}

// sttLoop processes transcription events from STT.
func (s *Session) sttLoop() {
	for {
		select {
		case <-s.done:
			return
		case <-s.ctx.Done():
			return
		default:
		}

		s.sttMu.Lock()
		session := s.sttSession
		s.sttMu.Unlock()

		if session == nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		select {
		case <-s.done:
			return
		case <-s.ctx.Done():
			return
		case delta, ok := <-session.Transcripts():
			if !ok {
				// STT session closed, try to restart
				s.debug("STT", "Session closed, attempting restart")
				if err := s.startSTT(); err != nil {
					s.debug("STT", "Failed to restart: "+err.Error())
				}
				continue
			}
			s.processTranscriptDelta(delta)
		}
	}
}

// processTranscriptDelta handles incoming transcription.
func (s *Session) processTranscriptDelta(delta stt.TranscriptDelta) {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()

	// Emit transcript delta event
	s.emit(&TranscriptDeltaEvent{
		Delta:   delta.Text,
		IsFinal: delta.IsFinal,
	})

	s.debug("STT", fmt.Sprintf("Transcribed: %q (final: %v)", delta.Text, delta.IsFinal))

	switch state {
	case StateListening:
		// Check if grace period is still active (e.g., TTS finished but grace window still open)
		// If so, this is a continuation, not a new turn
		if s.gracePeriod.IsActive() && delta.Text != "" {
			s.gracePeriod.HandleUserSpeech(delta.Text)
			return
		}
		// Process through VAD
		// Note: We don't have the raw audio here, so we pass empty chunk
		// The VAD should get energy from the actual audio in processAudio
		s.vad.AddTranscript(delta.Text)

	case StateGracePeriod:
		// User is continuing during grace period
		if delta.Text != "" {
			s.gracePeriod.HandleUserSpeech(delta.Text)
		}

	case StateSpeaking:
		// If grace period is still active, user speech is continuation
		if s.gracePeriod.IsActive() && delta.Text != "" {
			s.gracePeriod.HandleUserSpeech(delta.Text)
		}
		// Otherwise, transcript deltas during speaking are handled by interrupt flow

	case StateInterruptCapturing:
		// Add to interrupt transcript
		s.interrupt.AddTranscript(delta.Text)
	}
}

// handlePotentialInterrupt starts the interrupt capture flow.
func (s *Session) handlePotentialInterrupt(audioData []byte) {
	s.debug("INTERRUPT", "Audio detected during bot speech, pausing TTS")

	// Pause TTS and flush audio immediately so playback stops
	// This provides responsive interrupt detection - user shouldn't hear
	// audio continue while we're analyzing their speech
	s.pauseTTS()
	s.emit(&AudioFlushEvent{})

	// Start capture
	s.interrupt.StartCapture()
	s.interrupt.AddAudio(audioData)

	// Send audio to STT immediately so transcription can start
	// This is important because STT needs audio context to transcribe
	s.sttMu.Lock()
	if s.sttSession != nil {
		s.sttSession.SendAudio(audioData)
	}
	s.sttMu.Unlock()

	// Transition to capturing state
	s.setState(StateInterruptCapturing)
}

// handleInterruptResult processes the result of interrupt analysis.
func (s *Session) handleInterruptResult(result InterruptResult) {
	switch result {
	case InterruptNone, InterruptBackchannel:
		// Resume TTS
		s.debug("INTERRUPT", "Resuming TTS after "+result.String())
		s.resumeTTS()
		s.setState(StateSpeaking)

	case InterruptReal:
		s.debug("INTERRUPT", "Real interrupt confirmed")

		// Get the interrupt transcript
		transcript := s.interrupt.GetCapturedTranscript()

		// Save partial assistant response to conversation history
		s.mu.Lock()
		partial := s.partialResponse
		if partial != "" {
			s.messages = append(s.messages, types.Message{
				Role:    "assistant",
				Content: partial,
			})
		}
		s.mu.Unlock()

		// Emit interrupt event
		s.emit(&ResponseInterruptedEvent{
			PartialText:         partial,
			InterruptTranscript: transcript,
			AudioPositionMs:     s.ttsPosition,
		})

		// Cancel agent/TTS and flush audio
		s.cancelAgent()
		s.cancelTTS()
		s.emit(&AudioFlushEvent{})
		s.setState(StateListening)

		// Process interrupt through normal VAD commit flow
		// This starts agent processing + grace period for continuation
		s.vad.SetTranscript(transcript)
		s.onVADCommit(transcript, false)
	}
}

// onVADCommit is called when VAD commits a turn.
func (s *Session) onVADCommit(transcript string, forced bool) {
	// Note: Don't debug log here - VADCommittedEvent conveys the same info

	s.emit(&VADCommittedEvent{
		Transcript: transcript,
		Forced:     forced,
	})

	s.mu.Lock()
	s.currentTranscript = transcript
	s.mu.Unlock()

	// Start agent processing immediately - don't wait for grace period to expire
	// The grace period runs in parallel as a cancellation window
	s.startAgentProcessing(transcript)

	// Start grace period if enabled - this is a cancellation window, not a blocker
	// We stay in StateGracePeriod to continue capturing audio in case user continues
	// If user speaks during this window, we cancel the in-flight agent request and flush audio
	if s.config.GracePeriod.Enabled {
		s.setState(StateGracePeriod)
		s.gracePeriod.Start(transcript)
		s.emit(&GracePeriodStartedEvent{
			Transcript: transcript,
			DurationMs: s.config.GracePeriod.DurationMs,
			ExpiresAt:  s.gracePeriod.ExpiresAt(),
		})
	}
}

// onGracePeriodExpired is called when grace period ends.
func (s *Session) onGracePeriodExpired(transcript string) {
	// Note: Don't debug log here - GracePeriodExpiredEvent conveys the same info

	s.emit(&GracePeriodExpiredEvent{Transcript: transcript})

	// Agent is already running (started immediately on VAD commit)
	// Nothing else to do - the grace period was just a cancellation window
}

// onGracePeriodContinuation is called when user speaks during grace period.
func (s *Session) onGracePeriodContinuation(combined string) {
	s.debug("GRACE", "User continued, combined: "+combined)

	// Cancel any pending agent request
	s.cancelAgent()

	// Cancel TTS if it's running
	s.cancelTTS()

	// Signal client to flush audio buffers immediately
	// This ensures any audio already sent to the speaker is discarded
	s.emit(&AudioFlushEvent{})

	s.emit(&GracePeriodExtendedEvent{
		PreviousTranscript: s.currentTranscript,
		NewTranscript:      combined,
		DurationMs:         s.config.GracePeriod.DurationMs,
		ExpiresAt:          time.Now().Add(time.Duration(s.config.GracePeriod.DurationMs) * time.Millisecond),
	})

	// Update transcript and restart VAD/grace
	s.mu.Lock()
	s.currentTranscript = combined
	s.mu.Unlock()

	s.vad.SetTranscript(combined)
	s.setState(StateListening)
}

// onInterruptDismissed is called when interrupt is dismissed.
func (s *Session) onInterruptDismissed(transcript, reason string) {
	s.emit(&InterruptDismissedEvent{
		Transcript: transcript,
		Reason:     reason,
	})
}


// startAgentProcessing begins the agent response generation.
func (s *Session) startAgentProcessing(transcript string) {
	s.setState(StateProcessing)

	s.emit(&InputCommittedEvent{Transcript: transcript})

	// Add user message
	s.mu.Lock()
	s.messages = append(s.messages, types.Message{
		Role:    "user",
		Content: transcript,
	})
	messages := make([]types.Message, len(s.messages))
	copy(messages, s.messages)
	s.mu.Unlock()

	// Create agent context
	agentCtx, agentCancel := context.WithCancel(s.ctx)
	s.agentCancel = agentCancel

	go s.runAgent(agentCtx, messages)
}

// runAgent executes the agent with streaming and pipes to TTS incrementally.
func (s *Session) runAgent(ctx context.Context, messages []types.Message) {
	s.debug("LLM", "Sending to "+s.config.Model+" (streaming)")

	req := &types.MessageRequest{
		Model:     s.config.Model,
		Messages:  messages,
		MaxTokens: s.config.MaxTokens,
	}

	if s.config.System != "" {
		req.System = s.config.System
	}
	if len(s.config.Tools) > 0 {
		req.Tools = s.config.Tools
	}
	if s.config.Temperature != nil {
		req.Temperature = s.config.Temperature
	}

	// Start streaming LLM request
	stream, err := s.llmClient.StreamMessage(ctx, req)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.emit(&ErrorEvent{Code: "llm_error", Message: err.Error()})
		s.setState(StateListening)
		return
	}
	defer stream.Close()

	// Create TTS context upfront for streaming
	ttsCtx, err := s.createTTSContext(ctx)
	if err != nil {
		s.debug("TTS", "Failed to create context: "+err.Error())
		s.emit(&ErrorEvent{Code: "tts_error", Message: err.Error()})
		s.setState(StateListening)
		return
	}

	// Start audio streaming in background
	go s.streamTTSAudio(ctx, ttsCtx)

	// Process LLM stream and pipe to TTS via buffer
	buffer := NewTTSBuffer()
	var fullText strings.Builder
	firstChunk := true

	for {
		event, err := stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				s.debug("LLM", "Stream cancelled")
				buffer.Reset()
				return
			}
			s.debug("LLM", "Stream error: "+err.Error())
			break
		}

		// Handle content block delta events (check both value and pointer types)
		var textDelta types.TextDelta
		var deltaIndex int
		var hasTextDelta bool

		switch e := event.(type) {
		case types.ContentBlockDeltaEvent:
			deltaIndex = e.Index
			if td, ok := e.Delta.(types.TextDelta); ok {
				textDelta = td
				hasTextDelta = true
			}
		case *types.ContentBlockDeltaEvent:
			deltaIndex = e.Index
			if td, ok := e.Delta.(types.TextDelta); ok {
				textDelta = td
				hasTextDelta = true
			}
		}

		if hasTextDelta {
			text := textDelta.Text
			fullText.WriteString(text)

			// Emit delta event
			s.emit(&ContentBlockDeltaEvent{Index: deltaIndex, Delta: text})

			// Log first chunk for latency tracking
			if firstChunk {
				s.debug("LLM", "First token received")
				firstChunk = false
				s.setState(StateSpeaking)
			}

			// Buffer and send to TTS when ready
			if chunk := buffer.Add(text); chunk != "" {
				s.debug("TTS", "Sending chunk: "+chunk)
				if err := ttsCtx.SendText(chunk, false); err != nil {
					s.debug("TTS", "Send error: "+err.Error())
				}
			}
		}
	}

	// Flush remaining text to TTS
	if remaining := buffer.Flush(); remaining != "" {
		s.debug("TTS", "Sending final chunk: "+remaining)
		if err := ttsCtx.SendText(remaining, true); err != nil {
			s.debug("TTS", "Final send error: "+err.Error())
		}
	} else {
		// No remaining text, just flush TTS
		ttsCtx.Flush()
	}

	// Update conversation history
	finalText := fullText.String()
	if finalText != "" {
		s.mu.Lock()
		s.partialResponse = finalText
		s.messages = append(s.messages, types.Message{
			Role:    "assistant",
			Content: finalText,
		})
		s.mu.Unlock()
	}

	s.debug("LLM", "Stream complete")
	s.emit(&MessageStopEvent{})
}

// createTTSContext creates a TTS streaming context.
func (s *Session) createTTSContext(ctx context.Context) (*tts.StreamingContext, error) {
	opts := tts.StreamingContextOptions{
		SampleRate: s.audioConfig.SampleRate,
		Format:     "pcm",
	}
	if s.config.Voice != nil && s.config.Voice.Output != nil {
		opts.Voice = s.config.Voice.Output.Voice
		opts.Speed = s.config.Voice.Output.Speed
		opts.Volume = s.config.Voice.Output.Volume
		if s.config.Voice.Output.Format != "" {
			opts.Format = s.config.Voice.Output.Format
		}
		if s.config.Voice.Output.SampleRate > 0 {
			opts.SampleRate = s.config.Voice.Output.SampleRate
		}
	}

	if opts.Voice == "" {
		s.debug("TTS", "No voice ID configured, using default")
		opts.Voice = "98a34ef2-2140-4c28-9c71-663dc4dd7022"
	}

	s.debug("TTS", fmt.Sprintf("Creating TTS context (voice: %s, rate: %d, format: %s)", opts.Voice, opts.SampleRate, opts.Format))

	s.ttsMu.Lock()
	defer s.ttsMu.Unlock()

	// Close any existing TTS context first to prevent concurrent audio streams
	if s.ttsContext != nil {
		s.debug("TTS", "Closing previous TTS context before creating new one")
		s.ttsContext.Close()
		s.ttsContext = nil
	}

	ttsCtx, err := s.ttsClient.NewStreamingContext(ctx, opts)
	if err != nil {
		return nil, err
	}

	s.ttsContext = ttsCtx
	s.ttsPaused = false
	s.ttsPosition = 0

	return ttsCtx, nil
}

// startTTS begins text-to-speech synthesis.
func (s *Session) startTTS(ctx context.Context, text string) {
	s.setState(StateSpeaking)
	s.debug("TTS", "Synthesizing: "+text)

	// Build TTS options with sensible defaults
	opts := tts.StreamingContextOptions{
		SampleRate: s.audioConfig.SampleRate,
		Format:     "pcm", // Raw PCM for lowest latency
	}
	if s.config.Voice != nil && s.config.Voice.Output != nil {
		opts.Voice = s.config.Voice.Output.Voice
		opts.Speed = s.config.Voice.Output.Speed
		opts.Volume = s.config.Voice.Output.Volume
		if s.config.Voice.Output.Format != "" {
			opts.Format = s.config.Voice.Output.Format
		}
		if s.config.Voice.Output.SampleRate > 0 {
			opts.SampleRate = s.config.Voice.Output.SampleRate
		}
	}

	// Ensure we have a voice ID
	if opts.Voice == "" {
		s.debug("TTS", "No voice ID configured, using default")
		opts.Voice = "98a34ef2-2140-4c28-9c71-663dc4dd7022" // Sonic default
	}

	s.debug("TTS", fmt.Sprintf("Creating TTS context (voice: %s, rate: %d, format: %s)", opts.Voice, opts.SampleRate, opts.Format))

	s.ttsMu.Lock()
	ttsCtx, err := s.ttsClient.NewStreamingContext(ctx, opts)
	if err != nil {
		s.ttsMu.Unlock()
		s.debug("TTS", "Failed to start: "+err.Error())
		s.emit(&ErrorEvent{Code: "tts_error", Message: err.Error()})
		s.setState(StateListening)
		s.emit(&VADListeningEvent{})
		return
	}
	s.ttsContext = ttsCtx
	s.ttsPaused = false
	s.ttsPosition = 0
	s.ttsMu.Unlock()

	// Check if we were cancelled during context creation (e.g., grace period continuation)
	if ctx.Err() != nil {
		s.debug("TTS", "Context cancelled before sending text")
		s.ttsMu.Lock()
		if s.ttsContext == ttsCtx {
			s.ttsContext.Close()
			s.ttsContext = nil
		}
		s.ttsMu.Unlock()
		return
	}

	s.debug("TTS", "TTS context created, sending text...")

	// Send text to TTS
	if err := ttsCtx.SendText(text, true); err != nil {
		s.debug("TTS", "Failed to send text: "+err.Error())
		s.emit(&ErrorEvent{Code: "tts_send_error", Message: err.Error()})
		s.setState(StateListening)
		s.emit(&VADListeningEvent{})
		return
	}

	s.debug("TTS", "Text sent, streaming audio...")

	// Stream audio to client
	go s.streamTTSAudio(ctx, ttsCtx)
}

// streamTTSAudio streams audio from TTS to the client.
func (s *Session) streamTTSAudio(ctx context.Context, ttsCtx *tts.StreamingContext) {
	s.debug("TTS", "Starting audio stream loop...")
	audioChunks := 0

	for {
		select {
		case <-ctx.Done():
			s.debug("TTS", "Context cancelled")
			return
		case <-s.done:
			s.debug("TTS", "Session done")
			return
		case audioData, ok := <-ttsCtx.Audio():
			if !ok {
				// TTS complete - check if this is still the active TTS context
				// If not, another stream has started and we should exit silently
				s.ttsMu.Lock()
				isCurrentContext := s.ttsContext == ttsCtx
				s.ttsMu.Unlock()

				if !isCurrentContext {
					s.debug("TTS", "TTS context replaced, exiting old stream")
					return
				}

				// Check for errors
				if err := ttsCtx.Err(); err != nil {
					s.debug("TTS", "TTS error: "+err.Error())
					s.emit(&ErrorEvent{Code: "tts_stream_error", Message: err.Error()})
				}
				s.debug("TTS", fmt.Sprintf("Synthesis complete (%d chunks, %dms)", audioChunks, s.ttsPosition))
				s.emit(&AudioCommittedEvent{DurationMs: s.ttsPosition})

				s.setState(StateListening)
				s.emit(&VADListeningEvent{})
				s.vad.Reset()
				return
			}

			audioChunks++
			s.debug("TTS", fmt.Sprintf("Audio chunk %d: %d bytes", audioChunks, len(audioData)))

			// Check if paused
			s.ttsMu.Lock()
			paused := s.ttsPaused
			s.ttsMu.Unlock()

			if paused {
				// Buffer audio while paused
				continue
			}

			// Emit audio
			s.emit(&AudioDeltaEvent{
				Data:   audioData,
				Format: "pcm_s16le",
			})

			// Update position
			s.ttsMu.Lock()
			s.ttsPosition += s.audioConfig.DurationMs(len(audioData))
			s.ttsMu.Unlock()
		}
	}
}

// pauseTTS pauses TTS output.
func (s *Session) pauseTTS() {
	s.ttsMu.Lock()
	defer s.ttsMu.Unlock()

	if s.ttsContext != nil && !s.ttsPaused {
		s.ttsPaused = true
		s.emit(&TTSPausedEvent{PositionMs: s.ttsPosition})
	}
}

// resumeTTS resumes TTS output.
func (s *Session) resumeTTS() {
	s.ttsMu.Lock()
	defer s.ttsMu.Unlock()

	if s.ttsContext != nil && s.ttsPaused {
		s.ttsPaused = false
		s.emit(&TTSResumedEvent{PositionMs: s.ttsPosition})
	}
}

// cancelTTS cancels TTS output.
func (s *Session) cancelTTS() {
	s.ttsMu.Lock()
	defer s.ttsMu.Unlock()

	if s.ttsContext != nil {
		s.ttsContext.Close()
		s.ttsContext = nil
		s.emit(&TTSCancelledEvent{PositionMs: s.ttsPosition})
	}
}

// cancelAgent cancels the current agent request.
func (s *Session) cancelAgent() {
	if s.agentCancel != nil {
		s.agentCancel()
		s.agentCancel = nil
	}
}

// checkTurnComplete performs semantic turn completion check.
func (s *Session) checkTurnComplete(ctx context.Context, transcript string) (bool, error) {
	prompt := fmt.Sprintf(TurnCompletePrompt, transcript)

	// Use VAD-specific model or fall back to main model
	model := s.config.VAD.Model
	if model == "" {
		model = s.config.Model
	}

	req := &types.MessageRequest{
		Model: model,
		Messages: []types.Message{
			{Role: "user", Content: prompt},
		},
		MaxTokens: 5,
	}

	resp, err := s.llmClient.CreateMessage(ctx, req)
	if err != nil {
		return false, err
	}

	return ParseTurnCompleteResponse(resp.TextContent()), nil
}

// checkInterrupt performs semantic interrupt check.
func (s *Session) checkInterrupt(ctx context.Context, transcript string) (bool, error) {
	prompt := fmt.Sprintf(InterruptCheckPrompt, transcript)

	// Use interrupt-specific model or fall back to main model
	model := s.config.Interrupt.SemanticModel
	if model == "" {
		model = s.config.Model
	}

	req := &types.MessageRequest{
		Model: model,
		Messages: []types.Message{
			{Role: "user", Content: prompt},
		},
		MaxTokens: 10,
	}

	resp, err := s.llmClient.CreateMessage(ctx, req)
	if err != nil {
		return false, err
	}

	return ParseInterruptCheckResponse(resp.TextContent()), nil
}

// setState updates the session state and emits an event.
func (s *Session) setState(newState SessionState) {
	s.mu.Lock()
	oldState := s.state
	s.state = newState
	s.mu.Unlock()

	if oldState != newState {
		s.debug("SESSION", fmt.Sprintf("State: %s -> %s", oldState, newState))
		s.emit(&StateChangedEvent{From: oldState, To: newState})
	}
}

// emit sends an event to the events channel.
func (s *Session) emit(event Event) {
	select {
	case s.events <- event:
	case <-s.done:
	default:
		// Channel full, drop event
	}
}

// debug logs a debug message if debug mode is enabled.
// Logs are printed to stderr with timestamps for visibility.
func (s *Session) debug(category, message string) {
	if s.debugEnabled {
		// Print to stderr with timestamp for developer visibility
		timestamp := time.Now().Format("15:04:05.000")
		fmt.Fprintf(os.Stderr, "\033[90m%s\033[0m [\033[36m%-10s\033[0m] %s\n", timestamp, category, message)

		// Also emit event for programmatic access
		s.emit(&DebugEvent{Category: category, Message: message})
	}
}

// generateSessionID creates a unique session identifier.
func generateSessionID() string {
	return fmt.Sprintf("live_%d", time.Now().UnixNano())
}
