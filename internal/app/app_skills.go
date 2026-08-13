package app

import (
	"assistente/internal/configdir"
	"assistente/internal/logging"
	"assistente/internal/skills"
	skillloadertool "assistente/internal/tools/skillloader"
	"context"
	"os"
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

// initMemoryDir garante que o diretório memory/ existe no home (~/.assistente/memory/)
// e cria o arquivo memory.md inicial se não existir.
func (a *App) initMemoryDir() {
	resolver := configdir.NewResolver("memory")

	if err := resolver.EnsureHomeDir(); err != nil {
		logging.Errorf(context.Background(), "app.app-skills", "[Memory] Erro ao criar diretório de memória: %v", err)
		return
	}

	if !resolver.Exists("memory.md") {
		initial := []byte("## Sobre o Usuário\n\n(Ainda não há memórias salvas. Quando o usuário compartilhar informações pessoais ou pedir para lembrar algo, registre aqui.)\n")
		if err := resolver.Create("memory.md", initial); err != nil {
			logging.Errorf(context.Background(), "app.app-skills", "[Memory] Erro ao criar memory.md: %v", err)
		} else {
			logging.Infof(context.Background(), "app.app-skills", "[Memory] memory.md criado em ~/.assistente/memory/")
		}
	} else {
		logging.Infof(context.Background(), "app.app-skills", "[Memory] memory.md encontrado")
	}

	homeDir := resolver.GetHomeDir()
	if homeDir != "" {
		for _, sub := range []string{"daily", "weekly", "monthly", "yearly"} {
			subPath := homeDir + string(os.PathSeparator) + sub
			if err := os.MkdirAll(subPath, 0755); err != nil {
				logging.Errorf(context.Background(), "app.app-skills", "[Memory] Erro ao criar %s: %v", sub, err)
			}
		}
	}
}
