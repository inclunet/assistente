package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/apidto"
	"context"
	"sync"
)

// Signal é o bind Wails do domínio Signal (AEP-0088): provisionamento de
// contas via signal-cli-rest-api (registro, verificação, link de dispositivo).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
type Signal struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.SignalController
}

// NewSignal cria o bind vazio; AttachSignal preenche session + controller no startup.
func NewSignal() *Signal {
	return &Signal{}
}

// AttachSignal associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachSignal(s *Signal, session Session, ctrl *controllers.SignalController) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = session
	s.ctrl = ctrl
}

func (s *Signal) deps() (Session, *controllers.SignalController, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.session == nil || s.ctrl == nil {
		return nil, nil, ErrSignalNotWired
	}
	return s.session, s.ctrl, nil
}

// SignalRegister inicia o registro de uma conta Signal via signal-cli-rest-api.
func (s *Signal) SignalRegister(apiURL, number, mode, captcha, apiToken string) error {
	session, ctrl, err := s.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SignalRegister(apiURL, number, mode, captcha, apiToken)
	})
	return err
}

// SignalVerify verifica o código recebido via SMS/ligação.
func (s *Signal) SignalVerify(apiURL, number, code, apiToken string) error {
	session, ctrl, err := s.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SignalVerify(apiURL, number, code, apiToken)
	})
	return err
}

// SignalLink gera o QR code para vincular como dispositivo secundário.
func (s *Signal) SignalLink(apiURL, deviceName, apiToken string) (string, error) {
	session, ctrl, err := s.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return ctrl.SignalLink(apiURL, deviceName, apiToken)
	})
}

// SignalLinkRaw gera a URI texto para vincular como dispositivo secundário.
func (s *Signal) SignalLinkRaw(apiURL, deviceName, apiToken string) (string, error) {
	session, ctrl, err := s.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return ctrl.SignalLinkRaw(apiURL, deviceName, apiToken)
	})
}

// SignalUnregister remove uma conta da signal-cli-rest-api.
func (s *Signal) SignalUnregister(apiURL, number string, deleteLocalData bool, apiToken string) error {
	session, ctrl, err := s.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SignalUnregister(apiURL, number, deleteLocalData, apiToken)
	})
	return err
}

// SignalCheckAPI verifica se a signal-cli-rest-api está acessível na URL informada.
func (s *Signal) SignalCheckAPI(apiURL, apiToken string) (apidto.SignalAPIStatus, error) {
	session, ctrl, err := s.deps()
	if err != nil {
		return apidto.SignalAPIStatus{}, err
	}
	return WithUser(session, func(ctx context.Context) (apidto.SignalAPIStatus, error) {
		return ctrl.SignalCheckAPI(apiURL, apiToken)
	})
}

// SignalListAccounts retorna as contas já registradas na signal-cli-rest-api.
func (s *Signal) SignalListAccounts(apiURL, apiToken string) ([]string, error) {
	session, ctrl, err := s.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]string, error) {
		return ctrl.SignalListAccounts(apiURL, apiToken)
	})
}
