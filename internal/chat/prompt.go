package chat

import "assistente/internal/llm"

// DefaultSystemPrompt é o system prompt base usado quando nenhum prompt customizado é fornecido.
const DefaultSystemPrompt = `You are a helpful, intelligent assistant. You provide accurate, thoughtful responses and assist users with various tasks.

Key behaviors:
- Be concise but thorough
- When uncertain, acknowledge limitations
- Use markdown formatting for better readability
- Adapt your communication style to the user's needs`

// CatalogFirstToolPrompt instrui o modelo a seguir o fluxo catalog-first (AEP-0049, D16):
// quando o gating por catálogo está ativo, as únicas tools inicialmente disponíveis
// são tools de controle como "tool_catalog" e "load_skill"; as demais só ficam
// disponíveis APÓS serem selecionadas pelo catálogo.
// Esta seção é injetada sempre que o gating por catálogo está ativo, para que a ordem
// "consultar catálogo → tools ficam disponíveis → usar" não dependa apenas da exposição
// de tools nem de uma descrição de tool opcional.
const CatalogFirstToolPrompt = `<tool_selection_protocol>
Tool access in this session is gated by a catalog. Initially the only regular tool selection capability available to you is "tool_catalog"; runtime control tools such as "load_skill" may also be available for skill loading. Other tools (file access, web search, tasks, MCP servers, etc.) are NOT yet available and will not appear until you select them.

Follow this order strictly:
1. First, call "tool_catalog" to discover and select the capabilities you need for the task. You can filter by origin, category, class, package, risk or availability.
2. The tools you select become available only on the NEXT turn, after the catalog responds.
3. Only AFTER that may you invoke the real tools.

Never call a tool that has not been provided to you yet — such a call will fail. Whenever you are unsure which tools exist, consult "tool_catalog" first.
</tool_selection_protocol>`

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
