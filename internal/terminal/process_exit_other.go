//go:build !windows

package terminal

func conPTYProcessExited(int) bool { return false }
