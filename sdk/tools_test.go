package vai

import (
	"context"
	"reflect"
	"testing"
)

func TestNewFuncTool(t *testing.T) {
	type WeatherInput struct {
		Location string `json:"location"`
		Units    string `json:"units,omitempty"`
	}

	ft := NewFuncTool("get_weather", "Get weather for a location", func(ctx context.Context, input WeatherInput) (any, error) {
		return map[string]any{
			"temperature": 22,
			"condition":   "sunny",
			"location":    input.Location,
		}, nil
	})

	tool := ft.Tool()
	if tool.Name != "get_weather" {
		t.Errorf("Name = %q, want %q", tool.Name, "get_weather")
	}
	if tool.Type != "function" {
		t.Errorf("Type = %q, want %q", tool.Type, "function")
	}
	if tool.Description != "Get weather for a location" {
		t.Errorf("Description = %q, want %q", tool.Description, "Get weather for a location")
	}
	if tool.InputSchema == nil {
		t.Fatal("InputSchema should not be nil")
	}
	if tool.InputSchema.Type != "object" {
		t.Errorf("InputSchema.Type = %q, want %q", tool.InputSchema.Type, "object")
	}
}

func TestFuncTool_Execute(t *testing.T) {
	type AddInput struct {
		A int `json:"a"`
		B int `json:"b"`
	}

	ft := NewFuncTool("add", "Add two numbers", func(ctx context.Context, input AddInput) (any, error) {
		return input.A + input.B, nil
	})

	result, err := ft.Execute(context.Background(), []byte(`{"a": 5, "b": 3}`))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result != 8 {
		t.Errorf("Result = %v, want 8", result)
	}
}

func TestFuncAsTool(t *testing.T) {
	type Input struct {
		Query string `json:"query"`
	}

	tool, handler := FuncAsTool("search", "Search for something", func(ctx context.Context, input Input) (any, error) {
		return "Results for: " + input.Query, nil
	})

	if tool.Name != "search" {
		t.Errorf("Tool name = %q, want %q", tool.Name, "search")
	}

	result, err := handler(context.Background(), []byte(`{"query": "test"}`))
	if err != nil {
		t.Fatalf("Handler failed: %v", err)
	}

	if result != "Results for: test" {
		t.Errorf("Result = %v, want %q", result, "Results for: test")
	}
}

func TestToolSet(t *testing.T) {
	type SearchInput struct {
		Query string `json:"query"`
	}

	ts := NewToolSet()

	tool, handler := FuncAsTool("search", "Search", func(ctx context.Context, input SearchInput) (any, error) {
		return input.Query, nil
	})
	ts.Add(tool, handler)
	ts.AddNative(WebSearch())

	tools := ts.Tools()
	if len(tools) != 2 {
		t.Errorf("len(Tools()) = %d, want 2", len(tools))
	}

	handlers := ts.Handlers()
	if len(handlers) != 1 {
		t.Errorf("len(Handlers()) = %d, want 1", len(handlers))
	}

	h, ok := ts.Handler("search")
	if !ok {
		t.Error("Handler('search') should exist")
	}
	if h == nil {
		t.Error("Handler should not be nil")
	}
}

func TestMakeTool(t *testing.T) {
	type WeatherInput struct {
		Location string `json:"location"`
		Units    string `json:"units,omitempty"`
	}

	tool := MakeTool("get_weather", "Get weather for a location",
		func(ctx context.Context, input WeatherInput) (string, error) {
			return "Weather in " + input.Location + ": sunny", nil
		},
	)

	// Check tool properties
	if tool.Name != "get_weather" {
		t.Errorf("Name = %q, want %q", tool.Name, "get_weather")
	}
	if tool.Type != "function" {
		t.Errorf("Type = %q, want %q", tool.Type, "function")
	}
	if tool.Description != "Get weather for a location" {
		t.Errorf("Description = %q, want %q", tool.Description, "Get weather for a location")
	}
	if tool.InputSchema == nil {
		t.Fatal("InputSchema should not be nil")
	}
	if tool.Handler == nil {
		t.Fatal("Handler should not be nil")
	}

	// Test handler execution
	result, err := tool.Handler(context.Background(), []byte(`{"location": "Tokyo"}`))
	if err != nil {
		t.Fatalf("Handler failed: %v", err)
	}
	if result != "Weather in Tokyo: sunny" {
		t.Errorf("Result = %v, want %q", result, "Weather in Tokyo: sunny")
	}
}

func TestToolWithHandler_AsTools(t *testing.T) {
	type Input struct {
		Query string `json:"query"`
	}

	tool1 := MakeTool("search", "Search for something",
		func(ctx context.Context, input Input) (string, error) {
			return "Results for: " + input.Query, nil
		},
	)

	tool2 := MakeTool("calculate", "Do math",
		func(ctx context.Context, input struct{ A, B int }) (int, error) {
			return input.A + input.B, nil
		},
	)

	// Both should be usable as Tool in a slice
	tools := []Tool{tool1.Tool, tool2.Tool}
	if len(tools) != 2 {
		t.Errorf("len(tools) = %d, want 2", len(tools))
	}
}

func TestGenerateJSONSchema(t *testing.T) {
	type Person struct {
		Name   string   `json:"name" desc:"The person's name"`
		Age    int      `json:"age"`
		Active bool     `json:"active,omitempty"`
		Tags   []string `json:"tags,omitempty"`
		Role   string   `json:"role" enum:"admin,user,guest"`
	}

	var p Person
	schema := GenerateJSONSchema(reflect.TypeOf(p))

	if schema.Type != "object" {
		t.Errorf("Schema type = %q, want %q", schema.Type, "object")
	}

	if _, ok := schema.Properties["name"]; !ok {
		t.Error("Schema should have 'name' property")
	}

	if schema.Properties["name"].Type != "string" {
		t.Errorf("name type = %q, want %q", schema.Properties["name"].Type, "string")
	}

	if schema.Properties["name"].Description != "The person's name" {
		t.Errorf("name description = %q, want %q", schema.Properties["name"].Description, "The person's name")
	}

	if schema.Properties["age"].Type != "integer" {
		t.Errorf("age type = %q, want %q", schema.Properties["age"].Type, "integer")
	}

	if schema.Properties["active"].Type != "boolean" {
		t.Errorf("active type = %q, want %q", schema.Properties["active"].Type, "boolean")
	}

	if schema.Properties["tags"].Type != "array" {
		t.Errorf("tags type = %q, want %q", schema.Properties["tags"].Type, "array")
	}

	// Check required fields
	found := false
	for _, r := range schema.Required {
		if r == "name" {
			found = true
			break
		}
	}
	if !found {
		t.Error("'name' should be required")
	}
}
