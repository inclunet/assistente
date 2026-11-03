package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestWindowBudgetCountsRepeatedContinuationPresentation(t *testing.T) {
	f := newSnapshotFixture(t)
	turns := database.NewChatTurnRepository(f.db)
	execution, err := turns.Begin(f.ctx, f.conversation, f.question, f.answer, database.TurnModeContinue)
	if err != nil {
		t.Fatal(err)
	}
	destination := strings.Repeat("d", 512*1024)
	if err := f.db.Table("chat_messages").Create(map[string]any{"id": destination, "conversation_id": f.conversation, "turn_id": f.question, "role": "assistant", "content": "curto", "chat_execution_id": execution.ExecutionID, "created_at": time.Now(), "updated_at": time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("UPDATE chat_turn_attempts SET response_message_id=? WHERE execution_id=?", destination, execution.ExecutionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("UPDATE chat_turns SET response_message_id=? WHERE user_message_id=?", destination, f.question).Error; err != nil {
		t.Fatal(err)
	}
	var original database.ToolInvocation
	if err := f.db.First(&original, "id=?", f.invocation).Error; err != nil {
		t.Fatal(err)
	}
	metadata := fmt.Sprintf(`{"display":{"assistant_message_id":%q}}`, f.answer)
	if err := f.db.Model(&database.ToolInvocation{}).Where("id=?", f.invocation).Update("metadata", metadata).Error; err != nil {
		t.Fatal(err)
	}
	// Com uma chamada, prova a reancoragem real sem materializar o caso excedente.
	calls, err := toolinvocations.NewDBRepository(f.db).LoadSummariesForTurnIDs(f.ctx, []string{f.question})
	if err != nil || len(calls[f.question]) != 1 || calls[f.question][0].AssistantMessageID != destination {
		t.Fatal("continuação não exercitada", err)
	}
	for i := 0; i < 19; i++ {
		call := database.ToolInvocation{UserID: original.UserID, ToolCatalogID: original.ToolCatalogID, OriginType: "chat", ConversationID: &f.conversation, TurnID: &f.question, ToolCallID: fmt.Sprint("inherited-", i), Status: "succeeded", Metadata: metadata}
		if err := f.db.Create(&call).Error; err != nil {
			t.Fatal(err)
		}
	}
	payloadReads := 0
	if err := f.db.Callback().Query().After("gorm:query").Register("repeated_presentation", func(tx *gorm.DB) {
		sql := tx.Statement.SQL.String()
		if strings.Contains(sql, "selected_payload") || strings.Contains(sql, " AS assistant_message_id") {
			payloadReads++
		}
	}); err != nil {
		t.Fatal(err)
	}
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 1}
	if _, err := f.reader.ReadCanonicalWindow(f.ctx, query); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal("identidade repetida escapou da cota", err)
	}
	if payloadReads != 0 {
		t.Fatal("payloads materializados antes da recusa", payloadReads)
	}
}

func TestQuestionWindowReconcilesLifecycleWithoutResponsePayload(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(fmt.Sprint("retry=", retry), func(t *testing.T) {
			f, p, e, _ := automaticFixture(t)
			stream := e.StartStream(0)
			c := e.conversation
			c.mu.Lock()
			c.turns[e.turnID].publicationErr = errors.New("falha anterior")
			c.mu.Unlock()
			repo := database.NewChatTurnRepository(f.db)
			if err := repo.Finish(f.ctx, f.conversation, f.question, e.executionID, database.TurnCompleted, "completed"); err != nil {
				t.Fatal(err)
			}
			if retry {
				if _, err := repo.Begin(f.ctx, f.conversation, f.question, "", database.TurnModeRetry); err != nil {
					t.Fatal(err)
				}
			}
			ledgerReads := 0
			if err := f.db.Callback().Query().After("gorm:query").Register("question_lifecycle", func(tx *gorm.DB) {
				if strings.Contains(tx.Statement.SQL.String(), " AS assistant_message_id") {
					ledgerReads++
				}
			}); err != nil {
				t.Fatal(err)
			}
			snapshot, err := p.ReadCanonicalWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			if ledgerReads != 0 || len(snapshot.Responses) != 0 || !reflect.DeepEqual(snapshot.TurnIDs, []string{f.question}) || snapshot.Turns[f.question] == nil {
				t.Fatal("lifecycle ausente ou payload fora da página")
			}
			if stream.Append("atrasado") {
				t.Fatal("stream antigo continuou vinculado")
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if !c.turns[e.turnID].detached || c.publicationErrorLocked() != nil {
				t.Fatal("execução antiga não reconciliada")
			}
		})
	}
}

func TestWindowBudgetPrecedesPendingStorageRecovery(t *testing.T) {
	for _, visible := range []bool{false, true} {
		t.Run(fmt.Sprint("visible=", visible), func(t *testing.T) {
			f, p, e, _ := automaticFixture(t)
			// Mais de um lote: o último contém o turno com buffer excessivo.
			var messages []database.ChatMessage
			for i := 0; i < 70; i++ {
				messages = append(messages, database.ChatMessage{ConversationID: f.conversation, Role: "user", Content: "antes"})
			}
			if err := f.db.CreateInBatches(&messages, 100).Error; err != nil {
				t.Fatal(err)
			}
			sub, initial, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "recovery-budget"}, "surface", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 100}, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			reader := &failingSelectionReader{DBTimelineReader: f.reader}
			p.reader = reader
			reader.fail.Store(true)
			if err := p.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) { return nil, nil }); err != nil {
				t.Fatal(err)
			}
			reader.fail.Store(false)
			if err := f.db.Model(&database.ChatMessage{}).Where("id=?", messages[0].ID).Update("content", "depois").Error; err != nil {
				t.Fatal(err)
			}
			c := sub.conversation
			c.mu.Lock()
			part := &livePart{}
			part.text.WriteString(strings.Repeat("x", canonicalSourceBytes+1))
			c.turns[e.turnID].streams[0] = &liveStream{parts: []*livePart{part}}
			before, err := json.Marshal(c.publication.items)
			seq := c.publication.sequence
			c.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			selections := 0
			reader.afterSelection = func() { selections++ }
			limit := 1
			if visible {
				limit = 100
			}
			position := &TimelineWindowPosition{Anchor: "start", Direction: "after", Limit: limit}
			if _, err := sub.Refresh(f.ctx, initial.WindowRevision, position); !errors.Is(err, ErrTimelineSourceLimit) {
				t.Fatal("recuperação ultrapassou cota", err)
			}
			if visible && selections != 0 {
				t.Fatal("recuperação executada antes da cota da janela")
			}
			if !visible && selections < 2 {
				t.Fatal("recuperação em múltiplos lotes não exercitada", selections)
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			after, err := json.Marshal(c.publication.items)
			if err != nil || string(before) != string(after) || seq != c.publication.sequence {
				t.Fatal("recuperação publicou antes de recusar", err)
			}
			if c.windows[sub.windowID].revision != 1 || sub.conversation.windows[sub.windowID].query.Limit != 100 || c.turns[e.turnID].streams[0].parts[0].text.Len() != canonicalSourceBytes+1 {
				t.Fatal("recusa alterou inscrição ou buffer")
			}
		})
	}
}

func TestWindowAuthorizationDoesNotMaterializeConversationSummary(t *testing.T) {
	f := newSnapshotFixture(t)
	if err := f.db.Model(&database.Conversation{}).Where("id=?", f.conversation).Update("summary", strings.Repeat("s", 2*1024*1024)).Error; err != nil {
		t.Fatal(err)
	}
	reads := 0
	if err := f.db.Callback().Query().After("gorm:query").Register("window_owner_identity", func(tx *gorm.DB) {
		if conversation, ok := tx.Statement.Dest.(*database.Conversation); ok {
			reads++
			if conversation.ID != f.conversation || conversation.Summary != "" || !reflect.DeepEqual(tx.Statement.Selects, []string{"id"}) {
				t.Error("autorização materializou payload")
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadCanonicalWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, Limit: 1}); err != nil {
		t.Fatal(err)
	}
	if reads != 1 {
		t.Fatal("autorização não exercitada", reads)
	}
}

func TestPublicationBudgetPreservesOpaqueIDs(t *testing.T) {
	f := newSnapshotFixture(t)
	for _, id := range []string{"opaque-ação-\"", "opaque-\x00-suffix", "opaque-" + string([]byte{0xff})} {
		message := database.ChatMessage{UUIDModel: database.UUIDModel{ID: id}, ConversationID: f.conversation, Role: "user", Content: strings.Repeat("x", canonicalSourceBytes+1)}
		if err := f.db.Create(&message).Error; err != nil {
			t.Fatal(err)
		}
		_, err := publicationSelectionReadSize(f.db.WithContext(f.ctx), f.conversation, []string{id}, nil)
		if !errors.Is(err, ErrTimelineSourceLimit) {
			t.Fatalf("identidade normalizada deixou payload fora da conta: %q", id)
		}
	}
}

func TestStorageRecoveryRollsBackLaterBatch(t *testing.T) {
	for _, drift := range []bool{false, true} {
		t.Run(fmt.Sprint("drift=", drift), func(t *testing.T) {
			f, p, _, _ := automaticFixture(t)
			messages := make([]database.ChatMessage, 70)
			for i := range messages {
				messages[i] = database.ChatMessage{ConversationID: f.conversation, Role: "user", Content: "antes"}
			}
			if err := f.db.CreateInBatches(&messages, 100).Error; err != nil {
				t.Fatal(err)
			}
			sub, initial, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "rollback"}, "surface", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 100}, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			reader := &failingSelectionReader{DBTimelineReader: f.reader}
			p.reader = reader
			reader.fail.Store(true)
			if err := p.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) { return nil, nil }); err != nil {
				t.Fatal(err)
			}
			reader.fail.Store(false)
			if err := f.db.Model(&database.ChatMessage{}).Where("conversation_id=? AND role=?", f.conversation, "user").Update("content", "depois").Error; err != nil {
				t.Fatal(err)
			}
			c := sub.conversation
			c.mu.Lock()
			before := c.publication.fork()
			c.mu.Unlock()
			selections := 0
			reader.afterSelection = func() {
				selections++
				if selections == 3 {
					if drift {
						if err := database.UpdateMessageTextWithContext(f.ctx, messages[0].ID, "concorrente"); err != nil {
							t.Fatal(err)
						}
					} else {
						reader.fail.Store(true)
					}
				}
			}
			if _, err := sub.Refresh(f.ctx, initial.WindowRevision, &TimelineWindowPosition{Anchor: "start", Direction: "after", Limit: 1}); err == nil {
				t.Fatal("falha posterior ignorada")
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if selections < 3 {
				t.Fatal("segunda passagem não exercitada", selections)
			}
			if !reflect.DeepEqual(before, c.publication) {
				t.Fatal("cache/journal parcialmente publicados")
			}
		})
	}
}

func TestRecoveryAuthorizationDoesNotReadSummary(t *testing.T) {
	f, p, _, _ := automaticFixture(t)
	sub, initial, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "owner"}, "surface", database.MessageWindowQuery{ConversationID: f.conversation, Limit: 10}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	reader := &failingSelectionReader{DBTimelineReader: f.reader}
	p.reader = reader
	reader.fail.Store(true)
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	reader.fail.Store(false)
	if err := f.db.Model(&database.Conversation{}).Where("id=?", f.conversation).Update("summary", strings.Repeat("s", 2*1024*1024)).Error; err != nil {
		t.Fatal(err)
	}
	reads, selections := 0, 0
	reader.afterSelection = func() { selections++ }
	if err := f.db.Callback().Query().After("gorm:query").Register("recovery_owner_identity", func(tx *gorm.DB) {
		if conversation, ok := tx.Statement.Dest.(*database.Conversation); ok {
			reads++
			if conversation.Summary != "" || !reflect.DeepEqual(tx.Statement.Selects, []string{"id"}) {
				t.Error("resumo materializado na autorização")
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sub.Refresh(f.ctx, initial.WindowRevision, nil); err != nil {
		t.Fatal(err)
	}
	if reads < 3 || selections != 2 {
		t.Fatal("recuperação não exercitada", reads, selections)
	}
}

func TestWindowIdentityBudgetPrecedesPayloadGuard(t *testing.T) {
	f := newSnapshotFixture(t)
	var messages []database.ChatMessage
	for i := 0; i < 20; i++ {
		messages = append(messages, database.ChatMessage{UUIDModel: database.UUIDModel{ID: fmt.Sprint(i) + strings.Repeat("i", 512*1024)}, ConversationID: f.conversation, Role: "user", Content: "curto"})
	}
	if err := f.db.CreateInBatches(&messages, 100).Error; err != nil {
		t.Fatal(err)
	}
	guardCalls := 0
	_, err := database.NewMessageRepository(f.db).WithTimelineTurns().ReadMessageWindowWithContext(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, Limit: 100}, func(_ []database.MessageWindowItem) error { guardCalls++; return nil })
	if !errors.Is(err, ErrTimelineSourceLimit) || guardCalls != 0 {
		t.Fatal("identidades transferidas antes da cota", err, guardCalls)
	}
	// A recusa de parâmetros acontece antes de qualquer consulta ou expansão hex.
	if _, err := publicationSelectionReadSize(nil, f.conversation, []string{strings.Repeat("i", canonicalSourceBytes)}, nil); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal(err)
	}
}

func TestLiveCompositionBudgetsPotentialToolGroups(t *testing.T) {
	calls := make([]TurnSegmentToolCall, 20)
	for i := range calls {
		calls[i].InvocationID = fmt.Sprint(i) + strings.Repeat("i", 256*1024)
	}
	item := &TimelineItem{ItemID: "turn:q", TurnID: "q", Kind: "response", Segments: []ResponseSegment{responseToolSegment(calls)}}
	c := &projectionConversation{turns: map[string]*liveTurn{}}
	if !windowCompositionWithinBudget(c, []*TimelineItem{item}) {
		t.Fatal("base persistida deveria caber")
	}
	c.turns["q"] = &liveTurn{turnID: "q", executionID: "exec"}
	if windowCompositionWithinBudget(c, []*TimelineItem{item}) {
		t.Fatal("identidades de novos grupos não contadas")
	}
}

func TestWindowIdentityBudgetRejectsOversizedAnchor(t *testing.T) {
	f := newSnapshotFixture(t)
	id := strings.Repeat("i", 3*1024*1024)
	if err := f.db.Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: id}, ConversationID: f.conversation, Role: "user", Content: "curto"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, byItem := range []bool{false, true} {
		query := database.MessageWindowQuery{ConversationID: f.conversation, Direction: "around", AnchorMessageID: id, Limit: 1}
		if byItem {
			query.AnchorItemID = "message:" + id
			query.AnchorMessageID = ""
		}
		if _, err := f.reader.ReadCanonicalWindow(f.ctx, query); !errors.Is(err, ErrTimelineSourceLimit) {
			t.Fatal("âncora excessiva aceita", err)
		}
	}
}

func TestStorageRecoverySplitsAggregateLiveBatch(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	c := e.conversation
	var ids []string
	for i := 0; i < 9; i++ {
		question, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, Role: "user", Content: "pergunta"})
		if err != nil {
			t.Fatal(err)
		}
		executionUUID, err := uuid.NewV7()
		if err != nil {
			t.Fatal(err)
		}
		executionID := executionUUID.String()
		_, err = p.Admit(f.ctx, f.conversation, question.ID, executionID, func() (database.ChatTimelineVersion, error) {
			_, err := database.NewChatTurnRepository(f.db).BeginExecution(f.ctx, f.conversation, question.ID, "", database.TurnModeNew, executionID, "")
			if err != nil {
				return database.ChatTimelineVersion{}, err
			}
			return f.reader.ReadVersion(f.ctx, f.conversation)
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, "turn:"+question.ID)
		c.mu.Lock()
		part := &livePart{}
		part.text.WriteString(strings.Repeat("x", 1024*1024))
		c.turns[question.ID].streams[0] = &liveStream{parts: []*livePart{part}}
		c.mu.Unlock()
	}
	reader := &failingSelectionReader{DBTimelineReader: f.reader}
	p.reader = reader
	selections := 0
	reader.afterSelection = func() { selections++ }
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failStoragePublicationLocked(ids, errors.New("recuperação pendente"))
	if err := p.recoverStoragePublicationLocked(f.ctx, f.conversation, c, nil); err != nil {
		t.Fatal(err)
	}
	if selections < 6 {
		t.Fatal("leitura live agregada não subdividida", selections)
	}
	// Parcelas de leitura não são estados públicos: esta recuperação confirma
	// uma única sequência, cujo payload de 9MiB excede o journal de 2MiB.
	if c.publication.sequence != 1 || len(c.publication.batches) != 0 {
		t.Fatal("recuperação publicou parcelas ou journal parcial", c.publication.sequence)
	}
	if _, ok := c.publication.replay(c.epoch, 0); ok {
		t.Fatal("overflow aceitou replay incompleto")
	}
	if batches, ok := c.publication.replay(c.epoch, 1); !ok || len(batches) != 0 {
		t.Fatal("sequência final não confirmada")
	}
	if c.publication.itemBytes > canonicalSourceBytes || c.storagePublicationErr != nil {
		t.Fatal("recuperação sem cota ou incompleta")
	}
	for _, id := range ids {
		if c.turns[strings.TrimPrefix(id, "turn:")].streams[0].parts[0].text.Len() != 1024*1024 {
			t.Fatal("buffer alterado")
		}
	}
}

func TestSelectionBudgetsQuestionScopeWithoutContent(t *testing.T) {
	f := newSnapshotFixture(t)
	var refs []string
	parent := strings.Repeat("p", 1024*1024)
	for i := 0; i < 9; i++ {
		question := database.ChatMessage{ConversationID: f.conversation, ParentID: &parent, Role: "user", Content: "curto"}
		if err := f.db.Create(&question).Error; err != nil {
			t.Fatal(err)
		}
		refs = append(refs, "turn:"+question.ID)
	}
	if _, err := f.reader.ReadSelection(f.ctx, f.conversation, refs); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal("escopos agregados fora da cota", err)
	}
	id := strings.TrimPrefix(refs[0], "turn:")
	if err := f.db.Model(&database.ChatMessage{}).Where("id=?", id).Update("parent_id", strings.Repeat("p", canonicalSourceBytes+1)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadPublicationTurn(f.ctx, f.conversation, id); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal("escopo individual fora da cota", err)
	}
}

func TestThreadValidationDoesNotReadUnusedTurnID(t *testing.T) {
	f := newSnapshotFixture(t)
	if err := f.db.Model(&database.ChatMessage{}).Where("id=?", f.question).Update("turn_id", strings.Repeat("x", canonicalSourceBytes+1)).Error; err != nil {
		t.Fatal(err)
	}
	scopes := 0
	if err := f.db.Callback().Query().After("gorm:query").Register("thread_scope", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*database.MessageScope); ok {
			scopes++
			if strings.Contains(strings.Join(tx.Statement.Selects, ","), "turn_id") {
				t.Error("vínculo desnecessário lido")
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.reader.ReadCanonicalWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, ParentID: &f.question, Limit: 1})
	if err != nil || len(snapshot.Items) != 0 || scopes != 2 {
		t.Fatal("validação leve não exercitada", err, scopes)
	}
}
