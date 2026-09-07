//go:build !windows && !linux && !darwin

package wakelock

func enable()  {}
func disable() {}
