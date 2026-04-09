// Package wails contém os Outbound Adapters concretos para o runtime do Wails.
// Este pacote é o único lugar onde "github.com/wailsapp/wails/v2/pkg/runtime"
// deve ser importado fora do pacote main.
package wails

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// WindowAdapter implementa ports.WindowPort usando o runtime do Wails.
type WindowAdapter struct {
	ctx context.Context
}

// NewWindowAdapter cria um WindowAdapter a partir do contexto Wails.
func NewWindowAdapter(ctx context.Context) *WindowAdapter {
	return &WindowAdapter{ctx: ctx}
}

// SetContext atualiza o contexto Wails (chamado em OnDomReady/OnStartup).
func (a *WindowAdapter) SetContext(ctx context.Context) {
	a.ctx = ctx
}

// Show torna a janela visível e a traz para o primeiro plano.
func (a *WindowAdapter) Show() {
	runtime.WindowShow(a.ctx)
}
