// Package commanddecision mantém recibos de decisão de mutações de configuração
// locais e invocações locais/externas. Não autentica, autoriza ou registra
// presenters por payload.
package commanddecision

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalid = errors.New("decisão inválida")
	ErrStale   = errors.New("decisão obsoleta ou já utilizada")
)

const (
	ApplyAction = "apply"
	DenyAction  = "deny"
	Pending     = "pending"
	Accepted    = "accepted"
	Denied      = "denied"
	Cancelled   = "cancelled"
	Expired     = "expired"
	Consumed    = "consumed"
)

// Request vem exclusivamente do backend autenticado. Fingerprint é calculado
// pelo chamador confiável sobre a solicitação completa; não é segredo bruto.
// Body é o diff já validado a apresentar, nunca persistido neste repository.
// Os subjects config_mutation e invocation permanecem isolados; ações apply/deny.
type Request struct {
	// Metadata de apresentação derivada do catálogo; não concede autoridade.
	Destructive bool
	// AuthContextType vazio preserva compatibilidade e equivale a local_session.
	// SessionID mantém o nome histórico, mas armazena o auth_context_id exato
	// para todos os tipos de contexto.
	AuthContextType string
	// Vazio preserva o contrato legado config_mutation. Invocation usa o mesmo
	// protocolo de apresentação/consumo e vincula MutationID ao invocation_id.
	SubjectType                                     string
	DecisionID, MutationID, UserID, SessionID       string
	Fingerprint, AuthGeneration, SecurityGeneration string
	ExpiresAt                                       time.Time
	Body                                            string
}

type Response struct {
	DecisionID, ActionID string
	Cancelled            bool
}

// Presenter é instalado pelo bootstrap local confiável, nunca escolhido pela
// solicitação. Na v1, a implementação deve usar a sessão Wails autenticada.
type Presenter interface {
	Present(context.Context, Request) (Response, error)
}
