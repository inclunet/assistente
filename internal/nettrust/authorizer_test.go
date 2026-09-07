package nettrust

import (
	"context"
	"net"
	"testing"

	"assistente/internal/tools"
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

// Porta derivada do scheme (implícita) deve persistir a entrada por HOST
// (Port vazio), valendo para qualquer porta default depois.
func TestAuthorizer_ImplicitPortPersistsHostLevel(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()
	prompt := &spyPrompter{decision: PromptDecision{Approve: true, Scope: ScopeGlobal}}
	auth := NewAuthorizer(m, prompt)

	// blockedDest tem Port "443" mas PortExplicit=false (derivada do scheme).
	if _, ok, err := auth.Authorize(ctx, blockedDest()); err != nil || !ok {
		t.Fatalf("esperado autorizado, ok=%v err=%v", ok, err)
	}
	d := m.Match(ctx, "api.nu.workflows.dev", "443")
	if !d.Allowed || d.Entry == nil {
		t.Fatalf("deveria ter persistido, got %+v", d)
	}
	if d.Entry.Port != "" {
		t.Fatalf("porta implícita não deveria ser persistida, got %q", d.Entry.Port)
	}
	// Mesmo host via outra porta default (ex.: http:80) também deve casar.
	if d := m.Match(ctx, "api.nu.workflows.dev", "80"); !d.Allowed {
		t.Fatal("autorização por host deveria valer para portas default")
	}
	// Porta NÃO-default (ex.: 8443) NÃO deve ser liberada por uma autorização por
	// host — exige consentimento explícito daquela porta (evita afrouxamento).
	if d := m.Match(ctx, "api.nu.workflows.dev", "8443"); d.Allowed {
		t.Fatal("autorização por host não deveria valer para porta não-default")
	}
}

// Porta explícita na URL deve ser persistida e restringir a autorização.
func TestAuthorizer_ExplicitPortPersisted(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := context.Background()
	prompt := &spyPrompter{decision: PromptDecision{Approve: true, Scope: ScopeGlobal}}
	auth := NewAuthorizer(m, prompt)

	dest := blockedDest()
	dest.Port = "8443"
	dest.PortExplicit = true
	if _, ok, err := auth.Authorize(ctx, dest); err != nil || !ok {
		t.Fatalf("esperado autorizado, ok=%v err=%v", ok, err)
	}
	d := m.Match(ctx, "api.nu.workflows.dev", "8443")
	if !d.Allowed || d.Entry == nil || d.Entry.Port != "8443" {
		t.Fatalf("porta explícita deveria ser persistida, got %+v", d)
	}
	if d := m.Match(ctx, "api.nu.workflows.dev", "443"); d.Allowed {
		t.Fatal("entrada com porta explícita não deveria casar outra porta")
	}
}

// Com mgr=nil, aprovar um escopo persistente não deve entrar em pânico nem
// tentar persistir: a liberação vale só para esta request (degrada para once).
func TestAuthorizer_NilManagerPersistentScopeDegrades(t *testing.T) {
	prompt := &spyPrompter{decision: PromptDecision{Approve: true, Scope: ScopeGlobal}}
	auth := NewAuthorizer(nil, prompt)

	ips, ok, err := auth.Authorize(context.Background(), blockedDest())
	if err != nil {
		t.Fatalf("não deveria retornar erro: %v", err)
	}
	if !ok || len(ips) == 0 {
		t.Fatal("deveria liberar a request corrente mesmo sem manager")
	}
}

// O pedido diz QUAL host declarado pelo skill casa com o destino bloqueado.
// Sem isso, quem decide precisa comparar o destino com a lista de hosts na mão.
func TestAuthorizer_PromptTrazHostDoSkillQueCasa(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := tools.WithExecutionContext(context.Background(), tools.ExecutionContext{
		InvokedSkillSlug:   "workflows-api",
		NetworkAllowedHost: []string{"outra.coisa.dev", "*.nu.workflows.dev"},
	})

	prompt := &spyPrompter{decision: PromptDecision{Approve: false}}
	auth := NewAuthorizer(m, prompt)

	if _, _, err := auth.Authorize(ctx, blockedDest()); err != nil {
		t.Fatalf("não deveria retornar erro: %v", err)
	}
	if got := prompt.lastReq.SkillHostMatch; got != "*.nu.workflows.dev" {
		t.Fatalf("host do skill que casa = %q, quer o wildcard declarado", got)
	}
}

// Destaque só quando de fato casa: dizer "é o host esperado" sobre um destino
// que o skill não declarou empurraria a pessoa a aprovar o que não devia.
func TestAuthorizer_PromptSemDestaqueQuandoNenhumHostCasa(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := tools.WithExecutionContext(context.Background(), tools.ExecutionContext{
		InvokedSkillSlug: "workflows-api",
		// Wildcard não casa o apex, e o outro host é de outro domínio.
		NetworkAllowedHost: []string{"*.api.nu.workflows.dev", "outra.coisa.dev"},
	})

	prompt := &spyPrompter{decision: PromptDecision{Approve: false}}
	auth := NewAuthorizer(m, prompt)

	if _, _, err := auth.Authorize(ctx, blockedDest()); err != nil {
		t.Fatalf("não deveria retornar erro: %v", err)
	}
	if got := prompt.lastReq.SkillHostMatch; got != "" {
		t.Fatalf("host do skill que casa = %q, quer vazio", got)
	}
	if len(prompt.lastReq.SkillSuggestedHosts) != 2 {
		t.Fatalf("os hosts declarados deveriam continuar no pedido, got %v", prompt.lastReq.SkillSuggestedHosts)
	}
}

// Casar o host do skill NÃO dispensa o consentimento (AEP-0082 D5).
func TestAuthorizer_HostDoSkillNaoDispensaConsentimento(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDirs(dir, dir)
	ctx := tools.WithExecutionContext(context.Background(), tools.ExecutionContext{
		InvokedSkillSlug:   "workflows-api",
		NetworkAllowedHost: []string{"api.nu.workflows.dev"},
	})

	prompt := &spyPrompter{decision: PromptDecision{Approve: false}}
	auth := NewAuthorizer(m, prompt)

	_, ok, err := auth.Authorize(ctx, blockedDest())
	if err != nil {
		t.Fatalf("não deveria retornar erro: %v", err)
	}
	if prompt.called != 1 {
		t.Fatalf("esperado 1 pedido de consentimento, got %d", prompt.called)
	}
	if ok {
		t.Fatal("negar deveria manter o bloqueio mesmo com o host declarado pelo skill")
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
