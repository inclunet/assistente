//go:build linux

package hotkey

import (
	"fmt"

	"golang.design/x/hotkey"
)

// parseKeyStringImpl implementação específica para Linux/X11
// Usa keycodes do X11 (valores menores que evitam overflow)
func parseKeyStringImpl(key string) (hotkey.Key, error) {
	// X11 keycodes para letras e teclas comuns
	keyMap := map[string]hotkey.Key{
		"A": 0x26, "B": 0x38, "C": 0x36, "D": 0x28,
		"E": 0x1A, "F": 0x29, "G": 0x2A, "H": 0x2B,
		"I": 0x1F, "J": 0x2C, "K": 0x2D, "L": 0x2E,
		"M": 0x3A, "N": 0x39, "O": 0x20, "P": 0x21,
		"Q": 0x18, "R": 0x1B, "S": 0x27, "T": 0x1C,
		"U": 0x1E, "V": 0x37, "W": 0x19, "X": 0x35,
		"Y": 0x1D, "Z": 0x34,
		"SPACE": 0x41,
		"F1":    0x43, "F2": 0x44, "F3": 0x45, "F4": 0x46,
		"F5": 0x47, "F6": 0x48, "F7": 0x49, "F8": 0x4A,
		"F9": 0x4B, "F10": 0x4C, "F11": 0x5F, "F12": 0x60,
	}

	if k, ok := keyMap[key]; ok {
		return k, nil
	}
	return 0, fmt.Errorf("unknown key: %s", key)
}
