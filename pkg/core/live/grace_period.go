package live

import (
	"strings"
	"sync"
	"time"
)

// GracePeriodManager handles the post-VAD window where users can continue speaking.
//
// After VAD commits a turn, the grace period allows users to continue their thought
// without triggering the interrupt flow. If the user speaks during this window:
// 1. The current agent request is cancelled
// 2. The new speech is appended to the original transcript
// 3. VAD re-runs on the combined input
type GracePeriodManager struct {
	config GracePeriodConfig

	mu                 sync.Mutex
	active             bool
	startTime          time.Time
	originalTranscript string
	timer              *time.Timer

	// Callbacks
	onExpired      func(transcript string)
	onContinuation func(combinedTranscript string)
	onDebug        func(category, message string)
}

// NewGracePeriodManager creates a new grace period manager.
func NewGracePeriodManager(config GracePeriodConfig) *GracePeriodManager {
	return &GracePeriodManager{
		config: config,
	}
}

// SetCallbacks sets the event callbacks.
func (g *GracePeriodManager) SetCallbacks(
	onExpired func(transcript string),
	onContinuation func(combinedTranscript string),
	onDebug func(category, message string),
) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onExpired = onExpired
	g.onContinuation = onContinuation
	g.onDebug = onDebug
}

// Start begins the grace period with the given transcript.
// The onExpired callback is called if the period expires without continuation.
// The onContinuation callback is called if the user speaks during the period.
func (g *GracePeriodManager) Start(transcript string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.config.Enabled {
		// Grace period disabled, immediately "expire"
		g.debug("GRACE", "Grace period disabled, expiring immediately")
		if g.onExpired != nil {
			go g.onExpired(transcript)
		}
		return
	}

	// Cancel any existing timer
	if g.timer != nil {
		g.timer.Stop()
	}

	g.active = true
	g.startTime = time.Now()
	g.originalTranscript = transcript

	g.debug("GRACE", "Started (%dms window)", g.config.DurationMs)

	g.timer = time.AfterFunc(
		time.Duration(g.config.DurationMs)*time.Millisecond,
		g.expire,
	)
}

// expire is called when the grace period timer fires.
func (g *GracePeriodManager) expire() {
	g.mu.Lock()
	if !g.active {
		g.mu.Unlock()
		return
	}
	g.active = false
	transcript := g.originalTranscript
	callback := g.onExpired
	g.mu.Unlock()

	g.debug("GRACE", "Expired without continuation")

	if callback != nil {
		callback(transcript)
	}
}

// HandleUserSpeech is called when user audio/transcription is detected during grace period.
// Returns true if the grace period was active and handled the speech.
// The newTranscript is the new speech that was detected.
func (g *GracePeriodManager) HandleUserSpeech(newTranscript string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.active {
		return false
	}

	// Cancel the expiry timer
	if g.timer != nil {
		g.timer.Stop()
	}

	// Combine transcripts with proper spacing
	// Always ensure there's a space between original and new transcript
	combined := strings.TrimSpace(g.originalTranscript)
	newText := strings.TrimSpace(newTranscript)
	if combined != "" && newText != "" {
		combined += " " + newText
	} else {
		combined += newText
	}

	g.debug("GRACE", "User continued speaking, combined: %q", combined)

	// Reset grace period state
	g.active = false

	// Trigger continuation callback
	if g.onContinuation != nil {
		go g.onContinuation(combined)
	}

	return true
}

// IsActive returns whether the grace period is currently active.
func (g *GracePeriodManager) IsActive() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.active
}

// Cancel stops the grace period without triggering any callbacks.
func (g *GracePeriodManager) Cancel() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.timer != nil {
		g.timer.Stop()
	}
	g.active = false
	g.debug("GRACE", "Cancelled")
}

// TimeRemaining returns the time remaining in the grace period.
func (g *GracePeriodManager) TimeRemaining() time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.active {
		return 0
	}

	elapsed := time.Since(g.startTime)
	remaining := time.Duration(g.config.DurationMs)*time.Millisecond - elapsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ExpiresAt returns the time when the grace period will expire.
func (g *GracePeriodManager) ExpiresAt() time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.active {
		return time.Time{}
	}

	return g.startTime.Add(time.Duration(g.config.DurationMs) * time.Millisecond)
}

// OriginalTranscript returns the transcript that started this grace period.
func (g *GracePeriodManager) OriginalTranscript() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.originalTranscript
}

func (g *GracePeriodManager) debug(category, format string, args ...any) {
	if g.onDebug != nil {
		msg := format
		if len(args) > 0 {
			msg = formatMessage(format, args...)
		}
		go g.onDebug(category, msg)
	}
}

// formatMessage is a simple format helper that doesn't require importing fmt
// in the hot path.
func formatMessage(format string, args ...any) string {
	// For simplicity, we'll use a basic approach
	// In production you might want to use fmt.Sprintf
	result := format
	for _, arg := range args {
		switch v := arg.(type) {
		case string:
			result = strings.Replace(result, "%s", v, 1)
			result = strings.Replace(result, "%q", "\""+v+"\"", 1)
		case int:
			result = strings.Replace(result, "%d", intToString(v), 1)
			result = strings.Replace(result, "%dms", intToString(v)+"ms", 1)
		}
	}
	return result
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	// Reverse
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}
