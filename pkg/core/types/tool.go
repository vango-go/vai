package types

// Tool represents a tool that the model can use.
type Tool struct {
	Type        string      `json:"type"` // "function", "web_search", "code_execution", etc.
	Name        string      `json:"name,omitempty"`
	Description string      `json:"description,omitempty"`
	InputSchema *JSONSchema `json:"input_schema,omitempty"`
	Config      any         `json:"config,omitempty"`
}

// ToolChoice specifies how the model should choose tools.
type ToolChoice struct {
	Type string `json:"type"`           // "auto", "any", "none", "tool"
	Name string `json:"name,omitempty"` // Required when type="tool"
}

// Native tool types
const (
	ToolTypeFunction      = "function"
	ToolTypeWebSearch     = "web_search"
	ToolTypeCodeExecution = "code_execution"
	ToolTypeFileSearch    = "file_search"
	ToolTypeComputerUse   = "computer_use"
	ToolTypeTextEditor    = "text_editor"
)

// WebSearchConfig configures the web search tool.
type WebSearchConfig struct {
	MaxUses        int      `json:"max_uses,omitempty"`        // Limit searches per request
	AllowedDomains []string `json:"allowed_domains,omitempty"` // Restrict to these domains
	BlockedDomains []string `json:"blocked_domains,omitempty"` // Exclude these domains
	UserLocation   *UserLocation `json:"user_location,omitempty"`   // Localize results
}

// UserLocation provides location context for web search.
type UserLocation struct {
	City     string `json:"city,omitempty"`
	Region   string `json:"region,omitempty"`
	Country  string `json:"country,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

// CodeExecutionConfig configures the code execution tool.
type CodeExecutionConfig struct {
	Languages      []string `json:"languages,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}

// ComputerUseConfig configures the computer use tool.
type ComputerUseConfig struct {
	DisplayWidth  int `json:"display_width"`
	DisplayHeight int `json:"display_height"`
}

// FileSearchConfig configures the file search tool.
type FileSearchConfig struct {
	MaxResults int `json:"max_results,omitempty"`
}

// TextEditorConfig configures the text editor tool.
type TextEditorConfig struct {
	// Currently no configuration options
}

// NewFunctionTool creates a new function tool.
func NewFunctionTool(name, description string, schema *JSONSchema) Tool {
	return Tool{
		Type:        ToolTypeFunction,
		Name:        name,
		Description: description,
		InputSchema: schema,
	}
}

// NewWebSearchTool creates a new web search tool.
func NewWebSearchTool(config *WebSearchConfig) Tool {
	return Tool{
		Type:   ToolTypeWebSearch,
		Config: config,
	}
}

// NewCodeExecutionTool creates a new code execution tool.
func NewCodeExecutionTool(config *CodeExecutionConfig) Tool {
	return Tool{
		Type:   ToolTypeCodeExecution,
		Config: config,
	}
}

// ToolChoiceAuto returns a ToolChoice that lets the model decide.
func ToolChoiceAuto() *ToolChoice {
	return &ToolChoice{Type: "auto"}
}

// ToolChoiceAny forces the model to use at least one tool.
func ToolChoiceAny() *ToolChoice {
	return &ToolChoice{Type: "any"}
}

// ToolChoiceNone prevents the model from using tools.
func ToolChoiceNone() *ToolChoice {
	return &ToolChoice{Type: "none"}
}

// ToolChoiceTool forces the model to use a specific tool.
func ToolChoiceTool(name string) *ToolChoice {
	return &ToolChoice{Type: "tool", Name: name}
}
