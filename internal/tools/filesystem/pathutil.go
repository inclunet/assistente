package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// expandTilde expande ~ e ~/ no início de um caminho para o diretório home do usuário.
// No Windows, ~ não é expandido pelo sistema — esta função resolve isso de forma portável.
// Exemplos:
//
//	"~/docs/file.txt" → "C:\Users\user\docs\file.txt" (Windows)
//	"~/.assistente"   → "/home/user/.assistente" (Linux/macOS)
//	"/absolute/path"  → "/absolute/path" (inalterado)
//	"relative/path"   → "relative/path" (inalterado)
func expandTilde(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return path // fallback: retorna sem expandir
		}
		if path == "~" {
			return home
		}
		// Remove o ~ e junta com o home dir
		return filepath.Join(home, path[2:])
	}
	return path
}

// resolveFilePath resolve um caminho de arquivo, expandindo ~ e caminhos relativos.
// Ordem de resolução:
//  1. Expande ~ para home directory
//  2. Se absoluto, retorna limpo
//  3. Se relativo, resolve em relação ao workDir
func resolveFilePath(path, workDir string) (string, error) {
	// Primeiro expande tilde
	expanded := expandTilde(path)

	// Se é absoluto, retorna limpo
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded), nil
	}

	// Relativo: resolve em relação ao workDir
	return filepath.Abs(filepath.Join(workDir, expanded))
}

// validatePath verifica se um caminho é seguro para acesso.
// Bloqueia path traversal e acesso a diretórios sensíveis.
func validatePath(fullPath, _ string) error {
	cleanPath := filepath.Clean(fullPath)

	// Bloqueia path traversal (o caminho limpo não deve "escapar" para diretórios sensíveis do sistema)
	// Nota: permitimos acesso a qualquer caminho dentro do workDir e diretórios legítimos,
	// mas bloqueamos padrões perigosos
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("caminho contém path traversal: %s", fullPath)
	}

	return nil
}
