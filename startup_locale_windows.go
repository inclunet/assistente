//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const localeNameMaxLength = 85

var getUserDefaultLocaleName = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultLocaleName")

func platformStartupLocale() string {
	buffer := make([]uint16, localeNameMaxLength)
	result, _, _ := getUserDefaultLocaleName.Call(
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
	)
	if result == 0 {
		return ""
	}
	return windows.UTF16ToString(buffer)
}
