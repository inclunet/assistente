package commandconfig

import (
	"errors"
	"strings"
	"testing"

	"assistente/internal/commandbindings"
)

func requireInvalid(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrInvalid) || err.Error() != ErrInvalid.Error() {
		t.Fatalf("erro = %v, esperado ErrInvalid genérico", err)
	}
}

func TestDecodeKeyboardCanonicalizesAndValidates(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"sem modificadores", `{"version":1,"code":"KeyK","modifiers":[]}`, "keyboard.local:KeyK"},
		{"ordem canônica", `{"version":1,"code":"KeyK","modifiers":["Meta","Control","Shift","Alt"]}`, "keyboard.local:Control+Alt+Shift+Meta+KeyK"},
		{"código especial", `{"version":1,"code":"F24","modifiers":["Control"]}`, "keyboard.local:Control+F24"},
		{"escape em chave", `{"\u0076ersion":1,"\u0063ode":"Enter","\u006dodifiers":[]}`, "keyboard.local:Enter"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := decodeKeyboard("keyboard.local", test.raw)
			if err != nil || got != test.want {
				t.Fatalf("resultado = %q, erro = %v, esperado %q", got, err, test.want)
			}
		})
	}

	for _, code := range []string{"KeyA", "KeyZ", "Digit0", "Digit9", "F1", "F9", "F10", "F24", "ArrowUp", "PageDown", "Insert"} {
		t.Run("código "+code, func(t *testing.T) {
			if _, err := decodeKeyboard("keyboard.local", `{"version":1,"code":"`+code+`","modifiers":[]}`); err != nil {
				t.Fatalf("código válido rejeitado: %v", err)
			}
		})
	}
}

func TestDecodeKeyboardRejectsAdversarialDocuments(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		raw      string
	}{
		{"tipo global", "keyboard.global", `{"version":1,"code":"KeyK","modifiers":[]}`},
		{"tipo vazio", "", `{"version":1,"code":"KeyK","modifiers":[]}`},
		{"versão ausente", "keyboard.local", `{"code":"KeyK","modifiers":[]}`},
		{"versão futura", "keyboard.local", `{"version":2,"code":"KeyK","modifiers":[]}`},
		{"versão decimal", "keyboard.local", `{"version":1.0,"code":"KeyK","modifiers":[]}`},
		{"versão nula", "keyboard.local", `{"version":null,"code":"KeyK","modifiers":[]}`},
		{"campo desconhecido", "keyboard.local", `{"version":1,"code":"KeyK","modifiers":[],"Code":"KeyK"}`},
		{"chave repetida", "keyboard.local", `{"version":1,"code":"KeyK","code":"KeyL","modifiers":[]}`},
		{"chave repetida escapada", "keyboard.local", `{"version":1,"code":"KeyK","\u0063ode":"KeyL","modifiers":[]}`},
		{"null", "keyboard.local", `null`},
		{"array", "keyboard.local", `[]`},
		{"JSON posterior", "keyboard.local", `{"version":1,"code":"KeyK","modifiers":[]} {}`},
		{"código minúsculo", "keyboard.local", `{"version":1,"code":"keyK","modifiers":[]}`},
		{"código F0", "keyboard.local", `{"version":1,"code":"F0","modifiers":[]}`},
		{"código F25", "keyboard.local", `{"version":1,"code":"F25","modifiers":[]}`},
		{"código F com espaço", "keyboard.local", `{"version":1,"code":"F 1","modifiers":[]}`},
		{"código F com espaço final", "keyboard.local", `{"version":1,"code":"F1 ","modifiers":[]}`},
		{"código desconhecido", "keyboard.local", `{"version":1,"code":"Key1","modifiers":[]}`},
		{"código nulo", "keyboard.local", `{"version":1,"code":null,"modifiers":[]}`},
		{"modificadores ausentes", "keyboard.local", `{"version":1,"code":"KeyK"}`},
		{"modificadores nulos", "keyboard.local", `{"version":1,"code":"KeyK","modifiers":null}`},
		{"modificador objeto", "keyboard.local", `{"version":1,"code":"KeyK","modifiers":[{}]}`},
		{"modificador desconhecido", "keyboard.local", `{"version":1,"code":"KeyK","modifiers":["Command"]}`},
		{"modificador repetido", "keyboard.local", `{"version":1,"code":"KeyK","modifiers":["Control","Control"]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeKeyboard(test.typeName, test.raw)
			requireInvalid(t, err)
		})
	}

	tooLarge := `{"version":1,"code":"KeyK","modifiers":[]}` + strings.Repeat(" ", maxDocumentBytes)
	_, err := decodeKeyboard("keyboard.local", tooLarge)
	requireInvalid(t, err)
	invalidUTF8 := "{\"version\":1,\"code\":\"KeyK\xFF\",\"modifiers\":[]}"
	_, err = decodeKeyboard("keyboard.local", invalidUTF8)
	requireInvalid(t, err)
	_, err = decodeKeyboard("keyboard.local", `{"version":1,"code":"Key\ud800","modifiers":[]}`)
	requireInvalid(t, err)
}

func TestDecodeConditionDecodesFactsAndRejectsDuplicates(t *testing.T) {
	raw := `{"version":1,"clauses":[{"field":"app.focused","op":"eq","value":true},{"field":"surface.id","op":"eq","value":"editor-1"},{"field":"profile","op":"eq","value":"default"}]}`
	got, err := decodeCondition(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := commandbindings.Facts{
		commandbindings.AppFocused: true,
		commandbindings.SurfaceID:  "editor-1",
		commandbindings.Profile:    "default",
	}
	if got[commandbindings.AppFocused] != want[commandbindings.AppFocused] || got[commandbindings.SurfaceID] != want[commandbindings.SurfaceID] || got[commandbindings.Profile] != want[commandbindings.Profile] || len(got) != len(want) {
		t.Fatalf("facts = %#v, esperado %#v", got, want)
	}

	empty, err := decodeCondition(` { "version": 1, "clauses": [] } `)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("condição vazia = %#v, erro = %v", empty, err)
	}
	escapedKey, err := decodeCondition(`{"version":1,"clauses":[{"field":"app.focused","op":"eq","value":true},{"\u0066ield":"profile","op":"eq","value":"x"}]}`)
	if err != nil || escapedKey[commandbindings.Profile] != "x" {
		t.Fatalf("chave escapada válida rejeitada: facts = %#v, erro = %v", escapedKey, err)
	}
	emoji, err := decodeCondition(`{"version":1,"clauses":[{"field":"profile","op":"eq","value":"\ud83d\ude00"}]}`)
	if err != nil || emoji[commandbindings.Profile] != "😀" {
		t.Fatalf("par surrogate válido rejeitado: facts = %#v, erro = %v", emoji, err)
	}
	literalEscape, err := decodeCondition(`{"version":1,"clauses":[{"field":"profile","op":"eq","value":"\\ud800"}]}`)
	if err != nil || literalEscape[commandbindings.Profile] != `\ud800` || len(literalEscape[commandbindings.Profile].(string)) != 6 {
		t.Fatalf("escape literal não preservado: facts = %#v, erro = %v", literalEscape, err)
	}

	bad := []string{
		`{"version":1,"clauses":[{"field":"app.focused","op":"eq","value":true},{"field":"app.focused","op":"eq","value":false}]}`,
		`{"version":1,"clauses":[{"field":"profile","\u0066ield":"device","op":"eq","value":"x"}]}`,
		`{"version":1,"clauses":[{"field":"app.focused","op":"eq","value":true},{"field":"app.focused","op":"eq","value":true}]}`,
		`{"version":1,"clauses":[{"field":"app.focused","op":"ne","value":true}]}`,
		`{"version":1,"clauses":[{"field":"unknown","op":"eq","value":"x"}]}`,
	}
	for i, document := range bad {
		t.Run("inválido "+string(rune('a'+i)), func(t *testing.T) {
			_, err := decodeCondition(document)
			requireInvalid(t, err)
		})
	}
}

func TestDecodeConditionRejectsAdversarialDocumentsAndLimit(t *testing.T) {
	base := `{"version":1,"clauses":[]}`
	tests := []struct {
		name string
		raw  string
	}{
		{"objeto vazio", `{}`},
		{"versão ausente", `{"clauses":[]}`},
		{"versão futura", `{"version":2,"clauses":[]}`},
		{"versão decimal", `{"version":1.0,"clauses":[]}`},
		{"campo desconhecido", `{"version":1,"clauses":[],"Clauses":[]}`},
		{"chave duplicada", `{"version":1,"clauses":[],"clauses":[]}`},
		{"chave duplicada escapada", `{"version":1,"clauses":[],"\u0063lauses":[]}`},
		{"nulo", `null`},
		{"array", `[]`},
		{"JSON posterior", base + ` {}`},
		{"cláusulas nulas", `{"version":1,"clauses":null}`},
		{"cláusula nula", `{"version":1,"clauses":[null]}`},
		{"cláusula array", `{"version":1,"clauses":[[]]}`},
		{"campo ausente", `{"version":1,"clauses":[{"op":"eq","value":true}]}`},
		{"campo desconhecido", `{"version":1,"clauses":[{"field":"app.active","op":"eq","value":true}]}`},
		{"campo com case variante", `{"version":1,"clauses":[{"field":"App.Focused","op":"eq","value":true}]}`},
		{"op ausente", `{"version":1,"clauses":[{"field":"app.focused","value":true}]}`},
		{"operador inválido", `{"version":1,"clauses":[{"field":"app.focused","op":"neq","value":true}]}`},
		{"valor bool em string", `{"version":1,"clauses":[{"field":"app.focused","op":"eq","value":"true"}]}`},
		{"valor string em bool", `{"version":1,"clauses":[{"field":"profile","op":"eq","value":true}]}`},
		{"valor nulo", `{"version":1,"clauses":[{"field":"profile","op":"eq","value":null}]}`},
		{"string vazia", `{"version":1,"clauses":[{"field":"profile","op":"eq","value":""}]}`},
		{"string só espaços", `{"version":1,"clauses":[{"field":"profile","op":"eq","value":"   "}]}`},
		{"campo repetido na cláusula", `{"version":1,"clauses":[{"field":"profile","field":"device","op":"eq","value":"x"}]}`},
		{"low surrogate isolado", `{"version":1,"clauses":[{"field":"profile","op":"eq","value":"\ude00"}]}`},
		{"high surrogate sem low", `{"version":1,"clauses":[{"field":"profile","op":"eq","value":"\ud800\u0041"}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeCondition(test.raw)
			requireInvalid(t, err)
		})
	}

	clauses := make([]string, 33)
	for i := range clauses {
		clauses[i] = `{"field":"profile","op":"eq","value":"x` + string(rune('a'+i)) + `"}`
	}
	_, err := decodeCondition(`{"version":1,"clauses":[` + strings.Join(clauses, ",") + `]}`)
	requireInvalid(t, err)

	tooLarge := base + strings.Repeat(" ", maxDocumentBytes)
	_, err = decodeCondition(tooLarge)
	requireInvalid(t, err)
	invalidUTF8 := "{\"version\":1,\"clauses\":[{\"field\":\"profile\",\"op\":\"eq\",\"value\":\"x\xFF\"}]}"
	_, err = decodeCondition(invalidUTF8)
	requireInvalid(t, err)
	_, err = decodeCondition(`{"version":1,"clauses":[{"field":"profile","op":"eq","value":"\ud800"}]}`)
	requireInvalid(t, err)
}

func TestEmptyDocumentOnlyAcceptsEmptyObject(t *testing.T) {
	for _, raw := range []string{"{}", " { } ", "\n{\t}\r\n"} {
		if !emptyDocument(raw) {
			t.Errorf("documento vazio válido rejeitado: %q", raw)
		}
	}
	for _, raw := range []string{"", "null", "[]", `{"x":1}`, "{} {}", `{"x":1} lixo`, `{"\u0078":1}`} {
		if emptyDocument(raw) {
			t.Errorf("documento inválido aceito: %q", raw)
		}
	}
	if emptyDocument(strings.Repeat(" ", maxDocumentBytes+1)) {
		t.Fatal("documento acima do limite foi aceito")
	}
	if emptyDocument("{\xFF}") {
		t.Fatal("UTF-8 inválido foi aceito")
	}
}
