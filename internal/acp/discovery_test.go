package acp

import (
	"fmt"
	"testing"
)

// escrituracao é o que o agente falso contou de si na última sessão que abriu:
// quantas abriu e quantas foram fechadas. Vem pelo fio, no nome da opção de
// modelo, porque o agente roda em outro processo — contador que o teste pudesse
// ler de dentro não provaria nada sobre o que aconteceu no protocolo.
func escrituracao(t *testing.T, options []ConfigOption) (abertas, fechadas, processo int) {
	t.Helper()
	option := findOption(options, "model")
	if option == nil {
		t.Fatalf("o agente não devolveu opção de modelo: %+v", options)
	}
	_, err := fmt.Sscanf(option.Name, "abertas=%d fechadas=%d processo=%d", &abertas, &fechadas, &processo)
	if err != nil {
		t.Fatalf("escrituração ilegível em %q: %v", option.Name, err)
	}
	return abertas, fechadas, processo
}

func TestDescobertaListaOsModelosQueOAgenteOferece(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptDescoberta, nil)

	options, err := client.Options(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("descobrir opções: %v", err)
	}

	// A descoberta não é uma conversa: ela responde sem que ninguém tenha
	// aberto aba nenhuma, e é isso que a tela de configurações precisa.
	if abertas, fechadas, _ := escrituracao(t, options); abertas != 1 || fechadas != 0 {
		t.Errorf("descoberta abriu %d sessões e fechou %d; esperava abrir 1 e fechar 0", abertas, fechadas)
	}
	modelos := ModelValues(options)
	if len(modelos) == 0 {
		t.Fatalf("nenhum modelo descoberto: %+v", options)
	}
	// O modo vem junto: quem escolhe modelo escolhe modo no mesmo lugar.
	if modo := findOption(options, modeCategory); modo == nil || modo.CurrentValue != "agent" {
		t.Errorf("a descoberta não trouxe o modo: %+v", options)
	}
}

func TestDescobertaGuardaOQueLeuEmVezDeRepetirAPergunta(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptDescoberta, nil)
	dir := t.TempDir()

	primeira, err := client.Options(ctx, dir)
	if err != nil {
		t.Fatalf("primeira descoberta: %v", err)
	}
	segunda, err := client.Options(ctx, dir)
	if err != nil {
		t.Fatalf("segunda descoberta: %v", err)
	}

	// Cada sessão que este agente abre nasce num modelo diferente. A segunda
	// consulta respondendo o mesmo é a prova de que ninguém bateu no agente de
	// novo — a tela de configurações renderiza muitas vezes por interação.
	if primeira[0].CurrentValue != segunda[0].CurrentValue {
		t.Errorf("a segunda consulta reperguntou ao agente: %q depois %q",
			primeira[0].CurrentValue, segunda[0].CurrentValue)
	}
	if abertas, _, _ := escrituracao(t, segunda); abertas != 1 {
		t.Errorf("o agente abriu %d sessões para duas consultas; esperava 1", abertas)
	}
}

// Primeira das três invalidações do D6: a pessoa pede a lista de novo na tela.
func TestRefreshDaTelaFazADescobertaPerguntarDeNovo(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptDescoberta, nil)
	dir := t.TempDir()

	antes, err := client.Options(ctx, dir)
	if err != nil {
		t.Fatalf("primeira descoberta: %v", err)
	}

	client.InvalidateOptions()

	depois, err := client.Options(ctx, dir)
	if err != nil {
		t.Fatalf("descoberta depois do refresh: %v", err)
	}
	if antes[0].CurrentValue == depois[0].CurrentValue {
		t.Errorf("o refresh serviu a lista guardada: %q nas duas vezes", depois[0].CurrentValue)
	}
	// A sessão de descoberta anterior tem de ter sido encerrada pelo método do
	// protocolo: sessão abandonada no agente é rastro que ninguém recolhe.
	abertas, fechadas, _ := escrituracao(t, depois)
	if abertas != 2 || fechadas != 1 {
		t.Errorf("o agente abriu %d sessões e fechou %d; esperava abrir 2 e fechar 1", abertas, fechadas)
	}
}

// Segunda das três: o agente troca de modelo sozinho e conta por notificação. A
// lista guardada descreve um estado que acabou de mudar.
func TestAvisoDoAgenteFazADescobertaPerguntarDeNovo(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptDescoberta, nil)
	dir := t.TempDir()

	antes, err := client.Options(ctx, dir)
	if err != nil {
		t.Fatalf("primeira descoberta: %v", err)
	}

	// A troca chega por uma conversa de verdade, que é de onde ela vem no uso
	// real: o agente avisa na sessão em que está trabalhando.
	sess, err := client.NewSession(ctx, dir)
	if err != nil {
		t.Fatalf("abrir conversa: %v", err)
	}
	col := &collector{}
	if _, err := sess.Prompt(ctx, []Content{TextContent("oi")}, col.sink); err != nil {
		t.Fatalf("turno falhou: %v", err)
	}
	if got := findOption(sess.ConfigOptions(), "model"); got == nil || got.CurrentValue != "modelo-b" {
		t.Fatalf("o agente não anunciou a troca de modelo: %+v", sess.ConfigOptions())
	}

	depois, err := client.Options(ctx, dir)
	if err != nil {
		t.Fatalf("descoberta depois do aviso: %v", err)
	}
	if antes[0].CurrentValue == depois[0].CurrentValue {
		t.Errorf("o aviso do agente não invalidou a lista guardada: %q nas duas vezes", depois[0].CurrentValue)
	}
}

// Terceira das três: o processo caiu e o app reconectou. O cache é do processo,
// e a sessão que produziu a lista morreu com ele.
func TestProcessoNovoNaoServeALIstaDoProcessoAnterior(t *testing.T) {
	ctx := testContext(t)
	client := newTestClient(t, scriptDescoberta, nil)
	dir := t.TempDir()

	antes, err := client.Options(ctx, dir)
	if err != nil {
		t.Fatalf("primeira descoberta: %v", err)
	}
	_, _, primeiroProcesso := escrituracao(t, antes)

	// O agente morre no meio de uma conversa, com o cache do processo já
	// povoado. A próxima chamada sobe outro processo.
	sess, err := client.NewSession(ctx, dir)
	if err != nil {
		t.Fatalf("abrir conversa: %v", err)
	}
	if _, err := sess.Prompt(ctx, []Content{TextContent("morra")}, func(Update) {}); err == nil {
		t.Fatal("o turno deveria falhar com a morte do agente")
	}

	depois, err := client.Options(ctx, dir)
	if err != nil {
		t.Fatalf("descoberta no processo novo: %v", err)
	}
	// Quem respondeu tem de ser o processo novo. O cache do anterior contaria a
	// mesma quantidade de sessões — é o processo que denuncia a lista morta.
	abertas, fechadas, segundoProcesso := escrituracao(t, depois)
	if segundoProcesso == primeiroProcesso {
		t.Fatalf("a lista veio do processo anterior (%d), que já morreu", primeiroProcesso)
	}
	if abertas != 1 || fechadas != 0 {
		t.Errorf("o processo novo contou abertas=%d fechadas=%d; esperava 1 e 0", abertas, fechadas)
	}
}

func TestAgenteAvisaAsOpcoesDaSessaoParaQuemCuidaDoApp(t *testing.T) {
	ctx := testContext(t)

	avisos := make(chan []ConfigOption, 8)
	cfg := fakeConfig(t, scriptTurn)
	cfg.OnConfigOptions = func(_ string, options []ConfigOption) {
		select {
		case avisos <- options:
		default:
		}
	}
	client, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	sess := startSession(t, client, ctx)
	col := &collector{}
	if _, err := sess.Prompt(ctx, []Content{TextContent("oi")}, col.sink); err != nil {
		t.Fatalf("turno falhou: %v", err)
	}

	// O agente troca de modelo sozinho no meio do turno e anuncia. Sem este
	// aviso, a pessoa seguiria achando que fala com o modelo que escolheu.
	var trocou bool
	for len(avisos) > 0 {
		options := <-avisos
		if got := findOption(options, "model"); got != nil && got.CurrentValue == "modelo-b" {
			trocou = true
		}
	}
	if !trocou {
		t.Error("a troca de modelo feita pelo agente não chegou a quem cuida do app")
	}
}
