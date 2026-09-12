//go:build !windows && !darwin

package main

func platformStartupLocale() string {
	return ""
}
