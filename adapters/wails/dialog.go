package wails

import (
	"context"

	"assistente/internal/core/ports"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// DialogAdapter implementa ports.SystemDialogPort usando o runtime do Wails.
type DialogAdapter struct {
	ctx context.Context
}

// NewDialogAdapter cria um DialogAdapter a partir do contexto Wails.
func NewDialogAdapter(ctx context.Context) *DialogAdapter {
	return &DialogAdapter{ctx: ctx}
}

// SetContext atualiza o contexto Wails (chamado em OnDomReady/OnStartup).
func (a *DialogAdapter) SetContext(ctx context.Context) {
	a.ctx = ctx
}

// OpenFileDialog exibe o diálogo nativo de seleção de arquivo.
func (a *DialogAdapter) OpenFileDialog(opts ports.OpenFileOptions) (string, error) {
	filters := make([]wailsruntime.FileFilter, len(opts.Filters))
	for i, f := range opts.Filters {
		filters[i] = wailsruntime.FileFilter{
			DisplayName: f.DisplayName,
			Pattern:     f.Pattern,
		}
	}
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:   opts.Title,
		Filters: filters,
	})
}

// SaveFileDialog exibe o diálogo nativo de salvar arquivo.
func (a *DialogAdapter) SaveFileDialog(opts ports.SaveFileOptions) (string, error) {
	filters := make([]wailsruntime.FileFilter, len(opts.Filters))
	for i, f := range opts.Filters {
		filters[i] = wailsruntime.FileFilter{
			DisplayName: f.DisplayName,
			Pattern:     f.Pattern,
		}
	}
	return wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           opts.Title,
		DefaultFilename: opts.DefaultFilename,
		Filters:         filters,
	})
}
