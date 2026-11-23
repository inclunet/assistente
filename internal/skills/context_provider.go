package skills

import (
	"context"
	"errors"
	"io/fs"
	"slices"
	"sort"
	"strings"

	"assistente/internal/contextprovider"
	"assistente/internal/logging"
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

// A primeira construção prepara as saídas admitidas. Cada construção aplica
// a seleção e o orçamento atuais, inclusive a distinção entre seleção nil e vazia.
type preparedSkillContext struct {
	filesAt       func(string) ([]string, error)
	skills        []Skill
	paths         map[string]string // caminho do manifesto selecionado na preparação
	files         map[string][]string
	sourceErrors  map[string]error          // fontes sem caminho só podem ser consultadas novamente na próxima preparação
	outputs       map[string]map[int]string // slug e posição da diretiva no documento
	last          *preparedPrompt
	listingFailed bool
	ready         bool
	err           error
}

type preparedPrompt struct {
	enabledSkills                               []string
	disableSkills, disableOnDemand, toolCalling bool
	budget                                      int
	blocks                                      []contextprovider.Block
}

func (p *preparedPrompt) matches(req contextprovider.BuildRequest, budget int) bool {
	return p != nil && p.disableSkills == req.DisableSkills && p.disableOnDemand == req.DisableOnDemand &&
		p.toolCalling == req.ToolCallingEnabled && p.budget == budget &&
		(p.enabledSkills == nil) == (req.EnabledSkills == nil) && slices.Equal(p.enabledSkills, req.EnabledSkills)
}

func (s *preparedSkillContext) GetAllSkillsFull() ([]Skill, error) {
	return s.skills, s.err
}

func (s *preparedSkillContext) GetSkillFiles(slug string) ([]string, error) {
	if files, ok := s.files[slug]; ok {
		return files, nil
	}
	path := s.paths[slug]
	if path == "" {
		err := s.sourceErrors[slug]
		if err != nil {
			s.listingFailed = true
		}
		return nil, err
	}
	files, err := s.filesAt(path)
	if err != nil {
		s.listingFailed = true
		return nil, err
	}
	s.files[slug] = slices.Clone(files)
	return s.files[slug], nil
}

// A fatia e as permissões consultadas na seleção e no planejamento são
// privadas. Os demais metadados aninhados permanecem compartilhados para leitura.
func snapshotPromptSkills(skills []Skill) []Skill {
	out := cloneSkills(skills)
	for i := range out {
		skill := &out[i]
		if skill.Tools != nil {
			tools := *skill.Tools
			tools.Allowed = slices.Clone(tools.Allowed)
			tools.Denied = slices.Clone(tools.Denied)
			if tools.BashCommands != nil {
				bash := *tools.BashCommands
				bash.Allowed = slices.Clone(bash.Allowed)
				bash.Denied = slices.Clone(bash.Denied)
				tools.BashCommands = &bash
			}
			skill.Tools = &tools
		}
		// A compatibilidade consulta somente o comprimento dessas listas.
		if skill.Filesystem != nil {
			fs := *skill.Filesystem
			skill.Filesystem = &fs
		}
		if skill.Network != nil {
			network := *skill.Network
			skill.Network = &network
		}
	}
	return out
}

func (p *ContextProvider) Build(ctx context.Context, req contextprovider.BuildRequest) ([]contextprovider.Block, error) {
	if p == nil || p.source == nil {
		return nil, nil
	}
	turn, _ := ctx.Value(commandTurnKey{}).(*commandTurn)
	if turn == nil {
		skills, err := p.source.GetAllSkillsFull()
		if err != nil {
			return nil, err
		}
		return p.build(ctx, req, skills)
	}
	if turn.prepared == nil {
		skills, err := p.source.GetAllSkillsFull()
		turn.prepared = &preparedSkillContext{
			skills: snapshotPromptSkills(skills), err: err,
			files: make(map[string][]string), outputs: make(map[string]map[int]string),
		}
		prepared := turn.prepared
		if src, ok := p.source.(supportingFileLister); ok {
			prepared.filesAt = src.listSupportingFiles
			prepared.paths = make(map[string]string)
		}
		for _, skill := range prepared.skills {
			if prepared.filesAt != nil && skill.Path != "" {
				// O manager já resolveu a precedência de origem para este slug.
				prepared.paths[skill.Slug] = skill.Path
			} else {
				// Fontes que listam só por slug devem ser lidas enquanto o catálogo é o original.
				files, err := p.source.GetSkillFiles(skill.Slug)
				if err != nil {
					if prepared.sourceErrors == nil {
						prepared.sourceErrors = make(map[string]error)
					}
					prepared.sourceErrors[skill.Slug] = err
					continue
				}
				prepared.files[skill.Slug] = slices.Clone(files)
			}
		}
	}
	prepared := turn.prepared
	if prepared.err != nil {
		return nil, prepared.err
	}
	budget := req.Budget(p.Name(), defaultContextProviderBudget)
	if ctx.Err() == nil && prepared.last.matches(req, budget) {
		return slices.Clone(prepared.last.blocks), nil
	}
	prepared.listingFailed = false
	provider := ContextProvider{source: prepared}
	blocks, err := provider.build(ctx, req, prepared.skills)
	prepared.err = err
	prepared.ready = true
	if err == nil && !prepared.listingFailed {
		// Seleção e blocos retornados pertencem ao chamador. Os campos dos
		// blocos são escalares e strings imutáveis.
		prepared.last = &preparedPrompt{
			enabledSkills: slices.Clone(req.EnabledSkills), disableSkills: req.DisableSkills,
			disableOnDemand: req.DisableOnDemand, toolCalling: req.ToolCallingEnabled,
			budget: budget, blocks: slices.Clone(blocks),
		}
	}
	return blocks, err
}

func (p *ContextProvider) build(ctx context.Context, req contextprovider.BuildRequest, allSkills []Skill) ([]contextprovider.Block, error) {
	baseSkills, onDemandSkills := selectPromptSkills(allSkills, req)
	if len(baseSkills) == 0 && len(onDemandSkills) == 0 {
		return nil, nil
	}

	budget := req.Budget(p.Name(), defaultContextProviderBudget)
	catalogContent := buildAvailableSkillsBlock(ctx, onDemandSkills, p.source)
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
	filesComplete := true
	for i := range baseSkills {
		skill := &baseSkills[i]
		part := &sections[i]
		var heading, files strings.Builder
		if i > 0 {
			heading.WriteByte('\n')
		}
		writeSkillHeading(&heading, *skill)
		files.WriteByte('\n')
		if err := writeSupportingFiles(&files, source, skill.Slug, "Supporting files (use read_file to access when needed):", "- `", "`"); err != nil {
			filesComplete = false
			warnSupportingFiles(ctx, skill.Slug, err, true)
		}
		part.heading, part.files = heading.String(), files.String()
		planRunner := runner
		if turn != nil && turn.prepared != nil && turn.prepared.ready && len(turn.prepared.outputs[skill.Slug]) == 0 {
			// Uma base promovida depois da preparação só fornece instruções.
			planRunner = nil
		}
		doc, err := planCommandLines(ctx, skill, skill.Content, true, planRunner, nil, "")
		if err != nil {
			return "", err
		}
		part.doc = doc
		part.content = strings.Join(doc.lines, "\n")
		commands += len(doc.commands)
		remaining -= contextprovider.RuneLen(part.heading) + contextprovider.RuneLen(part.content) + contextprovider.RuneLen(part.files)
	}

	var sb strings.Builder
	sb.WriteString(open)
	admitted := filesComplete && turn != nil && commands > 0 && remaining/commands >= 2
	for i, part := range sections {
		count := len(part.doc.commands)
		if admitted && count > 0 {
			quota := (remaining/commands)*count + min(remaining%commands, count)
			content, err := turn.renderBase(ctx, &baseSkills[i], part.doc, quota)
			if err != nil {
				turn.err = err
				return "", err
			}
			remaining -= contextprovider.RuneLen(content) - contextprovider.RuneLen(part.content)
			commands -= count
			part.content = content
		}
		sb.WriteString(part.heading)
		sb.WriteString(part.content)
		sb.WriteString(part.files)
	}
	sb.WriteString(close)
	return sb.String(), nil
}

func (turn *commandTurn) renderBase(ctx context.Context, skill *Skill, doc commandDocument, quota int) (string, error) {
	prepared := turn.prepared
	if prepared != nil && prepared.ready {
		outputs := prepared.outputs[skill.Slug]
		return doc.renderLines(ctx, skill, true, quota, func(line commandLine) (string, error) {
			return outputs[line.index], nil
		})
	}
	var outputs map[int]string
	if prepared != nil {
		outputs = make(map[int]string, len(doc.commands))
	}
	content, err := doc.render(ctx, skill, turn.run, true, quota, outputs)
	if err == nil && prepared != nil {
		prepared.outputs[skill.Slug] = outputs
	}
	return content, err
}

func buildAvailableSkillsBlock(ctx context.Context, availableSkills []Skill, source ContextProviderSource) string {
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
		// Arquivos são opcionais no catálogo: seu envelope tem reserva fixa e
		// usa só o saldo da base. A preparação repete uma listagem que falhou.
		if err := writeSupportingFiles(&sb, source, skill.Slug, "  Supporting files:", "    - `", "`"); err != nil {
			warnSupportingFiles(ctx, skill.Slug, err, true)
		}
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

func warnSupportingFiles(ctx context.Context, slug string, err error, promptBuild bool) {
	if !firstSkillWarning(ctx, slug, "supporting_files", promptBuild) {
		return
	}
	reason := "listing_failed"
	switch {
	case errors.Is(err, fs.ErrNotExist):
		reason = "not_found"
	case errors.Is(err, fs.ErrPermission):
		reason = "permission_denied"
	}
	logging.Logger(ctx, "skills").WarnContext(ctx, "Listagem de arquivos de apoio indisponível", "skill", slug, "reason", reason)
}

func writeSupportingFiles(sb *strings.Builder, source ContextProviderSource, slug string, heading string, prefix string, suffix string) error {
	supplementary, err := source.GetSkillFiles(slug)
	if err != nil {
		return err
	}
	supplementary = sortedStrings(supplementary)
	if len(supplementary) == 0 {
		return nil
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
	return nil
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
