package shell

import (
	"fmt"
	"os"
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

	canonicalRoot, err := canonicalizeProjectPath(root)
	if err != nil {
		return "", fmt.Errorf("não foi possível canonicalizar o diretório do projeto: %w", err)
	}
	canonicalTarget, err := canonicalizeProjectPath(target)
	if err != nil {
		return "", fmt.Errorf("não foi possível canonicalizar working_directory: %w", err)
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

// canonicalizeProjectPath resolves the existing portion of a path and then
// appends any non-existing suffix. EvalSymlinks cannot resolve a target that
// has not been created yet; using its lexical spelling in that case can mix a
// canonical long Windows path with an 8.3 alias from the other operand.
func canonicalizeProjectPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	missing := make([]string, 0, 2)
	current := filepath.Clean(absolute)
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(resolveErr) {
			return "", resolveErr
		}
		if info, lstatErr := os.Lstat(current); lstatErr == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("não foi possível resolver symlink em working_directory: %w", resolveErr)
			}
		} else if !os.IsNotExist(lstatErr) {
			return "", lstatErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", resolveErr
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func looksLikeForeignAbsolutePath(path string) bool {
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") {
		return true
	}
	return len(path) >= 2 &&
		((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) &&
		path[1] == ':'
}
