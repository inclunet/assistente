package skills

import (
	"context"
	"sort"
	"strings"

	"assistente/internal/contextprovider"
)

const defaultContextProviderBudget = 50000
const baseSkillTruncationNotice = "... Additional base skill content omitted due to context budget."
const availableSkillsTruncationNotice = "... Additional on-demand skill catalog content omitted due to context budget."

type ContextProviderSource interface {
	GetAllSkillsFull() ([]Skill, error)
	GetSkillFiles(slug string) ([]string, error)
}

type ContextProvider struct {
	source ContextProviderSource
}

func NewContextProvider(source ContextProviderSource) *ContextProvider {
	return &ContextProvider{source: source}
}

const ContextProviderName = "skills"

func (p *ContextProvider) Name() string { return ContextProviderName }

func (p *ContextProvider) Metadata() contextprovider.ProviderMetadata {
	return contextprovider.ProviderMetadata{
		Name:             p.Name(),
		DisplayName:      "Skills",
		Description:      "Base skill instructions and compact catalog of on-demand skills.",
		DefaultEnabled:   true,
		DefaultBudget:    defaultContextProviderBudget,
		SupportsSettings: false,
	}
}

func (p *ContextProvider) Build(ctx context.Context, req contextprovider.BuildRequest) ([]contextprovider.Block, error) {
	if p == nil || p.source == nil {
		return nil, nil
	}
	allSkills, err := p.source.GetAllSkillsFull()
	if err != nil {
		return nil, err
	}
	baseSkills, onDemandSkills := selectPromptSkills(allSkills, req)
	if len(baseSkills) == 0 && len(onDemandSkills) == 0 {
		return nil, nil
	}

	budget := req.Budget(p.Name(), defaultContextProviderBudget)
	catalogContent := buildAvailableSkillsBlock(onDemandSkills, p.source)
	blocks := make([]contextprovider.Block, 0, 2)
	baseBudget := budget
	if catalogContent != "" {
		reserve := contextprovider.MinimalTaggedBlockLen("available_skills", availableSkillsTruncationNotice)
		if budget > reserve {
			baseBudget = budget - reserve
		}
	}
	baseContent, err := buildBaseSkillBlock(ctx, baseSkills, p.source, baseBudget)
	if err != nil {
		return nil, err
	}
	if baseContent != "" {
		baseContent = contextprovider.TrimTaggedBlockToBudget(baseContent, "base_skill", baseSkillTruncationNotice, baseBudget)
		if baseContent != "" {
			blocks = append(blocks, contextprovider.Block{
				Provider:   p.Name(),
				Name:       "base_skill",
				Volatility: contextprovider.VolatilityStable,
				Priority:   0,
				Content:    baseContent,
			})
		}
	}

	remainingBudget := budget - contextprovider.RuneLen(baseContent)
	if remainingBudget <= 0 {
		return blocks, nil
	}
	if catalogContent != "" {
		catalogContent = contextprovider.TrimTaggedBlockToBudget(catalogContent, "available_skills", availableSkillsTruncationNotice, remainingBudget)
		if catalogContent != "" {
			blocks = append(blocks, contextprovider.Block{
				Provider:   p.Name(),
				Name:       "available_skills",
				Volatility: contextprovider.VolatilityStable,
				Priority:   5,
				Content:    catalogContent,
			})
		}
	}
	return blocks, nil
}

func selectPromptSkills(allSkills []Skill, req contextprovider.BuildRequest) ([]Skill, []Skill) {
	policy := ResolveSelectionPolicy(allSkills, req.EnabledSkills, req.DisableSkills, req.DisableOnDemand)
	baseSkills := policy.Base
	availableSkills := policy.OnDemand
	if !req.ToolCallingEnabled {
		if req.EnabledSkills == nil {
			compatible := filterSkillsWithoutToolDependencies(append(append([]Skill{}, baseSkills...), availableSkills...))
			baseSkills = nil
			if len(compatible) > 0 {
				baseSkills = compatible[:1]
			}
		} else {
			baseSkills = filterSkillsWithoutToolDependencies(baseSkills)
		}
		availableSkills = nil
	}
	return baseSkills, availableSkills
}

func buildBaseSkillBlock(ctx context.Context, baseSkills []Skill, source ContextProviderSource, budget int) (string, error) {
	if len(baseSkills) == 0 {
		return "", nil
	}
	turn, _ := ctx.Value(commandTurnKey{}).(*commandTurn)
	var runner SkillCommandRunner
	if turn != nil {
		if turn.err != nil {
			return "", turn.err
		}
		runner = turn.run
	}

	// Reserve o quadro completo e as instruções de todas as skills antes do
	// primeiro efeito externo, inclusive o catálogo já reservado pelo chamador.
	const open, close = "<base_skill>\n", "</base_skill>"
	type section struct {
		heading, content, files string
		doc                     commandDocument
	}
	sections := make([]section, len(baseSkills))
	remaining := budget - contextprovider.RuneLen(open+close)
	commands := 0
	for i := range baseSkills {
		skill := &baseSkills[i]
		part := &sections[i]
		var heading, files strings.Builder
		if i > 0 {
			heading.WriteByte('\n')
		}
		writeSkillHeading(&heading, *skill)
		files.WriteByte('\n')
		writeSupportingFiles(&files, source, skill.Slug, "Supporting files (use read_file to access when needed):", "- `", "`")
		part.heading, part.files = heading.String(), files.String()
		content, cached := "", false
		if turn != nil {
			content, cached = turn.base[skill.Slug]
		}
		if !cached {
			doc, err := planCommandLines(ctx, skill, skill.Content, true, runner, nil, "")
			if err != nil {
				return "", err
			}
			part.doc = doc
			content = strings.Join(doc.lines, "\n")
			commands += len(doc.commands)
		}
		part.content = content
		remaining -= contextprovider.RuneLen(part.heading) + contextprovider.RuneLen(content) + contextprovider.RuneLen(part.files)
	}

	var sb strings.Builder
	sb.WriteString(open)
	admitted := turn != nil && commands > 0 && remaining/commands >= 2
	for i, part := range sections {
		count := len(part.doc.commands)
		if admitted && count > 0 {
			quota := (remaining/commands)*count + min(remaining%commands, count)
			content, err := part.doc.render(ctx, &baseSkills[i], runner, true, quota)
			if err != nil {
				turn.err = err
				return "", err
			}
			remaining -= contextprovider.RuneLen(content) - contextprovider.RuneLen(part.content)
			commands -= count
			part.content = content
			turn.base[baseSkills[i].Slug] = content
		}
		sb.WriteString(part.heading)
		sb.WriteString(part.content)
		sb.WriteString(part.files)
	}
	sb.WriteString(close)
	return sb.String(), nil
}

func buildAvailableSkillsBlock(availableSkills []Skill, source ContextProviderSource) string {
	modelInvocable := make([]Skill, 0, len(availableSkills))
	for _, skill := range availableSkills {
		if skill.IsModelInvocable() {
			modelInvocable = append(modelInvocable, skill)
		}
	}
	if len(modelInvocable) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<available_skills>\n")
	sb.WriteString("The user can invoke these on-demand skills with slash commands; you can invoke them by calling `load_skill` when tool calling is available.\n")
	sb.WriteString("Treat this as a lightweight catalog of available workflows; do not assume the full instructions are loaded until a skill is invoked or `load_skill` succeeds.\n")
	sb.WriteString("Do not assume disabled or unlisted skills are available.\n\n")
	for _, skill := range modelInvocable {
		sb.WriteString("- **")
		sb.WriteString(sanitizePromptMetadata(skill.GetDisplayName()))
		sb.WriteString("** (`")
		sb.WriteString(sanitizePromptMetadata(skill.Slug))
		sb.WriteString("`)")
		if skill.Type != "" {
			sb.WriteString(" [")
			sb.WriteString(sanitizePromptMetadata(skill.Type))
			sb.WriteString("]")
		}
		sb.WriteString(": ")
		sb.WriteString(sanitizePromptMetadata(skill.Description))
		sb.WriteString("\n  Identifier: `")
		sb.WriteString(sanitizePromptMetadata(skill.Slug))
		sb.WriteString("`\n")
		writeSupportingFiles(&sb, source, skill.Slug, "  Supporting files:", "    - `", "`")
	}
	sb.WriteString("</available_skills>")
	return sb.String()
}

func writeSkillHeading(sb *strings.Builder, skill Skill) {
	sb.WriteString("## ")
	sb.WriteString(sanitizePromptMetadata(skill.GetDisplayName()))
	if skill.Type != "" {
		sb.WriteString(" [")
		sb.WriteString(sanitizePromptMetadata(skill.Type))
		sb.WriteString("]")
	}
	sb.WriteString("\n")
}

func writeSupportingFiles(sb *strings.Builder, source ContextProviderSource, slug string, heading string, prefix string, suffix string) {
	supplementary, _ := source.GetSkillFiles(slug)
	supplementary = sortedStrings(supplementary)
	if len(supplementary) == 0 {
		return
	}
	sb.WriteString("\n")
	sb.WriteString(heading)
	sb.WriteString("\n")
	for _, file := range supplementary {
		sb.WriteString(prefix)
		sb.WriteString(sanitizeSupportingFilePath(file))
		sb.WriteString(suffix)
		sb.WriteString("\n")
	}
}

func sanitizeSupportingFilePath(path string) string {
	return sanitizePromptMetadata(path)
}

func sanitizePromptMetadata(value string) string {
	replacer := strings.NewReplacer(
		"\r", " ",
		"\n", " ",
		"`", "'",
		"<", "&lt;",
		">", "&gt;",
	)
	return strings.TrimSpace(replacer.Replace(value))
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func filterSkillsWithoutToolDependencies(input []Skill) []Skill {
	if len(input) == 0 {
		return input
	}
	filtered := make([]Skill, 0, len(input))
	for _, skill := range input {
		if skillDependsOnTools(skill) {
			continue
		}
		filtered = append(filtered, skill)
	}
	return filtered
}

func skillDependsOnTools(skill Skill) bool {
	if skill.Tools != nil {
		if len(skill.Tools.Allowed) > 0 || len(skill.Tools.Denied) > 0 || skill.Tools.BashCommands != nil {
			return true
		}
	}
	if skill.Filesystem != nil {
		if len(skill.Filesystem.Read) > 0 || len(skill.Filesystem.Write) > 0 || len(skill.Filesystem.Deny) > 0 {
			return true
		}
	}
	if skill.Network != nil {
		if len(skill.Network.AllowedHosts) > 0 || len(skill.Network.DeniedHosts) > 0 {
			return true
		}
	}
	return skill.MCP != nil
}
