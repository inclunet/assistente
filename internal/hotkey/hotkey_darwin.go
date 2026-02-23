//go:build darwin

package hotkey

import (
	"fmt"

	"golang.design/x/hotkey"
)

// parseKeyStringImpl implementação específica para Darwin/macOS
// Usa códigos de tecla do macOS (kVK_* constants)
func parseKeyStringImpl(key string) (hotkey.Key, error) {
	// Códigos de tecla do macOS baseados em Carbon HIToolbox/Events.h
	keyMap := map[string]hotkey.Key{
		"A": 0x00, "B": 0x0B, "C": 0x08, "D": 0x02,
		"E": 0x0E, "F": 0x03, "G": 0x05, "H": 0x04,
		"I": 0x22, "J": 0x26, "K": 0x28, "L": 0x25,
		"M": 0x2E, "N": 0x2D, "O": 0x1F, "P": 0x23,
		"Q": 0x0C, "R": 0x0F, "S": 0x01, "T": 0x11,
		"U": 0x20, "V": 0x09, "W": 0x0D, "X": 0x07,
		"Y": 0x10, "Z": 0x06,
		"SPACE": 0x31,
		"F1":    0x7A, "F2": 0x78, "F3": 0x63, "F4": 0x76,
		"F5": 0x60, "F6": 0x61, "F7": 0x62, "F8": 0x64,
		"F9": 0x65, "F10": 0x6D, "F11": 0x67, "F12": 0x6F,
	}

	if k, ok := keyMap[key]; ok {
		return k, nil
	}
	return 0, fmt.Errorf("unknown key: %s", key)
}
