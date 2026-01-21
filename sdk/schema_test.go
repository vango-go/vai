package vai

import (
	"testing"

	"github.com/vango-go/vai/pkg/core/types"
)

func TestGenerateSchema_BasicTypes(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"string", "", "string"},
		{"int", 0, "integer"},
		{"int64", int64(0), "integer"},
		{"float64", 0.0, "number"},
		{"bool", false, "boolean"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := generateSchema(tt.input)
			if schema.Type != tt.expected {
				t.Errorf("generateSchema() type = %q, want %q", schema.Type, tt.expected)
			}
		})
	}
}

func TestGenerateSchema_Slice(t *testing.T) {
	input := []string{}
	schema := generateSchema(input)

	if schema.Type != "array" {
		t.Errorf("Type = %q, want %q", schema.Type, "array")
	}
	if schema.Items == nil {
		t.Fatal("Items should not be nil")
	}
	if schema.Items.Type != "string" {
		t.Errorf("Items.Type = %q, want %q", schema.Items.Type, "string")
	}
}

func TestGenerateSchema_Struct(t *testing.T) {
	type TestStruct struct {
		Name    string   `json:"name" desc:"The name"`
		Age     int      `json:"age"`
		Active  bool     `json:"active,omitempty"`
		Score   float64  `json:"score"`
		Tags    []string `json:"tags,omitempty"`
		Role    string   `json:"role" enum:"admin,user,guest"`
		private string   // unexported, should be skipped
	}

	input := TestStruct{}
	schema := generateSchema(input)

	if schema.Type != "object" {
		t.Errorf("Type = %q, want %q", schema.Type, "object")
	}

	// Check properties
	props := schema.Properties
	if len(props) != 6 {
		t.Errorf("len(Properties) = %d, want 6", len(props))
	}

	// Check name property
	if props["name"].Type != "string" {
		t.Errorf("name.Type = %q, want %q", props["name"].Type, "string")
	}
	if props["name"].Description != "The name" {
		t.Errorf("name.Description = %q, want %q", props["name"].Description, "The name")
	}

	// Check age property
	if props["age"].Type != "integer" {
		t.Errorf("age.Type = %q, want %q", props["age"].Type, "integer")
	}

	// Check active property (boolean)
	if props["active"].Type != "boolean" {
		t.Errorf("active.Type = %q, want %q", props["active"].Type, "boolean")
	}

	// Check score property (float)
	if props["score"].Type != "number" {
		t.Errorf("score.Type = %q, want %q", props["score"].Type, "number")
	}

	// Check tags property (array)
	if props["tags"].Type != "array" {
		t.Errorf("tags.Type = %q, want %q", props["tags"].Type, "array")
	}

	// Check role property (with enum)
	if len(props["role"].Enum) != 3 {
		t.Errorf("len(role.Enum) = %d, want 3", len(props["role"].Enum))
	}

	// Check required fields
	required := make(map[string]bool)
	for _, r := range schema.Required {
		required[r] = true
	}

	if !required["name"] {
		t.Error("'name' should be required")
	}
	if !required["age"] {
		t.Error("'age' should be required")
	}
	if required["active"] {
		t.Error("'active' should NOT be required (has omitempty)")
	}
	if required["tags"] {
		t.Error("'tags' should NOT be required (has omitempty)")
	}
}

func TestGenerateSchema_Pointer(t *testing.T) {
	type Inner struct {
		Value string `json:"value"`
	}

	type Outer struct {
		Inner *Inner `json:"inner,omitempty"`
	}

	input := Outer{}
	schema := generateSchema(input)

	if _, ok := schema.Properties["inner"]; !ok {
		t.Error("'inner' property should exist")
	}

	// Check that pointer field is not required
	for _, r := range schema.Required {
		if r == "inner" {
			t.Error("'inner' should NOT be required (pointer type)")
		}
	}
}

func TestGenerateSchema_NestedStruct(t *testing.T) {
	type Address struct {
		City    string `json:"city"`
		Country string `json:"country"`
	}

	type Person struct {
		Name    string  `json:"name"`
		Address Address `json:"address"`
	}

	input := Person{}
	schema := generateSchema(input)

	// Check nested schema
	if schema.Properties["address"].Type != "object" {
		t.Errorf("address.Type = %q, want %q", schema.Properties["address"].Type, "object")
	}

	if schema.Properties["address"].Properties == nil {
		t.Fatal("address.Properties should not be nil")
	}

	if _, ok := schema.Properties["address"].Properties["city"]; !ok {
		t.Error("address should have 'city' property")
	}
}

func TestGenerateSchema_Map(t *testing.T) {
	input := map[string]int{}
	schema := generateSchema(input)

	if schema.Type != "object" {
		t.Errorf("Type = %q, want %q", schema.Type, "object")
	}
}

func TestParseEnumTag(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{"admin,user", []string{"admin", "user"}},
		{"single", []string{"single"}},
		{"", nil},
		{"a, b, c", []string{"a", "b", "c"}}, // with spaces
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseEnumTag(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("len(result) = %d, want %d", len(result), len(tt.expected))
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("result[%d] = %q, want %q", i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestSchemaFromStruct(t *testing.T) {
	type Input struct {
		Query string `json:"query"`
		Limit int    `json:"limit,omitempty"`
	}

	schema := SchemaFromStruct[Input]()

	if schema.Type != "object" {
		t.Errorf("Type = %q, want %q", schema.Type, "object")
	}

	if _, ok := schema.Properties["query"]; !ok {
		t.Error("schema should have 'query' property")
	}

	if _, ok := schema.Properties["limit"]; !ok {
		t.Error("schema should have 'limit' property")
	}
}

func TestGenerateSchemaFromType_Pointer(t *testing.T) {
	type Test struct {
		Value string `json:"value"`
	}

	// Test with pointer type
	var ptr *Test
	schema := generateSchema(ptr)

	if schema.Type != "object" {
		t.Errorf("Type = %q, want %q", schema.Type, "object")
	}
}

func TestGenerateSchemaFromType_NilInterface(t *testing.T) {
	var v interface{} = nil
	schema := generateSchema(v)

	// Nil interface results in empty schema
	if schema == nil {
		t.Error("schema should not be nil")
	}
	// Empty type is expected for nil interface
	if schema.Type != "" {
		t.Errorf("Type = %q, want empty string", schema.Type)
	}
}

func TestGenerateSchema_JSONTagSkip(t *testing.T) {
	type Test struct {
		Include string `json:"include"`
		Skip    string `json:"-"`
	}

	input := Test{}
	schema := generateSchema(input)

	if _, ok := schema.Properties["include"]; !ok {
		t.Error("'include' property should exist")
	}

	if _, ok := schema.Properties["Skip"]; ok {
		t.Error("'Skip' property should NOT exist (has json:\"-\")")
	}
}

// Test that the generated schema matches types.JSONSchema structure
func TestGenerateSchema_TypeCompatibility(t *testing.T) {
	type Input struct {
		Name string `json:"name"`
	}

	schema := generateSchema(Input{})

	// Should be able to use as types.JSONSchema
	var _ *types.JSONSchema = schema

	// Verify it can be used in OutputFormat
	_ = types.OutputFormat{
		Type:       "json_schema",
		JSONSchema: schema,
	}
}
