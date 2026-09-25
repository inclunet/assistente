// Package commandsecurity contém primitivas de coordenação para o despacho de
// comandos sensíveis.
package commandsecurity

import (
	"context"
	"errors"
	"sync"
)

var (
	errNilContext  = errors.New("commandsecurity: contexto nil")
	errNilCallback = errors.New("commandsecurity: callback nil")
	errNilGate     = errors.New("commandsecurity: gate nil")
)

// DispatchGate serializa a admissão compartilhada de comandos contra
// mutações exclusivas de segurança ou configuração.
//
// O zero value está pronto para uso. A espera pelo lock não é cancelável: um
// contexto cancelado enquanto outro callback detém o lock só é observado após
// a aquisição. O callback deve fazer apenas o handoff síncrono e não bloqueante
// de início dentro do gate; espera ou execução longa não mantém o gate. Um
// callback não deve readquirir este gate.
type DispatchGate struct {
	mu sync.RWMutex
}

// WithAdmission executa fn sob admissão compartilhada.
func (g *DispatchGate) WithAdmission(ctx context.Context, fn func() error) error {
	if g == nil {
		return errNilGate
	}
	if ctx == nil {
		return errNilContext
	}
	if fn == nil {
		return errNilCallback
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	g.mu.RLock()
	defer g.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn()
}

// WithMutation executa fn sob mutação exclusiva.
func (g *DispatchGate) WithMutation(ctx context.Context, fn func() error) error {
	if g == nil {
		return errNilGate
	}
	if ctx == nil {
		return errNilContext
	}
	if fn == nil {
		return errNilCallback
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn()
}
