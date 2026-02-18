// +build windows

package updater

import (
	"syscall"
	"unsafe"
)

var (
	shell32           = syscall.NewLazyDLL("shell32.dll")
	shellExecuteW     = shell32.NewProc("ShellExecuteW")
)

// shellExecute executa um programa usando ShellExecuteW do Windows
// verb: "runas" para elevação, "open" para execução normal
// file: caminho do executável
// params: parâmetros da linha de comando
// dir: diretório de trabalho (usar "" para padrão)
// show: modo de exibição da janela (1 = SW_SHOWNORMAL, 0 = SW_HIDE)
func shellExecute(verb, file, params, dir string, show int) error {
	verbPtr, _ := syscall.UTF16PtrFromString(verb)
	filePtr, _ := syscall.UTF16PtrFromString(file)
	paramsPtr, _ := syscall.UTF16PtrFromString(params)
	dirPtr, _ := syscall.UTF16PtrFromString(dir)

	ret, _, _ := shellExecuteW.Call(
		0, // hwnd
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(filePtr)),
		uintptr(unsafe.Pointer(paramsPtr)),
		uintptr(unsafe.Pointer(dirPtr)),
		uintptr(show),
	)

	// ShellExecute retorna um valor > 32 em caso de sucesso
	if ret <= 32 {
		return syscall.Errno(ret)
	}

	return nil
}
