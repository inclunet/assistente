package commandcatalog

import "testing"

func TestJSONSchemaDiscoveryDoesNotExposeMutableEnum(t *testing.T) {
	schema := &Schema{Type: SchemaObject, Properties: map[string]Schema{
		"choice": {Type: SchemaObject, Properties: map[string]Schema{"value": {Type: SchemaString}}, Enum: []any{map[string]any{"value": "original"}}},
	}}
	description := JSONSchema(schema)
	choice := description["properties"].(map[string]any)["choice"].(map[string]any)
	choice["enum"].([]any)[0].(map[string]any)["value"] = "modified"
	if got := schema.Properties["choice"].Enum[0].(map[string]any)["value"]; got != "original" {
		t.Fatalf("discovery changed schema enum: %v", got)
	}
}
