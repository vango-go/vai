package vai

import (
	"reflect"
	"strings"

	"github.com/vango-go/vai/pkg/core/types"
)

// generateSchema generates a JSON schema from a Go value.
// Supports struct tags:
//   - json:"name"        - field name in JSON
//   - desc:"description" - field description
//   - enum:"a,b,c"       - enum values
func generateSchema(v any) *types.JSONSchema {
	if v == nil {
		return &types.JSONSchema{}
	}
	t := reflect.TypeOf(v)
	if t == nil {
		return &types.JSONSchema{}
	}
	return generateSchemaFromType(t)
}

// generateSchemaFromType generates a JSON schema from a Go type.
func generateSchemaFromType(t reflect.Type) *types.JSONSchema {
	// Dereference pointer
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Struct:
		return generateObjectSchema(t)
	case reflect.Slice, reflect.Array:
		items := generateSchemaFromType(t.Elem())
		return &types.JSONSchema{
			Type:  "array",
			Items: items,
		}
	case reflect.String:
		return &types.JSONSchema{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &types.JSONSchema{Type: "integer"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &types.JSONSchema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return &types.JSONSchema{Type: "number"}
	case reflect.Bool:
		return &types.JSONSchema{Type: "boolean"}
	case reflect.Map:
		// For maps, we just return object type
		return &types.JSONSchema{Type: "object"}
	case reflect.Interface:
		// For interface{}, return without type constraint
		return &types.JSONSchema{}
	default:
		return &types.JSONSchema{Type: "string"} // Fallback
	}
}

// generateObjectSchema generates a JSON schema for a struct type.
func generateObjectSchema(t reflect.Type) *types.JSONSchema {
	schema := &types.JSONSchema{
		Type:       "object",
		Properties: make(map[string]types.JSONSchema),
		Required:   []string{},
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get JSON field name
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue // Skip this field
		}

		jsonName := field.Name
		omitempty := false
		if jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				jsonName = parts[0]
			}
			// Check for omitempty
			for _, part := range parts[1:] {
				if part == "omitempty" {
					omitempty = true
					break
				}
			}
		}

		// Generate field schema
		fieldSchema := generateSchemaFromType(field.Type)

		// Apply description from tag
		if desc := field.Tag.Get("desc"); desc != "" {
			fieldSchema.Description = desc
		}

		// Apply enum from tag
		if enum := field.Tag.Get("enum"); enum != "" {
			fieldSchema.Enum = parseEnumTag(enum)
		}

		schema.Properties[jsonName] = *fieldSchema

		// Check if required (not a pointer and no omitempty)
		isRequired := true
		if field.Type.Kind() == reflect.Ptr {
			isRequired = false
		}
		if omitempty {
			isRequired = false
		}
		if isRequired {
			schema.Required = append(schema.Required, jsonName)
		}
	}

	return schema
}

// parseEnumTag parses a comma-separated enum tag value.
func parseEnumTag(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// SchemaFromStruct generates a JSON schema from a struct type.
// This is an exported helper for users who want to generate schemas manually.
func SchemaFromStruct[T any]() *types.JSONSchema {
	var zero T
	return generateSchema(zero)
}
