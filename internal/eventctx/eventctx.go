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

type suppressKey struct{}

var domainSuppressKey = suppressKey{}

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

// WithSuppressed marca o ctx para que eventos de domínio NÃO sejam publicados.
// Usado em dry-run: a tool roda de verdade (e pode mutar estado), mas a ponte de
// eventos de domínio deve ficar muda para não cascatear outros jobs. A supressão
// é independente da proveniência (chave própria), então convive com With/From.
func WithSuppressed(ctx context.Context) context.Context {
	return context.WithValue(ctx, domainSuppressKey, true)
}

// IsSuppressed informa se a publicação de eventos de domínio está suprimida no ctx.
func IsSuppressed(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(domainSuppressKey).(bool)
	return v
}
