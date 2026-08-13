package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/apidto"
	"context"
	"sync"
)

// Credentials é o bind Wails do domínio credentials (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
//
// Métodos pré-sessão de vault (HasMasterKey, SetupMasterPassword,
// GetVaultIntegrityStatus, CanPersistCredentials) permanecem no *App e na
// UnauthenticatedAppMethods — usados no onboarding/CLI sem sessão.
type Credentials struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.CredentialsController
}

// NewCredentials cria o bind vazio; AttachCredentials preenche session + controller no startup.
func NewCredentials() *Credentials {
	return &Credentials{}
}

// AttachCredentials associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachCredentials(c *Credentials, session Session, ctrl *controllers.CredentialsController) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.session = session
	c.ctrl = ctrl
}

func (c *Credentials) deps() (Session, *controllers.CredentialsController, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.session == nil || c.ctrl == nil {
		return nil, nil, ErrCredentialsNotWired
	}
	return c.session, c.ctrl, nil
}

// ListCredentials retorna credenciais registradas (sem valores sensíveis).
func (c *Credentials) ListCredentials() ([]apidto.CredentialSummary, error) {
	session, ctrl, err := c.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]apidto.CredentialSummary, error) {
		return ctrl.ListCredentialsWithContext(ctx)
	})
}

// UpsertCredential cria ou atualiza uma credencial no credential manager.
func (c *Credentials) UpsertCredential(input apidto.CredentialInput) error {
	session, ctrl, err := c.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UpsertCredentialWithContext(ctx, input)
	})
	return err
}

// DeleteCredential remove uma credencial pelo padrão.
func (c *Credentials) DeleteCredential(pattern string) error {
	session, ctrl, err := c.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.DeleteCredentialWithContext(ctx, pattern)
	})
	return err
}

// ListExternalSources lista fontes externas disponíveis para autocomplete.
func (c *Credentials) ListExternalSources(prefix string) ([]apidto.ExternalSourceSuggestion, error) {
	session, ctrl, err := c.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]apidto.ExternalSourceSuggestion, error) {
		return ctrl.ListExternalSources(prefix)
	})
}
