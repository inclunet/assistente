//go:build !windows && !linux && !darwin

package logging

import (
	"errors"
	"os"
)

func requireSingleLogLink(*os.File) error {
	return errors.New("verificação de links de log não suportada nesta plataforma")
}
