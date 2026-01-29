//go:build integration
// +build integration

package integration_test

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	vai "github.com/vango-go/vai/sdk"
)

var (
	testClient *vai.Client
	testCtx    context.Context
	fixtures   fixtureLoader
)

type providerConfig struct {
	Name    string
	Model   string
	KeyName string

	// Key requirement helper
	RequireKey func(t *testing.T)

	// Input modalities
	SupportsVision     bool
	SupportsAudioInput bool
	SupportsVideoInput bool

	// Output modalities
	SupportsAudioOutput bool

	// Core features
	SupportsTools            bool
	SupportsToolStreaming    bool
	SupportsThinking         bool
	SupportsStructuredOutput bool
	SupportsStopSequences    bool
	SupportsTemperature      bool

	// Native tools this provider supports
	NativeTools []string

	// Extensions this provider supports
	Extensions []string
}

var providerConfigs = []providerConfig{
	{
		Name:                     "anthropic",
		Model:                    "anthropic/claude-haiku-4-5-20251001",
		KeyName:                  "anthropic",
		RequireKey:               requireAnthropicKey,
		SupportsVision:           true,
		SupportsAudioInput:       false,
		SupportsVideoInput:       false,
		SupportsAudioOutput:      false,
		SupportsTools:            true,
		SupportsToolStreaming:    true,
		SupportsThinking:         true,
		SupportsStructuredOutput: true,
		SupportsStopSequences:    true,
		SupportsTemperature:      true,
		NativeTools:              []string{"web_search", "code_execution", "computer_use", "text_editor"},
		Extensions:               []string{"thinking.budget_tokens"},
	},
	{
		Name:                     "gemini",
		Model:                    "gemini/gemini-3-flash-preview",
		KeyName:                  "gemini",
		RequireKey:               requireGeminiKey,
		SupportsVision:           true,
		SupportsAudioInput:       true,
		SupportsVideoInput:       true, // Unique to Gemini
		SupportsAudioOutput:      false,
		SupportsTools:            true,
		SupportsToolStreaming:    true,
		SupportsThinking:         true,
		SupportsStructuredOutput: false, // Gemini 3 preview models don't reliably respect JSON schema yet
		SupportsStopSequences:    false, // Gemini supports stop sequences but reports STOP for both natural and stop sequence ends
		SupportsTemperature:      true,
		NativeTools:              []string{"google_search", "code_execution", "file_search", "computer_use", "image_generation"},
		Extensions:               []string{"thinking_level", "thinking_budget", "media_resolution", "thought_signatures", "grounding_metadata"},
	},
	{
		Name:                     "oai-resp",
		Model:                    "oai-resp/gpt-5-mini",
		KeyName:                  "openai",
		RequireKey:               requireOpenAIKey,
		SupportsVision:           true,
		SupportsAudioInput:       true,
		SupportsVideoInput:       false,
		SupportsAudioOutput:      true,
		SupportsTools:            true,
		SupportsToolStreaming:    true,
		SupportsThinking:         true,
		SupportsStructuredOutput: true,
		SupportsStopSequences:    false, // Responses API doesn't support stop sequences
		SupportsTemperature:      false, // Ignored for reasoning models
		NativeTools:              []string{"web_search", "code_interpreter", "file_search", "computer_use", "image_generation"},
		Extensions:               []string{"reasoning.effort", "reasoning.summary", "store", "previous_response_id"},
	},
	{
		Name:                     "groq",
		Model:                    "groq/moonshotai/kimi-k2-instruct-0905",
		KeyName:                  "groq",
		RequireKey:               requireGroqKey,
		SupportsVision:           false, // Vision only on specific models
		SupportsAudioInput:       false,
		SupportsVideoInput:       false,
		SupportsAudioOutput:      false,
		SupportsTools:            true,
		SupportsToolStreaming:    true,
		SupportsThinking:         true, // Via GPT-OSS and DeepSeek-R1 models
		SupportsStructuredOutput: false,
		SupportsStopSequences:    true,
		SupportsTemperature:      true,
		NativeTools:              []string{"web_search", "browser_search"},
		Extensions:               []string{"reasoning_effort", "include_reasoning"},
	},
	{
		Name:                     "gemini-oauth",
		Model:                    "gemini-oauth/gemini-3-pro-preview",
		KeyName:                  "gemini-oauth",
		RequireKey:               requireGeminiOAuthProjectID,
		SupportsVision:           true,
		SupportsAudioInput:       true,
		SupportsVideoInput:       true,
		SupportsAudioOutput:      false,
		SupportsTools:            true,
		SupportsToolStreaming:    true,
		SupportsThinking:         true,
		SupportsStructuredOutput: false, // Same as regular Gemini
		SupportsStopSequences:    false, // Same as regular Gemini
		SupportsTemperature:      true,
		NativeTools:              []string{"google_search", "code_execution", "file_search", "computer_use", "image_generation"},
		Extensions:               []string{"thinking_level", "thinking_budget", "media_resolution", "thought_signatures", "grounding_metadata"},
	},
}

func TestMain(m *testing.M) {
	// Load .env file from project root
	loadEnvFile()
	normalizeEnvKeys()

	// Setup
	testClient = vai.NewClient(
		vai.WithTimeout(60*time.Second),
		vai.WithRetries(2),
	)
	testCtx = context.Background()
	fixtures = newFixtureLoader()

	// Run tests
	code := m.Run()

	os.Exit(code)
}

func normalizeEnvKeys() {
	if os.Getenv("GEMINI_API_KEY") == "" {
		if googleKey := os.Getenv("GOOGLE_API_KEY"); googleKey != "" {
			os.Setenv("GEMINI_API_KEY", googleKey)
		}
	}
}

// loadEnvFile loads environment variables from .env file
func loadEnvFile() {
	// Find project root (where .env is located)
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")
	envPath := filepath.Join(projectRoot, ".env")

	file, err := os.Open(envPath)
	if err != nil {
		return // .env file is optional
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Parse KEY=VALUE
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			// Only set if not already set (env vars take precedence)
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	}
}

// fixtureLoader loads test fixtures from disk
type fixtureLoader struct {
	basePath string
}

func newFixtureLoader() fixtureLoader {
	_, filename, _, _ := runtime.Caller(0)
	basePath := filepath.Join(filepath.Dir(filename), "..", "fixtures")
	return fixtureLoader{basePath: basePath}
}

func (f fixtureLoader) Audio(name string) []byte {
	data, err := os.ReadFile(filepath.Join(f.basePath, "audio", name))
	if err != nil {
		// Return minimal WAV header for tests when fixture is missing
		// This allows tests to run even without real fixtures
		return minimalWAVFile()
	}
	return data
}

func (f fixtureLoader) Image(name string) []byte {
	data, err := os.ReadFile(filepath.Join(f.basePath, "images", name))
	if err != nil {
		// Return minimal PNG for tests when fixture is missing
		return minimalPNGFile()
	}
	return data
}

func (f fixtureLoader) Document(name string) []byte {
	data, err := os.ReadFile(filepath.Join(f.basePath, "documents", name))
	if err != nil {
		panic("fixture not found: " + name)
	}
	return data
}

// minimalWAVFile returns a minimal valid WAV file with silence
func minimalWAVFile() []byte {
	// Minimal WAV: RIFF header + fmt chunk + data chunk with 1 second of silence at 8kHz mono
	sampleRate := 8000
	numSamples := sampleRate   // 1 second
	dataSize := numSamples * 2 // 16-bit samples
	fileSize := 36 + dataSize

	wav := make([]byte, 44+dataSize)
	// RIFF header
	copy(wav[0:4], "RIFF")
	wav[4] = byte(fileSize)
	wav[5] = byte(fileSize >> 8)
	wav[6] = byte(fileSize >> 16)
	wav[7] = byte(fileSize >> 24)
	copy(wav[8:12], "WAVE")
	// fmt chunk
	copy(wav[12:16], "fmt ")
	wav[16] = 16 // chunk size
	wav[20] = 1  // PCM format
	wav[22] = 1  // mono
	wav[24] = byte(sampleRate)
	wav[25] = byte(sampleRate >> 8)
	byteRate := sampleRate * 2
	wav[28] = byte(byteRate)
	wav[29] = byte(byteRate >> 8)
	wav[32] = 2  // block align
	wav[34] = 16 // bits per sample
	// data chunk
	copy(wav[36:40], "data")
	wav[40] = byte(dataSize)
	wav[41] = byte(dataSize >> 8)
	wav[42] = byte(dataSize >> 16)
	wav[43] = byte(dataSize >> 24)
	// Audio data is all zeros (silence)

	return wav
}

// minimalPNGFile returns a minimal valid 1x1 white PNG
func minimalPNGFile() []byte {
	// Minimal 1x1 white PNG
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, // PNG signature
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xde,
		0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41, 0x54, // IDAT chunk
		0x08, 0xd7, 0x63, 0xf8, 0xff, 0xff, 0xff, 0x00,
		0x05, 0xfe, 0x02, 0xfe, 0xa3, 0x56, 0x06, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, // IEND chunk
		0xae, 0x42, 0x60, 0x82,
	}
}

// --- Skip helpers ---

func requireAnthropicKey(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
}

func requireCartesiaKey(t *testing.T) {
	if os.Getenv("CARTESIA_API_KEY") == "" {
		t.Skip("CARTESIA_API_KEY not set")
	}
}

func requireOpenAIKey(t *testing.T) {
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY not set")
	}
}

func requireGeminiKey(t *testing.T) {
	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GEMINI_API_KEY not set")
	}
}

func requireGroqKey(t *testing.T) {
	if os.Getenv("GROQ_API_KEY") == "" {
		t.Skip("GROQ_API_KEY not set")
	}
}

func requireGeminiOAuthProjectID(t *testing.T) {
	// Check env var first
	if os.Getenv("GEMINI_OAUTH_PROJECT_ID") != "" {
		return
	}
	// Check credentials file
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("GEMINI_OAUTH_PROJECT_ID not set and cannot find home dir")
	}
	credsPath := filepath.Join(home, ".config", "vango", "gemini-oauth-credentials.json")
	data, err := os.ReadFile(credsPath)
	if err != nil {
		t.Skip("GEMINI_OAUTH_PROJECT_ID not set and no credentials file found")
	}
	var creds struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(data, &creds); err != nil || creds.ProjectID == "" {
		t.Skip("GEMINI_OAUTH_PROJECT_ID not set and credentials file has no project_id")
	}
}

func requireAllVoiceKeys(t *testing.T) {
	requireCartesiaKey(t)
}

// --- Provider iteration helpers ---

func forEachProvider(t *testing.T, fn func(t *testing.T, p providerConfig)) {
	for _, provider := range providerConfigs {
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			provider.RequireKey(t)
			fn(t, provider)
		})
	}
}

func forEachProviderWith(t *testing.T, predicate func(providerConfig) bool, fn func(t *testing.T, p providerConfig)) {
	for _, provider := range providerConfigs {
		if !predicate(provider) {
			continue
		}
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			provider.RequireKey(t)
			fn(t, provider)
		})
	}
}

// --- Test context helpers ---

func testContext(t *testing.T, timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

func defaultTestContext(t *testing.T) context.Context {
	return testContext(t, 30*time.Second)
}
