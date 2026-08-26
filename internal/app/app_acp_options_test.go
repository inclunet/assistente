package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"assistente/internal/acp"
	"assistente/internal/apidto"
	"assistente/internal/core/ports"
	"assistente/internal/wailsapi"
)

// agenteFalso é o agente do outro lado das ligações de modelo: guarda as opções
// que oferece, aceita a troca como o agente aceitaria e sabe anunciar por conta
// própria — que é como o fallback de limite de uso chega ao app (AEP-0084 D6).
type agenteFalso struct {
	mu         sync.Mutex
	opcoes     []acp.ConfigOption
	comandos   []acp.Command
	trocas     []string
	anuncia    func(sessionID string, options []acp.ConfigOption)
	anunciaCmd func(sessionID string, commands []acp.Command)
	sessoes    []*sessaoFalsa
	erroTroca  error
}

func novoAgenteFalso() *agenteFalso {
	return &agenteFalso{opcoes: opcoesDeAgente("modelo-a", "agent")}
}

// opcoesDeAgente monta o par que um agente de código oferece: o modelo corrente
// com os valores disponíveis e o modo de trabalho.
func opcoesDeAgente(modelo, modo string) []acp.ConfigOption {
	return []acp.ConfigOption{
		{
			ID:           "model",
			Name:         "Modelo",
			Category:     acp.CategoryModel,
			CurrentValue: modelo,
			Values: []acp.ConfigValue{
				{Value: "modelo-a", Name: "Modelo A"},
				{Value: "modelo-b", Name: "Modelo B"},
			},
		},
		{
			ID:           acp.CategoryMode,
			Name:         "Modo",
			Category:     acp.CategoryMode,
			CurrentValue: modo,
			Values: []acp.ConfigValue{
				{Value: "agent"}, {Value: "plan"}, {Value: "ask"},
				// Os modos de permissão do Claude Code. O `acceptEdits`
				// dispensa a pergunta só para edição e continua perguntando
				// pelo resto; os dois últimos a dispensam inteira. Um deles vem
				// com rótulo do agente e o outro sem, que são os dois jeitos de
				// o aviso ter de nomear o modo.
				{Value: "acceptEdits", Name: "Aceitar edições"},
				{Value: "dontAsk", Name: "Não perguntar"},
				{Value: "bypassPermissions"},
			},
		},
	}
}

func (a *agenteFalso) NewSession(_ context.Context, cwd string) (acp.Session, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := &sessaoFalsa{agente: a, id: "sessao-1", cwd: cwd}
	a.sessoes = append(a.sessoes, s)
	return s, nil
}

func (a *agenteFalso) LoadSession(ctx context.Context, sessionID, cwd string) (acp.Session, error) {
	return nil, errors.New("sem retomada neste agente")
}

func (a *agenteFalso) Capabilities(context.Context) (acp.Capabilities, error) {
	return acp.Capabilities{AgentName: "falso"}, nil
}

func (a *agenteFalso) CloseSession(context.Context, string) error { return nil }

func (a *agenteFalso) Options(context.Context, string) ([]acp.ConfigOption, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.opcoes, nil
}

func (a *agenteFalso) InvalidateOptions() {}

func (a *agenteFalso) Call(context.Context, string, any) (json.RawMessage, error) {
	return nil, errors.New("método não suportado pelo agente falso")
}

func (a *agenteFalso) Close() error { return nil }

// trocouPara registra a troca e move o valor corrente, como o agente faria.
func (a *agenteFalso) trocouPara(id, valor string) []acp.ConfigOption {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.trocas = append(a.trocas, id+"="+valor)
	atualizadas := make([]acp.ConfigOption, 0, len(a.opcoes))
	for _, opcao := range a.opcoes {
		if opcao.ID == id {
			opcao.CurrentValue = valor
		}
		atualizadas = append(atualizadas, opcao)
	}
	a.opcoes = atualizadas
	return atualizadas
}

func (a *agenteFalso) trocasFeitas() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.trocas...)
}

// avisaSozinho é o agente contando que mudou de modelo sem ninguém ter pedido.
func (a *agenteFalso) avisaSozinho(modelo string) {
	a.mu.Lock()
	aviso := a.anuncia
	a.mu.Unlock()
	if aviso == nil {
		return
	}
	aviso("sessao-1", opcoesDeAgente(modelo, "agent"))
}

// avisaModo é o agente contando que trocou de modo por conta própria, sem
// ninguém ter clicado no seletor — o que o protocolo permite por
// `config_option_update`.
func (a *agenteFalso) avisaModo(modo string) {
	a.mu.Lock()
	aviso := a.anuncia
	a.mu.Unlock()
	if aviso == nil {
		return
	}
	aviso("sessao-1", opcoesDeAgente("modelo-a", modo))
}

// avisaSemLista é o agente contando a troca sem repetir os valores que oferece —
// o protocolo permite descrever a mudança só pelo valor corrente.
func (a *agenteFalso) avisaSemLista(modelo string) {
	a.mu.Lock()
	aviso := a.anuncia
	a.mu.Unlock()
	if aviso == nil {
		return
	}
	aviso("sessao-1", []acp.ConfigOption{{
		ID:           "model",
		Name:         "Modelo",
		Category:     acp.CategoryModel,
		CurrentValue: modelo,
	}})
}

// sessaoFalsa é a sessão do agente falso.
type sessaoFalsa struct {
	agente *agenteFalso
	id     string
	cwd    string
}

func (s *sessaoFalsa) ID() string { return s.id }

func (s *sessaoFalsa) Prompt(context.Context, []acp.Content, acp.UpdateSink) (acp.StopReason, error) {
	return acp.StopEndTurn, nil
}

func (s *sessaoFalsa) Close(context.Context) error  { return nil }
func (s *sessaoFalsa) Cancel(context.Context) error { return nil }

func (s *sessaoFalsa) Commands() []acp.Command {
	s.agente.mu.Lock()
	defer s.agente.mu.Unlock()
	return append([]acp.Command(nil), s.agente.comandos...)
}

func (s *sessaoFalsa) ConfigOptions() []acp.ConfigOption {
	s.agente.mu.Lock()
	defer s.agente.mu.Unlock()
	return s.agente.opcoes
}

func (s *sessaoFalsa) SetConfigOption(_ context.Context, id, value string) ([]acp.ConfigOption, error) {
	if s.agente.erroTroca != nil {
		return nil, s.agente.erroTroca
	}
	return s.agente.trocouPara(id, value), nil
}

var _ acp.Client = (*agenteFalso)(nil)
var _ acp.Session = (*sessaoFalsa)(nil)

// appComAgente monta o app como a produção o monta na parte que importa aqui: um
// manager de verdade, ligado ao evento de opções pelo mesmo callback do
// initACP, com um agente falso no lugar do processo.
func appComAgente(t *testing.T, agente *agenteFalso) (*App, *testEmitter) {
	t.Helper()
	emissor := &testEmitter{}
	a := &App{ctx: context.Background(), emitter: emissor}
	a.setCurrentUserID("dono-1")
	a.acpMgr = acp.NewManager(acp.ManagerConfig{
		WorkDir:           func() (string, error) { return t.TempDir(), nil },
		OnSessionOptions:  a.agentSessionOptionsChanged,
		OnSessionCommands: a.agentSessionCommandsChanged,
		Dial: func(cfg acp.Config, _ acp.RequestHandler) (acp.Client, error) {
			agente.mu.Lock()
			agente.anuncia = cfg.OnConfigOptions
			agente.anunciaCmd = cfg.OnCommands
			agente.mu.Unlock()
			return agente, nil
		},
	})
	t.Cleanup(a.acpMgr.Shutdown)
	return a, emissor
}

func optionsAPI(a *App) *wailsapi.ACPOptions {
	api := wailsapi.NewACPOptions()
	wailsapi.AttachACPOptions(api, wailsSession{app: a}, a.acpMgr, a.noticePermissionBarrier)
	return api
}

// conversaComSessao faz nascer a sessão da conversa pelo caminho que o turno
// usa. Sem isso não há o que mostrar nem o que trocar, e é justamente a diferença
// entre a conversa que já falou com o agente e a que ainda não falou.
func conversaComSessao(t *testing.T, a *App, conversationID string) {
	t.Helper()
	spec := acp.ProviderSpec{ID: "cursor", Name: "Cursor", Command: "cursor-agent", Args: []string{"acp"}}
	if _, err := a.acpMgr.Conversation(context.Background(), spec, conversationID); err != nil {
		t.Fatalf("montar a sessão da conversa: %v", err)
	}
}

func TestOptionsDaConversaTrazemModeloEModoDoAgente(t *testing.T) {
	agente := novoAgenteFalso()
	a, _ := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	out, err := optionsAPI(a).GetAgentSessionOptions("conversa-1")
	if err != nil {
		t.Fatalf("GetAgentSessionOptions: %v", err)
	}
	if !out.Available {
		t.Fatal("a conversa fala com um agente que oferece escolhas, mas o seletor foi escondido")
	}
	modelo := opcaoPorCategoria(out.Options, acp.CategoryModel)
	if modelo == nil {
		t.Fatalf("nenhuma opção de modelo em %+v", out.Options)
	}
	if modelo.CurrentValue != "modelo-a" {
		t.Fatalf("modelo corrente = %q, esperado modelo-a", modelo.CurrentValue)
	}
	if len(modelo.Values) != 2 {
		t.Fatalf("valores de modelo = %d, esperado 2", len(modelo.Values))
	}
	if modelo.Values[0].Name != "Modelo A" {
		t.Fatalf("o rótulo do agente não chegou à tela: %+v", modelo.Values[0])
	}
	modo := opcaoPorCategoria(out.Options, acp.CategoryMode)
	if modo == nil || modo.CurrentValue != "agent" {
		t.Fatalf("modo corrente não chegou: %+v", modo)
	}
}

func TestConversaSemSessaoNaoMostraSeletorNemSobeAgente(t *testing.T) {
	agente := novoAgenteFalso()
	a, _ := appComAgente(t, agente)

	out, err := optionsAPI(a).GetAgentSessionOptions("conversa-que-nunca-falou")
	if err != nil {
		t.Fatalf("GetAgentSessionOptions: %v", err)
	}
	if out.Available || len(out.Options) != 0 {
		t.Fatalf("conversa sem sessão ofereceu seletor: %+v", out)
	}
	agente.mu.Lock()
	sessoes := len(agente.sessoes)
	agente.mu.Unlock()
	if sessoes != 0 {
		t.Fatalf("consultar o seletor abriu %d sessões no agente", sessoes)
	}
}

func TestTrocarModeloPelaTelaValeNoAgente(t *testing.T) {
	agente := novoAgenteFalso()
	a, _ := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	out, err := optionsAPI(a).SetAgentSessionOption("conversa-1", "model", "modelo-b")
	if err != nil {
		t.Fatalf("SetAgentSessionOption: %v", err)
	}
	if trocas := agente.trocasFeitas(); len(trocas) != 1 || trocas[0] != "model=modelo-b" {
		t.Fatalf("o agente recebeu %v", trocas)
	}
	if modelo := opcaoPorCategoria(out.Options, acp.CategoryModel); modelo == nil || modelo.CurrentValue != "modelo-b" {
		t.Fatalf("a resposta da troca não trouxe o modelo novo: %+v", out.Options)
	}

	// O turno seguinte é o que precisa sair no modelo novo: quem lê o estado
	// depois da troca precisa ver o que o agente passou a usar.
	depois, err := optionsAPI(a).GetAgentSessionOptions("conversa-1")
	if err != nil {
		t.Fatalf("GetAgentSessionOptions depois da troca: %v", err)
	}
	if modelo := opcaoPorCategoria(depois.Options, acp.CategoryModel); modelo == nil || modelo.CurrentValue != "modelo-b" {
		t.Fatalf("o modelo não ficou trocado para o turno seguinte: %+v", depois.Options)
	}
}

func TestTrocarModoPelaTelaValeNoAgente(t *testing.T) {
	agente := novoAgenteFalso()
	a, _ := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	out, err := optionsAPI(a).SetAgentSessionOption("conversa-1", acp.CategoryMode, "plan")
	if err != nil {
		t.Fatalf("SetAgentSessionOption: %v", err)
	}
	if modo := opcaoPorCategoria(out.Options, acp.CategoryMode); modo == nil || modo.CurrentValue != "plan" {
		t.Fatalf("o modo não ficou trocado: %+v", out.Options)
	}
}

func TestTrocaEmConversaSemSessaoExplicaEmVezDeCalar(t *testing.T) {
	agente := novoAgenteFalso()
	a, _ := appComAgente(t, agente)

	_, err := optionsAPI(a).SetAgentSessionOption("conversa-sem-sessao", "model", "modelo-b")
	if err == nil {
		t.Fatal("trocar modelo de conversa sem sessão deveria explicar o motivo")
	}
	if trocas := agente.trocasFeitas(); len(trocas) != 0 {
		t.Fatalf("a troca chegou ao agente mesmo sem sessão: %v", trocas)
	}
}

func TestQuandoOAgenteTrocaDeModeloOAppAvisaATela(t *testing.T) {
	agente := novoAgenteFalso()
	a, emissor := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	agente.avisaSozinho("modelo-b")

	// Dois eventos: o da montagem da sessão — que leva as opções iniciais ao
	// frontend sem esperar notificação do agente — e o da troca contada.
	eventos := emissor.find("chat:agent_options")
	if len(eventos) != 2 {
		t.Fatalf("eventos de opções = %d, esperado 2", len(eventos))
	}
	evento, ok := eventos[1].data.(AgentSessionOptionsEvent)
	if !ok {
		t.Fatalf("payload inesperado: %T", eventos[0].data)
	}
	if evento.ConversationID != "conversa-1" {
		t.Fatalf("o evento não disse de que conversa é: %q", evento.ConversationID)
	}
	if evento.Model != "modelo-b" {
		t.Fatalf("modelo do evento = %q, esperado modelo-b", evento.Model)
	}
	if !evento.ModelChanged || !evento.Announce {
		t.Fatalf("troca de modelo do agente não foi marcada para anúncio: %+v", evento)
	}
	if modelo := opcaoPorCategoria(evento.Options, acp.CategoryModel); modelo == nil || modelo.CurrentValue != "modelo-b" {
		t.Fatalf("o evento não trouxe o seletor atualizado: %+v", evento.Options)
	}
}

func TestOAgenteRepetindoOMesmoModeloNaoRendeAnuncio(t *testing.T) {
	agente := novoAgenteFalso()
	a, emissor := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	// O agente conta o estado da sessão também sem nada ter mudado. Anunciar
	// cada repetição atropelaria a leitura da resposta em curso.
	agente.avisaSozinho("modelo-a")

	eventos := emissor.find("chat:agent_options")
	// O primeiro é o da montagem da sessão; o segundo, a repetição.
	if len(eventos) != 2 {
		t.Fatalf("eventos de opções = %d, esperado 2", len(eventos))
	}
	evento := eventos[1].data.(AgentSessionOptionsEvent)
	if evento.Announce || evento.ModelChanged {
		t.Fatalf("estado repetido foi tratado como troca: %+v", evento)
	}
}

// A troca que o agente faz sozinho é a que mais depende do aviso, e o agente
// pode contá-la sem repetir a lista de valores. Descartar o aviso porque não
// sobrou seletor para desenhar seria descartar a única notícia de que a conversa
// mudou de modelo.
func TestTrocaContadaSemListaDeValoresAindaChegaATela(t *testing.T) {
	agente := novoAgenteFalso()
	a, emissor := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	agente.avisaSemLista("modelo-b")

	eventos := emissor.find("chat:agent_options")
	// O primeiro é o da montagem da sessão; o segundo, a troca sem lista.
	if len(eventos) != 2 {
		t.Fatalf("eventos de opções = %d, esperado 2", len(eventos))
	}
	evento := eventos[1].data.(AgentSessionOptionsEvent)
	if evento.Model != "modelo-b" || !evento.ModelChanged || !evento.Announce {
		t.Fatalf("a troca do agente não chegou marcada para anúncio: %+v", evento)
	}
}

// Aviso sem valores e sem troca nenhuma não tem o que fazer na tela: não há
// seletor a desenhar nem notícia a dar.
func TestAvisoSemValoresESemTrocaNaoViraEvento(t *testing.T) {
	agente := novoAgenteFalso()
	a, emissor := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	agente.avisaSemLista("modelo-a")

	// O único evento é o da montagem da sessão; o aviso sem valores e sem
	// troca nenhuma não vira evento.
	eventos := emissor.find("chat:agent_options")
	if len(eventos) != 1 {
		t.Fatalf("esperava só o evento da montagem, obtive %d: %+v", len(eventos), eventos)
	}
}

// Escolher um modo que dispensa o pedido de permissão derruba a única barreira
// que o app tem para autorizar o que o agente faz na máquina (AEP-0084 D9), e o
// seletor que recebeu a escolha não fica na tela contando isso.
func TestTrocarParaModoQueDispensaAPerguntaAvisaAConversa(t *testing.T) {
	agente := novoAgenteFalso()
	a, emissor := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	if _, err := optionsAPI(a).SetAgentSessionOption("conversa-1", acp.CategoryMode, "dontAsk"); err != nil {
		t.Fatalf("SetAgentSessionOption: %v", err)
	}

	aviso := avisoUnico(t, emissor)
	if aviso.Kind != ports.ChatNoticeKindModeSkipsPermission {
		t.Fatalf("motivo do aviso = %q", aviso.Kind)
	}
	if aviso.ConversationID != "conversa-1" {
		t.Fatalf("o aviso não disse de que conversa é: %q", aviso.ConversationID)
	}
	// Pelo nome que o agente deu ao modo: `dontAsk` lido em voz alta é inglês
	// no meio do português, e não diz a ninguém o que passou a valer.
	if aviso.Mode != "Não perguntar" {
		t.Fatalf("o aviso não nomeou o modo como o seletor o nomeia: %q", aviso.Mode)
	}
}

// Sem rótulo do agente sobra o valor cru — último recurso, mas melhor do que um
// aviso que não diz de que modo fala.
func TestModoSemRotuloDoAgenteEntraNoAvisoPeloValorCru(t *testing.T) {
	agente := novoAgenteFalso()
	a, emissor := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	if _, err := optionsAPI(a).SetAgentSessionOption("conversa-1", acp.CategoryMode, "bypassPermissions"); err != nil {
		t.Fatalf("SetAgentSessionOption: %v", err)
	}

	if aviso := avisoUnico(t, emissor); aviso.Mode != "bypassPermissions" {
		t.Fatalf("o aviso saiu sem nomear o modo: %+v", aviso)
	}
}

// O valor corrente e o da lista vêm os dois do agente, e nada garante que ele
// escreva os dois igual. Reconhecer o modo para alertar e não reconhecê-lo para
// nomear jogaria o identificador cru numa frase que já tinha rótulo.
func TestOModoENomeadoMesmoQuandoOAgenteMudaACaixaDoValor(t *testing.T) {
	agente := novoAgenteFalso()
	a, emissor := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	if _, err := optionsAPI(a).SetAgentSessionOption("conversa-1", acp.CategoryMode, "DONTASK"); err != nil {
		t.Fatalf("SetAgentSessionOption: %v", err)
	}

	aviso := avisoUnico(t, emissor)
	if aviso.Kind != ports.ChatNoticeKindModeSkipsPermission {
		t.Fatalf("motivo do aviso = %q", aviso.Kind)
	}
	if aviso.Mode != "Não perguntar" {
		t.Fatalf("o aviso não achou o rótulo do modo na lista: %q", aviso.Mode)
	}
}

// A barreira que volta fecha o aviso anterior: quem leu que o agente ia agir
// sozinho precisa saber quando isso deixou de valer, e nada mais na tela conta
// essa volta.
func TestVoltarParaModoQuePerguntaAvisaQueABarreiraVoltou(t *testing.T) {
	agente := novoAgenteFalso()
	a, emissor := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	if _, err := optionsAPI(a).SetAgentSessionOption("conversa-1", acp.CategoryMode, "dontAsk"); err != nil {
		t.Fatalf("ligar o modo sem pergunta: %v", err)
	}
	if _, err := optionsAPI(a).SetAgentSessionOption("conversa-1", acp.CategoryMode, "plan"); err != nil {
		t.Fatalf("voltar ao modo que pergunta: %v", err)
	}

	avisos := avisosDaConversa(t, emissor)
	if len(avisos) != 2 {
		t.Fatalf("avisos = %d, esperado 2: %+v", len(avisos), avisos)
	}
	if avisos[1].Kind != ports.ChatNoticeKindModeAsksPermission {
		t.Fatalf("a volta da barreira não foi contada: %+v", avisos[1])
	}
	if avisos[1].Mode != "plan" {
		t.Fatalf("o aviso da volta não nomeou o modo: %+v", avisos[1])
	}
}

// O aviso é da transição, e não do estado: quem já estava sem barreira e trocou
// para outro modo que também não pergunta não perdeu nada de novo. É o mesmo
// acordo da autorização permanente, que não se repete a cada pedido.
func TestTrocarEntreDoisModosSemPerguntaNaoRepeteOAviso(t *testing.T) {
	agente := novoAgenteFalso()
	a, emissor := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	if _, err := optionsAPI(a).SetAgentSessionOption("conversa-1", acp.CategoryMode, "dontAsk"); err != nil {
		t.Fatalf("ligar o modo sem pergunta: %v", err)
	}
	if _, err := optionsAPI(a).SetAgentSessionOption("conversa-1", acp.CategoryMode, "bypassPermissions"); err != nil {
		t.Fatalf("trocar para o outro modo sem pergunta: %v", err)
	}

	if avisos := avisosDaConversa(t, emissor); len(avisos) != 1 {
		t.Fatalf("avisos = %d, esperado 1: %+v", len(avisos), avisos)
	}
}

// Modo que continua pedindo permissão não vira aviso nenhum. O `acceptEdits`
// está aqui de propósito: ele dispensa a pergunta só para edição, e dizer que a
// barreira caiu inteira seria falso — a lista de modos conhecidos erra para o
// lado do silêncio.
func TestTrocaEntreModosQueContinuamPerguntandoNaoRendeAviso(t *testing.T) {
	agente := novoAgenteFalso()
	a, emissor := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	for _, modo := range []string{"plan", "acceptEdits", "agent"} {
		if _, err := optionsAPI(a).SetAgentSessionOption("conversa-1", acp.CategoryMode, modo); err != nil {
			t.Fatalf("trocar para %q: %v", modo, err)
		}
	}

	if avisos := avisosDaConversa(t, emissor); len(avisos) != 0 {
		t.Fatalf("troca entre modos que perguntam virou aviso: %+v", avisos)
	}
}

// Troca recusada pelo agente não mudou barreira nenhuma: a sessão segue no modo
// em que estava, e avisar contaria uma mudança que não houve.
func TestTrocaDeModoRecusadaPeloAgenteNaoAvisaNada(t *testing.T) {
	agente := novoAgenteFalso()
	agente.erroTroca = errors.New("modo indisponível")
	a, emissor := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	if _, err := optionsAPI(a).SetAgentSessionOption("conversa-1", acp.CategoryMode, "dontAsk"); err == nil {
		t.Fatal("a recusa do agente deveria virar erro")
	}

	if avisos := avisosDaConversa(t, emissor); len(avisos) != 0 {
		t.Fatalf("troca que não valeu virou aviso: %+v", avisos)
	}
}

// O modo muda por `config_option_update` sem ninguém ter clicado. O aviso é
// sobre a barreira ter caído, e não sobre quem a derrubou — e neste caso a
// pessoa tem ainda menos como saber.
func TestModoSemPerguntaLigadoPeloProprioAgenteTambemAvisa(t *testing.T) {
	agente := novoAgenteFalso()
	a, emissor := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	agente.avisaModo("bypassPermissions")

	aviso := avisoUnico(t, emissor)
	if aviso.Kind != ports.ChatNoticeKindModeSkipsPermission {
		t.Fatalf("motivo do aviso = %q", aviso.Kind)
	}
	if aviso.ConversationID != "conversa-1" {
		t.Fatalf("o aviso não disse de que conversa é: %q", aviso.ConversationID)
	}
}

// A primeira leitura de uma sessão é o estado inicial dela, não uma queda: a
// conversa que nasce num modo sem pergunta mostra esse modo no seletor desde o
// começo, e um aviso ali diria que algo mudou quando nada mudou. O agente
// repetindo o mesmo modo depois também não conta.
func TestSessaoQueJaNasceSemPerguntaNaoViraAvisoDeQueda(t *testing.T) {
	agente := &agenteFalso{opcoes: opcoesDeAgente("modelo-a", "dontAsk")}
	a, emissor := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	agente.avisaModo("dontAsk")

	if avisos := avisosDaConversa(t, emissor); len(avisos) != 0 {
		t.Fatalf("o estado inicial da sessão virou aviso: %+v", avisos)
	}
}

// avisosDaConversa lê os `chat:notice` que chegaram à tela.
func avisosDaConversa(t *testing.T, emissor *testEmitter) []ports.ChatNoticeEvent {
	t.Helper()
	eventos := emissor.find("chat:notice")
	out := make([]ports.ChatNoticeEvent, 0, len(eventos))
	for _, evento := range eventos {
		aviso, ok := evento.data.(ports.ChatNoticeEvent)
		if !ok {
			t.Fatalf("payload inesperado em chat:notice: %T", evento.data)
		}
		out = append(out, aviso)
	}
	return out
}

func avisoUnico(t *testing.T, emissor *testEmitter) ports.ChatNoticeEvent {
	t.Helper()
	avisos := avisosDaConversa(t, emissor)
	if len(avisos) != 1 {
		t.Fatalf("avisos = %d, esperado 1: %+v", len(avisos), avisos)
	}
	return avisos[0]
}

func TestLigacoesDeModeloExigemSessaoAutenticada(t *testing.T) {
	a := &App{ctx: context.Background()}
	mgr := acp.NewManager(acp.ManagerConfig{
		WorkDir: func() (string, error) { return t.TempDir(), nil },
	})
	t.Cleanup(mgr.Shutdown)
	api := wailsapi.NewACPOptions()
	wailsapi.AttachACPOptions(api, wailsSession{app: a}, mgr, a.noticePermissionBarrier)

	if _, err := api.GetAgentSessionOptions("conversa-1"); err == nil {
		t.Fatal("GetAgentSessionOptions sem sessão autenticada deveria falhar")
	}
	if _, err := api.SetAgentSessionOption("conversa-1", "model", "modelo-b"); err == nil {
		t.Fatal("SetAgentSessionOption sem sessão autenticada deveria falhar")
	}
}

// opcaoPorCategoria acha a opção de uma categoria no que a tela recebeu.
func opcaoPorCategoria(options []apidto.AgentConfigOption, category string) *apidto.AgentConfigOption {
	for i := range options {
		if strings.EqualFold(options[i].Category, category) {
			return &options[i]
		}
	}
	return nil
}
