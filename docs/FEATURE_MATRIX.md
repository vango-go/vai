# VAI Launch Feature Matrix (Direct vs Proxy)

This document defines the launch scope and the features that are in‑scope vs. deferred.

## 1) Scope Summary

### Direct Mode (Go SDK)
In scope:
- Providers: anthropic, openai, oai-resp, groq, gemini, gemini-oauth, cerebras
- Messages: non-streaming + streaming
- Tools: request/response tool flow supported
- Voice (STT/TTS): Cartesia (audio input + synthesized output)
- Live voice: RunStream + WithLive (direct mode)
- Audio utilities: STT/TTS helpers (Cartesia)

Out of scope:
- Provider keys via per-request headers/body (direct mode uses env/WithProviderKey)

### Proxy Mode (HTTP Server)
In scope:
- Providers: anthropic, openai, oai-resp, groq, gemini, gemini-oauth, cerebras
- Endpoints:
  - `POST /v1/messages` (non-streaming + streaming SSE)
  - `POST /v1/audio` (STT, TTS, streaming TTS)
  - `GET /v1/models` (provider capability listing)
  - `GET /health`
  - `GET /metrics` (if enabled)
  - `WS /v1/messages/live` (live voice sessions)
- Auth modes: `api_key`, `passthrough`, `none`
- BYOK (proxy): supported via `X-Provider-Key-*` headers and `provider_keys` body field
- CORS configuration and request body limits
- Rate limits: enabled by default with high limits (configurable)

Out of scope (deferred):
- Provider billing/usage aggregation beyond metrics
- Token-based (TPM) rate limiting (RPM only)
- Per-org quotas without proxy auth (requires org identity)
- Full model catalog per provider (only capability listing)

---

## 2) Detailed Feature Matrix

### Providers
| Provider | Direct Mode | Proxy Mode | Notes |
|---|---|---|---|
| Anthropic | ✅ | ✅ | |
| OpenAI (Chat) | ✅ | ✅ | |
| OpenAI Responses (`oai-resp`) | ✅ | ✅ | |
| Groq | ✅ | ✅ | |
| Gemini (API key) | ✅ | ✅ | Uses `GEMINI_API_KEY`/`GOOGLE_API_KEY` |
| Gemini OAuth | ✅ | ✅ | Requires OAuth credentials and project id |
| Cerebras | ✅ | ✅ | |
| Mistral/Together/Perplexity | ❌ | ❌ | Deferred |

### Endpoints / Services
| Feature | Direct Mode | Proxy Mode | Notes |
|---|---|---|---|
| Messages (non-streaming) | ✅ | ✅ | `POST /v1/messages` |
| Messages (streaming) | ✅ | ✅ | SSE |
| Audio STT | ✅ | ✅ | Cartesia only |
| Audio TTS | ✅ | ✅ | Cartesia only |
| Audio TTS streaming | ✅ | ✅ | SSE |
| Live Voice (real-time) | ✅ | ✅ | WebSocket on proxy |
| Models listing | ⚠️ | ✅ | Proxy returns provider capabilities only |

### Auth & BYOK
| Feature | Direct Mode | Proxy Mode | Notes |
|---|---|---|---|
| API key auth | N/A | ✅ | `api_key` mode |
| Passthrough auth | N/A | ✅ | `passthrough` mode |
| No auth | N/A | ✅ | `none` mode |
| BYOK via headers | N/A | ✅ | `X-Provider-Key-*` |
| BYOK via body | N/A | ✅ | `provider_keys` field |

### Voice / Live
| Feature | Direct Mode | Proxy Mode | Notes |
|---|---|---|---|
| STT provider | ✅ | ✅ | Cartesia |
| TTS provider | ✅ | ✅ | Cartesia |
| Live VAD/interrupts | ✅ | ✅ | Uses `pkg/core/live` |
| Live session update | ❌ | ❌ | Deferred |

### Rate Limits / Safety
| Feature | Direct Mode | Proxy Mode | Notes |
|---|---|---|---|
| RPM limits | N/A | ✅ | Configurable, high defaults |
| TPM limits | N/A | ❌ | Deferred |
| Live concurrency cap | N/A | ✅ | Configurable |
| Idle timeout | N/A | ✅ | Configurable |
| Request size limit | N/A | ✅ | Configurable |

---

## 3) Deferred Features (Explicitly Not Supported at Launch)
- Token-based (TPM) rate limiting and token buckets
- Per-org quotas without proxy auth/identity
- Live session config updates (`session.update`)
- Full per-model catalog in `/v1/models` (capabilities only for now)
- Additional providers: Mistral, Together, Perplexity

---

## 4) BYOK Decision

Launch decision: **BYOK is supported in proxy mode**.

Supported mechanisms:
- Request headers: `X-Provider-Key-Anthropic`, `X-Provider-Key-OpenAI`, etc.
- Request body: `provider_keys` object (per request)

Precedence:
1) Request headers/body BYOK keys
2) Proxy server config/env provider keys (fallback)

---

## 5) Release Checklist for Scope Lock
- [ ] Confirm providers list for launch.
- [ ] Confirm proxy auth modes exposed publicly.
- [ ] Confirm byok mechanism and precedence.
- [ ] Confirm rate limit defaults (and whether TPM is deferred).
- [ ] Confirm `/v1/models` response shape is acceptable for launch.

