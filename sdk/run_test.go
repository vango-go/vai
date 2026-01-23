package vai

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/vango-go/vai/pkg/core/types"
)

func TestDefaultRunConfig(t *testing.T) {
	cfg := defaultRunConfig()

	if cfg.toolHandlers == nil {
		t.Error("toolHandlers should be initialized")
	}
	if !cfg.parallelTools {
		t.Error("parallelTools should be true by default")
	}
	if cfg.toolTimeout != 30*time.Second {
		t.Errorf("toolTimeout = %v, want 30s", cfg.toolTimeout)
	}
}

func TestRunOptions(t *testing.T) {
	cfg := defaultRunConfig()

	// Apply options
	opts := []RunOption{
		WithMaxToolCalls(5),
		WithMaxTurns(3),
		WithMaxTokensRun(1000),
		WithRunTimeout(10 * time.Second),
		WithParallelTools(false),
		WithToolTimeout(5 * time.Second),
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.maxToolCalls != 5 {
		t.Errorf("maxToolCalls = %d, want 5", cfg.maxToolCalls)
	}
	if cfg.maxTurns != 3 {
		t.Errorf("maxTurns = %d, want 3", cfg.maxTurns)
	}
	if cfg.maxTokens != 1000 {
		t.Errorf("maxTokens = %d, want 1000", cfg.maxTokens)
	}
	if cfg.timeout != 10*time.Second {
		t.Errorf("timeout = %v, want 10s", cfg.timeout)
	}
	if cfg.parallelTools {
		t.Error("parallelTools should be false")
	}
	if cfg.toolTimeout != 5*time.Second {
		t.Errorf("toolTimeout = %v, want 5s", cfg.toolTimeout)
	}
}

func TestWithToolHandler(t *testing.T) {
	cfg := defaultRunConfig()

	handler := func(ctx context.Context, input json.RawMessage) (any, error) {
		return "test result", nil
	}

	WithToolHandler("test_tool", handler)(&cfg)

	if _, ok := cfg.toolHandlers["test_tool"]; !ok {
		t.Error("tool handler should be registered")
	}
}

func TestWithToolHandlers(t *testing.T) {
	cfg := defaultRunConfig()

	handlers := map[string]ToolHandler{
		"tool1": func(ctx context.Context, input json.RawMessage) (any, error) { return "1", nil },
		"tool2": func(ctx context.Context, input json.RawMessage) (any, error) { return "2", nil },
	}

	WithToolHandlers(handlers)(&cfg)

	if len(cfg.toolHandlers) != 2 {
		t.Errorf("len(toolHandlers) = %d, want 2", len(cfg.toolHandlers))
	}
}

func TestWithToolSet(t *testing.T) {
	type Input struct {
		Query string `json:"query"`
	}

	ts := NewToolSet()
	tool, handler := FuncAsTool("search", "Search", func(ctx context.Context, input Input) (any, error) {
		return "result for " + input.Query, nil
	})
	ts.Add(tool, handler)

	cfg := defaultRunConfig()
	WithToolSet(ts)(&cfg)

	if _, ok := cfg.toolHandlers["search"]; !ok {
		t.Error("tool handler from ToolSet should be registered")
	}
}

func TestWithTools_MakeTool(t *testing.T) {
	type Input struct {
		Query string `json:"query"`
	}

	tool := MakeTool("search", "Search", func(ctx context.Context, input Input) (string, error) {
		return "result for " + input.Query, nil
	})

	cfg := defaultRunConfig()
	WithTools(tool)(&cfg)

	if _, ok := cfg.toolHandlers["search"]; !ok {
		t.Error("tool handler from MakeTool should be registered via WithTools")
	}

	// Test that handler works
	handler := cfg.toolHandlers["search"]
	result, err := handler(context.Background(), []byte(`{"query": "test"}`))
	if err != nil {
		t.Fatalf("Handler failed: %v", err)
	}
	if result != "result for test" {
		t.Errorf("Result = %v, want %q", result, "result for test")
	}
}

func TestWithStopWhen(t *testing.T) {
	cfg := defaultRunConfig()

	stopFn := func(r *Response) bool {
		return r.TextContent() == "DONE"
	}

	WithStopWhen(stopFn)(&cfg)

	if cfg.stopWhen == nil {
		t.Error("stopWhen should be set")
	}
}

func TestWithHooks(t *testing.T) {
	cfg := defaultRunConfig()

	var beforeCalled, afterCalled, onToolCalled, onStopCalled bool

	WithBeforeCall(func(req *MessageRequest) { beforeCalled = true })(&cfg)
	WithAfterResponse(func(resp *Response) { afterCalled = true })(&cfg)
	WithOnToolCall(func(name string, input map[string]any, output any, err error) { onToolCalled = true })(&cfg)
	WithOnStop(func(result *RunResult) { onStopCalled = true })(&cfg)

	// Test beforeCall hook
	cfg.beforeCall(&types.MessageRequest{})
	if !beforeCalled {
		t.Error("beforeCall hook should have been called")
	}

	// Test afterResponse hook
	cfg.afterResponse(&Response{})
	if !afterCalled {
		t.Error("afterResponse hook should have been called")
	}

	// Test onToolCall hook
	cfg.onToolCall("test", nil, nil, nil)
	if !onToolCalled {
		t.Error("onToolCall hook should have been called")
	}

	// Test onStop hook
	cfg.onStop(&RunResult{})
	if !onStopCalled {
		t.Error("onStop hook should have been called")
	}
}

func TestOutputToContentBlocks(t *testing.T) {
	tests := []struct {
		name     string
		output   any
		expected string // Expected text in first block
	}{
		{
			name:     "string output",
			output:   "hello world",
			expected: "hello world",
		},
		{
			name:     "struct output",
			output:   struct{ Name string }{Name: "test"},
			expected: `{"Name":"test"}`,
		},
		{
			name:     "map output",
			output:   map[string]int{"count": 42},
			expected: `{"count":42}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := outputToContentBlocks(tt.output)
			if len(blocks) == 0 {
				t.Fatal("expected at least one content block")
			}
			if tb, ok := blocks[0].(types.TextBlock); ok {
				if tb.Text != tt.expected {
					t.Errorf("Text = %q, want %q", tb.Text, tt.expected)
				}
			} else {
				t.Errorf("expected TextBlock, got %T", blocks[0])
			}
		})
	}
}

func TestOutputToContentBlocks_ContentBlock(t *testing.T) {
	// Test single ContentBlock
	tb := types.TextBlock{Type: "text", Text: "test"}
	blocks := outputToContentBlocks(tb)
	if len(blocks) != 1 {
		t.Errorf("len(blocks) = %d, want 1", len(blocks))
	}

	// Test slice of ContentBlocks
	slice := []types.ContentBlock{
		types.TextBlock{Type: "text", Text: "one"},
		types.TextBlock{Type: "text", Text: "two"},
	}
	blocks = outputToContentBlocks(slice)
	if len(blocks) != 2 {
		t.Errorf("len(blocks) = %d, want 2", len(blocks))
	}
}

func TestRunStopReason_Constants(t *testing.T) {
	// Verify constants are defined correctly
	reasons := []RunStopReason{
		RunStopEndTurn,
		RunStopMaxToolCalls,
		RunStopMaxTurns,
		RunStopMaxTokens,
		RunStopTimeout,
		RunStopCustom,
		RunStopError,
	}

	for _, r := range reasons {
		if r == "" {
			t.Error("RunStopReason should not be empty")
		}
	}
}

func TestRunResult_Structure(t *testing.T) {
	result := RunResult{
		Steps:         make([]RunStep, 0),
		ToolCallCount: 5,
		TurnCount:     2,
		Usage:         types.Usage{InputTokens: 100, OutputTokens: 50},
		StopReason:    RunStopEndTurn,
	}

	if result.ToolCallCount != 5 {
		t.Errorf("ToolCallCount = %d, want 5", result.ToolCallCount)
	}
	if result.TurnCount != 2 {
		t.Errorf("TurnCount = %d, want 2", result.TurnCount)
	}
	if result.Usage.InputTokens != 100 {
		t.Errorf("Usage.InputTokens = %d, want 100", result.Usage.InputTokens)
	}
}

func TestRunStep_Structure(t *testing.T) {
	step := RunStep{
		Index:      0,
		DurationMs: 150,
		ToolCalls: []ToolCall{
			{ID: "call_1", Name: "search", Input: map[string]any{"query": "test"}},
		},
		ToolResults: []ToolExecutionResult{
			{ToolUseID: "call_1", Content: []types.ContentBlock{types.TextBlock{Type: "text", Text: "result"}}},
		},
	}

	if step.Index != 0 {
		t.Errorf("Index = %d, want 0", step.Index)
	}
	if len(step.ToolCalls) != 1 {
		t.Errorf("len(ToolCalls) = %d, want 1", len(step.ToolCalls))
	}
	if step.ToolCalls[0].Name != "search" {
		t.Errorf("ToolCalls[0].Name = %q, want %q", step.ToolCalls[0].Name, "search")
	}
}

func TestRunStreamEvents(t *testing.T) {
	// Test event types implement the interface
	events := []RunStreamEvent{
		StepStartEvent{Index: 0},
		StreamEventWrapper{Event: types.PingEvent{}},
		ToolCallStartEvent{ID: "call_1", Name: "test", Input: nil},
		ToolResultEvent{ID: "call_1", Name: "test", Content: nil},
		StepCompleteEvent{Index: 0, Response: nil},
		HistoryDeltaEvent{Append: nil},
		RunCompleteEvent{Result: nil},
	}

	expectedTypes := []string{
		"step_start",
		"stream_event",
		"tool_call_start",
		"tool_result",
		"step_complete",
		"history_delta",
		"run_complete",
	}

	for i, e := range events {
		if e.runStreamEventType() != expectedTypes[i] {
			t.Errorf("Event %d type = %q, want %q", i, e.runStreamEventType(), expectedTypes[i])
		}
	}
}

func TestRunStream_Methods(t *testing.T) {
	rs := &RunStream{
		events: make(chan RunStreamEvent, 1),
		done:   make(chan struct{}),
	}

	// Test Events() returns the channel
	if rs.Events() == nil {
		t.Error("Events() should return non-nil channel")
	}

	// Set the result before closing
	rs.result = &RunResult{StopReason: RunStopEndTurn}

	// Mark as closed and close done channel
	rs.closed.Store(true)
	close(rs.done)

	// Test Result() after done
	result := rs.Result()
	if result == nil {
		t.Error("Result() should return non-nil after done")
	}

	// Test Err()
	rs.err = nil
	if rs.Err() != nil {
		t.Error("Err() should return nil when no error")
	}

	// Test Close() is idempotent (already closed)
	err := rs.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestRunStream_Close_Idempotent(t *testing.T) {
	rs := &RunStream{
		events: make(chan RunStreamEvent, 1),
		done:   make(chan struct{}),
	}

	// First close
	err := rs.Close()
	if err != nil {
		t.Errorf("First Close() returned error: %v", err)
	}

	// Second close should not panic and return nil
	err = rs.Close()
	if err != nil {
		t.Errorf("Second Close() returned error: %v", err)
	}
}

func TestToolExecutionResult_Structure(t *testing.T) {
	result := ToolExecutionResult{
		ToolUseID: "call_123",
		Content: []types.ContentBlock{
			types.TextBlock{Type: "text", Text: "result"},
		},
		Error:    nil,
		ErrorMsg: "",
	}

	if result.ToolUseID != "call_123" {
		t.Errorf("ToolUseID = %q, want %q", result.ToolUseID, "call_123")
	}
	if len(result.Content) != 1 {
		t.Errorf("len(Content) = %d, want 1", len(result.Content))
	}
}

// TestInterruptBehavior_Constants verifies the interrupt behavior enum values.
func TestInterruptBehavior_Constants(t *testing.T) {
	if InterruptDiscard != 0 {
		t.Errorf("InterruptDiscard = %d, want 0", InterruptDiscard)
	}
	if InterruptSavePartial != 1 {
		t.Errorf("InterruptSavePartial = %d, want 1", InterruptSavePartial)
	}
	if InterruptSaveMarked != 2 {
		t.Errorf("InterruptSaveMarked = %d, want 2", InterruptSaveMarked)
	}
}

// TestRunStream_InterruptRequest_EmptyMessage tests that empty message is detected correctly.
func TestRunStream_InterruptRequest_EmptyMessage(t *testing.T) {
	tests := []struct {
		name     string
		msg      types.Message
		isCancel bool
	}{
		{
			name:     "empty message (cancel)",
			msg:      types.Message{},
			isCancel: true,
		},
		{
			name:     "empty role but nil content (cancel)",
			msg:      types.Message{Role: "", Content: nil},
			isCancel: true,
		},
		{
			name: "has role (interrupt)",
			msg: types.Message{
				Role:    "user",
				Content: nil,
			},
			isCancel: false,
		},
		{
			name: "has content (interrupt)",
			msg: types.Message{
				Role: "",
				Content: []types.ContentBlock{
					types.TextBlock{Type: "text", Text: "test"},
				},
			},
			isCancel: false,
		},
		{
			name: "full message (interrupt)",
			msg: types.Message{
				Role: "user",
				Content: []types.ContentBlock{
					types.TextBlock{Type: "text", Text: "test"},
				},
			},
			isCancel: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isCancel := tt.msg.Role == "" && tt.msg.Content == nil
			if isCancel != tt.isCancel {
				t.Errorf("isCancel = %v, want %v", isCancel, tt.isCancel)
			}
		})
	}
}

// TestRunStream_Interrupt_ClosedStream verifies error handling when interrupting a closed stream.
func TestRunStream_Interrupt_ClosedStream(t *testing.T) {
	rs := &RunStream{
		events:    make(chan RunStreamEvent, 1),
		done:      make(chan struct{}),
		interrupt: make(chan interruptRequest, 1),
	}

	// Close the stream
	close(rs.done)
	rs.closed.Store(true)

	// Try to interrupt
	err := rs.Interrupt(types.Message{
		Role: "user",
		Content: []types.ContentBlock{
			types.TextBlock{Type: "text", Text: "test"},
		},
	}, InterruptSaveMarked)

	if err == nil {
		t.Error("Interrupt() should return error for closed stream")
	}
	if err.Error() != "stream already closed" {
		t.Errorf("Error = %q, want %q", err.Error(), "stream already closed")
	}
}

// TestRunStream_Cancel_ClosedStream verifies Cancel returns nil for already-closed stream.
func TestRunStream_Cancel_ClosedStream(t *testing.T) {
	rs := &RunStream{
		events:    make(chan RunStreamEvent, 1),
		done:      make(chan struct{}),
		interrupt: make(chan interruptRequest, 1),
	}

	// Close the stream
	close(rs.done)
	rs.closed.Store(true)

	// Cancel should return nil (not error) for already-closed stream
	err := rs.Cancel()
	if err != nil {
		t.Errorf("Cancel() returned error for closed stream: %v", err)
	}
}

// TestRunStream_InterruptWithText verifies the convenience method.
func TestRunStream_InterruptWithText_ClosedStream(t *testing.T) {
	rs := &RunStream{
		events:    make(chan RunStreamEvent, 1),
		done:      make(chan struct{}),
		interrupt: make(chan interruptRequest, 1),
	}

	// Close the stream
	close(rs.done)
	rs.closed.Store(true)

	// Try to interrupt with text
	err := rs.InterruptWithText("hello")

	if err == nil {
		t.Error("InterruptWithText() should return error for closed stream")
	}
	if err.Error() != "stream already closed" {
		t.Errorf("Error = %q, want %q", err.Error(), "stream already closed")
	}
}

// TestInterruptedEvent_Structure verifies the InterruptedEvent type.
func TestInterruptedEvent_Structure(t *testing.T) {
	event := InterruptedEvent{
		PartialText: "partial response",
		Behavior:    InterruptSaveMarked,
	}

	if event.PartialText != "partial response" {
		t.Errorf("PartialText = %q, want %q", event.PartialText, "partial response")
	}
	if event.Behavior != InterruptSaveMarked {
		t.Errorf("Behavior = %d, want %d", event.Behavior, InterruptSaveMarked)
	}
	if event.runStreamEventType() != "interrupted" {
		t.Errorf("runStreamEventType() = %q, want %q", event.runStreamEventType(), "interrupted")
	}
}

// TestWithInterruptConfig verifies the WithInterruptConfig RunOption.
func TestWithInterruptConfig(t *testing.T) {
	cfg := defaultRunConfig()

	semanticCheck := true
	interruptCfg := LiveInterrupt{
		Mode:            "semantic",
		EnergyThreshold: 0.5,
		DebounceMs:      200,
		SemanticCheck:   &semanticCheck,
		SemanticModel:   "claude-3-haiku",
		SavePartial:     "marked",
	}

	// WithInterruptConfig configures interrupt handling but doesn't enable live mode
	// Use with WithLive() or WithLiveConfig()
	WithInterruptConfig(interruptCfg)(&cfg)

	if cfg.interruptConfig == nil {
		t.Fatal("interruptConfig should be set")
	}
	if cfg.interruptConfig.Mode != "semantic" {
		t.Errorf("Mode = %q, want %q", cfg.interruptConfig.Mode, "semantic")
	}
	if cfg.interruptConfig.EnergyThreshold != 0.5 {
		t.Errorf("EnergyThreshold = %v, want 0.5", cfg.interruptConfig.EnergyThreshold)
	}
	if cfg.interruptConfig.DebounceMs != 200 {
		t.Errorf("DebounceMs = %d, want 200", cfg.interruptConfig.DebounceMs)
	}
	if cfg.interruptConfig.SemanticCheck == nil || !*cfg.interruptConfig.SemanticCheck {
		t.Error("SemanticCheck should be true")
	}
	if cfg.interruptConfig.SemanticModel != "claude-3-haiku" {
		t.Errorf("SemanticModel = %q, want %q", cfg.interruptConfig.SemanticModel, "claude-3-haiku")
	}
	if cfg.interruptConfig.SavePartial != "marked" {
		t.Errorf("SavePartial = %q, want %q", cfg.interruptConfig.SavePartial, "marked")
	}

	// Verify it works with WithLive()
	cfg2 := defaultRunConfig()
	WithInterruptConfig(interruptCfg)(&cfg2)

	if cfg2.interruptConfig == nil {
		t.Fatal("interruptConfig should be set")
	}
}

// TestLiveVoiceOutput_Struct verifies the LiveVoiceOutput struct.
func TestLiveVoiceOutput_Struct(t *testing.T) {
	vo := LiveVoiceOutput{
		Provider: "cartesia",
		Voice:    "echo",
		Speed:    1.0,
		Format:   "wav",
	}

	if vo.Provider != "cartesia" {
		t.Errorf("Provider = %q, want %q", vo.Provider, "cartesia")
	}
	if vo.Voice != "echo" {
		t.Errorf("Voice = %q, want %q", vo.Voice, "echo")
	}
	if vo.Speed != 1.0 {
		t.Errorf("Speed = %v, want 1.0", vo.Speed)
	}
	if vo.Format != "wav" {
		t.Errorf("Format = %q, want %q", vo.Format, "wav")
	}
}

// TestLiveInterrupt_Struct verifies the LiveInterrupt struct.
func TestLiveInterrupt_Struct(t *testing.T) {
	semanticCheck := false
	li := LiveInterrupt{
		Mode:            "energy",
		EnergyThreshold: 0.3,
		DebounceMs:      100,
		SemanticCheck:   &semanticCheck,
		SemanticModel:   "",
		SavePartial:     "discard",
	}

	if li.Mode != "energy" {
		t.Errorf("Mode = %q, want %q", li.Mode, "energy")
	}
	if li.EnergyThreshold != 0.3 {
		t.Errorf("EnergyThreshold = %v, want 0.3", li.EnergyThreshold)
	}
	if li.DebounceMs != 100 {
		t.Errorf("DebounceMs = %d, want 100", li.DebounceMs)
	}
	if li.SemanticCheck == nil || *li.SemanticCheck {
		t.Error("SemanticCheck should be false")
	}
	if li.SavePartial != "discard" {
		t.Errorf("SavePartial = %q, want %q", li.SavePartial, "discard")
	}
}

// TestInterruptConfigAlias verifies the InterruptConfig type alias.
func TestInterruptConfigAlias(t *testing.T) {
	// Verify InterruptConfig is an alias for LiveInterrupt
	var ic InterruptConfig = LiveInterrupt{
		Mode: "semantic",
	}

	if ic.Mode != "semantic" {
		t.Errorf("Mode = %q, want %q", ic.Mode, "semantic")
	}
}
