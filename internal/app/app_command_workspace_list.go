package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

// startWorkspaceList é a única operação compartilhada de leitura do catálogo.
// Para sessões locais preserva a comparação completa user+session. Chamadas
// externas usam exclusivamente o envelope admitido pelo executor e a projeção
// de usuário autenticado no contexto; a sessão local serve só para verificar
// que o manager montado pertence ao mesmo usuário, não como identidade externa.
func (a *App) startWorkspaceList(ctx context.Context, invocation commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
	if ctx == nil {
		return commandexecution.ExecutionHandle{}, commandexecution.ErrInvalidRequest
	}
	ownerID, external, err := a.validateWorkspaceListInvocation(ctx, invocation)
	if err != nil {
		return commandexecution.ExecutionHandle{}, err
	}
	manager, err := a.workspaceListManager(ownerID, external)
	if err != nil {
		return commandexecution.ExecutionHandle{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return commandexecution.ExecutionHandle{}, err
	}
	workCtx, cancel := context.WithCancel(ctx)
	done := make(chan commandexecution.Outcome, 1)
	var cancelOnce sync.Once
	go func() {
		defer cancel()
		if workCtx.Err() != nil {
			done <- commandexecution.Outcome{Status: commandledger.Cancelled}
			return
		}
		infos, listErr := manager.List()
		if workCtx.Err() != nil {
			done <- commandexecution.Outcome{Status: commandledger.Cancelled}
			return
		}
		if listErr == nil {
			result := workspaceListResult{Workspaces: make([]CommandWorkspaceMetadata, 0, len(infos))}
			for _, info := range infos {
				result.Workspaces = append(result.Workspaces, CommandWorkspaceMetadata{ID: info.ID, Name: info.Name, Profile: info.Profile, TabCount: info.TabCount, IsActive: info.IsActive})
			}
			payload, marshalErr := json.Marshal(result)
			if marshalErr != nil {
				done <- commandexecution.Outcome{Status: commandledger.Failed}
				return
			}
			if workCtx.Err() != nil {
				done <- commandexecution.Outcome{Status: commandledger.Cancelled}
				return
			}
			// The manager read is asynchronous relative to lifecycle changes.
			// Revalidate the exact owner and manager after building the DTO and
			// immediately before making a successful result observable.
			if err := a.revalidateWorkspaceListOwner(workCtx, ownerID, external, invocation, manager); err != nil {
				done <- commandexecution.Outcome{Status: commandledger.Failed}
				return
			}
			select {
			case done <- commandexecution.Outcome{Status: commandledger.Succeeded, Result: payload}:
				return
			case <-workCtx.Done():
			}
		}
		if workCtx.Err() != nil {
			done <- commandexecution.Outcome{Status: commandledger.Cancelled}
			return
		}
		done <- commandexecution.Outcome{Status: commandledger.Failed}
	}()
	return commandexecution.ExecutionHandle{
		ID:   id.String(),
		Done: done,
		Cancel: func() {
			cancelOnce.Do(cancel)
		},
	}, nil
}

func (a *App) validateWorkspaceListInvocation(ctx context.Context, invocation commandexecution.Invocation) (string, bool, error) {
	if invocation.Envelope != nil && invocation.Envelope.AuthContextType == commandcontract.AuthExternalToken {
		if a == nil {
			return "", true, commandexecution.ErrInvalidConfiguration
		}
		envelope := invocation.Envelope
		userID, scoped := database.UserIDFromContext(ctx)
		admittedPrincipal, admitted := commandexecution.AuthenticatedExternalPrincipalFromContext(ctx)
		if !admitted || admittedPrincipal.UserID != userID || admittedPrincipal.AuthContextID != envelope.AuthContextID ||
			!scoped || userID == "" || invocation.Source != commandcatalog.UI || invocation.Principal.SessionID != "" || invocation.Principal.UserID != userID ||
			envelope.Version != commandcontract.EnvelopeVersion || envelope.InvocationID != invocation.ID || envelope.CorrelationID != invocation.CorrelationID ||
			envelope.CommandID == nil || *envelope.CommandID != commandProductWorkspaceListID || invocation.CommandID != commandProductWorkspaceListID ||
			envelope.UserID == nil || *envelope.UserID != userID || envelope.SessionID != nil || envelope.AuthContextID == "" || strings.TrimSpace(envelope.AuthContextID) != envelope.AuthContextID ||
			envelope.AuthGeneration == "" || envelope.SecurityGeneration == "" || envelope.RegistryVersion == "" || envelope.SourceType == nil || *envelope.SourceType != commandcontract.SourceUI ||
			envelope.ActorType != commandcontract.ActorUser || envelope.ActorID != userID ||
			envelope.BindingIDs == nil {
			return "", true, commandexecution.ErrDenied
		}
		parsed, err := uuid.Parse(userID)
		if err != nil || parsed.Version() != 7 || parsed.Variant() != uuid.RFC4122 || parsed.String() != userID {
			return "", true, commandexecution.ErrDenied
		}
		if envelope.WorkspaceID != nil {
			product := a.commandProduct.Load()
			if product == nil || *envelope.WorkspaceID != product.workspaceID {
				return "", true, commandexecution.ErrDenied
			}
		}
		return userID, true, nil
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil {
		return "", false, err
	}
	if principal != invocation.Principal {
		return "", false, commandexecution.ErrDenied
	}
	return principal.UserID, false, nil
}

func (a *App) workspaceListManager(userID string, external bool) (*workspace.Manager, error) {
	if a == nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	a.authMu.RLock()
	manager := a.workspaceMgr
	if external {
		// The external principal has no local SessionID. Check only that the
		// currently mounted workspace manager is still for that local user.
		if a.currentUserID != userID || a.currentAuthUser == nil || a.currentAuthUser.UserID != userID {
			a.authMu.RUnlock()
			return nil, commandexecution.ErrDenied
		}
	}
	a.authMu.RUnlock()
	if manager == nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	if external {
		product := a.commandProduct.Load()
		if product == nil || product.workspaceMgr != manager || product.principal.UserID != userID {
			return nil, commandexecution.ErrDenied
		}
	}
	return manager, nil
}

func (a *App) revalidateWorkspaceListOwner(ctx context.Context, userID string, external bool, invocation commandexecution.Invocation, expectedManager *workspace.Manager) error {
	if !external {
		principal, err := a.currentCommandPrincipal()
		if err != nil || principal != invocation.Principal {
			return commandexecution.ErrDenied
		}
		a.authMu.RLock()
		sameManager := a.workspaceMgr == expectedManager
		a.authMu.RUnlock()
		if !sameManager {
			return commandexecution.ErrDenied
		}
		return nil
	}
	ctxUserID, ok := database.UserIDFromContext(ctx)
	// Revalidate the mounted owner and the same runtime/manager pair after the
	// asynchronous read. Authentication/revocation itself remains owned by
	// ExternalService's epoch gate for the whole invocation.
	a.authMu.RLock()
	sameOwner := a.currentUserID == userID && a.currentAuthUser != nil && a.currentAuthUser.UserID == userID
	manager := a.workspaceMgr
	a.authMu.RUnlock()
	product := a.commandProduct.Load()
	if !ok || ctxUserID != userID || !sameOwner || manager != expectedManager || product == nil || product.workspaceMgr != manager || product.principal.UserID != userID {
		return commandexecution.ErrDenied
	}
	return nil
}
