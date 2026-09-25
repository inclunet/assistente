package commandconfig

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"assistente/internal/commandcatalog"
)

// StreamDeckTriggerSpec é a especificação versionada de uma tecla física do
// Stream Deck. Device é o serial canônico do dispositivo e Key é o índice da
// tecla, começando em zero.
type StreamDeckTriggerSpec struct {
	Device string
	Key    int
}

var streamDeckSerialPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// StreamDeckTriggerPort normaliza acionadores streamdeck.key sem conhecer o
// dispositivo físico nem executar qualquer efeito.
type StreamDeckTriggerPort struct{}

func (StreamDeckTriggerPort) Normalize(ctx context.Context, raw []byte) (string, error) {
	if ctx == nil {
		return "", ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	fields, err := strictObject(string(raw))
	if err != nil || !exactFields(fields, "version", "device", "key") || !versionOne(fields) {
		return "", ErrInvalid
	}
	device, ok := jsonString(fields["device"])
	if !ok || !streamDeckSerialPattern.MatchString(device) {
		return "", ErrInvalid
	}
	key, err := strconv.Atoi(string(fields["key"]))
	if err != nil || key < 0 || key > 255 || strconv.Itoa(key) != string(fields["key"]) {
		return "", ErrInvalid
	}
	return streamDeckIdentity(device, key), nil
}

func (StreamDeckTriggerPort) ValidateIdentity(ctx context.Context, identity string) error {
	if ctx == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := ParseStreamDeckTriggerIdentity(identity); err != nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func streamDeckIdentity(device string, key int) string {
	return string(commandcatalog.StreamDeck) + ":" + device + ":key:" + strconv.Itoa(key)
}

// ParseStreamDeckTriggerIdentity interpreta uma identidade canônica de tecla
// Stream Deck e retorna sua especificação estruturada.
func ParseStreamDeckTriggerIdentity(identity string) (StreamDeckTriggerSpec, error) {
	const prefix = "streamdeck.key:"
	var zero StreamDeckTriggerSpec
	if !strings.HasPrefix(identity, prefix) {
		return zero, ErrInvalid
	}
	rest := strings.TrimPrefix(identity, prefix)
	device, keyText, ok := strings.Cut(rest, ":key:")
	if !ok || strings.Contains(keyText, ":") || !streamDeckSerialPattern.MatchString(device) || keyText == "" {
		return zero, ErrInvalid
	}
	key, err := strconv.Atoi(keyText)
	if err != nil || key < 0 || key > 255 || strconv.Itoa(key) != keyText {
		return zero, ErrInvalid
	}
	return StreamDeckTriggerSpec{Device: device, Key: key}, nil
}
