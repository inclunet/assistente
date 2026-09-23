//go:build windows

package workspace

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestCreateForCommandWindowsIndexOpenWithoutDeleteShareCompensates(t *testing.T) {
	manager, homeDir := newCommandCreateWorkspaceManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("CommandSnapshot: %v", err)
	}
	indexPath := commandWorkspaceIndexPath(homeDir)
	oldBytes, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read original index: %v", err)
	}
	dirsBefore := commandWorkspaceDirCount(t, homeDir)

	handle, err := windows.CreateFile(
		windows.StringToUTF16Ptr(indexPath),
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatalf("open index without FILE_SHARE_DELETE: %v", err)
	}
	defer windows.CloseHandle(handle)

	_, err = manager.CreateForCommand(context.Background(), expected, "Windows rename guard")
	if err == nil || !strings.Contains(err.Error(), "publish workspace index after retries") {
		t.Fatalf("expected index publication failure while index denies delete sharing, got %v", err)
	}
	gotBytes, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index after rejected rename: %v", err)
	}
	if !bytes.Equal(gotBytes, oldBytes) {
		t.Fatal("index bytes changed after rejected rename")
	}
	if dirsAfter := commandWorkspaceDirCount(t, homeDir); dirsAfter != dirsBefore {
		t.Fatalf("new workspace directory was not compensated: before=%d after=%d", dirsBefore, dirsAfter)
	}
}
