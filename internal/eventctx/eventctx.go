// Package eventctx carrega a proveniência de uma mutação no context.Context,
// permitindo que produtores de eventos de domínio (ex.: tasklist.Service)
// distingam mutações originadas por um job das originadas por um humano.
//
// É um pacote neutro propositalmente, para que tanto o produtor (internal/tasklist)
// quanto o carimbador (internal/jobs) possam usá-lo sem criar ciclo de imports.
//
// Ver AEP-0067 (proveniência e anti-loop).
package eventctx

import "context"

type contextKey struct{}

var provenanceKey = contextKey{}

// Provenance descreve a origem de uma mutação que pode emitir eventos de domínio.
type Provenance struct {
	Source       string   // "user" (humano) | "job" (automação)
	SourceJobID  string   // job que originou a mutação (quando Source=="job")
	ChainID      string   // identificador da cadeia (circuit breaker)
	ChainHistory []string // histórico de jobs/eventos na cadeia
}

// With retorna um ctx derivado carregando a proveniência informada.
func With(ctx context.Context, p Provenance) context.Context {
	return context.WithValue(ctx, provenanceKey, p)
}

// From recupera a proveniência carimbada no ctx, se houver.
func From(ctx context.Context) (Provenance, bool) {
	if ctx == nil {
		return Provenance{}, false
	}
	p, ok := ctx.Value(provenanceKey).(Provenance)
	return p, ok
}
