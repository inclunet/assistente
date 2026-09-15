package commandidentity

import (
	"assistente/internal/commandsecurity"
	"context"
)

// CoreEpochs adapta assinaturas sem criar estado: identidades e executor
// compartilham exatamente a mesma instância de EpochService.
type CoreEpochs struct{ core *commandsecurity.EpochService }

func NewCoreEpochs(core *commandsecurity.EpochService) (*CoreEpochs, error) {
	if core == nil {
		return nil, ErrEpochUnavailable
	}
	return &CoreEpochs{core: core}, nil
}

func (p *CoreEpochs) CaptureContextAuthenticated(ctx context.Context, authenticate func() (ContextPrincipal, error)) (Epoch, error) {
	if p == nil || p.core == nil || authenticate == nil {
		return Epoch{}, ErrEpochUnavailable
	}
	e, err := p.core.CaptureContextAuthenticated(ctx, func(context.Context) (commandsecurity.ContextPrincipal, error) {
		v, err := authenticate()
		return commandsecurity.ContextPrincipal{UserID: v.UserID, Type: v.Type, ID: v.ID}, err
	})
	if err != nil {
		return Epoch{}, err
	}
	return Epoch{AuthGeneration: e.AuthGeneration, SecurityGeneration: e.SecurityGeneration}, nil
}

func (p *CoreEpochs) MutateContext(ctx context.Context, principal ContextPrincipal, action func() error) error {
	if p == nil || p.core == nil {
		return ErrEpochUnavailable
	}
	return p.core.MutateContext(ctx, commandsecurity.ContextPrincipal{UserID: principal.UserID, Type: principal.Type, ID: principal.ID}, action)
}
