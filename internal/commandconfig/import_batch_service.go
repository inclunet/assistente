package commandconfig

import (
	"context"
	"errors"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/userctx"
)

// ImportBatch autoriza todos os escopos antes de ler o primeiro. As decisões
// continuam vinculadas a cada diff; recusar qualquer uma impede todo o lote.
// O limite protege o gate e não autoriza criar escopos ausentes implicitamente.
func (s *CompleteMutationService) ImportBatch(ctx context.Context, token string, workspaces []*string, provider ImportedSnapshotProvider, revalidate ImportRevalidator) ([]MutationDiff, error) {
	if s == nil || s.service == nil || ctx == nil || provider == nil || revalidate == nil || len(workspaces) == 0 || len(workspaces) > 64 {
		return nil, ErrInvalid
	}
	config := s.service.config
	scopes := make([]Scope, len(workspaces))
	seen := make(map[string]bool, len(workspaces))
	for i, workspace := range workspaces {
		key := importScopeKey(workspace)
		if seen[key] {
			return nil, ErrInvalid
		}
		seen[key] = true
		scopes[i].WorkspaceID = cloneWorkspace(workspace)
	}
	var principal auth.LocalSessionPrincipal
	authorize := func(ctx context.Context, current auth.LocalSessionPrincipal) error {
		for i := range scopes {
			scopes[i].UserID = current.UserID
			if !validScope(scopes[i]) {
				return ErrInvalid
			}
			if err := config.Authorize(ctx, current, cloneScope(scopes[i]), ConfigImport); err != nil {
				return err
			}
		}
		return nil
	}
	epoch, err := config.Epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		current, err := config.Sessions.AuthenticateLocalAccess(ctx, token)
		if err != nil {
			return "", "", err
		}
		principal = current
		if err := authorize(ctx, current); err != nil {
			return "", "", err
		}
		return current.UserID, current.SessionID, nil
	})
	if err != nil {
		return nil, err
	}
	ctx = userctx.WithUserID(ctx, principal.UserID)
	authenticate := func(ctx context.Context) error {
		current, err := config.Sessions.AuthenticateLocalAccess(ctx, token)
		if err != nil {
			return err
		}
		if current.UserID != principal.UserID || current.SessionID != principal.SessionID {
			return ErrStale
		}
		return authorize(ctx, current)
	}
	var prepared []*PreparedMutation
	before := make([]Snapshot, len(scopes))
	after := make([]Snapshot, len(scopes))
	var version string
	validateCombined := func(ctx context.Context) error {
		var global *Snapshot
		for i := range after {
			if after[i].Scope.WorkspaceID == nil {
				global = &after[i]
			}
		}
		for _, snapshot := range after {
			if global != nil && snapshot.Scope.WorkspaceID != nil {
				snapshot = importFinalUnion(snapshot, *global)
			}
			if err := config.Validate(ctx, cloneConfigSnapshot(snapshot)); err != nil {
				return err
			}
		}
		return nil
	}
	err = config.Epochs.Admit(ctx, epoch, authenticate, func() error {
		var err error
		version, err = config.Version(ctx)
		if err != nil {
			return err
		}
		if version == "" {
			return ErrInvalid
		}
		for i, scope := range scopes {
			before[i], err = config.Store.Load(ctx, scope)
			if err != nil {
				return err
			}
			imported, err := provider(ctx, cloneScope(scope), cloneConfigSnapshot(before[i]))
			if err != nil {
				return err
			}
			p, err := config.Store.prepareImportedSnapshot(ctx, scope, before[i], imported, config.Validate)
			if errors.Is(err, ErrNoChanges) {
				after[i] = cloneConfigSnapshot(before[i])
				continue
			}
			if err != nil {
				return err
			}
			prepared = append(prepared, p)
			after[i] = cloneConfigSnapshot(p.after)
		}
		if err := validateCombined(ctx); err != nil {
			return err
		}
		if len(prepared) == 0 {
			return ErrNoChanges
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	watch, release, err := config.Epochs.WatchEpoch(ctx, epoch)
	if err != nil {
		return nil, err
	}
	defer release()
	confirmed := make([]*ConfirmedMutation, 0, len(prepared))
	// Um prazo para o lote inteiro: aceitar o último diálogo não prolonga os
	// anteriores. Nenhuma confirmação implica gravação parcial.
	expires := time.Now().Add(config.DecisionTTL)
	for _, p := range prepared {
		c, err := config.Store.ConfirmMutation(watch, p, epoch, config.Receipts, config.KeyVersion, config.Keys, expires, config.Render)
		if err != nil {
			return nil, err
		}
		confirmed = append(confirmed, c)
	}
	err = config.Epochs.AdmitMutation(ctx, epoch, func(ctx context.Context) error {
		if err := authenticate(ctx); err != nil {
			return err
		}
		current, err := config.Version(ctx)
		if err != nil {
			return err
		}
		if current != version {
			return ErrStale
		}
		for i, scope := range scopes {
			if err := config.Store.CheckCurrent(ctx, before[i]); err != nil {
				return err
			}
			if err := revalidate(ctx, cloneScope(scope)); err != nil {
				return err
			}
		}
		if err := validateCombined(ctx); err != nil {
			return err
		}
		// Portas do host podem revogar a sessão durante a revalidação.
		if err := authenticate(ctx); err != nil {
			return err
		}
		verified, err := config.Sessions.AuthenticateLocalAccess(ctx, token)
		if err != nil {
			return err
		}
		if verified.UserID != principal.UserID || verified.SessionID != principal.SessionID {
			return ErrStale
		}
		return nil
	}, func() error {
		if config.BeforeCommit != nil {
			// Um lote invalida o mapa do usuário inteiro, uma única vez.
			if err := config.BeforeCommit(ctx, Scope{UserID: principal.UserID}); err != nil {
				return err
			}
		}
		return config.Store.CommitConfirmedImportBatch(ctx, confirmed, epoch, config.OnMutationTx)
	})
	if err != nil {
		return nil, err
	}
	diffs := make([]MutationDiff, len(prepared))
	for i, p := range prepared {
		diffs[i] = p.Diff()
	}
	return diffs, nil
}

// Prova a união FINAL de global+workspace, não apenas dois previews isolados.
func importFinalUnion(workspace, global Snapshot) Snapshot {
	result := cloneConfigSnapshot(workspace)
	w := workspace.Scope.WorkspaceID
	result.Layers = append(exactImportRows(result.Layers, w, func(x Layer) *string { return x.WorkspaceID }), global.Layers...)
	result.Bindings = append(exactImportRows(result.Bindings, w, func(x Binding) *string { return x.WorkspaceID }), global.Bindings...)
	result.ActivationRules = append(exactImportRows(result.ActivationRules, w, func(x commandactivation.Rule) *string { return x.WorkspaceID }), global.ActivationRules...)
	result.AutomationGrants = append(exactImportRows(result.AutomationGrants, w, func(x commandautomation.Grant) *string { return x.Owner.WorkspaceID }), global.AutomationGrants...)
	result.ActivationClaims = append(exactImportRows(result.ActivationClaims, w, func(x commandactivation.Claim) *string { return x.WorkspaceID }), global.ActivationClaims...)
	return result
}
