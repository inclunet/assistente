//go:build !windows && !linux && !darwin

package desktopinstance

import (
	"errors"
	"os"
)

func physicalIdentity(*os.File) (string, error) {
	return "", errors.New("unsupported desktop platform")
}
