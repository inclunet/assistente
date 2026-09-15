package commandconfig

import (
	"context"
	"slices"
	"strings"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandsecurity"
)

// ConflictDiagnostic é uma leitura autenticada e versionada da configuração.
// O stamp privado torna explícita a janela entre diagnóstico e commit: o
// chamador deve revalidá-lo antes de apresentar uma decisão ou aplicar uma
// mutação. Os witnesses são produzidos pelo mesmo resolver usado em runtime.
type ConflictDiagnostic struct {
	Scope      Scope
	Generation []Generation
	Witnesses  []commandbindings.ConflictWitness
	store      *Store
	snapshot   Snapshot
	userID     string
	sessionID  string
	epoch      commandsecurity.EpochSnapshot
	// activeUserLayerIDs é a fotografia privada da ativação dinâmica usada na
	// projeção. Ela não é uma autoridade pública nem pode ser substituída pelo
	// payload do chamador; serve apenas para detectar mudança antes da decisão.
	activeUserLayerIDs []string
}

// CheckConflicts autentica o token, autoriza somente a ação de diagnóstico,
// carrega o snapshot e usa a projeção completa do host. Não executa comandos
// nem aceita fatos, IDs ou candidatos fornecidos pelo cliente.
func (s *CompleteMutationService) CheckConflicts(ctx context.Context, token string, workspace *string) (ConflictDiagnostic, error) {
	if s == nil || s.service == nil || ctx == nil {
		return ConflictDiagnostic{}, ErrInvalid
	}
	workspace = cloneWorkspace(workspace)
	var principal auth.LocalSessionPrincipal
	var scope Scope
	epoch, err := s.service.config.Epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		var err error
		principal, err = s.service.config.Sessions.AuthenticateLocalAccess(ctx, token)
		if err != nil {
			return "", "", err
		}
		scope = Scope{UserID: principal.UserID, WorkspaceID: workspace}
		if !validScope(scope) {
			return "", "", ErrInvalid
		}
		if err := s.service.config.Authorize(ctx, principal, cloneScope(scope), Operation("binding_check_conflict")); err != nil {
			return "", "", err
		}
		return principal.UserID, principal.SessionID, nil
	})
	if err != nil {
		return ConflictDiagnostic{}, err
	}
	var snapshot Snapshot
	var witnesses []commandbindings.ConflictWitness
	var dynamicActive []string
	if err := s.service.config.Epochs.Admit(ctx, epoch, func(ctx context.Context) error {
		current, err := s.service.config.Sessions.AuthenticateLocalAccess(ctx, token)
		if err != nil {
			return err
		}
		if current.UserID != principal.UserID || current.SessionID != principal.SessionID {
			return ErrStale
		}
		return s.service.config.Authorize(ctx, current, cloneScope(scope), Operation("binding_check_conflict"))
	}, func() error {
		providerVersion, err := s.service.config.Version(ctx)
		if err != nil {
			return err
		}
		if providerVersion == "" {
			return ErrInvalid
		}
		loaded, err := s.service.config.Store.Load(ctx, scope)
		if err != nil {
			return err
		}
		options, err := s.projection(ctx, cloneScope(scope))
		if err != nil {
			return err
		}
		activeUserLayerIDs, err := validatePreviewActiveUserLayerIDs(loaded, options.ActiveUserLayerIDs)
		if err != nil {
			return err
		}
		options.ActiveUserLayerIDs = activeUserLayerIDs
		configuration, err := ProjectComplete(ctx, loaded, options)
		if err != nil {
			return err
		}
		witnesses, err = configuration.CheckConflicts(ctx)
		if err != nil {
			return err
		}
		if loaded.stamp == nil {
			return ErrInvalid
		}
		loaded.stamp.providerVersion = providerVersion
		snapshot = loaded
		// A lista vem da porta confiável do host e só é retida depois de a
		// projeção completa validá-la contra o snapshot carregado.
		dynamicActive = slices.Clone(activeUserLayerIDs)
		return nil
	}); err != nil {
		return ConflictDiagnostic{}, err
	}
	return ConflictDiagnostic{Scope: cloneScope(snapshot.Scope), Generation: cloneGenerations(snapshot.Generations), Witnesses: witnesses,
		store: s.service.config.Store, snapshot: snapshot, userID: principal.UserID, sessionID: principal.SessionID, epoch: epoch,
		activeUserLayerIDs: slices.Clone(dynamicActive)}, nil
}

// RevalidateConflicts autentica novamente e faz o check de geração antes do
// commit subsequente. Qualquer mutação concorrente observável pelo store
// devolve ErrStale; não há uso de resultado parcial.
func (s *CompleteMutationService) RevalidateConflicts(ctx context.Context, token string, diagnostic ConflictDiagnostic) error {
	if s == nil || s.service == nil || ctx == nil || diagnostic.store != s.service.config.Store || diagnostic.snapshot.stamp == nil {
		return ErrInvalid
	}
	stamp := diagnostic.snapshot.stamp
	scope := cloneScope(stamp.scope)
	if scope.UserID != diagnostic.snapshot.Scope.UserID || !sameWorkspace(scope.WorkspaceID, diagnostic.snapshot.Scope.WorkspaceID) || stamp.store != diagnostic.store || stamp.providerVersion == "" || diagnostic.userID != scope.UserID {
		return ErrInvalid
	}
	return s.service.config.Epochs.Admit(ctx, diagnostic.epoch, func(ctx context.Context) error {
		principal, err := s.service.config.Sessions.AuthenticateLocalAccess(ctx, token)
		if err != nil {
			return err
		}
		if principal.UserID != scope.UserID || principal.UserID != diagnostic.userID || principal.SessionID != diagnostic.sessionID {
			return ErrStale
		}
		if err := s.service.config.Authorize(ctx, principal, cloneScope(scope), Operation("binding_check_conflict")); err != nil {
			return err
		}
		version, err := s.service.config.Version(ctx)
		if err != nil {
			return err
		}
		if version == "" || version != stamp.providerVersion {
			return ErrStale
		}
		options, err := s.projection(ctx, cloneScope(scope))
		if err != nil {
			return err
		}
		activeUserLayerIDs, err := validatePreviewActiveUserLayerIDs(diagnostic.snapshot, options.ActiveUserLayerIDs)
		if err != nil {
			return err
		}
		if !slices.Equal(activeUserLayerIDs, diagnostic.activeUserLayerIDs) {
			return ErrStale
		}
		return nil
	}, func() error {
		return diagnostic.store.CheckCurrent(ctx, diagnostic.snapshot)
	})
}

// canonicalActiveUserLayerIDs transforma a fotografia dinâmica em uma forma
// determinística para comparação. O conjunto é fornecido por uma porta
// confiável, mas ainda é validado para que uma projeção inconsistente falhe
// fechado e não seja confundida com ausência de ativação.
func canonicalActiveUserLayerIDs(ids []string) ([]string, error) {
	result := slices.Clone(ids)
	for _, id := range result {
		if id == "" || strings.TrimSpace(id) != id {
			return nil, ErrInvalid
		}
	}
	// A ordem da porta não é semântica. Ordenar antes de verificar duplicatas
	// torna o stamp estável sem alterar o conjunto projetado.
	slices.Sort(result)
	for i := 1; i < len(result); i++ {
		if result[i-1] == result[i] {
			return nil, ErrInvalid
		}
	}
	return result, nil
}
