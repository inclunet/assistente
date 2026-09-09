package chat

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/providers"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type blockingProviderStore struct {
	started chan struct{}
	release <-chan struct{}
}

func (s *blockingProviderStore) Save(context.Context, []*llm.ProviderConfig) error { return nil }
func (s *blockingProviderStore) Load(context.Context) ([]*llm.ProviderConfig, error) {
	return nil, nil
}
func (s *blockingProviderStore) SetDefault(context.Context, string) error { return nil }
func (s *blockingProviderStore) GetDefault(context.Context) (*llm.ProviderConfig, error) {
	return nil, nil
}
func (s *blockingProviderStore) Get(context.Context, string) (*llm.ProviderConfig, error) {
	return nil, nil
}
func (s *blockingProviderStore) Count(context.Context) (int, error) {
	s.started <- struct{}{}
	<-s.release
	return 1, nil
}

type blockingConversationRepo struct {
	started chan struct{}
	release <-chan struct{}
}

func (r *blockingConversationRepo) GetConversationInfo(context.Context, string) (*Conversation, error) {
	r.started <- struct{}{}
	<-r.release
	return &Conversation{Title: "Título existente"}, nil
}
func (*blockingConversationRepo) UpdateConversation(context.Context, string, string, string) error {
	return nil
}
func (*blockingConversationRepo) UpdateConversationChannel(context.Context, string, string, string) error {
	return nil
}

func TestPrepareContextStartsIndependentIOConcurrently(t *testing.T) {
	release := make(chan struct{})
	providerStarted := make(chan struct{}, 1)
	conversationStarted := make(chan struct{}, 1)
	providerSvc := providers.NewService(providers.ServiceConfig{
		Store: &blockingProviderStore{started: providerStarted, release: release},
	})
	interactor := NewInteractor(InteractorConfig{
		Emitter:     &spyEmitter{},
		ConvRepo:    &blockingConversationRepo{started: conversationStarted, release: release},
		ProviderSvc: providerSvc,
	})

	done := make(chan error, 1)
	go func() {
		_, err := interactor.PrepareContext(context.Background(), PrepareContextRequest{
			ConversationID: "conv-1",
			UserContent:    "mensagem",
		})
		done <- err
	}()

	for name, started := range map[string]<-chan struct{}{
		"provider": providerStarted,
		"conversa": conversationStarted,
	} {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("I/O de %s não iniciou antes da liberação do outro", name)
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("PrepareContext: %v", err)
	}
}

type zeroProviderStore struct {
	*blockingProviderStore
	conversationStarted <-chan struct{}
}

func (s *zeroProviderStore) Count(context.Context) (int, error) {
	<-s.conversationStarted
	return 0, nil
}

type cancelAwareConversationRepo struct {
	started   chan<- struct{}
	cancelled chan<- struct{}
}

func (r *cancelAwareConversationRepo) GetConversationInfo(ctx context.Context, _ string) (*Conversation, error) {
	r.started <- struct{}{}
	<-ctx.Done()
	r.cancelled <- struct{}{}
	return nil, ctx.Err()
}
func (*cancelAwareConversationRepo) UpdateConversation(context.Context, string, string, string) error {
	return nil
}
func (*cancelAwareConversationRepo) UpdateConversationChannel(context.Context, string, string, string) error {
	return nil
}

func TestPrepareContextCancelsConcurrentIOOnEarlyReturn(t *testing.T) {
	conversationStarted := make(chan struct{}, 1)
	conversationCancelled := make(chan struct{}, 1)
	providerSvc := providers.NewService(providers.ServiceConfig{
		Store: &zeroProviderStore{
			blockingProviderStore: &blockingProviderStore{},
			conversationStarted:   conversationStarted,
		},
	})
	interactor := NewInteractor(InteractorConfig{
		Emitter: &spyEmitter{},
		ConvRepo: &cancelAwareConversationRepo{
			started:   conversationStarted,
			cancelled: conversationCancelled,
		},
		ProviderSvc: providerSvc,
	})

	if _, err := interactor.PrepareContext(context.Background(), PrepareContextRequest{
		ConversationID: "conv-1",
		UserContent:    "mensagem",
	}); err == nil {
		t.Fatal("esperava erro por ausência de provider")
	}
	select {
	case <-conversationCancelled:
	case <-time.After(time.Second):
		t.Fatal("I/O concorrente não recebeu cancelamento após retorno antecipado")
	}
}

type batchHistoryRepo struct {
	*stubRepo
	mu            sync.Mutex
	batchCalls    int
	windowCalls   int
	createCalls   int
	summaryCalls  int
	messagesCalls int
	committed     bool
}

func newBatchHistoryRepo() *batchHistoryRepo {
	return &batchHistoryRepo{stubRepo: &stubRepo{}}
}

func (r *batchHistoryRepo) CreateMessage(context.Context, MessageOptions) (*Message, error) {
	r.createCalls++
	return nil, nil
}

func (r *batchHistoryRepo) GetConversationSummary(context.Context, string) (string, string, error) {
	r.summaryCalls++
	return "", "", nil
}

func (r *batchHistoryRepo) GetMessages(context.Context, string, *string) ([]Message, error) {
	r.messagesCalls++
	return nil, nil
}

func (r *batchHistoryRepo) CreateUserMessageAndLoadHistory(_ context.Context, opts MessageOptions, _ int) (*Message, *HistoryWindow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batchCalls++
	msg := &Message{
		UUIDModel:      database.UUIDModel{ID: "user-1"},
		ConversationID: opts.ConversationID,
		Role:           "user",
		Content:        opts.Content,
	}
	r.committed = true
	return msg, &HistoryWindow{
		Messages:                 []Message{*msg},
		SummaryBoundaryAvailable: true,
	}, nil
}

func (r *batchHistoryRepo) LoadHistoryWindow(context.Context, string, int) (*HistoryWindow, error) {
	r.windowCalls++
	return &HistoryWindow{
		Messages: []Message{{
			UUIDModel: database.UUIDModel{ID: "user-1"},
			Role:      "user",
			Content:   "mensagem",
		}},
		SummaryBoundaryAvailable: true,
	}, nil
}

type commitCheckingEmitter struct {
	repo              *batchHistoryRepo
	readyAfterCommit  bool
	messagesReadySeen int
}

func (e *commitCheckingEmitter) Emit(name string, _ any) {
	if name != "chat:messages_ready" {
		return
	}
	e.messagesReadySeen++
	e.readyAfterCommit = e.repo.committed
}

func TestRecordUserMessageUsesSingleBatchCallAndEmitsAfterCommit(t *testing.T) {
	repo := newBatchHistoryRepo()
	emitter := &commitCheckingEmitter{repo: repo}
	interactor := NewInteractor(InteractorConfig{Emitter: emitter, Repo: repo})

	result, err := interactor.RecordUserMessage(context.Background(), RecordUserMessageRequest{
		ConversationID: "conv-1",
		Content:        "mensagem",
	})
	if err != nil {
		t.Fatalf("RecordUserMessage: %v", err)
	}
	if repo.batchCalls != 1 || repo.createCalls != 0 || repo.summaryCalls != 0 || repo.messagesCalls != 0 {
		t.Fatalf("chamadas ao store: batch=%d create=%d summary=%d messages=%d",
			repo.batchCalls, repo.createCalls, repo.summaryCalls, repo.messagesCalls)
	}
	if emitter.messagesReadySeen != 1 || !emitter.readyAfterCommit {
		t.Fatalf("messages_ready deve ocorrer uma vez após commit: %+v", emitter)
	}
	if result.UserMsg.ID != "user-1" || len(result.Messages) != 1 {
		t.Fatalf("resultado batch inesperado: %+v", result)
	}
}

func TestRetryUsesCanonicalHistoryWindowWithoutFullConversation(t *testing.T) {
	repo := newBatchHistoryRepo()
	interactor := NewInteractor(InteractorConfig{Emitter: &spyEmitter{}, Repo: repo})
	user := &Message{
		UUIDModel:      database.UUIDModel{ID: "user-1"},
		ConversationID: "conv-1",
		Role:           "user",
		Content:        "mensagem",
	}

	result, err := interactor.ReuseLoadedUserMessage(context.Background(), RecordUserMessageRequest{
		ConversationID: "conv-1",
	}, user)
	if err != nil {
		t.Fatalf("ReuseLoadedUserMessage: %v", err)
	}
	if repo.windowCalls != 1 || repo.summaryCalls != 0 || repo.messagesCalls != 0 {
		t.Fatalf("janela canônica não foi exclusiva: window=%d summary=%d messages=%d",
			repo.windowCalls, repo.summaryCalls, repo.messagesCalls)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("esperava uma mensagem, obteve %d", len(result.Messages))
	}
}

var _ ports.Emitter = (*commitCheckingEmitter)(nil)

func setupHotPathDB(t *testing.T) (*gorm.DB, context.Context, string) {
	t.Helper()
	previous := database.DB()
	testDB, err := gorm.Open(sqlite.Open("file:chat-hot-path?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB.AutoMigrate(&database.Conversation{}, &database.ChatMessage{}); err != nil {
		t.Fatal(err)
	}
	database.SetDB(testDB)
	t.Cleanup(func() {
		sqlDB, _ := testDB.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		database.SetDB(previous)
	})

	ctx := database.WithUserID(context.Background(), "user-hot-path")
	conv := &database.Conversation{UserID: "user-hot-path", Title: DefaultConversationTitle}
	if err := testDB.Create(conv).Error; err != nil {
		t.Fatal(err)
	}
	return testDB, ctx, conv.ID
}

func TestDBMessageStorePlaceholderIsIdempotentUnderConcurrency(t *testing.T) {
	testDB, ctx, conversationID := setupHotPathDB(t)
	store := NewDBMessageStore()
	const workers = 16
	ids := make(chan string, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := store.EnsureAssistantPlaceholder(ctx, conversationID, "turn-1")
			ids <- id
			errs <- err
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("EnsureAssistantPlaceholder concorrente: %v", err)
		}
	}
	var first string
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatalf("IDs divergentes: primeiro=%s atual=%s", first, id)
		}
	}
	var count int64
	if err := testDB.Model(&database.ChatMessage{}).
		Where("conversation_id = ? AND turn_id = ? AND role = ?", conversationID, "turn-1", "assistant").
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("esperava um placeholder persistido, obteve %d", count)
	}
}

func TestDBMessageStorePlaceholderLocksAreScopedByTurn(t *testing.T) {
	store := NewDBMessageStore()
	unlockFirst := store.lockPlaceholderKey("user\x00conv-1\x00turn-1")
	defer unlockFirst()

	sameKeyAcquired := make(chan struct{}, 1)
	go func() {
		unlock := store.lockPlaceholderKey("user\x00conv-1\x00turn-1")
		unlock()
		sameKeyAcquired <- struct{}{}
	}()
	select {
	case <-sameKeyAcquired:
		t.Fatal("mesma chave não deveria adquirir lock concorrentemente")
	case <-time.After(20 * time.Millisecond):
	}

	otherKeyAcquired := make(chan struct{}, 1)
	go func() {
		unlock := store.lockPlaceholderKey("user\x00conv-2\x00turn-2")
		unlock()
		otherKeyAcquired <- struct{}{}
	}()
	select {
	case <-otherKeyAcquired:
	case <-time.After(time.Second):
		t.Fatal("turno independente foi serializado pelo lock de outro turno")
	}
}

func TestDBMessageStoreBatchKeepsHistoryBoundedAndUserScoped(t *testing.T) {
	testDB, ctx, conversationID := setupHotPathDB(t)
	var summaryUpToID string
	for index := range 100 {
		role := "assistant"
		if index%2 == 0 {
			role = "user"
		}
		historyMessage := &database.ChatMessage{
			ConversationID: conversationID,
			Role:           role,
			Content:        "histórico",
		}
		if err := testDB.Create(historyMessage).Error; err != nil {
			t.Fatal(err)
		}
		if index == 79 {
			summaryUpToID = historyMessage.ID
		}
	}
	if err := testDB.Model(&database.Conversation{}).
		Where("id = ?", conversationID).
		Updates(map[string]any{
			"summary":                  "resumo persistido",
			"summary_up_to_message_id": summaryUpToID,
		}).Error; err != nil {
		t.Fatal(err)
	}
	store := NewDBMessageStore()
	msg, window, err := store.CreateUserMessageAndLoadHistory(ctx, MessageOptions{
		ConversationID: conversationID,
		Role:           "user",
		Content:        "atual",
	}, 10)
	if err != nil {
		t.Fatalf("CreateUserMessageAndLoadHistory: %v", err)
	}
	if msg == nil || window == nil || len(window.Messages) > 12 {
		t.Fatalf("janela não limitada: msg=%v tamanho=%d", msg != nil, len(window.Messages))
	}
	if window.Summary != "resumo persistido" || !window.SummaryBoundaryAvailable {
		t.Fatalf("metadados de resumo não preservados: %+v", window)
	}
	if got := window.Messages[len(window.Messages)-1].ID; got != msg.ID {
		t.Fatalf("mensagem atual ausente do fim da janela: got=%s want=%s", got, msg.ID)
	}
	allMessages, err := database.GetMessagesWithContext(ctx, conversationID, nil)
	if err != nil {
		t.Fatal(err)
	}
	legacyLoader := HistoryLoader{Repo: &stubRepo{
		messages: allMessages,
		summary:  "resumo persistido",
		sumUpTo:  summaryUpToID,
	}, MaxMsgs: 10}
	legacy, legacySummary, err := legacyLoader.Load(ctx, conversationID)
	if err != nil {
		t.Fatal(err)
	}
	windowLoader := HistoryLoader{Repo: store, MaxMsgs: 10}
	optimized, optimizedSummary, err := windowLoader.Load(ctx, conversationID)
	if err != nil {
		t.Fatal(err)
	}
	if legacySummary != optimizedSummary || len(legacy) != len(optimized) {
		t.Fatalf("janela alterou semântica: legacy=(%q,%d) optimized=(%q,%d)",
			legacySummary, len(legacy), optimizedSummary, len(optimized))
	}
	for index := range legacy {
		if legacy[index].ID != optimized[index].ID {
			t.Fatalf("ordem divergiu em %d: legacy=%s optimized=%s", index, legacy[index].ID, optimized[index].ID)
		}
	}

	otherCtx := database.WithUserID(context.Background(), "outro-user")
	if _, _, err := store.CreateUserMessageAndLoadHistory(otherCtx, MessageOptions{
		ConversationID: conversationID,
		Role:           "user",
		Content:        "intrusão",
	}, 10); err == nil {
		t.Fatal("batch cross-user deveria falhar fechado")
	}
}

func TestHistoryWindowKeepsSummaryWhenBoundaryIsLastMessage(t *testing.T) {
	testDB, ctx, conversationID := setupHotPathDB(t)
	boundary := &database.ChatMessage{
		ConversationID: conversationID,
		Role:           "user",
		Content:        "já resumida",
	}
	if err := testDB.Create(boundary).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Model(&database.Conversation{}).
		Where("id = ?", conversationID).
		Updates(map[string]any{
			"summary":                  "resumo íntegro",
			"summary_up_to_message_id": boundary.ID,
		}).Error; err != nil {
		t.Fatal(err)
	}

	store := NewDBMessageStore()
	window, err := store.LoadHistoryWindow(ctx, conversationID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(window.Messages) != 0 || !window.SummaryBoundaryAvailable {
		t.Fatalf("boundary final deveria produzir janela vazia válida: %+v", window)
	}
	messages, summary, err := (&HistoryLoader{Repo: store, MaxMsgs: 10}).Load(ctx, conversationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 || summary != "resumo íntegro" {
		t.Fatalf("resumo descartado: summary=%q messages=%d", summary, len(messages))
	}
}

func TestBatchDoesNotMaskDatabaseFailureAsDeletedConversation(t *testing.T) {
	testDB, ctx, conversationID := setupHotPathDB(t)
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	_, _, err = NewDBMessageStore().CreateUserMessageAndLoadHistory(ctx, MessageOptions{
		ConversationID: conversationID,
		Role:           "user",
		Content:        "mensagem",
	}, 10)
	if err == nil {
		t.Fatal("esperava falha do banco fechado")
	}
	if errors.Is(err, ErrConversationDeleted) {
		t.Fatalf("falha de infraestrutura mascarada como conversa deletada: %v", err)
	}
}
