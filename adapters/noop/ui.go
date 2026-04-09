// Package noop contém Outbound Adapters que descartam silenciosamente operações de UI.
// Útil em testes unitários e futuros modos CLI/headless.
package noop

import "assistente/internal/core/ports"

// WindowAdapter é uma implementação no-op de ports.WindowPort.
type WindowAdapter struct{}

func (WindowAdapter) Show() {}

// DialogAdapter é uma implementação no-op de ports.SystemDialogPort.
type DialogAdapter struct{}

func (DialogAdapter) OpenFileDialog(_ ports.OpenFileOptions) (string, error) { return "", nil }
func (DialogAdapter) SaveFileDialog(_ ports.SaveFileOptions) (string, error) { return "", nil }

// EmitterAdapter é uma implementação no-op de ports.Emitter.
type EmitterAdapter struct{}

func (EmitterAdapter) Emit(_ string, _ any) {}
