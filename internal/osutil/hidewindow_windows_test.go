//go:build windows

package osutil

import (
	"os/exec"
	"syscall"
	"testing"
)

func TestHideConsoleWindowPreencheSysProcAttr(t *testing.T) {
	cmd := exec.Command("cmd", "/C", "echo")

	HideConsoleWindow(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr deveria ter sido inicializado")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Error("HideWindow deveria ser true")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Errorf("CreationFlags deveria conter CREATE_NO_WINDOW (0x%08x), obtido 0x%08x", createNoWindow, cmd.SysProcAttr.CreationFlags)
	}
}

func TestHideConsoleWindowPreservaSysProcAttrExistente(t *testing.T) {
	const outraFlag = 0x00000200 // CREATE_NEW_PROCESS_GROUP

	cmd := exec.Command("cmd", "/C", "echo")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: outraFlag}

	HideConsoleWindow(cmd)

	if cmd.SysProcAttr.CreationFlags&outraFlag == 0 {
		t.Error("flags preexistentes deveriam ser preservadas")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Error("CREATE_NO_WINDOW deveria ter sido adicionada às flags existentes")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Error("HideWindow deveria ser true")
	}
}
