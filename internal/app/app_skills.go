package app

import (
	"assistente/internal/agent"
	"assistente/internal/commandcatalog"
	"assistente/internal/database"
	"assistente/internal/logging"
	"assistente/internal/plugins"
	"assistente/internal/skills"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
	skillloadertool "assistente/internal/tools/skillloader"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"slices"
)

// ============================================================================
// Skills — funções de inicialização (internas ao App)
// A superfície Wails do domínio está em wailsapi.Skills (AEP-0088).
// ============================================================================

// initSkills inicializa o gerenciador de skills
func (a *App) initSkills() {
	a.skillMgr = skills.NewManagerWithInstalledSkills(func(ctx context.Context) ([]skills.Skill, error) {
		if _, ok := database.UserIDFromContext(ctx); !ok {
			return nil, nil
		}
		return plugins.NewInventory(database.DB()).ActiveSkills(ctx)
	})
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

func (a *App) runSkillCommand(ctx context.Context, skill *skills.Skill, command string) (string, error) {
	if a.toolInvocationSvc == nil {
		return "", fmt.Errorf("executor de comandos indisponível")
	}
	// A nested execution must pass the caller's restrictions before narrowing to the new skill.
	if err := tools.ValidateExecutionContextToolAccess(ctx, "run_command"); err != nil {
		return "", err
	}
	if parent, ok := tools.GetExecutionContext(ctx); ok {
		if slices.Contains(parent.DeniedBash, command) || (len(parent.AllowedBash) > 0 && !slices.Contains(parent.AllowedBash, command)) {
			return "", fmt.Errorf("comando bloqueado pelo escopo de origem")
		}
	}
	ec := tools.ExecutionContext{InvokedSkillSlug: skill.Slug}
	if skill.Source == "plugin" {
		if skill.CheckCommandActivation == nil {
			return "", plugins.ErrComponentUnavailable
		}
		if err := skill.CheckCommandActivation(ctx); err != nil {
			return "", err
		}
		ec.CheckCommandActivation = skill.CheckCommandActivation
	}
	if skill.Tools != nil {
		ec.AllowedTools = skill.Tools.Allowed
		ec.DeniedTools = skill.Tools.Denied
		if skill.Tools.BashCommands != nil {
			ec.DeniedBash = skill.Tools.BashCommands.Denied
		}
	}
	ctx = tools.WithExecutionContext(ctx, ec)
	args, err := json.Marshal(map[string]any{"command": command, "timeout_seconds": int(skills.DefaultCommandTimeout.Seconds())})
	if err != nil {
		return "", err
	}
	inv, ok := invocationctx.Get(ctx)
	execID := agent.TurnExecutionID(ctx)
	if !ok || inv.TurnID == "" || inv.ConversationID == "" || execID == "" {
		return "", fmt.Errorf("comando de skill requer um turno ativo")
	}
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	redacted := "{}"
	receipt := a.toolInvocationSvc.Execute(ctx, toolinvocations.ExecuteRequest{
		Call:                  tools.ToolCall{ID: id.String(), Type: "function", Function: tools.FunctionCall{Name: "run_command", Arguments: string(args)}},
		Origin:                toolinvocations.Origin{Type: toolinvocations.OriginChat, ID: inv.TurnID, TurnID: inv.TurnID, ConversationID: inv.ConversationID, ChatExecutionID: &execID},
		PersistedArguments:    &redacted,
		SensitivePaths:        commandcatalog.SensitivePaths{Output: []string{""}},
		RequireCompleteResult: true, ExecutionMaxResultSize: skills.MaxCommandOutputSize,
	})
	if !receipt.Persisted {
		return "", fmt.Errorf("falha ao registrar o comando da skill %q", skill.Slug)
	}
	result := receipt.Execution
	if result.Error != nil || result.Result.IsError {
		return "", fmt.Errorf("execução do comando da skill %q interrompida", skill.Slug)
	}
	if code, ok := result.Result.Metadata["exitCode"].(int); ok && code != 0 {
		return "", fmt.Errorf("comando da skill %q terminou com código %d", skill.Slug, code)
	}
	return result.Result.Content, nil
}
