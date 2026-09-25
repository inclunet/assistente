package commandconfig

import (
	"context"
	"strings"
)

// PaletteTriggerPort identifica a seleção lógica da paleta. Não aceita
// identidades físicas nem transforma uma seleção em observação de teclado.
type PaletteTriggerPort struct{}

func (PaletteTriggerPort) Normalize(ctx context.Context, raw []byte) (string, error) {
	if ctx == nil {
		return "", ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	fields, err := strictObject(string(raw))
	if err != nil || !exactFields(fields, "version", "selection") || !versionOne(fields) {
		return "", ErrInvalid
	}
	selection, ok := jsonString(fields["selection"])
	if !ok || !namespacedID(selection) {
		return "", ErrInvalid
	}
	return "palette:" + selection, nil
}

func (PaletteTriggerPort) ValidateIdentity(ctx context.Context, identity string) error {
	if ctx == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !strings.HasPrefix(identity, "palette:") || !namespacedID(strings.TrimPrefix(identity, "palette:")) {
		return ErrInvalid
	}
	return nil
}
