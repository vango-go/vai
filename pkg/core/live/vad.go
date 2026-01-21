package live

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// VADResult indicates the outcome of VAD processing.
type VADResult int

const (
	// VADContinue means keep listening for more audio.
	VADContinue VADResult = iota
	// VADCommit means the user turn is complete.
	VADCommit
)

// String returns a human-readable VAD result.
func (r VADResult) String() string {
	switch r {
	case VADContinue:
		return "CONTINUE"
	case VADCommit:
		return "COMMIT"
	default:
		return "UNKNOWN"
	}
}

// SemanticChecker is an interface for performing semantic turn completion checks.
// This is injected into the VAD to allow flexible LLM integration.
type SemanticChecker interface {
	// CheckTurnComplete asks the LLM if the given transcript appears complete.
	// Returns true if the user appears to be done speaking.
	CheckTurnComplete(ctx context.Context, transcript string) (bool, error)
}

// HybridVAD implements punctuation-based voice activity detection:
// 1. Punctuation triggers (. ! ?) → immediate semantic check
// 2. Timeout fallback (3 seconds no activity) → force semantic check
// 3. Semantic check confirms turn completion before commit
type HybridVAD struct {
	config        VADConfig
	semanticCheck SemanticChecker
	audioConfig   AudioConfig

	mu                       sync.Mutex
	ctx                      context.Context
	cancel                   context.CancelFunc
	transcript               strings.Builder
	lastTranscriptTime       time.Time
	pendingCheck             bool
	committed                bool // Prevents double commits before Reset is called
	lastCheckedTranscriptLen int  // Prevents re-checking same transcript on timeout

	// Callbacks for events
	onSilence   func(durationMs int) // Kept for API compatibility
	onAnalyzing func(transcript string)
	onCommit    func(transcript string, forced bool)
	onDebug     func(category, message string)
}

// NewHybridVAD creates a new hybrid VAD with the given configuration.
func NewHybridVAD(config VADConfig, audioConfig AudioConfig, checker SemanticChecker) *HybridVAD {
	return &HybridVAD{
		config:        config,
		audioConfig:   audioConfig,
		semanticCheck: checker,
	}
}

// SetCallbacks sets the event callbacks for the VAD.
func (v *HybridVAD) SetCallbacks(
	onSilence func(durationMs int),
	onAnalyzing func(transcript string),
	onCommit func(transcript string, forced bool),
	onDebug func(category, message string),
) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.onSilence = onSilence
	v.onAnalyzing = onAnalyzing
	v.onCommit = onCommit
	v.onDebug = onDebug
}

// Start begins the VAD timeout checker goroutine.
// Must be called before adding transcripts.
func (v *HybridVAD) Start(ctx context.Context) {
	v.mu.Lock()
	v.ctx, v.cancel = context.WithCancel(ctx)
	v.mu.Unlock()

	go v.timeoutLoop()
}

// Stop stops the VAD timeout checker goroutine.
func (v *HybridVAD) Stop() {
	v.mu.Lock()
	if v.cancel != nil {
		v.cancel()
	}
	v.mu.Unlock()
}

// timeoutLoop checks for transcript inactivity and triggers semantic check.
func (v *HybridVAD) timeoutLoop() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-v.ctx.Done():
			return
		case <-ticker.C:
			v.checkTimeout()
		}
	}
}

// checkTimeout triggers semantic check or force commit if no new transcript for NoActivityTimeoutMs.
func (v *HybridVAD) checkTimeout() {
	v.mu.Lock()

	// Skip if already committed, pending check, or no transcript
	if v.committed || v.pendingCheck || v.transcript.Len() == 0 {
		v.mu.Unlock()
		return
	}

	// Skip if no transcript time set yet
	if v.lastTranscriptTime.IsZero() {
		v.mu.Unlock()
		return
	}

	// Check if timeout exceeded
	timeout := time.Duration(v.config.NoActivityTimeoutMs) * time.Millisecond
	if time.Since(v.lastTranscriptTime) < timeout {
		v.mu.Unlock()
		return
	}

	transcript := v.transcript.String()
	transcriptLen := len(transcript)
	words := strings.Fields(transcript)

	// Check minimum word count
	if len(words) < v.config.MinWordsForCheck {
		v.mu.Unlock()
		return
	}

	// If we already checked this transcript (semantic returned INCOMPLETE),
	// force commit now instead of re-checking
	if transcriptLen <= v.lastCheckedTranscriptLen {
		v.committed = true
		v.mu.Unlock()
		v.debug("VAD", fmt.Sprintf("Timeout reached (%dms), force committing after previous INCOMPLETE", v.config.NoActivityTimeoutMs))
		if v.onCommit != nil {
			go v.onCommit(transcript, true)
		}
		return
	}

	v.debug("VAD", fmt.Sprintf("Timeout reached (%dms), triggering semantic check", v.config.NoActivityTimeoutMs))

	// Release lock during semantic check to not block AddTranscript
	v.pendingCheck = true
	v.mu.Unlock()

	v.triggerSemanticCheck(transcript, true)
}

// AddTranscript adds text to the accumulated transcript and checks for punctuation triggers.
// This is the primary method for turn detection - call it with each STT transcript delta.
func (v *HybridVAD) AddTranscript(text string) {
	if text == "" {
		return
	}

	v.mu.Lock()

	// Skip if already committed
	if v.committed {
		v.mu.Unlock()
		return
	}

	// Add text and update timestamp
	v.transcript.WriteString(text)
	v.lastTranscriptTime = time.Now()
	fullText := v.transcript.String()

	v.debug("VAD", fmt.Sprintf("Transcript updated: %q", fullText))

	// Check for punctuation trigger
	if v.endsWithPunctuation(fullText) {
		words := strings.Fields(fullText)

		// Check minimum word count
		if len(words) >= v.config.MinWordsForCheck && !v.pendingCheck {
			v.debug("VAD", fmt.Sprintf("Punctuation detected in %q, triggering semantic check", fullText))
			v.pendingCheck = true
			v.mu.Unlock()
			v.triggerSemanticCheck(fullText, false)
			return
		}
	}

	v.mu.Unlock()
}

// endsWithPunctuation checks if text ends with a trigger punctuation mark.
func (v *HybridVAD) endsWithPunctuation(text string) bool {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return false
	}
	lastChar := text[len(text)-1]
	return strings.ContainsRune(v.config.PunctuationTrigger, rune(lastChar))
}

// triggerSemanticCheck performs the LLM semantic check.
// Must be called WITHOUT the mutex held.
func (v *HybridVAD) triggerSemanticCheck(transcript string, forced bool) {
	if v.onAnalyzing != nil {
		go v.onAnalyzing(transcript)
	}

	// If semantic check is disabled, commit immediately
	if !v.config.SemanticCheck || v.semanticCheck == nil {
		v.mu.Lock()
		v.pendingCheck = false
		if !v.committed {
			v.committed = true
			v.mu.Unlock()
			v.debug("SEMANTIC", "Semantic check disabled, committing")
			if v.onCommit != nil {
				go v.onCommit(transcript, forced)
			}
			return
		}
		v.mu.Unlock()
		return
	}

	// Create a timeout context for the semantic check
	checkCtx, cancel := context.WithTimeout(v.ctx, 1200*time.Millisecond)
	defer cancel()

	complete, err := v.semanticCheck.CheckTurnComplete(checkCtx, transcript)
	if err != nil {
		v.debug("SEMANTIC", fmt.Sprintf("Check failed: %v, treating as complete", err))
		complete = true
	}

	v.mu.Lock()
	v.pendingCheck = false

	// Track what we checked to prevent infinite retry loops
	v.lastCheckedTranscriptLen = len(transcript)

	// Check if we were committed/reset while checking
	if v.committed {
		v.mu.Unlock()
		return
	}

	if complete {
		v.committed = true
		v.mu.Unlock()
		v.debug("SEMANTIC", fmt.Sprintf("Transcript %q is COMPLETE", transcript))
		if v.onCommit != nil {
			go v.onCommit(transcript, forced)
		}
		return
	}

	v.debug("SEMANTIC", fmt.Sprintf("Transcript %q is INCOMPLETE, waiting for more input...", transcript))
	v.mu.Unlock()
}

// Reset clears the VAD state for a new turn.
func (v *HybridVAD) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.transcript.Reset()
	v.lastTranscriptTime = time.Time{}
	v.pendingCheck = false
	v.committed = false
	v.lastCheckedTranscriptLen = 0
}

// GetTranscript returns the current accumulated transcript.
func (v *HybridVAD) GetTranscript() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.transcript.String()
}

// SetTranscript sets the transcript (used for grace period continuation).
// This also resets the committed flag to allow new commits.
func (v *HybridVAD) SetTranscript(text string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.transcript.Reset()
	v.transcript.WriteString(text)
	v.lastTranscriptTime = time.Now()
	v.committed = false
	v.pendingCheck = false
	v.lastCheckedTranscriptLen = 0
}

func (v *HybridVAD) debug(category, message string) {
	if v.onDebug != nil {
		go v.onDebug(category, message)
	}
}

// DefaultSemanticChecker is a simple implementation using a callback function.
type DefaultSemanticChecker struct {
	checkFunc func(ctx context.Context, transcript string) (bool, error)
}

// NewDefaultSemanticChecker creates a semantic checker from a callback function.
func NewDefaultSemanticChecker(checkFunc func(ctx context.Context, transcript string) (bool, error)) *DefaultSemanticChecker {
	return &DefaultSemanticChecker{checkFunc: checkFunc}
}

// CheckTurnComplete implements SemanticChecker.
func (c *DefaultSemanticChecker) CheckTurnComplete(ctx context.Context, transcript string) (bool, error) {
	return c.checkFunc(ctx, transcript)
}

// TurnCompletePrompt is the prompt template for semantic turn completion checks.
const TurnCompletePrompt = `Voice transcript: "%s"

You are part of a live mode voice AI agent. Your job is to look at the transcription of the what the user has said so far since the agent last spoke and determine if the user is done talking and therefore the agent should respond. Or if the user is not done talking and the agent should wait.

YES = The user is done talking
NO = The user is not done talking

Reply only: YES or NO`

// ParseTurnCompleteResponse parses the LLM response to determine if turn is complete.
func ParseTurnCompleteResponse(response string) bool {
	upper := strings.ToUpper(strings.TrimSpace(response))
	return strings.Contains(upper, "YES")
}
