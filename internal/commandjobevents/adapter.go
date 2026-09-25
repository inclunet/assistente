package commandjobevents

import (
	"context"
	"errors"
)

// ErrAdapterDisabled sinaliza que a outbox está persistida, mas ainda não há
// claims e coordenador de manutenção autorizando consumo. Isso evita que a
// presença de uma linha durável conceda capacidade por si só.
var ErrAdapterDisabled = errors.New("command job event adapter disabled until claims and maintenance are ready")

// Adapter é a borda reservada para o dispatcher da AEP-0103. Por desenho, a
// implementação desta fatia não ativa regras nem chama handlers: o método
// falha fechado até receber as dependências autoritativas de claims e
// manutenção em uma integração posterior.
type Adapter struct {
	store            *Store
	claimsReady      bool
	maintenanceReady bool
}

func NewAdapter(store *Store) *Adapter { return &Adapter{store: store} }

func (a *Adapter) Enabled() bool {
	return a != nil && a.store != nil && a.claimsReady && a.maintenanceReady
}

func (a *Adapter) Dispatch(context.Context, ActivationOutbox) error {
	if !a.Enabled() {
		return ErrAdapterDisabled
	}
	return ErrAdapterDisabled
}
