//go:build unix

package pluginpackage

import "syscall"

// A package entry can change between Stat and Open. A FIFO must not hold
// the inspection goroutine while it waits for a writer.
const readFlags = syscall.O_NONBLOCK
