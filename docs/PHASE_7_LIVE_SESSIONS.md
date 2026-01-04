This is a great refinement. The Grace Period handles the common case where users pause mid-thought, and the clarified interrupt flow is more robust. Let me update the spec with these additions.

---

# Phase 7: Live Sessions — Design Specification

**Version:** 2.2 (Universal Pipeline Architecture with Semantic Interrupt + Grace Period)
**Status:** Ready for Implementation
**Dependencies:** Phase 4 (Voice Pipeline), Phase 5 (Streaming), Core RunStream Implementation

---

## 1. Executive Summary

Phase 7 implements real-time bidirectional voice conversations for Vango. Rather than wrapping proprietary realtime APIs (OpenAI Realtime, Gemini Live), we implement a **Universal Voice Pipeline** that works with any text-based LLM.

**Core Principle:** Live mode is RunStream with ears (STT + VAD) and a mouth (TTS). The same agent definition—model, tools, system prompt—works identically across text, audio file, and real-time modes.

**Key Capabilities:**
- Real-time voice conversations with any LLM (Claude, GPT-4, Llama, Mistral)
- Intelligent turn-taking via hybrid VAD (energy detection + semantic analysis)
- **User Grace Period**: 5-second window to continue speaking after VAD commits
- Smart interruption handling with semantic barge-in detection
- Mid-session model and tool swapping
- Unified API: same config works for HTTP and WebSocket
- Tool execution works automatically (reuses RunStream agent loop)

---

## 2. Architecture Overview

### 2.1 High-Level Data Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              LIVE SESSION                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   USER AUDIO INPUT                                                          │
│   ════════════════                                                          │
│                                                                             │
│   ┌──────────┐    ┌──────────────┐    ┌─────────────────────────────────┐  │
│   │ Audio In │───▶│ STT Provider │───▶│         Hybrid VAD              │  │
│   │  (PCM)   │    │  (Cartesia)  │    │                                 │  │
│   └──────────┘    └──────────────┘    │  ┌─────────┐    ┌───────────┐  │  │
│                                        │  │ Energy  │───▶│ Semantic  │  │  │
│                                        │  │ Check   │    │ Check     │  │  │
│                                        │  │ (Local) │    │ (Fast LLM)│  │  │
│                                        │  └─────────┘    └─────────┬─┘  │  │
│                                        └────────────────────────────┼───┘  │
│                                                                     │      │
│                                                    Turn Complete? ──┘      │
│                                                          │                 │
│                                                         YES                │
│                                                          │                 │
│   GRACE PERIOD (5000ms)                                  ▼                 │
│   ═════════════════════                   ┌─────────────────────────────┐  │
│                                           │   Start Agent Processing    │  │
│   User speaks again within 5s?            │   (Grace Period Active)     │  │
│   ├── YES: Cancel agent, append text,     └──────────────┬──────────────┘  │
│   │        re-run VAD on combined input                  │                 │
│   │                                                      │                 │
│   └── NO: Grace period expires,                          ▼                 │
│           continue normally              ┌─────────────────────────────┐   │
│                                          │     RunStream (Agent)       │   │
│                                          └──────────────┬──────────────┘   │
│                                                         │                  │
│   AGENT PROCESSING                                      ▼                  │
│   ════════════════                       ┌─────────────────────────────┐   │
│                                          │      TTS Pipeline           │   │
│                                          │      (Cartesia)             │   │
│   ┌──────────┐    ┌──────────────┐      │                             │   │
│   │ Audio Out│◀───│ Audio Buffer │◀─────│  • Streaming synthesis      │   │
│   │  (PCM)   │    │              │      │  • Sentence buffering       │   │
│   └──────────┘    └──────────────┘      │  • Pausable/Cancelable      │   │
│                                          └─────────────────────────────┘   │
│                                                                             │
│   INTERRUPT DETECTION (During Bot Speech, After Grace Period)              │
│   ═══════════════════════════════════════════════════════════              │
│                                                                             │
│   ┌──────────────────────────────────────────────────────────────────────┐ │
│   │                                                                      │ │
│   │  1. Audio Detected → Immediately pause TTS                          │ │
│   │                                                                      │ │
│   │  2. Wait 600ms, capture transcription                               │ │
│   │     ├── No transcription (noise) → Resume TTS                       │ │
│   │     └── Has transcription → Continue to step 3                      │ │
│   │                                                                      │ │
│   │  3. Semantic check: "Is user interrupting?"                         │ │
│   │     ├── NO → Resume TTS (can be interrupted again)                  │ │
│   │     └── YES → Cancel agent, truncate assistant message,             │ │
│   │               enter VAD mode to capture full user input             │ │
│   │                                                                      │ │
│   └──────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Component Responsibilities

| Component | Responsibility | Location |
|-----------|---------------|----------|
| LiveSession | Orchestrates the full pipeline, manages state | `pkg/core/live/session.go` |
| HybridVAD | Determines when user turn is complete | `pkg/core/live/vad.go` |
| GracePeriodManager | Tracks grace period window, handles continuation | `pkg/core/live/grace_period.go` |
| InterruptDetector | Determines if user speech is a real interrupt | `pkg/core/live/interrupt.go` |
| AudioBuffer | Accumulates PCM chunks, tracks energy levels | `pkg/core/live/buffer.go` |
| RunStream | Executes agent logic (existing, enhanced with Interrupt) | `pkg/core/stream.go` |
| TTSPipeline | Converts text stream to audio stream, supports pause/resume | `pkg/core/live/tts_pipeline.go` |
| Protocol | WebSocket frame definitions and parsing | `pkg/core/live/protocol.go` |

### 2.3 Deployment Modes

The same code runs in two contexts:

**Direct Mode (Go SDK)**
```
┌─────────────────┐     ┌─────────────────┐
│  Go Application │────▶│  pkg/core/live  │────▶ STT/LLM/TTS APIs
└─────────────────┘     └─────────────────┘
        │
        └── Zero network hops for orchestration
```

**Proxy Mode (Any Language)**
```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│ Python/JS/Curl  │────▶│   Vango Proxy   │────▶│  pkg/core/live  │──▶ APIs
└─────────────────┘     └─────────────────┘     └─────────────────┘
        │                       │
        └── WebSocket ──────────┘
```

---

## 3. Core Concepts

### 3.1 User Grace Period

**Problem:** Users often pause mid-thought. A simple VAD might commit the turn too early:

```
User: "Book me a flight to..." [pause, thinking] "...Paris"
         ↑
         VAD commits here, sends incomplete request
```

**Solution:** After VAD commits and the agent starts processing, we maintain a **grace period** (default 5000ms) during which if the user speaks again, we:

1. Cancel the current agent request
2. Append the new speech to the original transcript
3. Re-run VAD on the combined input
4. Continue as if the original commit never happened

**Timeline Example:**

```
0ms      User: "Hello."
600ms    [Silence detected, VAD timer starts]
1200ms   [VAD timer expires, semantic check: "Hello." → YES]
1200ms   [Send to main LLM, START GRACE PERIOD]
1500ms   User: "How are you?"
         [GRACE PERIOD TRIGGERED]
         [Cancel main LLM request]
         [Combined transcript: "Hello. How are you?"]
         [Re-run VAD semantic check: "Hello. How are you?" → YES]
         [Send to main LLM: "Hello. How are you?"]
         [New grace period starts...]
6200ms   [Grace period expires without new speech]
         [Continue agent processing normally]
```

**After Grace Period:** Once the grace period expires, any user speech triggers the **interrupt flow** instead.

### 3.2 Interrupt Flow (Post-Grace Period)

When the bot is speaking and the user makes a sound **after** the grace period has expired:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         INTERRUPT DETECTION FLOW                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. AUDIO DETECTED                                                          │
│     └── Immediately pause TTS output (don't cancel yet)                    │
│     └── Emit: interrupt.detecting                                          │
│                                                                             │
│  2. CAPTURE WINDOW (600ms)                                                  │
│     └── Accumulate audio, attempt transcription                            │
│     └── Purpose: Capture "Actually wait—" not just "Act—"                  │
│                                                                             │
│  3. TRANSCRIPTION CHECK                                                     │
│     ├── No transcription (noise, cough, etc.)                              │
│     │   └── Resume TTS from pause point                                    │
│     │   └── Emit: interrupt.dismissed {reason: "no_speech"}                │
│     │   └── Return to normal speaking state (can be interrupted again)     │
│     │                                                                       │
│     └── Has transcription                                                   │
│         └── Continue to semantic check                                      │
│                                                                             │
│  4. SEMANTIC CHECK (~100-150ms)                                             │
│     └── Ask fast LLM: "Is '{transcript}' an interruption?"                 │
│                                                                             │
│     ├── NO (backchannel: "uh huh", "right", "okay")                        │
│     │   └── Resume TTS from pause point                                    │
│     │   └── Emit: interrupt.dismissed {reason: "backchannel"}              │
│     │   └── Return to normal speaking state (can be interrupted again)     │
│     │                                                                       │
│     └── YES (real interrupt: "wait", "actually", "stop")                   │
│         └── Cancel TTS completely                                          │
│         └── Cancel RunStream                                               │
│         └── Truncate assistant message at interruption point               │
│         └── Emit: response.interrupted                                     │
│         └── Enter VAD MODE to capture full user input                      │
│         └── When VAD commits → Send new user message to agent              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.3 State Transitions

```
                         ┌─────────────────┐
                         │                 │
                         │   CONFIGURING   │
                         │                 │
                         └────────┬────────┘
                                  │ session.configure
                                  ▼
                         ┌─────────────────┐
              ┌──────────│                 │◀───────────────────────────────┐
              │          │    LISTENING    │                                │
              │          │   (VAD Active)  │                                │
              │          └────────┬────────┘                                │
              │                   │ VAD commits (semantic: YES)             │
              │                   ▼                                         │
              │          ┌─────────────────┐                                │
              │          │                 │                                │
              │          │  GRACE_PERIOD   │──── User speaks ───┐          │
              │          │   (5000ms)      │                    │          │
              │          └────────┬────────┘                    │          │
              │                   │                             │          │
              │                   │ Grace expires               ▼          │
              │                   │                    ┌────────────────┐  │
              │                   │                    │ Cancel agent,  │  │
              │                   │                    │ append text,   │  │
              │                   │                    │ re-run VAD     │──┘
              │                   │                    └────────────────┘
              │                   ▼
              │          ┌─────────────────┐
              │          │                 │
              │          │   PROCESSING    │─────────────────────┐
              │          │                 │                     │
              │          └────────┬────────┘                     │
              │                   │ Text generated               │ Response done
              │                   ▼                              │
              │          ┌─────────────────┐                     │
              │          │                 │                     │
              │          │    SPEAKING     │─────────────────────┤
              │          │                 │ TTS complete        │
              │          └────────┬────────┘                     │
              │                   │                              │
              │     Audio detected│                              │
              │                   ▼                              │
              │          ┌─────────────────┐                     │
              │          │   INTERRUPT_    │                     │
              │          │   CAPTURING     │                     │
              │          │   (600ms)       │                     │
              │          └────────┬────────┘                     │
              │                   │                              │
              │     ┌─────────────┼─────────────┐               │
              │     │             │             │               │
              │     ▼             ▼             ▼               │
              │  No speech    Backchannel   Real interrupt      │
              │     │             │             │               │
              │     └─────────────┴──────┬──────┘               │
              │                          │                      │
              │              Resume TTS  │  Cancel & VAD mode   │
              │                   │      │         │            │
              │                   ▼      │         ▼            │
              │              SPEAKING ◀──┘    LISTENING ────────┘
              │                   │
              └───────────────────┘
```

---

## 4. API Design

### 4.1 The Portable Configuration Principle

A single JSON configuration object defines an agent and works identically across all endpoints.

```json
{
  "model": "anthropic/claude-sonnet-4-20250514",
  "system": "You are a helpful voice assistant for a travel agency.",
  "messages": [],
  "tools": [
    {
      "name": "search_flights",
      "description": "Search for available flights",
      "input_schema": {
        "type": "object",
        "properties": {
          "origin": { "type": "string" },
          "destination": { "type": "string" },
          "date": { "type": "string", "format": "date" }
        },
        "required": ["origin", "destination", "date"]
      }
    }
  ],
  "voice": {
    "input": {
      "provider": "cartesia",
      "language": "en"
    },
    "output": {
      "provider": "cartesia",
      "voice": "a0e99841-438c-4a64-b679-ae501e7d6091",
      "speed": 1.0
    },
    "vad": {
      "model": "cerebras/llama-3.1-8b",
      "energy_threshold": 0.02,
      "silence_duration_ms": 600,
      "semantic_check": true
    },
    "grace_period": {
      "enabled": true,
      "duration_ms": 5000
    },
    "interrupt": {
      "mode": "auto",
      "energy_threshold": 0.05,
      "capture_duration_ms": 600,
      "semantic_check": true,
      "semantic_model": "cerebras/llama-3.1-8b",
      "save_partial": "marked"
    }
  },
  "stream": true
}
```

### 4.2 HTTP Endpoint: Discrete Requests

**`POST /v1/messages`**

Handles text and audio file inputs. For audio, the pipeline transcribes → processes → synthesizes in a single request/response cycle.

**Request (Audio Input):**
```json
{
  "model": "anthropic/claude-sonnet-4-20250514",
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "audio",
          "source": {
            "type": "base64",
            "media_type": "audio/wav",
            "data": "<BASE64_ENCODED_AUDIO>"
          }
        }
      ]
    }
  ],
  "voice": {
    "input": { "provider": "cartesia" },
    "output": { "provider": "cartesia", "voice": "sonic-english" }
  },
  "stream": true
}
```

**Response (SSE Stream):**
```
event: message_start
data: {"type":"message_start","message":{"id":"msg_01X..."}}

event: transcript
data: {"type":"transcript","text":"Book me a flight to Paris"}

event: content_block_delta
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"I'd be happy"}}

event: audio_delta
data: {"type":"audio_delta","data":"<BASE64_PCM_CHUNK>"}

event: message_stop
data: {"type":"message_stop"}
```

### 4.3 WebSocket Endpoint: Continuous Sessions

**`WS /v1/messages/live`**

Maintains a persistent bidirectional connection for real-time voice interaction.

#### Client → Server Messages

**session.configure** (Required first message)
```json
{
  "type": "session.configure",
  "config": {
    "model": "anthropic/claude-sonnet-4-20250514",
    "system": "...",
    "tools": [...],
    "voice": {...}
  }
}
```

**session.update** (Change config mid-session)
```json
{
  "type": "session.update",
  "config": {
    "model": "openai/gpt-4o",
    "voice": {
      "output": { "voice": "different-voice-id" }
    }
  }
}
```

**input.interrupt** (Force interrupt, skip semantic check)
```json
{
  "type": "input.interrupt",
  "transcript": "Actually, never mind."
}
```

**input.commit** (Force end-of-turn, e.g., push-to-talk release)
```json
{
  "type": "input.commit"
}
```

**Binary Frames**
Raw PCM audio: 16-bit signed integer, configurable sample rate (16kHz/24kHz/48kHz), mono.

#### Server → Client Messages

**session.created**
```json
{
  "type": "session.created",
  "session_id": "live_01abc...",
  "config": {
    "model": "anthropic/claude-sonnet-4-20250514",
    "sample_rate": 24000,
    "channels": 1
  }
}
```

**vad.listening** / **vad.analyzing** / **vad.silence**
```json
{
  "type": "vad.listening"
}
```

**input.committed**
```json
{
  "type": "input.committed",
  "transcript": "Book me a flight to Paris tomorrow"
}
```

**grace_period.started**
```json
{
  "type": "grace_period.started",
  "transcript": "Hello.",
  "duration_ms": 5000,
  "expires_at": "2025-01-01T12:00:05.000Z"
}
```

**grace_period.extended** (User spoke, restarting)
```json
{
  "type": "grace_period.extended",
  "previous_transcript": "Hello.",
  "new_transcript": "Hello. How are you?",
  "duration_ms": 5000,
  "expires_at": "2025-01-01T12:00:08.500Z"
}
```

**grace_period.expired**
```json
{
  "type": "grace_period.expired",
  "transcript": "Hello. How are you?"
}
```

**transcript.delta** (Real-time transcription as user speaks)
```json
{
  "type": "transcript.delta",
  "delta": "Book me a"
}
```

**content_block_start** / **content_block_delta** / **content_block_stop**
Standard Vango streaming events, identical to HTTP streaming.

**tool_use** / **tool_result**
Standard tool events from RunStream.

**audio_delta** (Binary or base64)
```json
{
  "type": "audio_delta",
  "data": "<BASE64_PCM>"
}
```

**interrupt.detecting** (TTS paused, capturing audio)
```json
{
  "type": "interrupt.detecting"
}
```

**interrupt.captured** (600ms capture complete, checking semantic)
```json
{
  "type": "interrupt.captured",
  "transcript": "uh huh"
}
```

**interrupt.dismissed** (Not a real interrupt, TTS resuming)
```json
{
  "type": "interrupt.dismissed",
  "transcript": "uh huh",
  "reason": "backchannel"
}
```

**response.interrupted** (Real interrupt confirmed)
```json
{
  "type": "response.interrupted",
  "partial_text": "I'd be happy to help you book a fli—",
  "interrupt_transcript": "Actually wait",
  "audio_position_ms": 2340
}
```

**error**
```json
{
  "type": "error",
  "code": "vad_timeout",
  "message": "No speech detected for 30 seconds"
}
```

---

## 5. Hybrid VAD System (Turn Completion)

### 5.1 Design Rationale

Simple silence detection fails in natural speech:
- "Book me a flight to..." (grammatically incomplete, user is thinking)
- "Yes." (grammatically complete, but user might continue with "Yes, and also...")
- Filler words: "um", "uh", "so..."

**Solution:** Two-stage hybrid approach with energy detection + semantic analysis.

### 5.2 VAD Pipeline

```
┌─────────────────────────────────────────────────────────────────────┐
│                         HYBRID VAD                                  │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  Stage 1: Energy Detection (Local, ~0ms)                           │
│  ═══════════════════════════════════════                           │
│                                                                     │
│  Audio ──▶ RMS Energy ──▶ Below Threshold? ──▶ No ──▶ Keep Listening│
│                                  │                                  │
│                                 Yes                                 │
│                                  │                                  │
│                                  ▼                                  │
│            Start Silence Timer (configurable, default 600ms)        │
│                                  │                                  │
│                                  ▼                                  │
│                     Timer Expired Without Speech?                   │
│                                  │                                  │
│                                 Yes                                 │
│                                  │                                  │
│  Stage 2: Semantic Check (any LLM based on supported providers)     │
│  ═══════════════════════════════════════════════════               │
│                                  │                                  │
│                                  ▼                                  │
│  ┌───────────────────────────────────────────────────────────────┐ │
│  │ Prompt: "Voice transcript: '{text}'                           │ │
│  │                                                                │ │
│  │ Is the speaker clearly done and waiting for a response?       │ │
│  │ Consider: trailing conjunctions, incomplete thoughts,         │ │
│  │ rhetorical pauses, filler words.                              │ │
│  │                                                                │ │
│  │ Reply only: YES or NO"                                        │ │
│  └───────────────────────────────────────────────────────────────┘ │
│                                  │                                  │
│                                  ▼                                  │
│                        Response == "YES"?                          │
│                           │           │                             │
│                          Yes          No                            │
│                           │           │                             │
│                           ▼           ▼                             │
│                  COMMIT TURN &     Extend silence                   │
│                  START GRACE       timer, keep                      │
│                  PERIOD            listening                        │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### 5.3 VAD Configuration

```go
type VADConfig struct {
    // The fast LLM for semantic checking
    Model string `json:"model"` // default: "cerebras/llama-3.1-8b"
    
    // Energy-based detection
    EnergyThreshold   float64 `json:"energy_threshold"`    // default: 0.02 (RMS)
    SilenceDurationMs int     `json:"silence_duration_ms"` // default: 600
    
    // Semantic checking
    SemanticCheck bool `json:"semantic_check"` // default: true
    
    // Behavior tuning
    MinWordsForCheck int `json:"min_words_for_check"` // default: 2
    MaxSilenceMs     int `json:"max_silence_ms"`      // default: 3000 (force commit)
}
```

### 5.4 VAD Implementation

```go
// pkg/core/live/vad.go

type HybridVAD struct {
    config     VADConfig
    fastClient *Engine
    
    mu              sync.Mutex
    transcript      strings.Builder
    silenceStart    time.Time
    isInSilence     bool
    lastEnergyLevel float64
}

type VADResult int

const (
    VADContinue VADResult = iota
    VADCommit
)

func (v *HybridVAD) ProcessAudio(chunk []byte, transcriptDelta string) VADResult {
    v.mu.Lock()
    defer v.mu.Unlock()
    
    if transcriptDelta != "" {
        v.transcript.WriteString(transcriptDelta)
        v.isInSilence = false
        v.silenceStart = time.Time{}
    }
    
    energy := calculateRMSEnergy(chunk)
    v.lastEnergyLevel = energy
    
    if energy < v.config.EnergyThreshold {
        if !v.isInSilence {
            v.isInSilence = true
            v.silenceStart = time.Now()
        }
        
        silenceDuration := time.Since(v.silenceStart)
        
        if silenceDuration > time.Duration(v.config.MaxSilenceMs)*time.Millisecond {
            return VADCommit
        }
        
        if silenceDuration > time.Duration(v.config.SilenceDurationMs)*time.Millisecond {
            if v.config.SemanticCheck {
                return v.doSemanticCheck()
            }
            return VADCommit
        }
    } else {
        v.isInSilence = false
    }
    
    return VADContinue
}

func (v *HybridVAD) doSemanticCheck() VADResult {
    text := v.transcript.String()
    
    words := strings.Fields(text)
    if len(words) < v.config.MinWordsForCheck {
        return VADContinue
    }
    
    ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
    defer cancel()
    
    resp, err := v.fastClient.Messages.Create(ctx, &MessageRequest{
        Model: v.config.Model,
        Messages: []Message{
            {
                Role: "user",
                Content: Text(fmt.Sprintf(
                    `Voice transcript: "%s"

Is the speaker clearly done and waiting for a response? Consider: trailing conjunctions (and, but, so), incomplete thoughts, rhetorical pauses, or filler words.

Reply only: YES or NO`,
                    text,
                )),
            },
        },
        MaxTokens:   5,
        Temperature: Float64(0.0),
    })
    
    if err != nil {
        return VADCommit
    }
    
    result := strings.ToUpper(strings.TrimSpace(resp.TextContent()))
    if strings.Contains(result, "YES") {
        return VADCommit
    }
    
    v.silenceStart = time.Now()
    return VADContinue
}

func (v *HybridVAD) Reset() {
    v.mu.Lock()
    defer v.mu.Unlock()
    v.transcript.Reset()
    v.isInSilence = false
    v.silenceStart = time.Time{}
}

func (v *HybridVAD) GetTranscript() string {
    v.mu.Lock()
    defer v.mu.Unlock()
    return v.transcript.String()
}

// SetTranscript allows setting initial transcript (for grace period continuation)
func (v *HybridVAD) SetTranscript(text string) {
    v.mu.Lock()
    defer v.mu.Unlock()
    v.transcript.Reset()
    v.transcript.WriteString(text)
}

func calculateRMSEnergy(pcm []byte) float64 {
    samples := len(pcm) / 2
    if samples == 0 {
        return 0
    }
    
    var sum float64
    for i := 0; i < len(pcm)-1; i += 2 {
        sample := int16(pcm[i]) | int16(pcm[i+1])<<8
        normalized := float64(sample) / 32768.0
        sum += normalized * normalized
    }
    
    return math.Sqrt(sum / float64(samples))
}
```

---

## 6. User Grace Period

### 6.1 Design Rationale

Even with semantic VAD, users sometimes pause in ways that appear complete but aren't:

```
User: "Hello." [600ms pause, seems complete]
Bot:  [Starts generating response]
User: "How are you?" [continues their thought]
```

The grace period provides a safety window where the user can continue without triggering the interrupt flow.

### 6.2 Grace Period Configuration

```go
type GracePeriodConfig struct {
    Enabled    bool `json:"enabled"`     // default: true
    DurationMs int  `json:"duration_ms"` // default: 5000
}
```

### 6.3 Grace Period Implementation

```go
// pkg/core/live/grace_period.go

type GracePeriodManager struct {
    config           GracePeriodConfig
    
    mu               sync.Mutex
    active           bool
    startTime        time.Time
    originalTranscript string
    timer            *time.Timer
    
    // Callbacks
    onExpired        func()
    onContinuation   func(combinedTranscript string)
}

func NewGracePeriodManager(config GracePeriodConfig) *GracePeriodManager {
    return &GracePeriodManager{
        config: config,
    }
}

func (g *GracePeriodManager) Start(transcript string, onExpired func(), onContinuation func(string)) {
    g.mu.Lock()
    defer g.mu.Unlock()
    
    if !g.config.Enabled {
        // Grace period disabled, immediately "expire"
        go onExpired()
        return
    }
    
    g.active = true
    g.startTime = time.Now()
    g.originalTranscript = transcript
    g.onExpired = onExpired
    g.onContinuation = onContinuation
    
    g.timer = time.AfterFunc(
        time.Duration(g.config.DurationMs)*time.Millisecond,
        g.expire,
    )
}

func (g *GracePeriodManager) expire() {
    g.mu.Lock()
    if !g.active {
        g.mu.Unlock()
        return
    }
    g.active = false
    callback := g.onExpired
    g.mu.Unlock()
    
    if callback != nil {
        callback()
    }
}

// HandleUserSpeech is called when user audio is detected during grace period
// Returns true if grace period was active and handled the speech
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
    
    // Combine transcripts
    combined := g.originalTranscript
    if !strings.HasSuffix(combined, " ") && !strings.HasPrefix(newTranscript, " ") {
        combined += " "
    }
    combined += newTranscript
    
    // Reset grace period state
    g.active = false
    
    // Trigger continuation callback
    if g.onContinuation != nil {
        go g.onContinuation(combined)
    }
    
    return true
}

func (g *GracePeriodManager) IsActive() bool {
    g.mu.Lock()
    defer g.mu.Unlock()
    return g.active
}

func (g *GracePeriodManager) Cancel() {
    g.mu.Lock()
    defer g.mu.Unlock()
    
    if g.timer != nil {
        g.timer.Stop()
    }
    g.active = false
}

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
```

---

## 7. Semantic Interrupt Detection (Barge-In)

### 7.1 Design Rationale

When the bot is speaking and the user makes a sound (after grace period), we need to determine if it's:
- A **real interrupt**: "Actually wait—", "Stop", "No, I meant..."
- A **backchannel**: "uh huh", "mm hmm", "right", "okay"
- **Noise**: cough, background sound

### 7.2 Latency Budget

| Provider | Model | TTFT | Total Latency |
|----------|-------|------|---------------|
| **Cerebras** | Llama 3.1 8B | ~50-80ms | **~100-150ms** |
| **Groq** | Llama 3.1 8B | ~100-150ms | **~160-250ms** |

### 7.3 Interrupt Flow Detail

```
Bot speaking: "I'd be happy to help you book—"
                                           │
User makes sound                           │
                                           ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│ STEP 1: IMMEDIATE PAUSE (0ms)                                                │
│                                                                              │
│ • Pause TTS output instantly                                                 │
│ • Don't cancel generation yet (we might resume)                              │
│ • Emit: interrupt.detecting                                                  │
│ • Start 600ms capture window                                                 │
└──────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           │ 600ms
                                           ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│ STEP 2: CAPTURE COMPLETE                                                     │
│                                                                              │
│ • Attempt to transcribe captured audio                                       │
│ • Emit: interrupt.captured {transcript: "..."}                               │
│                                                                              │
│ ┌────────────────────────────────────────────────────────────────────────┐  │
│ │ IF: No transcription (noise, cough, ambient sound)                     │  │
│ │                                                                        │  │
│ │ • Resume TTS from pause point                                          │  │
│ │ • Emit: interrupt.dismissed {reason: "no_speech"}                      │  │
│ │ • Return to SPEAKING state (can be interrupted again)                  │  │
│ │ • DONE                                                                 │  │
│ └────────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│ ┌────────────────────────────────────────────────────────────────────────┐  │
│ │ IF: Has transcription                                                  │  │
│ │                                                                        │  │
│ │ • Continue to semantic check                                           │  │
│ └────────────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────────┘
                                           │
                                           │ ~100-150ms
                                           ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│ STEP 3: SEMANTIC CHECK                                                       │
│                                                                              │
│ Prompt to fast LLM:                                                          │
│ "The AI assistant is speaking. User said: '{transcript}'                     │
│  Is the user trying to interrupt? YES/NO"                                    │
│                                                                              │
│ ┌────────────────────────────────────────────────────────────────────────┐  │
│ │ IF: NO (backchannel)                                                   │  │
│ │                                                                        │  │
│ │ • Resume TTS from pause point                                          │  │
│ │ • Emit: interrupt.dismissed {reason: "backchannel", transcript: "..."}│  │
│ │ • Return to SPEAKING state (can be interrupted again)                  │  │
│ │ • DONE                                                                 │  │
│ └────────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│ ┌────────────────────────────────────────────────────────────────────────┐  │
│ │ IF: YES (real interrupt)                                               │  │
│ │                                                                        │  │
│ │ • Cancel TTS completely                                                │  │
│ │ • Cancel RunStream                                                     │  │
│ │ • Save partial assistant message (with [interrupted] marker)           │  │
│ │ • Emit: response.interrupted {partial_text: "...", transcript: "..."}  │  │
│ │ • Transition to LISTENING state with VAD                               │  │
│ │ • Pre-populate VAD with interrupt transcript                           │  │
│ │ • Continue capturing user's full input                                 │  │
│ │ • When VAD commits → process as new user turn                          │  │
│ └────────────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 7.4 Interrupt Configuration

```go
type InterruptConfig struct {
    // Mode: auto (VAD-triggered), manual (explicit only), disabled
    Mode InterruptMode `json:"mode"` // default: "auto"
    
    // Energy threshold for detecting potential interrupt
    EnergyThreshold float64 `json:"energy_threshold"` // default: 0.05
    
    // How long to capture audio before transcribing/checking
    CaptureDurationMs int `json:"capture_duration_ms"` // default: 600
    
    // Enable semantic check to distinguish interrupts from backchannels
    SemanticCheck bool `json:"semantic_check"` // default: true
    
    // Fast LLM for semantic check
    SemanticModel string `json:"semantic_model"` // default: "cerebras/llama-3.1-8b"
    
    // How to handle partial response when interrupted
    SavePartial SaveBehavior `json:"save_partial"` // default: "marked"
}

type InterruptMode string

const (
    InterruptAuto     InterruptMode = "auto"
    InterruptManual   InterruptMode = "manual"
    InterruptDisabled InterruptMode = "disabled"
)

type SaveBehavior string

const (
    SaveDiscard SaveBehavior = "discard"
    SavePartial SaveBehavior = "save"
    SaveMarked  SaveBehavior = "marked"
)
```

### 7.5 Interrupt Detector Implementation

```go
// pkg/core/live/interrupt.go

type InterruptDetector struct {
    config     InterruptConfig
    fastClient *Engine
    stt        STTProvider
    
    mu              sync.Mutex
    capturing       bool
    captureStart    time.Time
    capturedAudio   []byte
    capturedText    strings.Builder
}

type InterruptResult struct {
    HasSpeech   bool
    IsInterrupt bool
    Transcript  string
    Reason      string // "interrupt", "backchannel", "no_speech"
}

const interruptPrompt = `The AI assistant is currently speaking. The user said: "%s"

Is the user trying to interrupt and take over the conversation?

Answer YES if: stopping the assistant, changing topic, asking a new question, correcting, disagreeing, saying "wait", "stop", "actually", "no", "hold on"

Answer NO if: backchannel acknowledgment (uh huh, mm hmm, right, okay, yeah, got it), thinking sounds (um, uh), brief encouragement to continue

Reply only: YES or NO`

func NewInterruptDetector(config InterruptConfig, client *Engine, stt STTProvider) *InterruptDetector {
    return &InterruptDetector{
        config:     config,
        fastClient: client,
        stt:        stt,
    }
}

// StartCapture begins the 600ms capture window
func (d *InterruptDetector) StartCapture() {
    d.mu.Lock()
    defer d.mu.Unlock()
    
    d.capturing = true
    d.captureStart = time.Now()
    d.capturedAudio = nil
    d.capturedText.Reset()
}

// AddAudio adds audio data during capture window
func (d *InterruptDetector) AddAudio(chunk []byte) {
    d.mu.Lock()
    defer d.mu.Unlock()
    
    if d.capturing {
        d.capturedAudio = append(d.capturedAudio, chunk...)
    }
}

// AddTranscript adds real-time transcript during capture
func (d *InterruptDetector) AddTranscript(text string) {
    d.mu.Lock()
    defer d.mu.Unlock()
    
    if d.capturing && text != "" {
        d.capturedText.WriteString(text)
    }
}

// IsCaptureComplete checks if capture window has elapsed
func (d *InterruptDetector) IsCaptureComplete() bool {
    d.mu.Lock()
    defer d.mu.Unlock()
    
    if !d.capturing {
        return false
    }
    
    elapsed := time.Since(d.captureStart)
    return elapsed >= time.Duration(d.config.CaptureDurationMs)*time.Millisecond
}

// Evaluate completes capture and determines if this is a real interrupt
func (d *InterruptDetector) Evaluate(ctx context.Context) InterruptResult {
    d.mu.Lock()
    d.capturing = false
    transcript := d.capturedText.String()
    audio := d.capturedAudio
    d.mu.Unlock()
    
    // If no real-time transcript, try to transcribe the audio
    if transcript == "" && len(audio) > 0 {
        transcribed, err := d.stt.Transcribe(ctx, audio)
        if err == nil {
            transcript = transcribed
        }
    }
    
    // No speech detected
    if strings.TrimSpace(transcript) == "" {
        return InterruptResult{
            HasSpeech:   false,
            IsInterrupt: false,
            Transcript:  "",
            Reason:      "no_speech",
        }
    }
    
    // Skip semantic check if disabled
    if !d.config.SemanticCheck {
        return InterruptResult{
            HasSpeech:   true,
            IsInterrupt: true, // Assume interrupt if semantic check disabled
            Transcript:  transcript,
            Reason:      "interrupt",
        }
    }
    
    // Semantic check
    isInterrupt := d.doSemanticCheck(ctx, transcript)
    
    reason := "backchannel"
    if isInterrupt {
        reason = "interrupt"
    }
    
    return InterruptResult{
        HasSpeech:   true,
        IsInterrupt: isInterrupt,
        Transcript:  transcript,
        Reason:      reason,
    }
}

func (d *InterruptDetector) doSemanticCheck(ctx context.Context, transcript string) bool {
    checkCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
    defer cancel()
    
    resp, err := d.fastClient.Messages.Create(checkCtx, &MessageRequest{
        Model: d.config.SemanticModel,
        Messages: []Message{
            {
                Role:    "user",
                Content: Text(fmt.Sprintf(interruptPrompt, transcript)),
            },
        },
        MaxTokens:   5,
        Temperature: Float64(0.0),
    })
    
    if err != nil {
        // On error, assume interrupt (fail safe for user experience)
        return true
    }
    
    result := strings.ToUpper(strings.TrimSpace(resp.TextContent()))
    return strings.Contains(result, "YES")
}

// GetTranscript returns the captured transcript
func (d *InterruptDetector) GetTranscript() string {
    d.mu.Lock()
    defer d.mu.Unlock()
    return d.capturedText.String()
}

// Reset clears the detector state
func (d *InterruptDetector) Reset() {
    d.mu.Lock()
    defer d.mu.Unlock()
    d.capturing = false
    d.capturedAudio = nil
    d.capturedText.Reset()
}
```

---

## 8. RunStream Interrupt Enhancement

### 8.1 Enhanced RunStream Interface

```go
// pkg/core/stream.go

type RunStream struct {
    client *Client
    config *RunConfig
    
    mu             sync.RWMutex
    messages       []Message
    currentStream  *Stream
    partialContent strings.Builder
    partialToolUse *ToolUseBlock
    
    events    chan StreamEvent
    interrupt chan interruptRequest
    done      chan struct{}
    
    ctx    context.Context
    cancel context.CancelFunc
}

type interruptRequest struct {
    message  Message
    behavior SaveBehavior
    response chan error
}

func (rs *RunStream) Interrupt(msg Message, behavior SaveBehavior) error {
    req := interruptRequest{
        message:  msg,
        behavior: behavior,
        response: make(chan error, 1),
    }
    
    select {
    case rs.interrupt <- req:
        return <-req.response
    case <-rs.done:
        return fmt.Errorf("stream closed")
    case <-time.After(5 * time.Second):
        return fmt.Errorf("interrupt timeout")
    }
}

func (rs *RunStream) InterruptWithTranscript(transcript string, behavior SaveBehavior) error {
    return rs.Interrupt(
        Message{Role: "user", Content: Text(transcript)},
        behavior,
    )
}

// Cancel stops the current stream without adding a new message
// Used for grace period continuation
func (rs *RunStream) Cancel() {
    rs.mu.Lock()
    if rs.currentStream != nil {
        rs.currentStream.Close()
    }
    rs.mu.Unlock()
}

// GetPartialContent returns the partial response generated so far
func (rs *RunStream) GetPartialContent() string {
    rs.mu.RLock()
    defer rs.mu.RUnlock()
    return rs.partialContent.String()
}
```

### 8.2 Run Loop with Interrupt Handling

```go
func (rs *RunStream) run() {
    defer close(rs.events)
    defer close(rs.done)
    
    for {
        stream, err := rs.startTurn()
        if err != nil {
            rs.sendError(err)
            return
        }
        
        turnComplete := rs.processTurn(stream)
        
        if turnComplete {
            if !rs.needsAnotherTurn() {
                return
            }
        }
    }
}

func (rs *RunStream) processTurn(stream *Stream) bool {
    rs.mu.Lock()
    rs.currentStream = stream
    rs.partialContent.Reset()
    rs.partialToolUse = nil
    rs.mu.Unlock()
    
    defer func() {
        rs.mu.Lock()
        rs.currentStream = nil
        rs.mu.Unlock()
    }()
    
    for {
        select {
        case <-rs.ctx.Done():
            stream.Close()
            return true
            
        case req := <-rs.interrupt:
            rs.handleInterrupt(stream, req)
            req.response <- nil
            return false
            
        case event, ok := <-stream.Events():
            if !ok {
                rs.finalizePartialContent()
                return true
            }
            
            rs.processEvent(event)
        }
    }
}

func (rs *RunStream) handleInterrupt(stream *Stream, req interruptRequest) {
    stream.Close()
    
    rs.mu.Lock()
    defer rs.mu.Unlock()
    
    partialText := rs.partialContent.String()
    
    switch req.behavior {
    case SaveDiscard:
        // Don't save anything
        
    case SavePartial:
        if partialText != "" {
            rs.messages = append(rs.messages, Message{
                Role:    "assistant",
                Content: Text(partialText),
            })
        }
        
    case SaveMarked:
        if partialText != "" {
            rs.messages = append(rs.messages, Message{
                Role:    "assistant",
                Content: Text(partialText + " [interrupted]"),
            })
        }
    }
    
    rs.messages = append(rs.messages, req.message)
    
    rs.events <- &ResponseInterruptedEvent{
        PartialText: partialText,
    }
}

func (rs *RunStream) processEvent(event StreamEvent) {
    switch e := event.(type) {
    case *ContentBlockDeltaEvent:
        if delta, ok := e.Delta.(*TextDelta); ok {
            rs.mu.Lock()
            rs.partialContent.WriteString(delta.Text)
            rs.mu.Unlock()
        }
        
    case *ContentBlockStartEvent:
        if tu, ok := e.ContentBlock.(*ToolUseBlock); ok {
            rs.mu.Lock()
            rs.partialToolUse = tu
            rs.mu.Unlock()
        }
    }
    
    rs.events <- event
}

func (rs *RunStream) finalizePartialContent() {
    rs.mu.Lock()
    defer rs.mu.Unlock()
    
    if rs.partialContent.Len() > 0 {
        rs.messages = append(rs.messages, Message{
            Role:    "assistant",
            Content: Text(rs.partialContent.String()),
        })
    }
}
```

---

## 9. Live Session Implementation

### 9.1 Session States

```go
type SessionState int

const (
    StateConfiguring SessionState = iota
    StateListening          // VAD active, waiting for user input
    StateGracePeriod        // VAD committed, agent started, grace period active
    StateProcessing         // Grace period expired, agent processing
    StateSpeaking           // TTS outputting audio
    StateInterruptCapturing // Audio detected during speech, capturing
    StateInterruptChecking  // Capture complete, running semantic check
    StateClosed
)
```

### 9.2 Full Session Implementation

```go
// pkg/core/live/session.go

type LiveSession struct {
    id     string
    conn   *websocket.Conn
    config *SessionConfig
    
    // Core components
    engine            *Engine
    runStream         *RunStream
    vad               *HybridVAD
    gracePeriod       *GracePeriodManager
    interruptDetector *InterruptDetector
    stt               STTStream
    tts               *TTSPipeline
    
    // State
    mu          sync.RWMutex
    state       SessionState
    messages    []Message
    
    // Tracking
    lastUserSpeechTime time.Time
    pendingTranscript  string
    
    // Audio handling
    audioBuffer *AudioBuffer
    
    // Channels
    incomingAudio  chan []byte
    outgoingEvents chan SessionEvent
    done           chan struct{}
    
    ctx    context.Context
    cancel context.CancelFunc
}

func NewLiveSession(conn *websocket.Conn, engine *Engine) *LiveSession {
    ctx, cancel := context.WithCancel(context.Background())
    
    return &LiveSession{
        id:             generateSessionID(),
        conn:           conn,
        engine:         engine,
        state:          StateConfiguring,
        incomingAudio:  make(chan []byte, 100),
        outgoingEvents: make(chan SessionEvent, 100),
        done:           make(chan struct{}),
        ctx:            ctx,
        cancel:         cancel,
    }
}

func (s *LiveSession) Configure(cfg *SessionConfig) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    if s.state != StateConfiguring && s.state != StateListening {
        return fmt.Errorf("cannot configure in state %v", s.state)
    }
    
    cfg = s.applyDefaults(cfg)
    s.config = cfg
    
    // Initialize components
    s.vad = NewHybridVAD(cfg.Voice.VAD, s.engine)
    s.gracePeriod = NewGracePeriodManager(cfg.Voice.GracePeriod)
    s.interruptDetector = NewInterruptDetector(cfg.Voice.Interrupt, s.engine, s.engine.STT)
    
    sttProvider := cfg.Voice.Input.Provider
    if sttProvider == "" {
        sttProvider = "cartesia"
    }
    s.stt = s.engine.STT.NewStream(sttProvider)
    
    s.tts = NewTTSPipeline(s.engine, cfg.Voice.Output)
    s.audioBuffer = NewAudioBuffer(cfg.Voice.SampleRate)
    
    s.state = StateListening
    
    return nil
}

func (s *LiveSession) applyDefaults(cfg *SessionConfig) *SessionConfig {
    // VAD defaults
    if cfg.Voice.VAD.Model == "" {
        cfg.Voice.VAD.Model = "cerebras/llama-3.1-8b"
    }
    if cfg.Voice.VAD.EnergyThreshold == 0 {
        cfg.Voice.VAD.EnergyThreshold = 0.02
    }
    if cfg.Voice.VAD.SilenceDurationMs == 0 {
        cfg.Voice.VAD.SilenceDurationMs = 600
    }
    if cfg.Voice.VAD.MaxSilenceMs == 0 {
        cfg.Voice.VAD.MaxSilenceMs = 3000
    }
    if cfg.Voice.VAD.MinWordsForCheck == 0 {
        cfg.Voice.VAD.MinWordsForCheck = 2
    }
    
    // Grace period defaults
    if !cfg.Voice.GracePeriod.Enabled && cfg.Voice.GracePeriod.DurationMs == 0 {
        cfg.Voice.GracePeriod.Enabled = true
    }
    if cfg.Voice.GracePeriod.DurationMs == 0 {
        cfg.Voice.GracePeriod.DurationMs = 5000
    }
    
    // Interrupt defaults
    if cfg.Voice.Interrupt.Mode == "" {
        cfg.Voice.Interrupt.Mode = InterruptAuto
    }
    if cfg.Voice.Interrupt.EnergyThreshold == 0 {
        cfg.Voice.Interrupt.EnergyThreshold = 0.05
    }
    if cfg.Voice.Interrupt.CaptureDurationMs == 0 {
        cfg.Voice.Interrupt.CaptureDurationMs = 600
    }
    if cfg.Voice.Interrupt.SemanticModel == "" {
        cfg.Voice.Interrupt.SemanticModel = "cerebras/llama-3.1-8b"
    }
    if cfg.Voice.Interrupt.SavePartial == "" {
        cfg.Voice.Interrupt.SavePartial = SaveMarked
    }
    
    return cfg
}

func (s *LiveSession) Start() {
    go s.readLoop()
    go s.writeLoop()
    go s.processLoop()
    go s.sttLoop()
}

func (s *LiveSession) processLoop() {
    defer s.cleanup()
    
    for {
        select {
        case <-s.ctx.Done():
            return
            
        case <-s.done:
            return
            
        case audioChunk := <-s.incomingAudio:
            s.handleAudioInput(audioChunk)
        }
    }
}

func (s *LiveSession) handleAudioInput(chunk []byte) {
    s.mu.Lock()
    state := s.state
    s.mu.Unlock()
    
    // Feed audio to STT
    s.stt.Write(chunk)
    transcriptDelta := s.stt.GetLatestDelta()
    
    // Track last speech time
    energy := calculateRMSEnergy(chunk)
    if energy > s.config.Voice.VAD.EnergyThreshold {
        s.mu.Lock()
        s.lastUserSpeechTime = time.Now()
        s.mu.Unlock()
    }
    
    switch state {
    case StateListening:
        s.handleListeningState(chunk, transcriptDelta)
        
    case StateGracePeriod:
        s.handleGracePeriodState(chunk, transcriptDelta, energy)
        
    case StateProcessing:
        // No special handling, agent is working
        
    case StateSpeaking:
        s.handleSpeakingState(chunk, transcriptDelta, energy)
        
    case StateInterruptCapturing:
        s.handleInterruptCapturingState(chunk, transcriptDelta)
        
    case StateInterruptChecking:
        // Wait for semantic check to complete
    }
}

func (s *LiveSession) handleListeningState(chunk []byte, transcriptDelta string) {
    result := s.vad.ProcessAudio(chunk, transcriptDelta)
    
    if result == VADCommit {
        transcript := s.vad.GetTranscript()
        s.triggerAgentWithGracePeriod(transcript)
    }
}

func (s *LiveSession) handleGracePeriodState(chunk []byte, transcriptDelta string, energy float64) {
    // Check if user is speaking
    if energy > s.config.Voice.VAD.EnergyThreshold && transcriptDelta != "" {
        // User is continuing their thought
        handled := s.gracePeriod.HandleUserSpeech(transcriptDelta)
        if handled {
            s.sendEvent(&GracePeriodExtendedEvent{
                PreviousTranscript: s.pendingTranscript,
                NewTranscript:      transcriptDelta,
            })
        }
    }
}

func (s *LiveSession) handleSpeakingState(chunk []byte, transcriptDelta string, energy float64) {
    if s.config.Voice.Interrupt.Mode != InterruptAuto {
        return
    }
    
    // Check if user is speaking
    if energy < s.config.Voice.Interrupt.EnergyThreshold {
        return
    }
    
    // Start interrupt detection
    s.mu.Lock()
    s.state = StateInterruptCapturing
    s.mu.Unlock()
    
    // Immediately pause TTS
    s.tts.Pause()
    
    // Start capture
    s.interruptDetector.StartCapture()
    s.interruptDetector.AddAudio(chunk)
    s.interruptDetector.AddTranscript(transcriptDelta)
    
    s.sendEvent(&InterruptDetectingEvent{})
}

func (s *LiveSession) handleInterruptCapturingState(chunk []byte, transcriptDelta string) {
    // Add to capture buffer
    s.interruptDetector.AddAudio(chunk)
    s.interruptDetector.AddTranscript(transcriptDelta)
    
    // Check if capture window complete
    if s.interruptDetector.IsCaptureComplete() {
        s.mu.Lock()
        s.state = StateInterruptChecking
        s.mu.Unlock()
        
        transcript := s.interruptDetector.GetTranscript()
        s.sendEvent(&InterruptCapturedEvent{
            Transcript: transcript,
        })
        
        // Run evaluation asynchronously
        go s.evaluateInterrupt()
    }
}

func (s *LiveSession) evaluateInterrupt() {
    result := s.interruptDetector.Evaluate(s.ctx)
    
    if !result.HasSpeech {
        // No speech, resume TTS
        s.resumeFromInterrupt("no_speech", "")
        return
    }
    
    if !result.IsInterrupt {
        // Backchannel, resume TTS
        s.resumeFromInterrupt(result.Reason, result.Transcript)
        return
    }
    
    // Real interrupt - cancel everything and enter VAD mode
    s.confirmInterrupt(result.Transcript)
}

func (s *LiveSession) resumeFromInterrupt(reason string, transcript string) {
    s.mu.Lock()
    s.state = StateSpeaking
    s.mu.Unlock()
    
    // Resume TTS
    resumeChunks := s.tts.Resume()
    go func() {
        for chunk := range resumeChunks {
            s.sendAudio(chunk)
        }
    }()
    
    s.interruptDetector.Reset()
    
    s.sendEvent(&InterruptDismissedEvent{
        Transcript: transcript,
        Reason:     reason,
    })
}

func (s *LiveSession) confirmInterrupt(transcript string) {
    s.mu.Lock()
    s.state = StateListening
    s.mu.Unlock()
    
    // Cancel TTS
    s.tts.Cancel()
    
    // Get partial response before canceling
    partialText := ""
    if s.runStream != nil {
        partialText = s.runStream.GetPartialContent()
        s.runStream.Cancel()
    }
    
    // Save partial assistant message to history
    if partialText != "" {
        s.mu.Lock()
        s.messages = append(s.messages, Message{
            Role:    "assistant",
            Content: Text(partialText + " [interrupted]"),
        })
        s.mu.Unlock()
    }
    
    s.interruptDetector.Reset()
    
    // Emit interrupt event
    s.sendEvent(&ResponseInterruptedEvent{
        PartialText:         partialText,
        InterruptTranscript: transcript,
    })
    
    // Set up VAD to capture full user input
    // Pre-populate with interrupt transcript
    s.vad.Reset()
    s.vad.SetTranscript(transcript)
    
    // Continue in listening state - VAD will capture rest of user input
}

func (s *LiveSession) triggerAgentWithGracePeriod(transcript string) {
    s.mu.Lock()
    s.state = StateGracePeriod
    s.pendingTranscript = transcript
    s.mu.Unlock()
    
    s.vad.Reset()
    
    // Send committed event
    s.sendEvent(&InputCommittedEvent{
        Transcript: transcript,
    })
    
    // Start grace period
    s.gracePeriod.Start(
        transcript,
        s.onGracePeriodExpired,
        s.onGracePeriodContinuation,
    )
    
    s.sendEvent(&GracePeriodStartedEvent{
        Transcript: transcript,
        DurationMs: s.config.Voice.GracePeriod.DurationMs,
    })
    
    // Start agent processing (can be canceled during grace period)
    s.startAgentProcessing(transcript)
}

func (s *LiveSession) onGracePeriodExpired() {
    s.mu.Lock()
    if s.state != StateGracePeriod {
        s.mu.Unlock()
        return
    }
    s.state = StateProcessing
    s.mu.Unlock()
    
    s.sendEvent(&GracePeriodExpiredEvent{
        Transcript: s.pendingTranscript,
    })
    
    // Agent already processing, continue normally
}

func (s *LiveSession) onGracePeriodContinuation(combinedTranscript string) {
    s.mu.Lock()
    s.state = StateListening
    s.pendingTranscript = ""
    s.mu.Unlock()
    
    // Cancel current agent request
    if s.runStream != nil {
        s.runStream.Cancel()
        s.runStream = nil
    }
    
    // Set up VAD with combined transcript
    s.vad.Reset()
    s.vad.SetTranscript(combinedTranscript)
    
    // VAD will re-evaluate and either commit again or wait for more
}

func (s *LiveSession) startAgentProcessing(transcript string) {
    // Add user message to history
    s.mu.Lock()
    s.messages = append(s.messages, Message{
        Role:    "user",
        Content: Text(transcript),
    })
    messages := make([]Message, len(s.messages))
    copy(messages, s.messages)
    s.mu.Unlock()
    
    // Create RunStream
    req := &MessageRequest{
        Model:    s.config.Model,
        System:   s.config.System,
        Tools:    s.config.Tools,
        Messages: messages,
        Stream:   true,
    }
    
    var err error
    s.runStream, err = s.engine.Messages.RunStream(s.ctx, req)
    if err != nil {
        s.sendEvent(&ErrorEvent{
            Code:    "agent_error",
            Message: err.Error(),
        })
        return
    }
    
    // Process agent response
    go s.processAgentResponse()
}

func (s *LiveSession) processAgentResponse() {
    for event := range s.runStream.Events() {
        s.mu.RLock()
        state := s.state
        s.mu.RUnlock()
        
        // Check if we were interrupted during grace period
        if state == StateListening {
            // Grace period triggered continuation, stop processing
            return
        }
        
        // Forward event to client
        s.sendEvent(event)
        
        // Handle text for TTS
        switch e := event.(type) {
        case *ContentBlockDeltaEvent:
            if delta, ok := e.Delta.(*TextDelta); ok {
                s.mu.Lock()
                if s.state == StateGracePeriod || s.state == StateProcessing {
                    s.state = StateSpeaking
                }
                s.mu.Unlock()
                
                // Feed to TTS pipeline
                audioChunks := s.tts.Synthesize(delta.Text)
                for chunk := range audioChunks {
                    s.sendAudio(chunk)
                }
            }
            
        case *MessageStopEvent:
            // Flush remaining TTS
            for chunk := range s.tts.Flush() {
                s.sendAudio(chunk)
            }
            
            // Save assistant message to history
            s.mu.Lock()
            if s.runStream != nil {
                content := s.runStream.GetPartialContent()
                if content != "" {
                    s.messages = append(s.messages, Message{
                        Role:    "assistant",
                        Content: Text(content),
                    })
                }
            }
            s.state = StateListening
            s.mu.Unlock()
            
        case *ResponseInterruptedEvent:
            // Already handled
        }
    }
}

func (s *LiveSession) UpdateConfig(cfg *SessionConfig) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    if cfg.Model != "" {
        s.config.Model = cfg.Model
    }
    
    if cfg.Tools != nil {
        s.config.Tools = cfg.Tools
    }
    
    if cfg.Voice.Output.Voice != "" {
        s.tts.UpdateVoice(cfg.Voice.Output)
    }
    
    if cfg.Voice.VAD.Model != "" {
        s.vad.UpdateConfig(cfg.Voice.VAD)
    }
    
    if cfg.Voice.GracePeriod.DurationMs != 0 {
        s.gracePeriod.UpdateConfig(cfg.Voice.GracePeriod)
    }
    
    if cfg.Voice.Interrupt.SemanticModel != "" {
        s.interruptDetector.UpdateConfig(cfg.Voice.Interrupt)
    }
    
    return nil
}

func (s *LiveSession) sendEvent(event SessionEvent) {
    select {
    case s.outgoingEvents <- event:
    case <-s.done:
    }
}

func (s *LiveSession) sendAudio(chunk []byte) {
    s.sendEvent(&AudioDeltaEvent{Data: chunk})
}

func (s *LiveSession) Close() {
    s.cancel()
    close(s.done)
    
    s.gracePeriod.Cancel()
    
    if s.runStream != nil {
        s.runStream.Close()
    }
    if s.stt != nil {
        s.stt.Close()
    }
    if s.tts != nil {
        s.tts.Close()
    }
    
    s.conn.Close()
}
```

---

## 10. TTS Pipeline with Pause/Resume

```go
// pkg/core/live/tts_pipeline.go

type TTSPipeline struct {
    engine     *Engine
    config     VoiceOutputConfig
    
    mu         sync.Mutex
    buffer     strings.Builder
    paused     bool
    cancelled  bool
    
    pausedAt      time.Time
    pendingChunks [][]byte
    
    sentenceEnders []string
}

func NewTTSPipeline(engine *Engine, config VoiceOutputConfig) *TTSPipeline {
    return &TTSPipeline{
        engine:         engine,
        config:         config,
        sentenceEnders: []string{".", "!", "?", ":", ";"},
    }
}

func (p *TTSPipeline) Synthesize(text string) <-chan []byte {
    out := make(chan []byte, 10)
    
    go func() {
        defer close(out)
        
        p.mu.Lock()
        if p.cancelled {
            p.mu.Unlock()
            return
        }
        p.buffer.WriteString(text)
        content := p.buffer.String()
        p.mu.Unlock()
        
        for _, ender := range p.sentenceEnders {
            if idx := strings.LastIndex(content, ender); idx != -1 {
                sentence := content[:idx+1]
                
                p.mu.Lock()
                p.buffer.Reset()
                p.buffer.WriteString(content[idx+1:])
                p.mu.Unlock()
                
                audio, err := p.engine.TTS.Synthesize(context.Background(), sentence, TTSOptions{
                    Provider: p.config.Provider,
                    Voice:    p.config.Voice,
                })
                if err != nil {
                    continue
                }
                
                for chunk := range audio.Chunks() {
                    p.mu.Lock()
                    cancelled := p.cancelled
                    paused := p.paused
                    p.mu.Unlock()
                    
                    if cancelled {
                        return
                    }
                    
                    if paused {
                        p.mu.Lock()
                        p.pendingChunks = append(p.pendingChunks, chunk)
                        p.mu.Unlock()
                    } else {
                        out <- chunk
                    }
                }
                
                break
            }
        }
    }()
    
    return out
}

func (p *TTSPipeline) Pause() {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.paused = true
    p.pausedAt = time.Now()
}

func (p *TTSPipeline) Resume() <-chan []byte {
    out := make(chan []byte, 10)
    
    go func() {
        defer close(out)
        
        p.mu.Lock()
        p.paused = false
        pending := p.pendingChunks
        p.pendingChunks = nil
        p.mu.Unlock()
        
        for _, chunk := range pending {
            out <- chunk
        }
    }()
    
    return out
}

func (p *TTSPipeline) Cancel() {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.cancelled = true
    p.paused = false
    p.buffer.Reset()
    p.pendingChunks = nil
}

func (p *TTSPipeline) Reset() {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.cancelled = false
    p.paused = false
    p.buffer.Reset()
    p.pendingChunks = nil
}

func (p *TTSPipeline) Flush() <-chan []byte {
    out := make(chan []byte, 10)
    
    go func() {
        defer close(out)
        
        p.mu.Lock()
        content := p.buffer.String()
        p.buffer.Reset()
        cancelled := p.cancelled
        p.mu.Unlock()
        
        if cancelled || content == "" {
            return
        }
        
        audio, err := p.engine.TTS.Synthesize(context.Background(), content, TTSOptions{
            Provider: p.config.Provider,
            Voice:    p.config.Voice,
        })
        if err != nil {
            return
        }
        
        for chunk := range audio.Chunks() {
            out <- chunk
        }
    }()
    
    return out
}

func (p *TTSPipeline) UpdateVoice(config VoiceOutputConfig) {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.config = config
}
```

---

## 11. SDK Integration

### 11.1 RunStream Options

```go
// sdk/options.go

type RunOption func(*runConfig)

type runConfig struct {
    VoiceInput   *VoiceInputConfig
    VoiceOutput  *VoiceOutputConfig
    VAD          *VADConfig
    GracePeriod  *GracePeriodConfig
    Interrupt    *InterruptConfig
    Live         bool
}

func WithVoiceOutput(voice string) RunOption {
    return func(c *runConfig) {
        c.VoiceOutput = &VoiceOutputConfig{Voice: voice}
    }
}

func WithVoiceOutputConfig(cfg VoiceOutputConfig) RunOption {
    return func(c *runConfig) {
        c.VoiceOutput = &cfg
    }
}

func WithLive() RunOption {
    return func(c *runConfig) {
        c.Live = true
    }
}

func WithVAD(cfg VADConfig) RunOption {
    return func(c *runConfig) {
        c.VAD = &cfg
    }
}

func WithGracePeriod(cfg GracePeriodConfig) RunOption {
    return func(c *runConfig) {
        c.GracePeriod = &cfg
    }
}

func WithInterrupt(cfg InterruptConfig) RunOption {
    return func(c *runConfig) {
        c.Interrupt = &cfg
    }
}
```

### 11.2 Unified RunStream Entry Point

```go
// sdk/messages.go

func (s *MessagesService) RunStream(
    ctx context.Context, 
    req *MessageRequest, 
    opts ...RunOption,
) (*Stream, error) {
    
    cfg := &runConfig{}
    for _, opt := range opts {
        opt(cfg)
    }
    
    if cfg.Live {
        return s.startLiveSession(ctx, req, cfg)
    }
    
    if req.HasAudioInput() {
        transcribed, err := s.transcribeAudioInput(ctx, req, cfg.VoiceInput)
        if err != nil {
            return nil, err
        }
        req = transcribed
    }
    
    stream := s.startAgentLoop(ctx, req)
    
    if cfg.VoiceOutput != nil {
        stream = s.attachTTSPipeline(stream, cfg.VoiceOutput)
    }
    
    return stream, nil
}

func (s *MessagesService) startLiveSession(
    ctx context.Context,
    req *MessageRequest,
    cfg *runConfig,
) (*Stream, error) {
    
    wsURL := s.client.getLiveWebSocketURL()
    
    headers := make(http.Header)
    headers.Set("Authorization", "Bearer "+s.client.apiKey)
    
    conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, headers)
    if err != nil {
        return nil, fmt.Errorf("websocket connect: %w", err)
    }
    
    voiceConfig := VoiceConfig{
        Input:  cfg.VoiceInput,
        Output: cfg.VoiceOutput,
    }
    if cfg.VAD != nil {
        voiceConfig.VAD = *cfg.VAD
    }
    if cfg.GracePeriod != nil {
        voiceConfig.GracePeriod = *cfg.GracePeriod
    }
    if cfg.Interrupt != nil {
        voiceConfig.Interrupt = *cfg.Interrupt
    }
    
    sessionConfig := &SessionConfig{
        Model:  req.Model,
        System: req.System,
        Tools:  req.Tools,
        Voice:  voiceConfig,
    }
    
    configMsg := map[string]any{
        "type":   "session.configure",
        "config": sessionConfig,
    }
    
    if err := conn.WriteJSON(configMsg); err != nil {
        conn.Close()
        return nil, err
    }
    
    stream := newLiveStream(conn)
    
    select {
    case event := <-stream.Events():
        if _, ok := event.(*SessionCreatedEvent); !ok {
            conn.Close()
            return nil, fmt.Errorf("expected session.created, got %T", event)
        }
    case <-ctx.Done():
        conn.Close()
        return nil, ctx.Err()
    }
    
    return stream, nil
}
```

---

## 12. Event Types Summary

### 12.1 Server → Client Events

| Event Type | Description | Payload |
|------------|-------------|---------|
| `session.created` | Session initialized | `session_id`, `config` |
| `vad.listening` | Ready for audio | — |
| `vad.analyzing` | Running semantic check | — |
| `vad.silence` | Silence detected | `duration_ms` |
| `input.committed` | Turn complete | `transcript` |
| `grace_period.started` | Grace period begun | `transcript`, `duration_ms`, `expires_at` |
| `grace_period.extended` | User continued, restarting | `previous_transcript`, `new_transcript` |
| `grace_period.expired` | Grace period ended normally | `transcript` |
| `transcript.delta` | Real-time transcription | `delta` |
| `content_block_start` | Response block starting | `index`, `content_block` |
| `content_block_delta` | Response content | `index`, `delta` |
| `content_block_stop` | Response block complete | `index` |
| `tool_use` | Tool call requested | `id`, `name`, `input` |
| `audio_delta` | Audio output chunk | `data` |
| `interrupt.detecting` | TTS paused, capturing audio | — |
| `interrupt.captured` | Capture complete | `transcript` |
| `interrupt.dismissed` | Not a real interrupt | `transcript`, `reason` |
| `response.interrupted` | Real interrupt confirmed | `partial_text`, `interrupt_transcript` |
| `message_stop` | Response complete | `stop_reason` |
| `error` | Error occurred | `code`, `message` |

### 12.2 Client → Server Events

| Event Type | Description | Payload |
|------------|-------------|---------|
| `session.configure` | Initial configuration | `config` |
| `session.update` | Update configuration | `config` |
| `input.interrupt` | Force interrupt | `transcript` |
| `input.commit` | Force end of turn | — |
| `input.text` | Send text directly | `text` |
| `tool_result` | Return tool result | `tool_use_id`, `content` |
| (binary frame) | Audio input | Raw PCM bytes |

---

## 13. Configuration Defaults Summary

```json
{
  "voice": {
    "input": {
      "provider": "cartesia",
      "language": "en"
    },
    "output": {
      "provider": "cartesia",
      "voice": "sonic-english",
      "speed": 1.0
    },
    "vad": {
      "model": "cerebras/llama-3.1-8b",
      "energy_threshold": 0.02,
      "silence_duration_ms": 600,
      "semantic_check": true,
      "min_words_for_check": 2,
      "max_silence_ms": 3000
    },
    "grace_period": {
      "enabled": true,
      "duration_ms": 5000
    },
    "interrupt": {
      "mode": "auto",
      "energy_threshold": 0.05,
      "capture_duration_ms": 600,
      "semantic_check": true,
      "semantic_model": "cerebras/llama-3.1-8b",
      "save_partial": "marked"
    }
  }
}
```

---

## 14. Testing Strategy

### 14.1 Grace Period Tests

```go
func TestGracePeriod_UserContinues(t *testing.T) {
    client := vai.NewClient()
    
    stream, err := client.Messages.RunStream(ctx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4-20250514",
    }, vai.WithLive(), vai.WithGracePeriod(vai.GracePeriodConfig{
        Enabled:    true,
        DurationMs: 5000,
    }))
    require.NoError(t, err)
    defer stream.Close()
    
    // Send "Hello."
    stream.SendAudio(loadTestAudio("hello.wav"))
    
    // Wait for VAD to commit
    var gotCommitted bool
    for event := range stream.Events() {
        if e, ok := event.(*vai.InputCommittedEvent); ok {
            assert.Equal(t, "Hello.", e.Transcript)
            gotCommitted = true
            break
        }
    }
    require.True(t, gotCommitted)
    
    // Within grace period, send more audio
    time.Sleep(1 * time.Second) // Still within 5s grace
    stream.SendAudio(loadTestAudio("how_are_you.wav"))
    
    // Should get grace_period.extended, not interrupt
    var gotExtended bool
    for event := range stream.Events() {
        switch e := event.(type) {
        case *vai.GracePeriodExtendedEvent:
            assert.Equal(t, "Hello.", e.PreviousTranscript)
            assert.Contains(t, e.NewTranscript, "How are you")
            gotExtended = true
        case *vai.ResponseInterruptedEvent:
            t.Fatal("should not get interrupt during grace period")
        case *vai.InputCommittedEvent:
            // New commit with combined transcript
            if gotExtended {
                assert.Contains(t, e.Transcript, "Hello")
                assert.Contains(t, e.Transcript, "How are you")
                return
            }
        }
    }
}

func TestGracePeriod_Expires(t *testing.T) {
    client := vai.NewClient()
    
    stream, err := client.Messages.RunStream(ctx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4-20250514",
    }, vai.WithLive(), vai.WithGracePeriod(vai.GracePeriodConfig{
        Enabled:    true,
        DurationMs: 1000, // Short for testing
    }))
    require.NoError(t, err)
    defer stream.Close()
    
    stream.SendAudio(loadTestAudio("hello.wav"))
    
    var gotExpired bool
    timeout := time.After(5 * time.Second)
    for {
        select {
        case event := <-stream.Events():
            if _, ok := event.(*vai.GracePeriodExpiredEvent); ok {
                gotExpired = true
            }
            if _, ok := event.(*vai.AudioDeltaEvent); ok && gotExpired {
                // Audio started after grace period expired
                return
            }
        case <-timeout:
            t.Fatal("timeout")
        }
    }
}
```

### 14.2 Interrupt Tests

```go
func TestInterrupt_Backchannel_Dismissed(t *testing.T) {
    client := vai.NewClient()
    
    stream, err := client.Messages.RunStream(ctx, &vai.MessageRequest{
        Model:  "anthropic/claude-sonnet-4-20250514",
        System: "Give long detailed answers.",
    }, vai.WithLive())
    require.NoError(t, err)
    defer stream.Close()
    
    stream.SendText("Tell me about the history of computers")
    
    // Wait for bot to start speaking
    time.Sleep(3 * time.Second)
    
    // Send backchannel
    stream.SendAudio(loadTestAudio("uh_huh.wav"))
    
    var gotDismissed bool
    timeout := time.After(5 * time.Second)
    for {
        select {
        case event := <-stream.Events():
            switch e := event.(type) {
            case *vai.InterruptDismissedEvent:
                assert.Equal(t, "backchannel", e.Reason)
                gotDismissed = true
            case *vai.AudioDeltaEvent:
                if gotDismissed {
                    // TTS resumed
                    return
                }
            case *vai.ResponseInterruptedEvent:
                t.Fatal("backchannel should not trigger interrupt")
            }
        case <-timeout:
            t.Fatal("timeout")
        }
    }
}

func TestInterrupt_RealInterrupt_Handled(t *testing.T) {
    client := vai.NewClient()
    
    stream, err := client.Messages.RunStream(ctx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4-20250514",
    }, vai.WithLive())
    require.NoError(t, err)
    defer stream.Close()
    
    stream.SendText("Tell me a very long story")
    
    // Wait for bot to start speaking
    time.Sleep(3 * time.Second)
    
    // Send real interrupt
    stream.SendAudio(loadTestAudio("actually_wait.wav"))
    
    var gotInterrupted bool
    var gotNewResponse bool
    timeout := time.After(10 * time.Second)
    for {
        select {
        case event := <-stream.Events():
            switch event.(type) {
            case *vai.ResponseInterruptedEvent:
                gotInterrupted = true
            case *vai.InputCommittedEvent:
                if gotInterrupted {
                    // VAD captured the rest of user input
                }
            case *vai.ContentBlockDeltaEvent:
                if gotInterrupted {
                    gotNewResponse = true
                    return // Success
                }
            }
        case <-timeout:
            if !gotInterrupted {
                t.Fatal("expected interrupt")
            }
            if !gotNewResponse {
                t.Fatal("expected new response after interrupt")
            }
        }
    }
}

func TestInterrupt_NoSpeech_Resumes(t *testing.T) {
    client := vai.NewClient()
    
    stream, err := client.Messages.RunStream(ctx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4-20250514",
    }, vai.WithLive())
    require.NoError(t, err)
    defer stream.Close()
    
    stream.SendText("Hello")
    
    // Wait for bot to start speaking
    time.Sleep(2 * time.Second)
    
    // Send noise (no speech)
    stream.SendAudio(loadTestAudio("cough.wav"))
    
    var gotDismissed bool
    timeout := time.After(5 * time.Second)
    for {
        select {
        case event := <-stream.Events():
            switch e := event.(type) {
            case *vai.InterruptDismissedEvent:
                assert.Equal(t, "no_speech", e.Reason)
                gotDismissed = true
            case *vai.AudioDeltaEvent:
                if gotDismissed {
                    return // TTS resumed
                }
            }
        case <-timeout:
            t.Fatal("timeout")
        }
    }
}
```

---

## 15. Performance Targets

| Metric | Target | Measurement |
|--------|--------|-------------|
| VAD latency (energy only) | < 10ms | Silence detection to decision |
| VAD latency (with semantic) | < 150ms | Silence detection to Cerebras response |
| Grace period trigger | < 10ms | Audio detection to agent cancellation |
| Interrupt pause | < 5ms | Audio detection to TTS pause |
| Interrupt capture window | 600ms | Fixed capture duration |
| Interrupt semantic check | < 150ms | Capture complete to decision |
| TTS resume | < 10ms | Decision to audio resumption |
| Audio round-trip (Proxy) | < 500ms | User speaks → first audio response |
| Concurrent sessions | 100+ | Per server instance |

---

## 16. Package Structure

```
pkg/core/
├── live/
│   ├── session.go          # LiveSession orchestrator
│   ├── vad.go              # HybridVAD implementation
│   ├── grace_period.go     # Grace period manager
│   ├── interrupt.go        # Semantic interrupt detection
│   ├── buffer.go           # AudioBuffer for PCM handling
│   ├── tts_pipeline.go     # Streaming TTS with pause/resume
│   ├── protocol.go         # WebSocket message types
│   └── events.go           # Live-specific event types

├── stream.go               # Enhanced RunStream with Interrupt()

sdk/
├── messages.go             # RunStream() with Live mode support
├── options.go              # WithLive(), WithGracePeriod(), WithInterrupt()
├── live_stream.go          # WebSocket-backed Stream implementation
└── events.go               # Event type definitions

cmd/proxy/
├── handlers/
│   └── live.go             # WebSocket handler
└── main.go                 # Route registration

tests/
├── integration/
│   └── live_test.go        # End-to-end tests
└── unit/
    ├── vad_test.go         # VAD unit tests
    ├── grace_period_test.go # Grace period tests
    └── interrupt_test.go   # Interrupt detection tests
```

---

## 17. Acceptance Criteria

### Must Have (P0)
- [ ] WebSocket connection and configuration
- [ ] Audio streaming bidirectional
- [ ] VAD with energy + semantic detection
- [ ] **Grace period: user can continue within 5s**
- [ ] **Interrupt: 600ms capture window**
- [ ] **Interrupt: semantic check for backchannel vs real interrupt**
- [ ] TTS pause/resume for interrupt handling
- [ ] Tool calls work in live mode
- [ ] Conversation history maintained
- [ ] Mid-session model swapping
- [ ] Real-time transcript streaming
- [ ] Configurable grace period duration
- [ ] Configurable capture duration

### Nice to Have (P1)
- [ ] Local VAD fallback (Silero)
- [ ] Push-to-talk mode
- [ ] Opus codec support
- [ ] Echo cancellation
- [ ] Session persistence

---

## 18. Estimated Effort

| Component | Lines of Code | Complexity |
|-----------|---------------|------------|
| LiveSession orchestrator | ~500 | High |
| HybridVAD | ~200 | Medium |
| GracePeriodManager | ~150 | Medium |
| InterruptDetector | ~200 | Medium |
| TTS Pipeline | ~200 | Medium |
| RunStream enhancement | ~150 | Medium |
| Protocol/Events | ~300 | Low |
| SDK integration | ~350 | Medium |
| Proxy handler | ~100 | Low |
| Tests | ~700 | Medium |
| **Total** | **~2,850** | — |

**Estimated Timeline:** 3-4 weeks for core implementation, 1 week for testing and polish.

---

*End of Specification*

Implementation Risks & Recommendations
A. The "Click" Problem (TTS Pause/Resume)
In Section 10 (TTSPipeline), pausing and resuming audio streams can result in audible clicks or pops if you cut the PCM stream mid-wave.

Recommendation: When pausing, apply a very fast (5-10ms) fade-out to zero amplitude. When resuming, apply a fast fade-in. This should be handled in TTSPipeline.Pause() and Resume().

B. The "Double Speak" Race Condition
There is a tiny race condition window in Section 9.2 (processAgentResponse):

Grace period expires.

Agent generates text.

TTS starts buffering.

At this exact millisecond, the user speaks.

Risk: The system might emit a split second of audio before the Interrupt Logic kicks in.

Mitigation: The StateSpeaking transition must happen before the first byte of audio is sent to the websocket. Ensure the logic checks s.state inside the audio sending loop.

C. Dependency on Fast LLMs
The design relies heavily on cerebras/llama-3.1-8b or groq for the < 150ms semantic check.

Fallback: If the fast provider is down, the system should gracefully degrade.

Logic: If fastClient fails or times out, the InterruptDetector should default to IsInterrupt: true. It is better to interrupt falsely than to ignore a user trying to stop the bot.

D. Echo Cancellation (AEC)
The spec mentions AEC in "Nice to Have." In a real-world speakerphone scenario (laptop/phone), the microphone will pick up the bot's own voice.

Impact: The EnergyThreshold logic in VAD will trigger constantly because of the bot's own voice loopback.

Solution (for Phase 8): You will likely need "Server-Side Echo Cancellation" (subtracting the audio sent out from the audio coming in) or rely on client-side hardware AEC (standard in browsers/phones). For now, assume client-side AEC is active.

5. Refined Data Structures
I suggest one small addition to the SessionConfig to allow tuning the "Fast LLM" prompts, as developers might want to tweak the sensitivity of the interrupt/VAD logic.

In pkg/core/live/session.go:

```Go

type VADConfig struct {
    // ... existing fields ...
    // Allow overriding the system prompt for the VAD semantic check
    SemanticPrompt string `json:"semantic_prompt,omitempty"` 
}

type InterruptConfig struct {
    // ... existing fields ...
    // Allow overriding the system prompt for the Interrupt semantic check
    SemanticPrompt string `json:"semantic_prompt,omitempty"`
}
```