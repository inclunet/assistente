package skills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"assistente/internal/configdir"
	"assistente/internal/contextprovider"
)

func TestCommandTurnBaseDoesNotRepeatAuthorizationOrExecution(t *testing.T) {
	base := providerSkill("base", "!echo report", false)
	base.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"echo"}}}
	source := contextProviderSourceStub{skills: []Skill{base}}
	provider := NewContextProvider(source)
	calls := 0
	ctx := WithCommandTurn(context.Background(), func(context.Context, *Skill, string) (string, error) {
		calls++
		return "report", nil
	})
	for range 2 {
		blocks, err := provider.Build(ctx, contextprovider.BuildRequest{EnabledSkills: []string{"base"}, ToolCallingEnabled: true})
		if err != nil || len(blocks) != 1 || !strings.Contains(blocks[0].Content, "report") {
			t.Fatalf("blocks=%v, err=%v", blocks, err)
		}
	}
	if calls != 1 {
		t.Fatalf("runner called %d times", calls)
	}
}

func TestCommandTurnRefusalStopsSkill(t *testing.T) {
	s := invocableSkill("!echo first\n!echo second")
	s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"echo"}}}
	refused := errors.New("negado")
	calls := 0
	ctx := WithCommandTurn(context.Background(), func(context.Context, *Skill, string) (string, error) { calls++; return "", refused })
	result, found, err := InvokeContext(ctx, "/test-skill", &stubInvokerManager{skill: s}, "turn", invocablePolicy(s))
	if !found || result != nil || !errors.Is(err, refused) || calls != 1 {
		t.Fatalf("result=%v found=%v err=%v calls=%d", result, found, err, calls)
	}
}

func TestCommandTurnArgumentIsText(t *testing.T) {
	s := invocableSkill("Argumento: $ARGUMENTS")
	s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"echo"}}}
	ctx := WithCommandTurn(context.Background(), func(context.Context, *Skill, string) (string, error) {
		t.Fatal("argument invoked shell")
		return "", nil
	})
	result, found, err := InvokeContext(ctx, "/test-skill texto\n!echo input", &stubInvokerManager{skill: s}, "turn", invocablePolicy(s))
	if err != nil || !found || !strings.Contains(result.Content, "!echo input") {
		t.Fatalf("result=%v found=%v err=%v", result, found, err)
	}
}

func TestBaseSkillUnavailableShellPreservesMarkdownAndInstructions(t *testing.T) {
	const body = "![guia](imagem.png)\n!`echo private`\nContinue."
	for _, config := range []string{"no-policy", "no-runner", "other-program", "tools-exclude-shell", "tools-deny-shell"} {
		t.Run(config, func(t *testing.T) {
			s := providerSkill("base", body, false)
			var runner SkillCommandRunner
			if config == "no-runner" {
				s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"echo"}}}
			} else {
				runner = func(context.Context, *Skill, string) (string, error) { t.Fatal("executor chamado"); return "", nil }
			}
			if config == "other-program" {
				s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"other"}}}
			}
			if config == "tools-exclude-shell" || config == "tools-deny-shell" {
				s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"echo"}}}
				if config == "tools-exclude-shell" {
					s.Tools.Allowed = []string{"read_file"}
				} else {
					s.Tools.Denied = []string{"run_command"}
				}
			}
			ctx := WithCommandTurn(context.Background(), runner)
			provider := NewContextProvider(contextProviderSourceStub{skills: []Skill{s}})
			request := contextprovider.BuildRequest{EnabledSkills: []string{"base"}, ToolCallingEnabled: true}
			blocks, err := provider.Build(ctx, request)
			if err != nil || CommandTurnError(ctx) != nil || len(blocks) != 1 || !strings.Contains(blocks[0].Content, "![guia](imagem.png)\n\nContinue.") {
				t.Fatalf("blocks=%v err=%v", blocks, err)
			}
			if config == "no-runner" {
				request.ToolCallingEnabled = false
				blocks, err = provider.Build(ctx, request)
				if err != nil || len(blocks) != 0 {
					t.Fatalf("dependent base skill selected: %v, %v", blocks, err)
				}
			}
		})
	}
}

func TestBaseSkillShellDenialStopsPromptAndIsCached(t *testing.T) {
	s := providerSkill("base", "!echo first\n!echo second", false)
	s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"echo"}}}
	denied := contextprovider.ErrCommandRefused
	calls := 0
	provider := NewContextProvider(contextProviderSourceStub{skills: []Skill{s}})
	ctx := WithCommandTurn(t.Context(), func(context.Context, *Skill, string) (string, error) { calls++; return "", denied })
	for range 2 {
		blocks, err := provider.Build(ctx, contextprovider.BuildRequest{EnabledSkills: []string{"base"}, ToolCallingEnabled: true})
		if !errors.Is(err, denied) || !errors.Is(CommandTurnError(ctx), denied) || len(blocks) != 0 || calls != 1 {
			t.Fatalf("%v %v %d", blocks, err, calls)
		}
	}
}

func TestComposePreservesToolRestrictionsForEditedSkill(t *testing.T) {
	meta := &SkillMetadata{Name: "edited", Tools: &ToolPermissions{Allowed: []string{"read_file"}, Denied: []string{"run_command"}, BashCommands: &BashCommands{Allowed: []string{"git"}, Denied: []string{"rm"}}}}
	raw, err := Compose(meta, "!git status")
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Tools, meta.Tools) {
		t.Fatalf("policy round trip: %+v", got.Tools)
	}
}

func TestBaseSkillOperationalErrorsOmitOnlyCommandLine(t *testing.T) {
	for _, failure := range []error{errors.New("exit status 128"), context.DeadlineExceeded, errors.New("ledger unavailable"), errors.New("turn missing"), nil} {
		s := providerSkill("base", "antes\n!git status\ndepois", false)
		s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"git"}}}
		calls := 0
		ctx := WithCommandTurn(t.Context(), func(context.Context, *Skill, string) (string, error) {
			calls++
			if failure == nil {
				return strings.Repeat("x", MaxCommandOutputSize+1), nil
			}
			return "private partial", failure
		})
		provider := NewContextProvider(contextProviderSourceStub{skills: []Skill{s}})
		for range 2 {
			blocks, err := provider.Build(ctx, contextprovider.BuildRequest{EnabledSkills: []string{"base"}, ToolCallingEnabled: true})
			if err != nil || CommandTurnError(ctx) != nil || calls != 1 || len(blocks) != 1 || !strings.Contains(blocks[0].Content, "antes\n\ndepois") {
				t.Fatalf("failure=%v calls=%d err=%v", failure, calls, err)
			}
		}
	}
}

func TestFencedExamplesAndExclamationsRemainText(t *testing.T) {
	const body = "!IMPORTANTE: veja abaixo\n!atenção\n~~~sh\n!git status\n~~~\n!unknown example"
	for _, base := range []bool{true, false} {
		s := invocableSkill(body)
		s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"git"}}}
		got, err := renderCommands(t.Context(), s, body, base, func(context.Context, *Skill, string) (string, error) { t.Fatal("texto executado"); return "", nil }, nil)
		if err != nil || got != body {
			t.Fatalf("%q %v", got, err)
		}
	}
}

func TestBaseContinuesWhenConfirmationIsUnavailable(t *testing.T) {
	s := invocableSkill("Antes\n!git first\nDepois\n!git second")
	s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"git"}}}
	ctx := WithCommandTurn(t.Context(), func(_ context.Context, _ *Skill, command string) (string, error) {
		if command == "git first" {
			return "", contextprovider.ErrCommandNotAuthorized
		}
		return "ok", nil
	})
	content, err := buildBaseSkillBlock(ctx, []Skill{*s}, contextProviderSourceStub{}, 500)
	if err != nil || !strings.Contains(content, "Antes\n\nDepois\nok") || CommandTurnError(ctx) != nil {
		t.Fatalf("conteúdo=%q err=%v", content, err)
	}
}

func TestListFenceKeepsOnlyExampleLiteral(t *testing.T) {
	const body = "!git status\n- ~~~sh\n  !git status\n  ~~~\n!git status"
	s := invocableSkill(body)
	s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"git"}}}
	for _, base := range []bool{true, false} {
		got, err := renderCommands(t.Context(), s, body, base, func(context.Context, *Skill, string) (string, error) { return "executado", nil }, nil)
		if err != nil || got != "executado\n- ~~~sh\n  !git status\n  ~~~\nexecutado" {
			t.Fatalf("base=%t conteúdo=%q erro=%v", base, got, err)
		}
	}
}

func TestPreformattedHTMLKeepsOnlyExampleLiteral(t *testing.T) {
	const body = "<PRE class='example'>\n!git status\n!git status</PRE>\n\n!git status"
	s := invocableSkill(body)
	s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"git"}}}
	for _, base := range []bool{true, false} {
		got, err := renderCommands(t.Context(), s, body, base, func(context.Context, *Skill, string) (string, error) { return "executado", nil }, nil)
		if err != nil || got != "<PRE class='example'>\n!git status\n!git status</PRE>\n\nexecutado" {
			t.Fatalf("base=%t conteudo=%q erro=%v", base, got, err)
		}
	}
}

func TestSlashBudgetPreservesInstructionsAroundOutput(t *testing.T) {
	s := invocableSkill("Cabeçalho\n!git status\nInstruções finais")
	s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"git"}}}
	mgr := &stubInvokerManager{skill: s}
	calls := 0
	run := func(context.Context, *Skill, string) (string, error) { calls++; return strings.Repeat("á", 1000), nil }
	for _, budget := range []int{2, 256} {
		result, _, err := InvokeContext(WithSlashCommandTurn(t.Context(), run, budget), "/test-skill", mgr, "turn", invocablePolicy(s))
		if budget == 2 {
			if err == nil || calls != 0 {
				t.Fatal("execução sem espaço")
			}
			continue
		}
		if err != nil || calls != 1 || len([]rune(result.Content)) > budget || !strings.Contains(result.Content, "á…\nInstruções finais") {
			t.Fatalf("%+v err=%v", result, err)
		}
	}
}

func TestCommandOutputUsesRemainingBudget(t *testing.T) {
	doc := commandDocument{lines: []string{"", ""}, commands: []commandLine{{0, "first"}, {1, "second"}}}
	got, err := doc.render(t.Context(), &Skill{}, func(_ context.Context, _ *Skill, command string) (string, error) {
		if command == "first" {
			return "a", nil
		}
		return "1234567890", nil
	}, false, 10)
	if err != nil || got != "a\n12345678…" {
		t.Fatalf("conteúdo=%q err=%v", got, err)
	}
}

func TestSlashCommandRejectsShellSyntaxFromArguments(t *testing.T) {
	s := invocableSkill("!git log $ARGUMENTS")
	s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"git"}}}
	for _, arg := range []string{"a/../b", "..", "um ../dois", "--version", "nome -q", "--output=file", "nome -c core.pager=x", "@x", "/switch", "a; git status", "$(echo test)", "a'b", "a\"b"} {
		ctx := WithCommandTurn(t.Context(), func(context.Context, *Skill, string) (string, error) { t.Fatal("argumento executado"); return "", nil })
		if _, _, err := InvokeContext(ctx, "/test-skill "+arg, &stubInvokerManager{skill: s}, "turn", invocablePolicy(s)); err == nil {
			t.Fatalf("argumento aceito: %q", arg)
		}
	}
}

func TestSlashValidatesComposedPathOperand(t *testing.T) {
	for _, tc := range []struct {
		path, args string
		permitted  bool
	}{
		{"notas/.$0/arquivo", ".", false},
		{"$0", "", false},
		{"$ARGUMENTS[2]", "um", false},
		{"notas/$0$1/arquivo", ". .", false},
		{"notas/$0.md", "minha-nota", true},
		{"../repositorio $0", "main", true},
	} {
		s := invocableSkill("!git log " + tc.path)
		s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"git"}}}
		called := false
		ctx := WithCommandTurn(t.Context(), func(context.Context, *Skill, string) (string, error) { called = true; return "ok", nil })
		_, _, err := InvokeContext(ctx, "/test-skill "+tc.args, &stubInvokerManager{skill: s}, "turn", invocablePolicy(s))
		if called != tc.permitted || (err == nil) != tc.permitted {
			t.Fatalf("path=%q called=%v err=%v", tc.path, called, err)
		}
	}
}

func TestComposeImportedAllowedToolsUsesStructuredPermissions(t *testing.T) {
	meta, body, err := Parse("---\nname: example\nallowed-tools: Read, Grep\n---\nTexto")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := Compose(meta, body)
	if err != nil || strings.Contains(saved, "allowed-tools:") || !strings.Contains(saved, "tools:") {
		t.Fatal(saved, err)
	}
	reread, _, err := Parse(saved)
	if err != nil || !reflect.DeepEqual(reread.Tools, meta.Tools) {
		t.Fatal(reread, err)
	}
}

func TestCodeExamplesExpandArgumentsAsText(t *testing.T) {
	s := invocableSkill("    !git $0\n\t!git $0\n~~~sh\n!git $0\n~~~")
	s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"git"}}}
	got, err := renderCommands(t.Context(), s, s.Content, false, func(context.Context, *Skill, string) (string, error) { t.Fatal("exemplo executado"); return "", nil }, argumentExpander("log", []string{"log"}))
	if err != nil || strings.Count(got, "!git log") != 3 {
		t.Fatal(got, err)
	}
}

func TestBaseBudgetBeforeCommands(t *testing.T) {
	s := providerSkill("base", "cabeçalho\n!git status\ninstrução final", false)
	s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"git"}}}
	provider := NewContextProvider(contextProviderSourceStub{skills: []Skill{s}})
	ctx := WithCommandTurn(t.Context(), func(context.Context, *Skill, string) (string, error) {
		t.Fatal("comando chamado sem espaço para saída")
		return "", nil
	})
	_, err := provider.Build(ctx, contextprovider.BuildRequest{EnabledSkills: []string{"base"}, ToolCallingEnabled: true, ProviderBudgets: map[string]int{"skills": 1}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestParsedFirstIndentedExampleRemainsLiteral(t *testing.T) {
	for _, indent := range []string{"    ", "\t"} {
		_, body, err := Parse("---\r\nname: example\r\n---\r\n" + indent + "!git status\r\n")
		if err != nil {
			t.Fatal(err)
		}
		s := invocableSkill(body)
		s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"git"}}}
		got, err := renderCommands(t.Context(), s, body, false, func(context.Context, *Skill, string) (string, error) { t.Fatal("exemplo executado"); return "", nil }, nil)
		if err != nil || got != indent+"!git status" {
			t.Fatalf("%q %v", got, err)
		}
	}
}

func TestBaseBudgetReusesSpaceAcrossSkills(t *testing.T) {
	ss := []Skill{providerSkill("first", "!git first", false), providerSkill("second", "!git second", false)}
	for i := range ss {
		ss[i].Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"git"}}}
	}
	source := contextProviderSourceStub{skills: ss}
	static, err := buildBaseSkillBlock(t.Context(), ss, source, 500)
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithCommandTurn(t.Context(), func(_ context.Context, _ *Skill, command string) (string, error) {
		if command == "git first" {
			return "a", nil
		}
		return strings.Repeat("ç", 12), nil
	})
	budget := contextprovider.RuneLen(static) + 10
	content, err := buildBaseSkillBlock(ctx, ss, source, budget)
	if err != nil || contextprovider.RuneLen(content) != budget || !strings.Contains(content, strings.Repeat("ç", 8)+"…") {
		t.Fatalf("budget=%d conteúdo=%q err=%v", budget, content, err)
	}
}

func TestBaseProviderKeepsRemainderInLastSkill(t *testing.T) {
	s := providerSkill("base", "!git first\n!git second", false)
	s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"git"}}}
	provider := NewContextProvider(contextProviderSourceStub{skills: []Skill{s}})
	req := contextprovider.BuildRequest{EnabledSkills: []string{"base"}, ToolCallingEnabled: true}
	static, err := provider.Build(t.Context(), req)
	if err != nil || len(static) != 1 {
		t.Fatalf("blocos=%v erro=%v", static, err)
	}
	req.ProviderBudgets = map[string]int{"skills": contextprovider.RuneLen(static[0].Content) + 5}
	ctx := WithCommandTurn(t.Context(), func(_ context.Context, _ *Skill, cmd string) (string, error) {
		if cmd == "git first" {
			return "a", nil
		}
		return "bcde", nil
	})
	got, err := provider.Build(ctx, req)
	if err != nil || len(got) != 1 || !strings.Contains(got[0].Content, "a\nbcde") {
		t.Fatalf("blocos=%v erro=%v", got, err)
	}
}

func TestInterpolatedCommandNeedsParsedOperand(t *testing.T) {
	expand := argumentExpander(".", []string{"."})
	if validInterpolatedCommand(".", "VALUE=$0", expand) {
		t.Fatal("template sem comando aceito")
	}
}

func TestPreparedBaseKeepsCatalogUntilNextAttempt(t *testing.T) {
	s := providerSkill("base", "original", false)
	s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"git"}}}
	src := &contextProviderSourceStub{skills: []Skill{s}}
	p := NewContextProvider(src)
	req := contextprovider.BuildRequest{EnabledSkills: []string{"base"}, ToolCallingEnabled: true}
	calls := 0
	run := func(context.Context, *Skill, string) (string, error) { calls++; return "novo", nil }
	ctx := WithCommandTurn(t.Context(), run)
	first, err := p.Build(ctx, req)
	if err != nil || len(first) != 1 {
		t.Fatalf("prévia=%v err=%v", first, err)
	}
	src.skills[0].Content = "!git status"
	final, err := p.Build(ctx, req)
	if err != nil || !reflect.DeepEqual(first, final) || calls != 0 {
		t.Fatalf("final=%v chamadas=%d err=%v", final, calls, err)
	}
	limited := req
	limited.ToolCallingEnabled = false
	withoutTools, err := p.Build(ctx, limited)
	if err != nil || len(withoutTools) != 0 || calls != 0 {
		t.Fatalf("sem ferramentas=%v chamadas=%d err=%v", withoutTools, calls, err)
	}
	next, err := p.Build(WithCommandTurn(t.Context(), run), req)
	if err != nil || len(next) != 1 || !strings.Contains(next[0].Content, "novo") || calls != 1 {
		t.Fatalf("nova tentativa=%v chamadas=%d err=%v", next, calls, err)
	}
}

func TestPreparedBaseAppliesFinalSelectionAndBudget(t *testing.T) {
	base := providerSkill("base", "!echo value\ncontinue", true)
	base.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"echo"}}}
	later := providerSkill("later", "!echo later\nother instructions", false)
	later.Tools = base.Tools
	src := &contextProviderSourceStub{
		skills: []Skill{base, later}, files: map[string][]string{"base": {"original.md"}},
	}
	calls := 0
	output := strings.Repeat("ç", 24)
	ctx := WithCommandTurn(t.Context(), func(_ context.Context, _ *Skill, cmd string) (string, error) {
		calls++
		if cmd != "echo value" || calls != 1 {
			t.Fatalf("execução inesperada: %q, chamada %d", cmd, calls)
		}
		return output, nil
	})
	p := NewContextProvider(src)
	req := contextprovider.BuildRequest{EnabledSkills: []string{"base", "later"}, ToolCallingEnabled: true}
	build := func() []contextprovider.Block {
		t.Helper()
		blocks, err := p.Build(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		return blocks
	}
	first := build()
	if len(first) != 2 || !strings.Contains(first[0].Content, output) {
		t.Fatalf("prévia: %v", first)
	}
	fullSize := contextprovider.RuneLen(first[0].Content)
	first[0].Content = "alterado pelo consumidor"
	src.files["base"][0] = "alterado.md"
	src.skills[0].Tools.BashCommands.Allowed[0] = "outro"

	req.DisableOnDemand = true
	req.ProviderBudgets = map[string]int{"skills": fullSize - 10}
	blocks := build()
	if len(blocks) != 1 || contextprovider.RuneLen(blocks[0].Content) != fullSize-10 ||
		!strings.Contains(blocks[0].Content, strings.Repeat("ç", 13)+"…\ncontinue") ||
		!strings.Contains(blocks[0].Content, "original.md") {
		t.Fatalf("orçamento reduzido: %v", blocks)
	}
	req.ProviderBudgets["skills"] = fullSize
	if blocks = build(); len(blocks) != 1 || !strings.Contains(blocks[0].Content, output) {
		t.Fatalf("orçamento restaurado: %v", blocks)
	}
	req.EnabledSkills[0], req.EnabledSkills[1] = req.EnabledSkills[1], req.EnabledSkills[0]
	if blocks = build(); len(blocks) != 1 || !strings.Contains(blocks[0].Content, "other instructions") ||
		strings.Contains(blocks[0].Content, "!echo") || strings.Contains(blocks[0].Content, output) {
		t.Fatalf("outra base: %v", blocks)
	}
	req.EnabledSkills = []string{}
	if blocks = build(); len(blocks) != 0 {
		t.Fatalf("seleção vazia: %v", blocks)
	}
	req.EnabledSkills = nil
	if blocks = build(); len(blocks) != 1 || !strings.Contains(blocks[0].Content, output) {
		t.Fatalf("autoload: %v", blocks)
	}
	req.DisableSkills = true
	if blocks = build(); len(blocks) != 0 {
		t.Fatalf("desabilitadas: %v", blocks)
	}
	if calls != 1 {
		t.Fatalf("execuções: %d", calls)
	}
}

func TestPreparedBaseCannotStartSkippedCommandWhenBudgetGrows(t *testing.T) {
	s := providerSkill("base", "before\n!echo report\nafter", true)
	s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"echo"}}}
	p := NewContextProvider(contextProviderSourceStub{skills: []Skill{s}})
	ctx := WithCommandTurn(t.Context(), func(context.Context, *Skill, string) (string, error) {
		t.Fatal("comando não admitido durante a preparação")
		return "", nil
	})
	req := contextprovider.BuildRequest{ToolCallingEnabled: true, ProviderBudgets: map[string]int{"skills": 1}}
	if _, err := p.Build(ctx, req); err != nil {
		t.Fatal(err)
	}
	req.ProviderBudgets = nil
	blocks, err := p.Build(ctx, req)
	if err != nil || len(blocks) != 1 || !strings.Contains(blocks[0].Content, "before\n\nafter") {
		t.Fatalf("reconstrução: %v, %v", blocks, err)
	}
}

func TestExplicitCommandRefusalPrecedesConcurrentCancellation(t *testing.T) {
	for _, flow := range []string{"base", "slash"} {
		t.Run(flow, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			s := providerSkill("test-skill", "!echo first\n!echo next", true)
			s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"echo"}}}
			refusal := errors.Join(contextprovider.ErrCommandRefused, errors.New("decisão explícita"))
			calls := 0
			ctx = WithCommandTurn(ctx, func(context.Context, *Skill, string) (string, error) {
				calls++
				cancel()
				return "saída parcial", refusal
			})
			if flow == "base" {
				p := NewContextProvider(contextProviderSourceStub{skills: []Skill{s}})
				blocks, err := p.Build(ctx, contextprovider.BuildRequest{ToolCallingEnabled: true})
				if len(blocks) != 0 || err != refusal || CommandTurnError(ctx) != refusal {
					t.Fatalf("blocos=%v erro=%v erro do turno=%v", blocks, err, CommandTurnError(ctx))
				}
			} else {
				result, found, err := InvokeContext(ctx, "/test-skill", &stubInvokerManager{skill: &s}, "turn", invocablePolicy(&s))
				if !found || result != nil || err != refusal {
					t.Fatalf("encontrada=%v resultado=%v erro=%v", found, result, err)
				}
			}
			if calls != 1 {
				t.Fatalf("chamadas=%d", calls)
			}
		})
	}
}

func TestPreparedFilesFollowOriginalSourceForLateSelection(t *testing.T) {
	root := t.TempDir()
	for _, origin := range []string{"home", "workdir"} {
		dir := filepath.Join(root, origin, "report")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: report\n---\n"+origin), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, origin+".md"), []byte("aux"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	mgr := &Manager{resolver: configdir.NewResolverWithBase(filepath.Join(root, "home"))}
	p := NewContextProvider(mgr)
	ctx := WithCommandTurn(t.Context(), nil)
	if _, err := p.Build(ctx, contextprovider.BuildRequest{DisableSkills: true}); err != nil {
		t.Fatal(err)
	}
	mgr.resolver = configdir.NewResolverWithBase(filepath.Join(root, "workdir"))
	mgr.invalidateFullSkillsCache()
	req := contextprovider.BuildRequest{EnabledSkills: []string{"report"}, ToolCallingEnabled: true}
	for _, test := range []struct {
		ctx          context.Context
		want, reject string
	}{
		{ctx, "home.md", "workdir.md"}, {WithCommandTurn(t.Context(), nil), "workdir.md", "home.md"},
	} {
		blocks, err := p.Build(test.ctx, req)
		if err != nil || len(blocks) != 1 || !strings.Contains(blocks[0].Content, test.want) || strings.Contains(blocks[0].Content, test.reject) {
			t.Fatalf("esperado %q: %v, %v", test.want, blocks, err)
		}
	}
}

func TestNestedHTMLPreKeepsOnlyExamplesLiteral(t *testing.T) {
	content := "<div title='<pre>'>\n<!-- <pre> -->\n!echo before\n<PRE class='example'>\n!echo literal\n\n!echo still-literal\n</PRE>\n!echo sibling\n</div>\n\n!echo outside"
	for _, base := range []bool{true, false} {
		s := invocableSkill(content)
		s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"echo"}}}
		var calls []string
		got, err := renderCommands(t.Context(), s, content, base, func(_ context.Context, _ *Skill, command string) (string, error) {
			calls = append(calls, command)
			return "resultado", nil
		}, nil)
		if err != nil || !reflect.DeepEqual(calls, []string{"echo before", "echo sibling", "echo outside"}) ||
			!strings.Contains(got, "!echo literal\n\n!echo still-literal") {
			t.Fatalf("base=%v chamadas=%v texto=%q erro=%v", base, calls, got, err)
		}
	}
}

func TestSlashCommandRejectsNonCanonicalPositionBeforeAuthorization(t *testing.T) {
	for _, placeholder := range []string{"$ARGUMENTS[01]", "$01", "$ARGUMENTS[x]", "$ARGUMENTS[1"} {
		s := invocableSkill("!echo " + placeholder)
		s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"echo"}}}
		ctx := WithCommandTurn(t.Context(), func(context.Context, *Skill, string) (string, error) {
			t.Fatalf("autorização solicitada para %s", placeholder)
			return "", nil
		})
		result, found, err := InvokeContext(ctx, "/test-skill um dois", &stubInvokerManager{skill: s}, "turn", invocablePolicy(s))
		if err == nil || result != nil || !found {
			t.Fatalf("%s: %v, %v, %v", placeholder, result, found, err)
		}
	}
}

func TestMultilineCodeSpanKeepsCommandsLiteral(t *testing.T) {
	const example = "Exemplo ``início\n!echo literal\n!`echo example`\nfim``."
	const body = "!`echo before`\n" + example + "\n!echo after"
	for _, base := range []bool{true, false} {
		s := invocableSkill(body)
		s.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"echo"}}}
		var calls []string
		got, err := renderCommands(t.Context(), s, body, base, func(_ context.Context, _ *Skill, cmd string) (string, error) {
			calls = append(calls, cmd)
			return "saída", nil
		}, nil)
		if err != nil || got != "saída\n"+example+"\nsaída" || !reflect.DeepEqual(calls, []string{"echo before", "echo after"}) {
			t.Fatalf("base=%v chamadas=%v corpo=%q erro=%v", base, calls, got, err)
		}
	}
}
