//go:build !windows
// +build !windows

package updater

import (
	"fmt"
)

// shellExecute é um stub para sistemas não-Windows
func shellExecute(_ /* verb */, _ /* file */, _ /* params */, _ /* dir */ string, _ /* show */ int) error {
	return fmt.Errorf("shellExecute is only supported on Windows")
}
