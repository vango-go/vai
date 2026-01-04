package live

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockSemanticChecker implements SemanticChecker for testing.
type mockSemanticChecker struct {
	mu       sync.Mutex
	response bool
	err      error
	called   bool
	delay    time.Duration
}

func (m *mockSemanticChecker) CheckTurnComplete(ctx context.Context, transcript string) (bool, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.called = true
	return m.response, m.err
}

func (m *mockSemanticChecker) wasCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.called
}

func TestHybridVAD_PunctuationTrigger(t *testing.T) {
	config := VADConfig{
		Model:               "test-model",
		PunctuationTrigger:  ".!?",
		NoActivityTimeoutMs: 3000,
		SemanticCheck:       true,
		MinWordsForCheck:    1,
		EnergyThreshold:     0.02,
	}

	audioConfig := DefaultAudioConfig()
	checker := &mockSemanticChecker{response: true}
	vad := NewHybridVAD(config, audioConfig, checker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vad.Start(ctx)
	defer vad.Stop()

	commitCalled := false
	vad.SetCallbacks(
		nil, // onSilence
		nil, // onAnalyzing
		func(transcript string, forced bool) { commitCalled = true },
		nil, // onDebug
	)

	// Add transcript with period - should trigger semantic check
	vad.AddTranscript("Hello.")

	// Wait for async semantic check
	time.Sleep(50 * time.Millisecond)

	if !checker.wasCalled() {
		t.Error("Expected semantic checker to be called on punctuation")
	}
	if !commitCalled {
		t.Error("Expected commit callback when semantic check returns true")
	}
}

func TestHybridVAD_NoPunctuationNoTrigger(t *testing.T) {
	config := VADConfig{
		Model:               "test-model",
		PunctuationTrigger:  ".!?",
		NoActivityTimeoutMs: 3000,
		SemanticCheck:       true,
		MinWordsForCheck:    1,
		EnergyThreshold:     0.02,
	}

	audioConfig := DefaultAudioConfig()
	checker := &mockSemanticChecker{response: true}
	vad := NewHybridVAD(config, audioConfig, checker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vad.Start(ctx)
	defer vad.Stop()

	commitCalled := false
	vad.SetCallbacks(
		nil,
		nil,
		func(transcript string, forced bool) { commitCalled = true },
		nil,
	)

	// Add transcript WITHOUT punctuation - should NOT trigger semantic check
	vad.AddTranscript("Hello world")

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	if checker.wasCalled() {
		t.Error("Expected semantic checker NOT to be called without punctuation")
	}
	if commitCalled {
		t.Error("Expected NO commit without punctuation")
	}
}

func TestHybridVAD_TimeoutTrigger(t *testing.T) {
	config := VADConfig{
		Model:               "test-model",
		PunctuationTrigger:  ".!?",
		NoActivityTimeoutMs: 150, // Short for testing but allow for ticker interval (200ms)
		SemanticCheck:       true,
		MinWordsForCheck:    1,
		EnergyThreshold:     0.02,
	}

	audioConfig := DefaultAudioConfig()
	checker := &mockSemanticChecker{response: true}
	vad := NewHybridVAD(config, audioConfig, checker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vad.Start(ctx)
	defer vad.Stop()

	commitCalled := false
	var commitForced bool
	var mu sync.Mutex
	vad.SetCallbacks(
		nil,
		nil,
		func(transcript string, forced bool) {
			mu.Lock()
			commitCalled = true
			commitForced = forced
			mu.Unlock()
		},
		nil,
	)

	// Add transcript without punctuation
	vad.AddTranscript("Hello world")

	// Wait past timeout + ticker interval (200ms) + some buffer
	// Ticker runs every 200ms, timeout is 150ms, so first check at 200ms should trigger
	time.Sleep(350 * time.Millisecond)

	mu.Lock()
	called := checker.wasCalled()
	commit := commitCalled
	forced := commitForced
	mu.Unlock()

	if !called {
		t.Error("Expected semantic checker to be called on timeout")
	}
	if !commit {
		t.Error("Expected commit callback on timeout")
	}
	if !forced {
		t.Error("Expected forced=true when triggered by timeout")
	}
}

func TestHybridVAD_SemanticCheckIncomplete(t *testing.T) {
	config := VADConfig{
		Model:               "test-model",
		PunctuationTrigger:  ".!?",
		NoActivityTimeoutMs: 3000,
		SemanticCheck:       true,
		MinWordsForCheck:    1,
		EnergyThreshold:     0.02,
	}

	audioConfig := DefaultAudioConfig()
	checker := &mockSemanticChecker{response: false} // Will say incomplete
	vad := NewHybridVAD(config, audioConfig, checker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vad.Start(ctx)
	defer vad.Stop()

	commitCalled := false
	vad.SetCallbacks(
		nil,
		nil,
		func(transcript string, forced bool) { commitCalled = true },
		nil,
	)

	// Add transcript with punctuation
	vad.AddTranscript("Book me a flight to?")

	// Wait for semantic check
	time.Sleep(50 * time.Millisecond)

	if !checker.wasCalled() {
		t.Error("Expected semantic checker to be called on punctuation")
	}
	if commitCalled {
		t.Error("Expected NO commit when semantic check says incomplete")
	}
}

func TestHybridVAD_MinWordsForCheck(t *testing.T) {
	config := VADConfig{
		Model:               "test-model",
		PunctuationTrigger:  ".!?",
		NoActivityTimeoutMs: 100, // Short timeout
		SemanticCheck:       true,
		MinWordsForCheck:    3, // Require 3 words
		EnergyThreshold:     0.02,
	}

	audioConfig := DefaultAudioConfig()
	checker := &mockSemanticChecker{response: true}
	vad := NewHybridVAD(config, audioConfig, checker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vad.Start(ctx)
	defer vad.Stop()

	// Add transcript with only 2 words and punctuation
	vad.AddTranscript("Hi there!")

	// Wait a bit - should not trigger since only 2 words
	time.Sleep(50 * time.Millisecond)

	if checker.wasCalled() {
		t.Error("Expected semantic checker NOT to be called when word count below minimum")
	}
}

func TestHybridVAD_GetTranscript(t *testing.T) {
	config := DefaultVADConfig()
	audioConfig := DefaultAudioConfig()
	vad := NewHybridVAD(config, audioConfig, nil)

	vad.AddTranscript("Hello")
	vad.AddTranscript(" world")

	transcript := vad.GetTranscript()
	if transcript != "Hello world" {
		t.Errorf("Expected 'Hello world', got %q", transcript)
	}
}

func TestHybridVAD_Reset(t *testing.T) {
	config := DefaultVADConfig()
	audioConfig := DefaultAudioConfig()
	vad := NewHybridVAD(config, audioConfig, nil)

	vad.AddTranscript("Hello world")

	// Verify state before reset
	if vad.GetTranscript() == "" {
		t.Error("Expected transcript to be set before reset")
	}

	// Reset
	vad.Reset()

	// Verify state after reset
	if vad.GetTranscript() != "" {
		t.Errorf("Expected empty transcript after reset, got %q", vad.GetTranscript())
	}
}

func TestHybridVAD_SetTranscript(t *testing.T) {
	config := DefaultVADConfig()
	audioConfig := DefaultAudioConfig()
	vad := NewHybridVAD(config, audioConfig, nil)

	vad.AddTranscript("Initial text")
	vad.SetTranscript("Replaced text")

	transcript := vad.GetTranscript()
	if transcript != "Replaced text" {
		t.Errorf("Expected 'Replaced text', got %q", transcript)
	}
}

func TestHybridVAD_SemanticCheckDisabled(t *testing.T) {
	config := VADConfig{
		Model:               "test-model",
		PunctuationTrigger:  ".!?",
		NoActivityTimeoutMs: 3000,
		SemanticCheck:       false, // Disabled
		MinWordsForCheck:    1,
		EnergyThreshold:     0.02,
	}

	audioConfig := DefaultAudioConfig()
	checker := &mockSemanticChecker{response: true}
	vad := NewHybridVAD(config, audioConfig, checker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vad.Start(ctx)
	defer vad.Stop()

	commitCalled := false
	vad.SetCallbacks(
		nil,
		nil,
		func(transcript string, forced bool) { commitCalled = true },
		nil,
	)

	// Add transcript with punctuation
	vad.AddTranscript("Hello!")

	// Wait for async processing
	time.Sleep(50 * time.Millisecond)

	if checker.wasCalled() {
		t.Error("Expected semantic checker NOT to be called when disabled")
	}
	if !commitCalled {
		t.Error("Expected commit callback when semantic check is disabled")
	}
}

func TestHybridVAD_MultiplePunctuationMarks(t *testing.T) {
	config := VADConfig{
		Model:               "test-model",
		PunctuationTrigger:  ".!?",
		NoActivityTimeoutMs: 3000,
		SemanticCheck:       true,
		MinWordsForCheck:    1,
		EnergyThreshold:     0.02,
	}

	tests := []struct {
		name       string
		transcript string
		shouldFire bool
	}{
		{"period", "Hello.", true},
		{"exclamation", "Hello!", true},
		{"question", "Hello?", true},
		{"comma", "Hello,", false},
		{"no_punctuation", "Hello", false},
		{"trailing_space", "Hello. ", true}, // TrimSpace handles this
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			audioConfig := DefaultAudioConfig()
			checker := &mockSemanticChecker{response: true}
			vad := NewHybridVAD(config, audioConfig, checker)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			vad.Start(ctx)
			defer vad.Stop()

			vad.AddTranscript(tt.transcript)
			time.Sleep(50 * time.Millisecond)

			if checker.wasCalled() != tt.shouldFire {
				t.Errorf("Expected semantic checker called=%v, got %v", tt.shouldFire, checker.wasCalled())
			}
		})
	}
}

func TestHybridVAD_PreventDoubleCommit(t *testing.T) {
	config := VADConfig{
		Model:               "test-model",
		PunctuationTrigger:  ".!?",
		NoActivityTimeoutMs: 50, // Short timeout
		SemanticCheck:       true,
		MinWordsForCheck:    1,
		EnergyThreshold:     0.02,
	}

	audioConfig := DefaultAudioConfig()
	checker := &mockSemanticChecker{response: true}
	vad := NewHybridVAD(config, audioConfig, checker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vad.Start(ctx)
	defer vad.Stop()

	commitCount := 0
	var commitMu sync.Mutex
	vad.SetCallbacks(
		nil,
		nil,
		func(transcript string, forced bool) {
			commitMu.Lock()
			commitCount++
			commitMu.Unlock()
		},
		nil,
	)

	// Add transcript with punctuation
	vad.AddTranscript("Hello.")

	// Wait well past timeout
	time.Sleep(200 * time.Millisecond)

	commitMu.Lock()
	count := commitCount
	commitMu.Unlock()

	if count != 1 {
		t.Errorf("Expected exactly 1 commit, got %d (double commit bug)", count)
	}
}
