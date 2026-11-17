//go:build !windows

package terminal

import "path/filepath"

// ResolveDirectoryPath resolve o destino físico usado na autorização e execução.
func ResolveDirectoryPath(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
