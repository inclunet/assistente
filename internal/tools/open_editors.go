package tools

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
)

// openEditorPathsKey é a chave de contexto para caminhos de arquivos abertos em abas de editor.
type openEditorPathsKey struct{}

// WithOpenEditorPaths injeta os caminhos de arquivos abertos em abas de editor no ctx.
// Esses caminhos são usados como exceção na validação de paths de filesystem tools:
// arquivos abertos no editor podem ser lidos/editados mesmo fora do workDir.
func WithOpenEditorPaths(ctx context.Context, paths []string) context.Context {
	return context.WithValue(ctx, openEditorPathsKey{}, paths)
}

// GetOpenEditorPaths retorna os caminhos de arquivos abertos em abas de editor, se existirem no ctx.
func GetOpenEditorPaths(ctx context.Context) []string {
	v := ctx.Value(openEditorPathsKey{})
	paths, ok := v.([]string)
	if !ok {
		return nil
	}
	return paths
}

// IsOpenEditorFile verifica se fullPath (absoluto) corresponde a um arquivo aberto em aba de editor.
func IsOpenEditorFile(ctx context.Context, fullPath string) bool {
	paths := GetOpenEditorPaths(ctx)
	if len(paths) == 0 {
		return false
	}

	cleanPath := filepath.Clean(fullPath)
	if runtime.GOOS == "windows" {
		cleanPath = strings.ToLower(cleanPath)
	}

	for _, p := range paths {
		candidate := filepath.Clean(p)
		if runtime.GOOS == "windows" {
			candidate = strings.ToLower(candidate)
		}
		if cleanPath == candidate {
			return true
		}
	}
	return false
}
