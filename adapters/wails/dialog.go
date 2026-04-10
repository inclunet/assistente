package wails

import (
	"assistente/internal/core/ports"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// DialogAdapter implementa ports.SystemDialogPort usando o runtime do Wails v3.
type DialogAdapter struct{}

// NewDialogAdapter cria um DialogAdapter.
func NewDialogAdapter() *DialogAdapter {
	return &DialogAdapter{}
}

// OpenFileDialog exibe o diálogo nativo de seleção de arquivo.
func (a *DialogAdapter) OpenFileDialog(opts ports.OpenFileOptions) (string, error) {
	d := application.Get().Dialog.OpenFile()
	if opts.Title != "" {
		d.SetTitle(opts.Title)
	}
	for _, f := range opts.Filters {
		d.AddFilter(f.DisplayName, f.Pattern)
	}
	d.CanChooseFiles(true)
	return d.PromptForSingleSelection()
}

// SaveFileDialog exibe o diálogo nativo de salvar arquivo.
func (a *DialogAdapter) SaveFileDialog(opts ports.SaveFileOptions) (string, error) {
	filters := make([]application.FileFilter, len(opts.Filters))
	for i, f := range opts.Filters {
		filters[i] = application.FileFilter{
			DisplayName: f.DisplayName,
			Pattern:     f.Pattern,
		}
	}
	d := application.Get().Dialog.SaveFile()
	d.SetOptions(&application.SaveFileDialogOptions{
		Title:    opts.Title,
		Filename: opts.DefaultFilename,
		Filters:  filters,
	})
	return d.PromptForSingleSelection()
}
