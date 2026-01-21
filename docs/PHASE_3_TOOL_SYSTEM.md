# Phase 3: Tool System & Run Loop

**Status:** Complete
**Priority:** Critical Path
**Dependencies:** Phase 2 (Anthropic Provider)

---

## Overview

Phase 3 implements the complete tool system including type-safe tool definitions (`FuncAsTool`), native tool constructors, tool handlers, and the agentic `Run()` loop that automatically executes tools until a stop condition is met.

This phase enables building autonomous agents that can:
- Define custom tools with automatic schema generation
- Use native tools (web search, code execution)
- Execute multi-turn conversations with tool calls
- Control loop termination via configurable stop conditions

---

## Goals

1. Implement `FuncAsTool` for type-safe tool definitions with automatic schema generation
2. Create constructors for native tools (WebSearch, CodeExecution, etc.)
3. Implement `Messages.Run()` for blocking tool execution loops
4. Implement `Messages.RunStream()` for streaming tool execution loops
5. Support configurable stop conditions (max calls, max turns, custom predicates)
6. Provide hooks for observability (before call, after response, on tool call)

---

## Deliverables

### 3.1 Package Structure

```
sdk/
├── tools.go          # Tool type, FuncAsTool, native tool constructors
├── schema.go         # Automatic JSON schema generation from structs
├── run.go            # Run(), RunStream(), RunResult
├── run_options.go    # RunOption functions
├── run_stream.go     # RunStream implementation
└── handlers.go       # ToolHandler type and execution
```

### 3.2 Tool Definitions (sdk/tools.go)

```go
package vango

import (
    "context"
    "encoding/json"

    "github.com/vango-go/vai/pkg/core/types"
)

// Tool represents a tool that can be used by the model.
// Reexport from core types for SDK convenience.
type Tool = types.Tool

// ToolHandler is a function that executes a tool.
type ToolHandler func(ctx context.Context, input json.RawMessage) (any, error)

// ToolWithHandler wraps a Tool with its handler function.
type ToolWithHandler struct {
    Tool
    handler ToolHandler
}

// FuncAsTool creates a type-safe tool from a Go function.
// The function must have signature: func(ctx context.Context, input T) (R, error)
// where T is a struct that will be used to generate the JSON schema.
//
// Example:
//
//     tool := vai.FuncAsTool("get_weather", "Get weather for a location",
//         func(ctx context.Context, input struct {
//             Location string `json:"location" desc:"City name or coordinates"`
//             Units    string `json:"units" desc:"Temperature units" enum:"celsius,fahrenheit"`
//         }) (*WeatherData, error) {
//             return weatherAPI.Get(input.Location, input.Units)
//         },
//     )
func FuncAsTool[T any, R any](name, description string, fn func(context.Context, T) (R, error)) ToolWithHandler {
    // Generate schema from T
    var zero T
    schema := generateSchema(zero)

    // Create handler that unmarshals input and calls function
    handler := func(ctx context.Context, rawInput json.RawMessage) (any, error) {
        var input T
        if err := json.Unmarshal(rawInput, &input); err != nil {
            return nil, fmt.Errorf("invalid input: %w", err)
        }
        return fn(ctx, input)
    }

    return ToolWithHandler{
        Tool: types.Tool{
            Type:        types.ToolTypeFunction,
            Name:        name,
            Description: description,
            InputSchema: schema,
        },
        handler: handler,
    }
}

// --- Native Tool Constructors ---

// WebSearch creates a web search tool.
func WebSearch(configs ...WebSearchConfig) Tool {
    var cfg WebSearchConfig
    if len(configs) > 0 {
        cfg = configs[0]
    }
    return types.Tool{
        Type:   types.ToolTypeWebSearch,
        Config: cfg,
    }
}

// WebSearchConfig configures the web search tool.
type WebSearchConfig = types.WebSearchConfig

// CodeExecution creates a code execution tool.
func CodeExecution(configs ...CodeExecutionConfig) Tool {
    var cfg CodeExecutionConfig
    if len(configs) > 0 {
        cfg = configs[0]
    }
    return types.Tool{
        Type:   types.ToolTypeCodeExecution,
        Config: cfg,
    }
}

// CodeExecutionConfig configures the code execution tool.
type CodeExecutionConfig = types.CodeExecutionConfig

// ComputerUse creates a computer use tool.
func ComputerUse(width, height int) Tool {
    return types.Tool{
        Type: types.ToolTypeComputerUse,
        Config: types.ComputerUseConfig{
            DisplayWidth:  width,
            DisplayHeight: height,
        },
    }
}

// TextEditor creates a text editor tool.
func TextEditor() Tool {
    return types.Tool{
        Type: types.ToolTypeTextEditor,
    }
}

// FileSearch creates a file search tool (OpenAI-specific).
func FileSearch() Tool {
    return types.Tool{
        Type: types.ToolTypeFileSearch,
    }
}
```

### 3.3 Schema Generation (sdk/schema.go)

```go
package vango

import (
    "reflect"
    "strings"

    "github.com/vango-go/vai/pkg/core/types"
)

// generateSchema generates a JSON schema from a Go struct.
// Supports struct tags:
//   - json:"name"        - field name in JSON
//   - desc:"description" - field description
//   - enum:"a,b,c"       - enum values
func generateSchema(v any) *types.JSONSchema {
    t := reflect.TypeOf(v)
    return generateSchemaFromType(t)
}

func generateSchemaFromType(t reflect.Type) *types.JSONSchema {
    // Dereference pointer
    if t.Kind() == reflect.Ptr {
        t = t.Elem()
    }

    switch t.Kind() {
    case reflect.Struct:
        return generateObjectSchema(t)
    case reflect.Slice, reflect.Array:
        return &types.JSONSchema{
            Type:  "array",
            Items: generateSchemaFromType(t.Elem()),
        }
    case reflect.String:
        return &types.JSONSchema{Type: "string"}
    case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
        return &types.JSONSchema{Type: "integer"}
    case reflect.Float32, reflect.Float64:
        return &types.JSONSchema{Type: "number"}
    case reflect.Bool:
        return &types.JSONSchema{Type: "boolean"}
    case reflect.Map:
        return &types.JSONSchema{Type: "object"}
    default:
        return &types.JSONSchema{Type: "string"} // Fallback
    }
}

func generateObjectSchema(t reflect.Type) *types.JSONSchema {
    schema := &types.JSONSchema{
        Type:       "object",
        Properties: make(map[string]types.JSONSchema),
        Required:   []string{},
    }

    for i := 0; i < t.NumField(); i++ {
        field := t.Field(i)

        // Skip unexported fields
        if !field.IsExported() {
            continue
        }

        // Get JSON name
        jsonName := field.Name
        if tag := field.Tag.Get("json"); tag != "" {
            parts := strings.Split(tag, ",")
            if parts[0] != "" && parts[0] != "-" {
                jsonName = parts[0]
            }
            if parts[0] == "-" {
                continue // Skip this field
            }
        }

        // Generate field schema
        fieldSchema := generateSchemaFromType(field.Type)

        // Apply description
        if desc := field.Tag.Get("desc"); desc != "" {
            fieldSchema.Description = desc
        }

        // Apply enum
        if enum := field.Tag.Get("enum"); enum != "" {
            fieldSchema.Enum = strings.Split(enum, ",")
        }

        schema.Properties[jsonName] = *fieldSchema

        // Check if required (not a pointer and no omitempty)
        isRequired := true
        if field.Type.Kind() == reflect.Ptr {
            isRequired = false
        }
        if tag := field.Tag.Get("json"); strings.Contains(tag, "omitempty") {
            isRequired = false
        }
        if isRequired {
            schema.Required = append(schema.Required, jsonName)
        }
    }

    return schema
}
```

### 3.4 Run Result Types (sdk/run.go)

```go
package vango

import (
    "context"
    "time"

    "github.com/vango-go/vai/pkg/core/types"
)

// RunResult contains the result of a Run() execution.
type RunResult struct {
    Response      *types.MessageResponse // Final response
    Steps         []RunStep              // All steps taken
    ToolCallCount int                    // Total tool calls executed
    TurnCount     int                    // Total LLM turns
    Usage         types.Usage            // Aggregated usage across all turns
    StopReason    RunStopReason          // Why the loop stopped
}

// RunStopReason indicates why the run loop terminated.
type RunStopReason string

const (
    RunStopEndTurn       RunStopReason = "end_turn"        // Model finished naturally
    RunStopMaxToolCalls  RunStopReason = "max_tool_calls"  // Hit tool call limit
    RunStopMaxTurns      RunStopReason = "max_turns"       // Hit turn limit
    RunStopMaxTokens     RunStopReason = "max_tokens"      // Hit token limit
    RunStopTimeout       RunStopReason = "timeout"         // Hit timeout
    RunStopCustom        RunStopReason = "custom"          // Custom stop condition
    RunStopError         RunStopReason = "error"           // Error occurred
)

// RunStep represents a single turn in the run loop.
type RunStep struct {
    Index       int                    // Step number (0-indexed)
    Response    *types.MessageResponse // Response from this step
    ToolCalls   []ToolCall             // Tool calls made in this step
    ToolResults []ToolCallResult       // Results of tool calls
    DurationMs  int64                  // Time taken for this step
}

// ToolCall represents a single tool call.
type ToolCall struct {
    ID    string         // Tool use ID
    Name  string         // Tool name
    Input map[string]any // Tool input
}

// ToolCallResult represents the result of a tool call.
type ToolCallResult struct {
    ToolUseID string               // Matching tool use ID
    Content   []types.ContentBlock // Result content
    Error     error                // Error if tool failed
}

// Run executes a message request with automatic tool execution.
func (s *MessagesService) Run(ctx context.Context, req *types.MessageRequest, opts ...RunOption) (*RunResult, error) {
    cfg := defaultRunConfig()
    for _, opt := range opts {
        opt(&cfg)
    }

    return s.runLoop(ctx, req, &cfg)
}

func (s *MessagesService) runLoop(ctx context.Context, req *types.MessageRequest, cfg *runConfig) (*RunResult, error) {
    result := &RunResult{
        Steps: make([]RunStep, 0),
    }

    // Create a working copy of messages
    messages := make([]types.Message, len(req.Messages))
    copy(messages, req.Messages)

    // Apply timeout if configured
    if cfg.timeout > 0 {
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(ctx, cfg.timeout)
        defer cancel()
    }

    // Main loop
    for {
        // Check timeout
        select {
        case <-ctx.Done():
            result.StopReason = RunStopTimeout
            return result, ctx.Err()
        default:
        }

        // Check turn limit
        if cfg.maxTurns > 0 && result.TurnCount >= cfg.maxTurns {
            result.StopReason = RunStopMaxTurns
            return result, nil
        }

        // Check token limit
        if cfg.maxTokens > 0 && result.Usage.TotalTokens >= cfg.maxTokens {
            result.StopReason = RunStopMaxTokens
            return result, nil
        }

        // Build request for this turn
        turnReq := &types.MessageRequest{
            Model:         req.Model,
            Messages:      messages,
            MaxTokens:     req.MaxTokens,
            System:        req.System,
            Temperature:   req.Temperature,
            TopP:          req.TopP,
            TopK:          req.TopK,
            StopSequences: req.StopSequences,
            Tools:         req.Tools,
            ToolChoice:    req.ToolChoice,
            OutputFormat:  req.OutputFormat,
            Extensions:    req.Extensions,
            Metadata:      req.Metadata,
        }

        // Call before hook
        if cfg.beforeCall != nil {
            cfg.beforeCall(turnReq)
        }

        // Make the API call
        stepStart := time.Now()
        resp, err := s.Create(ctx, turnReq)
        stepDuration := time.Since(stepStart).Milliseconds()

        if err != nil {
            result.StopReason = RunStopError
            return result, err
        }

        // Call after hook
        if cfg.afterResponse != nil {
            cfg.afterResponse(resp)
        }

        // Aggregate usage
        result.Usage = result.Usage.Add(resp.Usage)
        result.TurnCount++

        // Create step record
        step := RunStep{
            Index:      len(result.Steps),
            Response:   resp,
            DurationMs: stepDuration,
        }

        // Check custom stop condition
        if cfg.stopWhen != nil && cfg.stopWhen(resp) {
            result.Response = resp
            result.Steps = append(result.Steps, step)
            result.StopReason = RunStopCustom
            return result, nil
        }

        // Check if model finished without tool calls
        if resp.StopReason != types.StopReasonToolUse {
            result.Response = resp
            result.Steps = append(result.Steps, step)
            result.StopReason = RunStopEndTurn
            return result, nil
        }

        // Process tool calls
        toolUses := resp.ToolUses()
        if len(toolUses) == 0 {
            result.Response = resp
            result.Steps = append(result.Steps, step)
            result.StopReason = RunStopEndTurn
            return result, nil
        }

        // Check tool call limit
        if cfg.maxToolCalls > 0 && result.ToolCallCount+len(toolUses) > cfg.maxToolCalls {
            result.Response = resp
            result.Steps = append(result.Steps, step)
            result.StopReason = RunStopMaxToolCalls
            return result, nil
        }

        // Execute tool calls
        toolResults, err := s.executeToolCalls(ctx, toolUses, cfg)
        if err != nil {
            result.StopReason = RunStopError
            return result, err
        }

        step.ToolCalls = make([]ToolCall, len(toolUses))
        for i, tu := range toolUses {
            step.ToolCalls[i] = ToolCall{
                ID:    tu.ID,
                Name:  tu.Name,
                Input: tu.Input,
            }
        }
        step.ToolResults = toolResults
        result.Steps = append(result.Steps, step)
        result.ToolCallCount += len(toolUses)

        // Append assistant message with tool calls
        messages = append(messages, types.Message{
            Role:    "assistant",
            Content: resp.Content,
        })

        // Append tool results as user message
        toolResultBlocks := make([]types.ContentBlock, len(toolResults))
        for i, tr := range toolResults {
            toolResultBlocks[i] = types.ToolResultBlock{
                Type:      "tool_result",
                ToolUseID: tr.ToolUseID,
                Content:   tr.Content,
                IsError:   tr.Error != nil,
            }
        }
        messages = append(messages, types.Message{
            Role:    "user",
            Content: toolResultBlocks,
        })
    }
}

func (s *MessagesService) executeToolCalls(ctx context.Context, toolUses []types.ToolUseBlock, cfg *runConfig) ([]ToolCallResult, error) {
    results := make([]ToolCallResult, len(toolUses))

    // Create a context with tool timeout if configured
    if cfg.toolTimeout > 0 {
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(ctx, cfg.toolTimeout)
        defer cancel()
    }

    if cfg.parallelTools {
        // Execute in parallel
        var wg sync.WaitGroup
        var mu sync.Mutex
        var firstErr error

        for i, tu := range toolUses {
            wg.Add(1)
            go func(idx int, toolUse types.ToolUseBlock) {
                defer wg.Done()
                result := s.executeToolCall(ctx, toolUse, cfg)
                mu.Lock()
                results[idx] = result
                if result.Error != nil && firstErr == nil {
                    firstErr = result.Error
                }
                mu.Unlock()
            }(i, tu)
        }
        wg.Wait()

        // Note: We don't return firstErr here - tool errors are returned as error results
    } else {
        // Execute sequentially
        for i, tu := range toolUses {
            results[i] = s.executeToolCall(ctx, tu, cfg)
        }
    }

    return results, nil
}

func (s *MessagesService) executeToolCall(ctx context.Context, toolUse types.ToolUseBlock, cfg *runConfig) ToolCallResult {
    result := ToolCallResult{
        ToolUseID: toolUse.ID,
    }

    // Find handler
    handler, ok := cfg.handlers[toolUse.Name]
    if !ok {
        // No handler - return a generic message
        result.Content = []types.ContentBlock{
            types.TextBlock{
                Type: "text",
                Text: fmt.Sprintf("Tool '%s' was called but no handler is registered.", toolUse.Name),
            },
        }
        return result
    }

    // Marshal input
    inputJSON, _ := json.Marshal(toolUse.Input)

    // Execute handler
    output, err := handler(ctx, inputJSON)

    // Call hook
    if cfg.onToolCall != nil {
        cfg.onToolCall(toolUse.Name, toolUse.Input, output, err)
    }

    if err != nil {
        result.Error = err
        result.Content = []types.ContentBlock{
            types.TextBlock{
                Type: "text",
                Text: fmt.Sprintf("Error executing tool: %v", err),
            },
        }
        return result
    }

    // Convert output to content
    switch v := output.(type) {
    case string:
        result.Content = []types.ContentBlock{
            types.TextBlock{Type: "text", Text: v},
        }
    case []types.ContentBlock:
        result.Content = v
    case types.ContentBlock:
        result.Content = []types.ContentBlock{v}
    default:
        // JSON encode other types
        jsonBytes, _ := json.Marshal(v)
        result.Content = []types.ContentBlock{
            types.TextBlock{Type: "text", Text: string(jsonBytes)},
        }
    }

    return result
}
```

### 3.5 Run Options (sdk/run_options.go)

```go
package vango

import (
    "time"

    "github.com/vango-go/vai/pkg/core/types"
)

// runConfig holds configuration for Run() execution.
type runConfig struct {
    // Stop conditions
    maxToolCalls int
    maxTurns     int
    maxTokens    int
    timeout      time.Duration
    stopWhen     func(*types.MessageResponse) bool

    // Tool handlers
    handlers map[string]ToolHandler

    // Hooks
    beforeCall    func(*types.MessageRequest)
    afterResponse func(*types.MessageResponse)
    onToolCall    func(name string, input map[string]any, output any, err error)
    onStop        func(*RunResult)

    // Behavior
    parallelTools bool
    toolTimeout   time.Duration
}

func defaultRunConfig() runConfig {
    return runConfig{
        handlers:      make(map[string]ToolHandler),
        parallelTools: true,
        toolTimeout:   30 * time.Second,
    }
}

// RunOption configures Run() behavior.
type RunOption func(*runConfig)

// --- Stop Conditions ---

// WithMaxToolCalls sets the maximum number of tool calls before stopping.
func WithMaxToolCalls(n int) RunOption {
    return func(c *runConfig) { c.maxToolCalls = n }
}

// WithMaxTurns sets the maximum number of LLM turns before stopping.
func WithMaxTurns(n int) RunOption {
    return func(c *runConfig) { c.maxTurns = n }
}

// WithMaxTokens sets the maximum total tokens before stopping.
func WithMaxTokens(n int) RunOption {
    return func(c *runConfig) { c.maxTokens = n }
}

// WithTimeout sets a timeout for the entire run.
func WithTimeout(d time.Duration) RunOption {
    return func(c *runConfig) { c.timeout = d }
}

// WithStopWhen sets a custom stop condition.
// The function is called after each response.
// If it returns true, the run stops.
func WithStopWhen(fn func(*types.MessageResponse) bool) RunOption {
    return func(c *runConfig) { c.stopWhen = fn }
}

// --- Tool Handlers ---

// WithToolHandler registers a handler for a specific tool.
func WithToolHandler(name string, handler ToolHandler) RunOption {
    return func(c *runConfig) { c.handlers[name] = handler }
}

// WithToolHandlers registers multiple tool handlers at once.
func WithToolHandlers(handlers map[string]ToolHandler) RunOption {
    return func(c *runConfig) {
        for name, handler := range handlers {
            c.handlers[name] = handler
        }
    }
}

// WithTools registers tools that have embedded handlers (from FuncAsTool).
func WithTools(tools ...ToolWithHandler) RunOption {
    return func(c *runConfig) {
        for _, t := range tools {
            if t.handler != nil {
                c.handlers[t.Name] = t.handler
            }
        }
    }
}

// --- Hooks ---

// WithBeforeCall sets a hook called before each LLM call.
func WithBeforeCall(fn func(*types.MessageRequest)) RunOption {
    return func(c *runConfig) { c.beforeCall = fn }
}

// WithAfterResponse sets a hook called after each LLM response.
func WithAfterResponse(fn func(*types.MessageResponse)) RunOption {
    return func(c *runConfig) { c.afterResponse = fn }
}

// WithOnToolCall sets a hook called after each tool execution.
func WithOnToolCall(fn func(name string, input map[string]any, output any, err error)) RunOption {
    return func(c *runConfig) { c.onToolCall = fn }
}

// WithOnStop sets a hook called when the run stops.
func WithOnStop(fn func(*RunResult)) RunOption {
    return func(c *runConfig) { c.onStop = fn }
}

// --- Behavior ---

// WithParallelTools enables or disables parallel tool execution.
// Default is true.
func WithParallelTools(enabled bool) RunOption {
    return func(c *runConfig) { c.parallelTools = enabled }
}

// WithToolTimeout sets the timeout for individual tool calls.
// Default is 30 seconds.
func WithToolTimeout(d time.Duration) RunOption {
    return func(c *runConfig) { c.toolTimeout = d }
}
```

### 3.6 RunStream Implementation (sdk/run_stream.go)

```go
package vango

import (
    "context"
    "sync/atomic"

    "github.com/vango-go/vai/pkg/core/types"
)

// RunStream is a streaming version of Run().
type RunStream struct {
    client  *MessagesService
    req     *types.MessageRequest
    cfg     *runConfig
    events  chan RunStreamEvent
    result  *RunResult
    err     error
    closed  atomic.Bool
    done    chan struct{}
}

// RunStreamEvent represents events during a streaming run.
type RunStreamEvent interface {
    runStreamEventType() string
}

// StepStartEvent signals the start of a new step.
type StepStartEvent struct {
    Index int
}

func (e StepStartEvent) runStreamEventType() string { return "step_start" }

// StreamEvent wraps regular stream events.
type StreamEventWrapper struct {
    Event types.StreamEvent
}

func (e StreamEventWrapper) runStreamEventType() string { return "stream_event" }

// ToolCallStartEvent signals a tool is being called.
type ToolCallStartEvent struct {
    ID    string
    Name  string
    Input map[string]any
}

func (e ToolCallStartEvent) runStreamEventType() string { return "tool_call_start" }

// ToolResultEvent signals a tool result is ready.
type ToolResultEvent struct {
    ID      string
    Content []types.ContentBlock
    Error   error
}

func (e ToolResultEvent) runStreamEventType() string { return "tool_result" }

// StepCompleteEvent signals a step is complete.
type StepCompleteEvent struct {
    Index    int
    Response *types.MessageResponse
}

func (e StepCompleteEvent) runStreamEventType() string { return "step_complete" }

// RunCompleteEvent signals the run is complete.
type RunCompleteEvent struct {
    Result *RunResult
}

func (e RunCompleteEvent) runStreamEventType() string { return "run_complete" }

// RunStream creates a streaming run.
func (s *MessagesService) RunStream(ctx context.Context, req *types.MessageRequest, opts ...RunOption) (*RunStream, error) {
    cfg := defaultRunConfig()
    for _, opt := range opts {
        opt(&cfg)
    }

    rs := &RunStream{
        client: s,
        req:    req,
        cfg:    &cfg,
        events: make(chan RunStreamEvent, 100),
        done:   make(chan struct{}),
    }

    go rs.run(ctx)
    return rs, nil
}

func (rs *RunStream) run(ctx context.Context) {
    defer close(rs.events)
    defer close(rs.done)

    result := &RunResult{
        Steps: make([]RunStep, 0),
    }

    // Create working copy of messages
    messages := make([]types.Message, len(rs.req.Messages))
    copy(messages, rs.req.Messages)

    // Apply timeout
    if rs.cfg.timeout > 0 {
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(ctx, rs.cfg.timeout)
        defer cancel()
    }

    stepIndex := 0

    for {
        select {
        case <-ctx.Done():
            result.StopReason = RunStopTimeout
            rs.result = result
            rs.err = ctx.Err()
            return
        default:
        }

        // Check limits
        if rs.cfg.maxTurns > 0 && result.TurnCount >= rs.cfg.maxTurns {
            result.StopReason = RunStopMaxTurns
            rs.result = result
            return
        }

        // Signal step start
        rs.send(StepStartEvent{Index: stepIndex})

        // Build request
        turnReq := &types.MessageRequest{
            Model:      rs.req.Model,
            Messages:   messages,
            MaxTokens:  rs.req.MaxTokens,
            System:     rs.req.System,
            Tools:      rs.req.Tools,
            ToolChoice: rs.req.ToolChoice,
            Stream:     true, // Always stream in RunStream
        }

        if rs.cfg.beforeCall != nil {
            rs.cfg.beforeCall(turnReq)
        }

        // Stream this turn
        stream, err := rs.client.Stream(ctx, turnReq)
        if err != nil {
            result.StopReason = RunStopError
            rs.result = result
            rs.err = err
            return
        }

        // Forward stream events
        for event := range stream.Events() {
            rs.send(StreamEventWrapper{Event: event})
        }

        if stream.Err() != nil {
            result.StopReason = RunStopError
            rs.result = result
            rs.err = stream.Err()
            stream.Close()
            return
        }

        resp := stream.Response()
        stream.Close()

        if rs.cfg.afterResponse != nil {
            rs.cfg.afterResponse(resp)
        }

        result.Usage = result.Usage.Add(resp.Usage)
        result.TurnCount++

        step := RunStep{
            Index:    stepIndex,
            Response: resp,
        }

        // Check custom stop
        if rs.cfg.stopWhen != nil && rs.cfg.stopWhen(resp) {
            result.Response = resp
            result.Steps = append(result.Steps, step)
            result.StopReason = RunStopCustom
            rs.send(StepCompleteEvent{Index: stepIndex, Response: resp})
            rs.result = result
            return
        }

        // Check if done
        if resp.StopReason != types.StopReasonToolUse {
            result.Response = resp
            result.Steps = append(result.Steps, step)
            result.StopReason = RunStopEndTurn
            rs.send(StepCompleteEvent{Index: stepIndex, Response: resp})
            rs.result = result
            return
        }

        // Process tool calls
        toolUses := resp.ToolUses()
        if len(toolUses) == 0 {
            result.Response = resp
            result.Steps = append(result.Steps, step)
            result.StopReason = RunStopEndTurn
            rs.send(StepCompleteEvent{Index: stepIndex, Response: resp})
            rs.result = result
            return
        }

        // Check tool call limit
        if rs.cfg.maxToolCalls > 0 && result.ToolCallCount+len(toolUses) > rs.cfg.maxToolCalls {
            result.Response = resp
            result.Steps = append(result.Steps, step)
            result.StopReason = RunStopMaxToolCalls
            rs.send(StepCompleteEvent{Index: stepIndex, Response: resp})
            rs.result = result
            return
        }

        // Execute tools with events
        toolResults := make([]ToolCallResult, len(toolUses))
        for i, tu := range toolUses {
            rs.send(ToolCallStartEvent{ID: tu.ID, Name: tu.Name, Input: tu.Input})

            tr := rs.client.executeToolCall(ctx, tu, rs.cfg)
            toolResults[i] = tr

            rs.send(ToolResultEvent{ID: tu.ID, Content: tr.Content, Error: tr.Error})
        }

        step.ToolCalls = make([]ToolCall, len(toolUses))
        for i, tu := range toolUses {
            step.ToolCalls[i] = ToolCall{ID: tu.ID, Name: tu.Name, Input: tu.Input}
        }
        step.ToolResults = toolResults
        result.Steps = append(result.Steps, step)
        result.ToolCallCount += len(toolUses)

        rs.send(StepCompleteEvent{Index: stepIndex, Response: resp})

        // Append messages for next turn
        messages = append(messages, types.Message{
            Role:    "assistant",
            Content: resp.Content,
        })

        toolResultBlocks := make([]types.ContentBlock, len(toolResults))
        for i, tr := range toolResults {
            toolResultBlocks[i] = types.ToolResultBlock{
                Type:      "tool_result",
                ToolUseID: tr.ToolUseID,
                Content:   tr.Content,
                IsError:   tr.Error != nil,
            }
        }
        messages = append(messages, types.Message{
            Role:    "user",
            Content: toolResultBlocks,
        })

        stepIndex++
    }
}

func (rs *RunStream) send(event RunStreamEvent) {
    select {
    case rs.events <- event:
    case <-rs.done:
    }
}

// Events returns the channel of run stream events.
func (rs *RunStream) Events() <-chan RunStreamEvent {
    return rs.events
}

// Result returns the final result after the stream ends.
func (rs *RunStream) Result() *RunResult {
    <-rs.done
    return rs.result
}

// Err returns any error that occurred.
func (rs *RunStream) Err() error {
    <-rs.done
    return rs.err
}

// Close stops the run stream.
func (rs *RunStream) Close() error {
    if rs.closed.Swap(true) {
        return nil
    }
    close(rs.done)
    return nil
}
```

---

## Testing Strategy

### Unit Tests
- Schema generation from various struct types
- Tool conversion (Vango AI -> Anthropic format)
- Stop condition logic
- Parallel vs sequential tool execution

### Integration Tests (Real API)

```go
// +build integration

func TestRun_WebSearch(t *testing.T) {
    client := vai.NewClient()

    result, err := client.Messages.Run(context.Background(),
        &vai.MessageRequest{
            Model: "anthropic/claude-sonnet-4",
            Messages: []vai.Message{
                {Role: "user", Content: vai.Text("What is the current stock price of Apple? Use web search.")},
            },
            Tools: []vai.Tool{vai.WebSearch()},
        },
        vai.WithMaxToolCalls(3),
    )

    require.NoError(t, err)
    assert.True(t, result.ToolCallCount > 0)
    assert.Contains(t, result.Response.TextContent(), "Apple")
}

func TestRun_CustomTool(t *testing.T) {
    client := vai.NewClient()

    weatherTool := vai.FuncAsTool("get_weather", "Get weather for a location",
        func(ctx context.Context, input struct {
            Location string `json:"location"`
        }) (string, error) {
            return fmt.Sprintf("Weather in %s: 72°F and sunny", input.Location), nil
        },
    )

    result, err := client.Messages.Run(context.Background(),
        &vai.MessageRequest{
            Model: "anthropic/claude-sonnet-4",
            Messages: []vai.Message{
                {Role: "user", Content: vai.Text("What's the weather in San Francisco?")},
            },
            Tools: []vai.Tool{weatherTool.Tool},
        },
        vai.WithTools(weatherTool),
        vai.WithMaxToolCalls(1),
    )

    require.NoError(t, err)
    assert.Equal(t, 1, result.ToolCallCount)
    assert.Contains(t, result.Response.TextContent(), "72°F")
}

func TestRun_MaxToolCalls(t *testing.T) {
    client := vai.NewClient()

    result, err := client.Messages.Run(context.Background(),
        &vai.MessageRequest{
            Model: "anthropic/claude-sonnet-4",
            Messages: []vai.Message{
                {Role: "user", Content: vai.Text("Search for 10 different things.")},
            },
            Tools: []vai.Tool{vai.WebSearch()},
        },
        vai.WithMaxToolCalls(2),
    )

    require.NoError(t, err)
    assert.LessOrEqual(t, result.ToolCallCount, 2)
    assert.Equal(t, vai.RunStopMaxToolCalls, result.StopReason)
}

func TestRunStream_WithHooks(t *testing.T) {
    client := vai.NewClient()

    var toolCalls []string
    var mu sync.Mutex

    stream, err := client.Messages.RunStream(context.Background(),
        &vai.MessageRequest{
            Model: "anthropic/claude-sonnet-4",
            Messages: []vai.Message{
                {Role: "user", Content: vai.Text("What is 2+2? Search the web to confirm.")},
            },
            Tools: []vai.Tool{vai.WebSearch()},
        },
        vai.WithOnToolCall(func(name string, input, output any, err error) {
            mu.Lock()
            toolCalls = append(toolCalls, name)
            mu.Unlock()
        }),
        vai.WithMaxToolCalls(1),
    )
    require.NoError(t, err)

    var gotToolCallStart bool
    for event := range stream.Events() {
        if _, ok := event.(vai.ToolCallStartEvent); ok {
            gotToolCallStart = true
        }
    }

    assert.True(t, gotToolCallStart)
    assert.Len(t, toolCalls, 1)
}
```

---

## Acceptance Criteria

1. [x] `FuncAsTool` generates correct JSON schemas from structs
2. [x] Schema generation handles all Go types correctly
3. [x] Native tool constructors work (WebSearch, CodeExecution)
4. [x] `Run()` executes tools and continues conversation
5. [x] `RunStream()` provides streaming events during tool execution
6. [x] Stop conditions work: maxToolCalls, maxTurns, maxTokens, timeout
7. [x] Custom stop conditions work via `WithStopWhen`
8. [x] Hooks fire at correct times
9. [x] Parallel tool execution works
10. [x] Tool errors are handled gracefully
11. [ ] Integration tests pass with real API (requires API key)

---

## Files to Create

```
sdk/tools.go
sdk/schema.go
sdk/run.go
sdk/run_options.go
sdk/run_stream.go
sdk/handlers.go
```

---

## Estimated Effort

- Tool definitions: ~200 lines
- Schema generation: ~150 lines
- Run loop: ~400 lines
- RunStream: ~300 lines
- Options: ~150 lines
- Tests: ~400 lines
- **Total: ~1600 lines**

---

## Next Phase

Phase 4: Voice Pipeline - implements STT/TTS integration for audio input/output with Anthropic.
