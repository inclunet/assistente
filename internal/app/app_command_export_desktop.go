package app

import (
	"context"
	"errors"
	"slices"
	"time"

	"assistente/internal/commandexecution"
	"assistente/internal/commandjson"
	"assistente/internal/commandportability"
	"assistente/internal/database"
	"assistente/internal/portability"
	"gorm.io/gorm"
)

var (
	errCommandExportEmpty = errors.New("command_export.empty")
	errCommandExportLimit = errors.New("command_export.limit")
)

// exportCommandLayers é somente leitura. Não promove includeCredentials para
// uma ação sensível e não aceita identidade/catalogo fornecidos pelo cliente.
func (a *App) exportCommandLayers(ctx context.Context, req portability.ExportRequest) (string, error) {
	if a == nil || ctx == nil {
		return "", commandexecution.ErrDenied
	}
	if err := portability.ValidateCommandExportRequest(req); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return "", safeCommandImportError(err)
	}
	user, err := database.RequireUserID(ctx)
	if err != nil || user != p.principal.UserID {
		return "", commandexecution.ErrDenied
	}
	db := database.DB()
	if !commandMutationRootDatabase(db) {
		return "", commandexecution.ErrInvalidConfiguration
	}
	stable := func() bool {
		return database.DB() == db && a.commandProduct.Load() == p && p.dependenciesMatch(a) && a.commandPrincipalMatches(p.sessionSvc, p.credMgr, p.principal)
	}
	revalidate := func(ctx context.Context) error {
		if !stable() {
			return commandexecution.ErrStale
		}
		principal, err := p.sessionSvc.RevalidateLocalSession(ctx, p.principal)
		if err != nil || principal != p.principal {
			return commandexecution.ErrDenied
		}
		state, err := p.host.Snapshot(ctx, principal)
		if err != nil || !state.Unlocked || !stable() {
			return commandexecution.ErrDenied
		}
		return nil
	}
	epoch, err := p.epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		if err := revalidate(ctx); err != nil {
			return "", "", err
		}
		return p.principal.UserID, p.principal.SessionID, nil
	})
	if err != nil {
		return "", safeCommandImportError(err)
	}
	refs, err := a.commandDesktopImportReferences(p, ctx)
	if err != nil {
		return "", safeCommandImportError(err)
	}
	// A seleção e cada snapshot pertencem à mesma transação. Nenhum I/O de
	// exportação, normalização ou serialização fica sob o gate dos atalhos.
	var layers []commandportability.LayerExport
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		principal, err := p.sessionSvc.RevalidateLocalSessionTx(ctx, tx, p.principal)
		if err != nil || principal != p.principal || !stable() {
			return commandexecution.ErrDenied
		}
		transactionRefs := refs
		transactionRefs.CredentialPattern = commandportability.NewCredentialPatternResolver(tx)
		layers, err = commandportability.ExportFromStore(ctx, tx, user, transactionRefs, slices.Clone(req.CommandLayerIDs), req.IncludeWorkspace)
		if err != nil {
			return err
		}
		if len(layers) == 0 {
			return errCommandExportEmpty
		}
		if len(layers) > 64 {
			return errCommandExportLimit
		}
		for _, layer := range layers {
			if layer.Scope.Kind == commandportability.WorkspaceScope && !req.IncludeWorkspace {
				return commandportability.ErrWorkspaceResolution
			}
		}
		return ctx.Err()
	})
	if err != nil {
		if errors.Is(err, errCommandExportEmpty) {
			return "", errCommandExportEmpty
		}
		if errors.Is(err, errCommandExportLimit) {
			return "", errCommandExportLimit
		}
		return "", safeCommandImportError(err)
	}
	raw, err := commandjson.Marshal(portability.ExportFile{Version: portability.ExportVersion, ExportedAt: time.Now().UTC(), Resources: portability.ExportResources{CommandLayers: layers}})
	if err != nil {
		if errors.Is(err, commandjson.ErrDocumentTooLarge) {
			return "", errCommandExportLimit
		}
		return "", safeCommandImportError(err)
	}
	// Reconfere acesso aos workspaces depois da leitura, fora do gate. A
	// publicação final é apenas uma entrega local curta sob a época capturada.
	seen := map[string]bool{}
	for _, layer := range layers {
		id := layer.Scope.WorkspaceID
		if layer.Scope.Kind != commandportability.WorkspaceScope || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := refs.Workspace(ctx, id); err != nil {
			return "", safeCommandImportError(err)
		}
	}
	var result string
	err = p.epochs.Admit(ctx, epoch, revalidate, func() error { result = string(raw); return nil })
	if err != nil {
		return "", safeCommandImportError(err)
	}
	return result, nil
}
