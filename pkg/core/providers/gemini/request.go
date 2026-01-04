package gemini

import (
	"encoding/json"
	"strings"

	"github.com/vango-go/vai/pkg/core/types"
)

// geminiRequest is the Gemini API request format.
type geminiRequest struct {
	Contents          []geminiContent       `json:"contents"`
	SystemInstruction *geminiContent        `json:"system_instruction,omitempty"`
	Tools             []geminiTool          `json:"tools,omitempty"`
	ToolConfig        *geminiToolConfig     `json:"tool_config,omitempty"`
	GenerationConfig  *geminiGenConfig      `json:"generation_config,omitempty"`
	SafetySettings    []geminiSafetySetting `json:"safety_settings,omitempty"`
}

// geminiContent represents a content object in Gemini format.
type geminiContent struct {
	Role  string       `json:"role,omitempty"` // "user", "model", "function"
	Parts []geminiPart `json:"parts"`
}

// geminiPart represents a single part within content.
type geminiPart struct {
	Text             string                   `json:"text,omitempty"`
	InlineData       *geminiBlob              `json:"inline_data,omitempty"`
	FileData         *geminiFileData          `json:"file_data,omitempty"`
	FunctionCall     *geminiFunctionCall      `json:"function_call,omitempty"`
	FunctionResponse *geminiFunctionResponse  `json:"function_response,omitempty"`
	ThoughtSignature string                   `json:"thought_signature,omitempty"`
}

// geminiBlob represents inline binary data.
type geminiBlob struct {
	MIMEType string `json:"mime_type"`
	Data     string `json:"data"` // base64 encoded
}

// geminiFileData represents a file reference.
type geminiFileData struct {
	MIMEType string `json:"mime_type,omitempty"`
	FileURI  string `json:"file_uri"`
}

// geminiFunctionCall represents a function call from the model.
type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

// geminiFunctionResponse represents a function response.
type geminiFunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

// geminiTool represents a tool definition.
type geminiTool struct {
	FunctionDeclarations []geminiFunctionDecl `json:"function_declarations,omitempty"`
	GoogleSearch         *geminiGoogleSearch  `json:"google_search,omitempty"`
	CodeExecution        *geminiCodeExecution `json:"code_execution,omitempty"`
}

// geminiFunctionDecl represents a function declaration.
type geminiFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// geminiGoogleSearch configures Google Search grounding.
type geminiGoogleSearch struct {
	ExcludeDomains []string `json:"exclude_domains,omitempty"`
}

// geminiCodeExecution configures code execution.
type geminiCodeExecution struct{}

// geminiToolConfig configures tool behavior.
type geminiToolConfig struct {
	FunctionCallingConfig *geminiFunctionCallingConfig `json:"function_calling_config,omitempty"`
}

// geminiFunctionCallingConfig controls function calling behavior.
type geminiFunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"` // AUTO, ANY, NONE
	AllowedFunctionNames []string `json:"allowed_function_names,omitempty"`
}

// geminiGenConfig contains generation configuration.
type geminiGenConfig struct {
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"top_p,omitempty"`
	TopK             *int            `json:"top_k,omitempty"`
	MaxOutputTokens  *int            `json:"max_output_tokens,omitempty"`
	StopSequences    []string        `json:"stop_sequences,omitempty"`
	ResponseMIMEType string          `json:"response_mime_type,omitempty"`
	ResponseSchema   json.RawMessage `json:"response_schema,omitempty"`
	ThinkingConfig   *geminiThinkingConfig `json:"thinking_config,omitempty"`
}

// geminiThinkingConfig controls thinking/reasoning behavior.
type geminiThinkingConfig struct {
	ThinkingBudget  *int   `json:"thinking_budget,omitempty"`   // For Gemini 2.5
	ThinkingLevel   string `json:"thinking_level,omitempty"`    // For Gemini 3: "low" or "high"
	IncludeThoughts bool   `json:"include_thoughts,omitempty"`
}

// geminiSafetySetting configures safety filters.
type geminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// buildRequest converts a Vango request to a Gemini request.
func (p *Provider) buildRequest(req *types.MessageRequest) *geminiRequest {
	geminiReq := &geminiRequest{}

	// Translate system instruction
	if req.System != nil {
		geminiReq.SystemInstruction = p.translateSystemInstruction(req.System)
	}

	// Translate messages to contents
	geminiReq.Contents = p.translateMessages(req.Messages)

	// Translate tools
	if len(req.Tools) > 0 {
		geminiReq.Tools = p.translateTools(req.Tools)
	}

	// Translate tool choice
	if req.ToolChoice != nil {
		geminiReq.ToolConfig = p.translateToolChoice(req.ToolChoice)
	}

	// Build generation config
	geminiReq.GenerationConfig = p.buildGenerationConfig(req)

	return geminiReq
}

// translateSystemInstruction converts system prompt to Gemini format.
func (p *Provider) translateSystemInstruction(system any) *geminiContent {
	content := &geminiContent{
		Parts: []geminiPart{},
	}

	switch s := system.(type) {
	case string:
		content.Parts = append(content.Parts, geminiPart{Text: s})
	case []types.ContentBlock:
		for _, block := range s {
			if tb, ok := block.(types.TextBlock); ok {
				content.Parts = append(content.Parts, geminiPart{Text: tb.Text})
			}
		}
	}

	return content
}

// translateMessages converts Vango messages to Gemini contents.
func (p *Provider) translateMessages(messages []types.Message) []geminiContent {
	contents := make([]geminiContent, 0, len(messages))

	for _, msg := range messages {
		blocks := msg.ContentBlocks()

		// Check if this is a tool result message
		// In Gemini, tool results have role "function"
		hasToolResults := false
		for _, block := range blocks {
			if _, ok := block.(types.ToolResultBlock); ok {
				hasToolResults = true
				break
			}
		}

		if hasToolResults {
			// Create separate function response content for each tool result
			for _, block := range blocks {
				if tr, ok := block.(types.ToolResultBlock); ok {
					contents = append(contents, geminiContent{
						Role: "function",
						Parts: []geminiPart{{
							FunctionResponse: &geminiFunctionResponse{
								Name:     p.getToolNameFromID(tr.ToolUseID, messages),
								Response: p.toolResultToMap(tr.Content),
							},
						}},
					})
				}
			}
			continue
		}

		// Translate role
		role := msg.Role
		if role == "assistant" {
			role = "model"
		}

		content := geminiContent{
			Role:  role,
			Parts: p.translateContentBlocks(blocks),
		}

		contents = append(contents, content)
	}

	return contents
}

// translateContentBlocks converts Vango content blocks to Gemini parts.
func (p *Provider) translateContentBlocks(blocks []types.ContentBlock) []geminiPart {
	parts := make([]geminiPart, 0, len(blocks))

	for _, block := range blocks {
		switch b := block.(type) {
		case types.TextBlock:
			parts = append(parts, geminiPart{Text: b.Text})

		case types.ImageBlock:
			part := geminiPart{}
			if b.Source.Type == "url" {
				// Gemini prefers inline_data for URLs too, but can use file_data for gs:// URIs
				if strings.HasPrefix(b.Source.URL, "gs://") {
					part.FileData = &geminiFileData{
						MIMEType: b.Source.MediaType,
						FileURI:  b.Source.URL,
					}
				} else {
					// For HTTP URLs, we pass as file_data (Gemini will fetch)
					part.FileData = &geminiFileData{
						MIMEType: b.Source.MediaType,
						FileURI:  b.Source.URL,
					}
				}
			} else {
				// Base64 data
				part.InlineData = &geminiBlob{
					MIMEType: b.Source.MediaType,
					Data:     b.Source.Data,
				}
			}
			parts = append(parts, part)

		case types.AudioBlock:
			parts = append(parts, geminiPart{
				InlineData: &geminiBlob{
					MIMEType: b.Source.MediaType,
					Data:     b.Source.Data,
				},
			})

		case types.VideoBlock:
			part := geminiPart{}
			if b.Source.Type == "url" || strings.HasPrefix(b.Source.Data, "gs://") {
				// Video usually comes from file URI
				uri := b.Source.Data
				if b.Source.Type == "url" {
					uri = b.Source.Data
				}
				part.FileData = &geminiFileData{
					MIMEType: b.Source.MediaType,
					FileURI:  uri,
				}
			} else {
				part.InlineData = &geminiBlob{
					MIMEType: b.Source.MediaType,
					Data:     b.Source.Data,
				}
			}
			parts = append(parts, part)

		case types.DocumentBlock:
			parts = append(parts, geminiPart{
				InlineData: &geminiBlob{
					MIMEType: b.Source.MediaType,
					Data:     b.Source.Data,
				},
			})

		case types.ToolUseBlock:
			// Tool use from assistant message - include function call
			part := geminiPart{
				FunctionCall: &geminiFunctionCall{
					Name: b.Name,
					Args: b.Input,
				},
			}
			// Check for thought signature in metadata (for Gemini 3)
			if b.Input != nil {
				if sig, ok := b.Input["__thought_signature"].(string); ok {
					part.ThoughtSignature = sig
					// Remove from args
					delete(b.Input, "__thought_signature")
				}
			}
			parts = append(parts, part)

		case types.ToolResultBlock:
			// Tool results are handled separately with role="function"
			// Skip here - they're processed in translateMessages
			continue
		}
	}

	return parts
}

// translateTools converts Vango tools to Gemini format.
func (p *Provider) translateTools(tools []types.Tool) []geminiTool {
	// Group function declarations together
	var funcDecls []geminiFunctionDecl
	var result []geminiTool

	for _, tool := range tools {
		switch tool.Type {
		case types.ToolTypeFunction:
			schemaBytes, _ := json.Marshal(tool.InputSchema)
			funcDecls = append(funcDecls, geminiFunctionDecl{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  schemaBytes,
			})

		case types.ToolTypeWebSearch:
			// Gemini uses google_search for web search
			gs := &geminiGoogleSearch{}
			if cfg, ok := tool.Config.(*types.WebSearchConfig); ok && cfg != nil {
				gs.ExcludeDomains = cfg.BlockedDomains
			}
			result = append(result, geminiTool{GoogleSearch: gs})

		case types.ToolTypeCodeExecution:
			result = append(result, geminiTool{CodeExecution: &geminiCodeExecution{}})

		case types.ToolTypeFileSearch:
			// Gemini doesn't have a direct equivalent - skip
			continue

		case types.ToolTypeComputerUse:
			// Gemini doesn't support computer use - skip
			continue

		case types.ToolTypeTextEditor:
			// Gemini doesn't support text editor - skip
			continue
		}
	}

	// Add function declarations as a single tool if we have any
	if len(funcDecls) > 0 {
		result = append(result, geminiTool{FunctionDeclarations: funcDecls})
	}

	return result
}

// translateToolChoice converts Vango tool choice to Gemini format.
func (p *Provider) translateToolChoice(tc *types.ToolChoice) *geminiToolConfig {
	config := &geminiToolConfig{
		FunctionCallingConfig: &geminiFunctionCallingConfig{},
	}

	switch tc.Type {
	case "auto":
		config.FunctionCallingConfig.Mode = "AUTO"
	case "none":
		config.FunctionCallingConfig.Mode = "NONE"
	case "any":
		config.FunctionCallingConfig.Mode = "ANY"
	case "tool":
		config.FunctionCallingConfig.Mode = "ANY"
		config.FunctionCallingConfig.AllowedFunctionNames = []string{tc.Name}
	}

	return config
}

// buildGenerationConfig creates the generation config from request.
func (p *Provider) buildGenerationConfig(req *types.MessageRequest) *geminiGenConfig {
	config := &geminiGenConfig{
		Temperature: req.Temperature,
		TopP:        req.TopP,
		TopK:        req.TopK,
	}

	// Set max tokens
	if req.MaxTokens > 0 {
		config.MaxOutputTokens = &req.MaxTokens
	}

	// Set stop sequences
	if len(req.StopSequences) > 0 {
		config.StopSequences = req.StopSequences
	}

	// Handle structured output
	if req.OutputFormat != nil && req.OutputFormat.Type == "json_schema" {
		config.ResponseMIMEType = "application/json"
		if req.OutputFormat.JSONSchema != nil {
			schemaBytes, _ := json.Marshal(req.OutputFormat.JSONSchema)
			config.ResponseSchema = schemaBytes
		}
	}

	// Handle thinking config from extensions
	if ext, ok := req.Extensions["gemini"].(map[string]any); ok {
		if thinking, ok := ext["thinking"].(map[string]any); ok {
			config.ThinkingConfig = &geminiThinkingConfig{}
			if level, ok := thinking["level"].(string); ok {
				config.ThinkingConfig.ThinkingLevel = level
			}
			if budget, ok := thinking["budget"].(float64); ok {
				b := int(budget)
				config.ThinkingConfig.ThinkingBudget = &b
			}
			if include, ok := thinking["include_thoughts"].(bool); ok {
				config.ThinkingConfig.IncludeThoughts = include
			}
		}
	}

	return config
}

// getToolNameFromID looks up the tool name from a tool use ID in previous messages.
func (p *Provider) getToolNameFromID(toolUseID string, messages []types.Message) string {
	// Search backwards through messages to find the tool use with this ID
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != "assistant" {
			continue
		}
		for _, block := range msg.ContentBlocks() {
			if tu, ok := block.(types.ToolUseBlock); ok {
				if tu.ID == toolUseID {
					return tu.Name
				}
			}
		}
	}
	// Fallback: return the ID itself (shouldn't happen normally)
	return toolUseID
}

// toolResultToMap converts tool result content to a map.
func (p *Provider) toolResultToMap(content []types.ContentBlock) map[string]any {
	result := make(map[string]any)

	// Concatenate text content
	var text strings.Builder
	for _, block := range content {
		if tb, ok := block.(types.TextBlock); ok {
			text.WriteString(tb.Text)
		}
	}

	if text.Len() > 0 {
		result["result"] = text.String()
	}

	return result
}

// stripProviderPrefix removes the provider prefix from a model string.
// "gemini/gemini-2.5-flash" -> "gemini-2.5-flash"
func stripProviderPrefix(model string) string {
	if idx := strings.Index(model, "/"); idx != -1 {
		return model[idx+1:]
	}
	return model
}
