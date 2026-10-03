package wailsapi

import (
	"context"
	"sync"
)

type welcomeController interface {
	NeedsWelcomeWizard(ctx context.Context) bool
	RunWelcomeWizard(ctx context.Context) (bool, error)
}

// WelcomeRuntime fornece deps de ciclo de vida / instance-wide para o wizard
// dual-mode (AEP-0088). Implementado pelo App via adapter privado — não entra
// no Bind Wails.
type WelcomeRuntime interface {
	AppContext() context.Context
	IsLoggedIn() bool
	HasMasterKey() (bool, error)
	UserCount() (int64, error)
	ProviderCount(ctx context.Context) (int64, error)
}

// Welcome é o bind Wails do domínio welcome (AEP-0088).
//
// NeedsWelcomeWizard é dual-mode pré/pós-login e NÃO usa WithUser: pré-login
// só consulta master key + userCount (instance-wide); pós-login usa Session
// (AuthenticatedContext) + controller/providers. Por isso o método vive neste
// bind e NÃO entra em UnauthenticatedAppMethods (allowlist só de *App).
//
// RunWelcomeWizard exige sessão autenticada, pois a configuração do provedor
// e sua credencial são gravadas no escopo do usuário.
type Welcome struct {
	mu      sync.RWMutex
	session Session
	ctrl    welcomeController
	runtime WelcomeRuntime
}

// NewWelcome cria o bind vazio; AttachWelcome preenche deps no startup.
func NewWelcome() *Welcome {
	return &Welcome{}
}

// AttachWelcome associa Session, controller e runtime após o startup.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachWelcome(w *Welcome, session Session, ctrl welcomeController, runtime WelcomeRuntime) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.session = session
	w.ctrl = ctrl
	w.runtime = runtime
}

// EvaluateNeedsWelcomeWizard replica a lógica dual-mode do wizard (fail-safe true).
// Exposta para o CLI (cmd/asst) reutilizar sem método no *App / Bind Wails.
func EvaluateNeedsWelcomeWizard(session Session, ctrl welcomeController, runtime WelcomeRuntime) bool {
	if runtime == nil {
		return true
	}

	hasMasterKey, err := runtime.HasMasterKey()
	if err != nil {
		return true
	}

	if !runtime.IsLoggedIn() {
		userCount, err := runtime.UserCount()
		if err != nil {
			return true
		}
		return !hasMasterKey || userCount == 0
	}

	if session == nil {
		return true
	}
	ctx, err := session.AuthenticatedContext()
	if err != nil {
		return true
	}
	if ctrl != nil {
		return ctrl.NeedsWelcomeWizard(ctx)
	}
	count, err := runtime.ProviderCount(ctx)
	if err != nil {
		return true
	}
	return count == 0 || !hasMasterKey
}

// NeedsWelcomeWizard verifica se o assistente precisa do wizard de boas-vindas.
// Bind vazio / runtime nil → true (fail-safe).
func (w *Welcome) NeedsWelcomeWizard() bool {
	w.mu.RLock()
	session := w.session
	ctrl := w.ctrl
	runtime := w.runtime
	w.mu.RUnlock()
	return EvaluateNeedsWelcomeWizard(session, ctrl, runtime)
}

// RunWelcomeWizard executa o wizard de boas-vindas.
// Retorna true se completou com sucesso, false se cancelado.
func (w *Welcome) RunWelcomeWizard() (bool, error) {
	w.mu.RLock()
	session := w.session
	ctrl := w.ctrl
	runtime := w.runtime
	w.mu.RUnlock()
	if ctrl == nil || runtime == nil {
		return false, ErrWelcomeNotWired
	}
	return WithUser(session, ctrl.RunWelcomeWizard)
}
