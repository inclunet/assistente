package app

import (
	"encoding/json"
	"testing"
)

func marshalJSONArgumentObject(t *testing.T, value map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal de argumentos JSON: %v", err)
	}
	return string(raw)
}

func TestCommandKeyboardWailsArgumentsStayJSONObjects(t *testing.T) {
	view := LocalCommandKeyboardMap{
		Bindings:              []LocalCommandKeyboardBinding{{Shortcut: LocalCommandShortcut{Version: 1, Code: "KeyG", Modifiers: []string{"Control", "Alt"}}, Arguments: map[string]any{"tab_id": "tab-a", "options": []any{map[string]any{"focus": true}}}}},
		LocalPaletteArguments: map[string]map[string]any{"workspace.tab.go_to": {"tab_id": "tab-b"}},
		LocalPaletteConditions: []LocalCommandPaletteCondition{{
			CommandID:          "workspace.tab.go_to",
			FallbackArguments:  map[string]any{"tab_id": "tab-c"},
			BySurfaceArguments: map[string]map[string]any{"chat": {"tab_id": "tab-d"}},
			BySurfaceIDArguments: map[string]map[string]map[string]any{
				"chat": {"surface-a": {"tab_id": "tab-e"}},
			},
		}},
	}

	wire, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal da projeção: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatalf("unmarshal da projeção: %v", err)
	}
	assertObject := func(value any, path string) map[string]any {
		t.Helper()
		object, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("%s deveria ser objeto JSON, recebeu %T (%s)", path, value, wire)
		}
		return object
	}
	assertObject(assertObject(decoded["bindings"].([]any)[0], "bindings[0]")["arguments"], "bindings[0].arguments")
	assertObject(assertObject(decoded["localPaletteArguments"], "localPaletteArguments")["workspace.tab.go_to"], "localPaletteArguments.workspace.tab.go_to")
	condition := assertObject(decoded["localPaletteConditions"].([]any)[0], "localPaletteConditions[0]")
	assertObject(condition["fallbackArguments"], "fallbackArguments")
	assertObject(assertObject(condition["bySurfaceArguments"], "bySurfaceArguments")["chat"], "bySurfaceArguments.chat")
	bySurfaceID := assertObject(assertObject(condition["bySurfaceIdArguments"], "bySurfaceIdArguments")["chat"], "bySurfaceIdArguments.chat")
	assertObject(bySurfaceID["surface-a"], "bySurfaceIdArguments.chat.surface-a")
}

func TestDecodeJSONArgumentObjectRequiresTopLevelObject(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage(``), json.RawMessage(`null`), json.RawMessage(`[]`), json.RawMessage(`"string"`), json.RawMessage(`{broken}`)} {
		if object, ok := decodeJSONArgumentObject(raw); ok || object != nil {
			t.Errorf("decodeJSONArgumentObject(%q) = (%v, %v), want nil,false", raw, object, ok)
		}
	}
	object, ok := decodeJSONArgumentObject(json.RawMessage(`{"count":9007199254740993,"nested":[{"enabled":true}]}`))
	if !ok || object["count"] != json.Number("9007199254740993") {
		t.Fatalf("objeto/número JSON não preservado: %#v, ok=%v", object, ok)
	}
}
