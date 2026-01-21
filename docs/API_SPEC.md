# Vango AI API Specification

**Version:** 1.0.0  
**Date:** December 2025  
**Status:** Implementation Ready

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
   - [1.1 System Architecture](#11-system-architecture)
2. [Design Principles](#2-design-principles)
3. [API Endpoints Overview](#3-api-endpoints-overview)
4. [Authentication](#4-authentication)
5. [The Messages Endpoint](#5-the-messages-endpoint)
6. [Input Content Blocks](#6-input-content-blocks)
7. [Output Content Blocks](#7-output-content-blocks)
8. [Tools](#8-tools)
9. [Streaming Protocol](#9-streaming-protocol)
10. [Voice Pipeline](#10-voice-pipeline)
11. [The Live Endpoint](#11-the-live-endpoint)
12. [The Audio Endpoint](#12-the-audio-endpoint)
13. [The Models Endpoint](#13-the-models-endpoint)
14. [Provider Translation](#14-provider-translation)
15. [Error Handling](#15-error-handling)
16. [Observability](#16-observability)
17. [Configuration](#17-configuration)
18. [Rate Limiting](#18-rate-limiting)
19. [Request & Response Examples](#19-request--response-examples)
20. [OpenAPI Schema](#20-openapi-schema)

---

## 1. Executive Summary

### What is Vango AI?

Vango AI is a self-hostable AI gateway that provides a unified API for interacting with multiple LLM providers. It normalizes the differences between OpenAI, Anthropic, Google Gemini, Groq, Mistral, and other providers behind a single, clean API surface.

### Core Value Propositions

1. **Provider Abstraction**: Switch between providers by changing one string (`model: "anthropic/claude-sonnet-4"` → `model: "openai/gpt-4o"`)
2. **Feature Normalization**: Native tools (web search, code execution) work consistently across providers
3. **Voice Pipeline**: Built-in STT → LLM → TTS orchestration
4. **Centralized Management**: API keys, usage tracking, rate limiting in one place
5. **Self-Hostable**: Run your own instance or use the hosted service

### 1.1 System Architecture: Shared Core

Vango AI utilizes a **Shared Core** architecture for maximum flexibility. The translation logic, voice orchestration, and tool normalization reside in a shared Go library (`pkg/core`). This core is consumed in two ways:

1.  **Embedded (Direct Mode):** Go applications import the core directly. Zero infrastructure required.
2.  **Remote (Proxy Mode):** The Vango AI Proxy wraps the core in an HTTP server for non-Go languages and centralized governance.

```
┌───────────────────────────────────────────────────────────────────┐
│                    Vango AI MONOREPO                                 │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                      pkg/core                                │ │
│  │           (The Source of Truth for All Logic)                │ │
│  │                                                               │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐        │ │
│  │  │  providers/  │  │    voice/    │  │    tools/    │        │ │
│  │  │  anthropic   │  │   pipeline   │  │  normalize   │        │ │
│  │  │  openai      │  │   stt/tts    │  │              │        │ │
│  │  │  gemini      │  │              │  │              │        │ │
│  │  └──────────────┘  └──────────────┘  └──────────────┘        │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                               │                                   │
│           ┌───────────────────┴───────────────────┐              │
│           ▼                                       ▼              │
│  ┌─────────────────────────┐     ┌─────────────────────────────┐│
│  │      GO SDK             │     │      Vango AI PROXY            ││
│  │   (Direct Mode)         │     │   (Server Mode)             ││
│  │                         │     │                             ││
│  │  import "Vango AI/pkg/core"│     │  import "Vango AI/pkg/core"    ││
│  │  Executes in-process    │     │  Exposes /v1/messages       ││
│  │  Zero network latency   │     │  Centralized governance     ││
│  └────────────┬────────────┘     └──────────────┬──────────────┘│
│               │                                  │               │
└───────────────┼──────────────────────────────────┼───────────────┘
                │                                  │
                ▼                                  ▼
        ┌─────────────┐                 ┌─────────────────────┐
        │ LLM APIs    │                 │ HTTP Clients        │
        │ (Anthropic, │                 │ (Python, Node,      │
        │  OpenAI,    │                 │  Rust, curl, etc.)  │
        │  Gemini)    │                 └─────────────────────┘
        └─────────────┘
```

---

#### 1.1.1 Why Shared Core?

The Shared Core architecture provides the best of both worlds:

| Feature | Direct Mode (Go SDK) | Proxy Mode (Server) |
|---------|----------------------|---------------------|
| **Use Case** | Local dev, CLIs, high-perf agents | Production, polyglot teams, SaaS |
| **Latency** | Zero (in-process) | Low (1 network hop) |
| **Setup** | `go get` (instant) | Docker / K8s (infrastructure) |
| **Secrets** | Env vars (`ANTHROPIC_API_KEY`) | Centralized / Vault |
| **Observability** | App-level logging | Prometheus, OpenTelemetry |
| **Updates** | Update Go module | Redeploy server |
| **Multi-Language** | Go only | Any HTTP client |

**Key Insight:** The `pkg/core` library is the single source of truth. Whether you're calling it directly from Go or accessing it through the Proxy's HTTP API, **the same translation code runs**.

---

#### 1.1.2 The Shared Core (`pkg/core`)

The "brain" of vai. This Go library contains all the complex logic:

**1. Provider Translation Engine**
*   Converts Vango AI API format (Anthropic Messages API style) ↔ Provider formats
*   Anthropic Messages API (passthrough)
*   OpenAI Chat Completions API
*   OpenAI Responses API (for advanced features)
*   Google Gemini API
*   OpenAI-compatible APIs (Groq, Together, Mistral)

**2. Voice Pipeline Orchestration**
*   Speech-to-Text (Deepgram, OpenAI Whisper, AssemblyAI)
*   Text-to-Speech (ElevenLabs, OpenAI TTS, Cartesia)
*   Sentence-boundary buffering for natural speech
*   Full STT → LLM → TTS flow

**3. Tool Normalization**
*   `{"type": "web_search"}` → Anthropic's `web_search_20250305`, OpenAI's `web_search`, Gemini's Google Search
*   `{"type": "code_execution"}` → Anthropic's `code_execution`, OpenAI's `code_interpreter`

**4. Validation & Error Mapping**
*   Request validation before sending to providers
*   Provider error → Vango AI error translation

---

#### 1.1.3 Direct Mode (Go SDK)

For Go developers, **Direct Mode** provides zero-infrastructure AI integration:

```go
// Direct Mode: No proxy needed
// SDK imports pkg/core and calls providers directly
client := vai.NewClient() // Reads ANTHROPIC_API_KEY, OPENAI_API_KEY from env

resp, err := client.Messages.Create(ctx, &vai.MessageRequest{
    Model: "anthropic/claude-sonnet-4",
    Messages: []vai.Message{
        {Role: "user", Content: vai.Text("Hello!")},
    },
})
```

**What happens under the hood:**
1. SDK calls `pkg/core/providers/anthropic.CreateMessage()`
2. Core translates request (passthrough for Anthropic)
3. Core makes HTTP call to `api.anthropic.com`
4. Core translates response back to Vango AI format
5. SDK returns `*vai.Response`

**Best for:**
- Local development
- CLI tools
- High-performance agents (no network hop)
- Simple projects where you don't need centralized governance

---

#### 1.1.4 Proxy Mode (HTTP Server)

For production and polyglot teams, the **Vango AI Proxy** wraps `pkg/core` in an HTTP server:

```go
// Proxy Mode: SDK talks to Vango AI Proxy via HTTP
client := vai.NewClient(
    vai.WithBaseURL("http://Vango AI-proxy.internal:8080"),
    vai.WithAPIKey("Vango AI_sk_..."),
)

resp, err := client.Messages.Create(ctx, &vai.MessageRequest{
    Model: "anthropic/claude-sonnet-4",
    Messages: []vai.Message{
        {Role: "user", Content: vai.Text("Hello!")},
    },
})
```

**What happens under the hood:**
1. SDK makes HTTP POST to `http://Vango AI-proxy:8080/v1/messages`
2. Proxy authenticates request, checks rate limits
3. Proxy calls `pkg/core/providers/anthropic.CreateMessage()`
4. Core makes HTTP call to `api.anthropic.com`
5. Proxy logs metrics, returns response to SDK

**The Proxy adds:**
*   **Authentication**: Issue `Vango AI_sk_...` API keys to internal teams
*   **Rate Limiting**: Global and per-user limits
*   **Observability**: Prometheus metrics, OpenTelemetry tracing, structured logging
*   **Secret Management**: Provider keys stay on the server, not on clients
*   **Caching**: Response caching (planned)

**Best for:**
- Production deployments
- Teams with multiple applications
- Non-Go languages (Python, Node, Rust, curl)
- Environments requiring centralized governance

---

#### 1.1.5 Non-Go SDKs (HTTP Clients)

Python, Node, Rust, and other language SDKs are **thin HTTP clients** that talk to the Proxy:

```python
# Python SDK - always talks to Proxy
import Vango AI

client = vai.Client(
    base_url="https://api.vai.dev",
    api_key="Vango AI_sk_..."
)

response = client.messages.create(
    model="anthropic/claude-sonnet-4",
    messages=[{"role": "user", "content": "Hello!"}]
)
```

These SDKs provide:
- Language-native types
- Ergonomic request builders
- Streaming support
- Error handling

They do NOT contain `pkg/core` logic (that would require porting Go code).

---

#### 1.1.6 Monorepo Structure

```
Vango AI-ai/Vango AI/
├── pkg/
│   └── core/
│       ├── types/          # Vango AI API types (Request, Response, ContentBlock)
│       ├── providers/
│       │   ├── anthropic/  # Anthropic (passthrough)
│       │   ├── openai/     # OpenAI ↔ Vango AI translation
│       │   ├── gemini/     # Gemini ↔ Vango AI translation
│       │   └── groq/       # Groq (OpenAI-compat)
│       ├── voice/
│       │   ├── pipeline.go # STT → LLM → TTS orchestration
│       │   ├── stt/        # Deepgram, Whisper
│       │   └── tts/        # ElevenLabs, OpenAI TTS
│       └── tools/
│           └── normalize.go # web_search → provider-specific
│
├── sdk/                    # The Go SDK (Direct + Proxy modes)
│   ├── client.go
│   ├── messages.go
│   ├── stream.go
│   └── ...
│
├── cmd/
│   └── Vango AI-proxy/        # The HTTP server binary
│       └── main.go
│
├── api/
│   └── openapi.yaml        # OpenAPI spec for non-Go SDKs
│
└── docker/
    └── Dockerfile
```

---

#### 1.1.7 Architectural Guarantees

1.  **Single Source of Truth**: `pkg/core` is THE implementation. Direct Mode and Proxy Mode run identical logic.

2.  **Zero-Drift**: Since both modes import the same code, there's no possibility of behavior divergence.

3.  **Zero-Infrastructure Option**: Go developers can use Vango AI without running any servers.

4.  **Centralized Option**: Teams can deploy the Proxy for governance, observability, and multi-language support.

5.  **Incremental Adoption**: Start with Direct Mode, migrate to Proxy Mode when you need governance.

---

#### 1.1.8 Comparison to Other Systems

| System | Architecture | Vango AI Equivalent |
|--------|--------------|------------------|
| **Stripe** | Server-first. SDKs are HTTP clients. | Vango AI Proxy + Non-Go SDKs |
| **OpenAI SDK** | SDK contains all logic. No proxy. | Vango AI Direct Mode |
| **LiteLLM** | Library-first. Proxy wraps library. | Vango AI (same pattern) |
| **Helicone** | Proxy-only (logging). No translation. | Vango AI Proxy (subset) |
| **Portkey** | Proxy with translation. | Vango AI Proxy (similar) |

Vango AI's Shared Core design is most similar to **LiteLLM**: a powerful library that can be used directly OR wrapped in a server.

### API Design Foundation

The API is based on **Anthropic's Messages API** because it provides:
- Clean content block model (extensible for multimodal)
- Explicit tool use/result flow
- Well-designed streaming events
- Clear separation of concerns

We extend this foundation to support the union of all providers' capabilities.

---

## 2. Design Principles

### 2.1 Anthropic Messages API as Canonical Format

All requests and responses follow Anthropic's structure:
- Messages as array of `{role, content}` objects
- Content as array of typed blocks
- Streaming via Server-Sent Events with lifecycle events

### 2.2 Provider/Model String Format

Models are specified as `provider/model-name[:version]`:
```
anthropic/claude-sonnet-4
openai/gpt-4o
gemini/gemini-2.0-flash
groq/llama-3.3-70b
mistral/mistral-large
```

### 2.3 Native Tool Normalization

The API exposes a unified tool interface. The proxy translates to provider-specific implementations:
```json
{"type": "web_search"}  →  Anthropic: web_search_20250305
                        →  OpenAI: web_search
                        →  Gemini: google_search
```

### 2.4 Graceful Degradation

When a feature isn't supported by a provider, the proxy:
1. Returns a clear error (if feature is critical)
2. Silently omits the feature (if optional)
3. Emulates the feature (if possible)

### 2.5 Extensibility via Extensions

Provider-specific features are accessible via the `extensions` field, ensuring the API can support new capabilities without breaking changes.

### 2.6 URL Ingestion Policy

Vango AI does **not** fetch external URLs (SSRF safety). 
- Images via URL: Passed directly to the provider. The provider's egress policy applies.
- Documents/Videos: Must be provided as base64 data to Vango AI unless using a provider-native file search feature.
- Gateway Timeout: Vango AI sets a hard 60s timeout for all upstream fetches.

---

## 3. API Endpoints Overview

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/v1/messages` | `POST` | Main LLM interaction (text, streaming, voice) |
| `/v1/messages/live` | `WebSocket` | Real-time bidirectional voice/text |
| `/v1/audio` | `POST` | Standalone STT or TTS |
| `/v1/models` | `GET` | List available models and capabilities |

### Base URL

- **Hosted**: `https://api.vai.dev`
- **Self-hosted**: `http://localhost:8080`

### Content Types

- Request: `application/json`
- Response: `application/json` (non-streaming) or `text/event-stream` (streaming)

---

## 4. Authentication

### 4.1 API Key Authentication

```http
POST /v1/messages
Authorization: Bearer Vango AI_sk_abc123...
Idempotency-Key: my-unique-key-123  (Optional)
Content-Type: application/json
```

### 4.2 API Key Modes

| Mode | Description |
|------|-------------|
| **BYO Keys** | User provides their own provider API keys via headers |
| **Managed Keys** | Vango AI manages provider keys, bills user |
| **Passthrough** | Forward user's key directly to provider |

### 4.3 BYO Keys Headers

```http
X-Provider-Key-Anthropic: sk-ant-...
X-Provider-Key-OpenAI: sk-...
X-Provider-Key-Google: AIza...
```

Or via request body:
```json
{
  "provider_keys": {
    "anthropic": "sk-ant-...",
    "openai": "sk-..."
  }
}
```

---

## 5. The Messages Endpoint

**Endpoint**: `POST /v1/messages`

This is the primary endpoint for all LLM interactions.

### 5.1 Request Schema

```typescript
interface MessageRequest {
  // Required
  model: string;                    // "provider/model-name"
  messages: Message[];              // Conversation history
  
  // Optional - Generation
  max_tokens?: number;              // Maximum tokens to generate
  system?: string | ContentBlock[]; // System prompt
  temperature?: number;             // 0.0 - 2.0
  top_p?: number;                   // Nucleus sampling
  top_k?: number;                   // Top-k sampling
  stop_sequences?: string[];        // Stop generation triggers
  
  // Optional - Tools
  tools?: Tool[];                   // Available tools
  tool_choice?: ToolChoice;         // Tool selection strategy
  
  // Optional - Streaming
  stream?: boolean;                 // Enable SSE streaming
  
  // Optional - Output
  output?: OutputConfig;            // Multimodal output settings
  output_format?: OutputFormat;     // Structured output schema
  
  // Optional - Voice
  voice?: VoiceConfig;              // STT/TTS pipeline config
  
  // Optional - Provider Extensions
  extensions?: Record<string, any>; // Provider-specific options
  
  // Optional - Metadata
  metadata?: Record<string, any>;   // User-defined metadata (passthrough)
}

interface OutputFormat {
  type: "json_schema";
  json_schema: JSONSchema;
}
```

### 5.2 Message Schema

```typescript
interface Message {
  role: "user" | "assistant";
  content: string | ContentBlock[];
}
```

### 5.3 Response Schema

```typescript
interface MessageResponse {
  id: string;                       // Unique response ID
  type: "message";                  // Always "message"
  role: "assistant";                // Always "assistant"
  model: string;                    // Resolved model name
  content: ContentBlock[];          // Response content blocks
  stop_reason: StopReason;          // Why generation stopped
  stop_sequence?: string;           // If stopped by stop sequence
  usage: Usage;                     // Token counts and cost
}

type StopReason = 
  | "end_turn"       // Natural end
  | "max_tokens"     // Hit token limit
  | "stop_sequence"  // Hit stop sequence
  | "tool_use";      // Model wants to use a tool

interface Usage {
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  cache_read_tokens?: number;    // Anthropic prompt caching
  cache_write_tokens?: number;
  cost_usd?: number;             // Calculated cost
}
```

### 5.4 Response Headers

```http
HTTP/1.1 200 OK
Content-Type: application/json
X-Request-ID: req_abc123
X-Model: anthropic/claude-sonnet-4
X-Input-Tokens: 1523
X-Output-Tokens: 847
X-Cost-USD: 0.0142
X-Duration-Ms: 2341
```

---

## 6. Input Content Blocks

Content blocks represent multimodal input in the `content` array.

### 6.1 Text Block

```json
{
  "type": "text",
  "text": "What's in this image?"
}
```

### 6.2 Image Block

```json
{
  "type": "image",
  "source": {
    "type": "base64",
    "media_type": "image/png",
    "data": "iVBORw0KGgo..."
  }
}
```

Or via URL:
```json
{
  "type": "image",
  "source": {
    "type": "url",
    "url": "https://example.com/image.png"
  }
}
```

**Supported formats**: `image/png`, `image/jpeg`, `image/gif`, `image/webp`

### 6.3 Audio Block

```json
{
  "type": "audio",
  "source": {
    "type": "base64",
    "media_type": "audio/wav",
    "data": "UklGRiQA..."
  }
}
```

**Supported formats**: `audio/wav`, `audio/mp3`, `audio/webm`, `audio/ogg`

**Behavior**: When `voice.input` is configured, audio is transcribed before sending to LLM. Otherwise, sent directly to models that support native audio (GPT-4o, Gemini).

### 6.4 Video Block

```json
{
  "type": "video",
  "source": {
    "type": "base64",
    "media_type": "video/mp4",
    "data": "AAAAIGZ0..."
  }
}
```

**Supported providers**: Gemini only (currently)

### 6.5 Document Block

```json
{
  "type": "document",
  "source": {
    "type": "base64",
    "media_type": "application/pdf",
    "data": "JVBERi0x..."
  },
  "filename": "report.pdf"
}
```

**Supported formats**: `application/pdf`, `text/plain`, `text/markdown`, `text/csv`

### 6.6 Tool Result Block

Used when returning results from tool calls:

```json
{
  "type": "tool_result",
  "tool_use_id": "call_abc123",
  "content": [
    {"type": "text", "text": "The weather in Tokyo is 22°C and sunny."}
  ]
}
```

Tool results can also contain images:
```json
{
  "type": "tool_result",
  "tool_use_id": "call_abc123",
  "content": [
    {"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "..."}}
  ]
}
```

---

## 7. Output Content Blocks

Response content blocks represent model outputs.

### 7.1 Text Block

```json
{
  "type": "text",
  "text": "The image shows a golden retriever playing in a park."
}
```

### 7.2 Tool Use Block

When the model wants to call a tool:

```json
{
  "type": "tool_use",
  "id": "call_abc123",
  "name": "web_search",
  "input": {
    "query": "current weather in Tokyo"
  }
}
```

### 7.3 Thinking Block

For models with reasoning capabilities (Claude with extended thinking, o1):

```json
{
  "type": "thinking",
  "thinking": "Let me break this problem down step by step...\n\n1. First, I need to consider...",
  "summary": "Analyzed the problem using step-by-step reasoning."
}
```

Enable via extensions:
```json
"extensions": {
  "anthropic": {
    "thinking": {"type": "enabled", "budget_tokens": 10000}
  }
}
```

### 7.4 Image Block (Generated)

When using image generation models or multimodal output:

```json
{
  "type": "image",
  "source": {
    "type": "base64",
    "media_type": "image/png",
    "data": "iVBORw0KGgo..."
  }
}
```

Enable via output config:
```json
"output": {
  "modalities": ["text", "image"],
  "image": {"size": "1024x1024"}
}
```

### 7.5 Audio Block (Generated)

When using voice output or native audio models:

```json
{
  "type": "audio",
  "source": {
    "type": "base64",
    "media_type": "audio/mp3",
    "data": "//uQxAAA..."
  },
  "transcript": "Hello, how can I help you today?"
}
```

---

## 8. Tools

### 8.1 Tool Types

| Type | Description | Executed By |
|------|-------------|-------------|
| `function` | Custom user-defined function | Client |
| `web_search` | Search the internet | Provider |
| `code_execution` | Execute code in sandbox | Provider |
| `file_search` | Search uploaded files | Provider |
| `computer_use` | Desktop automation | Provider |
| `text_editor` | File editing | Provider |

### 8.2 Function Tool (Custom)

```json
{
  "type": "function",
  "name": "get_weather",
  "description": "Get current weather for a location",
  "input_schema": {
    "type": "object",
    "properties": {
      "location": {
        "type": "string",
        "description": "City name or coordinates"
      },
      "units": {
        "type": "string",
        "enum": ["celsius", "fahrenheit"],
        "default": "celsius"
      }
    },
    "required": ["location"]
  }
}
```

### 8.3 Web Search Tool (Native)

```json
{
  "type": "web_search",
  "config": {
    "max_results": 5,
    "search_depth": "basic"
  }
}
```

**Provider Mapping**:
| Provider | Implementation |
|----------|----------------|
| Anthropic | `web_search_20250305` |
| OpenAI | `web_search` |
| Gemini | Google Search grounding |

### 8.4 Code Execution Tool (Native)

```json
{
  "type": "code_execution",
  "config": {
    "languages": ["python", "javascript"],
    "timeout_seconds": 30
  }
}
```

**Provider Mapping**:
| Provider | Implementation |
|----------|----------------|
| Anthropic | `code_execution_20250825` |
| OpenAI | `code_interpreter` |
| Gemini | Native code execution |

### 8.5 Computer Use Tool (Native)

```json
{
  "type": "computer_use",
  "config": {
    "display_width": 1920,
    "display_height": 1080
  }
}
```

**Provider Mapping**:
| Provider | Implementation |
|----------|----------------|
| Anthropic | `computer_20250124` |
| OpenAI | `computer_use` |

### 8.6 Tool Choice

Control how the model uses tools:

```json
"tool_choice": {"type": "auto"}       // Model decides
"tool_choice": {"type": "any"}        // Must use at least one tool
"tool_choice": {"type": "none"}       // Cannot use tools
"tool_choice": {"type": "tool", "name": "web_search"}  // Force specific tool
```

---

## 9. Streaming Protocol

When `stream: true`, responses are delivered as Server-Sent Events.

### 9.1 Event Types

| Event | Description |
|-------|-------------|
| `message_start` | Response begins |
| `content_block_start` | New content block |
| `content_block_delta` | Content chunk |
| `content_block_stop` | Block complete |
| `message_delta` | Metadata update |
| `message_stop` | Response complete |
| `error` | Error occurred |

### 9.2 Event Schemas

#### message_start
```json
event: message_start
data: {
  "type": "message_start",
  "message": {
    "id": "msg_abc123",
    "type": "message",
    "role": "assistant",
    "model": "anthropic/claude-sonnet-4",
    "content": [],
    "stop_reason": null,
    "usage": {"input_tokens": 1523, "output_tokens": 0}
  }
}
```

#### content_block_start
```json
event: content_block_start
data: {
  "type": "content_block_start",
  "index": 0,
  "content_block": {"type": "text", "text": ""}
}
```

For tool use:
```json
event: content_block_start
data: {
  "type": "content_block_start",
  "index": 1,
  "content_block": {"type": "tool_use", "id": "call_abc123", "name": "web_search", "input": {}}
}
```

#### content_block_delta
```json
event: content_block_delta
data: {
  "type": "content_block_delta",
  "index": 0,
  "delta": {"type": "text_delta", "text": "The weather"}
}
```

For tool input (streamed JSON):
```json
event: content_block_delta
data: {
  "type": "content_block_delta",
  "index": 1,
  "delta": {"type": "input_json_delta", "partial_json": "{\"query\": \"weather"}
}
```

For thinking:
```json
event: content_block_delta
data: {
  "type": "content_block_delta",
  "index": 0,
  "delta": {"type": "thinking_delta", "thinking": "Let me consider..."}
}
```

#### content_block_stop
```json
event: content_block_stop
data: {
  "type": "content_block_stop",
  "index": 0
}
```

#### message_delta
```json
event: message_delta
data: {
  "type": "message_delta",
  "delta": {"stop_reason": "end_turn"},
  "usage": {"output_tokens": 847}
}
```

#### message_stop
```json
event: message_stop
data: {"type": "message_stop"}
```

#### error
```json
event: error
data: {
  "type": "error",
  "error": {
    "type": "rate_limit_error",
    "message": "Rate limit exceeded. Please retry after 30 seconds."
  }
}
```

### 9.3 Voice Streaming Events

When `voice.output` is configured:

#### audio_delta
```json
event: audio_delta
data: {
  "type": "audio_delta",
  "delta": {
    "data": "//uQxAAA...",
    "format": "mp3"
  }
}
```

#### transcript_delta
```json
event: transcript_delta
data: {
  "type": "transcript_delta",
  "role": "user",
  "delta": {"text": "What's the"}
}
```

---

## 10. Voice Pipeline

The voice pipeline enables audio input/output on any model.

### 10.1 Configuration

```json
{
  "model": "anthropic/claude-sonnet-4",
  "messages": [...],
  "voice": {
    "input": {
      "provider": "deepgram",
      "model": "nova-2",
      "language": "en"
    },
    "output": {
      "provider": "elevenlabs",
      "voice": "rachel",
      "speed": 1.0
    }
  }
}
```

### 10.2 Pipeline Flow

```
┌─────────────────────────────────────────────────────────────┐
│                     Voice Pipeline                          │
│                                                             │
│  Audio Input ──► STT Provider ──► Transcribed Text         │
│                                          │                  │
│                                          ▼                  │
│                                   ┌──────────────┐          │
│                                   │  LLM Model   │          │
│                                   └──────────────┘          │
│                                          │                  │
│                                          ▼                  │
│                              LLM Text Response              │
│                                          │                  │
│                                          ▼                  │
│                               TTS Provider ──► Audio Output │
└─────────────────────────────────────────────────────────────┘
```

### 10.3 STT Providers

| Provider | Models | Languages |
|----------|--------|-----------|
| `deepgram` | `nova-2`, `nova`, `enhanced`, `base` | 30+ |
| `openai` | `whisper-1` | 50+ |
| `assemblyai` | `best`, `nano` | 10+ |

### 10.4 TTS Providers

| Provider | Voices | Quality |
|----------|--------|---------|
| `elevenlabs` | 100+ | High fidelity |
| `openai` | `alloy`, `echo`, `fable`, `onyx`, `nova`, `shimmer` | Good |
| `cartesia` | 20+ | Low latency |
| `playht` | 50+ | High fidelity |

### 10.5 Streaming Voice Response

When streaming with voice output:
1. Text chunks stream as `content_block_delta`
2. Text is buffered to sentence boundaries
3. Sentences are sent to TTS
4. Audio chunks stream as `audio_delta`

This provides low-latency audio while maintaining natural speech patterns.

---

## 11. The Live Endpoint

**Endpoint**: `WebSocket /v1/messages/live`

Real-time bidirectional voice communication using Vango's Universal Voice Pipeline. This endpoint works with **any text-based LLM** (Claude, GPT-4, Llama, Mistral) by orchestrating STT → LLM → TTS in real-time.

### 11.1 Unified Agent Architecture

**Core Principle:** The same agent definition works across all three modes:

| Mode | Transport | Input | Output | Use Case |
|------|-----------|-------|--------|----------|
| **Normal** | HTTP POST | Text | Text/Streaming | Chat, APIs |
| **Audio** | HTTP POST | Audio file | Audio + Text | Voice messages |
| **Live** | WebSocket | Real-time audio | Real-time audio | Conversations |

```json
{
  "model": "anthropic/claude-sonnet-4-20250514",
  "system": "You are a helpful travel assistant.",
  "tools": [{"name": "search_flights", ...}],
  "voice": {
    "input": {"provider": "cartesia"},
    "output": {"provider": "cartesia", "voice": "sonic-english"}
  }
}
```

This same config works for:
- `POST /v1/messages` with text → text response
- `POST /v1/messages` with audio content block → audio response
- `WS /v1/messages/live` → real-time bidirectional voice

**Key Features:**
- Hybrid VAD: Energy detection + semantic analysis for intelligent turn-taking
- User Grace Period: 5-second window to continue speaking after VAD commits
- Smart interruption: Semantic barge-in detection distinguishes real interrupts from backchannels
- Mid-session configuration: Change model, tools, or voice settings without reconnecting

### 11.2 Connection

```javascript
const ws = new WebSocket('wss://api.vai.dev/v1/messages/live', {
  headers: {
    'Authorization': 'Bearer vango_sk_...'
  }
});
```

### 11.3 Session Configuration

The first message **must** be `session.configure` with the agent definition:

```json
{
  "type": "session.configure",
  "config": {
    "model": "anthropic/claude-sonnet-4-20250514",
    "system": "You are a helpful voice assistant.",
    "tools": [...],
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
}
```

### 11.4 Voice Configuration Reference

#### VAD Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `model` | string | `cerebras/llama-3.1-8b` | Fast LLM for semantic turn completion |
| `energy_threshold` | float | `0.02` | RMS energy level for silence detection |
| `silence_duration_ms` | int | `600` | Silence duration before semantic check |
| `semantic_check` | bool | `true` | Enable semantic turn completion analysis |
| `min_words_for_check` | int | `2` | Minimum words before semantic check |
| `max_silence_ms` | int | `3000` | Force commit after this silence |

#### Grace Period Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable grace period for user continuation |
| `duration_ms` | int | `5000` | Window to continue speaking after VAD commits |

#### Interrupt Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `mode` | string | `auto` | `auto`, `manual`, or `disabled` |
| `energy_threshold` | float | `0.05` | RMS threshold (higher than VAD) |
| `capture_duration_ms` | int | `600` | Audio capture window before semantic check |
| `semantic_check` | bool | `true` | Distinguish interrupts from backchannels |
| `semantic_model` | string | `cerebras/llama-3.1-8b` | Fast LLM for interrupt detection |
| `save_partial` | string | `marked` | `discard`, `save`, or `marked` |

### 11.5 Client → Server Messages

#### session.configure (Required first message)
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

#### session.update (Change config mid-session)
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

#### input.interrupt (Force interrupt, skip semantic check)
```json
{
  "type": "input.interrupt",
  "transcript": "Actually, never mind."
}
```

#### input.commit (Force end-of-turn, e.g., push-to-talk release)
```json
{
  "type": "input.commit"
}
```

#### input.text (Send text directly for testing)
```json
{
  "type": "input.text",
  "text": "Hello, what can you help me with?"
}
```

#### tool_result (Return tool execution result)
```json
{
  "type": "tool_result",
  "tool_use_id": "toolu_01ABC",
  "content": [{"type": "text", "text": "The weather is sunny."}]
}
```

#### Binary Frames
Raw PCM audio: 16-bit signed integer, configurable sample rate (16kHz/24kHz/48kHz), mono.

### 11.6 Server → Client Messages

#### session.created
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

#### vad.listening / vad.analyzing / vad.silence
```json
{"type": "vad.listening"}
{"type": "vad.analyzing"}
{"type": "vad.silence", "duration_ms": 1500}
```

#### transcript.delta (Real-time transcription as user speaks)
```json
{
  "type": "transcript.delta",
  "delta": "Book me a"
}
```

#### input.committed (Turn complete, grace period starts)
```json
{
  "type": "input.committed",
  "transcript": "Book me a flight to Paris tomorrow"
}
```

#### grace_period.started
```json
{
  "type": "grace_period.started",
  "transcript": "Hello.",
  "duration_ms": 5000
}
```

#### grace_period.extended (User continued speaking)
```json
{
  "type": "grace_period.extended",
  "previous_transcript": "Hello.",
  "new_transcript": "Hello. How are you?"
}
```

#### grace_period.expired
```json
{
  "type": "grace_period.expired",
  "transcript": "Hello. How are you?"
}
```

#### content_block_start / content_block_delta / content_block_stop
Standard Vango streaming events, identical to HTTP streaming.

#### audio_delta (Audio output chunk)
```json
{
  "type": "audio_delta",
  "data": "<BASE64_PCM>"
}
```
Or sent as raw binary WebSocket frame.

#### interrupt.detecting (TTS paused, capturing audio)
```json
{
  "type": "interrupt.detecting"
}
```

#### interrupt.captured (Capture complete, checking semantic)
```json
{
  "type": "interrupt.captured",
  "transcript": "uh huh"
}
```

#### interrupt.dismissed (Not a real interrupt, TTS resuming)
```json
{
  "type": "interrupt.dismissed",
  "transcript": "uh huh",
  "reason": "backchannel"
}
```

#### response.interrupted (Real interrupt confirmed)
```json
{
  "type": "response.interrupted",
  "partial_text": "I'd be happy to help you book a fli—",
  "interrupt_transcript": "Actually wait",
  "audio_position_ms": 2340
}
```

#### message_stop
```json
{
  "type": "message_stop",
  "stop_reason": "end_turn"
}
```

#### error
```json
{
  "type": "error",
  "code": "vad_timeout",
  "message": "No speech detected for 30 seconds"
}
```

### 11.7 Connection Flow

```
Client                                          Server
   │                                               │
   │──────── WebSocket Connect ───────────────────▶│
   │                                               │
   │──────── session.configure (JSON) ────────────▶│
   │                                               │
   │◀─────── session.created (JSON) ──────────────│
   │                                               │
   │──────── Binary Audio Frames ─────────────────▶│
   │                                               │
   │◀─────── vad.listening (JSON) ────────────────│
   │◀─────── transcript.delta (JSON) ─────────────│
   │◀─────── input.committed (JSON) ──────────────│
   │◀─────── grace_period.started (JSON) ─────────│
   │                                               │
   │──────── Binary Audio (user continues) ───────▶│
   │◀─────── grace_period.extended (JSON) ────────│
   │                                               │
   │◀─────── grace_period.expired (JSON) ─────────│
   │◀─────── content_block_delta (JSON) ──────────│
   │◀─────── Binary Audio Frame ──────────────────│
   │                                               │
   │──────── Binary Audio (user interrupts) ──────▶│
   │◀─────── interrupt.detecting (JSON) ──────────│
   │◀─────── interrupt.captured (JSON) ───────────│
   │◀─────── response.interrupted (JSON) ─────────│
   │                                               │
```

### 11.8 Audio Format Requirements

| Parameter | Value | Notes |
|-----------|-------|-------|
| **Encoding** | PCM signed 16-bit | Little-endian |
| **Sample Rate** | 24000 Hz | Configurable: 16000, 24000, 48000 |
| **Channels** | 1 (mono) | Required |
| **Chunk Size** | 4096 bytes | ~85ms at 24kHz |

### 11.9 Implementation Note

Unlike proprietary realtime APIs, Vango's Live endpoint uses a **Universal Voice Pipeline** that works with any text-based LLM. The same agent definition (model, tools, system prompt) works identically across:

- `POST /v1/messages` — Text or audio file input
- `WS /v1/messages/live` — Real-time bidirectional voice

This allows developers to build agents that users can seamlessly switch between text chat, voice messages, and live conversations.

---

## 12. The Audio Endpoint

**Endpoint**: `POST /v1/audio`

Standalone audio utilities. The endpoint infers intent from request fields.

### 12.1 Transcription (STT)

**Trigger**: Request contains `audio` field

```json
// Request
{
  "audio": "base64_encoded_audio...",
  "provider": "deepgram",
  "model": "nova-2",
  "language": "en"
}

// Response
{
  "type": "transcription",
  "text": "Hello, how are you today?",
  "confidence": 0.98,
  "duration_seconds": 2.3,
  "language": "en",
  "words": [
    {"word": "Hello", "start": 0.0, "end": 0.5, "confidence": 0.99},
    {"word": "how", "start": 0.6, "end": 0.8, "confidence": 0.97}
  ]
}
```

### 12.2 Synthesis (TTS)

**Trigger**: Request contains `text` field

```json
// Request
{
  "text": "Hello, how can I help you today?",
  "provider": "elevenlabs",
  "voice": "rachel",
  "speed": 1.0,
  "format": "mp3"
}

// Response
{
  "type": "synthesis",
  "audio": "base64_encoded_audio...",
  "format": "mp3",
  "duration_seconds": 2.1
}
```

### 12.3 Streaming Synthesis

**Trigger**: Request contains `stream: true` with `text`

Response is `text/event-stream`:
```
event: audio_chunk
data: {"data": "//uQxAAA...", "index": 0}

event: audio_chunk
data: {"data": "//uQxBBB...", "index": 1}

event: done
data: {"duration_seconds": 2.1}
```

---

## 13. The Models Endpoint

**Endpoint**: `GET /v1/models`

List available models and their capabilities.

### 13.1 Response

```json
{
  "providers": [
    {
      "id": "anthropic",
      "name": "Anthropic",
      "models": [
        {
          "id": "claude-sonnet-4",
          "name": "Claude Sonnet 4",
          "full_id": "anthropic/claude-sonnet-4",
          "context_window": 200000,
          "max_output_tokens": 8192,
          "capabilities": {
            "text": true,
            "vision": true,
            "audio_input": false,
            "audio_output": false,
            "video": false,
            "tools": true,
            "tool_streaming": true,
            "thinking": true,
            "structured_output": true
          },
          "native_tools": ["web_search", "code_execution", "computer_use", "text_editor"],
          "pricing": {
            "input_per_mtok": 3.00,
            "output_per_mtok": 15.00,
            "cache_read_per_mtok": 0.30,
            "cache_write_per_mtok": 3.75
          }
        }
      ]
    },
    {
      "id": "openai",
      "name": "OpenAI",
      "models": [
        {
          "id": "gpt-4o",
          "name": "GPT-4o",
          "full_id": "openai/gpt-4o",
          "context_window": 128000,
          "max_output_tokens": 16384,
          "capabilities": {
            "text": true,
            "vision": true,
            "audio_input": true,
            "audio_output": true,
            "video": false,
            "tools": true,
            "tool_streaming": true,
            "thinking": false,
            "structured_output": true
          },
          "native_tools": ["web_search", "code_interpreter", "file_search"],
          "pricing": {
            "input_per_mtok": 2.50,
            "output_per_mtok": 10.00
          }
        }
      ]
    }
  ],
  "voice": {
    "stt": [
      {
        "id": "deepgram",
        "models": ["nova-2", "nova", "enhanced", "base"]
      }
    ],
    "tts": [
      {
        "id": "elevenlabs",
        "voices": ["rachel", "adam", "bella", "antoni"]
      }
    ]
  }
}
```

### 13.2 Filtering

```
GET /v1/models?provider=anthropic
GET /v1/models?capability=vision
GET /v1/models?capability=tools
```

---

## 14. Provider Translation

### 14.1 Supported Providers

| Provider | API Type | Notes |
|----------|----------|-------|
| Anthropic | Messages API | Native format |
| OpenAI | Chat Completions | Primary |
| OpenAI | Responses API | For image gen, file search |
| Google | Gemini API | Vertex or AI Studio |
| Gemini OAuth | Cloud Code Assist | Uses Google OAuth instead of API key |
| Groq | OpenAI-compatible | Fast inference |
| Mistral | Chat API | Similar to OpenAI |
| Together | OpenAI-compatible | Open models |
| Perplexity | OpenAI-compatible | Search-focused |

### 14.2 Request Translation

The proxy translates Vango AI (Anthropic-style) requests to provider formats:

#### Anthropic (passthrough)
No translation needed—native format.

#### OpenAI Translation

| Vango AI | OpenAI |
|-------|--------|
| `messages[].content` blocks | Flatten to string or `content` array |
| `{"type": "image", ...}` | `{"type": "image_url", "image_url": {...}}` |
| `{"type": "tool_use", ...}` | `tool_calls` array |
| `tools[].input_schema` | `tools[].function.parameters` |
| `stop_reason: "end_turn"` | `finish_reason: "stop"` |
| `stop_reason: "tool_use"` | `finish_reason: "tool_calls"` |

#### Gemini Translation

| Vango AI | Gemini |
|-------|--------|
| `messages` | `contents` |
| `{"type": "image", ...}` | `{"inlineData": {...}}` |
| `tools` | `functionDeclarations` |
| `tool_use` | `functionCall` |

### 14.3 Response Translation

All provider responses are normalized to Vango AI (Anthropic-style) format.

---

## 15. Error Handling

### 15.1 Error Response Schema

```json
{
  "type": "error",
  "error": {
    "type": "invalid_request_error",
    "message": "The 'messages' field is required.",
    "param": "messages",
    "code": "missing_required_field",
    "request_id": "req_abc123",
    "retry_after": 0
  }
}
```

### 15.2 Error Types

| Type | HTTP Status | Description |
|------|-------------|-------------|
| `invalid_request_error` | 400 | Malformed request |
| `authentication_error` | 401 | Invalid or missing API key |
| `permission_error` | 403 | Not allowed to access resource |
| `not_found_error` | 404 | Model or resource not found |
| `rate_limit_error` | 429 | Too many requests |
| `api_error` | 500 | Internal server error |
| `overloaded_error` | 503 | Service temporarily unavailable |
| `provider_error` | 502 | Upstream provider error |

### 15.3 Rate Limit Headers

```http
HTTP/1.1 429 Too Many Requests
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 2025-12-22T21:35:00Z
Retry-After: 30
```

### 15.4 Provider Error Passthrough

When a provider returns an error, it's wrapped:

```json
{
  "type": "error",
  "error": {
    "type": "provider_error",
    "message": "Anthropic API error: Overloaded",
    "provider": "anthropic",
    "provider_error": {
      "type": "overloaded_error",
      "message": "Overloaded"
    }
  }
}
```

---

## 16. Observability

### 16.1 Metrics Endpoint

`GET /metrics` — Prometheus format

```prometheus
# HELP Vango AI_requests_total Total number of API requests
# TYPE Vango AI_requests_total counter
Vango AI_requests_total{provider="anthropic",model="claude-sonnet-4",status="success"} 1234
Vango AI_requests_total{provider="openai",model="gpt-4o",status="error"} 12

# HELP Vango AI_request_duration_seconds Request duration in seconds
# TYPE Vango AI_request_duration_seconds histogram
Vango AI_request_duration_seconds_bucket{provider="anthropic",le="1"} 500
Vango AI_request_duration_seconds_bucket{provider="anthropic",le="5"} 1100

# HELP Vango AI_tokens_total Total tokens processed
# TYPE Vango AI_tokens_total counter
Vango AI_tokens_total{provider="anthropic",direction="input"} 2456789
Vango AI_tokens_total{provider="anthropic",direction="output"} 1234567

# HELP Vango AI_cost_usd_total Total cost in USD
# TYPE Vango AI_cost_usd_total counter
Vango AI_cost_usd_total{provider="anthropic"} 47.23
```

### 16.2 Request Logging

Structured JSON logs:

```json
{
  "timestamp": "2025-12-22T21:35:00Z",
  "level": "info",
  "event": "request_complete",
  "request_id": "req_abc123",
  "model": "anthropic/claude-sonnet-4",
  "provider": "anthropic",
  "input_tokens": 1523,
  "output_tokens": 847,
  "total_tokens": 2370,
  "cost_usd": 0.0142,
  "duration_ms": 2341,
  "stop_reason": "end_turn",
  "status": "success",
  "user_id": "user_xyz",
  "ip": "192.168.1.1"
}
```

### 16.3 OpenTelemetry Tracing

Traces are exported in OTLP format:

```
[Trace: req_abc123]
├─ vai.handle_request (2341ms)
│  ├─ vai.auth.validate (2ms)
│  ├─ vai.translate.request (5ms)
│  ├─ vai.provider.anthropic (2300ms)
│  │  └─ http.request api.anthropic.com (2290ms)
│  ├─ vai.translate.response (3ms)
│  └─ vai.response.encode (1ms)
```

### 16.4 Health Endpoint

`GET /health`

```json
{
  "status": "healthy",
  "version": "1.0.0",
  "providers": {
    "anthropic": {"status": "healthy", "latency_ms": 150},
    "openai": {"status": "healthy", "latency_ms": 200},
    "gemini": {"status": "degraded", "latency_ms": 2500, "error": "high latency"}
  }
}
```

---

## 17. Configuration

### 17.1 Configuration File

```yaml
# vai.yaml

server:
  host: "0.0.0.0"
  port: 8080
  tls:
    enabled: false
    cert_file: "/path/to/cert.pem"
    key_file: "/path/to/key.pem"

auth:
  mode: "api_key"  # api_key, passthrough, none
  api_keys:
    - key: "Vango AI_sk_abc..."
      name: "Production"
      user_id: "user_123"
      rate_limit: 1000  # requests per minute

providers:
  anthropic:
    api_key: "${ANTHROPIC_API_KEY}"
    base_url: "https://api.anthropic.com"
  openai:
    api_key: "${OPENAI_API_KEY}"
  gemini:
    api_key: "${GOOGLE_API_KEY}"
  groq:
    api_key: "${GROQ_API_KEY}"

voice:
  stt:
    deepgram:
      api_key: "${DEEPGRAM_API_KEY}"
    openai:
      api_key: "${OPENAI_API_KEY}"
  tts:
    elevenlabs:
      api_key: "${ELEVENLABS_API_KEY}"
    cartesia:
      api_key: "${CARTESIA_API_KEY}"

aliases:
  fast: "groq/llama-3.3-70b"
  smart: "anthropic/claude-sonnet-4"
  cheap: "openai/gpt-4o-mini"
  vision: "gemini/gemini-2.0-flash"

defaults:
  max_tokens: 4096
  temperature: 1.0

rate_limits:
  global:
    requests_per_minute: 5000
    tokens_per_minute: 1000000
  per_user:
    requests_per_minute: 100
    tokens_per_minute: 50000

observability:
  metrics:
    enabled: true
    path: "/metrics"
  logging:
    level: "info"
    format: "json"
  tracing:
    enabled: true
    exporter: "otlp"
    endpoint: "localhost:4317"
```

### 17.2 Environment Variables

| Variable | Description |
|----------|-------------|
| `Vango AI_CONFIG` | Path to config file |
| `Vango AI_PORT` | Server port (overrides config) |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `OPENAI_API_KEY` | OpenAI API key |
| `GOOGLE_API_KEY` | Google API key |
| `GEMINI_OAUTH_PROJECT_ID` | Google Cloud project ID for Gemini OAuth |
| `GEMINI_OAUTH_CREDENTIALS_PATH` | Custom path for OAuth credentials file |
| `GROQ_API_KEY` | Groq API key |
| `DEEPGRAM_API_KEY` | Deepgram STT key |
| `ELEVENLABS_API_KEY` | ElevenLabs TTS key |

---

## 18. Rate Limiting

### 18.1 Rate Limit Tiers

| Tier | Requests/min | Tokens/min | Concurrent |
|------|--------------|------------|------------|
| Free | 20 | 10,000 | 5 |
| Pro | 500 | 500,000 | 50 |
| Enterprise | Custom | Custom | Custom |

### 18.2 Rate Limit Response

```http
HTTP/1.1 429 Too Many Requests
Content-Type: application/json
X-RateLimit-Limit-Requests: 100
X-RateLimit-Remaining-Requests: 0
X-RateLimit-Reset-Requests: 2025-12-22T21:36:00Z
X-RateLimit-Limit-Tokens: 50000
X-RateLimit-Remaining-Tokens: 12000
Retry-After: 30

{
  "type": "error",
  "error": {
    "type": "rate_limit_error",
    "message": "Request rate limit exceeded. Please retry after 30 seconds.",
    "retry_after": 30
  }
}
```

---

## 19. Request & Response Examples

### 19.1 Simple Text Generation

**Request:**
```json
POST /v1/messages
Authorization: Bearer Vango AI_sk_...

{
  "model": "anthropic/claude-sonnet-4",
  "max_tokens": 1024,
  "messages": [
    {"role": "user", "content": "What is the capital of France?"}
  ]
}
```

**Response:**
```json
{
  "id": "msg_abc123",
  "type": "message",
  "role": "assistant",
  "model": "anthropic/claude-sonnet-4",
  "content": [
    {"type": "text", "text": "The capital of France is Paris."}
  ],
  "stop_reason": "end_turn",
  "usage": {
    "input_tokens": 15,
    "output_tokens": 10,
    "total_tokens": 25,
    "cost_usd": 0.0002
  }
}
```

### 19.2 Vision + Tools

**Request:**
```json
{
  "model": "openai/gpt-4o",
  "max_tokens": 4096,
  "messages": [
    {
      "role": "user",
      "content": [
        {"type": "text", "text": "What's in this image? Search for more information about it."},
        {"type": "image", "source": {"type": "url", "url": "https://upload.wikimedia.org/wikipedia/commons/a/a7/Eiffel_Tower.jpg"}}
      ]
    }
  ],
  "tools": [
    {"type": "web_search"}
  ]
}
```

**Response (first turn):**
```json
{
  "id": "msg_def456",
  "type": "message",
  "role": "assistant",
  "model": "openai/gpt-4o",
  "content": [
    {"type": "text", "text": "This is the Eiffel Tower in Paris, France. Let me search for more information about it."},
    {"type": "tool_use", "id": "call_xyz", "name": "web_search", "input": {"query": "Eiffel Tower history facts"}}
  ],
  "stop_reason": "tool_use",
  "usage": {"input_tokens": 1500, "output_tokens": 50}
}
```

### 19.3 Streaming with Voice

**Request:**
```json
{
  "model": "anthropic/claude-sonnet-4",
  "stream": true,
  "messages": [
    {
      "role": "user",
      "content": [
        {"type": "audio", "source": {"type": "base64", "media_type": "audio/wav", "data": "..."}}
      ]
    }
  ],
  "voice": {
    "input": {"provider": "deepgram", "model": "nova-2"},
    "output": {"provider": "elevenlabs", "voice": "rachel"}
  }
}
```

**Response (SSE):**
```
event: transcript_delta
data: {"type": "transcript_delta", "role": "user", "delta": {"text": "What's the weather like today?"}}

event: message_start
data: {"type": "message_start", "message": {"id": "msg_ghi789", "role": "assistant"}}

event: content_block_start
data: {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}}

event: content_block_delta
data: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "I don't have access to"}}

event: audio_delta
data: {"type": "audio_delta", "delta": {"data": "//uQxAAA...", "format": "mp3"}}

event: content_block_delta
data: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": " real-time weather data."}}

event: audio_delta
data: {"type": "audio_delta", "delta": {"data": "//uQxBBB...", "format": "mp3"}}

event: content_block_stop
data: {"type": "content_block_stop", "index": 0}

event: message_delta
data: {"type": "message_delta", "delta": {"stop_reason": "end_turn"}, "usage": {"output_tokens": 25}}

event: message_stop
data: {"type": "message_stop"}
```

### 19.4 Structured Output

**Request:**
```json
{
  "model": "openai/gpt-4o",
  "messages": [
    {"role": "user", "content": "Extract the person and company from: 'Tim Cook is the CEO of Apple Inc.'"}
  ],
  "output_format": {
    "type": "json_schema",
    "json_schema": {
      "type": "object",
      "properties": {
        "person": {"type": "string"},
        "role": {"type": "string"},
        "company": {"type": "string"}
      },
      "required": ["person", "role", "company"]
    }
  }
}
```

**Response:**
```json
{
  "id": "msg_jkl012",
  "type": "message",
  "role": "assistant",
  "model": "openai/gpt-4o",
  "content": [
    {"type": "text", "text": "{\"person\": \"Tim Cook\", \"role\": \"CEO\", \"company\": \"Apple Inc.\"}"}
  ],
  "stop_reason": "end_turn"
}
```

---

## 20. OpenAPI Schema

A full OpenAPI 3.1 specification is available at:

- **Hosted**: `https://api.vai.dev/openapi.yaml`
- **Repository**: `/openapi/Vango AI-api.yaml`

The schema can be used to:
- Generate clients in any language
- Validate requests/responses
- Generate documentation
- Power API explorers

---

## Appendix A: Provider Model IDs

### Anthropic
- `anthropic/claude-sonnet-4`
- `anthropic/claude-opus-4`
- `anthropic/claude-haiku-3`
- `anthropic/claude-3-5-sonnet`
- `anthropic/claude-3-opus`

### OpenAI
- `openai/gpt-4o`
- `openai/gpt-4o-mini`
- `openai/gpt-4-turbo`
- `openai/o1`
- `openai/o1-mini`
- `openai/gpt-4o-realtime` (for Live endpoint)

### Google Gemini

**Gemini 3 Series (Preview):**
- `gemini/gemini-3-pro-preview` - Most intelligent model with state-of-the-art reasoning
- `gemini/gemini-3-flash-preview` - Speed-optimized with improved reasoning
- `gemini/gemini-3-pro-image-preview` - Image generation with thinking and grounding

**Gemini 2.5 Series:**
- `gemini/gemini-2.5-pro` - Powerful model for complex tasks
- `gemini/gemini-2.5-flash` - Balanced model with 1M token context window
- `gemini/gemini-2.5-flash-lite` - Fast and cost-efficient

**Gemini 2.0 Series:**
- `gemini/gemini-2.0-flash` - Latest stable Flash model

### Gemini OAuth

Uses Google OAuth authentication via Cloud Code Assist endpoint. Allows using your existing Gemini plan quota instead of API billing. Same models as Google Gemini but with `gemini-oauth/` prefix:

- `gemini-oauth/gemini-2.5-flash` - Balanced model (recommended)
- `gemini-oauth/gemini-2.5-pro` - Powerful model for complex tasks
- `gemini-oauth/gemini-2.0-flash` - Latest stable Flash model

**Setup:** Run OAuth login first to store credentials at `~/.config/vango/gemini-oauth-credentials.json`, then set `GEMINI_OAUTH_PROJECT_ID` environment variable.

### Groq
- `groq/llama-3.3-70b`
- `groq/llama-3.1-70b`
- `groq/mixtral-8x7b`

### Mistral
- `mistral/mistral-large`
- `mistral/mistral-small`
- `mistral/codestral`

---

## Appendix B: Changelog

### v1.0.0 (December 2025)
- Initial release
- Core Messages API
- Provider support: Anthropic, OpenAI, Gemini, Groq, Mistral
- Voice pipeline with Deepgram + ElevenLabs
- Native tool normalization
- Streaming support
- Observability (metrics, logging, tracing)

---

*Document Version: 1.0.0*  
*Last Updated: December 2025*  
*Status: Implementation Ready*
