package acp

import (
	"context"
	"errors"
	"strings"
)

// SessionOptionsEvent conta que as opções da sessão de uma conversa mudaram: o
// modelo, o modo, ou os valores que o agente oferece para eles.
//
// O agente troca de modelo por conta própria — fallback de limite de uso, por
// exemplo — e avisa por notificação, inclusive entre turnos (AEP-0084 D6). A
// pessoa precisa saber com quem está falando, e é este evento que permite dizer.
type SessionOptionsEvent struct {
	ConversationID string
	ProviderID     string

	// Options é o conjunto completo que a sessão conhece agora.
	Options []ConfigOption

	// Model e Mode são os valores correntes, quando o agente expõe a escolha.
	Model string
	Mode  string

	// ModelChanged e ModeChanged dizem que o valor passou a ser outro do que o
	// app conhecia. Só isso merece anúncio: o agente também repete o estado sem
	// nada ter mudado, e falar a cada repetição atropelaria a leitura da
	// resposta em curso.
	ModelChanged bool
	ModeChanged  bool
}

// Announceable diz se o evento conta uma mudança de verdade.
func (e SessionOptionsEvent) Announceable() bool { return e.ModelChanged || e.ModeChanged }

// sessionKnowledge é o que o app já sabe de uma sessão do agente. Vive num mapa
// próprio, com trava própria, porque quem o consulta é a goroutine que entrega
// as notificações do agente: ela não pode esperar pelo lock de uma conversa, que
// fica segurado enquanto uma sessão é aberta ou retomada.
type sessionKnowledge struct {
	conversationID string
	providerID     string
	model          string
	mode           string
}

// rememberSession registra de que conversa é uma sessão e em que estado o app a
// conhece. É por aqui que um aviso vindo do agente encontra a conversa dona
// dele, e é o que impede anunciar uma troca que o próprio app acabou de pedir.
func (m *Manager) rememberSession(sessionID string, known sessionKnowledge) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	m.knownMu.Lock()
	defer m.knownMu.Unlock()
	if m.known == nil {
		m.known = make(map[string]sessionKnowledge)
	}
	m.known[sessionID] = known
}

func (m *Manager) forgetSession(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	m.knownMu.Lock()
	defer m.knownMu.Unlock()
	delete(m.known, sessionID)
}

// noteSessionValues anota o modelo e o modo que o app passou a conhecer numa
// sessão, sem mexer no vínculo com a conversa. Vale para a troca que o app pediu:
// o agente costuma repetir a escolha por notificação, e sem esta anotação a
// própria troca da pessoa voltaria como se fosse decisão do agente.
func (m *Manager) noteSessionValues(sessionID, model, mode string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	m.knownMu.Lock()
	defer m.knownMu.Unlock()
	known, ok := m.known[sessionID]
	if !ok {
		return
	}
	if model != "" {
		known.model = model
	}
	if mode != "" {
		known.mode = mode
	}
	m.known[sessionID] = known
}

// noteSessionIntent anota o valor que o app está pedindo ao agente antes de o
// pedido sair, e devolve o que estava anotado para quem precisar desfazer.
//
// Antes, e não depois: o agente confirma a troca por notificação, e a entrega
// vem por outra goroutine — nada garante que ela chegue depois da resposta.
// Anotada só na volta, a confirmação da troca que a pessoa acabou de pedir seria
// comparada com o valor antigo e voltaria anunciada como decisão do agente.
func (m *Manager) noteSessionIntent(sessionID, category, value string) (previous string, noted bool) {
	sessionID = strings.TrimSpace(sessionID)
	value = strings.TrimSpace(value)
	if sessionID == "" || value == "" {
		return "", false
	}
	m.knownMu.Lock()
	defer m.knownMu.Unlock()
	known, ok := m.known[sessionID]
	if !ok {
		return "", false
	}
	switch category {
	case CategoryModel:
		previous, known.model = known.model, value
	case CategoryMode:
		previous, known.mode = known.mode, value
	default:
		// Opção de categoria que este pacote não representa: não há anotação a
		// fazer, e inventar uma faria o aviso do agente ser comparado com o
		// valor de outra coisa.
		return "", false
	}
	m.known[sessionID] = known
	return previous, true
}

// undoSessionIntent desfaz a anotação de uma troca que não valeu — o app não pode
// ficar achando que está num modelo que o agente recusou.
//
// Só desfaz se a anotação ainda for a que este pedido escreveu: o agente pode ter
// contado uma troca dele enquanto o pedido falhava, e essa é a verdade mais nova.
func (m *Manager) undoSessionIntent(sessionID, category, intended, previous string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	intended = strings.TrimSpace(intended)
	m.knownMu.Lock()
	defer m.knownMu.Unlock()
	known, ok := m.known[sessionID]
	if !ok {
		return
	}
	switch category {
	case CategoryModel:
		if known.model != intended {
			return
		}
		known.model = previous
	case CategoryMode:
		if known.mode != intended {
			return
		}
		known.mode = previous
	default:
		return
	}
	m.known[sessionID] = known
}

// sessionOptionsChanged traduz o aviso do transporte num evento de conversa. O
// transporte só sabe o nome da sessão; quem sabe de quem ela é somos nós.
func (m *Manager) sessionOptionsChanged(sessionID string, options []ConfigOption) {
	if m.onOptions == nil {
		return
	}
	model, _ := currentValueOf(options, CategoryModel)
	mode, _ := currentValueOf(options, CategoryMode)

	m.knownMu.Lock()
	known, ok := m.known[sessionID]
	if !ok {
		// Sessão que não é de conversa nenhuma: a de descoberta, ou uma que a
		// conversa já soltou. Não há a quem contar.
		m.knownMu.Unlock()
		return
	}
	event := SessionOptionsEvent{
		ConversationID: known.conversationID,
		ProviderID:     known.providerID,
		Options:        options,
		Model:          model,
		Mode:           mode,
		// Valor que o app nunca soube não conta como troca: a primeira leitura
		// de uma sessão é o estado inicial dela, e anunciá-la faria toda conversa
		// começar dizendo que o agente mudou de modelo.
		ModelChanged: known.model != "" && model != "" && known.model != model,
		ModeChanged:  known.mode != "" && mode != "" && known.mode != mode,
	}
	if model != "" {
		known.model = model
	}
	if mode != "" {
		known.mode = mode
	}
	m.known[sessionID] = known
	m.knownMu.Unlock()

	m.onOptions(event)
}

// currentValueOf lê o valor corrente de uma categoria, aparado. O espaço nas
// pontas some aqui porque este valor é comparado com o que o app já conhecia:
// cru, um agente que respondesse o mesmo modelo com um espaço a mais viraria
// anúncio de troca que não houve.
func currentValueOf(options []ConfigOption, category string) (string, bool) {
	option, ok := OptionByCategory(options, category)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(option.CurrentValue), true
}

// ProviderOptions são as opções que o agente deste provider oferece — modelos e
// modos —, lidas da sessão de descoberta e servidas do cache por processo
// (AEP-0084 D6). Sobe o agente se ele ainda não estiver de pé: a consulta de
// modelos é um primeiro uso como qualquer outro (D3).
func (m *Manager) ProviderOptions(ctx context.Context, spec ProviderSpec) ([]ConfigOption, error) {
	proc, err := m.process(spec)
	if err != nil {
		return nil, err
	}
	dir, err := m.currentDir()
	if err != nil {
		return nil, err
	}
	return proc.client.Options(ctx, dir)
}

// InvalidateProviderOptions descarta a lista guardada deste provider, para a
// próxima consulta perguntar ao agente de novo. É o que a pessoa pede ao
// recarregar a lista na tela (AEP-0084 D6).
//
// De propósito não sobe o agente: invalidar o que não foi descoberto não tem
// efeito nenhum, e pagar um spawn para isso seria absurdo.
func (m *Manager) InvalidateProviderOptions(providerID string) {
	m.mu.Lock()
	proc := m.procs[strings.TrimSpace(providerID)]
	m.mu.Unlock()
	if proc == nil {
		return
	}
	proc.client.InvalidateOptions()
}

// ConversationOptions são as opções da sessão de uma conversa, procurada pelo
// identificador. Conversa sem sessão de pé não tem opção nenhuma, e de propósito
// nada é aberto aqui: quem só quer saber em que modelo o agente está não deve
// fazer nascer nem processo nem sessão.
func (m *Manager) ConversationOptions(conversationID string) []ConfigOption {
	conv := m.lookup(conversationID)
	if conv == nil {
		return nil
	}
	return conv.Options()
}

// SetConversationOption troca uma opção da sessão de uma conversa, procurada pelo
// identificador. Conversa sem sessão não tem o que trocar: a escolha do modelo
// antes do primeiro turno é do perfil, e é ele que a leva ao agente quando a
// sessão nascer (AEP-0084 D6).
func (m *Manager) SetConversationOption(ctx context.Context, conversationID, id, value string) ([]ConfigOption, error) {
	conv := m.lookup(conversationID)
	if conv == nil {
		return nil, errors.New("esta conversa ainda não tem sessão com o agente")
	}
	return conv.SetOption(ctx, id, value)
}

// lookup acha a conversa sem criá-la, ao contrário de entry: quem consulta não
// deve deixar registro de uma conversa que nunca falou com o agente.
func (m *Manager) lookup(conversationID string) *Conversation {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.clearing {
		return nil
	}
	return m.convs[conversationID]
}

// Options são as opções da sessão desta conversa: em que modelo e modo o agente
// está agora, com os valores que ele oferece para cada um.
func (c *Conversation) Options() []ConfigOption {
	session := c.Session()
	if session == nil {
		return nil
	}
	return session.ConfigOptions()
}

// SetOption troca uma opção da sessão desta conversa e devolve o estado
// resultante — trocar de modelo pode mexer nas opções que dependem dele.
//
// A troca é anotada como conhecida antes de o pedido sair, e reconciliada com o
// estado que voltar: o agente costuma repetir a escolha por notificação, e sem a
// anotação a troca que a pessoa acabou de pedir voltaria anunciada como decisão
// dele.
func (c *Conversation) SetOption(ctx context.Context, id, value string) ([]ConfigOption, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("opção do agente sem identificador")
	}
	session := c.Session()
	if session == nil {
		return nil, errors.New("conversa sem sessão ACP para trocar opção do agente")
	}
	// A categoria sai do estado da sessão porque a anotação é por categoria —
	// modelo, modo — enquanto o identificador é escolha do agente.
	category := categoryOfOption(session.ConfigOptions(), id)
	previous, noted := c.manager.noteSessionIntent(session.ID(), category, value)

	options, err := session.SetConfigOption(ctx, id, value)
	if err != nil {
		if noted {
			c.manager.undoSessionIntent(session.ID(), category, value, previous)
		}
		return nil, err
	}
	// O que voltou é a verdade, e pode não ser o que foi pedido: o agente às
	// vezes acomoda o pedido em outro valor. Guardar o real é o que impede a
	// repetição dele virar anúncio de troca — quem exibe conta à pessoa o valor
	// que voltou, não o que ela pediu.
	model, _ := currentValueOf(options, CategoryModel)
	mode, _ := currentValueOf(options, CategoryMode)
	c.manager.noteSessionValues(session.ID(), model, mode)
	return options, nil
}

// categoryOfOption acha a categoria de uma opção pelo identificador que o app
// pediu, no conjunto que a sessão conhece.
func categoryOfOption(options []ConfigOption, id string) string {
	for _, option := range options {
		if option.ID == id {
			return option.Category
		}
	}
	return ""
}
