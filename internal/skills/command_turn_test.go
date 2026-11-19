package skills

import (
	"context"
	"errors"
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
