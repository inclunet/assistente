package app

import (
	"context"
	"errors"
	"fmt"

	"assistente/internal/auth"
	"assistente/internal/credentials"
	"assistente/internal/database"
)

// NewConversationCLIApp usa o bootstrap sem serviços autônomos e revalida
// a sessão compartilhada com o desktop a cada comando, inclusive no REPL.
func NewConversationCLIApp() *App {
	a := NewCommandCLIApp()
	a.conversationCLI = true
	return a
}

// RestoreConversationCLISession vincula o processo curto à sessão salva pelo
// desktop. A leitura não renova o prazo nem disputa a rotação do token do desktop.
func RestoreConversationCLISession(a *App, chat bool) error {
	if a == nil || !a.conversationCLI {
		return errors.New("a restauração exige o runtime de CLI")
	}
	status, err := a.GetAuthStatus()
	if err != nil {
		return err
	}
	if !status.VaultConfigured || !status.HasUsers {
		return errors.New("configure a conta local com asst setup")
	}
	if !status.VaultUnlocked {
		return errors.New("desbloqueie o cofre no Assistente para usar a sessão local")
	}
	if err := a.ensureSessionService(); err != nil {
		return err
	}

	ctx := a.appContext()
	principal, role, err := a.savedConversationCLIPrincipal(ctx)
	if err != nil {
		return err
	}
	userCtx := database.WithUserID(ctx, principal.UserID)
	if chat {
		a.llmCatalog.StartOffline(ctx)
		if err := a.credMgr.LoadUserCredentials(userCtx, principal.UserID); err != nil {
			return fmt.Errorf("carregar credenciais: %w", err)
		}
		if err := a.providerSvc.Load(userCtx); err != nil {
			return fmt.Errorf("carregar provedores de chat: %w", err)
		}
	}
	a.setCurrentUserID(principal.UserID)
	a.setCurrentAuthUser(&AuthUser{UserID: principal.UserID, SessionID: principal.SessionID, Role: role})
	return nil
}

func (a *App) savedConversationCLIPrincipal(ctx context.Context) (auth.LocalSessionPrincipal, string, error) {
	// O login confirma a gravação no cofre deste banco antes de tentar
	// atualizar seu espelho no chaveiro, que pode conter outra conta válida.
	token, found, err := a.credMgr.ReadPersistedInstanceSecret(ctx, credentials.InstanceSecretAuthRefreshToken)
	if err != nil {
		return auth.LocalSessionPrincipal{}, "", fmt.Errorf("não foi possível ler a sessão local no cofre: %w", err)
	}
	if !found {
		return auth.LocalSessionPrincipal{}, "", fmt.Errorf("entre na sua conta no Assistente e tente novamente: %w", auth.ErrInvalidRefreshToken)
	}
	service := a.currentSessionService()
	if service == nil {
		return auth.LocalSessionPrincipal{}, "", auth.ErrUnauthenticatedLocalSession
	}
	principal, role, err := service.AuthenticateSavedSession(ctx, token)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidRefreshToken) || errors.Is(err, auth.ErrSessionRevoked) || errors.Is(err, auth.ErrSessionExpired) || errors.Is(err, auth.ErrInactiveUser) {
			return auth.LocalSessionPrincipal{}, "", fmt.Errorf("entre na sua conta no Assistente e tente novamente: %w", err)
		}
		return auth.LocalSessionPrincipal{}, "", err
	}
	return principal, role, nil
}
