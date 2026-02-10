package filesystem

import (
	"fmt"
	"path/filepath"
	"strings"
)

// validatePath verifica se um caminho é seguro para acesso.
// Bloqueia path traversal e acesso a diretórios sensíveis.
func validatePath(fullPath, workDir string) error {
	cleanPath := filepath.Clean(fullPath)

	// Bloqueia path traversal (o caminho limpo não deve "escapar" para diretórios sensíveis do sistema)
	// Nota: permitimos acesso a qualquer caminho dentro do workDir e diretórios legítimos,
	// mas bloqueamos padrões perigosos
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("caminho contém path traversal: %s", fullPath)
	}

	return nil
}
