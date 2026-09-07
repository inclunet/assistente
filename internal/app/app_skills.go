package app

import (
	"assistente/internal/logging"
	"assistente/internal/skills"
	skillloadertool "assistente/internal/tools/skillloader"
	"context"
)

// ============================================================================
// Skills — funções de inicialização (internas ao App)
// A superfície Wails do domínio está em wailsapi.Skills (AEP-0088).
// ============================================================================

// initSkills inicializa o gerenciador de skills
func (a *App) initSkills() {
	a.skillMgr = skills.NewManager()
	if err := a.skillMgr.EnsureDir(); err != nil {
		logging.Errorf(context.Background(), "app.app-skills", "[Skills] Erro ao garantir diretório de skills: %v", err)
	}

	a.installBuiltinSkills()
	if a.toolRegistry != nil && !a.toolRegistry.Has(skillloadertool.ToolName) {
		a.toolRegistry.MustRegisterDiscoverableOptIn(skillloadertool.New(a.skillMgr, a.profileManager))
	}

	list, err := a.skillMgr.List()
	if err != nil {
		logging.Errorf(context.Background(), "app.app-skills", "[Skills] Erro ao listar skills: %v", err)
	} else {
		logging.Infof(context.Background(), "app.app-skills", "[Skills] Manager inicializado com %d skills", len(list))
	}
}
