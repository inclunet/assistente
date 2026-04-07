package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// appEmitter implementa events.Emitter delegando para runtime.EventsEmit.
// Vive no pacote main para isolar a dependência do Wails neste layer.
type appEmitter struct {
	ctx context.Context
}

func (e appEmitter) Emit(event string, data any) {
	runtime.EventsEmit(e.ctx, event, data)
}
