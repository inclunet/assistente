//go:build !windows && !linux && !darwin

package commandinstance

import "os"

func acquirePlatformLock(file *os.File) (string, func() error, error) { return "", nil, ErrUnsupported }
