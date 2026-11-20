package skills

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

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
