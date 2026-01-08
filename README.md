# VAI - Vango AI

**A unified AI gateway for Go.** One API, every LLM provider.

```go
client := vai.NewClient()

// Define a custom tool
bookFlight := vai.FuncAsTool("book_flight", "Book a flight",
    func(ctx context.Context, input struct {
        From string `json:"from"`
        To   string `json:"to"`
        Date string `json:"date"`
    }) (string, error) {
        return fmt.Sprintf("Booked: %s → %s on %s", input.From, input.To, input.Date), nil
    },
)

// Run with native web search + custom tool
stream, _ := client.Messages.RunStream(ctx, &vai.MessageRequest{
    Model: "anthropic/claude-sonnet-4",
    Messages: []vai.Message{
        {Role: "user", Content: vai.Text("Find flights from NYC to Tokyo next week and book the best one")},
    },
    Tools: []vai.Tool{vai.WebSearch(), bookFlight},
})

for event := range stream.Events() {
    switch e := event.(type) {
    case *vai.TextDeltaEvent:
        fmt.Print(e.Delta)
    case *vai.ToolCallEvent:
        fmt.Printf("\n[%s] %v\n", e.Name, e.Input)
    }
}
```

Switch providers by changing one string:

```go
Model: "openai/gpt-5"          // OpenAI
Model: "gemini/gemini-3.0-flash" // Google
Model: "groq/llama-3.3-70b"    // Groq (fast)
```

---

## Why VAI?

| Problem | VAI Solution |
|---------|--------------|
| Different APIs for each provider | Single unified API (Anthropic Messages format) |
| Inconsistent tool implementations | Native tools (`web_search`, `code_execution`) work identically across providers |
| No voice support in most SDKs | Built-in STT → LLM → TTS pipeline |
| Proxy required for abstraction | **Direct Mode**: No proxy needed. Import and go. |
| Hard to switch providers | Change one string: `anthropic/claude-haiku-4-5-20251001` → `openai/gpt-5-mini` |

---

## Installation

```bash
go get github.com/vango-ai/vai
```

**Requirements:** Go 1.21+

---

## Quick Start

### 1. Set API Keys

```bash
export ANTHROPIC_API_KEY=sk-ant-...
export OPENAI_API_KEY=sk-...
# Add keys for providers you want to use
```

### 2. Simple Request

```go
package main

import (
    "context"
    "fmt"
    "github.com/vango-ai/vai"
)

func main() {
    client := vai.NewClient()

    resp, err := client.Messages.Create(context.Background(), &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("What is the capital of France?")},
        },
    })
    if err != nil {
        panic(err)
    }

    fmt.Println(resp.TextContent())
}
```

### 3. Streaming

```go
stream, _ := client.Messages.Stream(ctx, &vai.MessageRequest{
    Model: "openai/gpt-4o",
    Messages: []vai.Message{
        {Role: "user", Content: vai.Text("Write a haiku about Go")},
    },
})
defer stream.Close()

for event := range stream.Events() {
    if delta, ok := event.(*vai.ContentBlockDeltaEvent); ok {
        if text, ok := delta.Delta.(*vai.TextDelta); ok {
            fmt.Print(text.Text)
        }
    }
}
```

### 4. Tool Use (Agentic)

```go
// Define a tool with type-safe handler
weatherTool := vai.FuncAsTool(
    "get_weather",
    "Get current weather for a location",
    func(ctx context.Context, input struct {
        Location string `json:"location" desc:"City name"`
    }) (string, error) {
        return fmt.Sprintf("Weather in %s: 72°F, sunny", input.Location), nil
    },
)

// Run executes the tool loop automatically
result, _ := client.Messages.Run(ctx, &vai.MessageRequest{
    Model: "anthropic/claude-sonnet-4",
    Messages: []vai.Message{
        {Role: "user", Content: vai.Text("What's the weather in Tokyo?")},
    },
    Tools: []vai.Tool{weatherTool},
}, vai.WithMaxToolCalls(5))

fmt.Println(result.Response.TextContent())
```

### 5. Native Tools

VAI normalizes native tools across providers:

```go
// Web search works on Anthropic, OpenAI, and Gemini
result, _ := client.Messages.Run(ctx, &vai.MessageRequest{
    Model: "anthropic/claude-sonnet-4", // or openai/gpt-4o, gemini/gemini-2.0-flash
    Messages: []vai.Message{
        {Role: "user", Content: vai.Text("What happened in tech news today?")},
    },
    Tools: []vai.Tool{vai.WebSearch()},
})
```

| VAI Tool | Anthropic | OpenAI | Gemini |
|----------|-----------|--------|--------|
| `vai.WebSearch()` | `web_search_20250305` | `web_search` | Google Search |
| `vai.CodeExecution()` | `code_execution` | `code_interpreter` | Native |
| `vai.ComputerUse()` | `computer_20250124` | `computer_use` | - |

### 6. Vision

```go
resp, _ := client.Messages.Create(ctx, &vai.MessageRequest{
    Model: "openai/gpt-4o",
    Messages: []vai.Message{
        {Role: "user", Content: []vai.ContentBlock{
            vai.Text("What's in this image?"),
            vai.ImageURL("https://example.com/image.png"),
        }},
    },
})
```

### 7. Structured Output

```go
type Person struct {
    Name    string `json:"name" desc:"Full name"`
    Age     int    `json:"age" desc:"Age in years"`
    Company string `json:"company" desc:"Employer"`
}

var person Person
resp, _ := client.Messages.Extract(ctx, &vai.MessageRequest{
    Model: "openai/gpt-4o",
    Messages: []vai.Message{
        {Role: "user", Content: vai.Text("John Doe is a 30 year old engineer at Acme Corp.")},
    },
}, &person)

fmt.Printf("%s (%d) works at %s\n", person.Name, person.Age, person.Company)
```

---

## Architecture

VAI uses a **Shared Core** architecture:

```
┌─────────────────────────────────────────────────────────────┐
│                        pkg/core                              │
│              (The Source of Truth for All Logic)             │
│                                                              │
│   ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│   │  providers/  │  │    voice/    │  │    tools/    │      │
│   │  anthropic   │  │   pipeline   │  │  normalize   │      │
│   │  openai      │  │   stt/tts    │  │              │      │
│   │  gemini      │  │              │  │              │      │
│   └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
                              │
          ┌───────────────────┴───────────────────┐
          ▼                                       ▼
┌─────────────────────────┐     ┌─────────────────────────────┐
│      GO SDK             │     │      VAI PROXY              │
│   (Direct Mode)         │     │   (Server Mode)             │
│                         │     │                             │
│  import "vai/pkg/core"  │     │  import "vai/pkg/core"      │
│  Executes in-process    │     │  Exposes /v1/messages       │
│  Zero network latency   │     │  Centralized governance     │
└─────────────────────────┘     └─────────────────────────────┘
```

### Two Modes

| Feature | Direct Mode (Go SDK) | Proxy Mode (Server) |
|---------|----------------------|---------------------|
| **Use Case** | Local dev, CLIs, agents | Production, multi-language teams |
| **Latency** | Zero (in-process) | Low (1 network hop) |
| **Setup** | `go get` | Docker / K8s |
| **Secrets** | Env vars | Centralized / Vault |
| **Languages** | Go only | Any HTTP client |

```go
// Direct Mode (default) - SDK calls providers directly
client := vai.NewClient()

// Proxy Mode - SDK talks to VAI Proxy via HTTP
client := vai.NewClient(
    vai.WithBaseURL("http://vai-proxy.internal:8080"),
    vai.WithAPIKey("vai_sk_..."),
)
```

---

## Voice Pipeline

VAI includes built-in voice support: **STT → LLM → TTS**

### Audio Input/Output

```go
resp, _ := client.Messages.Create(ctx, &vai.MessageRequest{
    Model: "anthropic/claude-sonnet-4",
    Messages: []vai.Message{
        {Role: "user", Content: vai.Audio(audioData, "audio/wav")},
    },
    Voice: &vai.VoiceConfig{
        Input:  &vai.VoiceInputConfig{Provider: "deepgram", Model: "nova-2"},
        Output: &vai.VoiceOutputConfig{Provider: "elevenlabs", Voice: "rachel"},
    },
})

// Response includes audio
playAudio(resp.AudioContent().Data())
```

### Real-Time Voice (Live Sessions)

```go
stream, _ := client.Messages.RunStream(ctx, &vai.MessageRequest{
    Model:  "anthropic/claude-sonnet-4",
    System: "You are a helpful voice assistant.",
    Voice: &vai.VoiceConfig{
        Input:  &vai.VoiceInputConfig{Provider: "cartesia"},
        Output: &vai.VoiceOutputConfig{Provider: "cartesia", Voice: "sonic-english"},
    },
}, vai.WithLive(&vai.LiveConfig{SampleRate: 24000}))
defer stream.Close()

// Handle events
go func() {
    for event := range stream.Events() {
        switch e := event.(type) {
        case *vai.TranscriptDeltaEvent:
            fmt.Print(e.Delta) // Real-time transcription
        case *vai.AudioChunkEvent:
            speaker.Write(e.Data) // Play audio
        }
    }
}()

// Send audio from microphone
for chunk := range microphone.Chunks() {
    stream.SendAudio(chunk)
}
```

### Supported Providers

| STT | TTS |
|-----|-----|
| Deepgram (`nova-2`, `nova`, `enhanced`) | ElevenLabs (100+ voices) |
| OpenAI Whisper | OpenAI TTS (`alloy`, `echo`, `fable`, etc.) |
| AssemblyAI | Cartesia (low latency) |

---

## Supported Models

### Anthropic
- `anthropic/claude-sonnet-4`
- `anthropic/claude-opus-4`
- `anthropic/claude-haiku-3`

### OpenAI
- `openai/gpt-4o`
- `openai/gpt-4o-mini`
- `openai/o1`
- `openai/o1-mini`

### Google Gemini
- `gemini/gemini-2.5-pro`
- `gemini/gemini-2.5-flash`
- `gemini/gemini-2.0-flash`

### Groq (Fast Inference)
- `groq/llama-3.3-70b`
- `groq/llama-3.1-70b`
- `groq/mixtral-8x7b`

### Mistral
- `mistral/mistral-large`
- `mistral/mistral-small`
- `mistral/codestral`

---

## API Reference

### Client

```go
// Direct Mode (default)
client := vai.NewClient()

// With options
client := vai.NewClient(
    vai.WithProviderKey("anthropic", "sk-ant-..."),
    vai.WithTimeout(30 * time.Second),
    vai.WithRetries(3),
    vai.WithLogger(slog.Default()),
)

// Proxy Mode
client := vai.NewClient(
    vai.WithBaseURL("http://localhost:8080"),
    vai.WithAPIKey("vai_sk_..."),
)
```

### Messages Service

```go
// Single request/response
resp, err := client.Messages.Create(ctx, req)

// Streaming
stream, err := client.Messages.Stream(ctx, req)

// Tool loop (blocking)
result, err := client.Messages.Run(ctx, req, vai.WithMaxToolCalls(10))

// Tool loop (streaming)
stream, err := client.Messages.RunStream(ctx, req, vai.WithMaxToolCalls(10))

// Structured extraction
var output MyStruct
resp, err := client.Messages.Extract(ctx, req, &output)
```

### Run Options

```go
vai.WithMaxToolCalls(n)              // Stop after n tool calls
vai.WithMaxTurns(n)                  // Stop after n LLM turns
vai.WithTimeout(d)                   // Timeout for entire run
vai.WithStopWhen(fn)                 // Custom stop condition
vai.WithToolHandler(name, fn)        // Register tool handler
vai.WithOnToolCall(fn)               // Hook for tool execution
vai.WithLive(cfg)                    // Enable real-time voice
```

---

## Project Structure

```
vai/
├── pkg/core/                  # Shared logic (providers, voice, tools)
│   ├── providers/             # Anthropic, OpenAI, Gemini, Groq
│   ├── voice/                 # STT/TTS pipeline
│   └── tools/                 # Tool normalization
├── sdk/                       # Go SDK
│   ├── client.go
│   ├── messages.go
│   ├── stream.go
│   ├── run.go
│   ├── live.go
│   └── tools.go
├── cmd/vai-proxy/             # HTTP server binary
├── docs/
│   ├── API_SPEC.md            # Full API specification
│   └── SDK_SPEC.md            # Full SDK specification
└── examples/
```

---

## Environment Variables

| Variable | Description |
|----------|-------------|
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `OPENAI_API_KEY` | OpenAI API key |
| `GOOGLE_API_KEY` | Google Gemini API key |
| `GROQ_API_KEY` | Groq API key |
| `DEEPGRAM_API_KEY` | Deepgram STT key |
| `ELEVENLABS_API_KEY` | ElevenLabs TTS key |
| `CARTESIA_API_KEY` | Cartesia TTS key |

---

## Documentation

- [API Specification](docs/API_SPEC.md) - Complete API reference
- [SDK Specification](docs/SDK_SPEC.md) - Go SDK documentation

---

## License

MIT
