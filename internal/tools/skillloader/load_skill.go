package skillloader

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/profiles"
	"assistente/internal/skills"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
)

const ToolName = tools.LoadSkillName

type SkillManager interface {
	GetAllSkillsFull() ([]skills.Skill, error)
}

type ProfileManager interface {
	Get(slug string) (*profiles.Profile, error)
	GetActive() (*profiles.Profile, error)
}

type Tool struct {
	skills   SkillManager
	profiles ProfileManager
}

type request struct {
	Skill  string `json:"skill"`
	Reason string `json:"reason,omitempty"`
}

func New(skillMgr SkillManager, profileMgr ProfileManager) *Tool {
	return &Tool{skills: skillMgr, profiles: profileMgr}
}

func (t *Tool) Name() string { return ToolName }

func (t *Tool) Description() string {
	return "Load an enabled on-demand skill into the current turn. Use this when the task matches a skill from the prompt's skill catalog and you need the full skill instructions before continuing. Only profile-enabled on-demand skills can be loaded. This is a runtime control tool with ordering semantics: when load_skill appears in the same tool batch as other tool calls, the runtime executes load_skill first and applies the loaded skill's permissions/context before running the remaining calls, while preserving the original result order in the conversation."
}

func (t *Tool) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "skill": {"type": "string", "description": "Skill slug or canonical skill name to load."},
    "reason": {"type": "string", "description": "Brief reason why this skill is needed for the current task. If you need additional tools in the same assistant turn, include load_skill in the same batch; the runtime will execute it first and apply its permissions before the other calls."}
  },
  "required": ["skill"]
}`)
}

func (t *Tool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	if t.skills == nil || t.profiles == nil {
		return tools.ToolResult{Content: "runtime de skills não configurado", IsError: true}, nil
	}
	var req request
	if len(args) > 0 {
		if err := json.Unmarshal(args, &req); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("argumentos inválidos para load_skill: %v", err), IsError: true}, nil
		}
	}
	name := strings.TrimSpace(req.Skill)
	if name == "" {
		return tools.ToolResult{Content: "skill é obrigatória", IsError: true}, nil
	}

	profile, err := t.resolveProfile(ctx)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("não foi possível resolver o perfil para carregar skill: %v", err), IsError: true}, nil
	}
	if profile != nil && (profile.Chat.DisableSkills || profile.Chat.DisableOnDemandSkills) {
		return tools.ToolResult{Content: "carregamento sob demanda de skills está desativado neste perfil", IsError: true}, nil
	}

	allSkills, err := t.skills.GetAllSkillsFull()
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("erro ao listar skills: %v", err), IsError: true}, nil
	}
	var enabledSkills []string
	var disableSkills bool
	var disableOnDemand bool
	if profile != nil {
		enabledSkills = profile.Chat.EnabledSkills
		disableSkills = profile.Chat.DisableSkills
		disableOnDemand = profile.Chat.DisableOnDemandSkills
	}
	policy := skills.ResolveSelectionPolicy(allSkills, enabledSkills, disableSkills, disableOnDemand)
	if policy.ModeFor(name) != skills.SkillModeOnDemand {
		return tools.ToolResult{Content: fmt.Sprintf("skill %q não está disponível como on_demand neste perfil", name), IsError: true}, nil
	}

	loaded, ok := findSkill(policy.OnDemand, name)
	if !ok {
		return tools.ToolResult{Content: fmt.Sprintf("skill %q não encontrada", name), IsError: true}, nil
	}
	if !loaded.IsModelInvocable() {
		return tools.ToolResult{Content: fmt.Sprintf("skill %q não permite autoativação pelo modelo", loaded.Slug), IsError: true}, nil
	}

	content := formatLoadedSkill(loaded, strings.TrimSpace(req.Reason))
	metadata := map[string]any{
		"skill_slug": loaded.Slug,
		"skill_name": loaded.GetDisplayName(),
		"mode":       string(skills.SkillModeOnDemand),
	}
	if reason := strings.TrimSpace(req.Reason); reason != "" {
		metadata["reason"] = reason
	}
	if loaded.Filesystem != nil {
		metadata["filesystem_read"] = append([]string{}, loaded.Filesystem.Read...)
		metadata["filesystem_write"] = append([]string{}, loaded.Filesystem.Write...)
		metadata["filesystem_deny"] = append([]string{}, loaded.Filesystem.Deny...)
	}
	return tools.ToolResult{Content: content, Metadata: metadata}, nil
}

func (t *Tool) resolveProfile(ctx context.Context) (*profiles.Profile, error) {
	if inv, ok := invocationctx.Get(ctx); ok {
		if slug := strings.TrimSpace(inv.ProfileSlug); slug != "" {
			if profile, err := t.profiles.Get(slug); err == nil {
				return profile, nil
			}
		}
	}
	return t.profiles.GetActive()
}

func findSkill(input []skills.Skill, name string) (skills.Skill, bool) {
	name = strings.TrimSpace(name)
	for _, skill := range input {
		if skill.Slug == name || skill.Name == name {
			return skill, true
		}
	}
	return skills.Skill{}, false
}

func formatLoadedSkill(skill skills.Skill, reason string) string {
	var sb strings.Builder
	sb.WriteString("<loaded_skill>\n")
	sb.WriteString("slug: ")
	sb.WriteString(skill.Slug)
	sb.WriteString("\nname: ")
	sb.WriteString(skill.Name)
	if reason != "" {
		sb.WriteString("\nreason: ")
		sb.WriteString(reason)
	}
	sb.WriteString("\n\n")
	sb.WriteString(strings.TrimSpace(skill.Content))
	sb.WriteString("\n</loaded_skill>")
	return sb.String()
}
