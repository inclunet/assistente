package controllers

import (
	"fmt"

	"assistente/internal/core/ports"
	"assistente/internal/skills"
)

// SkillCreateRequest é o payload para criar/atualizar um skill via frontend.
type SkillCreateRequest struct {
	skills.SkillMetadata `json:",inline"`
	Content              string `json:"content"`
}

// SkillsControllerConfig agrupa as dependências do SkillsController.
type SkillsControllerConfig struct {
	SkillMgr *skills.Manager
	Emitter  ports.Emitter
}

// SkillsController é o adapter primário (Inbound) para operações de skills.
type SkillsController struct {
	skillMgr *skills.Manager
	emitter  ports.Emitter
}

// NewSkillsController cria um SkillsController com suas dependências.
func NewSkillsController(cfg SkillsControllerConfig) *SkillsController {
	return &SkillsController{skillMgr: cfg.SkillMgr, emitter: cfg.Emitter}
}

func (c *SkillsController) guard() error {
	if c.skillMgr == nil {
		return fmt.Errorf("skill manager não inicializado")
	}
	return nil
}

func (c *SkillsController) GetSkills() ([]skills.SkillInfo, error) {
	if err := c.guard(); err != nil {
		return nil, err
	}
	return c.skillMgr.List()
}

func (c *SkillsController) GetSkill(slug string) (*skills.Skill, error) {
	if err := c.guard(); err != nil {
		return nil, err
	}
	return c.skillMgr.Get(slug)
}

func (c *SkillsController) CreateSkill(req SkillCreateRequest) (string, error) {
	if err := c.guard(); err != nil {
		return "", err
	}
	meta := req.SkillMetadata
	slug, err := c.skillMgr.Create(&meta, req.Content)
	if err != nil {
		return "", err
	}
	c.emitter.Emit("skill:created", map[string]interface{}{"slug": slug, "name": req.Name})
	return slug, nil
}

func (c *SkillsController) DuplicateSkill(slug string) (string, error) {
	if err := c.guard(); err != nil {
		return "", err
	}
	newSlug, err := c.skillMgr.Duplicate(slug)
	if err != nil {
		return "", err
	}
	name := ""
	if copied, err := c.skillMgr.Get(newSlug); err == nil && copied != nil {
		name = copied.Name
	}
	c.emitter.Emit("skill:created", map[string]interface{}{"slug": newSlug, "name": name})
	return newSlug, nil
}

func (c *SkillsController) UpdateSkill(slug string, req SkillCreateRequest) error {
	if err := c.guard(); err != nil {
		return err
	}
	meta := req.SkillMetadata
	if err := c.skillMgr.Update(slug, &meta, req.Content); err != nil {
		return err
	}
	c.emitter.Emit("skill:updated", map[string]interface{}{"slug": slug, "name": req.Name})
	return nil
}

func (c *SkillsController) DeleteSkill(slug string) error {
	if err := c.guard(); err != nil {
		return err
	}
	if err := c.skillMgr.Delete(slug); err != nil {
		return err
	}
	c.emitter.Emit("skill:deleted", map[string]interface{}{"slug": slug})
	return nil
}

func (c *SkillsController) GetUserInvocableSkills() ([]skills.SkillInfo, error) {
	if err := c.guard(); err != nil {
		return nil, err
	}
	return c.skillMgr.GetUserInvocableSkills()
}
