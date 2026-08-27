//go:build windows

package wakelock

import "syscall"

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procSetThreadExecState   = kernel32.NewProc("SetThreadExecutionState")
	esContinuous       uint32 = 0x80000000
	esSystemRequired   uint32 = 0x00000001
	esDisplayRequired  uint32 = 0x00000002
)

func enable() {
	_, _, _ = procSetThreadExecState.Call(uintptr(esContinuous | esSystemRequired | esDisplayRequired))
}

func disable() {
	_, _, _ = procSetThreadExecState.Call(uintptr(esContinuous))
}
