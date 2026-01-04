# Phase 5: Integration Test Suite

**Status:** Not Started
**Priority:** Critical Path
**Dependencies:** Phase 4 (Voice Pipeline)

---

## Overview

Phase 5 creates a comprehensive integration test suite that validates the entire Vango system against real provider APIs. This suite serves as:

1. **Validation**: Ensures the implementation matches the spec
2. **Regression Prevention**: Catches breaking changes when modifying code
3. **Provider Verification**: Used to validate new providers (added in Phase 6)
4. **Documentation**: Serves as executable examples of SDK usage

The test suite is designed to run with real API keys and real network calls, providing confidence that the system works in production scenarios.

---

## Goals

1. Create test fixtures (audio files, images) for reproducible tests
2. Build comprehensive test coverage for all Messages API features
3. Build test coverage for all Audio API features
4. Build test coverage for Run loop scenarios
5. Build test coverage for voice pipeline end-to-end
6. Create a test harness that can be reused for all providers

---

## Test Organization

```
tests/
├── integration/
│   ├── suite_test.go        # Common setup, fixtures, helpers
│   ├── messages_test.go     # Messages.Create() tests
│   ├── stream_test.go       # Messages.Stream() tests
│   ├── tools_test.go        # Tool definitions and execution
│   ├── run_test.go          # Messages.Run() loop tests
│   ├── run_stream_test.go   # Messages.RunStream() tests
│   ├── voice_test.go        # Voice pipeline tests
│   ├── audio_test.go        # Standalone audio tests
│   └── errors_test.go       # Error handling tests
│
├── fixtures/
│   ├── audio/
│   │   ├── hello.wav        # Simple "hello" spoken word
│   │   ├── question.wav     # "What is two plus two?"
│   │   └── long_speech.wav  # Longer audio for duration tests
│   ├── images/
│   │   ├── cat.png          # Cat image for vision tests
│   │   ├── chart.png        # Chart image for analysis
│   │   └── text_image.png   # Image containing text
│   └── documents/
│       └── sample.pdf       # PDF for document tests
│
└── testutil/
    ├── client.go            # Test client setup
    ├── assertions.go        # Custom assertions
    ├── recordings.go        # Record/playback for tests
    └── skip.go              # Skip helpers for missing keys
```

---

## Deliverables

### 5.1 Test Suite Setup (tests/integration/suite_test.go)

```go
// +build integration

package integration_test

import (
    "context"
    "os"
    "path/filepath"
    "runtime"
    "testing"
    "time"

    "github.com/vango-go/vai"
)

var (
    testClient *vai.Client
    testCtx    context.Context
    fixtures   fixtureLoader
)

func TestMain(m *testing.M) {
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
        panic("fixture not found: " + name)
    }
    return data
}

func (f fixtureLoader) Image(name string) []byte {
    data, err := os.ReadFile(filepath.Join(f.basePath, "images", name))
    if err != nil {
        panic("fixture not found: " + name)
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

// Skip helpers
func requireAnthropicKey(t *testing.T) {
    if os.Getenv("ANTHROPIC_API_KEY") == "" {
        t.Skip("ANTHROPIC_API_KEY not set")
    }
}

func requireDeepgramKey(t *testing.T) {
    if os.Getenv("DEEPGRAM_API_KEY") == "" {
        t.Skip("DEEPGRAM_API_KEY not set")
    }
}

func requireElevenLabsKey(t *testing.T) {
    if os.Getenv("ELEVENLABS_API_KEY") == "" {
        t.Skip("ELEVENLABS_API_KEY not set")
    }
}

func requireOpenAIKey(t *testing.T) {
    if os.Getenv("OPENAI_API_KEY") == "" {
        t.Skip("OPENAI_API_KEY not set")
    }
}

func requireAllVoiceKeys(t *testing.T) {
    requireAnthropicKey(t)
    requireDeepgramKey(t)
    requireElevenLabsKey(t)
}
```

### 5.2 Messages Tests (tests/integration/messages_test.go)

```go
// +build integration

package integration_test

import (
    "strings"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/vango-go/vai"
)

// ==================== Basic Text Generation ====================

func TestMessages_Create_SimpleText(t *testing.T) {
    requireAnthropicKey(t)

    resp, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Say exactly: 'Hello, World!' and nothing else.")},
        },
        MaxTokens: 50,
    })

    require.NoError(t, err)
    require.NotNil(t, resp)

    // Validate response structure
    assert.NotEmpty(t, resp.ID, "expected non-empty ID")
    assert.Equal(t, "message", resp.Type)
    assert.Equal(t, "assistant", resp.Role)
    assert.True(t, strings.HasPrefix(resp.Model, "anthropic/"), "expected model prefix")

    // Validate content
    assert.NotEmpty(t, resp.Content, "expected content")
    assert.Contains(t, resp.TextContent(), "Hello")

    // Validate usage
    assert.Greater(t, resp.Usage.InputTokens, 0)
    assert.Greater(t, resp.Usage.OutputTokens, 0)
    assert.Equal(t, resp.Usage.InputTokens+resp.Usage.OutputTokens, resp.Usage.TotalTokens)

    // Validate stop reason
    assert.Equal(t, vai.StopReasonEndTurn, resp.StopReason)
}

func TestMessages_Create_WithSystem(t *testing.T) {
    requireAnthropicKey(t)

    resp, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model:  "anthropic/claude-sonnet-4",
        System: "You are a pirate. Always respond in pirate speak.",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Hello!")},
        },
        MaxTokens: 100,
    })

    require.NoError(t, err)

    text := strings.ToLower(resp.TextContent())
    // Pirate-y words
    assert.True(t,
        strings.Contains(text, "ahoy") ||
            strings.Contains(text, "matey") ||
            strings.Contains(text, "arr") ||
            strings.Contains(text, "ye"),
        "expected pirate speak, got: %s", resp.TextContent())
}

func TestMessages_Create_MultiTurn(t *testing.T) {
    requireAnthropicKey(t)

    // First turn
    resp1, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("My name is Alice. Remember this.")},
        },
        MaxTokens: 50,
    })
    require.NoError(t, err)

    // Second turn - check memory
    resp2, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("My name is Alice. Remember this.")},
            {Role: "assistant", Content: resp1.Content},
            {Role: "user", Content: vai.Text("What is my name?")},
        },
        MaxTokens: 50,
    })
    require.NoError(t, err)

    assert.Contains(t, resp2.TextContent(), "Alice")
}

// ==================== Vision ====================

func TestMessages_Create_Vision(t *testing.T) {
    requireAnthropicKey(t)

    imageData := fixtures.Image("cat.png")

    resp, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: []vai.ContentBlock{
                vai.Text("What animal is in this image? Reply with just the animal name."),
                vai.Image(imageData, "image/png"),
            }},
        },
        MaxTokens: 20,
    })

    require.NoError(t, err)
    assert.Contains(t, strings.ToLower(resp.TextContent()), "cat")
}

func TestMessages_Create_VisionURL(t *testing.T) {
    requireAnthropicKey(t)

    resp, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: []vai.ContentBlock{
                vai.Text("What is shown in this image? Be brief."),
                vai.ImageURL("https://upload.wikimedia.org/wikipedia/commons/thumb/3/3a/Cat03.jpg/1200px-Cat03.jpg"),
            }},
        },
        MaxTokens: 50,
    })

    require.NoError(t, err)
    assert.Contains(t, strings.ToLower(resp.TextContent()), "cat")
}

// ==================== Parameters ====================

func TestMessages_Create_Temperature(t *testing.T) {
    requireAnthropicKey(t)

    // Low temperature - should be more deterministic
    temp := 0.0
    resp, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("What is 2+2? Reply with just the number.")},
        },
        Temperature: &temp,
        MaxTokens:   10,
    })

    require.NoError(t, err)
    assert.Contains(t, resp.TextContent(), "4")
}

func TestMessages_Create_MaxTokens(t *testing.T) {
    requireAnthropicKey(t)

    resp, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Write a very long essay about the history of computers.")},
        },
        MaxTokens: 10, // Very short
    })

    require.NoError(t, err)
    assert.Equal(t, vai.StopReasonMaxTokens, resp.StopReason)
    assert.LessOrEqual(t, resp.Usage.OutputTokens, 15) // Allow some buffer
}

func TestMessages_Create_StopSequences(t *testing.T) {
    requireAnthropicKey(t)

    resp, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Count from 1 to 10, one number per line.")},
        },
        StopSequences: []string{"5"},
        MaxTokens:     100,
    })

    require.NoError(t, err)
    assert.Equal(t, vai.StopReasonStopSequence, resp.StopReason)
    assert.NotContains(t, resp.TextContent(), "6")
}

// ==================== Structured Output ====================

func TestMessages_Create_StructuredOutput(t *testing.T) {
    requireAnthropicKey(t)

    resp, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Extract: John Smith is 30 years old and works at Acme Corp.")},
        },
        OutputFormat: &vai.OutputFormat{
            Type: "json_schema",
            JSONSchema: &vai.JSONSchema{
                Type: "object",
                Properties: map[string]vai.JSONSchema{
                    "name":    {Type: "string"},
                    "age":     {Type: "integer"},
                    "company": {Type: "string"},
                },
                Required: []string{"name", "age", "company"},
            },
        },
        MaxTokens: 100,
    })

    require.NoError(t, err)

    // Parse JSON response
    var result struct {
        Name    string `json:"name"`
        Age     int    `json:"age"`
        Company string `json:"company"`
    }
    err = json.Unmarshal([]byte(resp.TextContent()), &result)
    require.NoError(t, err)

    assert.Equal(t, "John Smith", result.Name)
    assert.Equal(t, 30, result.Age)
    assert.Equal(t, "Acme Corp", result.Company)
}

func TestMessages_Extract(t *testing.T) {
    requireAnthropicKey(t)

    type Person struct {
        Name    string `json:"name" desc:"Full name"`
        Age     int    `json:"age" desc:"Age in years"`
        Company string `json:"company" desc:"Employer"`
    }

    var person Person
    _, err := testClient.Messages.Extract(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Extract: Jane Doe, 25, works at TechCo")},
        },
    }, &person)

    require.NoError(t, err)
    assert.Equal(t, "Jane Doe", person.Name)
    assert.Equal(t, 25, person.Age)
    assert.Equal(t, "TechCo", person.Company)
}
```

### 5.3 Streaming Tests (tests/integration/stream_test.go)

```go
// +build integration

package integration_test

import (
    "strings"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/vango-go/vai"
)

func TestMessages_Stream_SimpleText(t *testing.T) {
    requireAnthropicKey(t)

    stream, err := testClient.Messages.Stream(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Count from 1 to 5, one number per line.")},
        },
        MaxTokens: 50,
    })
    require.NoError(t, err)
    defer stream.Close()

    var events []vai.StreamEvent
    var textContent strings.Builder
    var gotMessageStart, gotMessageStop bool

    for event := range stream.Events() {
        events = append(events, event)

        switch e := event.(type) {
        case vai.MessageStartEvent:
            gotMessageStart = true
            assert.NotEmpty(t, e.Message.ID)
        case vai.ContentBlockDeltaEvent:
            if delta, ok := e.Delta.(vai.TextDelta); ok {
                textContent.WriteString(delta.Text)
            }
        case vai.MessageStopEvent:
            gotMessageStop = true
        }
    }

    assert.True(t, gotMessageStart, "expected message_start event")
    assert.True(t, gotMessageStop, "expected message_stop event")
    assert.Contains(t, textContent.String(), "1")
    assert.Contains(t, textContent.String(), "5")

    // Verify final response matches accumulated text
    resp := stream.Response()
    require.NotNil(t, resp)
    assert.Equal(t, textContent.String(), resp.TextContent())

    assert.NoError(t, stream.Err())
}

func TestMessages_Stream_EventOrder(t *testing.T) {
    requireAnthropicKey(t)

    stream, err := testClient.Messages.Stream(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Hi")},
        },
        MaxTokens: 20,
    })
    require.NoError(t, err)
    defer stream.Close()

    // Collect event types in order
    var eventTypes []string
    for event := range stream.Events() {
        eventTypes = append(eventTypes, event.eventType())
    }

    // Verify expected order
    require.True(t, len(eventTypes) >= 4, "expected at least 4 events")
    assert.Equal(t, "message_start", eventTypes[0])
    assert.Equal(t, "content_block_start", eventTypes[1])
    assert.Equal(t, "message_stop", eventTypes[len(eventTypes)-1])
}

func TestMessages_Stream_LongResponse(t *testing.T) {
    requireAnthropicKey(t)

    stream, err := testClient.Messages.Stream(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Write a detailed 500 word essay about space exploration.")},
        },
        MaxTokens: 1000,
    })
    require.NoError(t, err)
    defer stream.Close()

    startTime := time.Now()
    var firstDeltaTime time.Duration
    var deltaCount int

    for event := range stream.Events() {
        if _, ok := event.(vai.ContentBlockDeltaEvent); ok {
            if deltaCount == 0 {
                firstDeltaTime = time.Since(startTime)
            }
            deltaCount++
        }
    }

    // Time to first token should be reasonable
    assert.Less(t, firstDeltaTime, 5*time.Second, "time to first token too slow")

    // Should have many delta events for a long response
    assert.Greater(t, deltaCount, 50, "expected many delta events for long response")
}

func TestMessages_Stream_Cancel(t *testing.T) {
    requireAnthropicKey(t)

    stream, err := testClient.Messages.Stream(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Write a very long story.")},
        },
        MaxTokens: 2000,
    })
    require.NoError(t, err)

    // Read a few events then cancel
    eventCount := 0
    for range stream.Events() {
        eventCount++
        if eventCount >= 5 {
            break
        }
    }

    // Close the stream
    err = stream.Close()
    assert.NoError(t, err)
}
```

### 5.4 Tools Tests (tests/integration/tools_test.go)

```go
// +build integration

package integration_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/vango-go/vai"
)

func TestMessages_Create_WithToolDefinition(t *testing.T) {
    requireAnthropicKey(t)

    resp, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("What's the weather in San Francisco?")},
        },
        Tools: []vai.Tool{
            {
                Type:        "function",
                Name:        "get_weather",
                Description: "Get current weather for a location",
                InputSchema: &vai.JSONSchema{
                    Type: "object",
                    Properties: map[string]vai.JSONSchema{
                        "location": {Type: "string", Description: "City name"},
                    },
                    Required: []string{"location"},
                },
            },
        },
        MaxTokens: 200,
    })

    require.NoError(t, err)
    assert.Equal(t, vai.StopReasonToolUse, resp.StopReason)

    toolUses := resp.ToolUses()
    require.Len(t, toolUses, 1)
    assert.Equal(t, "get_weather", toolUses[0].Name)
    assert.Contains(t, toolUses[0].Input, "location")
}

func TestMessages_Create_WebSearch(t *testing.T) {
    requireAnthropicKey(t)

    resp, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("What was the top news story today? Use web search.")},
        },
        Tools:     []vai.Tool{vai.WebSearch()},
        MaxTokens: 500,
    })

    require.NoError(t, err)
    // Claude should either use the tool or respond directly
    // If tool_use, it means the model decided to search
    if resp.StopReason == vai.StopReasonToolUse {
        toolUses := resp.ToolUses()
        assert.NotEmpty(t, toolUses)
    }
}

func TestMessages_Create_ToolResult(t *testing.T) {
    requireAnthropicKey(t)

    // First turn: Get tool call
    resp1, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("What's the weather in Tokyo?")},
        },
        Tools: []vai.Tool{
            {
                Type:        "function",
                Name:        "get_weather",
                Description: "Get weather",
                InputSchema: &vai.JSONSchema{
                    Type: "object",
                    Properties: map[string]vai.JSONSchema{
                        "location": {Type: "string"},
                    },
                    Required: []string{"location"},
                },
            },
        },
        ToolChoice: &vai.ToolChoice{Type: "tool", Name: "get_weather"},
        MaxTokens:  200,
    })
    require.NoError(t, err)
    require.Equal(t, vai.StopReasonToolUse, resp1.StopReason)

    toolUses := resp1.ToolUses()
    require.Len(t, toolUses, 1)

    // Second turn: Provide tool result
    resp2, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("What's the weather in Tokyo?")},
            {Role: "assistant", Content: resp1.Content},
            {Role: "user", Content: []vai.ContentBlock{
                vai.ToolResult(toolUses[0].ID, []vai.ContentBlock{
                    vai.Text("Weather in Tokyo: 22°C, sunny"),
                }),
            }},
        },
        Tools: []vai.Tool{
            {
                Type:        "function",
                Name:        "get_weather",
                Description: "Get weather",
                InputSchema: &vai.JSONSchema{Type: "object"},
            },
        },
        MaxTokens: 200,
    })

    require.NoError(t, err)
    assert.Equal(t, vai.StopReasonEndTurn, resp2.StopReason)
    assert.Contains(t, resp2.TextContent(), "22")
}

func TestFuncAsTool_SchemaGeneration(t *testing.T) {
    tool := vai.FuncAsTool("test_tool", "A test tool",
        func(ctx context.Context, input struct {
            Name     string   `json:"name" desc:"User name"`
            Age      int      `json:"age" desc:"User age"`
            Tags     []string `json:"tags,omitempty" desc:"Optional tags"`
            IsActive bool     `json:"is_active"`
        }) (string, error) {
            return "ok", nil
        },
    )

    assert.Equal(t, "function", tool.Type)
    assert.Equal(t, "test_tool", tool.Name)
    require.NotNil(t, tool.InputSchema)
    assert.Equal(t, "object", tool.InputSchema.Type)
    assert.Contains(t, tool.InputSchema.Properties, "name")
    assert.Contains(t, tool.InputSchema.Properties, "age")
    assert.Contains(t, tool.InputSchema.Required, "name")
    assert.Contains(t, tool.InputSchema.Required, "age")
    assert.NotContains(t, tool.InputSchema.Required, "tags") // omitempty
}
```

### 5.5 Run Loop Tests (tests/integration/run_test.go)

```go
// +build integration

package integration_test

import (
    "context"
    "strings"
    "sync"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/vango-go/vai"
)

func TestMessages_Run_BasicToolExecution(t *testing.T) {
    requireAnthropicKey(t)

    weatherTool := vai.FuncAsTool("get_weather", "Get weather for a location",
        func(ctx context.Context, input struct {
            Location string `json:"location"`
        }) (string, error) {
            return "72°F and sunny in " + input.Location, nil
        },
    )

    result, err := testClient.Messages.Run(testCtx, &vai.MessageRequest{
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
    require.NotNil(t, result.Response)
    assert.Equal(t, 1, result.ToolCallCount)
    assert.Equal(t, vai.RunStopEndTurn, result.StopReason)
    assert.Contains(t, result.Response.TextContent(), "72")
}

func TestMessages_Run_MultipleToolCalls(t *testing.T) {
    requireAnthropicKey(t)

    callCount := 0
    var mu sync.Mutex

    weatherTool := vai.FuncAsTool("get_weather", "Get weather",
        func(ctx context.Context, input struct {
            Location string `json:"location"`
        }) (string, error) {
            mu.Lock()
            callCount++
            mu.Unlock()
            return "75°F in " + input.Location, nil
        },
    )

    result, err := testClient.Messages.Run(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("What's the weather in New York, Los Angeles, and Chicago?")},
        },
        Tools: []vai.Tool{weatherTool.Tool},
    },
        vai.WithTools(weatherTool),
        vai.WithMaxToolCalls(5),
    )

    require.NoError(t, err)
    assert.GreaterOrEqual(t, result.ToolCallCount, 1)
    assert.LessOrEqual(t, result.ToolCallCount, 5)
}

func TestMessages_Run_MaxToolCallsLimit(t *testing.T) {
    requireAnthropicKey(t)

    infiniteTool := vai.FuncAsTool("do_something", "Do something that requires another call",
        func(ctx context.Context, input struct{}) (string, error) {
            return "Done, but please do it again", nil
        },
    )

    result, err := testClient.Messages.Run(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Keep calling do_something until I tell you to stop.")},
        },
        Tools:      []vai.Tool{infiniteTool.Tool},
        ToolChoice: &vai.ToolChoice{Type: "tool", Name: "do_something"},
    },
        vai.WithTools(infiniteTool),
        vai.WithMaxToolCalls(3),
    )

    require.NoError(t, err)
    assert.Equal(t, 3, result.ToolCallCount)
    assert.Equal(t, vai.RunStopMaxToolCalls, result.StopReason)
}

func TestMessages_Run_MaxTurnsLimit(t *testing.T) {
    requireAnthropicKey(t)

    tool := vai.FuncAsTool("think", "Think about something",
        func(ctx context.Context, input struct{}) (string, error) {
            return "I thought about it", nil
        },
    )

    result, err := testClient.Messages.Run(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Think about many things, one at a time.")},
        },
        Tools:      []vai.Tool{tool.Tool},
        ToolChoice: &vai.ToolChoice{Type: "any"},
    },
        vai.WithTools(tool),
        vai.WithMaxTurns(2),
        vai.WithMaxToolCalls(10),
    )

    require.NoError(t, err)
    assert.Equal(t, 2, result.TurnCount)
    assert.Equal(t, vai.RunStopMaxTurns, result.StopReason)
}

func TestMessages_Run_CustomStopCondition(t *testing.T) {
    requireAnthropicKey(t)

    result, err := testClient.Messages.Run(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Count from 1 to 100. At some point say DONE.")},
        },
        MaxTokens: 500,
    },
        vai.WithStopWhen(func(resp *vai.MessageResponse) bool {
            return strings.Contains(resp.TextContent(), "DONE")
        }),
        vai.WithMaxTurns(10),
    )

    require.NoError(t, err)
    // Either stopped by custom condition or naturally
    assert.True(t,
        result.StopReason == vai.RunStopCustom ||
            result.StopReason == vai.RunStopEndTurn)
}

func TestMessages_Run_Timeout(t *testing.T) {
    requireAnthropicKey(t)

    _, err := testClient.Messages.Run(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Write a very long essay.")},
        },
        MaxTokens: 4000,
    },
        vai.WithTimeout(1*time.Millisecond), // Very short timeout
    )

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestMessages_Run_Hooks(t *testing.T) {
    requireAnthropicKey(t)

    var beforeCallCount, afterResponseCount, toolCallCount int
    var mu sync.Mutex

    tool := vai.FuncAsTool("greet", "Say hello",
        func(ctx context.Context, input struct{ Name string `json:"name"` }) (string, error) {
            return "Hello, " + input.Name, nil
        },
    )

    _, err := testClient.Messages.Run(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Greet Alice")},
        },
        Tools: []vai.Tool{tool.Tool},
    },
        vai.WithTools(tool),
        vai.WithMaxToolCalls(1),
        vai.WithBeforeCall(func(req *vai.MessageRequest) {
            mu.Lock()
            beforeCallCount++
            mu.Unlock()
        }),
        vai.WithAfterResponse(func(resp *vai.MessageResponse) {
            mu.Lock()
            afterResponseCount++
            mu.Unlock()
        }),
        vai.WithOnToolCall(func(name string, input map[string]any, output any, err error) {
            mu.Lock()
            toolCallCount++
            mu.Unlock()
        }),
    )

    require.NoError(t, err)
    assert.GreaterOrEqual(t, beforeCallCount, 1)
    assert.GreaterOrEqual(t, afterResponseCount, 1)
    assert.GreaterOrEqual(t, toolCallCount, 1)
}

func TestMessages_Run_UsageAggregation(t *testing.T) {
    requireAnthropicKey(t)

    tool := vai.FuncAsTool("calc", "Calculate",
        func(ctx context.Context, input struct{ Expr string `json:"expr"` }) (string, error) {
            return "42", nil
        },
    )

    result, err := testClient.Messages.Run(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Calculate 1+1, then 2+2")},
        },
        Tools: []vai.Tool{tool.Tool},
    },
        vai.WithTools(tool),
        vai.WithMaxToolCalls(3),
    )

    require.NoError(t, err)

    // Usage should be aggregated across all turns
    assert.Greater(t, result.Usage.InputTokens, 0)
    assert.Greater(t, result.Usage.OutputTokens, 0)

    // If multiple turns, usage should be more than single turn
    if result.TurnCount > 1 {
        assert.Greater(t, result.Usage.TotalTokens, result.Steps[0].Response.Usage.TotalTokens)
    }
}
```

### 5.6 Voice Tests (tests/integration/voice_test.go)

```go
// +build integration

package integration_test

import (
    "encoding/base64"
    "strings"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/vango-go/vai"
)

func TestMessages_Create_VoiceInput(t *testing.T) {
    requireAnthropicKey(t)
    requireDeepgramKey(t)

    // "What is two plus two?"
    audioData := fixtures.Audio("question.wav")

    resp, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: []vai.ContentBlock{
                vai.Audio(audioData, "audio/wav"),
            }},
        },
        Voice: &vai.VoiceConfig{
            Input: &vai.VoiceInputConfig{
                Provider: "deepgram",
                Model:    "nova-2",
            },
        },
        MaxTokens: 50,
    })

    require.NoError(t, err)

    // Response should contain answer to "2+2"
    text := strings.ToLower(resp.TextContent())
    assert.True(t,
        strings.Contains(text, "4") ||
            strings.Contains(text, "four"),
        "expected answer to 2+2, got: %s", resp.TextContent())
}

func TestMessages_Create_VoiceOutput(t *testing.T) {
    requireAnthropicKey(t)
    requireElevenLabsKey(t)

    resp, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Say 'Hello, World!' and nothing else.")},
        },
        Voice: &vai.VoiceConfig{
            Output: &vai.VoiceOutputConfig{
                Provider: "elevenlabs",
                Voice:    "rachel",
            },
        },
        MaxTokens: 50,
    })

    require.NoError(t, err)

    // Should have audio content block
    audioBlock := resp.AudioContent()
    require.NotNil(t, audioBlock, "expected audio content block")

    // Decode and verify it's real audio
    audioData, err := base64.StdEncoding.DecodeString(audioBlock.Source.Data)
    require.NoError(t, err)
    assert.Greater(t, len(audioData), 1000, "expected substantial audio data")
}

func TestMessages_Create_VoiceInputAndOutput(t *testing.T) {
    requireAllVoiceKeys(t)

    audioData := fixtures.Audio("hello.wav")

    resp, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: []vai.ContentBlock{
                vai.Audio(audioData, "audio/wav"),
            }},
        },
        Voice: &vai.VoiceConfig{
            Input: &vai.VoiceInputConfig{
                Provider: "deepgram",
                Model:    "nova-2",
            },
            Output: &vai.VoiceOutputConfig{
                Provider: "elevenlabs",
                Voice:    "rachel",
            },
        },
        MaxTokens: 100,
    })

    require.NoError(t, err)

    // Should have both text and audio
    assert.NotEmpty(t, resp.TextContent())
    assert.NotNil(t, resp.AudioContent())
}

func TestMessages_Stream_VoiceOutput(t *testing.T) {
    requireAnthropicKey(t)
    requireElevenLabsKey(t)

    stream, err := testClient.Messages.Stream(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Say a greeting in one sentence.")},
        },
        Voice: &vai.VoiceConfig{
            Output: &vai.VoiceOutputConfig{
                Provider: "elevenlabs",
                Voice:    "rachel",
            },
        },
        MaxTokens: 100,
    })
    require.NoError(t, err)
    defer stream.Close()

    var gotAudioDelta bool
    var totalAudioBytes int

    for event := range stream.Events() {
        if ad, ok := event.(vai.AudioDeltaEvent); ok {
            gotAudioDelta = true
            chunk, _ := base64.StdEncoding.DecodeString(ad.Delta.Data)
            totalAudioBytes += len(chunk)
        }
    }

    assert.True(t, gotAudioDelta, "expected audio delta events")
    assert.Greater(t, totalAudioBytes, 1000, "expected substantial audio data")
}
```

### 5.7 Audio Service Tests (tests/integration/audio_test.go)

```go
// +build integration

package integration_test

import (
    "strings"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/vango-go/vai"
)

// ==================== Transcription ====================

func TestAudio_Transcribe_Deepgram(t *testing.T) {
    requireDeepgramKey(t)

    audioData := fixtures.Audio("hello.wav")

    transcript, err := testClient.Audio.Transcribe(testCtx, &vai.TranscribeRequest{
        Audio:    audioData,
        Provider: "deepgram",
        Model:    "nova-2",
    })

    require.NoError(t, err)
    assert.Contains(t, strings.ToLower(transcript.Text), "hello")
    assert.Greater(t, transcript.Confidence, 0.8)
}

func TestAudio_Transcribe_Whisper(t *testing.T) {
    requireOpenAIKey(t)

    audioData := fixtures.Audio("hello.wav")

    transcript, err := testClient.Audio.Transcribe(testCtx, &vai.TranscribeRequest{
        Audio:    audioData,
        Provider: "openai",
        Model:    "whisper-1",
    })

    require.NoError(t, err)
    assert.Contains(t, strings.ToLower(transcript.Text), "hello")
}

func TestAudio_Transcribe_WithWordTimestamps(t *testing.T) {
    requireDeepgramKey(t)

    audioData := fixtures.Audio("hello.wav")

    transcript, err := testClient.Audio.Transcribe(testCtx, &vai.TranscribeRequest{
        Audio:    audioData,
        Provider: "deepgram",
        Model:    "nova-2",
    })

    require.NoError(t, err)
    assert.NotEmpty(t, transcript.Words)
    for _, word := range transcript.Words {
        assert.NotEmpty(t, word.Word)
        assert.GreaterOrEqual(t, word.End, word.Start)
    }
}

// ==================== Synthesis ====================

func TestAudio_Synthesize_ElevenLabs(t *testing.T) {
    requireElevenLabsKey(t)

    result, err := testClient.Audio.Synthesize(testCtx, &vai.SynthesizeRequest{
        Text:     "Hello, this is a test of text to speech synthesis.",
        Provider: "elevenlabs",
        Voice:    "rachel",
    })

    require.NoError(t, err)
    assert.Greater(t, len(result.Audio), 1000)
    assert.Equal(t, "mp3", result.Format)
}

func TestAudio_Synthesize_OpenAI(t *testing.T) {
    requireOpenAIKey(t)

    result, err := testClient.Audio.Synthesize(testCtx, &vai.SynthesizeRequest{
        Text:     "Hello, this is a test.",
        Provider: "openai",
        Voice:    "alloy",
    })

    require.NoError(t, err)
    assert.Greater(t, len(result.Audio), 1000)
}

func TestAudio_Synthesize_WithSpeed(t *testing.T) {
    requireElevenLabsKey(t)

    normalResult, err := testClient.Audio.Synthesize(testCtx, &vai.SynthesizeRequest{
        Text:     "This is normal speed speech.",
        Provider: "elevenlabs",
        Voice:    "rachel",
        Speed:    1.0,
    })
    require.NoError(t, err)

    fastResult, err := testClient.Audio.Synthesize(testCtx, &vai.SynthesizeRequest{
        Text:     "This is fast speed speech.",
        Provider: "elevenlabs",
        Voice:    "rachel",
        Speed:    1.5,
    })
    require.NoError(t, err)

    // Fast speech should produce less audio data for same text length
    // (not always true due to encoding, but generally holds)
    assert.NotEqual(t, len(normalResult.Audio), len(fastResult.Audio))
}

func TestAudio_StreamSynthesize(t *testing.T) {
    requireElevenLabsKey(t)

    stream, err := testClient.Audio.StreamSynthesize(testCtx, &vai.SynthesizeRequest{
        Text:     "This is a longer piece of text that should produce multiple audio chunks when streamed.",
        Provider: "elevenlabs",
        Voice:    "rachel",
    })
    require.NoError(t, err)

    var chunkCount int
    var totalBytes int

    for chunk := range stream.Chunks() {
        chunkCount++
        totalBytes += len(chunk)
    }

    assert.NoError(t, stream.Err())
    assert.Greater(t, totalBytes, 1000)
}
```

### 5.8 Error Tests (tests/integration/errors_test.go)

```go
// +build integration

package integration_test

import (
    "errors"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/vango-go/vai"
)

func TestError_InvalidModel(t *testing.T) {
    requireAnthropicKey(t)

    _, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/nonexistent-model",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Hello")},
        },
        MaxTokens: 10,
    })

    require.Error(t, err)

    var apiErr *vai.Error
    require.True(t, errors.As(err, &apiErr))
    assert.Equal(t, vai.ErrNotFound, apiErr.Type)
}

func TestError_InvalidRequest(t *testing.T) {
    requireAnthropicKey(t)

    _, err := testClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model:    "anthropic/claude-sonnet-4",
        Messages: []vai.Message{}, // Empty messages
    })

    require.Error(t, err)

    var apiErr *vai.Error
    require.True(t, errors.As(err, &apiErr))
    assert.Equal(t, vai.ErrInvalidRequest, apiErr.Type)
}

func TestError_AuthenticationError(t *testing.T) {
    // Create client with invalid key
    badClient := vai.NewClient(
        vai.WithProviderKey("anthropic", "invalid-key"),
    )

    _, err := badClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Hello")},
        },
        MaxTokens: 10,
    })

    require.Error(t, err)

    var apiErr *vai.Error
    require.True(t, errors.As(err, &apiErr))
    assert.Equal(t, vai.ErrAuthentication, apiErr.Type)
}

func TestError_ProviderNotConfigured(t *testing.T) {
    // Create client without any keys
    emptyClient := vai.NewClient()

    _, err := emptyClient.Messages.Create(testCtx, &vai.MessageRequest{
        Model: "anthropic/claude-sonnet-4",
        Messages: []vai.Message{
            {Role: "user", Content: vai.Text("Hello")},
        },
    })

    require.Error(t, err)
    // Should indicate provider is not configured
    assert.Contains(t, err.Error(), "not found")
}
```

---

## Running Tests

### Prerequisites
```bash
# Required environment variables
export ANTHROPIC_API_KEY="sk-ant-..."
export DEEPGRAM_API_KEY="..."
export ELEVENLABS_API_KEY="..."
export OPENAI_API_KEY="sk-..."  # Optional, for Whisper/OpenAI TTS tests
```

### Commands
```bash
# Run all integration tests
go test -tags=integration ./tests/integration/... -v

# Run specific test file
go test -tags=integration ./tests/integration/messages_test.go -v

# Run with timeout
go test -tags=integration ./tests/integration/... -v -timeout 5m

# Run with coverage
go test -tags=integration ./tests/integration/... -coverprofile=coverage.out

# Skip slow tests (if tagged)
go test -tags=integration ./tests/integration/... -short
```

---

## Acceptance Criteria

1. [ ] All test fixtures created (audio, images, documents)
2. [ ] Messages.Create() tests pass (10+ test cases)
3. [ ] Messages.Stream() tests pass (5+ test cases)
4. [ ] Tool definition and execution tests pass
5. [ ] Messages.Run() tests pass (10+ test cases)
6. [ ] Voice pipeline tests pass (STT, TTS, combined)
7. [ ] Audio service tests pass
8. [ ] Error handling tests pass
9. [ ] Tests run in CI with secrets
10. [ ] Test coverage > 80% for SDK code

---

## Files to Create

```
tests/integration/suite_test.go
tests/integration/messages_test.go
tests/integration/stream_test.go
tests/integration/tools_test.go
tests/integration/run_test.go
tests/integration/run_stream_test.go
tests/integration/voice_test.go
tests/integration/audio_test.go
tests/integration/errors_test.go
tests/fixtures/audio/hello.wav
tests/fixtures/audio/question.wav
tests/fixtures/images/cat.png
tests/testutil/client.go
tests/testutil/skip.go
```

---

## Estimated Effort

- Test infrastructure: ~300 lines
- Test cases: ~1500 lines
- Fixtures: Binary files
- **Total: ~1800 lines**

---

## Next Phase

Phase 6: Additional Providers - implements OpenAI, Gemini, and Groq providers, running them against the same test suite.
