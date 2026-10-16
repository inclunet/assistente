package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"assistente/internal/core/ports"
	"assistente/internal/llm"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
)

// Este arquivo implementa llm.AgentActivitySink no SimpleStreamHandler: é por
// aqui que um turno conduzido por agente externo conta o que o agente fez com as
// ferramentas dele e onde cada bloco de resposta termina (AEP-0084 D7 e D13).
//
// As ferramentas são do agente, não do app: os eventos saem com origem
// acp_agent e são arquivadas como atividade externa, nunca executadas pelo app.

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
	name       string
	title      string
	started    time.Time
	iteration  int
	textOffset int
}

// agentActivity acumula o estado da atividade do agente dentro de um turno.
// Tem trava própria porque chega pela mesma goroutine do streaming, mas é lida
// no fechamento de segmento.
type agentActivity struct {
	mu      sync.Mutex
	running map[string]agentToolTrack
	// unnamed liga a classe da ferramenta ao identificador inventado para ela,
	// enquanto essa chamada estiver em andamento.
	unnamed      map[string]string
	unnamedSeq   int
	segmentTools []ports.ToolSummary
	iteration    int
	archiveQueue []toolinvocations.RecordRequest
	archiveDone  chan struct{}
	archiveError error
	archived     map[string]bool
	received     bool
}

// OnAgentToolEvent traduz a atividade de ferramenta do agente para os eventos de
// chat que a UI já sabe renderizar e anunciar.
func (h *SimpleStreamHandler) OnAgentToolEvent(event llm.AgentToolEvent) {
	h.FlushStream()
	h.mu.Lock()
	textOffset := h.promotedContent.Len() + h.accumulatedContent.Len()
	h.mu.Unlock()
	name := singleLine(event.Kind)
	if name == "" {
		name = llm.AgentToolKindOther
	}
	title := singleLine(event.Title)
	failure := singleLine(event.Error)

	terminal := event.Status != "" && event.Status != llm.AgentToolRunning

	h.activity.mu.Lock()
	h.activity.received = true
	if h.activity.running == nil {
		h.activity.running = map[string]agentToolTrack{}
	}
	callID := singleLine(event.ID)
	if callID == "" {
		// O protocolo exige identificador; sem ele o fim ainda precisa achar o
		// começo, e a classe da ferramenta é o que resta para correlacionar. O
		// identificador inventado é único por chamada — reaproveitá-lo entre
		// chamadas seguidas da mesma classe faria a UI reabrir o item anterior.
		if h.activity.unnamed == nil {
			h.activity.unnamed = map[string]string{}
		}
		existente, emAndamento := h.activity.unnamed[name]
		if !emAndamento {
			h.activity.unnamedSeq++
			existente = fmt.Sprintf("agent-%s-%d", name, h.activity.unnamedSeq)
			h.activity.unnamed[name] = existente
		}
		callID = existente
		if terminal {
			delete(h.activity.unnamed, name)
		}
	}
	track, known := h.activity.running[callID]
	if h.activity.archived[callID] {
		h.activity.mu.Unlock()
		return
	}
	if !known {
		track = agentToolTrack{name: name, started: time.Now(), iteration: h.activity.iteration, textOffset: textOffset}
	}
	if title != "" {
		track.title = title
	}
	h.activity.running[callID] = track
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

	if !terminal {
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
		ErrorKind:          errorKind,
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
	h.archiveAgentTool(callID, track, errorKind, failure, duration)
}

// closePendingAgentTools encerra as ferramentas que o agente deixou sem desfecho
// quando o turno acaba — processo morto, cancelamento ou aviso de conclusão que
// nunca veio. Sem isso a ferramenta ficaria girando na tela até o fim do turno e
// sumiria sem explicação; ninguém saberia que ela não terminou.
func (h *SimpleStreamHandler) closePendingAgentTools(errorKind string) {
	if errorKind == "" {
		errorKind = "unknown"
	}
	h.activity.mu.Lock()
	pendentes := make([]pendingAgentTool, 0, len(h.activity.running))
	for callID, track := range h.activity.running {
		pendentes = append(pendentes, pendingAgentTool{callID: callID, track: track})
	}
	h.activity.running = nil
	h.activity.unnamed = nil
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
			ErrorKind:          errorKind,
			Origin:             OriginACPAgent,
			DurationMs:         duracao,
			SurfaceOrigin:      h.SurfaceOrigin,
		})
		if errorKind != "cancelled" {
			EmitToolFailure(h.Emitter, ports.ToolFailureEvent{
				ConversationID:     h.ConversationID,
				TurnID:             h.TurnID,
				AssistantMessageID: h.AssistantMessageID,
				Name:               pendente.track.name,
				CallID:             pendente.callID,
				ErrorKind:          errorKind,
				Retryable:          false,
				DurationMs:         duracao,
				Origin:             OriginACPAgent,
				SurfaceOrigin:      h.SurfaceOrigin,
			})
		}
		h.activity.mu.Lock()
		h.activity.segmentTools = append(h.activity.segmentTools, ports.ToolSummary{
			Name:       pendente.track.name,
			Status:     "error",
			ErrorKind:  errorKind,
			DurationMs: duracao,
			Origin:     OriginACPAgent,
		})
		h.activity.mu.Unlock()
		h.archiveAgentTool(pendente.callID, pendente.track, errorKind, "", duracao)
	}
}

// A escrita não roda no callback do transporte ACP. Um único consumidor
// serializa a fila; a barreira terminal garante o ledger antes do turnPatch.
func (h *SimpleStreamHandler) archiveAgentTool(callID string, track agentToolTrack, errorKind, failure string, duration int64) {
	if h.svc == nil || h.svc.toolInvocations == nil {
		return
	}
	h.activity.mu.Lock()
	defer h.activity.mu.Unlock()
	if h.activity.archived == nil {
		h.activity.archived = make(map[string]bool)
	}
	if h.activity.archived[callID] {
		return
	}
	h.activity.archived[callID] = true
	// O adaptador ACP conhece o protocolo; o ledger recebe só uma observação
	// normalizada, sem argumentos/resultado que o protocolo não forneceu.
	display, _ := json.Marshal(map[string]any{
		"version": 1, "name": track.name, "origin": OriginACPAgent,
		"iteration": track.iteration, "duration_ms": duration,
		"acp_text_offset": track.textOffset, "assistant_message_id": h.AssistantMessageID,
	})
	h.activity.archiveQueue = append(h.activity.archiveQueue, toolinvocations.RecordRequest{
		Observation: &toolinvocations.ExternalObservation{
			CatalogName: "acp_agent__" + track.name, Summary: track.title,
			StartedAt: track.started, DisplayMetadata: display,
		},
		Call:      tools.ToolCall{ID: callID, Type: "function", Function: tools.FunctionCall{Name: track.name}},
		Origin:    toolinvocations.Origin{Type: toolinvocations.OriginChat, ID: h.TurnID, ConversationID: h.ConversationID, TurnID: h.TurnID},
		Iteration: track.iteration, DurationMs: duration, ErrorKind: tools.ErrorKind(errorKind), ErrorMessage: failure,
	})
	if h.activity.archiveDone != nil {
		return
	}
	h.activity.archiveDone = make(chan struct{})
	go func() {
		for {
			h.activity.mu.Lock()
			if len(h.activity.archiveQueue) == 0 {
				close(h.activity.archiveDone)
				h.activity.archiveDone = nil
				h.activity.mu.Unlock()
				return
			}
			req := h.activity.archiveQueue[0]
			h.activity.archiveQueue = h.activity.archiveQueue[1:]
			h.activity.mu.Unlock()
			_, err := h.svc.toolInvocations.Record(h.ctx, req)
			if err != nil {
				h.activity.mu.Lock()
				if h.activity.archiveError == nil {
					h.activity.archiveError = err
				}
				h.activity.mu.Unlock()
			}
		}
	}()
}

func (h *SimpleStreamHandler) flushAgentTools() error {
	h.activity.mu.Lock()
	done := h.activity.archiveDone
	h.activity.mu.Unlock()
	if done != nil {
		<-done
	}
	h.activity.mu.Lock()
	defer h.activity.mu.Unlock()
	return h.activity.archiveError
}

func (h *SimpleStreamHandler) hasAgentActivity() bool {
	h.activity.mu.Lock()
	defer h.activity.mu.Unlock()
	return h.activity.received
}

func (h *SimpleStreamHandler) emitAgentErrorDone() {
	if !h.hasAgentActivity() {
		return
	}
	patch, err := h.svc.buildTurnPatch(h.ctx, h.ConversationID, h.TurnID)
	if err != nil {
		patch = nil
	}
	done := ports.DoneEvent{
		ConversationID: h.ConversationID, TurnID: h.TurnID, AssistantMessageID: h.AssistantMessageID,
		SurfaceOrigin: h.SurfaceOrigin, Reason: "error", ErrorMessage: h.lastError, TurnPatch: patch,
		FinishReason: string(h.finish.Reason), RawReason: h.finish.RawReason,
		Provider: h.finish.Provider, Model: h.finish.Model, EffectiveOutputLimit: h.finish.OutputLimit,
	}
	if h.finish.Provider != "" || h.finish.Model != "" || h.finish.OutputLimit != 0 || h.finish.RawReason != "" || h.finish.Reason != "" || h.finish.ResponseBytes != 0 {
		responseBytes := h.finish.ResponseBytes
		done.ResponseBytes = &responseBytes
	}
	if h.usage.OutputTokensReported {
		outputTokens := h.usage.CompletionTokens
		done.OutputTokens = &outputTokens
	}
	if h.usage.ReasoningTokensReported {
		reasoningTokens := h.usage.ReasoningTokens
		done.ReasoningTokens = &reasoningTokens
	}
	h.Emitter.Emit("chat:done", done)
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

// OnTurnNotice repassa à interface um aviso sobre o turno. Vai como evento
// próprio, e não como texto na resposta: o aviso é do app, e emendá-lo na
// mensagem do modelo o deixaria salvo e lido como se fosse fala dele.
func (h *SimpleStreamHandler) OnTurnNotice(notice llm.TurnNotice) {
	if h.Emitter == nil || strings.TrimSpace(string(notice.Kind)) == "" {
		return
	}
	h.Emitter.Emit("chat:notice", ports.ChatNoticeEvent{
		ConversationID: h.ConversationID,
		Kind:           string(notice.Kind),
		Count:          notice.Count,
		Model:          notice.Model,
	})
}
