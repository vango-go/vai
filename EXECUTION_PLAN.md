# VAI Launch Execution Plan (Proxy-Ready)

This plan is optimized for shipping a coherent product by fully implementing the proxy, aligning SDK/docs, and hardening for production. It assumes direct mode is acceptable for launch, but proxy must be complete and reliable.

## Goals
- Fully functional VAI Proxy with non-streaming, streaming, audio, and live endpoints.
- SDK Proxy Mode implemented and consistent with API behavior.
- Documentation and examples match shipped behavior.
- Operational readiness: auth, rate limiting, logging, metrics, and deployment artifacts.

## Non-Goals (For This Launch)
- Adding new providers beyond current direct-mode set (unless needed for parity).
- Building a hosted billing system or SaaS admin dashboard.

## Phase 0: Alignment and Scope Lock
**Outcome:** Clear, signed-off feature matrix for launch.

Tasks
- Define launch feature matrix (direct vs proxy): supported providers, endpoints, auth modes, streaming behavior, live voice, audio STT/TTS, BYOK, models listing.
- Decide which “planned” features are explicitly deferred and how they are labeled.
- Decide whether BYOK in proxy is supported via headers/body (recommended) or not at launch.

Acceptance Criteria
- Single-page feature matrix approved by stakeholders.
- List of deferred features with explicit “not supported” messaging.

## Phase 1: Proxy Core API Implementation
**Outcome:** Proxy implements all advertised endpoints and behavior.

1. `/v1/messages` (non-streaming)
- Use core engine to route requests.
- Support provider/model parsing.
- Return normalized response format.
- Include response headers (request id, tokens, model, duration).

2. `/v1/messages` (streaming SSE)
- Implement SSE for `stream: true`.
- Pass through event types: message_start, content_block_start/delta/stop, message_delta/stop, error.
- Ensure correct flushing and termination.

3. `/v1/messages/live` (WebSocket)
- Implement WebSocket endpoint for live sessions.
- Use `pkg/core/live` session logic.
- Enforce concurrency and idle timeout limits.
- Handle audio input, transcript deltas, response audio output, interrupts.

4. `/v1/audio`
- Implement transcription (STT) via Cartesia provider.
- Implement synthesis (TTS) via Cartesia provider.
- Support streaming audio output if documented.

5. `/v1/models`
- Enumerate providers registered in proxy.
- Return capabilities and metadata.
- Add filtering if documented (provider/capability).

Acceptance Criteria
- All endpoints respond with 2xx for valid requests.
- Streaming endpoints pass compliance tests (SSE/WS behavior).
- `/v1/audio` returns expected formats.
- `/v1/models` returns accurate provider list and capabilities.

## Phase 2: Provider Registration and Parity
**Outcome:** Proxy provider support matches direct mode or documented scope.

Tasks
- Register same providers as direct mode: anthropic, openai, oai_resp, groq, gemini, gemini_oauth (if supported), cerebras.
- Ensure provider keys can be configured via config/env.
- Implement provider capability aggregation for `/v1/models`.

Acceptance Criteria
- Proxy supports all providers listed in docs.
- Each provider validated with a basic message request.

## Phase 3: SDK Proxy Mode Implementation
**Outcome:** SDK works transparently in proxy mode.

Tasks
- Implement `Messages.Create` proxy path (HTTP POST /v1/messages).
- Implement `Messages.Stream` proxy path (SSE parser).
- Implement `ModelsService.List/Get` proxy path (HTTP GET /v1/models).
- Implement `AudioService` proxy path (HTTP POST /v1/audio).
- Add auth headers (Authorization Bearer or X-API-Key).
- Ensure retries/backoff apply to proxy requests.

Acceptance Criteria
- SDK can switch between direct/proxy by providing base URL.
- Streaming works with proxy via SDK.
- Errors from proxy are mapped to SDK error types.

## Phase 4: Auth Modes and BYOK
**Outcome:** Auth is robust and supports BYOK as documented.

Tasks
- Implement auth modes: api_key, passthrough, none (if in scope).
- BYOK support:
  - Parse per-request provider keys from headers and/or request body.
  - Merge or override server provider keys as configured.
  - Pass to core engine for provider calls.
- Add request context with user_id and key metadata.

Acceptance Criteria
- Requests with per-user API keys authenticate correctly.
- BYOK keys correctly route calls without exposing server keys.
- Unauthorized access returns consistent error payloads.

## Phase 5: Rate Limiting and Quotas
**Outcome:** Rate limits enforced per user and globally.

Tasks
- Implement token-based rate limiting (tokens/min).
- Enforce per-user limits with per-key overrides.
- Apply live session concurrency limits and idle timeouts.
- Ensure rate limit headers are emitted on 429.

Acceptance Criteria
- Requests exceeding RPM/TPM are blocked.
- Rate limit headers match docs.
- Live sessions are limited and cleaned up reliablyan 

## Phase 6: Observability
**Outcome:** Full metrics/logging/tracing coverage of proxy.

Tasks
- Metrics: requests, tokens, duration, errors, rate limits, live sessions.
- Logging: structured logs for request start/complete, errors, auth context.
- Tracing: enable if documented, otherwise remove docs references.

Acceptance Criteria
- Metrics endpoint emits Prometheus metrics for all endpoints.
- Logs include request_id, user_id, model/provider.

## Phase 7: Error Handling and Compatibility
**Outcome:** All errors are consistent and documented.

Tasks
- Standardize error schema across endpoints and streaming.
- Map provider errors to `provider_error`.
- Ensure streaming errors send `event: error` then terminate cleanly.

Acceptance Criteria
- All error responses match the spec.
- SDK error handling works in proxy mode.

## Phase 8: Configuration and Deployment
**Outcome:** Proxy can be deployed in real environments.

Tasks
- Implement config file loading (yaml/json) for proxy.
- Provide working proxy binary entrypoint in `cmd/proxy`.
- Provide Dockerfile and env var references.
- Add sample config file with minimal settings.

Acceptance Criteria
- Proxy runs via `go run ./cmd/proxy`.
- Docker image builds and runs with sample config.

## Phase 9: Documentation Alignment
**Outcome:** Docs accurately reflect shipped behavior.

Tasks
- Update `docs/API_SPEC.md` to match proxy implementation.
- Update `docs/SDK_SPEC.md` to reflect actual SDK capabilities.
- Align README with module path and actual supported features.
- Fix all example import paths and identifiers.

Acceptance Criteria
- All examples compile against current module.
- Docs do not describe unimplemented features.

## Phase 10: Testing and Verification
**Outcome:** Confidence in release stability.

Tests to Add
- Proxy:
  - `/v1/messages` non-streaming
  - `/v1/messages` streaming SSE
  - `/v1/messages/live` WS session
  - `/v1/audio` STT/TTS
  - `/v1/models` accuracy
  - Auth and rate limiting
- SDK:
  - Direct mode regression (existing tests)
  - Proxy mode message + stream

Acceptance Criteria
- All tests pass locally and in CI (if present).
- Manual smoke test documented and repeatable.

## Phase 11: Release Hygiene
**Outcome:** Clean release artifacts and versioning.

Tasks
- Confirm Go version in `go.mod` is valid and documented.
- Add versioning policy (API + SDK).
- Add release notes / changelog.

Acceptance Criteria
- Build/install instructions validated.
- Release notes reflect real scope.

---

## Dependencies and Sequencing
- Phase 0 must be done before Phase 9.
- Phase 1 and 3 can be parallelized but must complete before docs finalization.
- Phase 4 and 5 depend on auth design decisions.
- Phase 8 depends on a working proxy implementation.

## Risks and Mitigations
- Risk: Streaming edge cases (SSE/WS backpressure).
  - Mitigation: Add integration tests with timeouts and large payloads.
- Risk: BYOK handling could leak or override server keys.
  - Mitigation: Explicit precedence rules + tests for key handling.
- Risk: Docs drift during development.
  - Mitigation: Update docs as part of each phase’s acceptance.

## Deliverables
- Fully implemented proxy endpoints.
- SDK proxy mode support.
- Updated docs + README + examples.
- Deployment artifacts (binary + Docker + sample config).
- Test suite for proxy and SDK proxy mode.

