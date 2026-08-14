package shell

import (
	"fmt"
	"path/filepath"
	"strings"
)

func resolveProjectWorkDir(projectRoot, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return projectRoot, nil
	}
	if filepath.IsAbs(requested) || looksLikeForeignAbsolutePath(requested) {
		return "", fmt.Errorf("working_directory deve ser relativo ao diretório do projeto")
	}

	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", fmt.Errorf("não foi possível resolver o diretório do projeto: %w", err)
	}
	target, err := filepath.Abs(filepath.Join(root, filepath.Clean(requested)))
	if err != nil {
		return "", fmt.Errorf("não foi possível resolver working_directory: %w", err)
	}

	// Quando os caminhos existem, resolve symlinks para impedir escapes indiretos.
	canonicalRoot := root
	if resolved, resolveErr := filepath.EvalSymlinks(root); resolveErr == nil {
		canonicalRoot = resolved
	}
	canonicalTarget := target
	if resolved, resolveErr := filepath.EvalSymlinks(target); resolveErr == nil {
		canonicalTarget = resolved
	}

	relative, err := filepath.Rel(canonicalRoot, canonicalTarget)
	if err != nil {
		return "", fmt.Errorf("working_directory inválido: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("working_directory deve permanecer dentro do diretório do projeto")
	}
	return canonicalTarget, nil
}

func looksLikeForeignAbsolutePath(path string) bool {
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") {
		return true
	}
	return len(path) >= 2 &&
		((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) &&
		path[1] == ':'
}
