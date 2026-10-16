//go:build windows

package desktopinstance

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func physicalIdentity(file *os.File) (string, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return "", err
	}
	if info.FileIndexHigh == 0 && info.FileIndexLow == 0 {
		return "", fmt.Errorf("unsupported file identity")
	}
	return fmt.Sprintf("windows:%d:%d:%d", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), nil
}
