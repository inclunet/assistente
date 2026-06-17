package app

import (
	"assistente/controllers"
	"assistente/internal/configdir"
	"assistente/internal/skills"
	"log"
	"os"
)

// ============================================================================
// Skills Management API — delegação para SkillsController
// Os métodos abaixo existem para manter compatibilidade com o Wails Bind
// enquanto a migração para controllers/ está em andamento (Strangler Fig).
// ============================================================================

func (a *App) GetSkills() ([]skills.SkillInfo, error)      { return a.skillsCtrl.GetSkills() }
func (a *App) GetSkill(slug string) (*skills.Skill, error) { return a.skillsCtrl.GetSkill(slug) }
func (a *App) CreateSkill(req controllers.SkillCreateRequest) (string, error) {
	return a.skillsCtrl.CreateSkill(req)
}
func (a *App) DuplicateSkill(slug string) (string, error) { return a.skillsCtrl.DuplicateSkill(slug) }
func (a *App) UpdateSkill(slug string, req controllers.SkillCreateRequest) error {
	return a.skillsCtrl.UpdateSkill(slug, req)
}
func (a *App) DeleteSkill(slug string) error { return a.skillsCtrl.DeleteSkill(slug) }
func (a *App) GetUserInvocableSkills() ([]skills.SkillInfo, error) {
	return a.skillsCtrl.GetUserInvocableSkills()
}
func (a *App) GetUserInvocableSkillsForProfile(profileSlug string) ([]skills.SkillInfo, error) {
	return a.skillsCtrl.GetUserInvocableSkillsForProfile(profileSlug)
}
func (a *App) GetSkillSearchPaths() []string { return a.skillsCtrl.GetSkillSearchPaths() }

// ============================================================================
// Skills — funções de inicialização (internas ao App)
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
