package nettrust

import (
	"context"
	"net"
	"testing"

	httpclient "assistente/internal/tools/http"
)

type spyPrompter struct {
	called   int
	decision PromptDecision
	err      error
	lastReq  PromptRequest
}

func (s *spyPrompter) PromptNetworkAuthorization(_ context.Context, req PromptRequest) (PromptDecision, error) {
	s.called++
	s.lastReq = req
	return s.decision, s.err
}

func blockedDest() httpclient.BlockedDestination {
	return httpclient.BlockedDestination{
		Host:     "api.nu.workflows.dev",
		Port:     "443",
		URL:      "https://api.nu.workflows.dev/x",
		IPs:      []net.IP{net.ParseIP("100.64.1.112")},
		Category: httpclient.CategoryCGNAT,
		Reason:   "cgnat address blocked by anti-SSRF policy",
	}
}

func TestAuthorizer_AllowlistMatch_NoPrompt(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()
	_ = m.Add(ctx, AllowlistEntry{Host: "api.nu.workflows.dev", Port: "443", Scope: ScopeGlobal})

	prompt := &spyPrompter{}
	auth := NewAuthorizer(m, prompt)

	ips, ok, err := auth.Authorize(ctx, blockedDest())
	if err != nil || !ok {
		t.Fatalf("esperado autorizado por allowlist, got ok=%v err=%v", ok, err)
	}
	if prompt.called != 0 {
		t.Fatal("não deveria pedir consentimento quando já está na allowlist")
	}
	if len(ips) != 1 || !ips[0].Equal(net.ParseIP("100.64.1.112")) {
		t.Fatalf("IPs de trust inesperados: %v", ips)
	}
}

func TestAuthorizer_PromptApprove_PersistsAndAllows(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()

	prompt := &spyPrompter{decision: PromptDecision{Approve: true, Scope: ScopeGlobal, Reason: "API interna"}}
	auth := NewAuthorizer(m, prompt)

	_, ok, err := auth.Authorize(ctx, blockedDest())
	if err != nil || !ok {
		t.Fatalf("esperado autorizado após consentimento, got ok=%v err=%v", ok, err)
	}
	if prompt.called != 1 {
		t.Fatalf("esperado 1 pedido de consentimento, got %d", prompt.called)
	}
	// Deve ter persistido a entrada com o IP resolvido e a categoria.
	d := m.Match(ctx, "api.nu.workflows.dev", "443")
	if !d.Allowed || d.Entry == nil {
		t.Fatalf("entrada deveria ter sido persistida, got %+v", d)
	}
	if len(d.Entry.ResolvedIPs) == 0 || d.Entry.ResolvedIPs[0] != "100.64.1.112" {
		t.Fatalf("entrada deveria registrar o IP resolvido, got %+v", d.Entry.ResolvedIPs)
	}
	if d.Entry.Category != string(httpclient.CategoryCGNAT) {
		t.Fatalf("categoria deveria ser registrada, got %q", d.Entry.Category)
	}
}

func TestAuthorizer_PromptOnceDoesNotPersist(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()

	prompt := &spyPrompter{decision: PromptDecision{Approve: true, Scope: ScopeOnce}}
	auth := NewAuthorizer(m, prompt)

	_, ok, err := auth.Authorize(ctx, blockedDest())
	if err != nil || !ok {
		t.Fatalf("esperado autorizado once, got ok=%v err=%v", ok, err)
	}
	if d := m.Match(ctx, "api.nu.workflows.dev", "443"); d.Allowed {
		t.Fatal("escopo once não deveria persistir")
	}
}

// Proteção DNS rebinding: uma entrada por host autorizada para CGNAT não pode
// liberar silenciosamente o endpoint de metadados quando o DNS passa a apontar
// para lá — deve exigir novo consentimento (prompt).
func TestAuthorizer_AllowlistMatch_CategoryEscalationReprompts(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()
	_ = m.Add(ctx, AllowlistEntry{
		Host:     "api.nu.workflows.dev",
		Port:     "443",
		Scope:    ScopeGlobal,
		Category: string(httpclient.CategoryCGNAT),
	})

	prompt := &spyPrompter{decision: PromptDecision{Approve: false}}
	auth := NewAuthorizer(m, prompt)

	// Destino agora resolve para o endpoint de metadados (categoria mais sensível).
	dest := httpclient.BlockedDestination{
		Host:     "api.nu.workflows.dev",
		Port:     "443",
		URL:      "https://api.nu.workflows.dev/x",
		IPs:      []net.IP{net.ParseIP("169.254.169.254")},
		Category: httpclient.CategoryMetadata,
		Reason:   "metadata address blocked by anti-SSRF policy",
	}

	_, ok, err := auth.Authorize(ctx, dest)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if ok {
		t.Fatal("escalonamento para metadados não deveria ser liberado pela allowlist")
	}
	if prompt.called != 1 {
		t.Fatalf("deveria cair para novo consentimento, prompts=%d", prompt.called)
	}
}

// A rotação normal entre IPs privados (mesma categoria) continua sendo liberada
// pela allowlist, sem novo prompt.
func TestAuthorizer_AllowlistMatch_SameCategoryRotationAllowed(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()
	_ = m.Add(ctx, AllowlistEntry{
		Host:     "api.nu.workflows.dev",
		Port:     "443",
		Scope:    ScopeGlobal,
		Category: string(httpclient.CategoryCGNAT),
	})

	prompt := &spyPrompter{}
	auth := NewAuthorizer(m, prompt)

	dest := blockedDest()
	dest.IPs = []net.IP{net.ParseIP("100.64.9.9")} // outro IP CGNAT

	_, ok, err := auth.Authorize(ctx, dest)
	if err != nil || !ok {
		t.Fatalf("rotação na mesma categoria deveria ser liberada, ok=%v err=%v", ok, err)
	}
	if prompt.called != 0 {
		t.Fatalf("não deveria pedir consentimento em rotação de mesma categoria, prompts=%d", prompt.called)
	}
}

func TestAuthorizer_PromptDeny(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()

	prompt := &spyPrompter{decision: PromptDecision{Approve: false}}
	auth := NewAuthorizer(m, prompt)

	ips, ok, err := auth.Authorize(ctx, blockedDest())
	if err != nil {
		t.Fatalf("deny não deveria retornar erro: %v", err)
	}
	if ok || len(ips) != 0 {
		t.Fatal("deny deveria manter o bloqueio")
	}
}
