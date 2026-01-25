# Vango AI (WIP, NOT LAUNCHED)

**One API for every LLM. Text, tools, and voice.**

Vango AI is a unified Go SDK and self-hostable gateway for LLM providers. Switch between Anthropic, OpenAI, Gemini, Groq, and Mistral by changing one string. Built-in tool execution loops, streaming, and a complete voice pipeline (STT → LLM → TTS).

```go
stream, _ := client.Messages.RunStream(ctx, &vai.MessageRequest{
    Model: "anthropic/claude-sonnet-4",
    Messages: []vai.Message{
        {Role: "user", Content: vai.Text("Find the top 3 AI papers from this week and summarize them")},
    },
    Tools: []vai.Tool{vai.WebSearch()},
}, vai.WithMaxToolCalls(10))

for event := range stream.Events() {
    if text, ok := vai.TextDeltaFrom(event); ok {
        fmt.Print(text)
    }
}
```

## Why Vango AI?

| Problem | Vango AI Solution |
|---------|-------------------|
| Every provider has a different API | One unified interface for all providers |
| Tool loops require manual orchestration | `RunStream()` handles the entire loop |
| Voice requires stitching multiple services | Built-in STT → LLM → TTS pipeline |
| Provider lock-in | Switch models with one string change |

## API Design

Vango AI normalizes all providers to **Anthropic's Messages API** format. We chose this as the canonical format because it offers:

- **Clean content block model** — Extensible for text, images, audio, tools, and future modalities
- **Explicit tool use flow** — Clear separation between tool calls and tool results
- **Well-designed streaming** — Lifecycle events (`message_start`, `content_block_delta`, `message_stop`) that map cleanly to UI updates
- **First-class multimodal support** — Content as typed blocks, not string hacks

If you've used the Anthropic SDK, the Vango AI API will feel immediately familiar:

```go
// This is valid Vango AI code — and almost identical to Anthropic's SDK
resp, _ := client.Messages.Create(ctx, &vai.MessageRequest{
    Model:     "anthropic/claude-sonnet-4",  // Just add the provider prefix
    MaxTokens: 1024,
    Messages: []vai.Message{
        {Role: "user", Content: []vai.ContentBlock{
            vai.Text("What's in this image?"),
            vai.Image(imageData, "image/png"),
        }},
    },
})
```

The same request works with any provider — Vango handles the translation:

```go
// Switch to OpenAI by changing one string
resp, _ := client.Messages.Create(ctx, &vai.MessageRequest{
    Model: "openai/gpt-4o",  // Vango translates to OpenAI's Chat Completions format
    // ... same request structure
})
```

## Installation

```bash
go get github.com/vango-ai/vai
```

## Quick Start

### Direct Mode (No Server Required)

The SDK runs the translation engine directly in your process. Just set your provider API keys as environment variables:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
export OPENAI_API_KEY=sk-...
```

```go
package main

import (
    "context"
    "fmt"
    "github.com/vango-ai/vai"
)

func main() {
    client := vai.NewClient()

    resp, _ := client.Messages.Create(context.Background(), &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Hello!")},
        },
    })

    fmt.Println(resp.TextContent())
}
```

### Proxy Mode (For Production)

For centralized key management, observability, and multi-language support:

```go
client := vai.NewClient(
    vai.WithBaseURL("http://vai-proxy.internal:8080"),
    vai.WithAPIKey("vai_sk_..."),
)
```

## Core Features

### Streaming

Real-time token streaming with typed events:

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

### Tool Execution Loop

`Run()` and `RunStream()` handle the complete tool loop—you define the tools, Vango executes them:

```go
weatherTool := vai.FuncAsTool(
    "get_weather",
    "Get current weather for a location",
    func(ctx context.Context, input struct {
        Location string `json:"location" desc:"City name"`
    }) (string, error) {
        return fetchWeather(input.Location), nil
    },
)

result, _ := client.Messages.Run(ctx, &vai.MessageRequest{
    Model: "anthropic/claude-sonnet-4",
    Messages: []vai.Message{
        {Role: "user", Content: vai.Text("What's the weather in Tokyo and Paris?")},
    },
    Tools: []vai.Tool{weatherTool},
}, vai.WithMaxToolCalls(5))

fmt.Println(result.Response.TextContent())
fmt.Printf("Tool calls: %d\n", result.ToolCallCount)
```

### Native Tools

Web search, code execution, and computer use work consistently across providers:

```go
// Web search - translates to each provider's native implementation
tools := []vai.Tool{
    vai.WebSearch(),
    vai.CodeExecution(),
}
```

| Tool | Anthropic | OpenAI | Gemini |
|------|-----------|--------|--------|
| `WebSearch()` | `web_search_20250305` | `web_search` | Google Search |
| `CodeExecution()` | `code_execution` | `code_interpreter` | Native |

### Voice Pipeline

Add voice to any model with STT and TTS providers:

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

// Response includes both text and synthesized audio
fmt.Println(resp.TextContent())
playAudio(resp.AudioContent().Data())
```

### Live Voice Sessions

Real-time bidirectional voice with intelligent turn-taking:

```go
stream, _ := client.Messages.RunStream(ctx, &vai.MessageRequest{
    Model:  "anthropic/claude-sonnet-4",
    System: "You are a helpful voice assistant.",
    Tools:  []vai.Tool{vai.WebSearch()},
    Voice: &vai.VoiceConfig{
        Input:  &vai.VoiceInputConfig{Provider: "cartesia"},
        Output: &vai.VoiceOutputConfig{Provider: "cartesia", Voice: "sonic-english"},
        VAD:    &vai.VADConfig{SemanticCheck: true},
    },
}, vai.WithLive(&vai.LiveConfig{SampleRate: 24000}))

// Send microphone audio
go func() {
    for chunk := range microphone.Chunks() {
        stream.SendAudio(chunk)
    }
}()

// Receive events
for event := range stream.Events() {
    switch e := event.(type) {
    case *vai.TranscriptDeltaEvent:
        fmt.Print(e.Delta) // Real-time transcription
    case *vai.AudioChunkEvent:
        speaker.Write(e.Data) // Play response audio
    }
}
```

### Structured Data Extraction

Extract typed data with automatic schema generation:

```go
type Contact struct {
    Name  string `json:"name" desc:"Full name"`
    Email string `json:"email" desc:"Email address"`
    Role  string `json:"role" enum:"engineer,manager,designer"`
}

var contact Contact
client.Messages.Extract(ctx, &vai.MessageRequest{
    Model: "openai/gpt-4o",
    Messages: []vai.Message{
        {Role: "user", Content: vai.Text("John Smith (john@example.com) is our lead engineer")},
    },
}, &contact)

fmt.Printf("%s <%s> - %s\n", contact.Name, contact.Email, contact.Role)
```

### Vision

Images work the same across all vision-capable models:

```go
resp, _ := client.Messages.Create(ctx, &vai.MessageRequest{
    Model: "gemini/gemini-2.0-flash",
    Messages: []vai.Message{
        {Role: "user", Content: []vai.ContentBlock{
            vai.Text("What's in this image?"),
            vai.ImageURL("https://example.com/photo.jpg"),
        }},
    },
})
```

## Supported Providers

| Provider | Models | Native Tools |
|----------|--------|--------------|
| **Anthropic** | Claude Sonnet 4, Claude Opus 4, Claude Haiku | Web search, code execution, computer use |
| **OpenAI** | GPT-4o, GPT-4o-mini, o1 | Web search, code interpreter, file search |
| **Google** | Gemini 2.5 Pro/Flash, Gemini 2.0 Flash | Google Search, code execution |
| **Groq** | Llama 3.3 70B, Mixtral | — |
| **Mistral** | Mistral Large, Codestral | — |

## Architecture

Vango AI uses a **Shared Core** design. The same Go library (`pkg/core`) powers both the SDK and the Proxy:

```
┌─────────────────────────────────────────────────────┐
│                    pkg/core                         │
│         (Translation, Voice, Tools)                 │
└─────────────────────────────────────────────────────┘
              │                       │
              ▼                       ▼
    ┌─────────────────┐     ┌─────────────────┐
    │     Go SDK      │     │   Vango Proxy   │
    │  (Direct Mode)  │     │  (Server Mode)  │
    └─────────────────┘     └─────────────────┘
```

**Direct Mode**: SDK imports `pkg/core` directly. Zero infrastructure, zero latency overhead.

**Proxy Mode**: SDK talks to the Vango Proxy over HTTP. Centralized keys, observability, rate limiting.

## Running the Proxy

```bash
docker run -p 8080:8080 \
  -e ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY \
  -e OPENAI_API_KEY=$OPENAI_API_KEY \
  ghcr.io/vango-ai/vai-proxy
```

## Documentation

- [API Specification](./docs/API_SPEC.md) — Complete API reference
- [SDK Specification](./docs/SDK_SPEC.md) — Go SDK documentation
- [Examples](./examples/) — Working code examples

## License

MIT