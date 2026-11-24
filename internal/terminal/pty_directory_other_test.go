//go:build !windows

package terminal

import "testing"

func ptyTestDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}
