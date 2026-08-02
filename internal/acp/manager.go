package acp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"

	"assistente/internal/logging"
)

const managerComponent = "acp.manager"

// ProviderSpec descreve o provider ACP para o manager. É de propósito uma cópia
// magra do que o provider guarda: o transporte não conhece o pacote de
// providers, e o que ele precisa saber é como subir o agente e por qual nome
// chamá-lo.
type ProviderSpec struct {
	ID      string
	Name    string
	Command string
	Args    []string
	Env     map[string]string
}

func (s ProviderSpec) validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return errors.New("provider ACP sem identificador")
	}
	if strings.TrimSpace(s.Command) == "" {
		return errors.New("provider ACP sem comando do agente")
	}
	return nil
}

// sameProcess diz se dois specs sobem o mesmo agente. Mudar comando, argumento
// ou variável de ambiente é passar a falar com outro programa: o processo de pé
// precisa morrer, e as sessões que viviam nele não valem mais.
func (s ProviderSpec) sameProcess(other ProviderSpec) bool {
	return s.Command == other.Command &&
		slices.Equal(s.Args, other.Args) &&
		maps.Equal(s.Env, other.Env)
}

// SessionOrigin diz de onde veio a sessão que a conversa está usando. Não é
// detalhe interno: quando o agente não retoma a sessão anterior, ele esqueceu a
// conversa, e isso precisa ser dito à pessoa em vez de descoberto pela resposta
// estranha (AEP-0084 D4).
type SessionOrigin int

const (
	// SessionNew é a primeira sessão desta conversa com este agente.
	SessionNew SessionOrigin = iota
	// SessionResumed é a sessão registrada, retomada com session/load: o agente
	// continua sabendo o que já foi conversado.
	SessionResumed
	// SessionRecreated é sessão nova no lugar de uma registrada que não voltou.
	// A memória anterior do agente se perdeu.
	SessionRecreated
)

func (o SessionOrigin) String() string {
	switch o {
	case SessionResumed:
		return "retomada"
	case SessionRecreated:
		return "recriada"
	default:
		return "nova"
	}
}

// LostMemory diz se o agente perdeu o contexto anterior da conversa.
func (o SessionOrigin) LostMemory() bool { return o == SessionRecreated }

// ManagerConfig monta o serviço.
type ManagerConfig struct {
	// Store persiste o vínculo conversa↔sessão. Nulo é aceito: o manager
	// funciona só em memória e toda conversa recomeça sem memória do agente.
	Store SessionStore

	// Handler decide os pedidos que o agente faz ao app (permissões, extensões).
	// Nulo nega tudo na hora, que é o comportamento seguro para quem não tem
	// interlocutor (AEP-0084 D9).
	Handler RequestHandler

	// WorkDir devolve o diretório sobre o qual o agente age (AEP-0084 D5).
	// Padrão: o diretório de trabalho do app.
	WorkDir func() (string, error)

	// ClientName e ClientVersion identificam o app no handshake.
	ClientName    string
	ClientVersion string

	// Dial existe para os testes trocarem o transporte. Padrão: New.
	Dial func(cfg Config, handler RequestHandler) (Client, error)
}

// Manager é o dono dos processos e das sessões ACP (AEP-0084 D3): um processo
// por provider configurado, subindo no primeiro uso e vivendo até o shutdown,
// com uma sessão por conversa multiplexada nele.
//
// O dono precisa ser um serviço de longa duração, e não o ChatProvider:
// GetChatProvider devolve uma instância nova a cada chamada, então guardar
// processo ou sessão dentro dela daria um agente por turno — e a fila que
// impede dois turnos simultâneos na mesma sessão (D10) guardaria nada, porque
// cada goroutine seguraria o mutex de um objeto diferente. Aqui a fila vem de
// graça: as duas goroutines do barge-in recebem a mesma Session.
type Manager struct {
	store         SessionStore
	handler       RequestHandler
	workDir       func() (string, error)
	clientName    string
	clientVersion string
	dial          func(Config, RequestHandler) (Client, error)

	// mu protege os mapas. Ordem dos locks: quem segura o de uma conversa pode
	// pegar este; o contrário nunca — nenhum caminho daqui segura mu enquanto
	// toca no lock de uma conversa.
	mu     sync.Mutex
	procs  map[string]*agentProcess
	convs  map[string]*Conversation
	closed bool
}

// agentProcess é o processo de um provider e o spec que o descreve, para
// detectar quando a configuração mudou debaixo dele.
type agentProcess struct {
	spec   ProviderSpec
	client Client
}

func NewManager(cfg ManagerConfig) *Manager {
	m := &Manager{
		store:         cfg.Store,
		handler:       cfg.Handler,
		workDir:       cfg.WorkDir,
		clientName:    cfg.ClientName,
		clientVersion: cfg.ClientVersion,
		dial:          cfg.Dial,
		procs:         make(map[string]*agentProcess),
		convs:         make(map[string]*Conversation),
	}
	if m.workDir == nil {
		m.workDir = os.Getwd
	}
	if m.dial == nil {
		m.dial = New
	}
	return m
}

// Client devolve a conexão do provider, criando-a se ainda não existir. O
// processo só sobe no primeiro pedido que precisar dele de verdade.
func (m *Manager) Client(spec ProviderSpec) (Client, error) {
	proc, err := m.process(spec)
	if err != nil {
		return nil, err
	}
	return proc.client, nil
}

func (m *Manager) process(spec ProviderSpec) (*agentProcess, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
	if proc := m.procs[spec.ID]; proc != nil {
		if proc.spec.sameProcess(spec) {
			return proc, nil
		}
		// A configuração mudou: o que está de pé é outro agente. Fechar em
		// segundo plano porque derrubar processo pode demorar, e quem chegou
		// aqui só quer o novo.
		stale := proc
		go func() {
			if err := stale.client.Close(); err != nil {
				logging.Warnf(context.Background(), managerComponent,
					"[ACP] erro ao encerrar o agente antigo do provider %q: %v", stale.spec.ID, err)
			}
		}()
		delete(m.procs, spec.ID)
	}

	client, err := m.dial(Config{
		Command:       spec.Command,
		Args:          slices.Clone(spec.Args),
		Env:           maps.Clone(spec.Env),
		WorkDir:       m.processWorkDir(),
		ClientName:    m.clientName,
		ClientVersion: m.clientVersion,
	}, m.handler)
	if err != nil {
		return nil, err
	}
	proc := &agentProcess{spec: spec, client: client}
	m.procs[spec.ID] = proc
	return proc, nil
}

// processWorkDir é o diretório do processo. Um erro aqui não impede subir o
// agente: sem diretório o processo herda o do app, que é justamente o que o D5
// manda usar.
func (m *Manager) processWorkDir() string {
	dir, err := m.workDir()
	if err != nil {
		return ""
	}
	return dir
}

// Conversation devolve a sessão desta conversa com este provider, montando-a
// quando preciso: retoma a registrada quando o agente sabe retomar e abre uma
// nova quando não. Chamadas concorrentes para a mesma conversa recebem a mesma
// sessão — é o que faz a fila de turno do D10 valer para todo mundo.
func (m *Manager) Conversation(ctx context.Context, spec ProviderSpec, conversationID string) (*Conversation, error) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, errors.New("conversa sem identificador para sessão ACP")
	}
	if err := spec.validate(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrClosed
	}
	conv := m.convs[conversationID]
	if conv == nil {
		conv = &Conversation{manager: m, id: conversationID}
		m.convs[conversationID] = conv
	}
	m.mu.Unlock()

	if err := conv.ensure(ctx, spec); err != nil {
		return nil, err
	}
	return conv, nil
}

// CloseConversation encerra a sessão da conversa no agente e esquece o
// registro. É o que limpar ou excluir a conversa faz: a memória do agente
// deixou de corresponder a algo que a pessoa ainda vê.
//
// Se o processo do provider não está de pé, não vale subi-lo só para despedir:
// a sessão já não existe em lugar nenhum e basta apagar o registro.
func (m *Manager) CloseConversation(ctx context.Context, conversationID string) error {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil
	}

	m.mu.Lock()
	conv := m.convs[conversationID]
	delete(m.convs, conversationID)
	m.mu.Unlock()

	var errs []error
	if conv != nil {
		if err := conv.close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if m.store != nil {
		if err := m.store.Delete(ctx, conversationID); err != nil {
			errs = append(errs, fmt.Errorf("apagar registro da sessão ACP: %w", err))
		}
	}
	return errors.Join(errs...)
}

// DisconnectAll derruba os processos e esquece as sessões em memória, deixando
// o serviço utilizável. É o que a troca de usuário precisa: identificador de
// provider se repete entre pessoas, e um processo herdado faria a conversa de
// uma falar com o agente que a outra configurou (AEP-0052).
//
// Os registros no banco ficam — eles são por usuário, e cada um reencontra os
// seus no próximo login.
func (m *Manager) DisconnectAll() {
	m.drop(false)
}

// Shutdown derruba os processos dos agentes e fecha o serviço. Os registros das
// sessões ficam no banco de propósito: o agente costuma sobreviver ao app e, na
// volta, o session/load recupera a conversa de onde ela parou (AEP-0084 D4).
func (m *Manager) Shutdown() {
	m.drop(true)
}

func (m *Manager) drop(closing bool) {
	m.mu.Lock()
	procs := make([]*agentProcess, 0, len(m.procs))
	for _, proc := range m.procs {
		procs = append(procs, proc)
	}
	m.procs = make(map[string]*agentProcess)
	m.convs = make(map[string]*Conversation)
	if closing {
		m.closed = true
	}
	m.mu.Unlock()

	for _, proc := range procs {
		if err := proc.client.Close(); err != nil {
			logging.Warnf(context.Background(), managerComponent,
				"[ACP] erro ao encerrar o agente do provider %q: %v", proc.spec.ID, err)
		}
	}
}

// Conversation é a sessão de uma conversa com um agente, com o que o app
// precisa lembrar sobre ela entre turnos.
type Conversation struct {
	manager *Manager
	id      string

	// mu protege a montagem e a troca da sessão. Ordem dos locks: quem segura
	// este pode pegar o do manager, nunca o contrário.
	mu         sync.Mutex
	proc       *agentProcess
	providerID string
	session    Session
	origin     SessionOrigin
	prefixHash string
}

func (c *Conversation) ensure(ctx context.Context, spec ProviderSpec) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	proc, err := c.manager.process(spec)
	if err != nil {
		return err
	}
	if c.session != nil && c.proc == proc && c.providerID == spec.ID {
		return nil
	}
	// Ou o processo é outro (caiu, ou a configuração mudou) ou o provider é
	// outro: nos dois casos a sessão que estava aqui não existe mais do lado de
	// lá. Remontar é o caminho.
	c.session = nil
	c.proc = nil

	dir, err := c.manager.workDir()
	if err != nil {
		return fmt.Errorf("diretório de trabalho do agente ACP: %w", err)
	}

	var stored *StoredSession
	if c.manager.store != nil {
		stored, err = c.manager.store.Load(ctx, c.id, spec.ID)
		if err != nil {
			// Sem saber se havia sessão, não dá para escolher entre retomar e
			// abrir outra. Abrir outra por otimismo deixaria a sessão anterior
			// órfã no agente e faria a pessoa perder a memória em silêncio.
			return fmt.Errorf("ler registro da sessão ACP da conversa %s: %w", c.id, err)
		}
	}

	session, origin, prefix := c.resume(ctx, proc, stored, dir)
	if session == nil {
		session, err = proc.client.NewSession(ctx, dir)
		if err != nil {
			return err
		}
		prefix = ""
		if err := c.manager.saveSession(ctx, StoredSession{
			ConversationID: c.id,
			ProviderID:     spec.ID,
			SessionID:      session.ID(),
			WorkDir:        dir,
		}); err != nil {
			// Sem registro, o próximo turno abriria outra sessão e esta ficaria
			// aberta no agente sem ninguém para fechá-la.
			c.closeOrphan(ctx, session)
			return err
		}
	}

	c.proc = proc
	c.providerID = spec.ID
	c.session = session
	c.origin = origin
	c.prefixHash = prefix
	return nil
}

// resume tenta retomar a sessão registrada. Devolve sessão nula quando não há o
// que retomar ou quando a retomada falhou, junto da origem que explica o caso.
func (c *Conversation) resume(ctx context.Context, proc *agentProcess, stored *StoredSession, dir string) (Session, SessionOrigin, string) {
	if stored == nil || strings.TrimSpace(stored.SessionID) == "" {
		return nil, SessionNew, ""
	}
	if stored.WorkDir != dir {
		// Retomar em outro diretório seria continuar a conversa sobre outros
		// arquivos, com o agente achando que é a mesma (AEP-0084 D5).
		logging.Infof(ctx, managerComponent,
			"[ACP] conversa %s abre sessão nova: o diretório mudou de %q para %q", c.id, stored.WorkDir, dir)
		return nil, SessionRecreated, ""
	}
	caps, err := proc.client.Capabilities(ctx)
	if err != nil {
		logging.Warnf(ctx, managerComponent,
			"[ACP] conversa %s: agente %q indisponível para retomar a sessão: %v", c.id, proc.spec.ID, err)
		return nil, SessionRecreated, ""
	}
	if !caps.LoadSession {
		return nil, SessionRecreated, ""
	}
	session, err := proc.client.LoadSession(ctx, stored.SessionID, dir)
	if err != nil {
		logging.Warnf(ctx, managerComponent,
			"[ACP] conversa %s: o agente não retomou a sessão %q: %v", c.id, stored.SessionID, err)
		return nil, SessionRecreated, ""
	}
	return session, SessionResumed, stored.PrefixHash
}

// closeOrphan fecha uma sessão que o app não vai usar. O contexto de quem pediu
// pode já estar cancelado — é justamente o caso em que a sessão ficaria aberta.
func (c *Conversation) closeOrphan(ctx context.Context, session Session) {
	detached, cancel := context.WithTimeout(context.WithoutCancel(ctx), storeTimeout)
	defer cancel()
	if err := session.Close(detached); err != nil {
		logging.Warnf(detached, managerComponent,
			"[ACP] conversa %s: sessão %q ficou aberta no agente: %v", c.id, session.ID(), err)
	}
}

func (c *Conversation) close(ctx context.Context) error {
	c.mu.Lock()
	session := c.session
	c.session = nil
	c.proc = nil
	c.mu.Unlock()

	if session == nil {
		return nil
	}
	if err := session.Close(ctx); err != nil && !errors.Is(err, ErrSessionClosed) && !errors.Is(err, ErrSessionLost) {
		return fmt.Errorf("encerrar sessão ACP da conversa %s: %w", c.id, err)
	}
	return nil
}

// Session é a sessão viva desta conversa.
func (c *Conversation) Session() Session {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.session
}

// Origin diz como a sessão atual foi obtida. Vale a leitura antes do primeiro
// turno: sessão recriada é agente sem a memória anterior.
func (c *Conversation) Origin() SessionOrigin {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.origin
}

// Invalidate esquece a sessão em memória sem apagar o registro. É o que fazer
// quando o turno volta com ErrSessionLost: o próximo uso tenta retomar pelo
// identificador guardado e, se o agente não retomar, abre outra avisando.
func (c *Conversation) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.session = nil
	c.proc = nil
}

// NeedsPrefix diz se o prefixo estável do perfil ainda precisa ser entregue a
// esta sessão. Hash diferente é perfil trocado, e as instruções passaram a ser
// outras (AEP-0084 D4).
func (c *Conversation) NeedsPrefix(hash string) bool {
	if strings.TrimSpace(hash) == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.prefixHash != hash
}

// MarkPrefixSent registra que a sessão já ouviu este prefixo. Persiste junto do
// identificador da sessão porque uma sessão retomada depois de reiniciar o app
// já ouviu a persona, e repetir tudo seria desperdício.
func (c *Conversation) MarkPrefixSent(ctx context.Context, hash string) error {
	c.mu.Lock()
	if c.prefixHash == hash {
		c.mu.Unlock()
		return nil
	}
	c.prefixHash = hash
	providerID := c.providerID
	c.mu.Unlock()

	if c.manager.store == nil {
		return nil
	}
	detached, cancel := context.WithTimeout(context.WithoutCancel(ctx), storeTimeout)
	defer cancel()
	if err := c.manager.store.SavePrefixHash(detached, c.id, providerID, hash); err != nil {
		return fmt.Errorf("anotar prefixo já enviado à sessão ACP: %w", err)
	}
	return nil
}

// saveSession grava o vínculo fora do cancelamento do turno: o pedido que
// criou a sessão pode ser abortado no instante seguinte (barge-in), e perder o
// registro deixaria a sessão órfã no agente.
func (m *Manager) saveSession(ctx context.Context, rec StoredSession) error {
	if m.store == nil {
		return nil
	}
	detached, cancel := context.WithTimeout(context.WithoutCancel(ctx), storeTimeout)
	defer cancel()
	if err := m.store.Save(detached, rec); err != nil {
		return fmt.Errorf("registrar sessão ACP da conversa %s: %w", rec.ConversationID, err)
	}
	return nil
}