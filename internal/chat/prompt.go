package chat

import "assistente/internal/llm"

// DefaultSystemPrompt é o system prompt base usado quando nenhum prompt customizado é fornecido.
const DefaultSystemPrompt = `You are a helpful, intelligent assistant. You provide accurate, thoughtful responses and assist users with various tasks.

Key behaviors:
- Be concise but thorough
- When uncertain, acknowledge limitations
- Use markdown formatting for better readability
- Adapt your communication style to the user's needs`

// InjectSystemPrompt insere ou combina o fullSystemPrompt nas mensagens.
//   - Se fullSystemPrompt for vazio, retorna as mensagens sem alteração.
//   - Se não existir mensagem de sistema, prepend uma nova.
//   - Se existir, combina: fullSystemPrompt + "\n\n" + conteúdo existente.
func InjectSystemPrompt(messages []llm.Message, fullSystemPrompt string) []llm.Message {
	if fullSystemPrompt == "" {
		return messages
	}

	systemIndex := -1
	for i, msg := range messages {
		if msg.Role == "system" {
			systemIndex = i
			break
		}
	}

	if systemIndex == -1 {
		systemMsg := llm.Message{
			Role:    "system",
			Content: fullSystemPrompt,
		}
		return append([]llm.Message{systemMsg}, messages...)
	}

	newMessages := make([]llm.Message, len(messages))
	copy(newMessages, messages)

	switch content := messages[systemIndex].Content.(type) {
	case string:
		newMessages[systemIndex].Content = fullSystemPrompt + "\n\n" + content
	default:
		// Conteúdo não-string (multimodal): substitui integralmente
		newMessages[systemIndex].Content = fullSystemPrompt
	}

	return newMessages
}
