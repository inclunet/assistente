package commandconfig

import "context"

// ProjectionProvider reconsulta o catálogo/defaults/adapters do host. Versão
// correspondente deve ser coberta por MutationServiceConfig.Version no mesmo
// gate. Não é uma extensão configurável pelo cliente.
type ProjectionProvider func(context.Context, Scope) (CompleteProjection, error)

type CompleteMutationService struct {
	service    *MutationService
	projection ProjectionProvider
}

// NewCompleteMutationService impede que a montagem runtime omita a validação
// semântica do projetor. Ativação não é autorização para persistir: IDs ativos
// são somente prova dinâmica validada/canonizada para a projeção, sem efeitos.
func NewCompleteMutationService(c MutationServiceConfig, projection ProjectionProvider) (*CompleteMutationService, error) {
	if projection == nil {
		return nil, ErrInvalid
	}
	c.Validate = func(ctx context.Context, snapshot Snapshot) error {
		options, err := projection(ctx, cloneScope(snapshot.Scope))
		if err != nil {
			return err
		}
		if err := validateCompleteActivationAggregate(snapshot, options); err != nil {
			return err
		}
		// A porta descreve o estado persistido atual, não o estado proposto.
		// Validar diretamente contra o depois impediria excluir/desabilitar
		// justamente uma camada ativa. Primeiro comprovar a origem; depois
		// limitar a projeção às camadas que sobrevivem habilitadas à mutação.
		current, err := c.Store.Load(ctx, cloneScope(snapshot.Scope))
		if err != nil {
			return err
		}
		active, err := validatePreviewActiveUserLayerIDs(current, options.ActiveUserLayerIDs)
		if err != nil {
			return err
		}
		remaining := make(map[string]bool, len(snapshot.Layers))
		for _, layer := range snapshot.Layers {
			remaining[layer.ID] = layer.Enabled
		}
		options.ActiveUserLayerIDs = nil
		for _, id := range active {
			if remaining[id] {
				options.ActiveUserLayerIDs = append(options.ActiveUserLayerIDs, id)
			}
		}
		_, err = ProjectComplete(ctx, snapshot, options)
		return err
	}
	s, err := NewMutationService(c)
	if err != nil {
		return nil, err
	}
	return &CompleteMutationService{service: s, projection: projection}, nil
}

func (s *CompleteMutationService) Apply(ctx context.Context, token string, workspace *string, intent MutationIntent) (MutationDiff, error) {
	if s == nil {
		return MutationDiff{}, ErrInvalid
	}
	return s.service.Apply(ctx, token, workspace, intent)
}

func (s *CompleteMutationService) UpgradeDefaults(ctx context.Context, token string, workspace *string) (MutationDiff, error) {
	if s == nil {
		return MutationDiff{}, ErrInvalid
	}
	return s.service.applyPrepared(ctx, token, workspace, DefaultUpgrade, func(ctx context.Context, scope Scope) (*PreparedMutation, error) {
		options, err := s.projection(ctx, cloneScope(scope))
		if err != nil {
			return nil, err
		}
		return s.service.config.Store.PrepareDefaultUpgrade(ctx, scope, options)
	})
}

func (s *CompleteMutationService) RebaseDefault(ctx context.Context, token string, workspace *string, request DefaultRebaseRequest) (MutationDiff, error) {
	if s == nil {
		return MutationDiff{}, ErrInvalid
	}
	return s.service.applyPrepared(ctx, token, workspace, DefaultRebase, func(ctx context.Context, scope Scope) (*PreparedMutation, error) {
		options, err := s.projection(ctx, cloneScope(scope))
		if err != nil {
			return nil, err
		}
		return s.service.config.Store.PrepareDefaultRebase(ctx, scope, request, options)
	})
}
