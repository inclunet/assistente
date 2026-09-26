package commandjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestMarshalDefinitionPreservesCanonicalBytesAndProtocolLimit(t *testing.T) {
	small := map[string]any{"z": "<", "a": 1}
	want, err := Marshal(small)
	if err != nil {
		t.Fatal(err)
	}
	got, err := MarshalDefinition(small)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("identidade JCS alterada: %s %v", got, err)
	}
	large := strings.Repeat("x", maxDocumentSize)
	if _, err := Marshal(large); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("Marshal: %v", err)
	}
	canonical, err := MarshalDefinition(large)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Canonicalize(canonical); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("Canonicalize: %v", err)
	}
	if _, err := HMAC(bytes.Repeat([]byte{1}, 32), "test", canonical); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("HMAC: %v", err)
	}
}

func TestMarshalDefinitionBoundaryAndStrictValidation(t *testing.T) {
	if _, err := MarshalDefinition(strings.Repeat("x", maxDefinitionSize-2)); err != nil {
		t.Fatalf("limite exato: %v", err)
	}
	if _, err := MarshalDefinition(strings.Repeat("x", maxDefinitionSize-1)); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("acima do limite: %v", err)
	}
	for _, raw := range []string{
		`{"x":1,"x":2}`,
		`{"x":"\ud800"}`,
		strings.Repeat("[", maxDepth+1) + "0" + strings.Repeat("]", maxDepth+1),
	} {
		if _, err := MarshalDefinition(json.RawMessage(raw)); err == nil {
			t.Fatalf("JSON inválido aceito")
		}
	}
}
