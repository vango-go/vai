# RUN_UPDATE_IMPLEMENTATION_PLAN

Version: 0.2 (event-driven history design)

This plan describes a new, event-first design for RunStream history handling. The intent is to make history updates explicit and easy to control without adding `WithHistory(...)` or other hidden state. The caller owns a `[]Message` and updates it by consuming RunStream events (with a default helper available).

---

## 0) Goals and Non-Goals

### Goals
- Make history updates explicit and event-driven.
- Preserve current default behavior via a **DefaultHistoryHandler** helper.
- Avoid hidden state or implicit internal history accumulation in RunStream.
- Keep RunStream ergonomic for simple use cases.
- Keep RunStream event stream sufficiently rich to reconstruct history deterministically.

### Non-Goals
- Do not redesign Live mode in this change (only normal RunStream/Run).
- Do not change tool execution semantics (still handled in the RunStream loop).
- Do not change provider wire formats or core types.

---

## 1) Current State Summary (as of repo HEAD)

### Where history is currently handled
- `sdk/run.go`:
  - `runLoop()` creates a local `messages` slice and appends assistant + tool results per turn.
  - `RunStream.run()` holds `rs.messages` and appends assistant + tool results per step.
  - Interrupt handling appends partial assistant messages in-place.

### Limitations
- History mutation is internal and not observable except by reusing `req.Messages` manually between calls.
- There is no standardized helper for history mutation based on events.
- For RunStream, callers see events but must reverse-engineer how to update history.

---

## 2) Proposed Design Overview

Make RunStream events **sufficient to reconstruct history**, and provide a default helper that updates a `[]Message` based on those events.

### Key Ideas
- History updates happen in userland (caller-controlled `[]Message`).
- RunStream emits a **HistoryDeltaEvent** to make history updates trivial and precise.
- Default handler: `DefaultHistoryHandler(&msgs)` consumes events and applies deltas.
- No `WithHistory(...)` RunOption. No internal hidden history mutation for RunStream.

---

## 3) Required Event Contract

The event stream must expose enough data to update history deterministically.

### 3.1) Existing Events Used
- `StepCompleteEvent{Response}`
  - Contains full assistant message content
- `ToolResultEvent{ID, Name, Content, Error}`
  - Contains tool outputs for tool_result blocks
- `InterruptedEvent{PartialText, Behavior}`
  - Allows partial assistant message insertion

### 3.2) New Event: HistoryDeltaEvent

Add a new event that explicitly encodes the message(s) to append to history.

```go
type HistoryDeltaEvent struct {
    Append []types.Message `json:"append"`
}

func (e HistoryDeltaEvent) runStreamEventType() string { return "history_delta" }
```

**When emitted (non-live RunStream only):**
- After a step completes and tool results (if any) are known, emit a single delta
  containing:
  1) assistant message (full content)
  2) tool_result message (if any)
- On interrupt (when partial is saved), emit a delta containing the partial assistant message.

This makes the default handler trivial and lets advanced callers apply custom logic.

---

## 4) Default History Handler (Helper API)

Provide a helper that applies deltas to a user-owned message slice.

```go
func DefaultHistoryHandler(msgs *[]types.Message) func(event RunStreamEvent)
```

### Behavior
- On `HistoryDeltaEvent`: append `event.Append` to `*msgs`.
- On `StepCompleteEvent` and `ToolResultEvent`: **no-op** (delta already emitted).
- On `InterruptedEvent`: **no-op** (delta already emitted).

This keeps behavior explicit and avoids double-appends.

### Optional Advanced Helper
- `NewHistoryAccumulator()` that can listen to events and expose `Messages()`.
- Not required if you adopt the event-handler approach.

---

## 5) RunStream Internal Changes

### 5.1) Remove internal history ownership
- Stop mutating `rs.messages` as the canonical history.
- Instead, maintain a **local working copy** just for building the next turn request.

### 5.2) Emit HistoryDeltaEvent
- After each step completes and tool results are available, emit `HistoryDeltaEvent`.
- When interrupt saves partial assistant text, emit `HistoryDeltaEvent` with that message.

### 5.3) Maintain request history locally
Because RunStream still needs history for the next turn, it must track the same sequence.

Options:
- Keep an internal `localHistory []Message` and append *the same deltas* you emit.
- This internal slice is not exposed; it is only for building the next turn request.

This preserves the current run-loop behavior without forcing the user to provide updated history mid-run.

---

## 6) Run (Non-Streaming) Changes

### 6.1) Expose updated history
- Add `Messages []types.Message` to `RunResult`.
- This mirrors the event-based approach by providing a boundary snapshot.

### 6.2) Internal history updates
- `runLoop()` can keep internal history logic unchanged.
- At end, set `result.Messages` to the internal history.

Note: Run (non-streaming) does not emit events; the history snapshot is the only external artifact.

---

## 7) Detailed Implementation Plan

### Step 1: Add HistoryDeltaEvent
**Files:** `sdk/run.go`

1. Add new event type:
   - `HistoryDeltaEvent` struct + `runStreamEventType()`
2. Ensure event is included in public SDK types (same package).

---

### Step 2: Add DefaultHistoryHandler Helper
**Files:** `sdk/history.go` (new)

1. Create `DefaultHistoryHandler(msgs *[]types.Message) func(RunStreamEvent)`.
2. Handler appends on `HistoryDeltaEvent` only.
3. Provide a small test verifying:
   - Append on delta
   - No-op on other events

---

### Step 3: Update RunStream Loop to Emit Deltas
**Files:** `sdk/run.go`

#### 3.1) Internal history slice
- Replace `rs.messages` as “the” history with a local slice, e.g. `history []types.Message`.
- Seed from `req.Messages` at run start.

#### 3.2) Append history locally
- On step completion:
  - Append assistant message (full response content) to `history`.
  - If tool results exist, append tool_result message to `history`.

#### 3.3) Emit `HistoryDeltaEvent`
- Build delta messages with same content used to append locally.
- Emit `HistoryDeltaEvent{Append: delta}`.

#### 3.4) Interrupt handling
- When interrupt saves partial (SavePartial / SaveMarked):
  - Append partial assistant message to `history`.
  - Emit `HistoryDeltaEvent` with that partial message.
- Do **not** emit delta on `InterruptDiscard`.

#### 3.5) Remove external message mutation
- Do not mutate `req.Messages` or external slices.

---

### Step 4: Expose final history in RunResult
**Files:** `sdk/run.go`

1. Add `Messages []types.Message` to `RunResult`.
2. Set it in `runLoop()` and `RunStream.run()` when done.

---

### Step 5: Update Documentation and Examples

**Files:**
- `README.md`
- `docs/SDK_SPEC.md`

Add examples:

```go
msgs := GetMessagesFromS3()
req := &vai.MessageRequest{Model: model, Messages: msgs}

stream, _ := client.Messages.RunStream(ctx, req)
apply := vai.DefaultHistoryHandler(&msgs)

for event := range stream.Events() {
    apply(event)
    // custom logic
}

// msgs now contains updated history
```

Also show `Run()` usage with `result.Messages`.

---

## 8) Tests

### Unit Tests
- `sdk/history_test.go`:
  - Handler appends on `HistoryDeltaEvent`
  - Handler ignores other events

### Integration Tests
- Update existing RunStream tests to assert:
  - HistoryDeltaEvent emitted per step
  - Deltas include assistant message and tool_result blocks

---

## 9) Open Questions / Decisions

1. Should HistoryDeltaEvent be emitted **before** or **after** StepCompleteEvent?
   - Recommendation: **after** StepCompleteEvent so consumers can inspect response first.

2. Should delta contain the tool_result message if tool execution fails?
   - Recommendation: Yes. Keep parity with current behavior (tool_result includes error).

3. Should HistoryDeltaEvent be optional (behind RunOption)?
   - Recommendation: Always emit for RunStream (non-live). It’s low overhead and enables the intended DX.

---

## 10) Breaking Change Summary

- **New event type** may require users to handle/ignore it.
- **RunResult.Messages** is additive (non-breaking).
- **No new RunOption** required.

---

## 11) File Touch List (expected)

- `sdk/run.go` (emit HistoryDeltaEvent; adjust internal history handling)
- `sdk/history.go` (new: DefaultHistoryHandler)
- `sdk/history_test.go` (new)
- `docs/SDK_SPEC.md` (examples)
- `README.md` (examples)
- `tests/integration/run_test.go` (if needed)

---

## 12) Validation Checklist

- [ ] RunStream still executes tools correctly
- [ ] RunStream emits HistoryDeltaEvent per step
- [ ] DefaultHistoryHandler reconstructs history identical to previous internal behavior
- [ ] Interrupt SavePartial/SaveMarked emits correct deltas
- [ ] Interrupt Discard emits no delta
- [ ] RunResult.Messages contains final history for Run()

---

End of plan.
