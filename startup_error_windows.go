//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	messageBoxOK            = 0x00000000
	messageBoxIconError     = 0x00000010
	messageBoxSetForeground = 0x00010000
	messageBoxTaskModal     = 0x00002000
)

var messageBoxW = windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW")
var showNativeFatalError = showFatalErrorMessageBox

func showFatalErrorMessageBox(message string) {
	text, textErr := windows.UTF16PtrFromString(message)
	title, titleErr := windows.UTF16PtrFromString("Assistente")
	if textErr != nil || titleErr != nil {
		return
	}
	_, _, _ = messageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(text)),
		uintptr(unsafe.Pointer(title)),
		messageBoxOK|messageBoxIconError|messageBoxSetForeground|messageBoxTaskModal,
	)
}
