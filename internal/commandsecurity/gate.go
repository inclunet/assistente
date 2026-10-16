// Package commandsecurity contém primitivas de coordenação para o despacho de
// comandos sensíveis.
package commandsecurity

import (
	"context"
	"errors"
	"math"
	"sync"

	"golang.org/x/sync/semaphore"
)

var (
	errNilContext  = errors.New("commandsecurity: contexto nil")
	errNilCallback = errors.New("commandsecurity: callback nil")
	errNilGate     = errors.New("commandsecurity: gate nil")
)

// DispatchGate serializa a admissão compartilhada de comandos contra
// mutações exclusivas de segurança ou configuração.
//
// O zero value está pronto para uso. A espera por admissão é cancelável e
// respeita a ordem dos escritores que já aguardam. O callback deve fazer
// apenas o handoff síncrono e não bloqueante de início dentro do gate; uma vez
// iniciado, ele não é interrompido por cancelamento. Um callback não deve
// readquirir este gate.
type DispatchGate struct {
	init sync.Once
	sem  *semaphore.Weighted
}

func (g *DispatchGate) semaphore() *semaphore.Weighted {
	g.init.Do(func() { g.sem = semaphore.NewWeighted(math.MaxInt64) })
	return g.sem
}

// WithAdmission aguarda admissão compartilhada usando ctx somente para limitar
// a espera pela aquisição. fn executa sincronamente e deve fazer apenas o
// handoff curto; seu contexto de aquisição não controla nem interrompe fn.
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

	sem := g.semaphore()
	if err := sem.Acquire(ctx, 1); err != nil {
		return err
	}
	defer sem.Release(1)
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn()
}

// WithMutation aguarda mutação exclusiva usando ctx somente para limitar a
// espera pela aquisição. fn executa sincronamente e deve fazer apenas o
// handoff curto; seu contexto de aquisição não controla nem interrompe fn.
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

	sem := g.semaphore()
	if err := sem.Acquire(ctx, math.MaxInt64); err != nil {
		return err
	}
	defer sem.Release(math.MaxInt64)
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn()
}
