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

func showFatalErrorMessageBox(title, message string) {
	text, textErr := windows.UTF16PtrFromString(message)
	titleUTF16, titleErr := windows.UTF16PtrFromString(title)
	if textErr != nil || titleErr != nil {
		return
	}
	_, _, _ = messageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(text)),
		uintptr(unsafe.Pointer(titleUTF16)),
		messageBoxOK|messageBoxIconError|messageBoxSetForeground|messageBoxTaskModal,
	)
}
