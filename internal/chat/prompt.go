package chat

import "assistente/internal/llm"

// DefaultSystemPrompt é o system prompt base usado quando nenhum prompt customizado é fornecido.
const DefaultSystemPrompt = `You are a helpful, intelligent assistant. You provide accurate, thoughtful responses and assist users with various tasks.

Key behaviors:
- Be concise but thorough
- When uncertain, acknowledge limitations
- Use markdown formatting for better readability
- Adapt your communication style to the user's needs`

// CatalogFirstToolPrompt instrui o modelo sobre o fluxo híbrido da AEP-0081:
// baseline e um preload read-only do primeiro turno podem coexistir com o
// catálogo; demais tools só ficam disponíveis após load.
// Esta seção é injetada sempre que o gating por catálogo está ativo, para que a ordem
// "consultar catálogo → tools ficam disponíveis → usar" não dependa apenas da exposição
// de tools nem de uma descrição de tool opcional.
const CatalogFirstToolPrompt = `<tool_selection_protocol>
Tool access in this session is gated by a catalog. A small profile baseline and task-relevant read-only tools may already be available. Never assume other tools exist until they are provided or loaded through "tool_catalog".

When a needed capability is not already available:
1. Call "tool_catalog" once with action="search" and a task-oriented query. You can also filter by origin, category, class, package, risk or availability.
2. Call "tool_catalog" with action="load" and exact names or a bounded selector such as "mcp/atlassian/*".
3. Loaded tools become available on the NEXT iteration after the catalog responds with loaded_tools.
4. Then invoke the newly available tools.

Never call a tool that has not been provided. Search and preload never override profile policy, opt-in, risk controls, allowlists, confirmations or schema budget.
</tool_selection_protocol>`

// InjectSystemPrompt insere ou combina o fullSystemPrompt nas mensagens.
//   - Se fullSystemPrompt for vazio, retorna as mensagens sem alteração.
//   - Se não existir mensagem de sistema, prepend uma nova.
//   - Se existir, combina: fullSystemPrompt + "\n\n" + conteúdo existente.
func InjectSystemPrompt(messages []llm.Message, fullSystemPrompt string) []llm.Message {
	return InjectSystemPromptWithCachePrefix(messages, fullSystemPrompt, 0)
}

// InjectSystemPromptWithCachePrefix insere o system prompt e, quando
// cachePrefixLen > 0, marca o prefixo estável para providers que suportam
// cache_control explícito. O metadado é interno e não muda o texto do prompt.
func InjectSystemPromptWithCachePrefix(messages []llm.Message, fullSystemPrompt string, cachePrefixLen int) []llm.Message {
	if fullSystemPrompt == "" {
		return messages
	}
	if cachePrefixLen < 0 {
		cachePrefixLen = 0
	}
	if cachePrefixLen > len(fullSystemPrompt) {
		cachePrefixLen = len(fullSystemPrompt)
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
			Role:                        "system",
			Content:                     fullSystemPrompt,
			SystemCacheControlPrefixLen: cachePrefixLen,
		}
		return append([]llm.Message{systemMsg}, messages...)
	}

	newMessages := make([]llm.Message, len(messages))
	copy(newMessages, messages)

	switch content := messages[systemIndex].Content.(type) {
	case string:
		newMessages[systemIndex].Content = fullSystemPrompt + "\n\n" + content
		newMessages[systemIndex].SystemCacheControlPrefixLen = cachePrefixLen
	default:
		// Conteúdo não-string (multimodal): substitui integralmente
		newMessages[systemIndex].Content = fullSystemPrompt
		newMessages[systemIndex].SystemCacheControlPrefixLen = cachePrefixLen
	}

	return newMessages
}
