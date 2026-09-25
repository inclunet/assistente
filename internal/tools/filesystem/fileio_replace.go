package filesystem

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// WriteFileBytesReplacing prepara todo o conteúdo antes de substituir o destino.
// Não trunca o arquivo original em falhas de escrita, sync ou rename. Serializa
// escritores deste processo; não é CAS nem lock contra programas externos.
// Em plataformas nas quais rename não garante atomicidade, essa limitação
// permanece. Nunca removemos o destino como fallback de um rename recusado.
func WriteFileBytesReplacing(path string, content []byte, perm fs.FileMode) error {
	unlock := lockFileMutation(path)
	defer unlock()
	return writeFileBytesReplacing(path, content, perm, os.Rename)
}

func writeFileBytesReplacing(path string, content []byte, perm fs.FileMode, replace func(string, string) error) error {
	if err := EnsureParentDir(path); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		// O caller deve resolver symlinks com sua política antes da chamada.
		if !info.Mode().IsRegular() {
			return fmt.Errorf("destino não é arquivo regular")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".editor-save-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if _, err := temporary.Write(content); err != nil {
		return err
	}
	if err := temporary.Chmod(perm); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return replace(temporaryPath, path)
}
