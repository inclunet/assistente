package wails

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// EmitterAdapter implementa ports.Emitter delegando para runtime.EventsEmit.
type EmitterAdapter struct {
	ctx context.Context
}

// NewEmitterAdapter cria um EmitterAdapter a partir do contexto Wails.
func NewEmitterAdapter(ctx context.Context) *EmitterAdapter {
	return &EmitterAdapter{ctx: ctx}
}

// SetContext atualiza o contexto Wails (chamado em OnDomReady/OnStartup).
func (e *EmitterAdapter) SetContext(ctx context.Context) {
	e.ctx = ctx
}

// Emit emite um evento para o frontend via Wails runtime.
func (e *EmitterAdapter) Emit(event string, data any) {
	runtime.EventsEmit(e.ctx, event, data)
}
