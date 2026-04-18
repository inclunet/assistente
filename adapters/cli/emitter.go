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
	mu      sync.Mutex
	out     io.Writer // stdout por padrão
	errOut  io.Writer // stderr por padrão
	verbose bool
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

// Emit processa um evento do sistema e escreve no terminal quando relevante.
func (e *EmitterAdapter) Emit(event string, data any) {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch {
	case event == "chat:stream":
		e.handleStream(data)
	case event == "chat:error":
		e.handleError(data)
	case strings.HasPrefix(event, "chat:tool:"):
		e.handleTool(event, data)
	default:
		if e.verbose {
			fmt.Fprintf(e.errOut, "[event] %s\n", event)
		}
	}
}

// handleStream imprime tokens de streaming no stdout.
func (e *EmitterAdapter) handleStream(data any) {
	ev, ok := e.toStreamEvent(data)
	if !ok {
		return
	}

	if ev.Error != "" {
		fmt.Fprintf(e.errOut, "\nErro: %s\n", ev.Error)
		return
	}

	if ev.Done {
		fmt.Fprintln(e.out)
		return
	}

	fmt.Fprint(e.out, ev.Content)
}

// handleError imprime erros no stderr.
func (e *EmitterAdapter) handleError(data any) {
	fmt.Fprintf(e.errOut, "Erro: %v\n", data)
}

// handleTool imprime informações de tool calling quando verbose.
func (e *EmitterAdapter) handleTool(event string, data any) {
	if !e.verbose {
		return
	}
	fmt.Fprintf(e.errOut, "[tool] %s\n", event)
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
