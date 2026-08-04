package llm

import (
	"context"
	"math/rand"
	"strings"
	"time"

	"assistente/internal/acp"
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

// AgentToolKindOther é o nome usado quando o agente não classifica a
// ferramenta, ou classifica com algo fora do protocolo.
const AgentToolKindOther = acp.ToolKindOther

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

// TurnNoticeKind identifica o que o aviso conta. É código, e não frase: quem
// exibe traduz para o idioma de quem lê.
type TurnNoticeKind string

// TurnNoticeAttachmentsNotSent: o turno seguiu sem parte dos anexos, porque o
// agente não recebe esse tipo de conteúdo ou porque o anexo não pôde ser
// embutido no pedido (AEP-0084). Descartá-los em silêncio deixaria a pessoa
// esperando uma resposta sobre uma imagem que o agente nunca viu.
const TurnNoticeAttachmentsNotSent TurnNoticeKind = "attachments_not_sent"

// O modelo escolhido no perfil não pôde valer neste turno (AEP-0084 D6). O turno
// segue no modelo em que o agente está, porque uma resposta do modelo errado é
// melhor do que resposta nenhuma — mas quem escolheu precisa saber, senão lê a
// resposta atribuindo-a a um modelo que não a escreveu.
const (
	// TurnNoticeModelNotOffered é o modelo que este agente não tem. Costuma ser
	// escolha antiga do perfil, guardada quando o provider era outro.
	TurnNoticeModelNotOffered TurnNoticeKind = "model_not_offered"
	// TurnNoticeModelNotApplied é a troca que o agente recusou ou que não chegou
	// a ele.
	TurnNoticeModelNotApplied TurnNoticeKind = "model_not_applied"
)

// TurnNotice é um aviso sobre o próprio turno: não é a resposta, não é falha e
// não encerra nada.
type TurnNotice struct {
	Kind TurnNoticeKind
	// Count é a quantidade a que o aviso se refere, quando ele conta coisas.
	Count int
	// Model é o modelo de que o aviso fala — o que de fato atendeu ao turno,
	// quando o pedido não pôde valer. Vai como identificador do provedor, que é
	// o mesmo texto que a pessoa vê no seletor.
	Model string
}

// TurnNoticeSink é o canal por onde o provider avisa a pessoa de algo que
// aconteceu com o turno dela.
//
// É opcional — o provider descobre com type assertion sobre o StreamHandler.
type TurnNoticeSink interface {
	OnTurnNotice(notice TurnNotice)
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
