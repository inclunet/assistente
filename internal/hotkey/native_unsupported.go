package hotkey

import (
	"errors"

	"golang.design/x/hotkey"
)

// ErrNativeUnsupported impede registros sem as garantias de ownership e
// no-repeat exigidas pelo AEP-0103. Não há fallback para captura global.
var ErrNativeUnsupported = errors.New("global hotkeys require the Windows ownership and no-repeat adapter")

type unsupportedNativeHotkey struct{}

func (unsupportedNativeHotkey) Register() error              { return ErrNativeUnsupported }
func (unsupportedNativeHotkey) Unregister() error            { return ErrNativeUnsupported }
func (unsupportedNativeHotkey) Keydown() <-chan hotkey.Event { return nil }
