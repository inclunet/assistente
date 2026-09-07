package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/apidto"
	"assistente/internal/skills"
	"context"
	"sync"
)

// Skills é o bind Wails do domínio skills (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
type Skills struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.SkillsController
}

// NewSkills cria o bind vazio; AttachSkills preenche session + controller no startup.
func NewSkills() *Skills {
	return &Skills{}
}

// AttachSkills associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachSkills(s *Skills, session Session, ctrl *controllers.SkillsController) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = session
	s.ctrl = ctrl
}

func (s *Skills) deps() (Session, *controllers.SkillsController, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.session == nil || s.ctrl == nil {
		return nil, nil, ErrSkillsNotWired
	}
	return s.session, s.ctrl, nil
}

// GetSkills retorna a lista de skills disponíveis.
func (s *Skills) GetSkills() ([]skills.SkillInfo, error) {
	session, ctrl, err := s.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]skills.SkillInfo, error) {
		return ctrl.GetSkills()
	})
}

// GetSkill retorna um skill pelo slug.
func (s *Skills) GetSkill(slug string) (*skills.Skill, error) {
	session, ctrl, err := s.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*skills.Skill, error) {
		return ctrl.GetSkill(slug)
	})
}

// CreateSkill cria um novo skill.
func (s *Skills) CreateSkill(req apidto.SkillCreateRequest) (string, error) {
	session, ctrl, err := s.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return ctrl.CreateSkill(req)
	})
}

// DuplicateSkill duplica um skill existente.
func (s *Skills) DuplicateSkill(slug string) (string, error) {
	session, ctrl, err := s.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return ctrl.DuplicateSkill(slug)
	})
}

// UpdateSkill atualiza um skill existente.
func (s *Skills) UpdateSkill(slug string, req apidto.SkillCreateRequest) error {
	session, ctrl, err := s.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UpdateSkill(slug, req)
	})
	return err
}

// DeleteSkill exclui um skill.
func (s *Skills) DeleteSkill(slug string) error {
	session, ctrl, err := s.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.DeleteSkill(slug)
	})
	return err
}

// GetUserInvocableSkillsForProfile retorna skills invocáveis via slash para o perfil.
func (s *Skills) GetUserInvocableSkillsForProfile(profileSlug string) ([]skills.SkillInfo, error) {
	session, ctrl, err := s.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]skills.SkillInfo, error) {
		return ctrl.GetUserInvocableSkillsForProfile(profileSlug)
	})
}

// GetSkillSearchPaths retorna os caminhos de busca de skills.
func (s *Skills) GetSkillSearchPaths() ([]string, error) {
	session, ctrl, err := s.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]string, error) {
		return ctrl.GetSkillSearchPaths(), nil
	})
}
