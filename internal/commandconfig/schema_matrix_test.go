package commandconfig

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// Cada caso modifica somente a propriedade sob teste de uma linha válida,
// impedindo que outra constraint mascare a ausência da proteção esperada.
func TestBindingSchemaRejectsEachInvalidProperty(t *testing.T) {
	db := schemaTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	user, layer := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	base := func() map[string]any {
		return map[string]any{"id": uuid.Must(uuid.NewV7()).String(), "user_id": user,
			"workspace_id": nil, "layer_ref_kind": "user", "layer_ref": layer,
			"trigger_type": "hotkey", "trigger_spec": "{}", "command_id": "workspace.read",
			"arguments": "{}", "condition": "{}", "effect": "execute", "enabled": true,
			"source": "user", "resolution_priority": 0, "review_status": "active", "presentation": "{}"}
	}
	if err := db.Table("command_bindings").Create(base()).Error; err != nil {
		t.Fatal("baseline inválida", err)
	}
	cases := map[string]map[string]any{
		"id nulo":                 {"id": nil},
		"uuid4":                   {"id": uuid.NewString()},
		"workspace vazio":         {"workspace_id": ""},
		"documento array":         {"trigger_spec": "[]"},
		"documento inválido":      {"arguments": "{"},
		"documento null":          {"condition": "null"},
		"apresentação array":      {"presentation": "[]"},
		"comando vazio":           {"command_id": " "},
		"source vazio":            {"source": " "},
		"efeito desconhecido":     {"effect": "other"},
		"review desconhecido":     {"review_status": "other"},
		"boolean inválido":        {"enabled": 2},
		"builtin sem delta":       {"layer_ref_kind": "builtin", "layer_ref": "builtin.layer"},
		"replacement parcial":     {"replaces_default_id": "builtin.binding"},
		"replacement vazio":       {"replaces_default_id": "builtin.binding", "replaces_default_version": " ", "replaces_default_fingerprint": "v1:fp"},
		"suppress com comando":    {"effect": "suppress", "replaces_default_id": "builtin.binding", "replaces_default_version": "1", "replaces_default_fingerprint": "v1:fp"},
		"suppress sem default":    {"effect": "suppress", "command_id": nil},
		"suppress com argumentos": {"effect": "suppress", "command_id": nil, "arguments": "{\"x\":1}", "replaces_default_id": "builtin.binding", "replaces_default_version": "1", "replaces_default_fingerprint": "v1:fp"},
	}
	for name, changed := range cases {
		t.Run(name, func(t *testing.T) {
			row := base()
			for key, value := range changed {
				row[key] = value
			}
			if err := db.Table("command_bindings").Create(row).Error; err == nil {
				t.Fatal("constraint ausente", name)
			}
		})
	}
}
