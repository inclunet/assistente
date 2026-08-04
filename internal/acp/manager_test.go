package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeManagedClient é um agente de mentira: registra o que o manager pediu sem
// subir processo nenhum.
type fakeManagedClient struct {
	mu sync.Mutex

	caps Capabilities

	newErr          error
	loadErr         error
	closeSessionErr error

	newCalls   int
	loadCalls  int
	loadedIDs  []string
	closedByID []string
	closed     bool

	// options é o que a descoberta responde, optionCalls conta quantas vezes o
	// agente foi consultado de fato e invalidations quantas vezes o cache foi
	// descartado.
	options       []ConfigOption
	optionCalls   int
	invalidations int

	// sessionOptions é o estado com que cada sessão nova nasce, como o agente
	// devolve no session/new.
	sessionOptions []ConfigOption

	sessions []*fakeManagedSession
	nextID   int
}

func newFakeManagedClient() *fakeManagedClient {
	return &fakeManagedClient{caps: Capabilities{AgentName: "falso", LoadSession: true}}
}

func (c *fakeManagedClient) NewSession(_ context.Context, cwd string) (Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.newCalls++
	if c.newErr != nil {
		return nil, c.newErr
	}
	c.nextID++
	sess := &fakeManagedSession{
		id:      fmt.Sprintf("sess-%d", c.nextID),
		cwd:     cwd,
		options: copyOptions(c.sessionOptions),
	}
	c.sessions = append(c.sessions, sess)
	return sess, nil
}

func (c *fakeManagedClient) LoadSession(_ context.Context, sessionID, cwd string) (Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loadCalls++
	c.loadedIDs = append(c.loadedIDs, sessionID)
	if c.loadErr != nil {
		return nil, c.loadErr
	}
	sess := &fakeManagedSession{id: sessionID, cwd: cwd, options: copyOptions(c.sessionOptions)}
	c.sessions = append(c.sessions, sess)
	return sess, nil
}

func (c *fakeManagedClient) CloseSession(_ context.Context, sessionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closedByID = append(c.closedByID, sessionID)
	return c.closeSessionErr
}

func (c *fakeManagedClient) Capabilities(context.Context) (Capabilities, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.caps, nil
}

func (c *fakeManagedClient) Options(context.Context, string) ([]ConfigOption, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.optionCalls++
	return copyOptions(c.options), nil
}

func (c *fakeManagedClient) InvalidateOptions() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.invalidations++
}

func (c *fakeManagedClient) discovery() (calls, invalidations int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.optionCalls, c.invalidations
}

func (c *fakeManagedClient) Call(context.Context, string, any) (json.RawMessage, error) {
	return nil, errors.New("não usado no teste")
}

func (c *fakeManagedClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *fakeManagedClient) counters() (newCalls, loadCalls int, closed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.newCalls, c.loadCalls, c.closed
}

type fakeManagedSession struct {
	id  string
	cwd string

	mu       sync.Mutex
	closed   bool
	closeErr error

	options  []ConfigOption
	setErr   error
	setCalls []setOptionCall
	// duringSet roda no meio da troca, entre o pedido e a resposta. É o agente
	// contando a mudança por notificação enquanto ainda responde a ela: a
	// entrega vem por outra goroutine, e nada ordena as duas.
	duringSet func(id, value string)
	// setApplied é o valor que o agente aplica de verdade, quando ele acomoda o
	// pedido em outro. Vazio significa que ele aplica o que foi pedido.
	setApplied string
}

type setOptionCall struct {
	id    string
	value string
}

func (s *fakeManagedSession) ID() string { return s.id }

func (s *fakeManagedSession) Prompt(context.Context, []Content, UpdateSink) (StopReason, error) {
	return StopEndTurn, nil
}

func (s *fakeManagedSession) Close(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closeErr != nil {
		return s.closeErr
	}
	s.closed = true
	return nil
}

func (s *fakeManagedSession) Cancel(context.Context) error { return nil }

func (s *fakeManagedSession) ConfigOptions() []ConfigOption {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyOptions(s.options)
}

// SetConfigOption troca o valor corrente como um agente de verdade faria, e
// devolve o conjunto resultante. Guardar a troca importa: o teste da troca vinda
// do agente precisa distinguir o que o app pediu do que o agente decidiu.
func (s *fakeManagedSession) SetConfigOption(_ context.Context, id, value string) ([]ConfigOption, error) {
	s.mu.Lock()
	s.setCalls = append(s.setCalls, setOptionCall{id: id, value: value})
	during := s.duringSet
	s.mu.Unlock()

	if during != nil {
		during(id, value)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setErr != nil {
		return nil, s.setErr
	}
	applied := value
	if s.setApplied != "" {
		applied = s.setApplied
	}
	for i := range s.options {
		if s.options[i].ID == id {
			s.options[i].CurrentValue = applied
		}
	}
	return copyOptions(s.options), nil
}

func (s *fakeManagedSession) optionSets() []setOptionCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]setOptionCall(nil), s.setCalls...)
}

func (s *fakeManagedSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// memoryStore é o registro de sessões em memória, com ganchos para simular
// banco indisponível.
type memoryStore struct {
	mu      sync.Mutex
	rows    map[string]StoredSession
	loadErr error
	saveErr error

	// deleteHold segura o apagamento até o teste soltar, e deleteStarted avisa
	// quando ele já começou. É o que permite pôr um turno exatamente dentro da
	// janela da exclusão.
	deleteHold    chan struct{}
	deleteStarted chan struct{}
}

func newMemoryStore() *memoryStore {
	return &memoryStore{rows: make(map[string]StoredSession)}
}

func storeKey(conversationID, providerID string) string {
	return conversationID + "\x00" + providerID
}

func (s *memoryStore) Load(_ context.Context, conversationID, providerID string) (*StoredSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	rec, ok := s.rows[storeKey(conversationID, providerID)]
	if !ok {
		return nil, nil
	}
	return &rec, nil
}

func (s *memoryStore) Save(_ context.Context, rec StoredSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	s.rows[storeKey(rec.ConversationID, rec.ProviderID)] = rec
	return nil
}

func (s *memoryStore) SavePrefixHash(_ context.Context, conversationID, providerID, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := storeKey(conversationID, providerID)
	rec, ok := s.rows[key]
	if !ok {
		return errors.New("sessão não registrada")
	}
	rec.PrefixHash = hash
	s.rows[key] = rec
	return nil
}

func (s *memoryStore) blockDelete() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteHold = make(chan struct{})
	s.deleteStarted = make(chan struct{})
}

func (s *memoryStore) waitDeleteStarted(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	started := s.deleteStarted
	s.mu.Unlock()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("o apagamento do registro não começou")
	}
}

func (s *memoryStore) releaseDelete() {
	s.mu.Lock()
	hold := s.deleteHold
	s.deleteHold = nil
	s.mu.Unlock()
	if hold != nil {
		close(hold)
	}
}

// holdDelete segura o apagamento no ponto em que o teste pediu, avisando que
// ele começou.
func (s *memoryStore) holdDelete() {
	s.mu.Lock()
	hold, started := s.deleteHold, s.deleteStarted
	s.mu.Unlock()
	if hold == nil {
		return
	}
	close(started)
	<-hold
}

func (s *memoryStore) Delete(_ context.Context, conversationID string) error {
	s.holdDelete()

	s.mu.Lock()
	defer s.mu.Unlock()
	for key, rec := range s.rows {
		if rec.ConversationID == conversationID {
			delete(s.rows, key)
		}
	}
	return nil
}

func (s *memoryStore) DeleteAll(_ context.Context) error {
	s.holdDelete()
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.rows)
	return nil
}

func (s *memoryStore) get(conversationID, providerID string) (StoredSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.rows[storeKey(conversationID, providerID)]
	return rec, ok
}

func (s *memoryStore) size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.rows)
}

func testSpec() ProviderSpec {
	return ProviderSpec{ID: "cursor", Name: "Cursor", Command: "cursor-agent", Args: []string{"acp"}}
}

// dirDeTeste devolve um diretório absoluto no formato do sistema. O manager
// resolve o caminho antes de abrir a sessão, então um caminho cru de barra só
// bateria com o resolvido fora do Windows.
func dirDeTeste(nome string) string {
	dir, err := filepath.Abs(filepath.FromSlash("/" + nome))
	if err != nil {
		return filepath.FromSlash("/" + nome)
	}
	return dir
}

// managerWith monta um manager sobre um agente de mentira, contando quantas
// vezes o transporte foi criado.
func managerWith(store SessionStore, client *fakeManagedClient) (*Manager, *int) {
	dials := 0
	m := NewManager(ManagerConfig{
		Store:   store,
		WorkDir: func() (string, error) { return dirDeTeste("projeto"), nil },
		Dial: func(Config, RequestHandler) (Client, error) {
			dials++
			return client, nil
		},
	})
	return m, &dials
}

func TestConversasDiferentesCompartilhamOProcessoDoProvider(t *testing.T) {
	client := newFakeManagedClient()
	m, dials := managerWith(newMemoryStore(), client)
	ctx := context.Background()

	primeira, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("primeira conversa: %v", err)
	}
	segunda, err := m.Conversation(ctx, testSpec(), "conv-2")
	if err != nil {
		t.Fatalf("segunda conversa: %v", err)
	}

	if *dials != 1 {
		t.Fatalf("processos criados = %d, esperado 1 por provider", *dials)
	}
	if primeira.Session().ID() == segunda.Session().ID() {
		t.Fatal("conversas diferentes receberam a mesma sessão")
	}
	if novas, _, _ := client.counters(); novas != 2 {
		t.Fatalf("session/new chamado %d vezes, esperado 2", novas)
	}
}

func TestAMesmaConversaNaoAbreDuasSessoesEmParalelo(t *testing.T) {
	client := newFakeManagedClient()
	m, _ := managerWith(newMemoryStore(), client)
	ctx := context.Background()

	const goroutines = 8
	ids := make([]string, goroutines)
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conv, err := m.Conversation(ctx, testSpec(), "conv-1")
			if err != nil {
				t.Errorf("conversa: %v", err)
				return
			}
			ids[i] = conv.Session().ID()
		}()
	}
	wg.Wait()

	if novas, _, _ := client.counters(); novas != 1 {
		t.Fatalf("session/new chamado %d vezes; o barge-in precisa achar a mesma sessão", novas)
	}
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("sessões divergentes entre goroutines: %v", ids)
		}
	}
}

func TestConversaRetomaASessaoRegistradaDepoisDeReiniciar(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()

	primeiro, _ := managerWith(store, newFakeManagedClient())
	conv, err := primeiro.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("primeira sessão: %v", err)
	}
	if conv.Origin() != SessionNew {
		t.Fatalf("origem = %v, esperado nova", conv.Origin())
	}
	if err := conv.MarkPrefixSent(ctx, "hash-persona"); err != nil {
		t.Fatalf("marcar prefixo: %v", err)
	}
	sessionID := conv.Session().ID()
	primeiro.Shutdown()

	clienteNovo := newFakeManagedClient()
	segundo, _ := managerWith(store, clienteNovo)
	retomada, err := segundo.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("retomar sessão: %v", err)
	}

	if retomada.Origin() != SessionResumed {
		t.Fatalf("origem = %v, esperado retomada", retomada.Origin())
	}
	if got := retomada.Session().ID(); got != sessionID {
		t.Fatalf("sessão retomada = %q, esperado %q", got, sessionID)
	}
	if novas, cargas, _ := clienteNovo.counters(); novas != 0 || cargas != 1 {
		t.Fatalf("session/new=%d session/load=%d; a conversa devia ser retomada", novas, cargas)
	}
	if retomada.NeedsPrefix("hash-persona") {
		t.Fatal("prefixo já entregue foi cobrado de novo numa sessão que se lembra dele")
	}
	if !retomada.NeedsPrefix("hash-outro-perfil") {
		t.Fatal("troca de perfil precisa reenviar o prefixo")
	}
}

func TestContextoQueMudaSoEhCobradoQuandoMuda(t *testing.T) {
	ctx := context.Background()
	m, _ := managerWith(newMemoryStore(), newFakeManagedClient())
	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}

	if !conv.NeedsSuffix("hash-resumo") {
		t.Fatal("sessão nova ainda não ouviu contexto nenhum")
	}
	conv.MarkSuffixSent("hash-resumo")
	if conv.NeedsSuffix("hash-resumo") {
		t.Fatal("contexto inalterado foi cobrado de novo")
	}
	if !conv.NeedsSuffix("hash-outro-resumo") {
		t.Fatal("contexto que mudou precisa voltar ao agente")
	}
}

func TestSessaoRecriadaPrecisaOuvirOContextoDeNovo(t *testing.T) {
	ctx := context.Background()
	client := newFakeManagedClient()
	m, _ := managerWith(newMemoryStore(), client)
	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	conv.MarkSuffixSent("hash-resumo")

	// Sessão perdida no meio da conversa: a próxima é outra sessão, e o que a
	// anterior ouviu morreu com ela.
	conv.Invalidate()
	client.loadErr = errors.New("sessão desconhecida")
	if _, err := m.Conversation(ctx, testSpec(), "conv-1"); err != nil {
		t.Fatalf("remontar conversa: %v", err)
	}

	if !conv.NeedsSuffix("hash-resumo") {
		t.Fatal("a sessão nova não ouviu o contexto que a anterior recebeu")
	}
}

func TestAgenteQueNaoRetomaDeixaClaroQueAMemoriaSePerdeu(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()
	if err := store.Save(ctx, StoredSession{
		ConversationID: "conv-1",
		ProviderID:     "cursor",
		SessionID:      "sess-antiga",
		PrefixHash:     "hash-persona",
		WorkDir:        dirDeTeste("projeto"),
	}); err != nil {
		t.Fatalf("preparar registro: %v", err)
	}

	client := newFakeManagedClient()
	client.loadErr = errors.New("sessão desconhecida")
	m, _ := managerWith(store, client)

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	if conv.Origin() != SessionRecreated || !conv.Origin().LostMemory() {
		t.Fatalf("origem = %v, esperado recriada", conv.Origin())
	}
	if !conv.NeedsPrefix("hash-persona") {
		t.Fatal("sessão nova é agente sem memória: o prefixo precisa ser dito de novo")
	}
	rec, ok := store.get("conv-1", "cursor")
	if !ok || rec.SessionID == "sess-antiga" {
		t.Fatalf("registro não foi trocado pela sessão nova: %+v", rec)
	}
	if rec.PrefixHash != "" {
		t.Fatalf("prefixo da sessão morta sobreviveu: %q", rec.PrefixHash)
	}
}

func TestSessaoAbertaEmOutroDiretorioNaoEhRetomada(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()
	if err := store.Save(ctx, StoredSession{
		ConversationID: "conv-1",
		ProviderID:     "cursor",
		SessionID:      "sess-antiga",
		WorkDir:        dirDeTeste("outro-projeto"),
	}); err != nil {
		t.Fatalf("preparar registro: %v", err)
	}

	client := newFakeManagedClient()
	m, _ := managerWith(store, client)

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	if _, cargas, _ := client.counters(); cargas != 0 {
		t.Fatalf("session/load chamado %d vezes para outro diretório", cargas)
	}
	if conv.Origin() != SessionRecreated {
		t.Fatalf("origem = %v, esperado recriada", conv.Origin())
	}
}

func TestTrocarOComandoDoProviderDerrubaOAgenteAntigo(t *testing.T) {
	ctx := context.Background()
	antigo := newFakeManagedClient()
	novo := newFakeManagedClient()
	clientes := []*fakeManagedClient{antigo, novo}

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

	if _, err := m.Conversation(ctx, testSpec(), "conv-1"); err != nil {
		t.Fatalf("primeira conversa: %v", err)
	}

	trocado := testSpec()
	trocado.Command = "outro-agente"
	conv, err := m.Conversation(ctx, trocado, "conv-1")
	if err != nil {
		t.Fatalf("conversa após troca: %v", err)
	}

	if dials != 2 {
		t.Fatalf("processos criados = %d, esperado 2 após a troca de comando", dials)
	}
	// A conversa passou a falar com o agente novo — por retomada ou por sessão
	// nova, o que importa é que a sessão não continuou no processo antigo.
	novas, cargas, _ := novo.counters()
	if novas+cargas != 1 {
		t.Fatalf("o agente novo montou %d sessões (novas=%d, retomadas=%d), esperado 1", novas+cargas, novas, cargas)
	}
	if conv.Session().(*fakeManagedSession).cwd != dirDeTeste("projeto") {
		t.Fatal("a sessão não usou o diretório de trabalho do app")
	}
	// O fechamento do processo antigo é assíncrono para não segurar quem pediu
	// o novo, mas precisa acontecer: senão fica um agente órfão por edição.
	esperaFechar(t, antigo)
}

// esperaFechar aguarda o encerramento assíncrono do processo antigo.
func esperaFechar(t *testing.T, client *fakeManagedClient) {
	t.Helper()
	prazo := time.Now().Add(2 * time.Second)
	for time.Now().Before(prazo) {
		if _, _, fechado := client.counters(); fechado {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("o processo do agente antigo continuou de pé depois da troca de configuração")
}

func TestLimparConversaEncerraASessaoEApagaORegistro(t *testing.T) {
	store := newMemoryStore()
	client := newFakeManagedClient()
	m, _ := managerWith(store, client)
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	if _, err := m.Conversation(ctx, testSpec(), "conv-2"); err != nil {
		t.Fatalf("outra conversa: %v", err)
	}
	sessao := conv.Session().(*fakeManagedSession)

	if err := m.CloseConversation(ctx, "conv-1"); err != nil {
		t.Fatalf("encerrar conversa: %v", err)
	}
	if !sessao.isClosed() {
		t.Fatal("a sessão continuou aberta no agente depois de limpar a conversa")
	}
	if _, ok := store.get("conv-1", "cursor"); ok {
		t.Fatal("o registro da conversa apagada sobreviveu")
	}
	if _, ok := store.get("conv-2", "cursor"); !ok {
		t.Fatal("limpar uma conversa levou junto a sessão da outra")
	}
}

func TestShutdownDerrubaOsProcessosMasGuardaOsRegistros(t *testing.T) {
	store := newMemoryStore()
	client := newFakeManagedClient()
	m, _ := managerWith(store, client)
	ctx := context.Background()

	if _, err := m.Conversation(ctx, testSpec(), "conv-1"); err != nil {
		t.Fatalf("conversa: %v", err)
	}
	m.Shutdown()

	if _, _, fechado := client.counters(); !fechado {
		t.Fatal("o processo do agente não foi encerrado no shutdown")
	}
	if store.size() != 1 {
		t.Fatal("o vínculo conversa↔sessão foi apagado no shutdown e a conversa perderia a memória na volta")
	}
	if _, err := m.Conversation(ctx, testSpec(), "conv-2"); !errors.Is(err, ErrClosed) {
		t.Fatalf("erro após shutdown = %v, esperado ErrClosed", err)
	}
}

func TestTrocarDeAgenteNoMeioDaConversaNaoAbandonaASessaoAnterior(t *testing.T) {
	ctx := context.Background()
	cursor := newFakeManagedClient()
	claude := newFakeManagedClient()
	porComando := map[string]*fakeManagedClient{
		"cursor-agent": cursor,
		"claude-code":  claude,
	}

	store := newMemoryStore()
	m := NewManager(ManagerConfig{
		Store:   store,
		WorkDir: func() (string, error) { return dirDeTeste("projeto"), nil },
		Dial: func(cfg Config, _ RequestHandler) (Client, error) {
			return porComando[cfg.Command], nil
		},
	})

	primeiro := testSpec()
	conv, err := m.Conversation(ctx, primeiro, "conv-1")
	if err != nil {
		t.Fatalf("conversa com o primeiro agente: %v", err)
	}
	sessaoAntiga := conv.Session().(*fakeManagedSession)

	segundo := ProviderSpec{ID: "claude", Name: "Claude Code", Command: "claude-code"}
	conv, err = m.Conversation(ctx, segundo, "conv-1")
	if err != nil {
		t.Fatalf("conversa com o segundo agente: %v", err)
	}
	sessaoNova := conv.Session().(*fakeManagedSession)
	if sessaoAntiga == sessaoNova {
		t.Fatal("trocar de provider reaproveitou a sessão do agente anterior")
	}
	if sessaoAntiga.isClosed() {
		// Trocar de perfil não apaga a memória: voltar ao agente anterior deve
		// reencontrar a conversa dele.
		t.Fatal("a sessão do agente anterior foi encerrada só por troca de provider")
	}

	if err := m.CloseConversation(ctx, "conv-1"); err != nil {
		t.Fatalf("encerrar conversa: %v", err)
	}
	if !sessaoAntiga.isClosed() {
		t.Fatal("a sessão do agente anterior ficou aberta depois de excluir a conversa")
	}
	if !sessaoNova.isClosed() {
		t.Fatal("a sessão do agente em uso ficou aberta depois de excluir a conversa")
	}
	if store.size() != 0 {
		t.Fatalf("registros que sobraram = %d, esperado nenhum", store.size())
	}
}

func TestTrocarDeWorkspaceRecriaASessaoNoDiretorioNovo(t *testing.T) {
	store := newMemoryStore()
	client := newFakeManagedClient()
	ctx := context.Background()

	diretorio := dirDeTeste("projeto-a")
	m := NewManager(ManagerConfig{
		Store:   store,
		WorkDir: func() (string, error) { return diretorio, nil },
		Dial: func(Config, RequestHandler) (Client, error) {
			return client, nil
		},
	})

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa no primeiro workspace: %v", err)
	}
	sessaoA := conv.Session().(*fakeManagedSession)

	diretorio = dirDeTeste("projeto-b")
	conv, err = m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa no segundo workspace: %v", err)
	}
	sessaoB := conv.Session().(*fakeManagedSession)

	if sessaoA == sessaoB {
		t.Fatal("a conversa continuou na sessão aberta no workspace anterior")
	}
	if sessaoB.cwd != dirDeTeste("projeto-b") {
		t.Fatalf("sessão nova abriu em %q", sessaoB.cwd)
	}
	if !sessaoA.isClosed() {
		t.Fatal("a sessão do workspace anterior ficou aberta no agente")
	}
	if conv.Origin() != SessionRecreated {
		t.Fatalf("origem = %v, esperado recriada: o agente não tem a memória da conversa anterior", conv.Origin())
	}
}

func TestOMesmoDiretorioEscritoDeOutroJeitoRetomaASessao(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()

	registrado := dirDeTeste("projeto")
	if err := store.Save(ctx, StoredSession{
		ConversationID: "conv-1",
		ProviderID:     "cursor",
		SessionID:      "sess-antiga",
		WorkDir:        registrado,
	}); err != nil {
		t.Fatalf("preparar registro: %v", err)
	}

	// O mesmo diretório como outra fonte poderia devolvê-lo: barra ao final e,
	// no Windows, caixa diferente. Nada disso é troca de workspace.
	informado := registrado + string(filepath.Separator)
	if runtime.GOOS == "windows" {
		informado = strings.ToUpper(informado)
	}

	client := newFakeManagedClient()
	m := NewManager(ManagerConfig{
		Store:   store,
		WorkDir: func() (string, error) { return informado, nil },
		Dial: func(Config, RequestHandler) (Client, error) {
			return client, nil
		},
	})

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	if conv.Origin() != SessionResumed {
		t.Fatalf("origem = %v; %q e %q são o mesmo diretório e a memória do agente se perdeu à toa",
			conv.Origin(), registrado, informado)
	}
}

func TestTurnoQueChegaDuranteAExclusaoNaoRessuscitaAConversa(t *testing.T) {
	store := newMemoryStore()
	client := newFakeManagedClient()
	m, _ := managerWith(store, client)
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("primeira conversa: %v", err)
	}
	// O turno já tem o objeto da conversa em mãos quando a exclusão começa —
	// é essa a janela. A exclusão segura o lock da conversa enquanto apaga, e
	// depois dela o turno precisa descobrir que chegou tarde: montar sessão
	// nova aqui gravaria um vínculo de uma conversa que não existe mais.
	store.blockDelete()

	fim := make(chan error, 1)
	go func() { fim <- m.CloseConversation(ctx, "conv-1") }()
	store.waitDeleteStarted(t)

	tarde := make(chan error, 1)
	go func() { tarde <- conv.ensure(ctx, testSpec()) }()

	store.releaseDelete()
	if err := <-fim; err != nil {
		t.Fatalf("encerrar conversa: %v", err)
	}
	if err := <-tarde; !errors.Is(err, ErrConversationGone) {
		t.Fatalf("turno atrasado devolveu %v, esperado conversa encerrada", err)
	}
	if store.size() != 0 {
		t.Fatalf("registros que sobraram = %d; o turno atrasado regravou o vínculo da conversa apagada", store.size())
	}

	// Depois que a exclusão termina, a conversa volta a ser utilizável: o app
	// recicla conversas vazias em vez de criar outra.
	if _, err := m.Conversation(ctx, testSpec(), "conv-1"); err != nil {
		t.Fatalf("conversa depois da exclusão: %v", err)
	}
}

func TestLimparTudoEncerraAsSessoesDeTodasAsConversas(t *testing.T) {
	store := newMemoryStore()
	client := newFakeManagedClient()
	m, dials := managerWith(store, client)
	ctx := context.Background()

	primeira, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("primeira conversa: %v", err)
	}
	segunda, err := m.Conversation(ctx, testSpec(), "conv-2")
	if err != nil {
		t.Fatalf("segunda conversa: %v", err)
	}
	sessaoUm := primeira.Session().(*fakeManagedSession)
	sessaoDois := segunda.Session().(*fakeManagedSession)

	if err := m.CloseAllConversations(ctx); err != nil {
		t.Fatalf("limpar tudo: %v", err)
	}

	if !sessaoUm.isClosed() || !sessaoDois.isClosed() {
		t.Fatal("sessões continuaram abertas depois de apagar todas as conversas")
	}
	if store.size() != 0 {
		t.Fatalf("registros que sobraram = %d, esperado nenhum", store.size())
	}
	// O processo é por provider, não por conversa: derrubá-lo só faria a
	// próxima mensagem esperar o agente subir de novo.
	if _, _, fechado := client.counters(); fechado {
		t.Fatal("o agente foi derrubado por uma limpeza de conversas")
	}

	nova, err := m.Conversation(ctx, testSpec(), "conv-3")
	if err != nil {
		t.Fatalf("conversa depois da limpeza: %v", err)
	}
	if nova.Session() == nil {
		t.Fatal("a conversa criada depois da limpeza ficou sem sessão")
	}
	if *dials != 1 {
		t.Fatalf("processos criados = %d, esperado 1 para o provider", *dials)
	}
}

func TestTurnoQueChegaDuranteALimpezaGeralNaoDeixaVinculo(t *testing.T) {
	store := newMemoryStore()
	client := newFakeManagedClient()
	m, _ := managerWith(store, client)
	ctx := context.Background()

	if _, err := m.Conversation(ctx, testSpec(), "conv-1"); err != nil {
		t.Fatalf("primeira conversa: %v", err)
	}
	store.blockDelete()

	fim := make(chan error, 1)
	go func() { fim <- m.CloseAllConversations(ctx) }()
	store.waitDeleteStarted(t)

	// Conversa que nem existia quando a limpeza começou: montá-la agora
	// gravaria um vínculo que o apagamento em curso levaria embora.
	if _, err := m.Conversation(ctx, testSpec(), "conv-nova"); !errors.Is(err, ErrConversationGone) {
		t.Fatalf("conversa montada no meio da limpeza devolveu %v, esperado conversa encerrada", err)
	}

	store.releaseDelete()
	if err := <-fim; err != nil {
		t.Fatalf("limpar tudo: %v", err)
	}
	if store.size() != 0 {
		t.Fatalf("registros que sobraram = %d; a limpeza deixou vínculo sem conversa", store.size())
	}
}

func TestSessaoInvalidadaAindaSeDespedeQuandoAConversaEhExcluida(t *testing.T) {
	store := newMemoryStore()
	client := newFakeManagedClient()
	m, _ := managerWith(store, client)
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	perdida := conv.Session().ID()

	// O turno voltou com a sessão perdida: o app para de usá-la, mas o
	// processo do provider é compartilhado e pode ter sobrevivido.
	conv.Invalidate()
	if err := m.CloseConversation(ctx, "conv-1"); err != nil {
		t.Fatalf("excluir conversa: %v", err)
	}

	client.mu.Lock()
	despedidas := append([]string(nil), client.closedByID...)
	client.mu.Unlock()
	if len(despedidas) != 1 || despedidas[0] != perdida {
		t.Fatalf("despedidas = %v; a sessão invalidada ficou aberta no agente sem registro que a nomeasse", despedidas)
	}
}

func TestDespedidaQueFalhaTentaDeNovoPeloNomeDaSessao(t *testing.T) {
	store := newMemoryStore()
	client := newFakeManagedClient()
	m, _ := managerWith(store, client)
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	sessao := conv.Session().(*fakeManagedSession)
	sessao.mu.Lock()
	sessao.closeErr = errors.New("agente ocupado")
	sessao.mu.Unlock()

	// O registro some com a conversa: esta é a última vez que alguém sabe o
	// nome da sessão, e o agente pode continuar de pé.
	if err := m.CloseConversation(ctx, "conv-1"); err == nil {
		t.Fatal("a falha ao encerrar a sessão foi engolida")
	}

	client.mu.Lock()
	despedidas := append([]string(nil), client.closedByID...)
	client.mu.Unlock()
	if len(despedidas) != 1 || despedidas[0] != sessao.id {
		t.Fatalf("despedidas = %v; faltou a segunda tentativa pelo nome da sessão", despedidas)
	}
}

func TestDepoisDeInvalidarOProximoTurnoTentaRetomarAMesmaSessao(t *testing.T) {
	store := newMemoryStore()
	client := newFakeManagedClient()
	m, _ := managerWith(store, client)
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	registrada := conv.Session().ID()
	conv.Invalidate()

	if _, err := m.Conversation(ctx, testSpec(), "conv-1"); err != nil {
		t.Fatalf("conversa depois de invalidar: %v", err)
	}

	client.mu.Lock()
	retomadas := append([]string(nil), client.loadedIDs...)
	despedidas := append([]string(nil), client.closedByID...)
	client.mu.Unlock()
	if len(retomadas) != 1 || retomadas[0] != registrada {
		t.Fatalf("retomadas = %v, esperado a sessão registrada %q", retomadas, registrada)
	}
	// Encerrar a sessão que acabou de ser retomada seria jogar fora a memória
	// que se tentou recuperar.
	if len(despedidas) != 0 {
		t.Fatalf("despedidas = %v; a sessão retomada foi encerrada", despedidas)
	}
	if conv.Origin() != SessionResumed {
		t.Fatalf("origem = %v, esperado retomada", conv.Origin())
	}
}

func TestSessaoQueNaoVoltouEhEncerradaAntesDeSerSubstituida(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()
	if err := store.Save(ctx, StoredSession{
		ConversationID: "conv-1",
		ProviderID:     "cursor",
		SessionID:      "sess-antiga",
		WorkDir:        dirDeTeste("outro-projeto"),
	}); err != nil {
		t.Fatalf("preparar registro: %v", err)
	}

	client := newFakeManagedClient()
	m, _ := managerWith(store, client)

	if _, err := m.Conversation(ctx, testSpec(), "conv-1"); err != nil {
		t.Fatalf("conversa: %v", err)
	}

	client.mu.Lock()
	despedidas := append([]string(nil), client.closedByID...)
	client.mu.Unlock()
	if len(despedidas) != 1 || despedidas[0] != "sess-antiga" {
		t.Fatalf("despedidas = %v; a sessão substituída precisa ser encerrada no agente", despedidas)
	}
}

func TestTrocaDeUsuarioNaoHerdaOProcessoNemASessao(t *testing.T) {
	ctx := context.Background()
	daAna := newFakeManagedClient()
	doLeo := newFakeManagedClient()
	clientes := []*fakeManagedClient{daAna, doLeo}

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

	if _, err := m.Conversation(ctx, testSpec(), "conv-da-ana"); err != nil {
		t.Fatalf("conversa da ana: %v", err)
	}
	m.DisconnectAll()

	if _, _, fechado := daAna.counters(); !fechado {
		t.Fatal("o agente da pessoa que saiu continuou de pé")
	}
	// Mesmo id de provider, outra pessoa: precisa ser outro processo, senão a
	// conversa dela fala com o agente que a anterior configurou.
	if _, err := m.Conversation(ctx, testSpec(), "conv-do-leo"); err != nil {
		t.Fatalf("conversa do leo: %v", err)
	}
	if dials != 2 {
		t.Fatalf("processos criados = %d, esperado 2 depois da troca de usuário", dials)
	}
	if novas, _, _ := doLeo.counters(); novas != 1 {
		t.Fatalf("o agente novo abriu %d sessões, esperado 1", novas)
	}
}

func TestSessaoPerdidaEhRemontadaNoProximoUso(t *testing.T) {
	store := newMemoryStore()
	client := newFakeManagedClient()
	m, _ := managerWith(store, client)
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	idOriginal := conv.Session().ID()
	conv.Invalidate()
	if conv.Session() != nil {
		t.Fatal("a sessão perdida continuou pendurada na conversa")
	}

	remontada, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("remontar conversa: %v", err)
	}
	if _, cargas, _ := client.counters(); cargas != 1 {
		t.Fatalf("session/load chamado %d vezes; a retomada tenta o identificador guardado", cargas)
	}
	if got := remontada.Session().ID(); got != idOriginal {
		t.Fatalf("sessão remontada = %q, esperado %q", got, idOriginal)
	}
}

func TestRegistroIlegivelNaoAbreSessaoNovaPorCimaDaAntiga(t *testing.T) {
	store := newMemoryStore()
	store.loadErr = errors.New("banco indisponível")
	client := newFakeManagedClient()
	m, _ := managerWith(store, client)

	if _, err := m.Conversation(context.Background(), testSpec(), "conv-1"); err == nil {
		t.Fatal("erro de leitura do registro passou batido")
	}
	if novas, _, _ := client.counters(); novas != 0 {
		t.Fatalf("session/new chamado %d vezes sem saber se havia sessão", novas)
	}
}

func TestSessaoQueNaoConseguiuSerRegistradaNaoFicaAbertaNoAgente(t *testing.T) {
	store := newMemoryStore()
	store.saveErr = errors.New("banco indisponível")
	client := newFakeManagedClient()
	m, _ := managerWith(store, client)

	_, err := m.Conversation(context.Background(), testSpec(), "conv-1")
	if err == nil {
		t.Fatal("falha ao registrar a sessão passou batido")
	}
	client.mu.Lock()
	sessoes := append([]*fakeManagedSession(nil), client.sessions...)
	client.mu.Unlock()
	if len(sessoes) != 1 {
		t.Fatalf("sessões abertas = %d, esperado 1", len(sessoes))
	}
	if !sessoes[0].isClosed() {
		t.Fatal("a sessão sem registro ficou órfã no agente")
	}
}

func TestProviderSemComandoNaoViraProcesso(t *testing.T) {
	m, dials := managerWith(newMemoryStore(), newFakeManagedClient())

	semComando := testSpec()
	semComando.Command = "  "
	if _, err := m.Conversation(context.Background(), semComando, "conv-1"); err == nil {
		t.Fatal("provider sem comando foi aceito")
	}
	if *dials != 0 {
		t.Fatalf("processos criados = %d para um provider sem comando", *dials)
	}
}
