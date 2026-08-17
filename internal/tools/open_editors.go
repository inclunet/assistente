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
// Usado para UX (confirmação de diff no documento aberto, etc.). Desde o AEP-0092
// NÃO libera path fora do sandbox — isso passa por fstrust / DecisionDialog.
func WithOpenEditorPaths(ctx context.Context, paths []string) context.Context {
	// Copia o slice para evitar aliasing se o caller mutar o original.
	cp := make([]string, len(paths))
	copy(cp, paths)
	return context.WithValue(ctx, openEditorPathsKey{}, cp)
}

// GetOpenEditorPaths retorna os caminhos de arquivos abertos em abas de editor, se existirem no ctx.
// Retorna uma cópia defensiva para impedir que callers mutem o slice armazenado no context.
func GetOpenEditorPaths(ctx context.Context) []string {
	v := ctx.Value(openEditorPathsKey{})
	paths, ok := v.([]string)
	if !ok {
		return nil
	}
	cp := make([]string, len(paths))
	copy(cp, paths)
	return cp
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
