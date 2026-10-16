//go:build !windows

package hotkey

import "golang.design/x/hotkey"

func newNativeHotkey(modifiers []hotkey.Modifier, key hotkey.Key) NativeHotkey {
	return unsupportedNativeHotkey{}
}
