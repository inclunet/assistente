package telegram

import "strings"

// maxMessageLength limita cada parte a 4096 bytes, conservador para texto Unicode.
const maxMessageLength = 4096

// SplitMessage divide uma mensagem longa em múltiplas mensagens respeitando
// o limite do Telegram. Prefere linhas e espaços e mantém os separadores na mensagem.
func SplitMessage(text string) []string {
	if len(text) <= maxMessageLength {
		return []string{text}
	}

	var parts []string
	remaining := text

	for len(remaining) > 0 {
		if len(remaining) <= maxMessageLength {
			parts = append(parts, remaining)
			break
		}

		// range visita apenas o início dos caracteres. Inclua até o último que
		// cabe inteiro, mesmo quando o limite cai dentro de um emoji.
		cutAt := 0
		for pos := range remaining {
			if pos > maxMessageLength {
				break
			}
			cutAt = pos
		}
		windowStart := maxMessageLength / 2
		window := remaining[windowStart:cutAt]
		if newline := strings.LastIndexByte(window, '\n'); newline >= 0 {
			cutAt = windowStart + newline + 1
		} else if space := strings.LastIndexByte(window, ' '); space >= 0 {
			cutAt = windowStart + space + 1
		}
		parts = append(parts, remaining[:cutAt])
		remaining = remaining[cutAt:]
	}

	return parts
}
