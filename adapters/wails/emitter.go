// Package wails contém os Outbound Adapters concretos para o runtime do Wails v3.
// Este pacote é o único lugar onde "github.com/wailsapp/wails/v3/pkg/application"
// deve ser importado fora do pacote main.
package wails

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// EmitterAdapter implementa ports.Emitter delegando para application.Get().Event.Emit.
type EmitterAdapter struct{}

// NewEmitterAdapter cria um EmitterAdapter.
func NewEmitterAdapter() *EmitterAdapter {
	return &EmitterAdapter{}
}

// Emit emite um evento para o frontend via Wails v3 runtime.
func (e *EmitterAdapter) Emit(event string, data any) {
	application.Get().Event.Emit(event, data)
}
