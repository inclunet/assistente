package wakelock

import "testing"

func TestManagerIdempotente(t *testing.T) {
	var m Manager
	m.SetEnabled(true)
	m.SetEnabled(true)
	m.SetEnabled(false)
	m.SetEnabled(false)
	m.Release()
}
