package cli

import (
	"fmt"

	"assistente/internal/core/ports"
)

// DialogAdapter implementa ports.SystemDialogPort para o modo CLI.
// Diálogos de arquivo não são suportados interativamente no CLI —
// retornam erro indicando que o caminho deve ser passado via flag.
type DialogAdapter struct{}

// OpenFileDialog retorna erro no CLI — use flags para especificar caminhos.
func (DialogAdapter) OpenFileDialog(_ ports.OpenFileOptions) (string, error) {
	return "", fmt.Errorf("diálogos de arquivo não disponíveis no modo CLI; use --file para especificar o caminho")
}

// SaveFileDialog retorna erro no CLI — use flags para especificar caminhos.
func (DialogAdapter) SaveFileDialog(_ ports.SaveFileOptions) (string, error) {
	return "", fmt.Errorf("diálogos de arquivo não disponíveis no modo CLI; use --output para especificar o caminho")
}
