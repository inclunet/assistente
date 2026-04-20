package cli

import (
	"fmt"

	"assistente/internal/core/ports"
)

// DialogAdapter implementa ports.SystemDialogPort para o modo CLI.
// Diálogos de arquivo não são suportados interativamente no CLI —
// retornam erro indicando que o caminho deve ser passado via argumento ou flag do comando.
type DialogAdapter struct{}

// OpenFileDialog retorna erro no CLI — informe o caminho via argumento ou flag do comando.
func (DialogAdapter) OpenFileDialog(_ ports.OpenFileOptions) (string, error) {
	return "", fmt.Errorf("diálogos de arquivo não disponíveis no modo CLI; informe o caminho via argumento ou flag do comando")
}

// SaveFileDialog retorna erro no CLI — informe o caminho via argumento ou flag do comando.
func (DialogAdapter) SaveFileDialog(_ ports.SaveFileOptions) (string, error) {
	return "", fmt.Errorf("diálogos de arquivo não disponíveis no modo CLI; informe o caminho via argumento ou flag do comando")
}
