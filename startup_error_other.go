//go:build !windows && !darwin && !linux

package main

var showNativeFatalError = func(string) {}
