package commandconfig

import (
	"context"
	"errors"
	"testing"
)

func TestKeyboardLocalTriggerPortNormalizeCanonicalCtrlKeyK(t *testing.T) {
	port := KeyboardLocalTriggerPort{}

	got, err := port.Normalize(context.Background(), []byte(`{"version":1,"code":"KeyK","modifiers":["Control"]}`))
	if err != nil {
		t.Fatalf("Normalize retornou erro: %v", err)
	}
	if got != "keyboard.local:Control+KeyK" {
		t.Fatalf("identidade = %q, want %q", got, "keyboard.local:Control+KeyK")
	}
}

func TestKeyboardLocalTriggerPortNormalizeV2SequenceAndRoundTrip(t *testing.T) {
	port := KeyboardLocalTriggerPort{}
	raw := []byte(`{"version":2,"steps":[{"code":"KeyN","modifiers":["Shift","Control"]},{"code":"KeyC","modifiers":[]}]}`)
	want := "keyboard.local:Control+Shift+KeyN KeyC"
	identity, err := port.Normalize(context.Background(), raw)
	if err != nil || identity != want {
		t.Fatalf("v2 identity=%q err=%v want=%q", identity, err, want)
	}
	if err := port.ValidateIdentity(context.Background(), identity); err != nil {
		t.Fatalf("identidade v2 rejeitada: %v", err)
	}
	encoded, err := EncodeKeyboardLocalIdentity(identity)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := port.Normalize(context.Background(), encoded)
	if err != nil || roundTrip != identity {
		t.Fatalf("round-trip=%q err=%v want=%q documento=%s", roundTrip, err, identity, encoded)
	}
}

func TestKeyboardLocalTriggerPortRejectsInvalidV2Sequences(t *testing.T) {
	port := KeyboardLocalTriggerPort{}
	for name, raw := range map[string]string{
		"one step":              `{"version":2,"steps":[{"code":"KeyN","modifiers":["Control"]}]}`,
		"three steps":           `{"version":2,"steps":[{"code":"KeyN","modifiers":["Control"]},{"code":"KeyC","modifiers":[]},{"code":"KeyR","modifiers":[]}]}`,
		"first without primary": `{"version":2,"steps":[{"code":"KeyN","modifiers":["Shift"]},{"code":"KeyC","modifiers":[]}]}`,
		"second modifiers":      `{"version":2,"steps":[{"code":"KeyN","modifiers":["Control"]},{"code":"KeyC","modifiers":["Shift"]}]}`,
		"final escape reserved": `{"version":2,"steps":[{"code":"KeyN","modifiers":["Control"]},{"code":"Escape","modifiers":[]}]}`,
		"unknown step field":    `{"version":2,"steps":[{"code":"KeyN","modifiers":["Control"],"extra":true},{"code":"KeyC","modifiers":[]}]}`,
		"noncanonical identity": `{"version":2,"steps":[{"code":"KeyN","modifiers":["Control","Control"]},{"code":"KeyC","modifiers":[]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := port.Normalize(context.Background(), []byte(raw)); !errors.Is(err, ErrInvalid) {
				t.Fatalf("erro=%v want ErrInvalid", err)
			}
		})
	}
}

func TestKeyboardLocalTriggerPortNormalizeRejeitaDocumentosNaoCanonicos(t *testing.T) {
	port := KeyboardLocalTriggerPort{}
	cases := []struct {
		name string
		raw  string
	}{
		{name: "missing version", raw: `{"code":"KeyK","modifiers":["Control"]}`},
		{name: "unknown field", raw: `{"version":1,"code":"KeyK","modifiers":["Control"],"extra":true}`},
		{name: "repeated modifier", raw: `{"version":1,"code":"KeyK","modifiers":["Control","Control"]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := port.Normalize(context.Background(), []byte(tc.raw)); !errors.Is(err, ErrInvalid) {
				t.Fatalf("erro = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestKeyboardLocalTriggerPortValidateIdentityDefault(t *testing.T) {
	port := KeyboardLocalTriggerPort{}

	if err := port.ValidateIdentity(context.Background(), "keyboard.local:Control+KeyK"); err != nil {
		t.Fatalf("identidade canônica rejeitada: %v", err)
	}
	if err := port.ValidateIdentity(context.Background(), "keyboard.local:KeyK+Control"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ordem não canônica aceita: %v", err)
	}
	if err := port.ValidateIdentity(context.Background(), "keyboard.local:UnknownKey"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("identidade desconhecida aceita: %v", err)
	}
}

func TestKeyboardLocalTriggerPortCancellation(t *testing.T) {
	port := KeyboardLocalTriggerPort{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := port.Normalize(ctx, []byte(`{"version":1,"code":"KeyK","modifiers":[]}`)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Normalize cancelado retornou %v", err)
	}
	if err := port.ValidateIdentity(ctx, "keyboard.local:KeyK"); !errors.Is(err, context.Canceled) {
		t.Fatalf("ValidateIdentity cancelado retornou %v", err)
	}
}

func TestKeyboardLocalTriggerPortSatisfiesProjectionPorts(t *testing.T) {
	var _ TriggerPort = KeyboardLocalTriggerPort{}
	var _ TriggerIdentityValidator = KeyboardLocalTriggerPort{}
}
