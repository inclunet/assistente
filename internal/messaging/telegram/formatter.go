package telegram

// maxMessageLength é o limite de caracteres de uma mensagem no Telegram.
const maxMessageLength = 4096

// SplitMessage divide uma mensagem longa em múltiplas mensagens respeitando
// o limite do Telegram (4096 caracteres). Tenta quebrar em linhas quando possível.
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

		// Tenta encontrar uma quebra de linha antes do limite
		cutAt := maxMessageLength
		for i := maxMessageLength - 1; i > maxMessageLength/2; i-- {
			if remaining[i] == '\n' {
				cutAt = i + 1 // inclui a quebra de linha
				break
			}
		}

		// Se não achou quebra de linha, tenta um espaço
		if cutAt == maxMessageLength {
			for i := maxMessageLength - 1; i > maxMessageLength/2; i-- {
				if remaining[i] == ' ' {
					cutAt = i + 1
					break
				}
			}
		}

		parts = append(parts, remaining[:cutAt])
		remaining = remaining[cutAt:]
	}

	return parts
}
