package main

import (
	"assistente/internal/configdir"
	"assistente/internal/skills"
	"fmt"
	"log"
	"os"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ============================================================================
// Skills Management API
// ============================================================================

// initSkills inicializa o gerenciador de skills
func (a *App) initSkills() {
	a.skillMgr = skills.NewManager()
	if err := a.skillMgr.EnsureDir(); err != nil {
		log.Printf("[Skills] Erro ao garantir diretório de skills: %v", err)
	}

	a.installBuiltinSkills()

	list, err := a.skillMgr.List()
	if err != nil {
		log.Printf("[Skills] Erro ao listar skills: %v", err)
	} else {
		log.Printf("[Skills] Manager inicializado com %d skills", len(list))
	}
}

// initMemoryDir garante que o diretório memory/ existe no home (~/.assistente/memory/)
// e cria o arquivo memory.md inicial se não existir.
func (a *App) initMemoryDir() {
	resolver := configdir.NewResolver("memory")

	if err := resolver.EnsureHomeDir(); err != nil {
		log.Printf("[Memory] Erro ao criar diretório de memória: %v", err)
		return
	}

	// Cria memory.md se não existir em nenhum diretório
	if !resolver.Exists("memory.md") {
		initial := []byte("## Sobre o Usuário\n\n(Ainda não há memórias salvas. Quando o usuário compartilhar informações pessoais ou pedir para lembrar algo, registre aqui.)\n")
		if err := resolver.Create("memory.md", initial); err != nil {
			log.Printf("[Memory] Erro ao criar memory.md: %v", err)
		} else {
			log.Printf("[Memory] memory.md criado em ~/.assistente/memory/")
		}
	} else {
		log.Printf("[Memory] memory.md encontrado")
	}

	// Garante que os subdiretórios de memória temporal existem no home
	homeDir := resolver.GetHomeDir()
	if homeDir != "" {
		for _, sub := range []string{"daily", "weekly", "monthly", "yearly"} {
			subPath := homeDir + string(os.PathSeparator) + sub
			if err := os.MkdirAll(subPath, 0755); err != nil {
				log.Printf("[Memory] Erro ao criar %s: %v", sub, err)
			}
		}
	}
}

// GetSkills retorna a lista de skills disponíveis (metadados apenas).
func (a *App) GetSkills() ([]skills.SkillInfo, error) {
	if a.skillMgr == nil {
		return nil, fmt.Errorf("skill manager não inicializado")
	}
	return a.skillMgr.List()
}

// GetSkill retorna um skill completo pelo slug.
func (a *App) GetSkill(slug string) (*skills.Skill, error) {
	if a.skillMgr == nil {
		return nil, fmt.Errorf("skill manager não inicializado")
	}
	return a.skillMgr.Get(slug)
}

// SkillCreateRequest é o payload para criar/atualizar um skill via frontend.
// Contém a SkillMetadata completa conforme spec + conteúdo Markdown.
type SkillCreateRequest struct {
	skills.SkillMetadata `json:",inline"`
	Content              string `json:"content"`
}

// CreateSkill cria um novo skill.
func (a *App) CreateSkill(req SkillCreateRequest) (string, error) {
	if a.skillMgr == nil {
		return "", fmt.Errorf("skill manager não inicializado")
	}

	meta := req.SkillMetadata
	slug, err := a.skillMgr.Create(&meta, req.Content)
	if err != nil {
		return "", err
	}

	runtime.EventsEmit(a.ctx, "skill:created", map[string]interface{}{
		"slug": slug,
		"name": req.Name,
	})

	return slug, nil
}

// DuplicateSkill cria uma copia de um skill existente.
func (a *App) DuplicateSkill(slug string) (string, error) {
	if a.skillMgr == nil {
		return "", fmt.Errorf("skill manager não inicializado")
	}

	newSlug, err := a.skillMgr.Duplicate(slug)
	if err != nil {
		return "", err
	}

	name := ""
	if copied, err := a.skillMgr.Get(newSlug); err == nil && copied != nil {
		name = copied.Name
	}

	runtime.EventsEmit(a.ctx, "skill:created", map[string]interface{}{
		"slug": newSlug,
		"name": name,
	})

	return newSlug, nil
}

// UpdateSkill atualiza um skill existente.
func (a *App) UpdateSkill(slug string, req SkillCreateRequest) error {
	if a.skillMgr == nil {
		return fmt.Errorf("skill manager não inicializado")
	}

	meta := req.SkillMetadata
	if err := a.skillMgr.Update(slug, &meta, req.Content); err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "skill:updated", map[string]interface{}{
		"slug": slug,
		"name": req.Name,
	})

	return nil
}

// DeleteSkill exclui um skill.
func (a *App) DeleteSkill(slug string) error {
	if a.skillMgr == nil {
		return fmt.Errorf("skill manager não inicializado")
	}

	if err := a.skillMgr.Delete(slug); err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "skill:deleted", map[string]interface{}{
		"slug": slug,
	})

	return nil
}

// GetUserInvocableSkills retorna skills que o usuário pode invocar via /slash.
func (a *App) GetUserInvocableSkills() ([]skills.SkillInfo, error) {
	if a.skillMgr == nil {
		return nil, fmt.Errorf("skill manager não inicializado")
	}
	return a.skillMgr.GetUserInvocableSkills()
}

// GetSkillSearchPaths retorna os caminhos de busca de skills.
func (a *App) GetSkillSearchPaths() []string {
	if a.skillMgr == nil {
		return []string{}
	}
	return a.skillMgr.GetSearchPaths()
}
