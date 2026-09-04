//go:build darwin && !cgo

package wakelock

// Builds sem cgo não podem acessar IOKit. O wakelock degrada de forma segura
// para no-op, preservando a CLI portátil compilada pelo workflow de release.
func enable()  {}
func disable() {}
