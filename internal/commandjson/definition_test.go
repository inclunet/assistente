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

func TestCanonicalizationEnforcesExpandedOutputLimit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limit  int
		encode func([]byte) ([]byte, error)
	}{
		{"definition", maxDefinitionSize, func(raw []byte) ([]byte, error) { return MarshalDefinition(json.RawMessage(raw)) }},
		{"protocol-marshal", maxDocumentSize, func(raw []byte) ([]byte, error) { return Marshal(json.RawMessage(raw)) }},
		{"protocol-canonicalize", maxDocumentSize, Canonicalize},
		{"protocol-hmac", maxDocumentSize, func(raw []byte) ([]byte, error) {
			value, err := HMAC(bytes.Repeat([]byte{1}, 32), "test", raw)
			if err != nil {
				return nil, err
			}
			return []byte(value), nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Cada 1e20 ocupa 4 bytes na entrada e 21 na representação JCS.
			count := tc.limit / 22
			raw := []byte("[" + strings.Repeat("1e20,", count-1) + "1e20]")
			if len(raw) >= tc.limit {
				t.Fatal("fixture precisa caber no limite de entrada")
			}
			if _, err := tc.encode(raw); err != nil {
				t.Fatalf("expansão dentro do limite rejeitada: %v", err)
			}
			raw = append(raw[:len(raw)-1], []byte(",1e20]")...)
			if len(raw) >= tc.limit {
				t.Fatal("fixture expandida precisa caber no limite de entrada")
			}
			if got, err := tc.encode(raw); !errors.Is(err, ErrDocumentTooLarge) || got != nil {
				t.Fatalf("saída acima do limite aceita: bytes=%d err=%v", len(got), err)
			}
		})
	}
}
