package live

import (
	"context"
	"strings"
	"sync"
	"time"
)

// InterruptResult indicates the outcome of interrupt analysis.
type InterruptResult int

const (
	// InterruptNone means no interrupt was detected (noise, silence).
	InterruptNone InterruptResult = iota
	// InterruptBackchannel means user made a backchannel response (uh huh, okay).
	InterruptBackchannel
	// InterruptReal means user is genuinely interrupting.
	InterruptReal
)

// String returns a human-readable interrupt result.
func (r InterruptResult) String() string {
	switch r {
	case InterruptNone:
		return "NONE"
	case InterruptBackchannel:
		return "BACKCHANNEL"
	case InterruptReal:
		return "REAL"
	default:
		return "UNKNOWN"
	}
}

// InterruptChecker is an interface for performing semantic interrupt analysis.
type InterruptChecker interface {
	// CheckInterrupt analyzes if the transcript is a real interrupt or backchannel.
	// Returns true if it's a real interrupt that should stop the bot.
	CheckInterrupt(ctx context.Context, transcript string) (bool, error)
}

// InterruptDetector handles detection and classification of user interrupts
// during bot speech.
//
// The flow is:
// 1. Audio detected during bot speech → Immediately pause TTS
// 2. Capture audio for CaptureDurationMs (default 600ms)
// 3. Attempt transcription
// 4. If no transcription → Resume TTS (noise)
// 5. If transcription → Semantic check for interrupt vs backchannel
// 6. If backchannel → Resume TTS
// 7. If real interrupt → Cancel TTS, enter listening mode
type InterruptDetector struct {
	config      InterruptConfig
	audioConfig AudioConfig
	checker     InterruptChecker

	mu            sync.Mutex
	capturing     bool
	captureStart  time.Time
	captureBuffer *AudioBuffer
	transcript    string

	// Callbacks
	onDetecting func()
	onCaptured  func(transcript string)
	onDismissed func(transcript string, reason string)
	onConfirmed func(transcript string)
	onDebug     func(category, message string)
}

// NewInterruptDetector creates a new interrupt detector.
func NewInterruptDetector(config InterruptConfig, audioConfig AudioConfig, checker InterruptChecker) *InterruptDetector {
	return &InterruptDetector{
		config:        config,
		audioConfig:   audioConfig,
		checker:       checker,
		captureBuffer: NewAudioBuffer(audioConfig, config.CaptureDurationMs+100),
	}
}

// SetCallbacks sets the event callbacks.
func (d *InterruptDetector) SetCallbacks(
	onDetecting func(),
	onCaptured func(transcript string),
	onDismissed func(transcript string, reason string),
	onConfirmed func(transcript string),
	onDebug func(category, message string),
) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onDetecting = onDetecting
	d.onCaptured = onCaptured
	d.onDismissed = onDismissed
	d.onConfirmed = onConfirmed
	d.onDebug = onDebug
}

// StartCapture begins the interrupt capture window.
// This should be called when audio is detected during bot speech.
func (d *InterruptDetector) StartCapture() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.capturing {
		return
	}

	d.capturing = true
	d.captureStart = time.Now()
	d.captureBuffer.Clear()
	d.transcript = ""

	d.debug("INTERRUPT", "Audio detected, starting capture window (%dms)", d.config.CaptureDurationMs)

	if d.onDetecting != nil {
		go d.onDetecting()
	}
}

// AddAudio adds audio to the capture buffer.
func (d *InterruptDetector) AddAudio(data []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.capturing {
		return
	}

	d.captureBuffer.Write(data)
}

// AddTranscript adds transcription to the capture.
func (d *InterruptDetector) AddTranscript(text string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.capturing {
		return
	}

	if d.transcript != "" && !strings.HasSuffix(d.transcript, " ") {
		d.transcript += " "
	}
	d.transcript += text
}

// IsCapturing returns whether we're in the capture window.
func (d *InterruptDetector) IsCapturing() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.capturing
}

// CaptureComplete returns whether the capture window has elapsed.
func (d *InterruptDetector) CaptureComplete() bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.capturing {
		return false
	}

	elapsed := time.Since(d.captureStart)
	return elapsed >= time.Duration(d.config.CaptureDurationMs)*time.Millisecond
}

// Analyze analyzes the captured audio/transcript and returns the result.
// This should be called after CaptureComplete() returns true.
func (d *InterruptDetector) Analyze(ctx context.Context) InterruptResult {
	d.mu.Lock()
	transcript := d.transcript
	d.capturing = false
	mode := d.config.Mode
	d.mu.Unlock()

	d.debug("INTERRUPT", "Capture complete, analyzing...")

	if d.onCaptured != nil {
		go d.onCaptured(transcript)
	}

	// Handle different modes
	switch mode {
	case InterruptModeNever:
		d.debug("INTERRUPT", "Mode is 'never', dismissing")
		if d.onDismissed != nil {
			go d.onDismissed(transcript, "mode_never")
		}
		return InterruptNone

	case InterruptModeAlways:
		if transcript != "" {
			d.debug("INTERRUPT", "Mode is 'always', confirming interrupt")
			if d.onConfirmed != nil {
				go d.onConfirmed(transcript)
			}
			return InterruptReal
		}
		d.debug("INTERRUPT", "No transcript, dismissing")
		if d.onDismissed != nil {
			go d.onDismissed("", "no_speech")
		}
		return InterruptNone

	case InterruptModeAuto:
		return d.analyzeAuto(ctx, transcript)

	default:
		return d.analyzeAuto(ctx, transcript)
	}
}

// analyzeAuto performs semantic analysis to distinguish interrupts from backchannels.
func (d *InterruptDetector) analyzeAuto(ctx context.Context, transcript string) InterruptResult {
	// No transcription = noise
	if transcript == "" {
		d.debug("INTERRUPT", "No transcript detected, likely noise")
		if d.onDismissed != nil {
			go d.onDismissed("", "no_speech")
		}
		return InterruptNone
	}

	// Check for common backchannels first (fast path)
	if d.isLikelyBackchannel(transcript) {
		d.debug("INTERRUPT", "Detected backchannel: %q", transcript)
		if d.onDismissed != nil {
			go d.onDismissed(transcript, "backchannel")
		}
		return InterruptBackchannel
	}

	// Perform semantic check if enabled
	if d.config.SemanticCheck && d.checker != nil {
		// 1200ms allows time for Anthropic API calls (~800-1500ms typical)
		checkCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
		defer cancel()

		isInterrupt, err := d.checker.CheckInterrupt(checkCtx, transcript)
		if err != nil {
			d.debug("INTERRUPT", "Semantic check failed: %v, treating as interrupt", err)
			if d.onConfirmed != nil {
				go d.onConfirmed(transcript)
			}
			return InterruptReal
		}

		if !isInterrupt {
			d.debug("INTERRUPT", "Semantic check: backchannel")
			if d.onDismissed != nil {
				go d.onDismissed(transcript, "semantic_backchannel")
			}
			return InterruptBackchannel
		}

		d.debug("INTERRUPT", "Semantic check: real interrupt")
		if d.onConfirmed != nil {
			go d.onConfirmed(transcript)
		}
		return InterruptReal
	}

	// No semantic check, treat as interrupt
	d.debug("INTERRUPT", "No semantic check, treating as interrupt")
	if d.onConfirmed != nil {
		go d.onConfirmed(transcript)
	}
	return InterruptReal
}

// isLikelyBackchannel does a quick check for common backchannel phrases.
func (d *InterruptDetector) isLikelyBackchannel(transcript string) bool {
	lower := strings.ToLower(strings.TrimSpace(transcript))

	// Common backchannels
	backchannels := []string{
		"uh huh", "uh-huh", "uhuh",
		"mm hmm", "mm-hmm", "mmhmm", "mhm",
		"yeah", "yep", "yup",
		"okay", "ok", "k",
		"right", "i see", "got it",
		"sure", "alright", "all right",
		"hmm", "hm", "ah",
		"oh", "oh okay", "oh ok",
	}

	for _, bc := range backchannels {
		if lower == bc {
			return true
		}
	}

	return false
}

// Reset clears the interrupt detector state.
func (d *InterruptDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.capturing = false
	d.captureStart = time.Time{}
	d.captureBuffer.Clear()
	d.transcript = ""
}

// GetCapturedAudio returns the audio captured during the window.
func (d *InterruptDetector) GetCapturedAudio() []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.captureBuffer.Read()
}

// GetCapturedTranscript returns the transcript captured during the window.
func (d *InterruptDetector) GetCapturedTranscript() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.transcript
}

func (d *InterruptDetector) debug(category, format string, args ...any) {
	if d.onDebug != nil {
		msg := format
		if len(args) > 0 {
			msg = formatDebugMessage(format, args...)
		}
		go d.onDebug(category, msg)
	}
}

func formatDebugMessage(format string, args ...any) string {
	result := format
	for _, arg := range args {
		switch v := arg.(type) {
		case string:
			result = strings.Replace(result, "%s", v, 1)
			result = strings.Replace(result, "%q", "\""+v+"\"", 1)
			result = strings.Replace(result, "%v", v, 1)
		case int:
			s := intToString(v)
			result = strings.Replace(result, "%d", s, 1)
			result = strings.Replace(result, "%dms", s+"ms", 1)
		case error:
			result = strings.Replace(result, "%v", v.Error(), 1)
		}
	}
	return result
}

// DefaultInterruptChecker is a simple implementation using a callback function.
type DefaultInterruptChecker struct {
	checkFunc func(ctx context.Context, transcript string) (bool, error)
}

// NewDefaultInterruptChecker creates an interrupt checker from a callback function.
func NewDefaultInterruptChecker(checkFunc func(ctx context.Context, transcript string) (bool, error)) *DefaultInterruptChecker {
	return &DefaultInterruptChecker{checkFunc: checkFunc}
}

// CheckInterrupt implements InterruptChecker.
func (c *DefaultInterruptChecker) CheckInterrupt(ctx context.Context, transcript string) (bool, error) {
	return c.checkFunc(ctx, transcript)
}

// InterruptCheckPrompt is the prompt template for semantic interrupt checks.
const InterruptCheckPrompt = `The user said: "%s"

The AI assistant was speaking when this was said. Is the user trying to interrupt and change the topic or stop the assistant? Or is this just a backchannel response (like "uh huh", "okay", "right") that doesn't require stopping?

Reply only: INTERRUPT or BACKCHANNEL`

// ParseInterruptCheckResponse parses the LLM response to determine if it's an interrupt.
func ParseInterruptCheckResponse(response string) bool {
	upper := strings.ToUpper(strings.TrimSpace(response))
	return strings.Contains(upper, "INTERRUPT")
}
