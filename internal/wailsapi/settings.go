package wailsapi

import (
	"assistente/controllers"
	"context"
	"sync"
)

// Settings é o bind Wails do domínio settings (AEP-0088).
// Auth só via WithUser / WithAdmin — sem chamar o helper de auth do App no call site.
type Settings struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.SettingsController
}

// NewSettings cria o bind vazio; AttachSettings preenche session + controller no startup.
func NewSettings() *Settings {
	return &Settings{}
}

// AttachSettings associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachSettings(s *Settings, session Session, ctrl *controllers.SettingsController) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = session
	s.ctrl = ctrl
}

func (s *Settings) deps() (Session, *controllers.SettingsController, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.session == nil || s.ctrl == nil {
		return nil, nil, ErrSettingsNotWired
	}
	return s.session, s.ctrl, nil
}

// GetNativeTTSProviders retorna os IDs de provedores TTS nativos da plataforma.
func (s *Settings) GetNativeTTSProviders() ([]string, error) {
	session, ctrl, err := s.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]string, error) {
		return ctrl.GetNativeTTSProviders(), nil
	})
}

// TestConnection verifica se a API do LLM responde com modelos.
func (s *Settings) TestConnection() (bool, error) {
	session, ctrl, err := s.deps()
	if err != nil {
		return false, err
	}
	return WithUser(session, func(ctx context.Context) (bool, error) {
		return ctrl.TestConnection()
	})
}

// TestConnectionWithModels testa a conexão e devolve a lista de modelos.
func (s *Settings) TestConnectionWithModels() ([]string, error) {
	session, ctrl, err := s.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]string, error) {
		return ctrl.TestConnectionWithModels()
	})
}

// ResetConfig remove o arquivo de configuração (requer admin).
func (s *Settings) ResetConfig() error {
	session, ctrl, err := s.deps()
	if err != nil {
		return err
	}
	_, err = WithAdmin(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.ResetConfig()
	})
	return err
}

// ClearAllCredentials apaga as credenciais visíveis ao usuário autenticado.
func (s *Settings) ClearAllCredentials() error {
	session, ctrl, err := s.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.ClearAllCredentials(ctx)
	})
	return err
}

// ClearAllProfiles apaga todos os perfis (requer admin).
func (s *Settings) ClearAllProfiles() error {
	session, ctrl, err := s.deps()
	if err != nil {
		return err
	}
	_, err = WithAdmin(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.ClearAllProfiles()
	})
	return err
}

// ClearAllSkills apaga todas as skills (requer admin).
func (s *Settings) ClearAllSkills() error {
	session, ctrl, err := s.deps()
	if err != nil {
		return err
	}
	_, err = WithAdmin(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.ClearAllSkills()
	})
	return err
}

// ClearAllChannels emite limpeza de canais (requer admin).
func (s *Settings) ClearAllChannels() error {
	session, ctrl, err := s.deps()
	if err != nil {
		return err
	}
	_, err = WithAdmin(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.ClearAllChannels()
	})
	return err
}
