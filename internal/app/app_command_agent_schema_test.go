package app

import (
	"encoding/json"
	"reflect"
	"testing"

	"assistente/internal/commandcatalog"
)

func TestCommandAgentJSONSchemaPreservesCatalogConstraints(t *testing.T) {
	minimum, maximum := float64(0), float64(12)
	min, max := 1, 7
	schema := &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{
		"title": {Type: commandcatalog.SchemaString, MinLength: &min, MaxLength: &max},
		"count": {Type: commandcatalog.SchemaInteger, Optional: true, Minimum: &minimum, Maximum: &maximum},
		"items": {Type: commandcatalog.SchemaArray, Nullable: true, MinItems: &min, MaxItems: &max, Items: &commandcatalog.Schema{Type: commandcatalog.SchemaString, Enum: []any{"one", "two"}}},
	}}
	got := commandcatalog.JSONSchema(schema)
	if !reflect.DeepEqual(got["required"], []string{"items", "title"}) || got["additionalProperties"] != false {
		t.Fatalf("required/closed lost: %+v", got)
	}
	properties := got["properties"].(map[string]any)
	count := properties["count"].(map[string]any)
	if count["minimum"] != minimum || count["maximum"] != maximum {
		t.Fatal(count)
	}
	items := properties["items"].(map[string]any)
	if !reflect.DeepEqual(items["type"], []string{"array", "null"}) || items["minItems"] != min || items["maxItems"] != max {
		t.Fatal(items)
	}
	schema.Required = []string{}
	got = commandcatalog.JSONSchema(schema)
	if len(got["required"].([]string)) != 0 {
		t.Fatal("explicit empty required ignored")
	}
	if _, err := json.Marshal(got); err != nil {
		t.Fatal(err)
	}
	if commandcatalog.JSONSchema(nil) != nil {
		t.Fatal("nil schema became nonnil")
	}
}
