package agent

import (
	"sort"
	"strings"
	"sync"
	"time"

	"assistente/internal/core/ports"
	"assistente/internal/llm"
)

// Este arquivo implementa llm.AgentActivitySink no SimpleStreamHandler: é por
// aqui que um turno conduzido por agente externo conta o que o agente fez com as
// ferramentas dele e onde cada bloco de resposta termina (AEP-0084 D7 e D13).
//
// As ferramentas são do agente, não do app: os eventos saem com origem
// acp_agent, servem para a UI e o leitor de telas, e nada é executado nem
// persistido como invocação de ferramenta do app.

// singleLine achata quebras de linha vindas do protocolo. O saneamento de
// conteúdo não confiável é do provider (AEP-0084 D11), mas rótulo e anúncio são
// de linha única: uma quebra aqui estoura o layout e atrapalha o leitor de
// telas, então a garantia também vale na saída.
func singleLine(s string) string {
	replaced := strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(s)
	return strings.TrimSpace(replaced)
}

// agentToolTrack guarda o que o app precisa lembrar de uma ferramenta do agente
// entre o aviso de início e o de fim.
type agentToolTrack struct {
	name    string
	started time.Time
}

// agentActivity acumula o estado da atividade do agente dentro de um turno.
// Tem trava própria porque chega pela mesma goroutine do streaming, mas é lida
// no fechamento de segmento.
type agentActivity struct {
	mu           sync.Mutex
	running      map[string]agentToolTrack
	segmentTools []ports.ToolSummary
	iteration    int
}

// OnAgentToolEvent traduz a atividade de ferramenta do agente para os eventos de
// chat que a UI já sabe renderizar e anunciar.
func (h *SimpleStreamHandler) OnAgentToolEvent(event llm.AgentToolEvent) {
	name := singleLine(event.Kind)
	if name == "" {
		name = llm.AgentToolKindOther
	}
	title := singleLine(event.Title)
	failure := singleLine(event.Error)

	h.activity.mu.Lock()
	if h.activity.running == nil {
		h.activity.running = map[string]agentToolTrack{}
	}
	callID := singleLine(event.ID)
	if callID == "" {
		// O protocolo exige identificador; sem ele o fim ainda precisa achar o
		// começo. A classe da ferramenta é o que resta para correlacionar — pior
		// que isso só seria deixar o item preso em execução para sempre e abrir
		// outro no lugar dele.
		callID = "agent-" + name
	}
	track, known := h.activity.running[callID]
	if !known {
		track = agentToolTrack{name: name, started: time.Now()}
		h.activity.running[callID] = track
	}
	h.activity.mu.Unlock()

	// Um fim sem início conhecido ainda precisa aparecer: sem o start a UI não
	// tem item para atualizar e a ferramenta passaria despercebida.
	if !known {
		EmitToolStart(h.Emitter, ports.ToolStartEvent{
			ConversationID:     h.ConversationID,
			TurnID:             h.TurnID,
			AssistantMessageID: h.AssistantMessageID,
			Name:               name,
			CallID:             callID,
			Summary:            title,
			Origin:             OriginACPAgent,
			SurfaceOrigin:      h.SurfaceOrigin,
		})
	}

	if event.Status == "" || event.Status == llm.AgentToolRunning {
		return
	}

	h.activity.mu.Lock()
	delete(h.activity.running, callID)
	h.activity.mu.Unlock()

	duration := time.Since(track.started).Milliseconds()
	failed := event.Status != llm.AgentToolCompleted
	status := "ok"
	if failed {
		status = "error"
	}
	errorKind := ""
	if failed {
		errorKind = "unknown"
		if event.Status == llm.AgentToolCancelled {
			errorKind = "cancelled"
		}
	}

	EmitToolEnd(h.Emitter, ports.ToolEndEvent{
		ConversationID:     h.ConversationID,
		TurnID:             h.TurnID,
		AssistantMessageID: h.AssistantMessageID,
		Name:               track.name,
		CallID:             callID,
		Status:             status,
		Summary:            title,
		Error:              failure,
		Origin:             OriginACPAgent,
		DurationMs:         duration,
		SurfaceOrigin:      h.SurfaceOrigin,
	})

	// Cancelamento não é falha da ferramenta: quem cancelou o turno já sabe o
	// que aconteceu e o anúncio assertivo de falha seria ruído.
	if failed && event.Status != llm.AgentToolCancelled {
		EmitToolFailure(h.Emitter, ports.ToolFailureEvent{
			ConversationID:     h.ConversationID,
			TurnID:             h.TurnID,
			AssistantMessageID: h.AssistantMessageID,
			Name:               track.name,
			CallID:             callID,
			ErrorKind:          errorKind,
			Retryable:          false,
			Message:            failure,
			DurationMs:         duration,
			Origin:             OriginACPAgent,
			SurfaceOrigin:      h.SurfaceOrigin,
		})
	}

	h.activity.mu.Lock()
	h.activity.segmentTools = append(h.activity.segmentTools, ports.ToolSummary{
		Name:       track.name,
		Status:     status,
		ErrorKind:  errorKind,
		DurationMs: duration,
		Origin:     OriginACPAgent,
	})
	h.activity.mu.Unlock()
}

// closePendingAgentTools encerra as ferramentas que o agente deixou sem desfecho
// quando o turno acaba — processo morto, cancelamento ou aviso de conclusão que
// nunca veio. Sem isso a ferramenta ficaria girando na tela até o fim do turno e
// sumiria sem explicação; ninguém saberia que ela não terminou.
func (h *SimpleStreamHandler) closePendingAgentTools() {
	h.activity.mu.Lock()
	pendentes := make([]pendingAgentTool, 0, len(h.activity.running))
	for callID, track := range h.activity.running {
		pendentes = append(pendentes, pendingAgentTool{callID: callID, track: track})
	}
	h.activity.running = nil
	h.activity.mu.Unlock()

	if len(pendentes) == 0 {
		return
	}
	sort.Slice(pendentes, func(i, j int) bool {
		if pendentes[i].track.started.Equal(pendentes[j].track.started) {
			return pendentes[i].callID < pendentes[j].callID
		}
		return pendentes[i].track.started.Before(pendentes[j].track.started)
	})

	for _, pendente := range pendentes {
		duracao := time.Since(pendente.track.started).Milliseconds()
		EmitToolEnd(h.Emitter, ports.ToolEndEvent{
			ConversationID:     h.ConversationID,
			TurnID:             h.TurnID,
			AssistantMessageID: h.AssistantMessageID,
			Name:               pendente.track.name,
			CallID:             pendente.callID,
			Status:             "error",
			Origin:             OriginACPAgent,
			DurationMs:         duracao,
			SurfaceOrigin:      h.SurfaceOrigin,
		})
		h.activity.mu.Lock()
		h.activity.segmentTools = append(h.activity.segmentTools, ports.ToolSummary{
			Name:       pendente.track.name,
			Status:     "error",
			ErrorKind:  "unknown",
			DurationMs: duracao,
			Origin:     OriginACPAgent,
		})
		h.activity.mu.Unlock()
	}
}

type pendingAgentTool struct {
	callID string
	track  agentToolTrack
}

// OnSegmentDone fecha o bloco corrente do turno: o texto acumulado até aqui vira
// segmento, é lido em voz alta sem esperar o fim do turno e sai do acumulador
// para não voltar repetido no próximo chat:stream.
func (h *SimpleStreamHandler) OnSegmentDone() {
	text := h.CutSegment()

	h.activity.mu.Lock()
	tools := h.activity.segmentTools
	h.activity.segmentTools = nil
	iteration := h.activity.iteration
	h.activity.iteration++
	h.activity.mu.Unlock()

	if strings.TrimSpace(text) == "" && len(tools) == 0 {
		return
	}

	h.Emitter.Emit("chat:segment_done", ports.SegmentDoneEvent{
		ConversationID:     h.ConversationID,
		TurnID:             h.TurnID,
		AssistantMessageID: h.AssistantMessageID,
		Content:            text,
		Iteration:          iteration,
		HasMore:            true,
		ToolsInIteration:   tools,
		SurfaceOrigin:      h.SurfaceOrigin,
	})

	if h.svc != nil && h.svc.onSpeechRequest != nil && strings.TrimSpace(text) != "" {
		h.svc.onSpeechRequest(h.ConversationID, "", "assistant", text, "segment", h.profileSlug, false)
	}
}
