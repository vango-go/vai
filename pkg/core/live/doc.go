// Package live implements real-time bidirectional voice conversations for Vango AI.
//
// Live mode is conceptually "RunStream with ears (STT + VAD) and a mouth (TTS)".
// The same agent definition—model, tools, system prompt—works identically across
// text, audio file, and real-time modes.
//
// # Architecture
//
// The live package provides several core components:
//
//   - Session: The main orchestrator that coordinates the full pipeline
//   - HybridVAD: Determines when user turn is complete using energy + semantic analysis
//   - GracePeriodManager: Allows users to continue speaking after VAD commits
//   - InterruptDetector: Distinguishes real interrupts from backchannels
//   - AudioBuffer: Accumulates PCM chunks and tracks energy levels
//   - TTSPipeline: Converts text stream to audio with pause/resume support
//
// # Data Flow
//
//	Audio In → STT → Hybrid VAD → Grace Period → Agent Loop (RunStream)
//	                      │              │              │
//	                      └──── Semantic Check ────────┘
//
//	Audio Out ← TTS Pipeline ← Text Stream ← RunStream
//	                 │
//	                 └── Pause/Resume (Interrupt Detection)
//
// # State Machine
//
// The session progresses through these states:
//
//	CONFIGURING → LISTENING → GRACE_PERIOD → PROCESSING → SPEAKING
//	                  ↑                                        │
//	                  └── INTERRUPT_CAPTURING ←────────────────┘
//
// # Usage
//
// The live package is used by both the SDK (direct mode) and the proxy
// (WebSocket mode), ensuring no duplicate code between deployment modes.
//
//	// Create session configuration
//	cfg := &live.SessionConfig{
//	    Model:  "anthropic/claude-haiku-4-5-20251001",
//	    System: "You are a helpful voice assistant.",
//	    VAD: live.VADConfig{
//	        EnergyThreshold:   0.02,
//	        SilenceDurationMs: 600,
//	        SemanticCheck:     true,
//	    },
//	    GracePeriod: live.GracePeriodConfig{
//	        Enabled:    true,
//	        DurationMs: 5000,
//	    },
//	}
//
//	// Create and start session
//	session := live.NewSession(cfg, engine, voicePipeline)
//	session.Start(ctx)
//
//	// Send audio chunks
//	session.SendAudio(pcmData)
//
//	// Receive events
//	for event := range session.Events() {
//	    switch e := event.(type) {
//	    case *live.TranscriptDeltaEvent:
//	        fmt.Println("User said:", e.Text)
//	    case *live.AudioDeltaEvent:
//	        playAudio(e.Data)
//	    }
//	}
package live
