package filesystem

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// EnsureParentDir garante que o diretório pai de path exista.
func EnsureParentDir(path string) error {
	parent := filepath.Dir(path)
	if parent == "." || parent == "" {
		return nil
	}
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("falha ao criar diretório pai: %w", err)
	}
	return nil
}

// ReadFileBytes lê um arquivo e retorna bytes.
func ReadFileBytes(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// WriteFileBytes escreve bytes em um arquivo. Cria diretórios intermediários automaticamente.
func WriteFileBytes(path string, content []byte, perm fs.FileMode) error {
	if err := EnsureParentDir(path); err != nil {
		return err
	}
	if err := os.WriteFile(path, content, perm); err != nil {
		return err
	}
	return nil
}
