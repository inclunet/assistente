package app

import (
	"context"
	"time"
)

// Orçamento cooperativo de leitura/reconstrução, não de execução de comandos.
// Não interrompe à força uma porta que desrespeite o contexto.
const commandReadTimeout = 5 * time.Second

func (p *commandProductRuntime) acquireCommandProjection(ctx context.Context) (func(), error) {
	p.projectionOnce.Do(func() { p.projectionGate = make(chan struct{}, 1) })
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case p.projectionGate <- struct{}{}:
	}
	if err := ctx.Err(); err != nil {
		<-p.projectionGate
		return nil, err
	}
	return func() { <-p.projectionGate }, nil
}

func (p *commandProductRuntime) withCommandProjection(ctx context.Context, rebuild func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, commandReadTimeout)
	defer cancel()
	commandLoadStage(ctx, "projection_wait")
	release, err := p.acquireCommandProjection(ctx)
	if err != nil {
		return err
	}
	defer release()
	commandLoadStage(ctx, "projection_rebuild")
	return rebuild(ctx)
}
