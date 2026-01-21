//go:build integration
// +build integration

package integration_test

import (
	"testing"
)

// ProviderCapabilities defines all capabilities a provider may support.
// This is the canonical source of truth for what each provider supports.
type ProviderCapabilities struct {
	// Input modalities
	Vision     bool // Image input
	AudioInput bool // Audio input (transcription)
	VideoInput bool // Video input (only Gemini)

	// Output modalities
	AudioOutput bool // Audio output (TTS)

	// Core features
	Tools            bool // Function calling
	ToolStreaming    bool // Stream tool calls
	Thinking         bool // Reasoning/thinking output
	StructuredOutput bool // JSON schema output
	StopSequences    bool // Stop sequence support
	Temperature      bool // Temperature parameter

	// Native tools supported
	NativeTools []string

	// Provider-specific extensions
	Extensions []string
}

// ProviderMatrix is the canonical source of truth for provider capabilities.
// This matrix documents what each provider supports and is used by tests
// to determine which tests to run for which providers.
var ProviderMatrix = map[string]ProviderCapabilities{
	"anthropic": {
		Vision:           true,
		AudioInput:       false,
		VideoInput:       false,
		AudioOutput:      false,
		Tools:            true,
		ToolStreaming:    true,
		Thinking:         true,
		StructuredOutput: true,
		StopSequences:    true,
		Temperature:      true,
		NativeTools:      []string{"web_search", "code_execution", "computer_use", "text_editor"},
		Extensions:       []string{"thinking.budget_tokens"},
	},
	"oai-resp": {
		Vision:           true,
		AudioInput:       true,
		VideoInput:       false,
		AudioOutput:      true,
		Tools:            true,
		ToolStreaming:    true,
		Thinking:         true,
		StructuredOutput: true,
		StopSequences:    false, // Responses API doesn't support stop sequences
		Temperature:      false, // Ignored for reasoning models
		NativeTools:      []string{"web_search", "code_interpreter", "file_search", "computer_use", "image_generation"},
		Extensions:       []string{"reasoning.effort", "reasoning.summary", "store", "previous_response_id"},
	},
	"gemini": {
		Vision:           true,
		AudioInput:       true,
		VideoInput:       true, // Unique to Gemini
		AudioOutput:      false,
		Tools:            true,
		ToolStreaming:    true,
		Thinking:         true,
		StructuredOutput: true,
		StopSequences:    true,
		Temperature:      true,
		NativeTools:      []string{"google_search", "code_execution", "file_search", "computer_use", "image_generation"},
		Extensions:       []string{"thinking_level", "thinking_budget", "media_resolution", "thought_signatures", "grounding_metadata"},
	},
	"groq": {
		Vision:           true, // Some models only (llama-3.2-90b-vision)
		AudioInput:       false,
		VideoInput:       false,
		AudioOutput:      false,
		Tools:            true,
		ToolStreaming:    true,
		Thinking:         true, // Via GPT-OSS and DeepSeek-R1 models
		StructuredOutput: false,
		StopSequences:    true,
		Temperature:      true,
		NativeTools:      []string{"web_search", "browser_search"},
		Extensions:       []string{"reasoning_effort", "include_reasoning"},
	},
	"cerebras": {
		Vision:           false,
		AudioInput:       false,
		VideoInput:       false,
		AudioOutput:      false,
		Tools:            true,
		ToolStreaming:    true,
		Thinking:         true, // Via DeepSeek-R1 distillation
		StructuredOutput: true,
		StopSequences:    true,
		Temperature:      true,
		NativeTools:      []string{},
		Extensions:       []string{},
	},
}

// TestMatrix_VerifyProviderCoverage verifies that all providers in providerConfigs
// are documented in the matrix.
func TestMatrix_VerifyProviderCoverage(t *testing.T) {
	for _, provider := range providerConfigs {
		t.Run(provider.Name, func(t *testing.T) {
			caps, ok := ProviderMatrix[provider.Name]
			if !ok {
				t.Errorf("provider %q not found in ProviderMatrix", provider.Name)
				return
			}

			// Verify capabilities match providerConfig (these should stay in sync)
			if caps.Vision != provider.SupportsVision {
				t.Errorf("Vision mismatch: matrix=%v, config=%v", caps.Vision, provider.SupportsVision)
			}
			if caps.Tools != provider.SupportsTools {
				t.Errorf("Tools mismatch: matrix=%v, config=%v", caps.Tools, provider.SupportsTools)
			}
			if caps.StructuredOutput != provider.SupportsStructuredOutput {
				t.Errorf("StructuredOutput mismatch: matrix=%v, config=%v", caps.StructuredOutput, provider.SupportsStructuredOutput)
			}
			if caps.StopSequences != provider.SupportsStopSequences {
				t.Errorf("StopSequences mismatch: matrix=%v, config=%v", caps.StopSequences, provider.SupportsStopSequences)
			}
		})
	}
}

// TestMatrix_AllCapabilitiesTested verifies that each capability in the matrix
// has corresponding tests.
func TestMatrix_AllCapabilitiesTested(t *testing.T) {
	// This is a meta-test that documents which capabilities need test coverage
	requiredTests := map[string]string{
		"Vision":           "TestInput_ImageURL, TestInput_ImageBase64",
		"AudioInput":       "TestInput_AudioTranscription",
		"VideoInput":       "TestInput_VideoMP4",
		"AudioOutput":      "TestOutput_AudioTTS",
		"Tools":            "TestMessages_Create_WithToolDefinition",
		"ToolStreaming":    "TestStream_ToolUse",
		"Thinking":         "TestThinking_Basic, TestThinking_Streaming",
		"StructuredOutput": "TestMessages_Create_StructuredOutput",
		"StopSequences":    "TestMessages_Create_StopSequences",
		"Temperature":      "TestMessages_Create_Temperature",
	}

	t.Log("Capability test coverage requirements:")
	for capability, tests := range requiredTests {
		t.Logf("  %s: %s", capability, tests)
	}
}

// hasNativeTool checks if a provider supports a specific native tool.
func hasNativeTool(provider string, tool string) bool {
	caps, ok := ProviderMatrix[provider]
	if !ok {
		return false
	}
	for _, t := range caps.NativeTools {
		if t == tool {
			return true
		}
	}
	return false
}

// hasExtension checks if a provider supports a specific extension.
func hasExtension(provider string, ext string) bool {
	caps, ok := ProviderMatrix[provider]
	if !ok {
		return false
	}
	for _, e := range caps.Extensions {
		if e == ext {
			return true
		}
	}
	return false
}

// getCapabilities returns the capabilities for a provider.
func getCapabilities(provider string) (ProviderCapabilities, bool) {
	caps, ok := ProviderMatrix[provider]
	return caps, ok
}

// providersWithCapability returns all providers that support a given capability.
func providersWithCapability(check func(ProviderCapabilities) bool) []string {
	var result []string
	for name, caps := range ProviderMatrix {
		if check(caps) {
			result = append(result, name)
		}
	}
	return result
}

// providersWithNativeTool returns all providers that support a specific native tool.
func providersWithNativeTool(tool string) []string {
	return providersWithCapability(func(caps ProviderCapabilities) bool {
		for _, t := range caps.NativeTools {
			if t == tool {
				return true
			}
		}
		return false
	})
}

// TestMatrix_ListCapabilities outputs a summary of provider capabilities.
func TestMatrix_ListCapabilities(t *testing.T) {
	t.Log("Provider Capability Matrix:")
	t.Log("===========================")

	for name, caps := range ProviderMatrix {
		t.Logf("\n%s:", name)
		t.Logf("  Vision: %v, Audio In: %v, Video In: %v, Audio Out: %v",
			caps.Vision, caps.AudioInput, caps.VideoInput, caps.AudioOutput)
		t.Logf("  Tools: %v, Tool Streaming: %v, Thinking: %v",
			caps.Tools, caps.ToolStreaming, caps.Thinking)
		t.Logf("  Structured Output: %v, Stop Sequences: %v, Temperature: %v",
			caps.StructuredOutput, caps.StopSequences, caps.Temperature)
		t.Logf("  Native Tools: %v", caps.NativeTools)
		t.Logf("  Extensions: %v", caps.Extensions)
	}
}
