package live

import (
	"sync"
	"testing"
	"time"
)

func TestGracePeriodManager_Start_Enabled(t *testing.T) {
	config := GracePeriodConfig{
		Enabled:    true,
		DurationMs: 100, // Short for testing
	}

	gpm := NewGracePeriodManager(config)

	expired := false
	var mu sync.Mutex

	gpm.SetCallbacks(
		func(transcript string) {
			mu.Lock()
			expired = true
			mu.Unlock()
		},
		nil,
		nil,
	)

	gpm.Start("Hello")

	if !gpm.IsActive() {
		t.Error("Expected grace period to be active")
	}

	// Wait for expiry
	time.Sleep(120 * time.Millisecond)

	mu.Lock()
	wasExpired := expired
	mu.Unlock()

	if !wasExpired {
		t.Error("Expected grace period to expire")
	}

	if gpm.IsActive() {
		t.Error("Expected grace period to be inactive after expiry")
	}
}

func TestGracePeriodManager_Start_Disabled(t *testing.T) {
	config := GracePeriodConfig{
		Enabled:    false,
		DurationMs: 100,
	}

	gpm := NewGracePeriodManager(config)

	expired := false
	var mu sync.Mutex

	gpm.SetCallbacks(
		func(transcript string) {
			mu.Lock()
			expired = true
			mu.Unlock()
		},
		nil,
		nil,
	)

	gpm.Start("Hello")

	// Give callback time to execute
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	wasExpired := expired
	mu.Unlock()

	if !wasExpired {
		t.Error("Expected immediate expiry when disabled")
	}

	if gpm.IsActive() {
		t.Error("Expected grace period to be inactive when disabled")
	}
}

func TestGracePeriodManager_HandleUserSpeech(t *testing.T) {
	config := GracePeriodConfig{
		Enabled:    true,
		DurationMs: 500, // Long enough to not expire during test
	}

	gpm := NewGracePeriodManager(config)

	var combinedTranscript string
	var mu sync.Mutex

	gpm.SetCallbacks(
		nil,
		func(combined string) {
			mu.Lock()
			combinedTranscript = combined
			mu.Unlock()
		},
		nil,
	)

	gpm.Start("Hello.")

	// Simulate user continuing to speak
	handled := gpm.HandleUserSpeech("How are you?")

	if !handled {
		t.Error("Expected HandleUserSpeech to return true when grace period is active")
	}

	// Give callback time to execute
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	result := combinedTranscript
	mu.Unlock()

	expected := "Hello. How are you?"
	if result != expected {
		t.Errorf("Expected combined transcript %q, got %q", expected, result)
	}

	if gpm.IsActive() {
		t.Error("Expected grace period to be inactive after user speech")
	}
}

func TestGracePeriodManager_HandleUserSpeech_NotActive(t *testing.T) {
	config := GracePeriodConfig{
		Enabled:    true,
		DurationMs: 100,
	}

	gpm := NewGracePeriodManager(config)

	// Don't start grace period

	handled := gpm.HandleUserSpeech("Hello")

	if handled {
		t.Error("Expected HandleUserSpeech to return false when grace period is not active")
	}
}

func TestGracePeriodManager_Cancel(t *testing.T) {
	config := GracePeriodConfig{
		Enabled:    true,
		DurationMs: 500,
	}

	gpm := NewGracePeriodManager(config)

	expired := false
	var mu sync.Mutex

	gpm.SetCallbacks(
		func(transcript string) {
			mu.Lock()
			expired = true
			mu.Unlock()
		},
		nil,
		nil,
	)

	gpm.Start("Hello")

	if !gpm.IsActive() {
		t.Error("Expected grace period to be active")
	}

	gpm.Cancel()

	if gpm.IsActive() {
		t.Error("Expected grace period to be inactive after cancel")
	}

	// Wait a bit to ensure expiry callback is NOT called
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	wasExpired := expired
	mu.Unlock()

	if wasExpired {
		t.Error("Expected expiry callback NOT to be called after cancel")
	}
}

func TestGracePeriodManager_TimeRemaining(t *testing.T) {
	config := GracePeriodConfig{
		Enabled:    true,
		DurationMs: 500,
	}

	gpm := NewGracePeriodManager(config)

	// Before start
	if gpm.TimeRemaining() != 0 {
		t.Error("Expected TimeRemaining to be 0 before start")
	}

	gpm.Start("Hello")

	remaining := gpm.TimeRemaining()
	if remaining < 400*time.Millisecond || remaining > 500*time.Millisecond {
		t.Errorf("Expected TimeRemaining between 400-500ms, got %v", remaining)
	}

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	remaining = gpm.TimeRemaining()
	if remaining < 300*time.Millisecond || remaining > 400*time.Millisecond {
		t.Errorf("Expected TimeRemaining between 300-400ms after 100ms wait, got %v", remaining)
	}
}

func TestGracePeriodManager_ExpiresAt(t *testing.T) {
	config := GracePeriodConfig{
		Enabled:    true,
		DurationMs: 500,
	}

	gpm := NewGracePeriodManager(config)

	before := time.Now()
	gpm.Start("Hello")
	after := time.Now()

	expiresAt := gpm.ExpiresAt()

	expectedMin := before.Add(500 * time.Millisecond)
	expectedMax := after.Add(500 * time.Millisecond)

	if expiresAt.Before(expectedMin) || expiresAt.After(expectedMax) {
		t.Errorf("ExpiresAt %v not in expected range [%v, %v]", expiresAt, expectedMin, expectedMax)
	}
}

func TestGracePeriodManager_CombineTranscripts(t *testing.T) {
	tests := []struct {
		name     string
		original string
		new      string
		expected string
	}{
		{
			name:     "Both without trailing/leading space",
			original: "Hello.",
			new:      "How are you?",
			expected: "Hello. How are you?",
		},
		{
			name:     "Original with trailing space",
			original: "Hello. ",
			new:      "How are you?",
			expected: "Hello. How are you?",
		},
		{
			name:     "New with leading space - gets trimmed",
			original: "Hello.",
			new:      " How are you?",
			expected: "Hello. How are you?", // New transcript is TrimSpace'd
		},
		{
			name:     "Both have space - trailing preserved, leading trimmed",
			original: "Hello. ",
			new:      " How are you?",
			expected: "Hello. How are you?", // Original trailing + TrimSpace(new)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := GracePeriodConfig{
				Enabled:    true,
				DurationMs: 500,
			}

			gpm := NewGracePeriodManager(config)

			var combined string
			gpm.SetCallbacks(nil, func(c string) { combined = c }, nil)

			gpm.Start(tt.original)
			gpm.HandleUserSpeech(tt.new)

			time.Sleep(10 * time.Millisecond)

			if combined != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, combined)
			}
		})
	}
}
