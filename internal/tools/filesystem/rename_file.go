package filesystem

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"assistente/internal/configdir"
)

// MoveFileWithPolicy move/renomeia um arquivo no disco aplicando uma policy.
// Retorna erro se o destino já existir (a não ser que overwrite=true).
// Em caso de falha de os.Rename (ex: cross-device), tenta fallback de copiar+remover.
func MoveFileWithPolicy(oldPath string, newPath string, overwrite bool, policy Policy) error {
	src := strings.TrimSpace(oldPath)
	dst := strings.TrimSpace(newPath)
	if src == "" {
		return fmt.Errorf("oldPath vazio")
	}
	if dst == "" {
		return fmt.Errorf("newPath vazio")
	}
	if src == dst {
		return nil
	}

	// Bloqueia manipulação de arquivos sensíveis (toolcalling)
	if policy.BlockSensitive {
		if isSensitiveFile(src) || isSensitiveFile(dst) {
			return fmt.Errorf("não é permitido mover/renomear arquivos sensíveis")
		}
	}

	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("arquivo não encontrado: %s", oldPath)
		}
		return fmt.Errorf("erro ao acessar arquivo: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("'%s' é um diretório, não um arquivo", oldPath)
	}

	if !overwrite {
		if _, err := os.Stat(dst); err == nil {
			return fmt.Errorf("já existe um arquivo no destino")
		}
	}

	if err := EnsureParentDir(dst); err != nil {
		return fmt.Errorf("falha ao criar diretório de destino: %w", err)
	}

	// Tenta renomear/mover diretamente
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else {
		// Fallback: copiar + remover (ex: cross-device)
		in, openErr := os.Open(src)
		if openErr != nil {
			return fmt.Errorf("falha ao mover arquivo: %w", openErr)
		}
		defer func() { _ = in.Close() }()

		flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		if !overwrite {
			flags = os.O_CREATE | os.O_WRONLY | os.O_EXCL
		}
		out, createErr := os.OpenFile(dst, flags, info.Mode())
		if createErr != nil {
			return fmt.Errorf("falha ao mover arquivo: %w", createErr)
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			_ = os.Remove(dst)
			return fmt.Errorf("falha ao mover arquivo: %w", copyErr)
		}
		if closeErr != nil {
			_ = os.Remove(dst)
			return fmt.Errorf("falha ao mover arquivo: %w", closeErr)
		}

		if remErr := os.Remove(src); remErr != nil {
			return fmt.Errorf("arquivo copiado mas falha ao remover origem: %w", remErr)
		}
		return nil
	}
}

// MoveFile move/renomeia um arquivo no disco usando a policy padrão de toolcalling.
func MoveFile(oldPath string, newPath string, overwrite bool) error {
	return MoveFileWithPolicy(oldPath, newPath, overwrite, ToolPolicy())
}

// RenameFileSameDirWithPolicy renomeia um arquivo no disco, mantendo o diretório, aplicando policy.
// newBaseName deve ser apenas o nome do arquivo (sem diretórios).
// Se newBaseName não tiver extensão, preserva a extensão do oldPath.
// Retorna o novo path completo.
func RenameFileSameDirWithPolicy(oldPath string, newBaseName string, policy Policy) (string, error) {
	src := strings.TrimSpace(oldPath)
	if src == "" {
		return "", fmt.Errorf("oldPath vazio")
	}

	name := strings.TrimSpace(newBaseName)
	if err := configdir.ValidateFilename(name); err != nil {
		return "", fmt.Errorf("novo nome inválido: %w", err)
	}

	if filepath.Ext(name) == "" {
		ext := filepath.Ext(src)
		if ext != "" {
			name = name + ext
		}
	}

	dst := filepath.Join(filepath.Dir(src), name)
	if err := MoveFileWithPolicy(src, dst, false, policy); err != nil {
		return "", err
	}
	return dst, nil
}

// RenameFileSameDir renomeia um arquivo no disco, mantendo o diretório, usando a policy padrão de toolcalling.
func RenameFileSameDir(oldPath string, newBaseName string) (string, error) {
	return RenameFileSameDirWithPolicy(oldPath, newBaseName, ToolPolicy())
}
