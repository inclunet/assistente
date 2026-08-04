package acp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestProbeAgenteSaudavelAbreEFechaASessaoDeSondagem(t *testing.T) {
	client := newFakeManagedClient()
	client.caps = Capabilities{AgentName: "Cursor", AgentVersion: "2026.07.23"}
	m, dials := managerWith(newMemoryStore(), client)
	t.Cleanup(m.Shutdown)

	report := m.Probe(context.Background(), testSpec())

	if report.State != HealthOnline {
		t.Fatalf("estado = %q (erro %q), esperado online", report.State, report.Error)
	}
	if report.Error != "" {
		t.Errorf("erro em provider saudável = %q", report.Error)
	}
	if report.AgentName != "Cursor" || report.AgentVersion != "2026.07.23" {
		t.Errorf("identificação do agente = %q %q", report.AgentName, report.AgentVersion)
	}
	if report.WorkDir != dirDeTeste("projeto") {
		t.Errorf("diretório sondado = %q, esperado o workspace ativo %q", report.WorkDir, dirDeTeste("projeto"))
	}
	if report.Latency < 0 {
		t.Errorf("tempo medido = %v", report.Latency)
	}
	if *dials != 1 {
		t.Errorf("transportes criados = %d, esperado 1", *dials)
	}

	client.mu.Lock()
	sessoes := append([]*fakeManagedSession(nil), client.sessions...)
	client.mu.Unlock()
	if len(sessoes) != 1 {
		t.Fatalf("sessões abertas = %d, esperado 1", len(sessoes))
	}
	if !sessoes[0].isClosed() {
		t.Error("a sessão de sondagem ficou aberta no agente")
	}
}

func TestProbeNaoDerrubaOProcessoQueOTurnoVaiUsar(t *testing.T) {
	client := newFakeManagedClient()
	m, dials := managerWith(newMemoryStore(), client)
	t.Cleanup(m.Shutdown)
	ctx := context.Background()

	if report := m.Probe(ctx, testSpec()); report.State != HealthOnline {
		t.Fatalf("estado da sonda = %q", report.State)
	}
	if _, err := m.Conversation(ctx, testSpec(), "conv-1"); err != nil {
		t.Fatalf("conversa depois da sonda: %v", err)
	}

	if *dials != 1 {
		t.Errorf("transportes criados = %d; a sonda derrubou o processo e o turno pagou outro spawn", *dials)
	}
	if _, _, encerrado := client.counters(); encerrado {
		t.Error("a sonda encerrou o cliente do provider")
	}
}

func TestProbeCandidateEncerraOProcessoDaConfiguracaoEmTeste(t *testing.T) {
	client := newFakeManagedClient()
	m, dials := managerWith(newMemoryStore(), client)
	t.Cleanup(m.Shutdown)

	report := m.ProbeCandidate(context.Background(), testSpec())

	if report.State != HealthOnline {
		t.Fatalf("estado = %q (erro %q), esperado online", report.State, report.Error)
	}
	if *dials != 1 {
		t.Fatalf("transportes criados = %d, esperado 1", *dials)
	}
	if _, _, encerrado := client.counters(); !encerrado {
		t.Error("o agente da configuração em teste ficou de pé sem provider a que pertencer")
	}
}

func TestProbeCandidateNaoAssumeOProcessoDoProviderSalvo(t *testing.T) {
	ctx := context.Background()
	salvo := newFakeManagedClient()
	candidato := newFakeManagedClient()
	clientes := []*fakeManagedClient{salvo, candidato}

	dials := 0
	m := NewManager(ManagerConfig{
		Store:   newMemoryStore(),
		WorkDir: func() (string, error) { return dirDeTeste("projeto"), nil },
		Dial: func(Config, RequestHandler) (Client, error) {
			c := clientes[dials]
			dials++
			return c, nil
		},
	})
	t.Cleanup(m.Shutdown)

	if _, err := m.Conversation(ctx, testSpec(), "conv-1"); err != nil {
		t.Fatalf("conversa: %v", err)
	}
	if report := m.ProbeCandidate(ctx, testSpec()); report.State != HealthOnline {
		t.Fatalf("estado da sonda = %q", report.State)
	}

	if _, _, encerrado := salvo.counters(); encerrado {
		t.Error("testar uma configuração derrubou o agente que já atendia uma conversa")
	}
	if _, _, encerrado := candidato.counters(); !encerrado {
		t.Error("o processo da configuração em teste sobrou")
	}
}

func TestProbeCandidateSemComandoNaoSobeNada(t *testing.T) {
	client := newFakeManagedClient()
	m, dials := managerWith(newMemoryStore(), client)
	t.Cleanup(m.Shutdown)

	report := m.ProbeCandidate(context.Background(), ProviderSpec{ID: "cursor", Name: "Cursor"})

	if report.State != HealthOffline || report.Error == "" {
		t.Fatalf("relatório = %+v, esperado offline com motivo", report)
	}
	if *dials != 0 {
		t.Errorf("transportes criados = %d, esperado nenhum", *dials)
	}
}

func TestProbeSemLoginEhEstadoProprioComOsMetodosDeAutenticacao(t *testing.T) {
	client := newFakeManagedClient()
	client.caps = Capabilities{
		AgentName: "Cursor",
		AuthMethods: []AuthMethod{{
			ID:          "cursor_login",
			Name:        "Entrar no Cursor",
			Description: "rode o login do CLI",
			Kind:        AuthKindAgent,
		}},
	}
	client.newErr = fmt.Errorf("abrir sessão no agente ACP: %w", ErrNotAuthenticated)
	m, _ := managerWith(newMemoryStore(), client)
	t.Cleanup(m.Shutdown)

	report := m.Probe(context.Background(), testSpec())

	if report.State != HealthUnauthenticated {
		t.Fatalf("estado = %q, esperado unauthenticated", report.State)
	}
	if !report.Unauthenticated() {
		t.Error("Unauthenticated() discorda do estado")
	}
	if len(report.AuthMethods) != 1 || report.AuthMethods[0].ID != "cursor_login" {
		t.Fatalf("métodos de autenticação = %+v; sem eles a tela não sabe o que instruir", report.AuthMethods)
	}
	if report.Error == "" {
		t.Error("falta de login sem motivo nenhum no relatório")
	}
}

func TestProbeAgenteQueNaoSobeEhOffline(t *testing.T) {
	m := NewManager(ManagerConfig{
		Store:   newMemoryStore(),
		WorkDir: func() (string, error) { return dirDeTeste("projeto"), nil },
		Dial: func(Config, RequestHandler) (Client, error) {
			return nil, errors.New("executável não encontrado")
		},
	})
	t.Cleanup(m.Shutdown)

	report := m.Probe(context.Background(), testSpec())

	if report.State != HealthOffline {
		t.Fatalf("estado = %q, esperado offline", report.State)
	}
	if !strings.Contains(report.Error, "executável não encontrado") {
		t.Errorf("erro = %q, esperado o motivo do spawn", report.Error)
	}
}

func TestProbeSemComandoRecusaAntesDeSondar(t *testing.T) {
	client := newFakeManagedClient()
	m, dials := managerWith(newMemoryStore(), client)
	t.Cleanup(m.Shutdown)

	report := m.Probe(context.Background(), ProviderSpec{ID: "cursor", Name: "Cursor"})

	if report.State != HealthOffline {
		t.Fatalf("estado = %q, esperado offline", report.State)
	}
	if report.Error == "" {
		t.Error("provider sem comando falhou sem dizer por quê")
	}
	if *dials != 0 {
		t.Errorf("transportes criados = %d, esperado nenhum", *dials)
	}
}

func TestProbeHandshakeQueFalhaEhOffline(t *testing.T) {
	client := newFakeManagedClient()
	client.capsErr = errors.New("o agente não respondeu ao initialize")
	m, _ := managerWith(newMemoryStore(), client)
	t.Cleanup(m.Shutdown)

	report := m.Probe(context.Background(), testSpec())

	if report.State != HealthOffline {
		t.Fatalf("estado = %q, esperado offline", report.State)
	}
	if !strings.Contains(report.Error, "initialize") {
		t.Errorf("erro = %q, esperado o motivo do handshake", report.Error)
	}
	client.mu.Lock()
	sessoes := len(client.sessions)
	client.mu.Unlock()
	if sessoes != 0 {
		t.Errorf("sessões abertas = %d; sem handshake não há o que sondar", sessoes)
	}
}

// Nome e descrição do método de login são texto do agente (D11) e vão para a
// tela junto com o estado sem login. Sem saneamento, um escape de terminal ou um
// parágrafo inteiro chegariam ali como se fossem rótulo.
func TestProbeSaneiaOsRotulosDosMetodosDeAutenticacao(t *testing.T) {
	client := newFakeManagedClient()
	client.caps = Capabilities{
		AgentName: "Cursor",
		AuthMethods: []AuthMethod{{
			ID:          "cursor_login",
			Name:        "\x1b[31mEntrar\x1b[0m no\tCursor",
			Description: "rode\no login\ndo CLI",
			Kind:        AuthKindAgent,
		}},
	}
	client.newErr = fmt.Errorf("abrir sessão no agente ACP: %w", ErrNotAuthenticated)
	m, _ := managerWith(newMemoryStore(), client)
	t.Cleanup(m.Shutdown)

	report := m.Probe(context.Background(), testSpec())

	if len(report.AuthMethods) != 1 {
		t.Fatalf("métodos de autenticação = %+v", report.AuthMethods)
	}
	metodo := report.AuthMethods[0]
	if metodo.Name != "Entrar no Cursor" {
		t.Errorf("nome = %q, esperado sem escape nem tabulação", metodo.Name)
	}
	if metodo.Description != "rode o login do CLI" {
		t.Errorf("descrição = %q, esperada em uma linha", metodo.Description)
	}
	// O ID endereça o método no protocolo: ele não é rótulo, e mexer nele
	// quebraria quem o usa para escolher o fluxo de login.
	if metodo.ID != "cursor_login" {
		t.Errorf("id = %q, esperado intacto", metodo.ID)
	}
}

func TestProbeHandshakeSemLoginTambemEhEstadoDeAutenticacao(t *testing.T) {
	client := newFakeManagedClient()
	client.capsErr = fmt.Errorf("apresentar o agente: %w", ErrNotAuthenticated)
	m, _ := managerWith(newMemoryStore(), client)
	t.Cleanup(m.Shutdown)

	report := m.Probe(context.Background(), testSpec())

	if report.State != HealthUnauthenticated {
		t.Fatalf("estado = %q, esperado unauthenticated", report.State)
	}
}

func TestProbeAchataOErroDoAgenteEmUmaLinha(t *testing.T) {
	client := newFakeManagedClient()
	client.newErr = errors.New("falhou\nERRO: tudo bem, nada aconteceu")
	m, _ := managerWith(newMemoryStore(), client)
	t.Cleanup(m.Shutdown)

	report := m.Probe(context.Background(), testSpec())

	if report.State != HealthOffline {
		t.Fatalf("estado = %q, esperado offline", report.State)
	}
	if strings.ContainsAny(report.Error, "\n\r") {
		t.Errorf("erro = %q; texto do agente com quebra de linha forja linha de log e de anúncio", report.Error)
	}
}
