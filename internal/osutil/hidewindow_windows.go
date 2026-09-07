//go:build windows

package osutil

import (
	"os/exec"
	"syscall"
)

// createNoWindow corresponde à flag CREATE_NO_WINDOW do Windows: impede a
// alocação de um console para o processo-filho. Não afeta processos GUI.
const createNoWindow = 0x08000000

// HideConsoleWindow impede que processos-filho de console abram janela e
// roubem o foco no Windows. Sem isso, cada spawn (ex.: servidores MCP stdio,
// comandos de pré-processamento de skills) abre um prompt de comando por um
// instante; leitores de tela como o NVDA anunciam o título da janela (o
// caminho do executável) e interrompem o usuário.
func HideConsoleWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
