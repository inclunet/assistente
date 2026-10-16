package commandconfig

import "context"

// KeyboardGlobalTriggerPort normaliza hotkeys globais como acordes simples
// keyboard.global v1. A porta compartilha a gramática do acorde local v1, mas
// não aceita sequências e mantém a identidade de origem global.
//
// A existência desta porta não registra a hotkey nem habilita bindings. O
// bootstrap precisa fornecê-la explicitamente à projeção confiável.
type KeyboardGlobalTriggerPort struct{}

// Normalize implementa TriggerPort.
func (KeyboardGlobalTriggerPort) Normalize(ctx context.Context, raw []byte) (string, error) {
	if ctx == nil {
		return "", ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	identity, err := decodeKeyboardGlobal(string(raw))
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return identity, nil
}

// ValidateIdentity implementa TriggerIdentityValidator para defaults builtin.
func (KeyboardGlobalTriggerPort) ValidateIdentity(ctx context.Context, identity string) error {
	if ctx == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !canonicalKeyboardV1Trigger("keyboard.global", identity) {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
