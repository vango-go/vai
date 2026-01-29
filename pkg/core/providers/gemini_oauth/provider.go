package gemini_oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/vango-go/vai/pkg/core"
	"github.com/vango-go/vai/pkg/core/types"
)

var debugRequests = os.Getenv("DEBUG_GEMINI_OAUTH") != ""

// Provider implements the Gemini OAuth provider using Cloud Code Assist.
type Provider struct {
	projectID   string
	tokenSource TokenSource
	httpClient  *http.Client
	credsPath   string
	creds       *Credentials
}

// New creates a new Gemini OAuth provider.
func New(opts ...Option) (*Provider, error) {
	p := &Provider{
		httpClient: &http.Client{},
	}

	for _, opt := range opts {
		opt(p)
	}

	// If no token source provided, create one from credentials
	if p.tokenSource == nil {
		var creds *Credentials
		var err error

		if p.creds != nil {
			// Use provided credentials
			creds = p.creds
		} else {
			// Load from file
			store := NewFileCredentialsStore(p.credsPath)
			creds, err = store.Load()
			if err != nil {
				return nil, fmt.Errorf("load credentials: %w", err)
			}
		}

		// Use project ID from credentials if not explicitly set
		if p.projectID == "" && creds.ProjectID != "" {
			p.projectID = creds.ProjectID
		}

		// Create token source with auto-refresh
		store := NewFileCredentialsStore(p.credsPath)
		p.tokenSource = NewOAuthTokenSource(creds, store)
	}

	if p.projectID == "" {
		return nil, fmt.Errorf("project ID is required")
	}

	return p, nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "gemini-oauth"
}

// Capabilities returns the provider's capabilities.
func (p *Provider) Capabilities() core.ProviderCapabilities {
	return core.ProviderCapabilities{
		Vision:           true,
		AudioInput:       true,
		AudioOutput:      false,
		Video:            true,
		Tools:            true,
		ToolStreaming:    true,
		Thinking:         true,
		StructuredOutput: true,
		NativeTools:      []string{"web_search", "code_execution"},
	}
}

// CreateMessage sends a non-streaming message request.
func (p *Provider) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
	model := stripProviderPrefix(req.Model)

	// Apply model fallbacks
	if fallback, ok := ModelFallbacks[model]; ok {
		model = fallback
	}

	// Build Gemini request
	geminiReq := p.buildRequest(req)

	// Send request
	respBody, err := p.doRequest(ctx, model, geminiReq)
	if err != nil {
		return nil, err
	}

	// Parse response
	return p.parseResponse(respBody, model)
}

// StreamMessage sends a streaming message request.
func (p *Provider) StreamMessage(ctx context.Context, req *types.MessageRequest) (EventStream, error) {
	model := stripProviderPrefix(req.Model)

	// Apply model fallbacks
	if fallback, ok := ModelFallbacks[model]; ok {
		model = fallback
	}

	// Build Gemini request
	geminiReq := p.buildRequest(req)

	// Send streaming request
	body, err := p.doStreamRequest(ctx, model, geminiReq)
	if err != nil {
		return nil, err
	}

	return newEventStream(body, model), nil
}

// stripProviderPrefix removes the provider prefix from a model string.
func stripProviderPrefix(model string) string {
	if idx := strings.Index(model, "/"); idx != -1 {
		return model[idx+1:]
	}
	return model
}

// =============================================================================
// Gemini API Types (duplicated from gemini provider for independence)
// =============================================================================

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
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

// geminiPart represents a single part within content.
type geminiPart struct {
	Text             string                   `json:"text,omitempty"`
	InlineData       *geminiBlob              `json:"inlineData,omitempty"`
	FileData         *geminiFileData          `json:"fileData,omitempty"`
	FunctionCall     *geminiFunctionCall      `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse  `json:"functionResponse,omitempty"`
	ThoughtSignature string                   `json:"thoughtSignature,omitempty"`
}

// geminiBlob represents inline binary data.
type geminiBlob struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

// geminiFileData represents a file reference.
type geminiFileData struct {
	MIMEType string `json:"mimeType,omitempty"`
	FileURI  string `json:"fileUri"`
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
	Mode                 string   `json:"mode,omitempty"`
	AllowedFunctionNames []string `json:"allowed_function_names,omitempty"`
}

// geminiGenConfig contains generation configuration.
type geminiGenConfig struct {
	Temperature      *float64              `json:"temperature,omitempty"`
	TopP             *float64              `json:"top_p,omitempty"`
	TopK             *int                  `json:"top_k,omitempty"`
	MaxOutputTokens  *int                  `json:"max_output_tokens,omitempty"`
	StopSequences    []string              `json:"stop_sequences,omitempty"`
	ResponseMIMEType string                `json:"response_mime_type,omitempty"`
	ResponseSchema   json.RawMessage       `json:"response_schema,omitempty"`
	ThinkingConfig   *geminiThinkingConfig `json:"thinking_config,omitempty"`
}

// geminiThinkingConfig controls thinking/reasoning behavior.
type geminiThinkingConfig struct {
	ThinkingBudget  *int   `json:"thinking_budget,omitempty"`
	ThinkingLevel   string `json:"thinking_level,omitempty"`
	IncludeThoughts bool   `json:"include_thoughts,omitempty"`
}

// geminiSafetySetting configures safety filters.
type geminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// geminiResponse is the Gemini API response format.
type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsage      `json:"usageMetadata,omitempty"`
	ModelVersion  string            `json:"modelVersion,omitempty"`
}

// geminiCandidate represents a single candidate response.
type geminiCandidate struct {
	Content       geminiContent `json:"content"`
	FinishReason  string        `json:"finishReason"`
	Index         int           `json:"index"`
	SafetyRatings []safetyRating `json:"safetyRatings,omitempty"`
}

// geminiUsage contains token usage information.
type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
	ThinkingTokenCount   int `json:"thinkingTokenCount,omitempty"`
}

// safetyRating represents a safety assessment.
type safetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
	Blocked     bool   `json:"blocked"`
}

// =============================================================================
// Request Building
// =============================================================================

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

func (p *Provider) translateMessages(messages []types.Message) []geminiContent {
	contents := make([]geminiContent, 0, len(messages))

	for _, msg := range messages {
		blocks := msg.ContentBlocks()

		hasToolResults := false
		for _, block := range blocks {
			if _, ok := block.(types.ToolResultBlock); ok {
				hasToolResults = true
				break
			}
		}

		if hasToolResults {
			// Gemini requires all function responses in a single content block
			var parts []geminiPart
			for _, block := range blocks {
				if tr, ok := block.(types.ToolResultBlock); ok {
					parts = append(parts, geminiPart{
						FunctionResponse: &geminiFunctionResponse{
							Name:     p.getToolNameFromID(tr.ToolUseID, messages),
							Response: p.toolResultToMap(tr.Content),
						},
					})
				}
			}
			if len(parts) > 0 {
				contents = append(contents, geminiContent{
					Role:  "function",
					Parts: parts,
				})
			}
			continue
		}

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

func (p *Provider) translateContentBlocks(blocks []types.ContentBlock) []geminiPart {
	parts := make([]geminiPart, 0, len(blocks))

	for _, block := range blocks {
		switch b := block.(type) {
		case types.TextBlock:
			parts = append(parts, geminiPart{Text: b.Text})

		case types.ImageBlock:
			part := geminiPart{}
			if b.Source.Type == "url" {
				part.FileData = &geminiFileData{
					MIMEType: b.Source.MediaType,
					FileURI:  b.Source.URL,
				}
			} else {
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
			part := geminiPart{
				FunctionCall: &geminiFunctionCall{
					Name: b.Name,
					Args: b.Input,
				},
			}
			if b.Input != nil {
				if sig, ok := b.Input["__thought_signature"].(string); ok {
					part.ThoughtSignature = sig
					delete(b.Input, "__thought_signature")
				}
			}
			parts = append(parts, part)

		case types.ToolResultBlock:
			continue
		}
	}

	return parts
}

func (p *Provider) translateTools(tools []types.Tool) []geminiTool {
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
			gs := &geminiGoogleSearch{}
			if cfg, ok := tool.Config.(*types.WebSearchConfig); ok && cfg != nil {
				gs.ExcludeDomains = cfg.BlockedDomains
			}
			result = append(result, geminiTool{GoogleSearch: gs})

		case types.ToolTypeCodeExecution:
			result = append(result, geminiTool{CodeExecution: &geminiCodeExecution{}})
		}
	}

	if len(funcDecls) > 0 {
		result = append(result, geminiTool{FunctionDeclarations: funcDecls})
	}

	return result
}

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

func (p *Provider) buildGenerationConfig(req *types.MessageRequest) *geminiGenConfig {
	config := &geminiGenConfig{
		Temperature: req.Temperature,
		TopP:        req.TopP,
		TopK:        req.TopK,
	}

	if req.MaxTokens > 0 {
		config.MaxOutputTokens = &req.MaxTokens
	}

	if len(req.StopSequences) > 0 {
		config.StopSequences = req.StopSequences
	}

	if req.OutputFormat != nil && req.OutputFormat.Type == "json_schema" {
		config.ResponseMIMEType = "application/json"
		if req.OutputFormat.JSONSchema != nil {
			schemaBytes, _ := json.Marshal(req.OutputFormat.JSONSchema)
			config.ResponseSchema = schemaBytes
		}
	}

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

func (p *Provider) getToolNameFromID(toolUseID string, messages []types.Message) string {
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
	return toolUseID
}

func (p *Provider) toolResultToMap(content []types.ContentBlock) map[string]any {
	result := make(map[string]any)

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

// =============================================================================
// Response Parsing
// =============================================================================

func (p *Provider) parseResponse(body []byte, model string) (*types.MessageResponse, error) {
	var geminiResp geminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in response")
	}

	candidate := geminiResp.Candidates[0]

	if debugRequests {
		log.Printf("[GEMINI-OAUTH] Parsed %d parts", len(candidate.Content.Parts))
		for i, part := range candidate.Content.Parts {
			log.Printf("[GEMINI-OAUTH] Part %d: Text=%q, FunctionCall=%v, ThoughtSig=%q",
				i, part.Text, part.FunctionCall, part.ThoughtSignature)
		}
	}

	content := p.parseContentParts(candidate.Content.Parts)
	stopReason := mapFinishReason(candidate.FinishReason)

	// Cloud Code API returns "STOP" even for function calls, so check content
	for _, block := range content {
		if _, ok := block.(types.ToolUseBlock); ok {
			stopReason = types.StopReasonToolUse
			break
		}
	}

	usage := types.Usage{}
	if geminiResp.UsageMetadata != nil {
		usage.InputTokens = geminiResp.UsageMetadata.PromptTokenCount
		usage.OutputTokens = geminiResp.UsageMetadata.CandidatesTokenCount
		usage.TotalTokens = geminiResp.UsageMetadata.TotalTokenCount
	}

	return &types.MessageResponse{
		ID:         fmt.Sprintf("msg_%s", model),
		Type:       "message",
		Role:       "assistant",
		Model:      "gemini-oauth/" + model,
		Content:    content,
		StopReason: stopReason,
		Usage:      usage,
	}, nil
}

func (p *Provider) parseContentParts(parts []geminiPart) []types.ContentBlock {
	content := make([]types.ContentBlock, 0, len(parts))

	for _, part := range parts {
		if part.Text != "" {
			content = append(content, types.TextBlock{
				Type: "text",
				Text: part.Text,
			})
		}

		if part.FunctionCall != nil {
			input := part.FunctionCall.Args
			if input == nil {
				input = make(map[string]any)
			}

			if part.ThoughtSignature != "" {
				input["__thought_signature"] = part.ThoughtSignature
			}

			content = append(content, types.ToolUseBlock{
				Type:  "tool_use",
				ID:    fmt.Sprintf("call_%s", part.FunctionCall.Name),
				Name:  part.FunctionCall.Name,
				Input: input,
			})
		}

		if part.InlineData != nil && part.InlineData.MIMEType == "text/x-python" {
			content = append(content, types.TextBlock{
				Type: "text",
				Text: part.InlineData.Data,
			})
		}
	}

	return content
}

func mapFinishReason(reason string) types.StopReason {
	switch reason {
	case "STOP":
		return types.StopReasonEndTurn
	case "MAX_TOKENS":
		return types.StopReasonMaxTokens
	case "SAFETY":
		return types.StopReasonEndTurn
	case "RECITATION":
		return types.StopReasonEndTurn
	case "TOOL_USE", "FUNCTION_CALL":
		return types.StopReasonToolUse
	default:
		return types.StopReasonEndTurn
	}
}
