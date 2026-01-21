package live

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockInterruptChecker implements InterruptChecker for testing.
type mockInterruptChecker struct {
	response bool
	err      error
	called   bool
}

func (m *mockInterruptChecker) CheckInterrupt(ctx context.Context, transcript string) (bool, error) {
	m.called = true
	return m.response, m.err
}

func TestInterruptDetector_StartCapture(t *testing.T) {
	config := InterruptConfig{
		Mode:              InterruptModeAuto,
		EnergyThreshold:   0.05,
		CaptureDurationMs: 100,
		SemanticCheck:     true,
	}

	audioConfig := DefaultAudioConfig()
	checker := &mockInterruptChecker{}
	detector := NewInterruptDetector(config, audioConfig, checker)

	detectingCalled := false
	detector.SetCallbacks(
		func() { detectingCalled = true },
		nil, nil, nil, nil,
	)

	detector.StartCapture()

	// Give callback time to execute
	time.Sleep(10 * time.Millisecond)

	if !detector.IsCapturing() {
		t.Error("Expected detector to be capturing")
	}

	if !detectingCalled {
		t.Error("Expected detecting callback to be called")
	}
}

func TestInterruptDetector_AddAudio(t *testing.T) {
	config := InterruptConfig{
		Mode:              InterruptModeAuto,
		EnergyThreshold:   0.05,
		CaptureDurationMs: 100,
		SemanticCheck:     true,
	}

	audioConfig := DefaultAudioConfig()
	detector := NewInterruptDetector(config, audioConfig, nil)

	// Should not capture if not started
	detector.AddAudio([]byte{1, 2, 3, 4})
	captured := detector.GetCapturedAudio()
	if len(captured) != 0 {
		t.Error("Expected no audio captured when not in capture mode")
	}

	// Start capture and add audio
	detector.StartCapture()
	detector.AddAudio([]byte{1, 2, 3, 4})
	detector.AddAudio([]byte{5, 6, 7, 8})

	captured = detector.GetCapturedAudio()
	if len(captured) != 8 {
		t.Errorf("Expected 8 bytes captured, got %d", len(captured))
	}
}

func TestInterruptDetector_AddTranscript(t *testing.T) {
	config := InterruptConfig{
		Mode:              InterruptModeAuto,
		EnergyThreshold:   0.05,
		CaptureDurationMs: 100,
		SemanticCheck:     true,
	}

	audioConfig := DefaultAudioConfig()
	detector := NewInterruptDetector(config, audioConfig, nil)

	// Should not capture if not started
	detector.AddTranscript("Hello")
	transcript := detector.GetCapturedTranscript()
	if transcript != "" {
		t.Error("Expected no transcript captured when not in capture mode")
	}

	// Start capture and add transcript
	detector.StartCapture()
	detector.AddTranscript("Hello")
	detector.AddTranscript("world")

	transcript = detector.GetCapturedTranscript()
	if transcript != "Hello world" {
		t.Errorf("Expected 'Hello world', got %q", transcript)
	}
}

func TestInterruptDetector_CaptureComplete(t *testing.T) {
	config := InterruptConfig{
		Mode:              InterruptModeAuto,
		EnergyThreshold:   0.05,
		CaptureDurationMs: 50, // Short for testing
		SemanticCheck:     true,
	}

	audioConfig := DefaultAudioConfig()
	detector := NewInterruptDetector(config, audioConfig, nil)

	detector.StartCapture()

	if detector.CaptureComplete() {
		t.Error("Expected capture NOT to be complete immediately")
	}

	// Wait for capture duration
	time.Sleep(60 * time.Millisecond)

	if !detector.CaptureComplete() {
		t.Error("Expected capture to be complete after duration")
	}
}

func TestInterruptDetector_Analyze_ModeNever(t *testing.T) {
	config := InterruptConfig{
		Mode:              InterruptModeNever,
		CaptureDurationMs: 50,
	}

	audioConfig := DefaultAudioConfig()
	detector := NewInterruptDetector(config, audioConfig, nil)

	dismissedCalled := false
	var mu sync.Mutex

	detector.SetCallbacks(
		nil,
		nil,
		func(transcript, reason string) {
			mu.Lock()
			dismissedCalled = true
			mu.Unlock()
		},
		nil,
		nil,
	)

	detector.StartCapture()
	detector.AddTranscript("Stop!")
	time.Sleep(60 * time.Millisecond)

	result := detector.Analyze(context.Background())

	if result != InterruptNone {
		t.Errorf("Expected InterruptNone in never mode, got %v", result)
	}

	time.Sleep(10 * time.Millisecond)
	mu.Lock()
	dismissed := dismissedCalled
	mu.Unlock()

	if !dismissed {
		t.Error("Expected dismissed callback to be called")
	}
}

func TestInterruptDetector_Analyze_ModeAlways_WithTranscript(t *testing.T) {
	config := InterruptConfig{
		Mode:              InterruptModeAlways,
		CaptureDurationMs: 50,
	}

	audioConfig := DefaultAudioConfig()
	detector := NewInterruptDetector(config, audioConfig, nil)

	confirmedCalled := false
	var mu sync.Mutex

	detector.SetCallbacks(
		nil, nil, nil,
		func(transcript string) {
			mu.Lock()
			confirmedCalled = true
			mu.Unlock()
		},
		nil,
	)

	detector.StartCapture()
	detector.AddTranscript("Anything")
	time.Sleep(60 * time.Millisecond)

	result := detector.Analyze(context.Background())

	if result != InterruptReal {
		t.Errorf("Expected InterruptReal in always mode with transcript, got %v", result)
	}

	time.Sleep(10 * time.Millisecond)
	mu.Lock()
	confirmed := confirmedCalled
	mu.Unlock()

	if !confirmed {
		t.Error("Expected confirmed callback to be called")
	}
}

func TestInterruptDetector_Analyze_ModeAlways_NoTranscript(t *testing.T) {
	config := InterruptConfig{
		Mode:              InterruptModeAlways,
		CaptureDurationMs: 50,
	}

	audioConfig := DefaultAudioConfig()
	detector := NewInterruptDetector(config, audioConfig, nil)

	detector.StartCapture()
	// No transcript added
	time.Sleep(60 * time.Millisecond)

	result := detector.Analyze(context.Background())

	if result != InterruptNone {
		t.Errorf("Expected InterruptNone with no transcript, got %v", result)
	}
}

func TestInterruptDetector_Analyze_ModeAuto_Backchannel(t *testing.T) {
	backchannels := []string{"uh huh", "mm hmm", "yeah", "okay", "right", "I see", "got it", "sure"}

	for _, bc := range backchannels {
		t.Run(bc, func(t *testing.T) {
			config := InterruptConfig{
				Mode:              InterruptModeAuto,
				CaptureDurationMs: 50,
				SemanticCheck:     false, // Rely on local detection
			}

			audioConfig := DefaultAudioConfig()
			detector := NewInterruptDetector(config, audioConfig, nil)

			detector.StartCapture()
			detector.AddTranscript(bc)
			time.Sleep(60 * time.Millisecond)

			result := detector.Analyze(context.Background())

			if result != InterruptBackchannel {
				t.Errorf("Expected InterruptBackchannel for %q, got %v", bc, result)
			}
		})
	}
}

func TestInterruptDetector_Analyze_ModeAuto_RealInterrupt(t *testing.T) {
	config := InterruptConfig{
		Mode:              InterruptModeAuto,
		CaptureDurationMs: 50,
		SemanticCheck:     true,
	}

	audioConfig := DefaultAudioConfig()
	checker := &mockInterruptChecker{response: true} // Says it's a real interrupt
	detector := NewInterruptDetector(config, audioConfig, checker)

	detector.StartCapture()
	detector.AddTranscript("Actually wait, I need to change that")
	time.Sleep(60 * time.Millisecond)

	result := detector.Analyze(context.Background())

	if result != InterruptReal {
		t.Errorf("Expected InterruptReal, got %v", result)
	}

	if !checker.called {
		t.Error("Expected semantic checker to be called")
	}
}

func TestInterruptDetector_Analyze_ModeAuto_SemanticSaysBackchannel(t *testing.T) {
	config := InterruptConfig{
		Mode:              InterruptModeAuto,
		CaptureDurationMs: 50,
		SemanticCheck:     true,
	}

	audioConfig := DefaultAudioConfig()
	checker := &mockInterruptChecker{response: false} // Says it's NOT an interrupt (backchannel)
	detector := NewInterruptDetector(config, audioConfig, checker)

	detector.StartCapture()
	// Something that's not a common backchannel but still might be
	detector.AddTranscript("interesting")
	time.Sleep(60 * time.Millisecond)

	result := detector.Analyze(context.Background())

	if result != InterruptBackchannel {
		t.Errorf("Expected InterruptBackchannel when semantic says no interrupt, got %v", result)
	}

	if !checker.called {
		t.Error("Expected semantic checker to be called")
	}
}

func TestInterruptDetector_Reset(t *testing.T) {
	config := InterruptConfig{
		Mode:              InterruptModeAuto,
		CaptureDurationMs: 100,
	}

	audioConfig := DefaultAudioConfig()
	detector := NewInterruptDetector(config, audioConfig, nil)

	detector.StartCapture()
	detector.AddAudio([]byte{1, 2, 3, 4})
	detector.AddTranscript("Hello")

	if !detector.IsCapturing() {
		t.Error("Expected capturing before reset")
	}

	detector.Reset()

	if detector.IsCapturing() {
		t.Error("Expected not capturing after reset")
	}
	if len(detector.GetCapturedAudio()) != 0 {
		t.Error("Expected empty audio after reset")
	}
	if detector.GetCapturedTranscript() != "" {
		t.Error("Expected empty transcript after reset")
	}
}

func TestInterruptResult_String(t *testing.T) {
	tests := []struct {
		result   InterruptResult
		expected string
	}{
		{InterruptNone, "NONE"},
		{InterruptBackchannel, "BACKCHANNEL"},
		{InterruptReal, "REAL"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.result.String() != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, tt.result.String())
			}
		})
	}
}

func TestParseInterruptCheckResponse(t *testing.T) {
	tests := []struct {
		response string
		expected bool
	}{
		{"INTERRUPT", true},
		{"interrupt", true},
		{"Interrupt", true},
		{"BACKCHANNEL", false},
		{"backchannel", false},
		{"This is an INTERRUPT.", true},
		{"This is a backchannel.", false},
		{"YES", false}, // Not the right keyword
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.response, func(t *testing.T) {
			result := ParseInterruptCheckResponse(tt.response)
			if result != tt.expected {
				t.Errorf("ParseInterruptCheckResponse(%q) = %v, expected %v", tt.response, result, tt.expected)
			}
		})
	}
}
