package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"assistente/internal/acp"
)

// agenteFalso é o agente do outro lado das ligações de modelo: guarda as opções
// que oferece, aceita a troca como o agente aceitaria e sabe anunciar por conta
// própria — que é como o fallback de limite de uso chega ao app (AEP-0084 D6).
type agenteFalso struct {
	mu        sync.Mutex
	opcoes    []acp.ConfigOption
	trocas    []string
	anuncia   func(sessionID string, options []acp.ConfigOption)
	sessoes   []*sessaoFalsa
	erroTroca error
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
		WorkDir:          func() (string, error) { return t.TempDir(), nil },
		OnSessionOptions: a.agentSessionOptionsChanged,
		Dial: func(cfg acp.Config, _ acp.RequestHandler) (acp.Client, error) {
			agente.mu.Lock()
			agente.anuncia = cfg.OnConfigOptions
			agente.mu.Unlock()
			return agente, nil
		},
	})
	t.Cleanup(a.acpMgr.Shutdown)
	return a, emissor
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

	out, err := a.GetAgentSessionOptions("conversa-1")
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

	out, err := a.GetAgentSessionOptions("conversa-que-nunca-falou")
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

	out, err := a.SetAgentSessionOption("conversa-1", "model", "modelo-b")
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
	depois, err := a.GetAgentSessionOptions("conversa-1")
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

	out, err := a.SetAgentSessionOption("conversa-1", acp.CategoryMode, "plan")
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

	_, err := a.SetAgentSessionOption("conversa-sem-sessao", "model", "modelo-b")
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

	eventos := emissor.find("chat:agent_options")
	if len(eventos) != 1 {
		t.Fatalf("eventos de opções = %d, esperado 1", len(eventos))
	}
	evento, ok := eventos[0].data.(AgentSessionOptionsEvent)
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
	if len(eventos) != 1 {
		t.Fatalf("eventos de opções = %d, esperado 1", len(eventos))
	}
	evento := eventos[0].data.(AgentSessionOptionsEvent)
	if evento.Announce || evento.ModelChanged {
		t.Fatalf("estado repetido foi tratado como troca: %+v", evento)
	}
}

func TestLigacoesDeModeloExigemSessaoAutenticada(t *testing.T) {
	a := &App{ctx: context.Background()}

	if _, err := a.GetAgentSessionOptions("conversa-1"); err == nil {
		t.Fatal("GetAgentSessionOptions sem sessão autenticada deveria falhar")
	}
	if _, err := a.SetAgentSessionOption("conversa-1", "model", "modelo-b"); err == nil {
		t.Fatal("SetAgentSessionOption sem sessão autenticada deveria falhar")
	}
}

// opcaoPorCategoria acha a opção de uma categoria no que a tela recebeu.
func opcaoPorCategoria(options []AgentConfigOption, category string) *AgentConfigOption {
	for i := range options {
		if strings.EqualFold(options[i].Category, category) {
			return &options[i]
		}
	}
	return nil
}
