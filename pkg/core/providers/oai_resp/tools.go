package oai_resp

import (
	"encoding/json"

	"github.com/vango-go/vai/pkg/core/types"
)

// translateTools converts Vango tools to OpenAI Responses API format.
func (p *Provider) translateTools(tools []types.Tool) []responsesTool {
	result := make([]responsesTool, 0, len(tools))

	for _, tool := range tools {
		switch tool.Type {
		case types.ToolTypeFunction:
			// Convert function tool
			respTool := responsesTool{
				Type:        "function",
				Name:        tool.Name,
				Description: tool.Description,
			}
			if tool.InputSchema != nil {
				schemaBytes, _ := json.Marshal(tool.InputSchema)
				respTool.Parameters = schemaBytes
			}
			result = append(result, respTool)

		case types.ToolTypeWebSearch:
			// Convert web search tool
			respTool := responsesTool{
				Type: "web_search",
			}
			// Apply config if present
			if cfg, ok := tool.Config.(*types.WebSearchConfig); ok && cfg != nil {
				// Map user location or other config if needed
				respTool.SearchContextSize = "medium" // default
			}
			result = append(result, respTool)

		case types.ToolTypeCodeExecution:
			// Convert code execution -> code_interpreter
			respTool := responsesTool{
				Type: "code_interpreter",
			}
			// Apply container config if present
			if cfg, ok := tool.Config.(*types.CodeExecutionConfig); ok && cfg != nil {
				respTool.Container = &containerConfig{
					Type: "auto",
				}
			}
			result = append(result, respTool)

		case types.ToolTypeFileSearch:
			// Convert file search tool
			respTool := responsesTool{
				Type: "file_search",
			}
			// Extract vector store IDs from config or extensions
			if cfg, ok := tool.Config.(map[string]any); ok {
				if vsIDs, exists := cfg["vector_store_ids"]; exists {
					if ids, ok := vsIDs.([]string); ok {
						respTool.VectorStoreIDs = ids
					} else if ids, ok := vsIDs.([]any); ok {
						for _, id := range ids {
							if s, ok := id.(string); ok {
								respTool.VectorStoreIDs = append(respTool.VectorStoreIDs, s)
							}
						}
					}
				}
			}
			result = append(result, respTool)

		case types.ToolTypeComputerUse:
			// Convert computer use tool
			respTool := responsesTool{
				Type:        "computer_use_preview",
				Environment: "browser",
			}
			// Apply display config if present
			if cfg, ok := tool.Config.(*types.ComputerUseConfig); ok && cfg != nil {
				respTool.DisplayWidth = cfg.DisplayWidth
				respTool.DisplayHeight = cfg.DisplayHeight
			} else {
				// Defaults
				respTool.DisplayWidth = 1024
				respTool.DisplayHeight = 768
			}
			result = append(result, respTool)

		case "image_generation":
			// Image generation tool (OpenAI Responses API native)
			result = append(result, responsesTool{
				Type: "image_generation",
			})

		case types.ToolTypeTextEditor:
			// Text editor is Anthropic-specific, skip for OpenAI
			continue
		}
	}

	return result
}

// translateToolChoice converts Vango tool choice to OpenAI Responses API format.
func (p *Provider) translateToolChoice(tc *types.ToolChoice) any {
	if tc == nil {
		return nil
	}

	switch tc.Type {
	case "auto":
		return "auto"
	case "none":
		return "none"
	case "any":
		return "required"
	case "tool":
		return map[string]any{
			"type": "function",
			"name": tc.Name,
		}
	}
	return "auto"
}
