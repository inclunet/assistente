//go:build windows

package logging

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func requireSingleLogLink(file *os.File) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return err
	}
	if info.NumberOfLinks != 1 {
		return errors.New("arquivo de log possui links físicos")
	}
	return nil
}
