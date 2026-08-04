package acp

import (
	"context"
	"errors"
	"testing"
)

// managerComAvisos monta um manager que guarda os eventos de opção e devolve o
// gancho que o transporte usaria para contar uma mudança. O gancho é o que a
// própria configuração leva ao transporte: pegá-lo daqui é o que faz o teste
// passar pelo caminho real em vez de chamar o método interno na mão.
func managerComAvisos(t *testing.T, client *fakeManagedClient) (*Manager, func() []SessionOptionsEvent, func(string, []ConfigOption)) {
	t.Helper()

	var eventos []SessionOptionsEvent
	var doTransporte func(string, []ConfigOption)

	m := NewManager(ManagerConfig{
		Store:   newMemoryStore(),
		WorkDir: func() (string, error) { return dirDeTeste("projeto"), nil },
		OnSessionOptions: func(event SessionOptionsEvent) {
			eventos = append(eventos, event)
		},
		Dial: func(cfg Config, _ RequestHandler) (Client, error) {
			doTransporte = cfg.OnConfigOptions
			return client, nil
		},
	})
	t.Cleanup(m.Shutdown)

	avisar := func(sessionID string, options []ConfigOption) {
		if doTransporte == nil {
			t.Fatal("o manager não passou ao transporte quem escuta as opções da sessão")
		}
		doTransporte(sessionID, options)
	}
	return m, func() []SessionOptionsEvent { return eventos }, avisar
}

func opcaoDeModelo(corrente string, valores ...string) ConfigOption {
	option := ConfigOption{ID: "model", Name: "Modelo", Category: CategoryModel, CurrentValue: corrente}
	for _, valor := range valores {
		option.Values = append(option.Values, ConfigValue{Value: valor, Name: valor})
	}
	return option
}

func opcaoDeModo(corrente string, valores ...string) ConfigOption {
	option := ConfigOption{ID: "mode", Category: CategoryMode, CurrentValue: corrente}
	for _, valor := range valores {
		option.Values = append(option.Values, ConfigValue{Value: valor, Name: valor})
	}
	return option
}

func TestOpcoesDoProviderVemDaDescobertaDoProcesso(t *testing.T) {
	client := newFakeManagedClient()
	client.options = []ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b")}
	m, _ := managerWith(newMemoryStore(), client)

	options, err := m.ProviderOptions(context.Background(), testSpec())
	if err != nil {
		t.Fatalf("opções do provider: %v", err)
	}
	if got := ModelValues(options); len(got) != 2 || got[0] != "modelo-a" {
		t.Fatalf("modelos descobertos = %v", got)
	}
	if consultas, _ := client.discovery(); consultas != 1 {
		t.Errorf("descoberta consultada %d vezes; esperava 1", consultas)
	}
}

func TestInvalidarOpcoesChegaAoProcessoSemSubirOAgente(t *testing.T) {
	client := newFakeManagedClient()
	m, dials := managerWith(newMemoryStore(), client)

	// Antes de qualquer uso não há processo, e invalidar o que ninguém
	// descobriu não pode custar um spawn.
	m.InvalidateProviderOptions(testSpec().ID)
	if *dials != 0 {
		t.Fatalf("invalidar subiu o agente %d vez(es)", *dials)
	}

	if _, err := m.ProviderOptions(context.Background(), testSpec()); err != nil {
		t.Fatalf("opções do provider: %v", err)
	}
	m.InvalidateProviderOptions(testSpec().ID)
	if _, invalidacoes := client.discovery(); invalidacoes != 1 {
		t.Errorf("invalidações que chegaram ao processo = %d; esperava 1", invalidacoes)
	}
}

func TestTrocaDeModeloDaConversaChegaAoAgenteEFicaVisivelNaConversa(t *testing.T) {
	client := newFakeManagedClient()
	client.sessionOptions = []ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b")}
	m, _ := managerWith(newMemoryStore(), client)
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	options, err := conv.SetOption(ctx, "model", "modelo-b")
	if err != nil {
		t.Fatalf("trocar modelo: %v", err)
	}
	if got, _ := OptionByCategory(options, CategoryModel); got.CurrentValue != "modelo-b" {
		t.Errorf("estado devolvido = %+v", got)
	}
	if got, _ := OptionByCategory(conv.Options(), CategoryModel); got.CurrentValue != "modelo-b" {
		t.Errorf("a conversa não reflete a troca: %+v", conv.Options())
	}

	sess := client.sessions[0]
	if trocas := sess.optionSets(); len(trocas) != 1 || trocas[0].value != "modelo-b" {
		t.Errorf("o que chegou ao agente = %+v", trocas)
	}
}

func TestTrocaSemIdentificadorNaoVaiAoAgente(t *testing.T) {
	client := newFakeManagedClient()
	client.sessionOptions = []ConfigOption{opcaoDeModelo("modelo-a", "modelo-a")}
	m, _ := managerWith(newMemoryStore(), client)
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	if _, err := conv.SetOption(ctx, "   ", "modelo-b"); err == nil {
		t.Fatal("trocar opção sem identificador deveria falhar antes de falar com o agente")
	}
	if trocas := client.sessions[0].optionSets(); len(trocas) != 0 {
		t.Errorf("pedido inválido chegou ao agente: %+v", trocas)
	}
}

func TestTrocaRecusadaPeloAgenteNaoMudaAConversa(t *testing.T) {
	client := newFakeManagedClient()
	client.sessionOptions = []ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b")}
	m, _ := managerWith(newMemoryStore(), client)
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	client.sessions[0].setErr = errors.New("modelo indisponível")

	if _, err := conv.SetOption(ctx, "model", "modelo-b"); err == nil {
		t.Fatal("a recusa do agente deveria virar erro")
	}
	// A tela não pode passar a anunciar um modelo que o agente recusou.
	if got, _ := OptionByCategory(conv.Options(), CategoryModel); got.CurrentValue != "modelo-a" {
		t.Errorf("a conversa mudou de modelo apesar da recusa: %+v", got)
	}
}

func TestTrocaFeitaPeloAgenteViraEventoDaConversa(t *testing.T) {
	client := newFakeManagedClient()
	client.sessionOptions = []ConfigOption{
		opcaoDeModelo("modelo-a", "modelo-a", "modelo-b"),
		opcaoDeModo("agent", "agent", "plan"),
	}
	m, eventos, avisar := managerComAvisos(t, client)
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	sessionID := conv.Session().ID()

	// O agente troca de modelo por conta própria — fallback de limite de uso — e
	// conta pela notificação do protocolo.
	avisar(sessionID, []ConfigOption{
		opcaoDeModelo("modelo-b", "modelo-a", "modelo-b"),
		opcaoDeModo("agent", "agent", "plan"),
	})

	got := eventos()
	if len(got) != 1 {
		t.Fatalf("esperava 1 evento, obtive %d: %+v", len(got), got)
	}
	if got[0].ConversationID != "conv-1" || got[0].ProviderID != testSpec().ID {
		t.Errorf("o evento não achou a conversa dona da sessão: %+v", got[0])
	}
	if got[0].Model != "modelo-b" || !got[0].ModelChanged {
		t.Errorf("o evento não contou a troca de modelo: %+v", got[0])
	}
	if got[0].ModeChanged {
		t.Errorf("o modo não mudou e o evento diz que sim: %+v", got[0])
	}
	if !got[0].Announceable() {
		t.Error("uma troca de modelo feita pelo agente precisa ser anunciada")
	}
}

func TestAvisoQueRepeteOEstadoNaoPedeAnuncio(t *testing.T) {
	client := newFakeManagedClient()
	client.sessionOptions = []ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b")}
	m, eventos, avisar := managerComAvisos(t, client)
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}

	// O agente repete o estado corrente, o que ele faz de rotina. Anunciar a
	// cada repetição atropelaria a leitura da resposta em curso.
	avisar(conv.Session().ID(), []ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b")})

	got := eventos()
	if len(got) != 1 {
		t.Fatalf("esperava 1 evento, obtive %d", len(got))
	}
	if got[0].Announceable() {
		t.Errorf("estado repetido pediu anúncio: %+v", got[0])
	}
}

// O mesmo modelo com um espaço a mais na resposta do agente não é troca. Sem
// aparar, toda repetição desse tipo seria anunciada como decisão dele — e o
// anúncio atropelaria a leitura da resposta em curso por nada.
func TestRepeticaoComEspacoNoValorNaoViraTroca(t *testing.T) {
	client := newFakeManagedClient()
	client.sessionOptions = []ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b")}
	m, eventos, avisar := managerComAvisos(t, client)
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}

	avisar(conv.Session().ID(), []ConfigOption{opcaoDeModelo(" modelo-a ", "modelo-a", "modelo-b")})

	got := eventos()
	if len(got) != 1 {
		t.Fatalf("esperava 1 evento, obtive %d", len(got))
	}
	if got[0].ModelChanged || got[0].Announceable() {
		t.Errorf("espaço na resposta do agente virou troca de modelo: %+v", got[0])
	}
	if got[0].Model != "modelo-a" {
		t.Errorf("o modelo do evento saiu com espaço: %q", got[0].Model)
	}
}

func TestTrocaPedidaPeloAppNaoVoltaComoDecisaoDoAgente(t *testing.T) {
	client := newFakeManagedClient()
	client.sessionOptions = []ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b")}
	m, eventos, avisar := managerComAvisos(t, client)
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	if _, err := conv.SetOption(ctx, "model", "modelo-b"); err != nil {
		t.Fatalf("trocar modelo: %v", err)
	}

	// O agente confirma a troca pela notificação, como ele faz. Isso não é
	// decisão dele: anunciar aqui diria à pessoa que o agente mudou de modelo
	// no instante em que ela mesma acabou de mudar.
	avisar(conv.Session().ID(), []ConfigOption{opcaoDeModelo("modelo-b", "modelo-a", "modelo-b")})

	for _, event := range eventos() {
		if event.Announceable() {
			t.Errorf("o eco da troca pedida pelo app pediu anúncio: %+v", event)
		}
	}
}

// A confirmação do agente pode chegar enquanto ele ainda responde ao pedido: a
// notificação vem pela goroutine de entrega do transporte, e nada a ordena com a
// resposta. Anotada só na volta da chamada, essa confirmação seria comparada com
// o modelo antigo, e a pessoa ouviria "o agente trocou de modelo" no instante em
// que ela mesma acabou de trocar.
func TestConfirmacaoQueChegaNoMeioDaTrocaNaoViraAnuncio(t *testing.T) {
	client := newFakeManagedClient()
	client.sessionOptions = []ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b")}
	m, eventos, avisar := managerComAvisos(t, client)
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	sessionID := conv.Session().ID()
	client.sessions[0].duringSet = func(_, value string) {
		avisar(sessionID, []ConfigOption{opcaoDeModelo(value, "modelo-a", "modelo-b")})
	}

	if _, err := conv.SetOption(ctx, "model", "modelo-b"); err != nil {
		t.Fatalf("trocar modelo: %v", err)
	}

	got := eventos()
	if len(got) != 1 {
		t.Fatalf("esperava 1 evento vindo da notificação, obtive %d: %+v", len(got), got)
	}
	if got[0].Announceable() {
		t.Errorf("a confirmação da troca pedida pelo app pediu anúncio: %+v", got[0])
	}
}

// Troca que o agente recusou não pode deixar anotação para trás: o app ficaria
// achando que está num modelo em que não está, e o próximo aviso do agente com o
// modelo de verdade viraria anúncio de uma troca que ele não fez.
func TestTrocaRecusadaNaoDeixaAnotacaoDeModeloQueNaoValeu(t *testing.T) {
	client := newFakeManagedClient()
	client.sessionOptions = []ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b")}
	m, eventos, avisar := managerComAvisos(t, client)
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	sessionID := conv.Session().ID()
	client.sessions[0].setErr = errors.New("modelo indisponível")

	if _, err := conv.SetOption(ctx, "model", "modelo-b"); err == nil {
		t.Fatal("a recusa do agente deveria virar erro")
	}

	// O agente segue no modelo de sempre e conta isso na próxima notificação.
	avisar(sessionID, []ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b")})

	got := eventos()
	if len(got) != 1 {
		t.Fatalf("esperava 1 evento, obtive %d: %+v", len(got), got)
	}
	if got[0].Announceable() {
		t.Errorf("o modelo de sempre virou anúncio de troca depois de uma recusa: %+v", got[0])
	}
}

// O agente pode acomodar o pedido em outro valor. Aí quem decidiu foi ele, e é o
// valor que voltou que precisa ficar anotado: a pessoa ouve esse — quem exibe
// anuncia o estado devolvido — e a repetição dele não pode ser contada de novo
// como decisão nova.
func TestAgenteQueAcomodaOPedidoDeixaAnotadoOValorQueValeu(t *testing.T) {
	client := newFakeManagedClient()
	client.sessionOptions = []ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b", "modelo-c")}
	m, eventos, avisar := managerComAvisos(t, client)
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	sessionID := conv.Session().ID()
	client.sessions[0].setApplied = "modelo-c"

	options, err := conv.SetOption(ctx, "model", "modelo-b")
	if err != nil {
		t.Fatalf("trocar modelo: %v", err)
	}
	if got, _ := OptionByCategory(options, CategoryModel); got.CurrentValue != "modelo-c" {
		t.Fatalf("o estado devolvido esconde o que o agente aplicou: %+v", got)
	}

	avisar(sessionID, []ConfigOption{opcaoDeModelo("modelo-c", "modelo-a", "modelo-b", "modelo-c")})

	for _, event := range eventos() {
		if event.Announceable() {
			t.Errorf("a repetição do valor que o agente já devolveu pediu anúncio: %+v", event)
		}
	}
}

func TestAvisoDeSessaoQueNaoEDeConversaNaoViraEvento(t *testing.T) {
	client := newFakeManagedClient()
	m, eventos, avisar := managerComAvisos(t, client)

	// Sobe o processo sem abrir conversa: é o que a tela de configurações faz.
	if _, err := m.ProviderOptions(context.Background(), testSpec()); err != nil {
		t.Fatalf("opções do provider: %v", err)
	}

	// A sessão de descoberta também recebe notificação do agente, e ela não é
	// de conversa nenhuma: não há a quem contar.
	avisar("sessao-de-descoberta", []ConfigOption{opcaoDeModelo("modelo-b", "modelo-b")})

	if got := eventos(); len(got) != 0 {
		t.Errorf("aviso de sessão sem conversa virou evento: %+v", got)
	}
}

func TestAvisoDeConversaEncerradaNaoViraEvento(t *testing.T) {
	client := newFakeManagedClient()
	client.sessionOptions = []ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b")}
	m, eventos, avisar := managerComAvisos(t, client)
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	sessionID := conv.Session().ID()
	if err := m.CloseConversation(ctx, "conv-1"); err != nil {
		t.Fatalf("encerrar conversa: %v", err)
	}

	// Um aviso atrasado do agente sobre uma conversa que a pessoa apagou não
	// pode virar anúncio sobre uma aba que já não existe.
	avisar(sessionID, []ConfigOption{opcaoDeModelo("modelo-b", "modelo-a", "modelo-b")})

	if got := eventos(); len(got) != 0 {
		t.Errorf("aviso de conversa encerrada virou evento: %+v", got)
	}
}
