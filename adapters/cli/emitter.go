// Package cli contém Outbound Adapters para o modo CLI/headless.
// O EmitterAdapter traduz eventos do sistema para output no terminal.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"assistente/internal/core/ports"
)

// EmitterAdapter implementa ports.Emitter escrevendo eventos relevantes no terminal.
// Eventos de streaming de chat são impressos token a token no stdout.
// Demais eventos são ignorados em modo silencioso ou logados em modo verbose.
type EmitterAdapter struct {
	mu             sync.Mutex
	out            io.Writer // stdout por padrão
	errOut         io.Writer // stderr por padrão
	verbose        bool
	done           chan struct{} // sinaliza fim do streaming (chat:stream Done=true ou chat:error)
	lastPrinted    int          // quantidade de bytes de Content já impressos (para imprimir só o delta)
	conversationID uint         // conversa ativa; 0 = aceita qualquer conversa
}

// EmitterOption configura o EmitterAdapter.
type EmitterOption func(*EmitterAdapter)

// WithVerbose habilita logging de todos os eventos no stderr.
func WithVerbose(v bool) EmitterOption {
	return func(e *EmitterAdapter) { e.verbose = v }
}

// WithOutput define o writer de saída (padrão: os.Stdout).
func WithOutput(w io.Writer) EmitterOption {
	return func(e *EmitterAdapter) { e.out = w }
}

// WithErrOutput define o writer de erros (padrão: os.Stderr).
func WithErrOutput(w io.Writer) EmitterOption {
	return func(e *EmitterAdapter) { e.errOut = w }
}

// NewEmitterAdapter cria um EmitterAdapter com as opções fornecidas.
func NewEmitterAdapter(opts ...EmitterOption) *EmitterAdapter {
	e := &EmitterAdapter{
		out:    os.Stdout,
		errOut: os.Stderr,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// WaitDone retorna um canal que é fechado quando o streaming da conversa especificada
// termina (chat:stream com Done=true ou chat:error). Deve ser chamado ANTES de SendMessage.
// Se conversationID é 0, aceita qualquer conversa (compatível com modo REPL).
func (e *EmitterAdapter) WaitDone(conversationID uint) <-chan struct{} {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.done = make(chan struct{})
	e.conversationID = conversationID
	return e.done
}

// signalDone fecha o canal done se estiver aberto.
func (e *EmitterAdapter) signalDone() {
	if e.done != nil {
		select {
		case <-e.done:
			// já fechado
		default:
			close(e.done)
		}
	}
}

// Emit processa um evento do sistema e escreve no terminal quando relevante.
func (e *EmitterAdapter) Emit(event string, data any) {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch {
	case event == "chat:stream":
		e.handleStream(data)
	case event == "chat:error":
		e.handleError(data)
	case event == "chat:done":
		e.handleDone(data)
	case event == "chat:segment_done":
		e.handleSegmentDone(data)
	case strings.HasPrefix(event, "chat:tool_"):
		e.handleTool(event, data)
	default:
		if e.verbose {
			_, _ = fmt.Fprintf(e.errOut, "[event] %s\n", event)
		}
	}
}

// handleStream imprime tokens de streaming no stdout.
// Content chega acumulado: só imprimimos o delta em relação ao que já foi escrito.
func (e *EmitterAdapter) handleStream(data any) {
	ev, ok := e.toStreamEvent(data)
	if !ok {
		return
	}

	// Filtra eventos de outras conversas
	if e.conversationID != 0 && ev.ConversationId != 0 && ev.ConversationId != e.conversationID {
		return
	}

	if ev.Error != "" {
		_, _ = fmt.Fprintf(e.errOut, "\nErro: %s\n", ev.Error)
		e.lastPrinted = 0
		e.signalDone()
		return
	}

	if ev.Done {
		_, _ = fmt.Fprintln(e.out)
		e.lastPrinted = 0
		e.signalDone()
		return
	}

	// Content é acumulado; imprime só o que é novo.
	if len(ev.Content) > e.lastPrinted {
		_, _ = fmt.Fprint(e.out, ev.Content[e.lastPrinted:])
		e.lastPrinted = len(ev.Content)
	}
}

// handleError imprime erros no stderr.
func (e *EmitterAdapter) handleError(data any) {
	convID := e.errorConversationID(data)
	// Filtra eventos de outras conversas
	if e.conversationID != 0 && convID != 0 && convID != e.conversationID {
		return
	}

	switch v := data.(type) {
	case ports.ErrorEvent:
		if e.verbose && v.ConversationID != 0 {
			_, _ = fmt.Fprintf(e.errOut, "Erro: %s (conversationId=%d)\n", v.Error, v.ConversationID)
		} else {
			_, _ = fmt.Fprintf(e.errOut, "Erro: %s\n", v.Error)
		}
	case *ports.ErrorEvent:
		if v == nil {
			_, _ = fmt.Fprintln(e.errOut, "Erro: <nil>")
		} else if e.verbose && v.ConversationID != 0 {
			_, _ = fmt.Fprintf(e.errOut, "Erro: %s (conversationId=%d)\n", v.Error, v.ConversationID)
		} else {
			_, _ = fmt.Fprintf(e.errOut, "Erro: %s\n", v.Error)
		}
	default:
		_, _ = fmt.Fprintf(e.errOut, "Erro: %v\n", data)
	}
	e.lastPrinted = 0
	e.signalDone()
}

// handleTool imprime informações de tool calling quando verbose (AEP-0039 Fase 1).
// Em modo verbose exibe nome, origin e serverLabel de cada tool event.
func (e *EmitterAdapter) handleTool(event string, data any) {
	if !e.verbose {
		return
	}
	switch event {
	case "chat:tool_start":
		if ev, ok := e.toToolStartEvent(data); ok {
			if e.conversationID != 0 && ev.ConversationID != 0 && ev.ConversationID != e.conversationID {
				return
			}
			origin := ev.Origin
			if origin == "" && ev.Native {
				origin = "mcp_native"
			}
			if origin == "" {
				origin = "builtin"
			}
			label := origin
			if ev.ServerLabel != "" {
				label = origin + "/" + ev.ServerLabel
			}
			_, _ = fmt.Fprintf(e.errOut, "[tool:start] %s (%s)\n", ev.Name, label)
			return
		}
	case "chat:tool_end":
		if ev, ok := e.toToolEndEvent(data); ok {
			if e.conversationID != 0 && ev.ConversationID != 0 && ev.ConversationID != e.conversationID {
				return
			}
			name := ev.Name
			if name == "" {
				name = ev.CallID
			}
			status := ev.Status
			if status == "" {
				status = "ok"
			}
			if ev.DurationMs > 0 {
				_, _ = fmt.Fprintf(e.errOut, "[tool:end]   %s — %s (%dms)\n", name, status, ev.DurationMs)
			} else {
				_, _ = fmt.Fprintf(e.errOut, "[tool:end]   %s — %s\n", name, status)
			}
			return
		}
	case "chat:tool_failure":
		if ev, ok := e.toToolFailureEvent(data); ok {
			if e.conversationID != 0 && ev.ConversationID != 0 && ev.ConversationID != e.conversationID {
				return
			}
			retry := ""
			if ev.WillRetry {
				retry = " [retrying]"
			}
			_, _ = fmt.Fprintf(e.errOut, "[tool:failure] %s — %s (retryable=%v)%s\n", ev.Name, ev.ErrorKind, ev.Retryable, retry)
			return
		}
	}
	_, _ = fmt.Fprintf(e.errOut, "[tool] %s\n", event)
}

// toToolStartEvent converte o payload genérico para ports.ToolStartEvent.
func (e *EmitterAdapter) toToolStartEvent(data any) (ports.ToolStartEvent, bool) {
	switch v := data.(type) {
	case ports.ToolStartEvent:
		return v, true
	case *ports.ToolStartEvent:
		if v != nil {
			return *v, true
		}
	}
	return ports.ToolStartEvent{}, false
}

// toToolEndEvent converte o payload genérico para ports.ToolEndEvent.
func (e *EmitterAdapter) toToolEndEvent(data any) (ports.ToolEndEvent, bool) {
	switch v := data.(type) {
	case ports.ToolEndEvent:
		return v, true
	case *ports.ToolEndEvent:
		if v != nil {
			return *v, true
		}
	}
	return ports.ToolEndEvent{}, false
}

// toToolFailureEvent converte o payload genérico para ports.ToolFailureEvent.
func (e *EmitterAdapter) toToolFailureEvent(data any) (ports.ToolFailureEvent, bool) {
	switch v := data.(type) {
	case ports.ToolFailureEvent:
		return v, true
	case *ports.ToolFailureEvent:
		if v != nil {
			return *v, true
		}
	}
	return ports.ToolFailureEvent{}, false
}

// handleSegmentDone imprime resumo por iteração no modo padrão (AEP-0039 Fase 2).
// Em modo padrão: uma linha por iteração com contagem e nomes das tools.
// Em modo verbose: apenas loga o evento (tools individuais já são exibidos via tool_start/end).
func (e *EmitterAdapter) handleSegmentDone(data any) {
	ev, ok := e.toSegmentDoneEvent(data)
	if !ok {
		return
	}
	// Filtra eventos de outras conversas
	if e.conversationID != 0 && ev.ConversationID != 0 && ev.ConversationID != e.conversationID {
		return
	}
	// Só exibe linha de tools se é iteração intermediária com tools
	if !ev.HasMore || len(ev.ToolsInIteration) == 0 {
		return
	}

	n := len(ev.ToolsInIteration)
	toolWord := "tools"
	if n == 1 {
		toolWord = "tool"
	}

	// Exibe iteration como 1-based para UX humana (backend emite 0-based)
	displayIter := ev.Iteration + 1

	if e.verbose {
		// Verbose já exibe cada tool individualmente via tool_start/end;
		// apenas loga o segment_done como confirmação.
		_, _ = fmt.Fprintf(e.errOut, "[segment] iteração %d concluída, %d %s\n",
			displayIter, n, toolWord)
		return
	}

	// Modo padrão: linha compacta com nomes e duração total
	names := make([]string, 0, n)
	var totalMs int64
	for _, t := range ev.ToolsInIteration {
		names = append(names, t.Name)
		totalMs += t.DurationMs
	}
	nameList := strings.Join(names, ", ")

	if totalMs > 0 {
		_, _ = fmt.Fprintf(e.errOut, "[tools] iteração %d: %d %s (%s) — %dms\n",
			displayIter, n, toolWord, nameList, totalMs)
	} else {
		_, _ = fmt.Fprintf(e.errOut, "[tools] iteração %d: %d %s (%s)\n",
			displayIter, n, toolWord, nameList)
	}
}

// toSegmentDoneEvent converte o payload genérico para ports.SegmentDoneEvent.
func (e *EmitterAdapter) toSegmentDoneEvent(data any) (ports.SegmentDoneEvent, bool) {
	switch v := data.(type) {
	case ports.SegmentDoneEvent:
		return v, true
	case *ports.SegmentDoneEvent:
		if v != nil {
			return *v, true
		}
	case map[string]any:
		b, err := json.Marshal(v)
		if err != nil {
			return ports.SegmentDoneEvent{}, false
		}
		var ev ports.SegmentDoneEvent
		if err := json.Unmarshal(b, &ev); err != nil {
			return ports.SegmentDoneEvent{}, false
		}
		return ev, true
	}
	return ports.SegmentDoneEvent{}, false
}

// handleDone imprime resumo do chat:done no stderr (AEP-0039 Fase 2).
func (e *EmitterAdapter) handleDone(data any) {
	ev, ok := e.toDoneEvent(data)
	if !ok {
		return
	}
	// Filtra eventos de outras conversas
	if e.conversationID != 0 && ev.ConversationID != 0 && ev.ConversationID != e.conversationID {
		return
	}
	// Só exibe resumo se houve tool calls (evita ruído em respostas simples).
	// Usa HadToolCalls como fallback para eventos backward-compatible que não
	// trazem LoopStats/contadores preenchidos.
	reason := ev.Reason
	if reason == "" {
		reason = "completed"
	}

	showSummary := ev.ToolCallCount > 0 || ev.HadToolCalls || ev.Reason == "limit_reached" || ev.Reason == "error"
	if showSummary {
		if ev.IterationCount > 0 || ev.ToolCallCount > 0 {
			_, _ = fmt.Fprintf(e.errOut, "[done] %d iterações, %d tool calls, %s\n",
				ev.IterationCount, ev.ToolCallCount, reason)
			return
		}
		if ev.HadToolCalls {
			_, _ = fmt.Fprintf(e.errOut, "[done] tool calls executadas, %s\n", reason)
			return
		}
		_, _ = fmt.Fprintf(e.errOut, "[done] %s\n", reason)
	} else if e.verbose {
		_, _ = fmt.Fprintf(e.errOut, "[done] %s\n", reason)
	}
	e.signalDone()
}

// toDoneEvent converte o payload genérico para ports.DoneEvent.
func (e *EmitterAdapter) toDoneEvent(data any) (ports.DoneEvent, bool) {
	switch v := data.(type) {
	case ports.DoneEvent:
		return v, true
	case *ports.DoneEvent:
		if v != nil {
			return *v, true
		}
	case map[string]any:
		// Fallback: desserializa de map (caso venha como JSON decoded)
		b, err := json.Marshal(v)
		if err != nil {
			return ports.DoneEvent{}, false
		}
		var ev ports.DoneEvent
		if err := json.Unmarshal(b, &ev); err != nil {
			return ports.DoneEvent{}, false
		}
		return ev, true
	}
	return ports.DoneEvent{}, false
}

// toStreamEvent converte o payload genérico para StreamEvent.
func (e *EmitterAdapter) toStreamEvent(data any) (ports.StreamEvent, bool) {
	switch v := data.(type) {
	case ports.StreamEvent:
		return v, true
	case *ports.StreamEvent:
		if v != nil {
			return *v, true
		}
	case map[string]any:
		// Fallback: desserializa de map (caso venha como JSON decoded)
		b, err := json.Marshal(v)
		if err != nil {
			return ports.StreamEvent{}, false
		}
		var ev ports.StreamEvent
		if err := json.Unmarshal(b, &ev); err != nil {
			return ports.StreamEvent{}, false
		}
		return ev, true
	}
	return ports.StreamEvent{}, false
}

// errorConversationID extrai o ConversationID do payload de erro, se disponível.
func (e *EmitterAdapter) errorConversationID(data any) uint {
	switch v := data.(type) {
	case ports.ErrorEvent:
		return v.ConversationID
	case *ports.ErrorEvent:
		if v != nil {
			return v.ConversationID
		}
	}
	return 0
}
