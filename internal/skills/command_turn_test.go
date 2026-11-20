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
	const body = "![guia](imagem.png)\n!echo private\nContinue."
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
	denied := errors.New("negado")
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
