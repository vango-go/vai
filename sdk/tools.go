package vai

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/vango-go/vai/pkg/core/types"
)

// Tool represents a tool definition for the SDK.
// This is a re-export of types.Tool with additional helpers.
type Tool = types.Tool

// FuncTool creates a function tool with type-safe input handling.
// The handler receives the parsed input and returns a result.
type FuncTool[T any] struct {
	Name        string
	Description string
	Handler     func(ctx context.Context, input T) (any, error)
	schema      *types.JSONSchema
}

// NewFuncTool creates a new function tool from a typed handler.
func NewFuncTool[T any](name, description string, handler func(ctx context.Context, input T) (any, error)) *FuncTool[T] {
	var zero T
	schema := GenerateJSONSchema(reflect.TypeOf(zero))

	return &FuncTool[T]{
		Name:        name,
		Description: description,
		Handler:     handler,
		schema:      schema,
	}
}

// Tool returns the Tool definition for use in requests.
func (f *FuncTool[T]) Tool() types.Tool {
	return types.Tool{
		Type:        types.ToolTypeFunction,
		Name:        f.Name,
		Description: f.Description,
		InputSchema: f.schema,
	}
}

// Execute parses the input and calls the handler.
func (f *FuncTool[T]) Execute(ctx context.Context, input json.RawMessage) (any, error) {
	var parsed T
	if err := json.Unmarshal(input, &parsed); err != nil {
		return nil, err
	}
	return f.Handler(ctx, parsed)
}

// AsToolHandler returns this as a ToolHandler function.
func (f *FuncTool[T]) AsToolHandler() ToolHandler {
	return func(ctx context.Context, input json.RawMessage) (any, error) {
		return f.Execute(ctx, input)
	}
}

// FuncAsTool is a convenience function to create a tool from a function.
// This is useful for simple cases where you don't need the full FuncTool struct.
// Returns both the tool definition and handler as a tuple for flexibility.
func FuncAsTool[T any](name, description string, handler func(ctx context.Context, input T) (any, error)) (types.Tool, ToolHandler) {
	ft := NewFuncTool(name, description, handler)
	return ft.Tool(), ft.AsToolHandler()
}

// ToolWithHandler wraps a Tool with its handler function.
// This allows passing tools to WithTools() for automatic handler registration.
type ToolWithHandler struct {
	types.Tool
	Handler ToolHandler
}

// MakeTool creates a ToolWithHandler from a typed function.
// This is the preferred way to create tools with handlers.
//
// Example:
//
//	tool := vai.MakeTool("get_weather", "Get weather for a location",
//	    func(ctx context.Context, input struct {
//	        Location string `json:"location" desc:"City name or coordinates"`
//	        Units    string `json:"units" desc:"Temperature units" enum:"celsius,fahrenheit"`
//	    }) (string, error) {
//	        return weatherAPI.Get(input.Location, input.Units)
//	    },
//	)
//
//	// Use in request:
//	client.Messages.Run(ctx, &vai.MessageRequest{
//	    Tools: []vai.Tool{tool.Tool},
//	}, vai.WithTools(tool))
func MakeTool[T any, R any](name, description string, fn func(context.Context, T) (R, error)) ToolWithHandler {
	var zero T
	schema := GenerateJSONSchema(reflect.TypeOf(zero))

	handler := func(ctx context.Context, rawInput json.RawMessage) (any, error) {
		var input T
		if err := json.Unmarshal(rawInput, &input); err != nil {
			return nil, err
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
		Handler: handler,
	}
}

// ToolSet is a collection of tools with their handlers.
type ToolSet struct {
	tools    []types.Tool
	handlers map[string]ToolHandler
}

// NewToolSet creates a new empty tool set.
func NewToolSet() *ToolSet {
	return &ToolSet{
		tools:    []types.Tool{},
		handlers: make(map[string]ToolHandler),
	}
}

// Add adds a tool with its handler to the set.
func (ts *ToolSet) Add(tool types.Tool, handler ToolHandler) *ToolSet {
	ts.tools = append(ts.tools, tool)
	if handler != nil && tool.Name != "" {
		ts.handlers[tool.Name] = handler
	}
	return ts
}

// AddFunc adds a typed function tool to the set.
func AddFunc[T any](ts *ToolSet, name, description string, handler func(ctx context.Context, input T) (any, error)) *ToolSet {
	tool, h := FuncAsTool(name, description, handler)
	return ts.Add(tool, h)
}

// AddNative adds a native tool (like web_search) without a handler.
func (ts *ToolSet) AddNative(tool types.Tool) *ToolSet {
	ts.tools = append(ts.tools, tool)
	return ts
}

// Tools returns all tool definitions.
func (ts *ToolSet) Tools() []types.Tool {
	return ts.tools
}

// Handlers returns all tool handlers.
func (ts *ToolSet) Handlers() map[string]ToolHandler {
	return ts.handlers
}

// Handler returns the handler for a specific tool.
func (ts *ToolSet) Handler(name string) (ToolHandler, bool) {
	h, ok := ts.handlers[name]
	return h, ok
}

// --- Native Tool Constructors ---

// WebSearchConfig configures the web search tool.
type WebSearchConfig = types.WebSearchConfig

// WebSearch creates a web search tool.
// This tool allows the model to search the internet for information.
func WebSearch(configs ...WebSearchConfig) types.Tool {
	var cfg *WebSearchConfig
	if len(configs) > 0 {
		cfg = &configs[0]
	}
	return types.Tool{
		Type:   types.ToolTypeWebSearch,
		Config: cfg,
	}
}

// CodeExecutionConfig configures the code execution tool.
type CodeExecutionConfig = types.CodeExecutionConfig

// CodeExecution creates a code execution tool.
// This tool allows the model to execute code in a sandboxed environment.
func CodeExecution(configs ...CodeExecutionConfig) types.Tool {
	var cfg *CodeExecutionConfig
	if len(configs) > 0 {
		cfg = &configs[0]
	}
	return types.Tool{
		Type:   types.ToolTypeCodeExecution,
		Config: cfg,
	}
}

// ComputerUseConfig configures the computer use tool.
type ComputerUseConfig = types.ComputerUseConfig

// ComputerUse creates a computer use tool for desktop automation.
// displayWidth and displayHeight specify the screen dimensions.
func ComputerUse(displayWidth, displayHeight int) types.Tool {
	return types.Tool{
		Type: types.ToolTypeComputerUse,
		Config: types.ComputerUseConfig{
			DisplayWidth:  displayWidth,
			DisplayHeight: displayHeight,
		},
	}
}

// TextEditor creates a text editor tool.
// This tool allows the model to view and edit files.
func TextEditor() types.Tool {
	return types.Tool{
		Type: types.ToolTypeTextEditor,
	}
}

// FileSearchConfig configures the file search tool.
type FileSearchConfig = types.FileSearchConfig

// FileSearch creates a file search tool (OpenAI-specific).
// This tool allows the model to search through uploaded files.
func FileSearch(configs ...FileSearchConfig) types.Tool {
	var cfg *FileSearchConfig
	if len(configs) > 0 {
		cfg = &configs[0]
	}
	return types.Tool{
		Type:   types.ToolTypeFileSearch,
		Config: cfg,
	}
}

// --- Tool Choice Helpers ---

// ToolChoiceAuto returns a ToolChoice that lets the model decide.
func ToolChoiceAuto() *types.ToolChoice {
	return types.ToolChoiceAuto()
}

// ToolChoiceAny forces the model to use at least one tool.
func ToolChoiceAny() *types.ToolChoice {
	return types.ToolChoiceAny()
}

// ToolChoiceNone prevents the model from using tools.
func ToolChoiceNone() *types.ToolChoice {
	return types.ToolChoiceNone()
}

// ToolChoiceTool forces the model to use a specific tool.
func ToolChoiceTool(name string) *types.ToolChoice {
	return types.ToolChoiceTool(name)
}
