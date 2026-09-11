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
	mu              sync.Mutex
	out             io.Writer // stdout por padrão
	errOut          io.Writer // stderr por padrão
	verbose         bool
	locale          string
	done            chan struct{} // sinaliza fim do streaming (chat:stream Done=true ou chat:error)
	conversationID  string        // conversa ativa; "" = aceita qualquer conversa
	streamSequence  int64
	streamActive    bool
	streamHasOutput bool
}

// EmitterOption configura o EmitterAdapter.
type EmitterOption func(*EmitterAdapter)

// WithVerbose habilita logging de todos os eventos no stderr.
func WithVerbose(v bool) EmitterOption {
	return func(e *EmitterAdapter) { e.verbose = v }
}

// WithLocale define o idioma das mensagens próprias do adapter.
func WithLocale(locale string) EmitterOption {
	return func(e *EmitterAdapter) { e.locale = normalizeCLILocale(locale) }
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
		locale: detectCLILocale(),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// WaitDone retorna um canal que é fechado quando o backend sinaliza término:
//   - chat:done (fonte de verdade — emitido pelo fluxo normal)
//   - chat:error (fallback — cobre erros pré-streaming sem chat:done)
//   - chat:stream com Error (fallback — cobre HandlePanic e paths que não emitem chat:done)
//
// signalDone() é idempotente: se mais de um evento terminal chegar, o canal já estará fechado.
// Deve ser chamado ANTES de SendMessage.
// Se conversationID é "", aceita qualquer conversa (compatível com modo REPL).
func (e *EmitterAdapter) WaitDone(conversationID string) <-chan struct{} {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.done = make(chan struct{})
	e.conversationID = conversationID
	e.resetStreamState()
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
	case event == "chat:notice":
		e.handleNotice(data)
	case strings.HasPrefix(event, "chat:tool_"):
		e.handleTool(event, data)
	default:
		if e.verbose {
			_, _ = fmt.Fprintf(e.errOut, "[event] %s\n", event)
		}
	}
}

func (e *EmitterAdapter) handleNotice(data any) {
	ev, ok := e.toNoticeEvent(data)
	if !ok || (e.conversationID != "" && ev.ConversationID != e.conversationID) {
		return
	}
	if ev.Kind == "stream_retry" {
		_, _ = fmt.Fprintln(e.errOut, localizedStreamRetryNotice(e.locale, ev.Count))
	}
}

// handleStream imprime os deltas de streaming no stdout.
func (e *EmitterAdapter) handleStream(data any) {
	ev, ok := e.toStreamEvent(data)
	if !ok {
		return
	}

	// Quando há uma conversa em espera, aceita somente eventos explicitamente
	// correlacionados a ela. ConversationId vazio viola o contrato do stream.
	if e.conversationID != "" && ev.ConversationId != e.conversationID {
		return
	}

	if ev.Error != "" {
		_, _ = fmt.Fprintf(e.errOut, "\n%s: %s\n", localizedErrorPrefix(e.locale), readableChatError(ev.Error, e.locale))
		e.resetStreamState()
		// Fallback: sinaliza done em chat:stream com Error porque há caminhos
		// no backend (ex.: HandlePanic) que emitem apenas chat:stream terminal
		// sem emitir chat:done. signalDone() é idempotente.
		e.signalDone()
		return
	}

	if ev.Done {
		_, _ = fmt.Fprintln(e.out)
		e.resetStreamState()
		// NÃO chama signalDone aqui: o fluxo normal emite chat:done após
		// chat:stream Done=true, e signalDone fica com chat:done para garantir
		// que o CLI processe o resumo final antes de encerrar.
		return
	}

	if ev.Reset {
		if ev.Sequence != 0 {
			return
		}
		if e.streamActive && e.streamHasOutput {
			// stdout não pode apagar uma tentativa já exibida. Uma nova linha
			// separa o retry e evita concatená-lo como se fosse continuação.
			_, _ = fmt.Fprintln(e.out)
		}
		e.streamSequence = -1
		e.streamActive = true
		e.streamHasOutput = ev.BaseContent != ""
		if ev.BaseContent != "" {
			_, _ = fmt.Fprint(e.out, ev.BaseContent)
		}
	} else if !e.streamActive || int64(ev.Sequence) != e.streamSequence+1 {
		return
	}
	e.streamSequence = int64(ev.Sequence)
	if ev.Delta != "" {
		e.streamHasOutput = true
		_, _ = fmt.Fprint(e.out, ev.Delta)
	}
}

func (e *EmitterAdapter) resetStreamState() {
	e.streamSequence = -1
	e.streamActive = false
	e.streamHasOutput = false
}

// handleError imprime erros no stderr.
func (e *EmitterAdapter) handleError(data any) {
	convID := e.errorConversationID(data)
	// Filtra eventos de outras conversas
	if e.conversationID != "" && convID != "" && convID != e.conversationID {
		return
	}

	switch v := data.(type) {
	case ports.ErrorEvent:
		if e.verbose && v.ConversationID != "" {
			_, _ = fmt.Fprintf(e.errOut, "Erro: %s (conversationId=%s)\n", v.Error, v.ConversationID)
		} else {
			_, _ = fmt.Fprintf(e.errOut, "Erro: %s\n", v.Error)
		}
	case *ports.ErrorEvent:
		if v == nil {
			_, _ = fmt.Fprintln(e.errOut, "Erro: <nil>")
		} else if e.verbose && v.ConversationID != "" {
			_, _ = fmt.Fprintf(e.errOut, "Erro: %s (conversationId=%s)\n", v.Error, v.ConversationID)
		} else {
			_, _ = fmt.Fprintf(e.errOut, "Erro: %s\n", v.Error)
		}
	default:
		_, _ = fmt.Fprintf(e.errOut, "Erro: %v\n", data)
	}
	// Fallback: sinaliza done em chat:error para cobrir caminhos onde o backend
	// não emite chat:done (erro pré-streaming, sem provedor, etc.).
	// Na prática chat:error e chat:done são mutuamente exclusivos: chat:error é
	// emitido em paths que retornam antes de entrar no agent loop (que emite chat:done).
	// Se chat:done eventualmente chegar, signalDone() é idempotente (canal já fechado).
	e.resetStreamState()
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
			if e.conversationID != "" && ev.ConversationID != "" && ev.ConversationID != e.conversationID {
				return
			}
			origin := ev.Origin
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
			if e.conversationID != "" && ev.ConversationID != "" && ev.ConversationID != e.conversationID {
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
			origin := ev.Origin
			if origin == "" {
				origin = "builtin"
			}
			label := origin
			if ev.ServerLabel != "" {
				label = origin + "/" + ev.ServerLabel
			}
			if ev.DurationMs > 0 {
				_, _ = fmt.Fprintf(e.errOut, "[tool:end]   %s (%s) — %s (%dms)\n", name, label, status, ev.DurationMs)
			} else {
				_, _ = fmt.Fprintf(e.errOut, "[tool:end]   %s (%s) — %s\n", name, label, status)
			}
			return
		}
	case "chat:tool_failure":
		if ev, ok := e.toToolFailureEvent(data); ok {
			if e.conversationID != "" && ev.ConversationID != "" && ev.ConversationID != e.conversationID {
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
	if e.conversationID != "" && ev.ConversationID != "" && ev.ConversationID != e.conversationID {
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
	// Filtra eventos de outras conversas — NÃO sinaliza done para conversas
	// que não são a ativa (evita fechar WaitDone prematuramente).
	if e.conversationID != "" && ev.ConversationID != "" && ev.ConversationID != e.conversationID {
		return
	}
	defer e.signalDone()

	// chat:done com ErrorMessage: exibe erro (substitui chat:stream terminal)
	if ev.ErrorMessage != "" {
		_, _ = fmt.Fprintf(e.errOut, "\n%s: %s\n", localizedErrorPrefix(e.locale), readableChatError(ev.ErrorMessage, e.locale))
		e.resetStreamState()
	}

	// Só exibe resumo se houve tool calls (evita ruído em respostas simples).
	// Usa HadToolCalls como fallback para eventos backward-compatible que não
	// trazem LoopStats/contadores preenchidos.
	reason := ev.Reason
	if reason == "" {
		reason = "completed"
	}

	showSummary := ev.ToolCallCount > 0 || ev.HadToolCalls ||
		ev.Reason == "limit_reached" || ev.Reason == "output_limit" || ev.Reason == "error"
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
}

func readableChatError(message, locale string) string {
	catalog := map[string]map[string]string{
		"en": {
			"streaming_interrupted":  "Response interrupted by provider without finish reason; please try again.",
			"streaming_idle_timeout": "Provider stopped responding mid-generation (idle timeout).",
		},
		"es": {
			"streaming_interrupted":  "Respuesta interrumpida por el proveedor sin motivo de finalización; inténtelo de nuevo.",
			"streaming_idle_timeout": "El proveedor dejó de responder a mitad de la generación (timeout de inactividad).",
		},
		"pt-BR": {
			"streaming_interrupted":  "Resposta interrompida pelo provedor sem motivo de finalização; tente novamente.",
			"streaming_idle_timeout": "O provedor parou de responder no meio da geração (timeout de inatividade).",
		},
	}
	if translated := catalog[normalizeCLILocale(locale)][message]; translated != "" {
		return translated
	}
	return message
}

func localizedErrorPrefix(locale string) string {
	switch normalizeCLILocale(locale) {
	case "pt-BR":
		return "Erro"
	case "es":
		return "Error"
	default:
		return "Error"
	}
}

func localizedStreamRetryNotice(locale string, count int) string {
	switch normalizeCLILocale(locale) {
	case "pt-BR":
		return fmt.Sprintf("A conexão com o provedor falhou na tentativa %d. Tentando de novo…", count)
	case "es":
		return fmt.Sprintf("La conexión con el proveedor falló en el intento %d. Reintentando…", count)
	default:
		return fmt.Sprintf("The connection to the provider failed on attempt %d. Retrying…", count)
	}
}

func detectCLILocale() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return normalizeCLILocale(value)
		}
	}
	return "en"
}

func normalizeCLILocale(locale string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	switch {
	case strings.HasPrefix(locale, "pt"):
		return "pt-BR"
	case strings.HasPrefix(locale, "es"):
		return "es"
	default:
		return "en"
	}
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

func (e *EmitterAdapter) toNoticeEvent(data any) (ports.ChatNoticeEvent, bool) {
	switch v := data.(type) {
	case ports.ChatNoticeEvent:
		return v, true
	case *ports.ChatNoticeEvent:
		if v != nil {
			return *v, true
		}
	case map[string]any:
		b, err := json.Marshal(v)
		if err != nil {
			return ports.ChatNoticeEvent{}, false
		}
		var ev ports.ChatNoticeEvent
		if err := json.Unmarshal(b, &ev); err != nil {
			return ports.ChatNoticeEvent{}, false
		}
		return ev, true
	}
	return ports.ChatNoticeEvent{}, false
}

// errorConversationID extrai o ConversationID do payload de erro, se disponível.
func (e *EmitterAdapter) errorConversationID(data any) string {
	switch v := data.(type) {
	case ports.ErrorEvent:
		return v.ConversationID
	case *ports.ErrorEvent:
		if v != nil {
			return v.ConversationID
		}
	}
	return ""
}
