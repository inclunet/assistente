package filesystem

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func CopyFileWithPolicy(srcPath string, dstPath string, overwrite bool, policy Policy) (int64, error) {
	src := strings.TrimSpace(srcPath)
	dst := strings.TrimSpace(dstPath)
	if src == "" {
		return 0, fmt.Errorf("srcPath vazio")
	}
	if dst == "" {
		return 0, fmt.Errorf("dstPath vazio")
	}
	if src == dst {
		return 0, nil
	}

	if policy.BlockSensitive {
		if isSensitiveFile(src) || isSensitiveFile(dst) {
			return 0, fmt.Errorf("não é permitido copiar arquivos sensíveis")
		}
	}

	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("arquivo não encontrado: %s", srcPath)
		}
		return 0, fmt.Errorf("erro ao acessar arquivo: %w", err)
	}
	if info.IsDir() {
		return 0, fmt.Errorf("'%s' é um diretório, não um arquivo", srcPath)
	}

	if !overwrite {
		if _, err := os.Stat(dst); err == nil {
			return 0, fmt.Errorf("já existe um arquivo no destino")
		}
	}

	if err := EnsureParentDir(dst); err != nil {
		return 0, fmt.Errorf("falha ao criar diretório de destino: %w", err)
	}

	in, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("erro ao abrir origem: %w", err)
	}
	defer in.Close()

	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if !overwrite {
		flags = os.O_CREATE | os.O_WRONLY | os.O_EXCL
	}
	out, err := os.OpenFile(dst, flags, info.Mode()&fs.ModePerm)
	if err != nil {
		return 0, fmt.Errorf("erro ao criar destino: %w", err)
	}

	bytes, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(dst)
		return 0, fmt.Errorf("erro ao copiar arquivo: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return 0, fmt.Errorf("erro ao finalizar escrita: %w", closeErr)
	}

	return bytes, nil
}

func RemoveFileWithPolicy(path string, policy Policy) error {
	p := strings.TrimSpace(path)
	if p == "" {
		return fmt.Errorf("path vazio")
	}
	if policy.BlockSensitive && isSensitiveFile(p) {
		return fmt.Errorf("não é permitido remover arquivos sensíveis")
	}
	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("'%s' é um diretório, não um arquivo", filepath.Base(p))
	}
	if err := os.Remove(p); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return nil
}

func EnsureDirWithPolicy(path string, perm fs.FileMode, policy Policy) error {
	p := strings.TrimSpace(path)
	if p == "" {
		return fmt.Errorf("path vazio")
	}
	if policy.BlockSensitive && isSensitiveFile(p) {
		return fmt.Errorf("não é permitido criar diretórios sensíveis")
	}
	if err := os.MkdirAll(p, perm); err != nil {
		return err
	}
	return nil
}
