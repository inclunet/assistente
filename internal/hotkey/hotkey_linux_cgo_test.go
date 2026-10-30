//go:build linux && cgo

package hotkey

import (
	"testing"

	"golang.design/x/hotkey"
)

func TestLinuxModifierMasksMatchNativeBackend(t *testing.T) {
	for _, pair := range []struct {
		name      string
		got, want hotkey.Modifier
	}{
		{"control", ModCtrl, hotkey.ModCtrl},
		{"shift", ModShift, hotkey.ModShift},
		{"alt", ModAlt, hotkey.Mod1},
		{"super", ModWin, hotkey.Mod4},
	} {
		if pair.got != pair.want {
			t.Errorf("%s: máscara %#x, backend nativo %#x", pair.name, pair.got, pair.want)
		}
	}
}
