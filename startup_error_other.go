//go:build !windows

package main

import (
	"fmt"
	"io"
)

func reportFatalError(output io.Writer, message string) {
	_, _ = fmt.Fprintln(output, message)
}
