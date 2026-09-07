//go:build !windows

package osutil

import "os/exec"

// HideConsoleWindow é no-op fora do Windows: apenas no Windows processos-filho
// de console abrem janela própria e roubam o foco da aplicação.
func HideConsoleWindow(_ *exec.Cmd) {}
