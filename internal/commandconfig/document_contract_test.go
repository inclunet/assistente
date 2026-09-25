package commandconfig

import (
	"assistente/internal/commandjson"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSharedPersistedJSONCorpus(t *testing.T) {
	raw, err := os.ReadFile("../commandjson/testdata/lexical.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Raw string
		Valid     bool
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			_, canonicalErr := commandjson.Canonicalize([]byte(tc.Raw))
			_, persistedErr := strictObject(tc.Raw)
			if (canonicalErr == nil) != tc.Valid || (persistedErr == nil) != tc.Valid {
				t.Fatalf("corpus divergente: canonical=%v persisted=%v", canonicalErr, persistedErr)
			}
		})
	}
}

// Persistência e ingresso devem rejeitar a mesma classe de documentos antes
// de interpretar seu schema. Não adaptar versões desconhecidas silenciosamente.
func TestDocumentSharedLexicalContract(t *testing.T) {
	for _, raw := range []string{
		`{"version":1,"nested":{"x":1,"\u0078":2}}`,
		`{"version":1,"nested":["\ud800"]}`,
		`{"version":1,"number":1e999}`,
		`{"version":1} {}`,
		`{"nested":` + strings.Repeat("[", 70) + "0" + strings.Repeat("]", 70) + "}",
	} {
		if _, err := commandjson.Canonicalize([]byte(raw)); err == nil {
			t.Fatalf("canonicalizador aceitou %s", raw)
		}
		if _, err := strictObject(raw); err == nil {
			t.Fatalf("persistência aceitou %s", raw)
		}
	}
	for _, raw := range []string{`{"version":1,"code":"KeyN","modifiers":["Control"]}`} {
		if _, err := decodeKeyboard("keyboard.local", raw); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := decodeKeyboard("keyboard.local", `{"version":2,"code":"KeyN","modifiers":[]}`); err == nil {
		t.Fatal("versão futura reinterpretada")
	}
	if _, err := decodeKeyboard("keyboard.local", `{"version":1.0,"code":"KeyN","modifiers":[]}`); err == nil {
		t.Fatal("representação não inteira de versão aceita")
	}
}
