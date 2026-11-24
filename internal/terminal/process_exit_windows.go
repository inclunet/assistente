package terminal

import "golang.org/x/sys/windows"

// O ptyx fecha o leitor do ConPTY assim que o processo é sinalizado. O
// handle mantido pela sessão conserva o PID até Wait e Close terminarem.
func conPTYProcessExited(pid int) bool {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	state, err := windows.WaitForSingleObject(handle, 0)
	return err == nil && state == windows.WAIT_OBJECT_0
}
