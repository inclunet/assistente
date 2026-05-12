package main

import (
	"fmt"
	"io"
)

// errWriter wraps an io.Writer and captures the first write error,
// turning subsequent writes into no-ops. This avoids repetitive
// error checks on sequential fmt.Fprint* calls.
type errWriter struct {
	w   io.Writer
	err error
}

func (ew *errWriter) printf(format string, args ...interface{}) {
	if ew.err != nil {
		return
	}
	_, ew.err = fmt.Fprintf(ew.w, format, args...)
}

func (ew *errWriter) println(args ...interface{}) {
	if ew.err != nil {
		return
	}
	_, ew.err = fmt.Fprintln(ew.w, args...)
}
