package skills

import (
	"context"
	"log"
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

func (p *ContextProvider) Name() string { return "skills" }

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

func (p *ContextProvider) Build(_ context.Context, req contextprovider.BuildRequest) ([]contextprovider.Block, error) {
	if p == nil || p.source == nil {
		return nil, nil
	}
	allSkills, err := p.source.GetAllSkillsFull()
	if err != nil {
		log.Printf("[context/skills] erro ao carregar skills: %v", err)
		return nil, err
	}
	baseSkills, onDemandSkills := selectPromptSkills(allSkills, req)
	if len(baseSkills) == 0 && len(onDemandSkills) == 0 {
		return nil, nil
	}

	budget := req.Budget(p.Name(), defaultContextProviderBudget)
	baseContent := buildBaseSkillBlock(baseSkills, p.source)
	catalogContent := buildAvailableSkillsBlock(onDemandSkills, p.source)
	blocks := make([]contextprovider.Block, 0, 2)
	baseBudget := budget
	if catalogContent != "" {
		reserve := minimalTaggedBlockLen("available_skills", availableSkillsTruncationNotice)
		if budget > reserve {
			baseBudget = budget - reserve
		}
	}
	if baseContent != "" {
		baseContent = trimTaggedBlockToBudget(baseContent, "base_skill", baseSkillTruncationNotice, baseBudget)
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

	remainingBudget := budget - runeLen(baseContent)
	if remainingBudget <= 0 {
		return blocks, nil
	}
	if catalogContent != "" {
		catalogContent = trimTaggedBlockToBudget(catalogContent, "available_skills", availableSkillsTruncationNotice, remainingBudget)
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

func buildBaseSkillBlock(baseSkills []Skill, source ContextProviderSource) string {
	if len(baseSkills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<base_skill>\n")
	for i, skill := range baseSkills {
		if i > 0 {
			sb.WriteString("\n")
		}
		writeSkillHeading(&sb, skill)
		content := skill.Content
		var allowedBash []string
		if skill.Tools != nil && skill.Tools.BashCommands != nil {
			allowedBash = skill.Tools.BashCommands.Allowed
		}
		sb.WriteString(PreprocessCommands(content, allowedBash))
		sb.WriteString("\n")
		writeSupportingFiles(&sb, source, skill.Slug, "Supporting files (use read_file to access when needed):", "- `", "`")
	}
	sb.WriteString("</base_skill>")
	return sb.String()
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
		sb.WriteString(skill.GetDisplayName())
		sb.WriteString("** (`")
		sb.WriteString(skill.Slug)
		sb.WriteString("`)")
		if skill.Type != "" {
			sb.WriteString(" [")
			sb.WriteString(skill.Type)
			sb.WriteString("]")
		}
		sb.WriteString(": ")
		sb.WriteString(skill.Description)
		sb.WriteString("\n  Identifier: `")
		sb.WriteString(skill.Slug)
		sb.WriteString("`\n")
		writeSupportingFiles(&sb, source, skill.Slug, "  Supporting files:", "    - `", "`")
	}
	sb.WriteString("</available_skills>")
	return sb.String()
}

func writeSkillHeading(sb *strings.Builder, skill Skill) {
	sb.WriteString("## ")
	sb.WriteString(skill.GetDisplayName())
	if skill.Type != "" {
		sb.WriteString(" [")
		sb.WriteString(skill.Type)
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
		sb.WriteString(file)
		sb.WriteString(suffix)
		sb.WriteString("\n")
	}
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func trimBlockToBudget(content string, budgetChars int) string {
	if budgetChars <= 0 {
		return ""
	}
	content = strings.TrimSpace(content)
	if runeLen(content) <= budgetChars {
		return content
	}
	return ""
}

func trimTaggedBlockToBudget(content string, tag string, truncationNotice string, budgetChars int) string {
	if budgetChars <= 0 {
		return ""
	}
	content = strings.TrimSpace(content)
	if runeLen(content) <= budgetChars {
		return content
	}
	prefix := "<" + tag + ">\n"
	suffix := "\n</" + tag + ">"
	if !strings.HasPrefix(content, prefix) || !strings.HasSuffix(content, suffix) {
		return trimBlockToBudget(content, budgetChars)
	}
	notice := "\n" + truncationNotice
	minimal := prefix + truncationNotice + suffix
	if runeLen(minimal) > budgetChars {
		return truncateRunes(truncationNotice, budgetChars)
	}
	bodyBudget := budgetChars - runeLen(prefix) - runeLen(notice) - runeLen(suffix)
	if bodyBudget <= 0 {
		return minimal
	}
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(content, prefix), suffix))
	bodyRunes := []rune(body)
	if len(bodyRunes) > bodyBudget {
		body = strings.TrimSpace(string(bodyRunes[:bodyBudget]))
	}
	if body == "" {
		return minimal
	}
	return prefix + body + notice + suffix
}

func minimalTaggedBlockLen(tag string, truncationNotice string) int {
	return runeLen("<"+tag+">\n") + runeLen(truncationNotice) + runeLen("\n</"+tag+">")
}

func truncateRunes(value string, budgetChars int) string {
	if budgetChars <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= budgetChars {
		return value
	}
	return strings.TrimSpace(string(runes[:budgetChars]))
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

func runeLen(value string) int {
	count := 0
	for range value {
		count++
	}
	return count
}
