package acp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"assistente/internal/logging"
)

const managerComponent = "acp.manager"

// ErrConversationGone é o turno que chegou tarde: a conversa foi limpa ou
// excluída enquanto ele começava. Não é falha do agente, e retomar seria
// ressuscitar no banco um vínculo de algo que a pessoa acabou de apagar.
var ErrConversationGone = errors.New("conversa encerrada")

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

	// OnSessionOptions é avisado quando o modelo ou o modo da sessão de uma
	// conversa muda, inclusive por decisão do próprio agente (AEP-0084 D6).
	// Nulo apenas silencia o aviso.
	//
	// Roda na goroutine de entrega do transporte: precisa retornar rápido e não
	// pode voltar a falar com o agente.
	OnSessionOptions func(event SessionOptionsEvent)

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
	onOptions     func(SessionOptionsEvent)
	dial          func(Config, RequestHandler) (Client, error)

	// mu protege os mapas. Ordem dos locks: quem segura o de uma conversa pode
	// pegar este; o contrário nunca — nenhum caminho daqui segura mu enquanto
	// toca no lock de uma conversa.
	mu     sync.Mutex
	procs  map[string]*agentProcess
	convs  map[string]*Conversation
	closed bool
	// clearing marca o "limpar tudo" em andamento. Enquanto ele roda nenhuma
	// conversa é montada: a que nascesse no meio teria o vínculo apagado logo
	// depois de gravado.
	clearing bool

	// owners diz, por sessão do agente, de quem é o turno em voo (AEP-0084
	// D9). Lock próprio: quem consulta é a goroutine que entrega o pedido do
	// agente, e ela não pode ficar atrás do lock que sobe processo e monta
	// sessão — o agente fica parado esperando essa resposta.
	ownersMu   sync.Mutex
	owners     map[string]turnRegistration
	ownerToken uint64

	// known diz, por sessão do agente, de que conversa ela é e em que modelo e
	// modo o app a conhece (AEP-0084 D6). Lock próprio pelo mesmo motivo de
	// owners, e mais um: o aviso de troca de modelo chega pela goroutine de
	// entrega do transporte, que ficaria parada atrás do lock de uma conversa
	// que está abrindo sessão — e o protocolo inteiro pararia com ela.
	knownMu sync.Mutex
	known   map[string]sessionKnowledge
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
		onOptions:     cfg.OnSessionOptions,
		dial:          cfg.Dial,
		procs:         make(map[string]*agentProcess),
		convs:         make(map[string]*Conversation),
		known:         make(map[string]sessionKnowledge),
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
		// O transporte só conhece o nome da sessão; quem sabe de que conversa
		// ela é, e o que o app já sabia dela, é este serviço (AEP-0084 D6).
		OnConfigOptions: m.sessionOptionsChanged,
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
	dir, err := m.currentDir()
	if err != nil {
		return ""
	}
	return dir
}

// currentDir é o diretório do turno já resolvido, do mesmo jeito que ele vai
// para o agente. Guardar o caminho como veio deixaria "projeto" e "./projeto/"
// parecerem diretórios diferentes na próxima comparação.
func (m *Manager) currentDir() (string, error) {
	dir, err := m.workDir()
	if err != nil {
		return "", fmt.Errorf("diretório de trabalho do agente ACP: %w", err)
	}
	return absoluteDir(dir)
}

// sameDir diz se dois caminhos apontam para o mesmo diretório. A comparação
// literal erraria: no Windows o mesmo diretório volta com maiúsculas diferentes
// conforme quem responde — o workspace ativo, o os.Getwd, o que a pessoa
// digitou —, e tratar isso como troca de workspace faria a conversa perder a
// memória do agente sem que nada tivesse mudado.
func sameDir(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
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

	conv, err := m.entry(conversationID)
	if err != nil {
		return nil, err
	}
	if err := conv.ensure(ctx, spec); err != nil {
		return nil, err
	}
	return conv, nil
}

// entry devolve o objeto da conversa, criando-o se ainda não existir. É por ele
// que as chamadas concorrentes da mesma conversa se encontram.
func (m *Manager) entry(conversationID string) (*Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
	if m.clearing {
		return nil, ErrConversationGone
	}
	conv := m.convs[conversationID]
	if conv == nil {
		conv = &Conversation{manager: m, id: conversationID}
		m.convs[conversationID] = conv
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

	// Mesmo sem nada em memória a conversa é obtida pelo objeto: é ele que
	// serializa com um turno que esteja começando agora. Apagar direto no banco
	// levaria embora o vínculo que esse turno acabou de gravar, e a sessão dele
	// ficaria aberta no agente sem ninguém que soubesse o nome dela.
	conv, err := m.entry(conversationID)
	if err != nil {
		// Serviço encerrado: não há sessão viva para despedir, mas o registro
		// precisa sumir mesmo assim — senão a próxima execução retomaria uma
		// sessão que fala de mensagens que a pessoa apagou.
		return m.forget(ctx, conversationID)
	}

	conv.mu.Lock()
	errs := []error{conv.closeLocked(ctx), m.forget(ctx, conversationID)}
	// Marcada antes de sair do mapa: quem estava esperando neste lock segue
	// segurando este objeto e precisa descobrir que a conversa acabou.
	conv.gone = true
	conv.mu.Unlock()

	m.mu.Lock()
	if m.convs[conversationID] == conv {
		delete(m.convs, conversationID)
	}
	m.mu.Unlock()

	return errors.Join(errs...)
}

// CloseAllConversations encerra as sessões de todas as conversas e esquece os
// registros de quem pediu. É o "limpar tudo": nenhuma das conversas que essas
// sessões descrevem existe mais, e um vínculo sem conversa nunca seria
// reencontrado — ficaria no banco para sempre, com a sessão aberta no agente.
//
// Os processos continuam de pé: eles são por provider, não por conversa, e
// derrubá-los só faria a próxima mensagem esperar o agente subir de novo.
func (m *Manager) CloseAllConversations(ctx context.Context) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return m.forgetAll(ctx)
	}
	// Enquanto limpa, nenhuma conversa nova é montada: uma que nascesse agora
	// gravaria um vínculo que o apagamento logo em seguida levaria embora.
	m.clearing = true
	convs := slices.Collect(maps.Values(m.convs))
	m.mu.Unlock()

	errs := make([]error, 0, len(convs)+1)
	for _, conv := range convs {
		conv.mu.Lock()
		errs = append(errs, conv.closeLocked(ctx))
		conv.gone = true
		conv.mu.Unlock()
	}
	errs = append(errs, m.forgetAll(ctx))

	m.mu.Lock()
	m.convs = make(map[string]*Conversation)
	m.clearing = false
	m.mu.Unlock()

	return errors.Join(errs...)
}

func (m *Manager) forgetAll(ctx context.Context) error {
	if m.store == nil {
		return nil
	}
	if err := m.store.DeleteAll(ctx); err != nil {
		return fmt.Errorf("apagar registros de sessão ACP: %w", err)
	}
	return nil
}

func (m *Manager) forget(ctx context.Context, conversationID string) error {
	if m.store == nil {
		return nil
	}
	if err := m.store.Delete(ctx, conversationID); err != nil {
		return fmt.Errorf("apagar registro da sessão ACP: %w", err)
	}
	return nil
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

	// Sem processo não há sessão viva, e um vínculo sobrevivente faria o aviso
	// da próxima sessão de mesmo nome cair na conversa de antes.
	m.knownMu.Lock()
	m.known = make(map[string]sessionKnowledge)
	m.knownMu.Unlock()

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
	mu sync.Mutex
	// active é a sessão do provider em uso. mounted guarda também as dos
	// providers que a conversa usou antes: trocar de perfil no meio da conversa
	// troca de agente, e a sessão do anterior continua viva no processo dele.
	// Esquecê-la deixaria uma conversa aberta que ninguém mais fecharia.
	active  *mountedSession
	mounted map[string]*mountedSession
	// gone marca a conversa que acabou de ser limpa ou excluída. O objeto ainda
	// existe porque alguém pode estar esperando no lock; o que ele responde é
	// que chegou tarde.
	gone bool
}

// mountedSession é uma sessão montada e o que o app lembra sobre ela.
type mountedSession struct {
	proc       *agentProcess
	providerID string
	// dir é o diretório com que ela foi aberta. Guardado porque o workspace
	// ativo muda em runtime, e uma sessão de outro diretório fala de outros
	// arquivos (AEP-0084 D5).
	dir string
	// session é nula depois de Invalidate: o app deixou de usá-la, mas o nome
	// dela continua conhecido para que a despedida ainda seja possível.
	session    Session
	sessionID  string
	origin     SessionOrigin
	// originTold marca que a origem desta sessão já foi contada à pessoa. A
	// sessão recriada precisa ser anunciada uma vez, e não a cada turno: o
	// agente perdeu a memória quando ela nasceu, e repetir o aviso em todo
	// turno seguinte diria que ele a perdeu de novo.
	originTold bool
	prefixHash string
	// suffixHash resume o que o app já contou de contexto que muda — resumo da
	// conversa, memória, tasklists. Diferente do prefixo, ele fica só em
	// memória: é conteúdo que muda sozinho, e recontá-lo uma vez depois de
	// reiniciar o app custa pouco perto de uma escrita no banco por mudança.
	suffixHash string
}

func (c *Conversation) ensure(ctx context.Context, spec ProviderSpec) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.gone {
		return ErrConversationGone
	}

	proc, err := c.manager.process(spec)
	if err != nil {
		return err
	}
	dir, err := c.manager.currentDir()
	if err != nil {
		return err
	}

	// lost é a sessão invalidada que ainda pode estar de pé no agente. Se
	// nenhum registro a reencontra, ela não volta e precisa se despedir.
	var lost *mountedSession
	if current := c.mounted[spec.ID]; current != nil {
		switch {
		case current.session == nil:
			lost = current
		case current.proc == proc && sameDir(current.dir, dir):
			c.active = current
			return nil
		case current.proc == proc:
			// Mesmo agente, outro diretório: quem trocou de workspace passou a
			// falar de outros arquivos. A sessão continua viva do lado de lá, e
			// é agora que ela se despede — o registro vai apontar para outra.
			c.closeOrphan(ctx, current.session)
		}
		// Quando o processo é outro (caiu, ou a configuração mudou), a sessão
		// morreu com ele e não há despedida a fazer.
		delete(c.mounted, spec.ID)
		c.manager.forgetSession(current.sessionID)
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
	if lost != nil {
		// Quando o registro é o dela e o agente é o mesmo, o caminho da
		// retomada decide: retoma ou encerra antes de abrir outra. Fora disso
		// ninguém mais vai reencontrá-la, e a despedida é agora ou nunca.
		reencontravel := lost.proc == proc && stored != nil && stored.SessionID == lost.sessionID
		if !reencontravel {
			c.manager.abandon(ctx, lost.proc, lost.sessionID)
		}
	}

	session, origin, prefix := c.resume(ctx, proc, stored, dir)
	if session == nil {
		if origin == SessionRecreated && stored != nil {
			// A sessão registrada ficou para trás — não retomou, ou era de
			// outro diretório. Se o agente ainda a tem, é agora que ela some:
			// daqui a pouco o registro aponta para outra e ninguém mais saberia
			// que ela existiu.
			c.manager.abandon(ctx, proc, stored.SessionID)
		}
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

	mounted := &mountedSession{
		proc:       proc,
		providerID: spec.ID,
		dir:        dir,
		session:    session,
		sessionID:  session.ID(),
		origin:     origin,
		prefixHash: prefix,
	}
	if c.mounted == nil {
		c.mounted = make(map[string]*mountedSession)
	}
	c.mounted[spec.ID] = mounted
	c.active = mounted
	// O aviso de troca de modelo chega pelo transporte sabendo só o nome da
	// sessão; é este registro que o liga de volta à conversa (AEP-0084 D6).
	model, _ := currentValueOf(session.ConfigOptions(), CategoryModel)
	mode, _ := currentValueOf(session.ConfigOptions(), CategoryMode)
	c.manager.rememberSession(mounted.sessionID, sessionKnowledge{
		conversationID: c.id,
		providerID:     spec.ID,
		model:          model,
		mode:           mode,
	})
	return nil
}

// resume tenta retomar a sessão registrada. Devolve sessão nula quando não há o
// que retomar ou quando a retomada falhou, junto da origem que explica o caso.
func (c *Conversation) resume(ctx context.Context, proc *agentProcess, stored *StoredSession, dir string) (Session, SessionOrigin, string) {
	if stored == nil || strings.TrimSpace(stored.SessionID) == "" {
		return nil, SessionNew, ""
	}
	if !sameDir(stored.WorkDir, dir) {
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

// closeLocked encerra todas as sessões desta conversa, e não só a do provider
// em uso: a conversa pode ter passado por mais de um agente, e cada um ainda
// guarda a sua. Roda com o lock da conversa segurado — quem apaga precisa que
// nenhum turno monte sessão nova no meio da despedida.
func (c *Conversation) closeLocked(ctx context.Context) error {
	mounted := make([]*mountedSession, 0, len(c.mounted))
	for _, entry := range c.mounted {
		mounted = append(mounted, entry)
	}
	c.mounted = nil
	c.active = nil

	var errs []error
	for _, entry := range mounted {
		// A conversa acabou: um aviso do agente sobre esta sessão já não tem a
		// quem chegar, e deixar o vínculo faria o evento apontar para uma
		// conversa que a pessoa apagou.
		c.manager.forgetSession(entry.sessionID)
		if entry.session == nil {
			// Sessão invalidada: o app não a usa mais, mas o agente pode ainda
			// tê-la. Sem esta despedida ela ficaria aberta no processo do
			// provider — que é compartilhado — sem registro que a nomeasse.
			c.manager.abandon(ctx, entry.proc, entry.sessionID)
			continue
		}
		if err := entry.session.Close(ctx); err != nil && !errors.Is(err, ErrSessionClosed) && !errors.Is(err, ErrSessionLost) {
			// A despedida falhou e o registro está prestes a sumir junto com a
			// conversa: esta é a última vez que alguém sabe o nome da sessão.
			// Segunda tentativa pelo nome, que ainda pega o agente de pé quando
			// o que falhou foi só a chamada.
			c.manager.abandon(ctx, entry.proc, entry.sessionID)
			errs = append(errs, fmt.Errorf("encerrar sessão ACP da conversa %s no provider %q: %w", c.id, entry.providerID, err))
		}
	}
	return errors.Join(errs...)
}

// Session é a sessão viva desta conversa.
func (c *Conversation) Session() Session {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active == nil {
		return nil
	}
	return c.active.session
}

// Origin diz como a sessão atual foi obtida. Vale a leitura antes do primeiro
// turno: sessão recriada é agente sem a memória anterior.
func (c *Conversation) Origin() SessionOrigin {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active == nil {
		return SessionNew
	}
	return c.active.origin
}

// TakeLostMemoryNotice diz se ainda falta contar que o agente perdeu o contexto
// anterior desta conversa, e marca o aviso como dado (AEP-0084 D4).
//
// Reabrir uma conversa tenta retomar a sessão do agente; quando ela não volta —
// o agente não sabe retomar, o identificador guardado não vale mais, o diretório
// mudou —, a conversa segue com um agente que não viveu o que está na tela. Sem
// dizer isso, a pessoa descobre pela resposta estranha.
//
// O aviso é consumido no turno que o entrega, e não na montagem da sessão: turno
// que nem chegou ao agente não contou nada a ninguém, e perder o aviso aqui
// seria perdê-lo para sempre.
func (c *Conversation) TakeLostMemoryNotice() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active == nil || c.active.originTold || !c.active.origin.LostMemory() {
		return false
	}
	c.active.originTold = true
	return true
}

// Invalidate esquece a sessão em memória sem apagar o registro. É o que fazer
// quando o turno volta com ErrSessionLost: o próximo uso tenta retomar pelo
// identificador guardado e, se o agente não retomar, abre outra avisando.
//
// O nome da sessão fica: o processo do provider é compartilhado e pode ter
// sobrevivido ao que derrubou a sessão. Excluir a conversa depois disso ainda
// precisa conseguir despedir-se dela, senão ela fica aberta lá sem registro que
// a nomeie.
func (c *Conversation) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active == nil {
		return
	}
	c.active.session = nil
	c.active = nil
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
	if c.active == nil {
		return true
	}
	return c.active.prefixHash != hash
}

// MarkPrefixSent registra que a sessão já ouviu este prefixo. Persiste junto do
// identificador da sessão porque uma sessão retomada depois de reiniciar o app
// já ouviu a persona, e repetir tudo seria desperdício.
func (c *Conversation) MarkPrefixSent(ctx context.Context, hash string) error {
	c.mu.Lock()
	active := c.active
	if active == nil {
		c.mu.Unlock()
		return errors.New("conversa sem sessão ACP para anotar o prefixo")
	}
	if active.prefixHash == hash {
		c.mu.Unlock()
		return nil
	}
	active.prefixHash = hash
	providerID := active.providerID
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

// Capabilities diz o que o agente desta conversa sabe receber — imagem, áudio,
// contexto embutido. Vem do initialize e já está em memória: quem monta o turno
// precisa disso antes de mandar um anexo que o agente não aceita.
func (c *Conversation) Capabilities(ctx context.Context) (Capabilities, error) {
	c.mu.Lock()
	active := c.active
	c.mu.Unlock()
	if active == nil {
		return Capabilities{}, errors.New("conversa sem sessão ACP")
	}
	return active.proc.client.Capabilities(ctx)
}

// NeedsSuffix diz se o contexto que muda ainda precisa ser contado a esta
// sessão neste turno (AEP-0084 D4). Reenviá-lo sem ter mudado gastaria contexto
// do agente repetindo o que ele acabou de ouvir.
func (c *Conversation) NeedsSuffix(hash string) bool {
	if strings.TrimSpace(hash) == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active == nil {
		return true
	}
	return c.active.suffixHash != hash
}

// MarkSuffixSent registra que a sessão já ouviu este contexto.
func (c *Conversation) MarkSuffixSent(hash string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active == nil {
		return
	}
	c.active.suffixHash = hash
}

// abandon se despede de uma sessão registrada que o app não vai mais usar. É
// tentativa: o agente pode não saber encerrar sessões, e a sessão pode já ter
// morrido — o que não pode é o app trocar o registro sem nem tentar, deixando
// no agente uma conversa que ninguém mais consegue nomear.
func (m *Manager) abandon(ctx context.Context, proc *agentProcess, sessionID string) {
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	detached, cancel := context.WithTimeout(context.WithoutCancel(ctx), storeTimeout)
	defer cancel()
	if err := proc.client.CloseSession(detached, sessionID); err != nil {
		logging.Debugf(detached, managerComponent,
			"[ACP] sessão %q não pôde ser encerrada ao ser substituída: %v", sessionID, err)
	}
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
