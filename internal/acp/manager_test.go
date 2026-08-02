package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	sess := &fakeManagedSession{id: fmt.Sprintf("sess-%d", c.nextID), cwd: cwd}
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
	sess := &fakeManagedSession{id: sessionID, cwd: cwd}
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

	mu     sync.Mutex
	closed bool
}

func (s *fakeManagedSession) ID() string { return s.id }

func (s *fakeManagedSession) Prompt(context.Context, []Content, UpdateSink) (StopReason, error) {
	return StopEndTurn, nil
}

func (s *fakeManagedSession) Close(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *fakeManagedSession) Cancel(context.Context) error { return nil }

func (s *fakeManagedSession) ConfigOptions() []ConfigOption { return nil }

func (s *fakeManagedSession) SetConfigOption(context.Context, string, string) ([]ConfigOption, error) {
	return nil, nil
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

func (s *memoryStore) Delete(_ context.Context, conversationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, rec := range s.rows {
		if rec.ConversationID == conversationID {
			delete(s.rows, key)
		}
	}
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

// managerWith monta um manager sobre um agente de mentira, contando quantas
// vezes o transporte foi criado.
func managerWith(store SessionStore, client *fakeManagedClient) (*Manager, *int) {
	dials := 0
	m := NewManager(ManagerConfig{
		Store:   store,
		WorkDir: func() (string, error) { return "/projeto", nil },
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

func TestAgenteQueNaoRetomaDeixaClaroQueAMemoriaSePerdeu(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()
	if err := store.Save(ctx, StoredSession{
		ConversationID: "conv-1",
		ProviderID:     "cursor",
		SessionID:      "sess-antiga",
		PrefixHash:     "hash-persona",
		WorkDir:        "/projeto",
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
		WorkDir:        "/outro-projeto",
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
		WorkDir: func() (string, error) { return "/projeto", nil },
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
	if conv.Session().(*fakeManagedSession).cwd != "/projeto" {
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
		WorkDir: func() (string, error) { return "/projeto", nil },
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

func TestSessaoQueNaoVoltouEhEncerradaAntesDeSerSubstituida(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()
	if err := store.Save(ctx, StoredSession{
		ConversationID: "conv-1",
		ProviderID:     "cursor",
		SessionID:      "sess-antiga",
		WorkDir:        "/outro-projeto",
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
		WorkDir: func() (string, error) { return "/projeto", nil },
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
