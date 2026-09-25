package app

import (
	"context"
	"errors"
	"time"

	"assistente/internal/auth"
	"assistente/internal/database"
)

var errEditorDialogSessionStale = errors.New("sessão do diálogo do editor ficou obsoleta")

// captureEditorDialogSession captura a identidade e o epoch antes de abrir um
// diálogo nativo. A validação devolvida não mantém authMu nem o gate durante
// o diálogo; a mesma validação é usada antes e depois dele.
func (a *App) captureEditorDialogSession(ctx context.Context) (func() error, error) {
	if a == nil {
		return nil, database.ErrUserScopeRequired
	}
	if ctx == nil {
		return nil, database.ErrUserScopeRequired
	}
	requestedUserID, ok := database.UserIDFromContext(ctx)
	if !ok || requestedUserID == "" {
		return nil, database.ErrUserScopeRequired
	}
	epochs, err := a.commandSecurityService()
	if err != nil {
		return nil, err
	}
	snapshot, err := epochs.CaptureAuthenticated(ctx, func(context.Context) (string, string, error) {
		a.authMu.RLock()
		defer a.authMu.RUnlock()
		if a.currentAuthUser == nil || a.currentUserID == "" ||
			a.currentAuthUser.UserID != a.currentUserID || a.currentAuthUser.SessionID == "" ||
			a.currentAuthUser.UserID != requestedUserID {
			return "", "", database.ErrUserScopeRequired
		}
		return a.currentAuthUser.UserID, a.currentAuthUser.SessionID, nil
	})
	if err != nil {
		return nil, err
	}

	validate := func() error {
		validationCtx, cancel := context.WithTimeout(a.appContext(), 5*time.Second)
		defer cancel()
		return epochs.Admit(validationCtx, snapshot, func(ctx context.Context) error {
			a.authMu.RLock()
			current := a.currentAuthUser
			currentUserID := a.currentUserID
			sessions := a.sessionSvc
			a.authMu.RUnlock()
			if current == nil || currentUserID != snapshot.UserID ||
				current.UserID != snapshot.UserID || current.SessionID != snapshot.SessionID {
				return errEditorDialogSessionStale
			}
			if sessions == nil {
				return database.ErrUserScopeRequired
			}
			principal := auth.LocalSessionPrincipal{UserID: snapshot.UserID, SessionID: snapshot.SessionID}
			validated, err := sessions.RevalidateLocalSession(ctx, principal)
			if err != nil {
				return err
			}
			if validated != principal {
				return errEditorDialogSessionStale
			}
			return nil
		}, func() error { return nil })
	}
	if err := validate(); err != nil {
		return nil, err
	}
	return validate, nil
}
