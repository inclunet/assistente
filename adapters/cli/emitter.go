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

// handleTool imprime informações de tool calling quando verbose.
func (e *EmitterAdapter) handleTool(event string, data any) {
	if !e.verbose {
		return
	}
	_, _ = fmt.Fprintf(e.errOut, "[tool] %s\n", event)
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
