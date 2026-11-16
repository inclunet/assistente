//go:build linux || darwin

package logging

import (
	"errors"
	"os"
	"syscall"
)

func requireSingleLogLink(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		return errors.New("arquivo de log possui links físicos ou identidade indisponível")
	}
	return nil
}
