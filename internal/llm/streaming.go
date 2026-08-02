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

// Status de uma ferramenta executada por um agente externo (AEP-0084 D7).
const (
	AgentToolRunning   = "running"
	AgentToolCompleted = "completed"
	AgentToolFailed    = "failed"
	AgentToolCancelled = "cancelled"
)

// AgentToolKindOther é o nome usado quando o agente não classifica a ferramenta.
const AgentToolKindOther = "other"

// AgentToolEvent descreve uma ferramenta que o agente externo executou por conta
// própria. O app não executa nada aqui: o evento existe para a UI e o leitor de
// telas contarem o que está acontecendo do outro lado (AEP-0084 D7).
//
// Kind e Title vêm do protocolo e são dado não confiável: quem preenche este
// evento já os entrega saneados e sem quebras de linha (AEP-0084 D11).
type AgentToolEvent struct {
	ID     string // Identificador da chamada no agente; correlaciona início e fim
	Kind   string // Classe da ferramenta (read, edit, execute, search…), vira o nome exibido
	Title  string // Resumo legível do que a ferramenta está fazendo
	Status string // AgentToolRunning | AgentToolCompleted | AgentToolFailed | AgentToolCancelled
	Error  string // Mensagem de erro quando o agente reporta falha
}

// AgentActivitySink é o canal por onde um provider cujo turno é conduzido por um
// agente externo conta o que aconteceu além do texto: as ferramentas que o
// agente rodou sozinho e o fim de cada bloco da resposta.
//
// É opcional — o provider descobre com type assertion sobre o StreamHandler e
// segue sem ele quando o handler não sabe receber esses avisos.
type AgentActivitySink interface {
	// OnAgentToolEvent informa início, conclusão ou falha de uma ferramenta do agente.
	OnAgentToolEvent(event AgentToolEvent)
	// OnSegmentDone fecha o bloco de texto corrente: o que já foi emitido vira
	// segmento e é lido em voz alta sem esperar o turno acabar (AEP-0084 D13).
	OnSegmentDone()
}

// NonRetryableErrorSink recebe do provider o aviso de que o erro que vem a
// seguir não pode ser repetido sozinho pela auto-recuperação.
//
// O laço de recuperação retenta pela presença de erro, sem noção de
// retentabilidade. Para um provider HTTP repetir é inofensivo; para um turno
// que um agente de código já aceitou, repetir é refazer edição de arquivo e
// comando na máquina (AEP-0084 D4). Quem sabe se repetir é seguro é o provider,
// e é por aqui que ele diz.
//
// É opcional — o provider descobre com type assertion sobre o StreamHandler.
type NonRetryableErrorSink interface {
	// MarkErrorNotRetryable é chamado antes de OnError, e vale para ele.
	MarkErrorNotRetryable()
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
