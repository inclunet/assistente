package pluginhooks

import "golang.org/x/sys/windows"

func currentUserPrivateFile(path string) (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	id := user.User.Sid.String()
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + id + ")")
	if err != nil {
		return "", err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return "", err
	}
	err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
	return id, err
}
