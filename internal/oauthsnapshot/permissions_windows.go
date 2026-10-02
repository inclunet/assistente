package oauthsnapshot

import (
	"golang.org/x/sys/windows"
	"os"
	"strings"
)

var reopenFile = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReOpenFile")

// Windows has no directory fsync through os.File; content is flushed before publication.
func (f *Files) syncDirectory() error { return nil }

func rejectReparse(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	a, err := windows.GetFileAttributes(p)
	if err != nil || a&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return ErrStorage
	}
	return nil
}
func openPrivateDirectory(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.READ_CONTROL|windows.WRITE_DAC, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}
func protect(file *os.File, directory bool) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	flags := ""
	if directory {
		flags = "OICI"
	}
	desired, err := windows.SecurityDescriptorFromString("D:P(A;" + flags + ";FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return err
	}
	dacl, _, err := desired.DACL()
	if err != nil {
		return err
	}
	// Reopen the same object (not its path) with WRITE_DAC. Go's ordinary
	// read/write handle does not request permission to change its ACL.
	h := windows.Handle(file.Fd())
	if !directory {
		value, _, openErr := reopenFile.Call(file.Fd(), windows.READ_CONTROL|windows.WRITE_DAC, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, 0)
		h = windows.Handle(value)
		if h == windows.InvalidHandle {
			return openErr
		}
		defer func() { _ = windows.CloseHandle(h) }()
	}
	if err = windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		return err
	}
	actual, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	actualACL, _, err := actual.DACL()
	if err != nil || actualACL == nil || actualACL.AceCount != 1 {
		return ErrStorage
	}
	control, _, err := actual.Control()
	// Windows may add AUTO_INHERITED to the descriptor. The protected DACL
	// must still contain exactly the one explicit allow ACE for this user.
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 || !strings.HasSuffix(actual.String(), "(A;"+flags+";FA;;;"+user.User.Sid.String()+")") {
		return ErrStorage
	}
	return nil
}
