package database

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupConversationBatchDeleteDB(t *testing.T) (*gorm.DB, context.Context, context.Context) {
	t.Helper()
	previousDB, previousPath := db, dbPath
	path := filepath.ToSlash(filepath.Join(t.TempDir(), "conversation-delete.db"))
	testDB, err := gorm.Open(sqlite.Open(sqliteDSN(path)), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	configureSQLitePool(sqlDB)
	if err := testDB.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.AutoMigrate(
		&User{},
		&Conversation{},
		&ChatMessage{},
		&MemoryRecord{},
		&ACPSession{},
		&TaskListWorkflow{},
		&TaskList{},
		&Task{},
		&ToolInvocation{},
		&SubAgentRun{},
		&ChannelResponsePending{},
		&Channel{},
		&ChannelContactConversation{},
		&Tag{},
		&TagAssignment{},
	); err != nil {
		t.Fatal(err)
	}
	db, dbPath = testDB, path
	t.Cleanup(func() {
		_ = sqlDB.Close()
		db, dbPath = previousDB, previousPath
	})
	return testDB,
		WithUserID(context.Background(), "delete-owner"),
		WithUserID(context.Background(), "delete-other")
}

func seedDeleteConversation(t *testing.T, testDB *gorm.DB, userID, title string) (*Conversation, *ChatMessage) {
	t.Helper()
	conv := &Conversation{UserID: userID, Title: title}
	if err := testDB.Create(conv).Error; err != nil {
		t.Fatal(err)
	}
	msg := &ChatMessage{ConversationID: conv.ID, Role: "user", Content: title}
	if err := testDB.Create(msg).Error; err != nil {
		t.Fatal(err)
	}
	return conv, msg
}

func countWhere(t *testing.T, testDB *gorm.DB, model any, query string, args ...any) int64 {
	t.Helper()
	var count int64
	if err := testDB.Model(model).Where(query, args...).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func TestDeleteConversationsWithContextNormalizesAndCleansAssociations(t *testing.T) {
	testDB, ownerCtx, _ := setupConversationBatchDeleteDB(t)
	first, firstMsg := seedDeleteConversation(t, testDB, "delete-owner", "first")
	second, secondMsg := seedDeleteConversation(t, testDB, "delete-owner", "second")
	other, otherMsg := seedDeleteConversation(t, testDB, "delete-other", "other")
	child, _ := seedDeleteConversation(t, testDB, "delete-owner", "child kept")
	child.Kind = ConversationKindSubagent
	child.ParentConversationID = first.ID
	if err := testDB.Save(child).Error; err != nil {
		t.Fatal(err)
	}

	for userID, msgID := range map[string]string{
		"delete-owner": firstMsg.ID,
		"delete-other": otherMsg.ID,
	} {
		if err := testDB.Create(&ToolInvocation{
			UserID: userID, ToolCatalogID: "tool", OriginType: "chat",
			OriginID: msgID, Status: "succeeded", QueuedAt: time.Now(),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := testDB.Create(&ToolInvocation{
		UserID: "delete-owner", ToolCatalogID: "tool", OriginType: "chat",
		OriginID: secondMsg.ID, Status: "succeeded", QueuedAt: time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&ToolInvocation{
		UserID: "delete-other", ToolCatalogID: "tool", OriginType: "chat",
		OriginID: firstMsg.ID, Status: "succeeded", QueuedAt: time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	for _, row := range []any{
		&ChannelResponsePending{ConversationID: first.ID, OwnerUserID: "delete-other", Channel: "signal", ChatID: "legacy"},
		&ChannelResponsePending{ConversationID: other.ID, OwnerUserID: "delete-other", Channel: "signal", ChatID: "other"},
		&ACPSession{UserID: "delete-owner", ConversationID: first.ID, ProviderID: "acp", SessionID: "owner"},
		&ACPSession{UserID: "delete-other", ConversationID: first.ID, ProviderID: "acp-legacy", SessionID: "legacy"},
		&ACPSession{UserID: "delete-other", ConversationID: other.ID, ProviderID: "acp", SessionID: "other"},
	} {
		if err := testDB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	ownerChannel := &Channel{UserID: "delete-owner", Type: "signal", Slug: "owner", DisplayName: "Owner"}
	otherChannel := &Channel{UserID: "delete-other", Type: "signal", Slug: "other", DisplayName: "Other"}
	if err := testDB.Create(ownerChannel).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(otherChannel).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range []*ChannelContactConversation{
		{ChannelID: ownerChannel.ID, ContactExternalID: "owner", ConversationID: first.ID},
		{ChannelID: otherChannel.ID, ContactExternalID: "legacy", ConversationID: first.ID},
		{ChannelID: otherChannel.ID, ContactExternalID: "other", ConversationID: other.ID},
	} {
		if err := testDB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := testDB.Create(&SubAgentRun{
		UserID: "delete-owner", ParentConversationID: first.ID, ParentTurnID: firstMsg.ID,
		ChildConversationID: second.ID, Status: SubAgentRunStatusSucceeded,
	}).Error; err != nil {
		t.Fatal(err)
	}
	keptRun := &SubAgentRun{
		UserID: "delete-owner", ParentConversationID: first.ID, ParentTurnID: firstMsg.ID,
		ChildConversationID: child.ID, Status: SubAgentRunStatusSucceeded,
	}
	if err := testDB.Create(keptRun).Error; err != nil {
		t.Fatal(err)
	}

	linkedID := first.ID
	list := &TaskList{UserID: "delete-owner", Title: "linked", ConversationID: &linkedID}
	if err := testDB.Create(list).Error; err != nil {
		t.Fatal(err)
	}
	taskID := second.ID
	task := &Task{TaskListID: list.ID, Title: "linked task", StatusID: 1, ConversationID: &taskID}
	if err := testDB.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&MemoryRecord{
		UserID: "delete-owner", Content: "scoped", Kind: MemoryKindHistoricalNote,
		LoadPolicy: MemoryLoadPolicyRetrievable, Scope: MemoryScopeConversation, ScopeRef: first.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	tag := &Tag{UserID: "delete-owner", Slug: "history", Name: "History"}
	if err := testDB.Create(tag).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&TagAssignment{
		UserID: "delete-owner", TagID: tag.ID, ResourceType: "conversation", ResourceID: second.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}

	deleted, err := DeleteConversationsWithContext(ownerCtx, []string{" " + first.ID, second.ID, first.ID + " "})
	if err != nil {
		t.Fatalf("DeleteConversationsWithContext: %v", err)
	}
	if len(deleted) != 2 || deleted[0] != first.ID || deleted[1] != second.ID {
		t.Fatalf("IDs normalizados inesperados: %v", deleted)
	}
	if countWhere(t, testDB, &Conversation{}, "id IN ?", []string{first.ID, second.ID}) != 0 ||
		countWhere(t, testDB, &ChatMessage{}, "conversation_id IN ?", []string{first.ID, second.ID}) != 0 ||
		countWhere(t, testDB, &ToolInvocation{}, "user_id = ?", "delete-owner") != 0 ||
		countWhere(t, testDB, &ChannelResponsePending{}, "conversation_id = ?", first.ID) != 0 ||
		countWhere(t, testDB, &ACPSession{}, "conversation_id = ?", first.ID) != 0 ||
		countWhere(t, testDB, &ChannelContactConversation{}, "conversation_id = ?", first.ID) != 0 ||
		countWhere(t, testDB, &MemoryRecord{}, "scope_ref = ?", first.ID) != 0 ||
		countWhere(t, testDB, &TagAssignment{}, "resource_id = ?", second.ID) != 0 {
		t.Fatal("dependências das conversas excluídas permaneceram")
	}
	if countWhere(t, testDB, &Conversation{}, "id = ?", other.ID) != 1 ||
		countWhere(t, testDB, &ChatMessage{}, "id = ?", otherMsg.ID) != 1 ||
		countWhere(t, testDB, &ToolInvocation{}, "user_id = ?", "delete-other") != 2 ||
		countWhere(t, testDB, &ToolInvocation{}, "user_id = ? AND origin_id = ?", "delete-other", firstMsg.ID) != 1 {
		t.Fatal("dados de outro usuário foram alterados")
	}
	if err := testDB.First(child, "id = ?", child.ID).Error; err != nil {
		t.Fatal(err)
	}
	if child.ParentConversationID != "" {
		t.Fatalf("sub-conversa mantida ainda aponta para pai excluído: %q", child.ParentConversationID)
	}
	if err := testDB.First(keptRun, "id = ?", keptRun.ID).Error; err != nil {
		t.Fatal(err)
	}
	if keptRun.ParentConversationID != "" || keptRun.ParentTurnID != "" {
		t.Fatalf("run mantido ainda aponta para pai excluído: %+v", keptRun)
	}
	if err := testDB.First(list, "id = ?", list.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.First(task, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if list.ConversationID != nil || task.ConversationID != nil {
		t.Fatalf("vínculos de tarefas não foram limpos: list=%v task=%v", list.ConversationID, task.ConversationID)
	}
}

func TestDeleteConversationsWithContextFailsClosedBeforeMutation(t *testing.T) {
	testDB, ownerCtx, _ := setupConversationBatchDeleteDB(t)
	owner, ownerMsg := seedDeleteConversation(t, testDB, "delete-owner", "owner")
	other, _ := seedDeleteConversation(t, testDB, "delete-other", "other")

	for _, invalidID := range []string{"missing", other.ID} {
		_, err := DeleteConversationsWithContext(ownerCtx, []string{owner.ID, invalidID})
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("ID inválido %q: erro=%v, esperado record not found", invalidID, err)
		}
		if countWhere(t, testDB, &Conversation{}, "id = ?", owner.ID) != 1 ||
			countWhere(t, testDB, &ChatMessage{}, "id = ?", ownerMsg.ID) != 1 {
			t.Fatalf("lote parcialmente apagado para ID inválido %q", invalidID)
		}
	}
}

func TestSetTaskLinksNormalizaConversationID(t *testing.T) {
	testDB, ownerCtx, _ := setupConversationBatchDeleteDB(t)
	conv, _ := seedDeleteConversation(t, testDB, "delete-owner", "target")
	list := &TaskList{UserID: "delete-owner", Title: "Lista"}
	if err := testDB.Create(list).Error; err != nil {
		t.Fatal(err)
	}
	task := &Task{TaskListID: list.ID, Title: "Tarefa"}
	if err := testDB.Create(task).Error; err != nil {
		t.Fatal(err)
	}

	padded := "  " + conv.ID + "  "
	if err := SetTaskListConversationWithContext(ownerCtx, list.ID, &padded); err != nil {
		t.Fatalf("SetTaskListConversationWithContext: %v", err)
	}
	if err := SetTaskConversationWithContext(ownerCtx, task.ID, &padded); err != nil {
		t.Fatalf("SetTaskConversationWithContext: %v", err)
	}
	if err := testDB.First(list, "id = ?", list.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.First(task, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if list.ConversationID == nil || *list.ConversationID != conv.ID ||
		task.ConversationID == nil || *task.ConversationID != conv.ID {
		t.Fatalf("IDs não normalizados: list=%v task=%v", list.ConversationID, task.ConversationID)
	}

	blank := " \t "
	if err := SetTaskListConversationWithContext(ownerCtx, list.ID, &blank); err != nil {
		t.Fatalf("desvincular tasklist: %v", err)
	}
	if err := SetTaskConversationWithContext(ownerCtx, task.ID, &blank); err != nil {
		t.Fatalf("desvincular task: %v", err)
	}
	if err := testDB.First(list, "id = ?", list.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.First(task, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if list.ConversationID != nil || task.ConversationID != nil {
		t.Fatalf("IDs vazios não viraram nil: list=%v task=%v", list.ConversationID, task.ConversationID)
	}
}

func TestDeleteConversationWithContextUsesCanonicalBatchCleanup(t *testing.T) {
	testDB, ownerCtx, _ := setupConversationBatchDeleteDB(t)
	conv, msg := seedDeleteConversation(t, testDB, "delete-owner", "single")
	if err := testDB.Create(&ToolInvocation{
		UserID: "delete-owner", ToolCatalogID: "tool", OriginType: "chat",
		OriginID: msg.ID, Status: "succeeded", QueuedAt: time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := DeleteConversationWithContext(ownerCtx, conv.ID); err != nil {
		t.Fatalf("DeleteConversationWithContext: %v", err)
	}
	if countWhere(t, testDB, &Conversation{}, "id = ?", conv.ID) != 0 ||
		countWhere(t, testDB, &ChatMessage{}, "id = ?", msg.ID) != 0 ||
		countWhere(t, testDB, &ToolInvocation{}, "origin_id = ?", msg.ID) != 0 {
		t.Fatal("exclusão unitária divergiu da limpeza batch canônica")
	}
}

func TestDeleteConversationsWithContextRollsBackAllDependencies(t *testing.T) {
	testDB, ownerCtx, _ := setupConversationBatchDeleteDB(t)
	conv, msg := seedDeleteConversation(t, testDB, "delete-owner", "rollback")
	if err := testDB.Create(&ToolInvocation{
		UserID: "delete-owner", ToolCatalogID: "tool", OriginType: "chat",
		OriginID: msg.ID, Status: "succeeded", QueuedAt: time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	forced := errors.New("falha forçada")
	const callback = "test:fail_conversation_batch_delete"
	if err := testDB.Callback().Delete().Before("gorm:delete").Register(callback, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "conversations" {
			_ = tx.AddError(forced)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testDB.Callback().Delete().Remove(callback) })

	if _, err := DeleteConversationsWithContext(ownerCtx, []string{conv.ID}); !errors.Is(err, forced) {
		t.Fatalf("erro=%v, esperado falha forçada", err)
	}
	if countWhere(t, testDB, &Conversation{}, "id = ?", conv.ID) != 1 ||
		countWhere(t, testDB, &ChatMessage{}, "id = ?", msg.ID) != 1 ||
		countWhere(t, testDB, &ToolInvocation{}, "origin_id = ?", msg.ID) != 1 {
		t.Fatal("rollback não preservou integralmente conversa, mensagem e invocação")
	}
}

func TestDeleteConversationsConcurrentWithReaderAndMaintenance(t *testing.T) {
	testDB, ownerCtx, _ := setupConversationBatchDeleteDB(t)
	const groups = 4
	const perGroup = 6
	batches := make([][]string, groups)
	for group := range groups {
		for range perGroup {
			conv, _ := seedDeleteConversation(t, testDB, "delete-owner", "concurrent")
			batches[group] = append(batches[group], conv.ID)
		}
	}

	start := make(chan struct{})
	stopReader := make(chan struct{})
	errs := make(chan error, groups+4)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for {
			select {
			case <-stopReader:
				return
			default:
				var count int64
				if err := testDB.WithContext(ownerCtx).Model(&Conversation{}).
					Where("user_id = ?", "delete-owner").Count(&count).Error; err != nil {
					errs <- err
					return
				}
			}
		}
	}()

	for _, batch := range batches {
		batch := batch
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := DeleteConversationsWithContext(ownerCtx, batch)
			errs <- err
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		_, err := Compact(ownerCtx, false, 1<<60)
		errs <- err
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		_, err := CreateConversationWithContext(ownerCtx, "criada durante exclusão", "")
		errs <- err
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		_, _, err := FindOrCreateChannelConversationWithContext(ownerCtx, "signal", "concurrent-contact", "Contato")
		errs <- err
	}()

	close(start)
	for range groups + 3 {
		if err := <-errs; err != nil {
			t.Errorf("operação concorrente falhou: %v", err)
		}
	}
	close(stopReader)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("leitor concorrente falhou: %v", err)
		}
	}
	if countWhere(t, testDB, &Conversation{}, "user_id = ?", "delete-owner") != 2 {
		t.Fatal("writers de criação não concluíram após as exclusões")
	}
}

func TestValidateOwnedConversationIDsWaitsForMaintenanceGate(t *testing.T) {
	testDB, ownerCtx, _ := setupConversationBatchDeleteDB(t)
	conv, _ := seedDeleteConversation(t, testDB, "delete-owner", "preflight gate")
	release, err := acquireSQLiteMaintenance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := ValidateOwnedConversationIDsWithContext(ownerCtx, []string{conv.ID})
		result <- err
	}()
	select {
	case err := <-result:
		release()
		t.Fatalf("preflight atravessou manutenção ativa: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	release()
	if err := <-result; err != nil {
		t.Fatalf("preflight após manutenção: %v", err)
	}
}

func TestCreateMessageCannotRaceIntoDeletedConversation(t *testing.T) {
	testDB, ownerCtx, _ := setupConversationBatchDeleteDB(t)
	for range 12 {
		conv, _ := seedDeleteConversation(t, testDB, "delete-owner", "message race")
		start := make(chan struct{})
		errs := make(chan error, 2)
		go func() {
			<-start
			_, err := CreateMessageWithContext(ownerCtx, MessageOptions{
				ConversationID: conv.ID,
				Role:           "assistant",
				Content:        "concorrente",
			})
			errs <- err
		}()
		go func() {
			<-start
			errs <- DeleteConversationWithContext(ownerCtx, conv.ID)
		}()
		close(start)

		for range 2 {
			err := <-errs
			if err != nil && !errors.Is(err, ErrConversationDeleted) {
				t.Fatalf("corrida create/delete: %v", err)
			}
		}
		if countWhere(t, testDB, &ChatMessage{}, "conversation_id = ?", conv.ID) != 0 {
			t.Fatal("mensagem órfã criada após exclusão concorrente")
		}
	}
}

func TestRecycleCannotRecreateDeletedConversation(t *testing.T) {
	testDB, ownerCtx, _ := setupConversationBatchDeleteDB(t)
	conv, _ := seedDeleteConversation(t, testDB, "delete-owner", "recycle race")
	if err := testDB.Where("conversation_id = ?", conv.ID).Delete(&ChatMessage{}).Error; err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	go func() {
		<-start
		_, err := RecycleOrCreateConversationWithContext(ownerCtx, "recycled")
		errs <- err
	}()
	go func() {
		<-start
		errs <- DeleteConversationWithContext(ownerCtx, conv.ID)
	}()
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("corrida recycle/delete: %v", err)
		}
	}
	if countWhere(t, testDB, &Conversation{}, "id = ?", conv.ID) != 0 {
		t.Fatal("reciclagem recriou a conversa excluída")
	}
}

func TestPendingResponseCannotRaceIntoDeletedConversation(t *testing.T) {
	testDB, ownerCtx, _ := setupConversationBatchDeleteDB(t)
	conv, _ := seedDeleteConversation(t, testDB, "delete-owner", "pending race")
	start := make(chan struct{})
	errs := make(chan error, 2)
	go func() {
		<-start
		errs <- UpsertChannelResponsePending(ownerCtx, &ChannelResponsePending{
			ConversationID: conv.ID,
			OwnerUserID:    "delete-owner",
			Channel:        "signal",
			ChatID:         "race",
		})
	}()
	go func() {
		<-start
		errs <- DeleteConversationWithContext(ownerCtx, conv.ID)
	}()
	close(start)
	for range 2 {
		err := <-errs
		if err != nil && !errors.Is(err, ErrConversationDeleted) {
			t.Fatalf("corrida pending/delete: %v", err)
		}
	}
	if countWhere(t, testDB, &ChannelResponsePending{}, "conversation_id = ?", conv.ID) != 0 {
		t.Fatal("pendência órfã criada após exclusão concorrente")
	}
}
