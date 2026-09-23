package commandconfig

import (
	"context"
	"errors"
	"testing"
)

func TestKeyboardGlobalTriggerPortNormalizeAndValidateIdentity(t *testing.T) {
	port := KeyboardGlobalTriggerPort{}

	identity, err := port.Normalize(context.Background(), []byte(`{"version":1,"code":"KeyK","modifiers":["Meta","Control"]}`))
	if err != nil || identity != "keyboard.global:Control+Meta+KeyK" {
		t.Fatalf("Normalize = %q, %v", identity, err)
	}
	if err := port.ValidateIdentity(context.Background(), identity); err != nil {
		t.Fatalf("identidade canônica rejeitada: %v", err)
	}
}

func TestKeyboardGlobalTriggerPortAcceptsOnlySimpleChordV1(t *testing.T) {
	port := KeyboardGlobalTriggerPort{}
	for name, raw := range map[string]string{
		"sequence v2":       `{"version":2,"steps":[{"code":"KeyK","modifiers":["Control"]},{"code":"KeyC","modifiers":[]}]}`,
		"sequence no v1":    `{"version":1,"steps":[{"code":"KeyK","modifiers":["Control"]},{"code":"KeyC","modifiers":[]}]}`,
		"campo extra":       `{"version":1,"code":"KeyK","modifiers":[],"repeat":false}`,
		"modificador duplo": `{"version":1,"code":"KeyK","modifiers":["Control","Control"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := port.Normalize(context.Background(), []byte(raw)); !errors.Is(err, ErrInvalid) {
				t.Fatalf("documento aceito: %v", err)
			}
		})
	}

	for _, identity := range []string{
		"keyboard.local:Control+KeyK",
		"keyboard.global:Control+Shift+KeyK KeyC",
		"keyboard.global:Shift+Control+KeyK",
		"keyboard.global:Control+Control+KeyK",
		"keyboard.global:UnknownKey",
	} {
		if err := port.ValidateIdentity(context.Background(), identity); !errors.Is(err, ErrInvalid) {
			t.Fatalf("identidade aceita %q: %v", identity, err)
		}
	}
}

func TestKeyboardLocalGrammarDoesNotAcceptGlobal(t *testing.T) {
	local := KeyboardLocalTriggerPort{}
	if _, err := local.Normalize(context.Background(), []byte(`{"version":1,"code":"KeyK","modifiers":["Control"]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeKeyboard("keyboard.global", `{"version":1,"code":"KeyK","modifiers":["Control"]}`); !errors.Is(err, ErrInvalid) {
		t.Fatalf("decodeKeyboard local aceitou origem global: %v", err)
	}
	if err := local.ValidateIdentity(context.Background(), "keyboard.global:Control+KeyK"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("validador local aceitou identidade global: %v", err)
	}
}

func TestKeyboardGlobalTriggerPortCancellation(t *testing.T) {
	port := KeyboardGlobalTriggerPort{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := port.Normalize(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Normalize cancelado = %v", err)
	}
	if err := port.ValidateIdentity(ctx, "keyboard.global:KeyK"); !errors.Is(err, context.Canceled) {
		t.Fatalf("ValidateIdentity cancelado = %v", err)
	}
	if _, err := port.Normalize(nil, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Normalize nil = %v", err)
	}
	if err := port.ValidateIdentity(nil, "keyboard.global:KeyK"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ValidateIdentity nil = %v", err)
	}
}

func TestKeyboardGlobalTriggerPortSatisfiesProjectionPorts(t *testing.T) {
	var _ TriggerPort = KeyboardGlobalTriggerPort{}
	var _ TriggerIdentityValidator = KeyboardGlobalTriggerPort{}
}
