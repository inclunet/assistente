// Package wails contém os Outbound Adapters concretos para o runtime do Wails v3.
// Este pacote é o único lugar onde "github.com/wailsapp/wails/v3/pkg/application"
// deve ser importado fora do pacote main.
package wails

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// WindowAdapter implementa ports.WindowPort usando o runtime do Wails v3.
type WindowAdapter struct{}

// NewWindowAdapter cria um WindowAdapter.
func NewWindowAdapter() *WindowAdapter {
	return &WindowAdapter{}
}

// Show torna a janela visível e a traz para o primeiro plano.
func (a *WindowAdapter) Show() {
	application.Get().Show()
}
