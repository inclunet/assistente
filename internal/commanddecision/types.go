// Package commanddecision mantém recibos de decisão do subconjunto de mutações
// de configuração. Não autentica, autoriza ou registra presenters por payload.
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
// O subconjunto admite só subject_type=config_mutation e apply/deny.
type Request struct {
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
