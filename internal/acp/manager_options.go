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

func currentValueOf(options []ConfigOption, category string) (string, bool) {
	option, ok := OptionByCategory(options, category)
	if !ok {
		return "", false
	}
	return option.CurrentValue, true
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
// A troca é anotada como conhecida antes de voltar: o agente costuma repetir a
// escolha por notificação, e sem a anotação a troca que a pessoa acabou de pedir
// voltaria anunciada como decisão dele.
func (c *Conversation) SetOption(ctx context.Context, id, value string) ([]ConfigOption, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("opção do agente sem identificador")
	}
	session := c.Session()
	if session == nil {
		return nil, errors.New("conversa sem sessão ACP para trocar opção do agente")
	}
	options, err := session.SetConfigOption(ctx, id, value)
	if err != nil {
		return nil, err
	}
	model, _ := currentValueOf(options, CategoryModel)
	mode, _ := currentValueOf(options, CategoryMode)
	c.manager.noteSessionValues(session.ID(), model, mode)
	return options, nil
}
