package llm

import (
	"context"
	"math/rand"
	"strings"
	"time"
)

// MCPToolEvent descreve uma chamada MCP nativa executada server-side pelo LLM provider.
// Usado para tracking/auditoria — no caminho nativo o Assistente não executa a tool localmente,
// então este evento é a única forma de auditar o que aconteceu.
type MCPToolEvent struct {
	ID          string // ID único da chamada (atribuído pelo provider)
	Name        string // Nome da tool chamada (ex: "jira_search")
	ServerLabel string // Label do servidor MCP (ex: "Atlassian")
	Arguments   string // JSON dos argumentos enviados
	Output      string // Resultado retornado pelo servidor MCP (pode ser grande)
	Error       string // Mensagem de erro, se houver
	IsCompleted bool   // true = chamada concluída (com ou sem erro), false = em andamento
}

// StreamHandler é a interface para lidar com eventos de streaming de LLM.
type StreamHandler interface {
	OnChunk(content string)
	OnThinking(content string)
	OnThinkingDone(fullReasoning string)
	OnToolCalls(calls []ToolCall, fullResponse string, usage Usage, model string)
	OnError(err string)
	OnDone(fullResponse string, usage Usage, model string)

	// OnMCPToolEvent é chamado quando uma tool MCP nativa é invocada ou concluída server-side.
	// Permite tracking/auditoria de chamadas que o Assistente não executa localmente.
	OnMCPToolEvent(event MCPToolEvent)
}

func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}

func sleepWithJitter(ctx context.Context, base time.Duration) {
	if base <= 0 {
		return
	}

	jitter := time.Duration(rand.Intn(250)) * time.Millisecond
	wait := base + jitter

	t := time.NewTimer(wait)
	defer t.Stop()

	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// processThinkingTags detecta e extrai conteúdo de tags <thinking> do streaming.
// Retorna o conteúdo que NÃO é thinking para ser processado normalmente.
func processThinkingTags(content string, isThinking *bool, thinkingBuffer, fullReasoning *strings.Builder, handler StreamHandler) string {
	var result strings.Builder
	i := 0

	for i < len(content) {
		if *isThinking {
			endIdx := strings.Index(content[i:], "</thinking>")
			if endIdx != -1 {
				thinkingContent := content[i : i+endIdx]
				thinkingBuffer.WriteString(thinkingContent)
				fullReasoning.WriteString(thinkingContent)
				handler.OnThinking(thinkingContent)

				*isThinking = false
				i += endIdx + len("</thinking>")
			} else {
				thinkingContent := content[i:]
				thinkingBuffer.WriteString(thinkingContent)
				fullReasoning.WriteString(thinkingContent)
				handler.OnThinking(thinkingContent)
				return result.String()
			}
		} else {
			startIdx := strings.Index(content[i:], "<thinking>")
			if startIdx != -1 {
				result.WriteString(content[i : i+startIdx])
				*isThinking = true
				thinkingBuffer.Reset()
				i += startIdx + len("<thinking>")
			} else {
				result.WriteString(content[i:])
				break
			}
		}
	}

	return result.String()
}
