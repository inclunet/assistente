package shell

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"assistente/internal/terminal"
)

var errProjectDirectoryUnresolved = errors.New("não foi possível resolver o diretório do projeto")

func resolveProjectWorkDir(projectRoot, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if strings.IndexByte(requested, 0) >= 0 {
		return "", errors.New("o caminho contém um caractere NUL")
	}
	if filepath.IsAbs(requested) || looksLikeForeignAbsolutePath(requested) {
		return "", fmt.Errorf("working_directory deve ser relativo ao diretório do projeto")
	}

	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errProjectDirectoryUnresolved, directoryDiagnostic(err))
	}
	canonicalRoot, err := existingDirectoryPath(root)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errProjectDirectoryUnresolved, directoryDiagnostic(err))
	}
	if requested == "" || requested == "." {
		return canonicalRoot, nil
	}
	target := filepath.Join(root, filepath.Clean(requested))
	canonicalTarget, err := existingDirectoryPath(target)
	if err != nil {
		return "", directoryDiagnostic(err)
	}

	relative, err := filepath.Rel(canonicalRoot, canonicalTarget)
	if err != nil {
		return "", errors.New("não é possível relacionar o caminho ao diretório do projeto")
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("working_directory deve permanecer dentro do diretório do projeto")
	}
	return canonicalTarget, nil
}

func existingDirectoryPath(path string) (string, error) {
	resolved, err := terminal.ResolveDirectoryPath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errNotDirectory
	}
	return resolved, nil
}

var errNotDirectory = errors.New("o caminho não é um diretório")

// O resultado da ferramenta pode ser persistido. Erros do sistema de arquivos
// frequentemente carregam caminhos absolutos do host junto com a causa.
func directoryDiagnostic(err error) error {
	switch {
	case errors.Is(err, errNotDirectory):
		return errNotDirectory
	case errors.Is(err, os.ErrPermission):
		return errors.New("permissão insuficiente para acessar o diretório")
	case errors.Is(err, os.ErrNotExist):
		return errors.New("o diretório ou destino do link simbólico não existe")
	case errors.Is(err, os.ErrInvalid):
		return errors.New("o caminho contém um componente inválido")
	default:
		return errors.New("não foi possível resolver o caminho informado")
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
