package commandcatalog

import (
	"encoding/json"
	"testing"
)

func TestSchemaEnumJSONEquality(t *testing.T) {
	if !jsonValuesEqual(map[string]any{"n": 1}, map[string]any{"n": json.Number("1.0")}) {
		t.Fatal("enum usa igualdade Go em vez de JSON")
	}
	if jsonValuesEqual(map[string]any{"n": 1}, map[string]any{"n": 2}) {
		t.Fatal("enum aceitou valor diferente")
	}
	schema := &Schema{Type: SchemaString, Nullable: true, Enum: []any{"allowed"}}
	if validateSchemaValue(nil, schema, "$", false) == nil {
		t.Fatal("nullable contornou enum")
	}
	schema.Enum = append(schema.Enum, nil)
	if err := validateSchemaValue(nil, schema, "$", false); err != nil {
		t.Fatal(err)
	}
}
