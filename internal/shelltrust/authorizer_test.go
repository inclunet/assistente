package shelltrust

import (
	"context"
	"testing"

	"assistente/internal/allowlist"
	"assistente/internal/authorization"
	"assistente/internal/testutil/authorizationfixture"
	"assistente/internal/tools/invocationctx"
)

func TestLimparConversaInvalidaConfirmacaoDeComandoLivre(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(authorizationfixture.Options(t, dir, dir))
	ctx := invocationctx.With(authorizationfixture.Context(), invocationctx.InvocationContext{ConversationID: "chat"})
	a := Authorizer{Manager: m, Policy: func(context.Context) (*allowlist.Allowlist, error) {
		return &allowlist.Allowlist{DefaultAction: "confirm"}, nil
	}, Prompt: func(context.Context, PromptRequest) (PromptDecision, error) {
		m.ClearSession("chat")
		return PromptDecision{Approve: true, Scope: authorization.ScopeOnce}, nil
	}}
	if allowed, err := a.Authorize(ctx, "echo arbitrary", dir); err == nil || allowed {
		t.Fatalf("aprovou conversa encerrada: %v %v", allowed, err)
	}
}

func TestPolicyAndGrantKeepOriginProfileDuringPrompt(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(authorizationfixture.Options(t, dir, dir))
	active := "origin"
	m.SetActiveProfileSlugFunc(func() string { return active })
	reads := 0
	a := Authorizer{Manager: m, PersistentAllowed: func(context.Context) bool { return true }, Policy: func(ctx context.Context) (*allowlist.Allowlist, error) {
		inv, _ := invocationctx.Get(ctx)
		if inv.ProfileSlug != "origin" {
			t.Errorf("política seguiu a UI: %s", inv.ProfileSlug)
		}
		reads++
		return &allowlist.Allowlist{DefaultAction: "confirm"}, nil
	}, Prompt: func(context.Context, PromptRequest) (PromptDecision, error) {
		active = "other"
		return PromptDecision{Approve: true, Scope: authorization.ScopeProfile}, nil
	}}
	if ok, err := a.Authorize(authorizationfixture.Context(), "git status", dir); err != nil || !ok {
		t.Fatalf("decisão: %v %v", ok, err)
	}
	if reads < 2 {
		t.Fatal("política não reavaliada após pergunta")
	}
	selector, ok := Represent("git status", dir)
	if !ok {
		t.Fatal("seletor ausente")
	}
	if match := m.Find(authorizationfixture.Context(), selector); match.Err != nil || match.Found {
		t.Fatalf("concessão seguiu a UI: %+v", match)
	}
}

func TestRepresentRejectsCompoundAndSecretArguments(t *testing.T) {
	dir := t.TempDir()
	for _, cmd := range []string{"git status && curl evil", "git status | sh", "TOKEN=secret git status", "git -c http.extraHeader=secret status", "git status > output", "echo secret", "git log --format=secret", "$(git status)", "git status;"} {
		if selector, ok := Represent(cmd, dir); ok {
			t.Errorf("representou comando não suportado %q: %+v", cmd, selector)
		}
	}
	if _, ok := Represent("git status", dir); !ok {
		t.Fatal("operação estática não representada")
	}
}

func TestGrantIsScopedToExactOperationAndDirectoryAndDenyWins(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(authorizationfixture.Options(t, dir, dir))
	policy := &allowlist.Allowlist{DefaultAction: "confirm"}
	prompts := 0
	authorizer := Authorizer{Manager: manager, Policy: func(context.Context) (*allowlist.Allowlist, error) { return policy, nil }, PersistentAllowed: func(context.Context) bool { return true }, Prompt: func(context.Context, PromptRequest) (PromptDecision, error) {
		prompts++
		return PromptDecision{Approve: true, Scope: authorization.ScopeGlobal}, nil
	}}
	ctx := authorizationfixture.Context()
	for i := 0; i < 2; i++ {
		if ok, err := authorizer.Authorize(ctx, "git status", dir); err != nil || !ok {
			t.Fatalf("autorização: %v %v", ok, err)
		}
	}
	if prompts != 1 {
		t.Fatalf("concessão não reutilizada: %d", prompts)
	}
	policy.AlwaysDeny = []string{"git status"}
	if ok, err := authorizer.Authorize(ctx, "git status", dir); err != nil || ok {
		t.Fatalf("concessão contornou negativa: %v %v", ok, err)
	}
	policy.AlwaysDeny = nil
	if ok, err := authorizer.Authorize(ctx, "git diff", dir); err != nil || !ok {
		t.Fatalf("nova operação: %v %v", ok, err)
	}
	if ok, err := authorizer.Authorize(ctx, "git status", t.TempDir()); err != nil || !ok {
		t.Fatalf("novo diretório: %v %v", ok, err)
	}
	if prompts != 3 {
		t.Fatalf("operação/diretório herdaram permissão: %d", prompts)
	}
}

func TestUnknownCommandCannotAcquireReusableGrant(t *testing.T) {
	dir := t.TempDir()
	a := Authorizer{Manager: NewManager(authorizationfixture.Options(t, dir, dir)), PersistentAllowed: func(context.Context) bool { return true }, Prompt: func(_ context.Context, req PromptRequest) (PromptDecision, error) {
		if len(req.AvailableScopes) != 1 || req.AvailableScopes[0] != authorization.ScopeOnce {
			t.Fatalf("escopos: %v", req.AvailableScopes)
		}
		return PromptDecision{Approve: true, Scope: authorization.ScopeGlobal}, nil
	}}
	if ok, err := a.Authorize(authorizationfixture.Context(), "echo secret", dir); err == nil || ok {
		t.Fatalf("resposta adulterada autorizada: %v %v", ok, err)
	}
}
