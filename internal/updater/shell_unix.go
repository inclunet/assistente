// +build !windows

package updater

import (
	"fmt"
)

// shellExecute é um stub para sistemas não-Windows
func shellExecute(verb, file, params, dir string, show int) error {
	return fmt.Errorf("shellExecute is only supported on Windows")
}
