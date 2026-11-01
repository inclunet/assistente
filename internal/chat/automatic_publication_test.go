package chat

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type countedPublicationReader struct {
	*DBTimelineReader
	reads atomic.Int64
	fail  atomic.Bool
	block atomic.Bool
}

func (r *countedPublicationReader) ReadPublicationTurn(ctx context.Context, conversationID, turnID string) (*TimelineTurnSnapshot, error) {
	r.reads.Add(1)
	if r.block.Load() {
		if _, ok := ctx.Deadline(); !ok {
			return nil, errors.New("deadline ausente")
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if r.fail.Load() {
		return nil, errors.New("leitura indisponível")
	}
	return r.DBTimelineReader.ReadPublicationTurn(ctx, conversationID, turnID)
}

func automaticFixture(t *testing.T) (snapshotFixture, *LiveProjection, *ExecutionProjection, *countedPublicationReader) {
	t.Helper()
	f := newSnapshotFixture(t)
	r := &countedPublicationReader{DBTimelineReader: f.reader}
	p := NewLiveProjection(r)
	t.Cleanup(func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		for _, c := range p.conversations {
			c.mu.Lock()
			if c.timer != nil {
				c.timer.Stop()
			}
			if c.publicationTimer != nil {
				c.publicationTimer.Stop()
			}
			c.mu.Unlock()
		}
		p.conversations = make(map[projectionKey]*projectionConversation)
	})
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	repo := database.NewChatTurnRepository(f.db)
	e, err := p.Admit(f.ctx, f.conversation, f.question, id.String(), func() (database.ChatTimelineVersion, error) {
		version, err := f.reader.ReadVersion(f.ctx, f.conversation)
		if err != nil {
			return version, err
		}
		_, err = repo.BeginExecution(f.ctx, f.conversation, f.question, "", database.TurnModeRetry, id.String(), "")
		return version, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return f, p, e, r
}

func publicationItem(t *testing.T, e *ExecutionProjection) (CanonicalItem, uint64) {
	t.Helper()
	c := e.conversation
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.publicationErrorLocked(); err != nil {
		t.Fatal(err)
	}
	return c.publication.items["turn:"+e.turnID], c.publication.sequence
}

func canonicalText(item CanonicalItem) string {
	var text strings.Builder
	for _, segment := range item.Segments {
		if segment.Kind == "text" {
			text.WriteString(segment.Text)
		}
	}
	return text.String()
}

func TestAutomaticPublicationAdmitsAndCoalescesWithoutTokenReads(t *testing.T) {
	_, _, e, reader := automaticFixture(t)
	item, _ := publicationItem(t, e)
	if item.State != database.TurnPreparing || item.RepresentativeMessageID != nil {
		t.Fatalf("admissão não publicada: %+v", item)
	}
	before := reader.reads.Load()
	s := e.StartStream(0)
	for range 1000 {
		s.Append("x")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		item, _ = publicationItem(t, e)
		if canonicalText(item) == strings.Repeat("x", 1000) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("tokens não publicados automaticamente")
		}
		time.Sleep(time.Millisecond)
	}
	if reader.reads.Load() != before {
		t.Fatal("token consultou storage")
	}
	if item.ItemID != "turn:"+e.turnID || item.State != database.TurnStreaming {
		t.Fatal("identidade/estado")
	}
}

func TestAutomaticPublicationTerminalAbsorbsScheduledFlush(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	repo := database.NewChatTurnRepository(f.db)
	id, err := e.AssignResponse(func() (string, error) { return repo.EnsureResponse(f.ctx, f.conversation, f.question, e.executionID) })
	if err != nil {
		t.Fatal(err)
	}
	s := e.StartStream(0)
	s.Append("final")
	c := e.conversation
	c.mu.Lock()
	generation := c.publicationGeneration
	c.mu.Unlock()
	if err := e.CommitResponse(database.TurnCompleted, "completed", "final", "", func() (database.ChatTimelineVersion, error) {
		return repo.FinishResponseWithVersion(f.ctx, f.conversation, f.question, e.executionID, database.TurnCompleted, "completed", id, &database.MessageOptions{Content: "final"})
	}); err != nil {
		t.Fatal(err)
	}
	item, sequence := publicationItem(t, e)
	if canonicalText(item) != "final" || item.State != database.TurnCompleted || item.Durability != "persisted" || item.RepresentativeMessageID == nil || *item.RepresentativeMessageID != id {
		t.Fatalf("terminal não publicado antes de retornar: %+v", item)
	}
	p.flushScheduledPublication(e.key, c, generation) // callback que já estava na fila
	_, after := publicationItem(t, e)
	if after != sequence || s.Append("tardio") {
		t.Fatal("terminal aceitou publicação tardia")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.publicationTimer != nil || c.turns[e.turnID].publicationBase != nil {
		t.Fatal("terminal reteve timer/base")
	}
}

func TestAutomaticPublicationEmptyStreamKeepsPersistedProviderError(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	repo := database.NewChatTurnRepository(f.db)
	id, err := e.AssignResponse(func() (string, error) { return repo.EnsureResponse(f.ctx, f.conversation, f.question, e.executionID) })
	if err != nil {
		t.Fatal(err)
	}
	e.StartStream(0)
	const reason = "Falha na resposta do agente: provedor indisponível"
	if err := repo.WritePartial(f.ctx, e.executionID, id, reason, "", true); err != nil {
		t.Fatal(err)
	}
	if err := e.CommitTerminal(database.TurnFailed, "provider_error", func() (database.ChatTimelineVersion, error) {
		return repo.FinishResponseWithVersion(f.ctx, f.conversation, f.question, e.executionID, database.TurnFailed, "provider_error", "", nil)
	}); err != nil {
		t.Fatal(err)
	}
	item, _ := publicationItem(t, e)
	if item.Durability != "persisted" || item.State != database.TurnFailed || canonicalText(item) != reason {
		t.Fatal("flush substituiu motivo persistido por stream vazio", item)
	}
	recovered, err := p.ReadRecovery(f.ctx, f.conversation, f.question, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Item.Durability != item.Durability || canonicalText(canonicalItem(recovered.Item)) != reason {
		t.Fatal("recovery divergiu do journal")
	}
}

func TestAutomaticPublicationFailedTerminalIsVolatileWithoutPendingContent(t *testing.T) {
	for _, confirmed := range []bool{false, true} {
		t.Run(map[bool]string{false: "sem stream", true: "stream confirmado"}[confirmed], func(t *testing.T) {
			f, p, e, _ := automaticFixture(t)
			repo := database.NewChatTurnRepository(f.db)
			id, err := e.AssignResponse(func() (string, error) { return repo.EnsureResponse(f.ctx, f.conversation, f.question, e.executionID) })
			if err != nil {
				t.Fatal(err)
			}
			if confirmed {
				e.StartStream(0).Append("conteúdo salvo")
				if err := e.CommitContent(0, "conteúdo salvo", "", func() (string, error) {
					return id, repo.WritePartial(f.ctx, e.executionID, id, "conteúdo salvo", "", false)
				}); err != nil {
					t.Fatal(err)
				}
			}
			failure := errors.New("falha de escrita terminal")
			if err := e.CommitTerminal(database.TurnFailed, "provider_error", func() (database.ChatTimelineVersion, error) { return database.ChatTimelineVersion{}, failure }); !errors.Is(err, failure) {
				t.Fatal(err)
			}
			item, _ := publicationItem(t, e)
			if item.Durability != "volatile" || item.State != database.TurnFailed {
				t.Fatal("terminal não gravado anunciado como persistido", item)
			}
			recovered, err := p.ReadRecovery(f.ctx, f.conversation, f.question, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			if recovered.Item.Durability != "volatile" || recovered.Item.State != database.TurnFailed {
				t.Fatal("recovery anunciou persistência inexistente")
			}
		})
	}
}

func TestAutomaticPublicationReadFailurePreservesCommitAndRecoversNewerBuffers(t *testing.T) {
	f, p, e, reader := automaticFixture(t)
	subscription, snapshot, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "publication-test"}, "history", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	repo := database.NewChatTurnRepository(f.db)
	s := e.StartStream(0)
	s.Append("antes")
	reader.fail.Store(true)
	id, err := e.AssignResponse(func() (string, error) { return repo.EnsureResponse(f.ctx, f.conversation, f.question, e.executionID) })
	if err != nil || id == "" {
		t.Fatalf("writer bem-sucedido virou erro: %v", err)
	}
	c := e.conversation
	c.mu.Lock()
	epoch := c.epoch
	if !errors.Is(c.publicationErrorLocked(), ErrTimelinePublication) || c.turns[e.turnID].publicationBase != nil {
		t.Fatal("base antiga ainda utilizável após erro")
	}
	c.mu.Unlock()
	if batches, ok, err := subscription.Replay(f.ctx, snapshot.Epoch, 0, f.reader); !errors.Is(err, ErrTimelinePublication) || ok || len(batches) != 0 {
		t.Fatalf("replay serviu base inválida: %v %v %v", batches, ok, err)
	}
	s.Append(" depois")
	_, _ = e.AssignResponse(func() (string, error) { return id, nil })
	c.mu.Lock()
	if epoch != c.epoch {
		t.Fatal("falha repetida girou época")
	}
	c.mu.Unlock()
	reader.fail.Store(false)
	recovery, err := p.ReadRecovery(f.ctx, f.conversation, f.question, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	item, _ := publicationItem(t, e)
	if canonicalText(item) != "antes depois" || canonicalText(canonicalItem(recovery.Item)) != "antes depois" {
		t.Fatal("recuperação perdeu bytes mais novos")
	}
}

func TestAutomaticPublicationRecoveryFlushesOtherFinishedTurns(t *testing.T) {
	for _, name := range []string{"turn", "turn-absent", "window", "discard", "legacy-window"} {
		t.Run(name, func(t *testing.T) {
			f, p, e, reader := automaticFixture(t)
			repo := database.NewChatTurnRepository(f.db)
			id, err := e.AssignResponse(func() (string, error) { return repo.EnsureResponse(f.ctx, f.conversation, f.question, e.executionID) })
			if err != nil {
				t.Fatal(err)
			}
			reader.fail.Store(true)
			if err := e.CommitResponse(database.TurnCompleted, "completed", "final", "", func() (database.ChatTimelineVersion, error) {
				return repo.FinishResponseWithVersion(f.ctx, f.conversation, f.question, e.executionID, database.TurnCompleted, "completed", id, &database.MessageOptions{Content: "final"})
			}); err != nil {
				t.Fatal(err)
			}
			c := e.conversation
			c.mu.Lock()
			other := &liveTurn{turnID: "other", incarnation: c.incarnation, finished: true, state: database.TurnCompleted}
			c.turns["other"] = other
			c.setPublicationBaseLocked(other, &TimelineItem{ItemID: "turn:other", TurnID: "other", Kind: "response", State: database.TurnCompleted, Durability: "persisted", Segments: []ResponseSegment{{Kind: "text", SegmentID: "other-text", Text: "aguardando"}}})
			c.flushPublicationLocked(f.conversation)
			c.mu.Unlock()
			// Simula vencimento da retenção antes de liberar o bloqueador. Tanto
			// A (erro) quanto B (terminal sujo) ainda precisam de reconciliação.
			p.mu.Lock()
			c.mu.Lock()
			if c.timer != nil {
				c.timer.Stop()
				c.timer = nil
			}
			p.pruneLocked(e.key, c, time.Now().Add(2*projectionRetention))
			kept := p.conversations[e.key] == c && c.turns[e.turnID] != nil && c.turns["other"] == other
			c.mu.Unlock()
			p.mu.Unlock()
			if !kept {
				t.Fatal("retenção descartou publicação pendente")
			}
			reader.fail.Store(false)
			denied := errors.New("leitura recusada")
			if err := p.DiscardRetained(f.ctx, f.conversation, f.question, e.executionID, recoveryReader{func() (*TimelineTurnSnapshot, error) { return nil, denied }}); !errors.Is(err, denied) {
				t.Fatal(err)
			}
			if _, err := p.ReadWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation}, windowRecoveryReader(func(context.Context, database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
				return nil, denied
			})); !errors.Is(err, denied) {
				t.Fatal(err)
			}
			c.mu.Lock()
			_, published := c.publication.items["turn:other"]
			c.mu.Unlock()
			if published {
				t.Fatal("leitura recusada liberou publicação")
			}
			switch name {
			case "window":
				_, err = p.ReadCanonicalWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}, f.reader)
			case "turn":
				_, err = p.ReadRecovery(f.ctx, f.conversation, f.question, f.reader)
			case "turn-absent":
				_, err = p.ReadRecovery(f.ctx, f.conversation, f.question, recoveryReader{func() (*TimelineTurnSnapshot, error) {
					version, err := f.reader.ReadVersion(f.ctx, f.conversation)
					return &TimelineTurnSnapshot{Version: version}, err
				}})
			case "discard":
				err = p.DiscardRetained(f.ctx, f.conversation, f.question, e.executionID, f.reader)
			case "legacy-window":
				// A janela autorizada comprova substituição de A; B está fora dela.
				_, err = p.ReadWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation}, windowRecoveryReader(func(context.Context, database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
					version, err := f.reader.ReadVersion(f.ctx, f.conversation)
					return &TimelineWindowSnapshot{Version: version, TurnIDs: []string{f.question}, Turns: map[string]*database.ChatTurn{f.question: {CurrentExecutionID: refString("replacement")}}}, err
				}))
			}
			if err != nil {
				t.Fatal(err)
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			current := c.turns[e.turnID]
			if canonicalText(c.publication.items["turn:other"]) != "aguardando" || other.publicationBase != nil || (current != nil && current.publicationBase != nil) {
				t.Fatal("recuperação deixou publicação/base terminal pendente")
			}
		})
	}
}

func TestAutomaticPublicationNewIncarnationInvalidatesOffPageTurns(t *testing.T) {
	for _, mode := range []string{"window", "admit"} {
		t.Run(mode, func(t *testing.T) {
			f, p, e, _ := automaticFixture(t)
			stream := e.StartStream(0)
			stream.Append("conteúdo da encarnação anterior")
			c := e.conversation
			c.mu.Lock()
			if c.publicationTimer != nil {
				c.publicationTimer.Stop()
				c.publicationTimer = nil
				c.publicationGeneration++
			}
			old := c.turns[e.turnID]
			old.publicationDirty = true
			c.turns["off-page-blocker"] = &liveTurn{turnID: "off-page-blocker", incarnation: c.incarnation, publicationErr: errors.New("falha antiga")}
			before := c.epoch
			c.mu.Unlock()
			question, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, Role: "user", Content: "nova pergunta"})
			if err != nil {
				t.Fatal(err)
			}
			// O storage informa uma nova encarnação, como após recriação/importação.
			if err := f.db.Exec("UPDATE chat_timeline_versions SET incarnation=? WHERE conversation_id=?", strings.ReplaceAll(uuid.NewString(), "-", ""), f.conversation).Error; err != nil {
				t.Fatal(err)
			}
			if mode == "window" {
				_, err = p.ReadCanonicalWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 1}, f.reader)
			} else {
				id, uuidErr := uuid.NewV7()
				if uuidErr != nil {
					t.Fatal(uuidErr)
				}
				_, err = p.Admit(f.ctx, f.conversation, question.ID, id.String(), func() (database.ChatTimelineVersion, error) {
					version, err := f.reader.ReadVersion(f.ctx, f.conversation)
					if err != nil {
						return version, err
					}
					_, err = database.NewChatTurnRepository(f.db).BeginExecution(f.ctx, f.conversation, question.ID, "", database.TurnModeRetry, id.String(), "")
					return version, err
				})
			}
			if err != nil {
				t.Fatal(err)
			}
			if stream.Append("atrasado") {
				t.Fatal("callback antigo aceito")
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if before == c.epoch || c.publicationErrorLocked() != nil {
				t.Fatal("encarnação não invalidou falha antiga")
			}
			if _, exists := c.publication.items["turn:"+f.question]; exists {
				t.Fatal("conteúdo anterior publicado na nova época")
			}
			if !old.detached || old.publicationBase != nil || old.publicationDirty {
				t.Fatal("base antiga retida")
			}
			retained := snapshotLiveTurn(c, old)
			if len(retained.Streams) != 1 || testLiveSegment(retained.Streams[0], "text", 0).Text != "conteúdo da encarnação anterior" {
				t.Fatal("bytes recuperáveis perdidos")
			}
		})
	}
}

func TestAutomaticPublicationReadBudgetAndMediaExclusion(t *testing.T) {
	f := newSnapshotFixture(t)
	large := strings.Repeat("x", canonicalSourceBytes+1)
	if err := f.db.Model(&database.ChatMessage{}).Where("id = ?", f.question).Updates(map[string]any{"audio": large, "media": large}).Error; err != nil {
		t.Fatal(err)
	}
	questionRead := false
	if err := f.db.Callback().Query().After("gorm:query").Register("publication_question_columns", func(tx *gorm.DB) {
		if message, ok := tx.Statement.Dest.(*database.ChatMessage); ok && message.ID == f.question {
			questionRead = true
			if len(tx.Statement.Selects) != 4 || message.Media != "" || message.Audio != "" {
				t.Error("pergunta carregou payload em vez de metadados")
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&database.ChatMessage{}).Where("id = ?", f.answer).Updates(map[string]any{"audio": large, "media": large}).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.reader.ReadPublicationTurn(f.ctx, f.conversation, f.question)
	if err != nil {
		t.Fatal(err)
	}
	if !questionRead {
		t.Fatal("metadados da pergunta não verificados")
	}
	if snapshot.Response.Representative.Audio != "" || snapshot.Response.Representative.Media != "" {
		t.Fatal("leitura automática materializou mídia")
	}
	if err := f.db.Model(&database.ChatMessage{}).Where("id = ?", f.answer).Update("content", large).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadPublicationTurn(f.ctx, f.conversation, f.question); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatalf("payload excedente: %v", err)
	}
	if err := f.db.Model(&database.ChatMessage{}).Where("id = ?", f.answer).Update("content", "curto").Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&database.ToolInvocation{}).Where("id = ?", f.invocation).Update("output_preview", large).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadPublicationTurn(f.ctx, f.conversation, f.question); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatalf("ledger excedente: %v", err)
	}
	if err := f.db.Model(&database.ToolInvocation{}).Where("id = ?", f.invocation).Update("conversation_id", nil).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadPublicationTurn(f.ctx, f.conversation, f.question); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatalf("ledger legado excedente: %v", err)
	}
}

func TestAutomaticPublicationVolatileTerminalMatchesRecovery(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	e.StartStream(0).Append("preservar")
	failure := errors.New("writer indisponível")
	if err := e.CommitTerminal(database.TurnInterrupted, "cancelled", func() (database.ChatTimelineVersion, error) { return database.ChatTimelineVersion{}, failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	item, _ := publicationItem(t, e)
	if canonicalText(item) != "preservar" || item.State != database.TurnFailed || item.Durability != "volatile" {
		t.Fatalf("terminal volátil: %+v", item)
	}
	r, err := p.ReadRecovery(f.ctx, f.conversation, f.question, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := publicationItem(t, e)
	if canonicalText(after) != "preservar" || canonicalText(canonicalItem(r.Item)) != "preservar" {
		t.Fatal("leitura substituiu terminal volátil")
	}
}

func TestAutomaticPublicationReadDeadlineDoesNotUndoWriter(t *testing.T) {
	f, _, e, reader := automaticFixture(t)
	reader.block.Store(true)
	repo := database.NewChatTurnRepository(f.db)
	id, err := e.AssignResponse(func() (string, error) { return repo.EnsureResponse(f.ctx, f.conversation, f.question, e.executionID) })
	if err != nil || id == "" {
		t.Fatalf("writer confirmado perdido: %s %v", id, err)
	}
	c := e.conversation
	c.mu.Lock()
	publicationErr := c.publicationErrorLocked()
	c.mu.Unlock()
	if !errors.Is(publicationErr, context.DeadlineExceeded) {
		t.Fatalf("leitura não limitada: %v", publicationErr)
	}
	stored, err := f.reader.ReadTurn(f.ctx, f.conversation, f.question)
	if err != nil || stored.Response.Representative.ID != id {
		t.Fatalf("commit alterado: %v", err)
	}
}

func TestAutomaticPublicationDetachedFailureDoesNotBlockOtherTurn(t *testing.T) {
	testDetachedPublicationFailure(t, false)
}

func TestAutomaticPublicationConfirmedFailureDoesNotBlockAfterReplacement(t *testing.T) {
	testDetachedPublicationFailure(t, true)
}

func testDetachedPublicationFailure(t *testing.T, confirmed bool) {
	t.Helper()
	f, p, e, reader := automaticFixture(t)
	repo := database.NewChatTurnRepository(f.db)
	responseID, err := e.AssignResponse(func() (string, error) { return repo.EnsureResponse(f.ctx, f.conversation, f.question, e.executionID) })
	if err != nil {
		t.Fatal(err)
	}
	e.StartStream(0).Append("bytes retidos")
	reader.fail.Store(true)
	if confirmed {
		if err := e.CommitResponse(database.TurnCompleted, "completed", "bytes retidos", "", func() (database.ChatTimelineVersion, error) {
			return repo.FinishResponseWithVersion(f.ctx, f.conversation, f.question, e.executionID, database.TurnCompleted, "completed", responseID, &database.MessageOptions{Content: "bytes retidos"})
		}); err != nil {
			t.Fatal(err)
		}
	} else {
		_, _ = e.AssignResponse(func() (string, error) { return responseID, nil })
	}
	reader.fail.Store(false)
	if !confirmed {
		if err := repo.Finish(f.ctx, f.conversation, f.question, e.executionID, database.TurnFailed, "internal_error"); err != nil {
			t.Fatal(err)
		}
	}
	replacement, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.BeginExecution(f.ctx, f.conversation, f.question, "", database.TurnModeRetry, replacement.String(), ""); err != nil {
		t.Fatal(err)
	}
	r, err := p.ReadRecovery(f.ctx, f.conversation, f.question, f.reader)
	if err != nil || !r.RecoveryRequired {
		t.Fatalf("desvinculação perdeu recuperação: %+v %v", r, err)
	}
	if !confirmed && (len(r.Retained) != 1 || testLiveSegment(r.Retained[0].Streams[0], "text", 0).Text != "bytes retidos") {
		t.Fatal("bytes voláteis perdidos")
	}
	question, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, Role: "user", Content: "outro turno"})
	if err != nil {
		t.Fatal(err)
	}
	nextID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	next, err := p.Admit(f.ctx, f.conversation, question.ID, nextID.String(), func() (database.ChatTimelineVersion, error) {
		version, err := f.reader.ReadVersion(f.ctx, f.conversation)
		if err != nil {
			return version, err
		}
		_, err = repo.BeginExecution(f.ctx, f.conversation, question.ID, "", database.TurnModeNew, nextID.String(), "")
		return version, err
	})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := publicationItem(t, next)
	if item.ItemID != "turn:"+question.ID || item.State != database.TurnPreparing {
		t.Fatal("turno desvinculado bloqueou publicação de outro")
	}
}

func TestAutomaticPublicationOldTimerCannotTouchReplacement(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	e.StartStream(0).Append("antigo")
	c := e.conversation
	c.mu.Lock()
	generation := c.publicationGeneration
	c.mu.Unlock()
	p.mu.Lock()
	delete(p.conversations, e.key)
	p.mu.Unlock()
	other, release := p.lockConversation(e.key, true)
	other.windows = map[string]*subscribedWindow{"window": {}}
	other.publishCanonical(f.conversation, []*TimelineItem{{ItemID: "message:other", ConversationID: f.conversation, Kind: "message"}})
	sequence := other.publication.sequence
	release()
	p.flushScheduledPublication(e.key, c, generation)
	other.mu.Lock()
	defer other.mu.Unlock()
	if other.publication.sequence != sequence {
		t.Fatal("callback antigo tocou nova instância")
	}
}

func TestAutomaticPublicationCancellationAndRetryKeepItemIdentity(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	repo := database.NewChatTurnRepository(f.db)
	s := e.StartStream(0)
	s.Append("interrompido")
	if err := e.CommitTerminal(database.TurnInterrupted, "cancelled", func() (database.ChatTimelineVersion, error) {
		return repo.FinishResponseWithVersion(f.ctx, f.conversation, f.question, e.executionID, database.TurnInterrupted, "cancelled", "", nil)
	}); err != nil {
		t.Fatal(err)
	}
	before, _ := publicationItem(t, e)
	if before.State != database.TurnInterrupted || canonicalText(before) != "interrompido" {
		t.Fatal("cancelamento perdeu conteúdo")
	}
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	next, err := p.Admit(f.ctx, f.conversation, f.question, id.String(), func() (database.ChatTimelineVersion, error) {
		version, err := f.reader.ReadVersion(f.ctx, f.conversation)
		if err != nil {
			return version, err
		}
		_, err = repo.BeginExecution(f.ctx, f.conversation, f.question, "", database.TurnModeRetry, id.String(), "")
		return version, err
	})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := publicationItem(t, next)
	if after.ItemID != before.ItemID || after.State != database.TurnPreparing || canonicalText(after) != "" || s.Append("atrasado") {
		t.Fatalf("retry não substituiu execução: %+v", after)
	}
}

func TestAutomaticPublicationSourceBudgetExcludesMedia(t *testing.T) {
	c := &projectionConversation{epoch: "e", turns: make(map[string]*liveTurn)}
	target := &liveTurn{}
	c.turns["q"] = target
	message := Message{Content: "texto", Audio: strings.Repeat("a", canonicalSourceBytes+1), Media: "mídia"}
	source := &TimelineItem{ItemID: "turn:q", messages: []Message{message}, Representative: &message, Segments: []ResponseSegment{{SegmentID: "s", Text: "texto"}}}
	c.setPublicationBaseLocked(target, source)
	if target.publicationErr != nil || target.publicationBase.Representative.Audio != "" || target.publicationBase.messages[0].Media != "" {
		t.Fatal("mídia retida na base")
	}
	if source.Representative.Audio == "" {
		t.Fatal("snapshot do chamador alterado")
	}
	source.Segments[0].Text = "alterado pelo chamador"
	if target.publicationBase.Segments[0].Text != "texto" {
		t.Fatal("base compartilha memória com snapshot")
	}
	source.messages[0].Content = strings.Repeat("x", canonicalSourceBytes+1)
	c.setPublicationBaseLocked(target, source)
	if !errors.Is(target.publicationErr, ErrTimelineSourceLimit) || target.publicationBase != nil {
		t.Fatal("base excedente retida")
	}
}
