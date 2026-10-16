package commandconfig

import "context"

// KeyboardLocalTriggerPort normaliza documentos de acionador keyboard.local
// usando a gramática canônica do contrato de configuração.
//
// O tipo não carrega estado nem aceita regras fornecidas pelo chamador. Isso
// permite que a mesma porta seja usada na projeção persistida e na validação
// de identidades builtin.
type KeyboardLocalTriggerPort struct{}

// Normalize implementa TriggerPort.
func (KeyboardLocalTriggerPort) Normalize(ctx context.Context, raw []byte) (string, error) {
	if ctx == nil {
		return "", ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	identity, err := decodeKeyboard("keyboard.local", string(raw))
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return identity, nil
}

// ValidateIdentity implementa TriggerIdentityValidator para defaults builtin.
func (KeyboardLocalTriggerPort) ValidateIdentity(ctx context.Context, identity string) error {
	if ctx == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !canonicalLocalTrigger(identity) {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
