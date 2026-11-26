package skills

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"assistente/internal/contextprovider"
	"assistente/internal/tools/invocationctx"
)

type contextProviderSourceStub struct {
	skills []Skill
	files  map[string][]string
	err    error
}

func (s contextProviderSourceStub) GetAllSkillsFull() ([]Skill, error) {
	return s.skills, s.err
}

func (s contextProviderSourceStub) GetSkillFiles(slug string) ([]string, error) {
	return s.files[slug], nil
}

func providerSkill(slug string, content string, autoLoad bool) Skill {
	skill := Skill{Slug: slug, Content: content}
	skill.Name = slug
	skill.Description = slug + " desc"
	if autoLoad {
		skill.AutoLoad = true
		skill.Behavior = &BehaviorConfig{}
	}
	return skill
}

func TestContextProviderBuildsBaseAndAvailableSkillBlocks(t *testing.T) {
	provider := NewContextProvider(contextProviderSourceStub{
		skills: []Skill{
			providerSkill("base", "Base instructions.", false),
			providerSkill("later", "Later instructions.", false),
		},
		files: map[string][]string{
			"base":  {"z.md", "a.md"},
			"later": {"guide.md"},
		},
	})
	blocks, err := provider.Build(context.Background(), contextprovider.BuildRequest{
		EnabledSkills:      []string{"base", "later"},
		ToolCallingEnabled: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want 2", len(blocks))
	}
	if blocks[0].Name != "base_skill" || blocks[0].Volatility != contextprovider.VolatilityStable || blocks[0].Priority != 0 {
		t.Fatalf("unexpected base block: %+v", blocks[0])
	}
	if blocks[1].Name != "available_skills" || blocks[1].Priority != 5 {
		t.Fatalf("unexpected catalog block: %+v", blocks[1])
	}
	for _, needle := range []string{"<base_skill>", "Base instructions.", "- `a.md`", "- `z.md`"} {
		if !strings.Contains(blocks[0].Content, needle) {
			t.Fatalf("base block missing %q: %q", needle, blocks[0].Content)
		}
	}
	for _, needle := range []string{"<available_skills>", "Identifier: `later`", "guide.md"} {
		if !strings.Contains(blocks[1].Content, needle) {
			t.Fatalf("catalog block missing %q: %q", needle, blocks[1].Content)
		}
	}
}

func TestContextProviderOmitsSkillsWhenDisabled(t *testing.T) {
	provider := NewContextProvider(contextProviderSourceStub{
		skills: []Skill{providerSkill("base", "Base instructions.", true)},
	})
	blocks, err := provider.Build(context.Background(), contextprovider.BuildRequest{
		DisableSkills:      true,
		ToolCallingEnabled: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("len(blocks) = %d, want 0", len(blocks))
	}
}

func TestContextProviderTruncatesBaseSkillInsteadOfDroppingIt(t *testing.T) {
	provider := NewContextProvider(contextProviderSourceStub{
		skills: []Skill{providerSkill("base", strings.Repeat("Base instructions. ", 40), false)},
	})
	blocks, err := provider.Build(context.Background(), contextprovider.BuildRequest{
		EnabledSkills:      []string{"base"},
		ToolCallingEnabled: true,
		ProviderBudgets:    map[string]int{"skills": 180},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want truncated base skill", len(blocks))
	}
	if blocks[0].Name != "base_skill" {
		t.Fatalf("unexpected block: %+v", blocks[0])
	}
	if contextprovider.RuneLen(blocks[0].Content) > 180 {
		t.Fatalf("base block length = %d, want <= 180: %q", contextprovider.RuneLen(blocks[0].Content), blocks[0].Content)
	}
	if !strings.Contains(blocks[0].Content, "omitted due to context budget") {
		t.Fatalf("expected truncation notice: %q", blocks[0].Content)
	}
}

func TestContextProviderOmitsTaggedBlocksWhenEnvelopeCannotFit(t *testing.T) {
	provider := NewContextProvider(contextProviderSourceStub{
		skills: []Skill{providerSkill("base", strings.Repeat("Base instructions. ", 40), false)},
	})
	blocks, err := provider.Build(context.Background(), contextprovider.BuildRequest{
		EnabledSkills:      []string{"base"},
		ToolCallingEnabled: true,
		ProviderBudgets:    map[string]int{"skills": 10},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("len(blocks) = %d, want omitted block when tagged envelope cannot fit: %+v", len(blocks), blocks)
	}
}

func TestContextProviderTruncatesAvailableSkillsInsteadOfDroppingCatalog(t *testing.T) {
	provider := NewContextProvider(contextProviderSourceStub{
		skills: []Skill{
			providerSkill("base", strings.Repeat("Base instructions. ", 20), false),
			providerSkill("later", strings.Repeat("Later description. ", 20), false),
		},
	})
	blocks, err := provider.Build(context.Background(), contextprovider.BuildRequest{
		EnabledSkills:      []string{"base", "later"},
		ToolCallingEnabled: true,
		ProviderBudgets:    map[string]int{"skills": 260},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want base and truncated catalog", len(blocks))
	}
	if blocks[1].Name != "available_skills" {
		t.Fatalf("unexpected catalog block: %+v", blocks[1])
	}
	total := contextprovider.RuneLen(blocks[0].Content) + contextprovider.RuneLen(blocks[1].Content)
	if total > 260 {
		t.Fatalf("skills blocks length = %d, want <= 260: base=%q catalog=%q", total, blocks[0].Content, blocks[1].Content)
	}
	if !strings.Contains(blocks[1].Content, "omitted due to context budget") {
		t.Fatalf("expected catalog truncation notice: %q", blocks[1].Content)
	}
}

func TestContextProviderSanitizesSupportingFilePaths(t *testing.T) {
	provider := NewContextProvider(contextProviderSourceStub{
		skills: []Skill{providerSkill("base", "Base instructions.", false)},
		files: map[string][]string{
			"base": {"safe.md", "bad\n`<inject>`.md"},
		},
	})
	blocks, err := provider.Build(context.Background(), contextprovider.BuildRequest{
		EnabledSkills:      []string{"base"},
		ToolCallingEnabled: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want base block", len(blocks))
	}
	content := blocks[0].Content
	if strings.Contains(content, "bad\n") || strings.Contains(content, "<inject>") || strings.Contains(content, "`&lt;inject&gt;`") {
		t.Fatalf("supporting file path was not sanitized: %q", content)
	}
	if !strings.Contains(content, "bad '&lt;inject&gt;'.md") {
		t.Fatalf("expected escaped path in supporting files: %q", content)
	}
}

func TestContextProviderSanitizesSkillMetadata(t *testing.T) {
	base := providerSkill("base\n`<slug>`", "Base instructions.", false)
	base.Name = "Base\n`<name>`"
	base.Type = "base\n`<type>`"

	later := providerSkill("later\n`<slug>`", "Later instructions.", false)
	later.Name = "Later\n`<name>`"
	later.Type = "later\n`<type>`"
	later.Description = "desc\n`<inject>`"

	provider := NewContextProvider(contextProviderSourceStub{
		skills: []Skill{base, later},
	})
	blocks, err := provider.Build(context.Background(), contextprovider.BuildRequest{
		EnabledSkills:      []string{base.Slug, later.Slug},
		ToolCallingEnabled: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want base and catalog blocks", len(blocks))
	}
	combined := blocks[0].Content + "\n" + blocks[1].Content
	for _, bad := range []string{"Base\n", "Later\n", "<name>", "<type>", "<slug>", "<inject>", "`&lt;name&gt;`", "`&lt;type&gt;`", "`&lt;slug&gt;`", "`&lt;inject&gt;`"} {
		if strings.Contains(combined, bad) {
			t.Fatalf("metadata was not sanitized; found %q in %q", bad, combined)
		}
	}
	for _, want := range []string{
		"Base '&lt;name&gt;' [base '&lt;type&gt;']",
		"Later '&lt;name&gt;'** (`later '&lt;slug&gt;'`)",
		"desc '&lt;inject&gt;'",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("expected sanitized metadata %q in %q", want, combined)
		}
	}
}

func TestContextProviderToolCallingDisabledKeepsOnlyCompatibleBase(t *testing.T) {
	toolDependent := providerSkill("tool-dependent", "Tool dependent.", true)
	toolDependent.Tools = &ToolPermissions{Allowed: []string{"read_file"}}
	plain := providerSkill("plain", "Plain instructions.", false)
	provider := NewContextProvider(contextProviderSourceStub{
		skills: []Skill{toolDependent, plain},
	})
	blocks, err := provider.Build(context.Background(), contextprovider.BuildRequest{
		ToolCallingEnabled: false,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want compatible base only", len(blocks))
	}
	if strings.Contains(blocks[0].Content, "Tool dependent.") || !strings.Contains(blocks[0].Content, "Plain instructions.") {
		t.Fatalf("unexpected base block: %q", blocks[0].Content)
	}
}

func TestPreparedSupportingFilesRetryCapturedPathAfterError(t *testing.T) {
	const original = "home/report/SKILL.md"
	failure := errors.New("falha transitória")
	var paths []string
	prepared := &preparedSkillContext{
		paths: map[string]string{"report": original},
		files: make(map[string][]string),
		filesAt: func(path string) ([]string, error) {
			paths = append(paths, path)
			if len(paths) == 1 {
				return []string{"incompleto.md"}, failure
			}
			return []string{"completo.md"}, nil
		},
	}
	if files, err := prepared.GetSkillFiles("report"); !errors.Is(err, failure) || len(files) != 0 {
		t.Fatalf("primeira leitura: %v, %v", files, err)
	}
	for range 2 {
		files, err := prepared.GetSkillFiles("report")
		if err != nil || !slices.Equal(files, []string{"completo.md"}) {
			t.Fatalf("após recuperação: %v, %v", files, err)
		}
	}
	if !slices.Equal(paths, []string{original, original}) {
		t.Fatalf("leituras: %v", paths)
	}
}

type slugOnlyFileSource struct {
	contextProviderSourceStub
	listErr error
}

func (s *slugOnlyFileSource) GetSkillFiles(slug string) ([]string, error) {
	return s.files[slug], s.listErr
}

func TestPreparedFilesWithoutPathRecoverOnNewAttempt(t *testing.T) {
	failure := errors.New("listagem indisponível")
	src := &slugOnlyFileSource{contextProviderSourceStub: contextProviderSourceStub{
		skills: []Skill{providerSkill("report", "report instructions", true)},
		files:  map[string][]string{"report": {"parcial.md"}},
	}, listErr: failure}
	p := NewContextProvider(src)
	ctx := WithCommandTurn(t.Context(), nil)
	req := contextprovider.BuildRequest{ToolCallingEnabled: true}
	first, err := p.Build(ctx, req)
	if err != nil || len(first) != 1 || strings.Contains(first[0].Content, "parcial.md") {
		t.Fatalf("primeira construção: %v, %v", first, err)
	}
	src.listErr = nil
	src.files["report"] = []string{"original-ou-nova.md"}
	turn := ctx.Value(commandTurnKey{}).(*commandTurn)
	if files, err := turn.prepared.GetSkillFiles("report"); !errors.Is(err, failure) || len(files) != 0 {
		t.Fatalf("falha perdida: %v, %v", files, err)
	}
	next, err := p.Build(WithCommandTurn(t.Context(), nil), req)
	if err != nil || len(next) != 1 || !strings.Contains(next[0].Content, "original-ou-nova.md") {
		t.Fatalf("nova preparação: %v, %v", next, err)
	}
}

type retryableFileSource struct {
	contextProviderSourceStub
	calls int
}

func (s *retryableFileSource) listSupportingFiles(path string) ([]string, error) {
	s.calls++
	if s.calls == 1 {
		return nil, errors.New("falha temporária")
	}
	return []string{path + ".aux"}, nil
}

func TestIdenticalPreparedRequestCachesOnlyCompleteRender(t *testing.T) {
	skill := providerSkill("base", "instruções", true)
	skill.Path = "base/SKILL.md"
	src := &retryableFileSource{contextProviderSourceStub: contextProviderSourceStub{skills: []Skill{skill}}}
	p := NewContextProvider(src)
	ctx := WithCommandTurn(t.Context(), nil)
	req := contextprovider.BuildRequest{EnabledSkills: []string{"base"}, ToolCallingEnabled: true}
	build := func() []contextprovider.Block {
		t.Helper()
		blocks, err := p.Build(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		return blocks
	}
	before := build()
	if len(before) != 1 || strings.Contains(before[0].Content, ".aux") {
		t.Fatalf("falha transitória: %v", before)
	}
	after := build()
	if len(after) != 1 || !strings.Contains(after[0].Content, skill.Path+".aux") {
		t.Fatalf("recuperação: %v", after)
	}
	saved := ctx.Value(commandTurnKey{}).(*commandTurn).prepared
	entry := saved.last
	if entry == nil {
		t.Fatal("renderização completa ausente")
	}
	after[0].Content = "alterado na origem"
	// O orçamento efetivo continua o mesmo.
	req.ProviderBudgets = map[string]int{ContextProviderName: defaultContextProviderBudget}
	for range 2 {
		same := build()
		if entry != saved.last || len(same) != 1 || !strings.Contains(same[0].Content, skill.Path+".aux") {
			t.Fatalf("renderização repetida: %v", same)
		}
		same[0].Content = "alterado após reuso"
	}
	req.EnabledSkills[0] = "outra"
	if changed := build(); len(changed) != 0 {
		t.Fatalf("seleção alterada: %v", changed)
	}
	if src.calls != 2 {
		t.Fatalf("leituras de arquivo: %d", src.calls)
	}
}

func TestCatalogFileRetryPreservesBaseReservation(t *testing.T) {
	base := providerSkill("base", "antes\n!echo run\ndepois", false)
	base.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"echo"}}}
	base.Path = "base/SKILL.md"
	later := providerSkill("later", "contém apenas instruções", false)
	later.Path = "later/SKILL.md"
	src := &retryableFileSource{contextProviderSourceStub: contextProviderSourceStub{skills: []Skill{base, later}}}
	p := NewContextProvider(src)
	calls := 0
	ctx := WithCommandTurn(t.Context(), func(context.Context, *Skill, string) (string, error) { calls++; return "resultado", nil })
	req := contextprovider.BuildRequest{EnabledSkills: []string{"base", "later"}, ToolCallingEnabled: true, ProviderBudgets: map[string]int{"skills": 1500}}
	initial, err := p.Build(ctx, req)
	if err != nil || len(initial) != 2 || !strings.Contains(initial[0].Content, "resultado") || !strings.Contains(initial[1].Content, "later desc") || strings.Contains(initial[1].Content, "later/SKILL.md.aux") {
		t.Fatalf("prévia: %v erro=%v", initial, err)
	}
	final, err := p.Build(ctx, req)
	if err != nil || len(final) != 2 || initial[0] != final[0] || !strings.Contains(final[1].Content, "later/SKILL.md.aux") || calls != 1 || src.calls != 3 {
		t.Fatalf("final: %v err=%v execuções=%d listagens=%d", final, err, calls, src.calls)
	}
	prepared := ctx.Value(commandTurnKey{}).(*commandTurn).prepared
	cached := prepared.last
	again, err := p.Build(ctx, req)
	if err != nil || cached == nil || prepared.last != cached || !slices.Equal(again, final) || calls != 1 || src.calls != 3 {
		t.Fatalf("reuso após recuperação: %v err=%v execuções=%d listagens=%d", again, err, calls, src.calls)
	}
}

func TestRestrictedDelegationDoesNotAdvertiseLoadSkill(t *testing.T) {
	skill := providerSkill("later", "Later instructions.", false)
	source := contextProviderSourceStub{skills: []Skill{skill}}
	for _, tc := range []struct {
		tools      []string
		advertised bool
	}{{[]string{"read_file"}, false}, {nil, true}} {
		ctx := invocationctx.WithDelegatedTools(t.Context(), tc.tools)
		content := buildAvailableSkillsBlock(ctx, source.skills, source)
		if strings.Contains(content, "load_skill") != tc.advertised {
			t.Fatalf("catálogo inesperado: %q", content)
		}
	}
}
