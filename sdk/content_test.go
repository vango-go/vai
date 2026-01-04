package vai

import (
	"testing"
)

func TestText(t *testing.T) {
	block := Text("Hello, world!")
	if block.BlockType() != "text" {
		t.Errorf("BlockType() = %q, want %q", block.BlockType(), "text")
	}
}

func TestImage(t *testing.T) {
	block := Image([]byte("test data"), "image/png")
	if block.BlockType() != "image" {
		t.Errorf("BlockType() = %q, want %q", block.BlockType(), "image")
	}
}

func TestImageURL(t *testing.T) {
	block := ImageURL("https://example.com/image.png")
	if block.BlockType() != "image" {
		t.Errorf("BlockType() = %q, want %q", block.BlockType(), "image")
	}
}

func TestAudio(t *testing.T) {
	block := Audio([]byte("audio data"), "audio/wav")
	if block.BlockType() != "audio" {
		t.Errorf("BlockType() = %q, want %q", block.BlockType(), "audio")
	}
}

func TestVideo(t *testing.T) {
	block := Video([]byte("video data"), "video/mp4")
	if block.BlockType() != "video" {
		t.Errorf("BlockType() = %q, want %q", block.BlockType(), "video")
	}
}

func TestDocument(t *testing.T) {
	block := Document([]byte("pdf data"), "application/pdf", "report.pdf")
	if block.BlockType() != "document" {
		t.Errorf("BlockType() = %q, want %q", block.BlockType(), "document")
	}
}

func TestToolResult(t *testing.T) {
	block := ToolResult("call_123", []ContentBlock{Text("result")})
	if block.BlockType() != "tool_result" {
		t.Errorf("BlockType() = %q, want %q", block.BlockType(), "tool_result")
	}
}

func TestToolResultError(t *testing.T) {
	block := ToolResultError("call_123", "error message")
	if block.BlockType() != "tool_result" {
		t.Errorf("BlockType() = %q, want %q", block.BlockType(), "tool_result")
	}
}

func TestContentBlocks(t *testing.T) {
	blocks := ContentBlocks(
		Text("Hello"),
		ImageURL("https://example.com/img.png"),
	)
	if len(blocks) != 2 {
		t.Errorf("len(blocks) = %d, want 2", len(blocks))
	}
}

func TestWebSearch(t *testing.T) {
	// Test without config
	tool := WebSearch()
	if tool.Type != "web_search" {
		t.Errorf("Type = %q, want %q", tool.Type, "web_search")
	}

	// Test with config
	toolWithConfig := WebSearch(WebSearchConfig{
		MaxUses:        5,
		AllowedDomains: []string{"example.com"},
	})
	if toolWithConfig.Type != "web_search" {
		t.Errorf("Type = %q, want %q", toolWithConfig.Type, "web_search")
	}
	if toolWithConfig.Config == nil {
		t.Error("Config should not be nil")
	}
}

func TestCodeExecution(t *testing.T) {
	// Test without config
	tool := CodeExecution()
	if tool.Type != "code_execution" {
		t.Errorf("Type = %q, want %q", tool.Type, "code_execution")
	}

	// Test with config
	toolWithConfig := CodeExecution(CodeExecutionConfig{
		Languages:      []string{"python", "javascript"},
		TimeoutSeconds: 60,
	})
	if toolWithConfig.Type != "code_execution" {
		t.Errorf("Type = %q, want %q", toolWithConfig.Type, "code_execution")
	}
	if toolWithConfig.Config == nil {
		t.Error("Config should not be nil")
	}
}

func TestComputerUse(t *testing.T) {
	tool := ComputerUse(1920, 1080)
	if tool.Type != "computer_use" {
		t.Errorf("Type = %q, want %q", tool.Type, "computer_use")
	}
	if tool.Config == nil {
		t.Error("Config should not be nil")
	}
}

func TestTextEditor(t *testing.T) {
	tool := TextEditor()
	if tool.Type != "text_editor" {
		t.Errorf("Type = %q, want %q", tool.Type, "text_editor")
	}
}

func TestFileSearch(t *testing.T) {
	// Test without config
	tool := FileSearch()
	if tool.Type != "file_search" {
		t.Errorf("Type = %q, want %q", tool.Type, "file_search")
	}

	// Test with config
	toolWithConfig := FileSearch(FileSearchConfig{MaxResults: 10})
	if toolWithConfig.Type != "file_search" {
		t.Errorf("Type = %q, want %q", toolWithConfig.Type, "file_search")
	}
}

func TestToolChoiceHelpers(t *testing.T) {
	auto := ToolChoiceAuto()
	if auto.Type != "auto" {
		t.Errorf("ToolChoiceAuto().Type = %q, want %q", auto.Type, "auto")
	}

	any := ToolChoiceAny()
	if any.Type != "any" {
		t.Errorf("ToolChoiceAny().Type = %q, want %q", any.Type, "any")
	}

	none := ToolChoiceNone()
	if none.Type != "none" {
		t.Errorf("ToolChoiceNone().Type = %q, want %q", none.Type, "none")
	}

	specific := ToolChoiceTool("my_tool")
	if specific.Type != "tool" {
		t.Errorf("ToolChoiceTool().Type = %q, want %q", specific.Type, "tool")
	}
	if specific.Name != "my_tool" {
		t.Errorf("ToolChoiceTool().Name = %q, want %q", specific.Name, "my_tool")
	}
}
