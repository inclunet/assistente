package skills

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/contextprovider"
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
	if runeLen(blocks[0].Content) > 180 {
		t.Fatalf("base block length = %d, want <= 180: %q", runeLen(blocks[0].Content), blocks[0].Content)
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
	total := runeLen(blocks[0].Content) + runeLen(blocks[1].Content)
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
	if strings.Contains(content, "bad\n") || strings.Contains(content, "<inject>") {
		t.Fatalf("supporting file path was not sanitized: %q", content)
	}
	if !strings.Contains(content, "bad \\`&lt;inject&gt;\\`.md") {
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
	for _, bad := range []string{"Base\n", "Later\n", "<name>", "<type>", "<slug>", "<inject>"} {
		if strings.Contains(combined, bad) {
			t.Fatalf("metadata was not sanitized; found %q in %q", bad, combined)
		}
	}
	for _, want := range []string{
		"Base \\`&lt;name&gt;\\` [base \\`&lt;type&gt;\\`]",
		"Later \\`&lt;name&gt;\\`** (`later \\`&lt;slug&gt;\\``)",
		"desc \\`&lt;inject&gt;\\`",
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
