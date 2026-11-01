package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"assistente/internal/database"
	"github.com/google/uuid"
)

func TestDiscardRetainedReconcilesPublishedVolatileItem(t *testing.T) {
	for _, mode := range []string{"persisted", "absent", "blocked", "old-attempt"} {
		t.Run(mode, func(t *testing.T) {
			f, p, e, _ := automaticFixture(t)
			repo := database.NewChatTurnRepository(f.db)
			id, err := e.AssignResponse(func() (string, error) { return repo.EnsureResponse(f.ctx, f.conversation, f.question, e.executionID) })
			if err != nil {
				t.Fatal(err)
			}
			stream := e.StartStream(0)
			stream.Append("persistido")
			if err := e.CommitContent(0, "persistido", "", func() (string, error) {
				return id, repo.WritePartial(f.ctx, e.executionID, id, "persistido", "", false)
			}); err != nil {
				t.Fatal(err)
			}
			stream.Append(" + descartado")
			failure := errors.New("falha terminal")
			if err := e.CommitTerminal(database.TurnFailed, "provider_error", func() (database.ChatTimelineVersion, error) { return database.ChatTimelineVersion{}, failure }); !errors.Is(err, failure) {
				t.Fatal(err)
			}
			c := e.conversation
			if mode == "old-attempt" {
				if err := repo.Finish(f.ctx, f.conversation, f.question, e.executionID, database.TurnFailed, "provider_error"); err != nil {
					t.Fatal(err)
				}
				nextID, err := uuid.NewV7()
				if err != nil {
					t.Fatal(err)
				}
				next, err := p.Admit(f.ctx, f.conversation, f.question, nextID.String(), func() (database.ChatTimelineVersion, error) {
					version, err := f.reader.ReadVersion(f.ctx, f.conversation)
					if err != nil {
						return version, err
					}
					_, err = repo.BeginExecution(f.ctx, f.conversation, f.question, "", database.TurnModeRetry, nextID.String(), "")
					return version, err
				})
				if err != nil {
					t.Fatal(err)
				}
				next.StartStream(0).Append("execução atual")
			}
			sub, _, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "discard"}, "history", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			c.mu.Lock()
			before := c.publication.sequence
			epoch := c.epoch
			c.mu.Unlock()
			denied := errors.New("acesso negado")
			if err := p.DiscardRetained(f.ctx, f.conversation, f.question, e.executionID, recoveryReader{func() (*TimelineTurnSnapshot, error) { return nil, denied }}); !errors.Is(err, denied) {
				t.Fatal(err)
			}
			c.mu.Lock()
			unchanged := c.publication.sequence == before && c.epoch == epoch
			c.mu.Unlock()
			if !unchanged {
				t.Fatal("recusa de autorização alterou journal")
			}
			if mode == "absent" {
				if err := f.db.Where("conversation_id=? AND role <> 'user'", f.conversation).Delete(&database.ChatMessage{}).Error; err != nil {
					t.Fatal(err)
				}
				if err := f.db.Where("user_message_id=?", f.question).Delete(&database.ChatTurn{}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if mode == "blocked" {
				c.mu.Lock()
				blocker := &liveTurn{turnID: "blocker", incarnation: c.incarnation}
				c.turns["blocker"] = blocker
				c.failPublicationLocked(blocker, errors.New("outro bloqueador"))
				c.mu.Unlock()
			}
			if err := p.DiscardRetained(f.ctx, f.conversation, f.question, e.executionID, f.reader); err != nil {
				t.Fatal(err)
			}
			c.mu.Lock()
			item, exists := c.publication.items["turn:"+f.question]
			batches, ok := c.publication.replay(epoch, before)
			after := c.publication.sequence
			newEpoch := c.epoch
			c.mu.Unlock()
			switch mode {
			case "persisted":
				if !exists || canonicalText(item) != "persistido" || item.Durability != "persisted" || !ok || len(batches) == 0 {
					t.Fatal("descarte não corrigiu item/replay", item, ok)
				}
				if replay, ok, err := sub.Replay(f.ctx, epoch, before, f.reader); err != nil || !ok || len(replay) == 0 {
					t.Fatal("assinante não recebeu correção", err)
				}
			case "absent":
				removed := false
				for _, batch := range batches {
					for _, op := range batch.Operations {
						removed = removed || (op.Kind == "item_remove" && op.ItemID == "turn:"+f.question)
					}
				}
				if exists || !ok || !removed {
					t.Fatal("ausência comprovada não publicou remoção")
				}
			case "old-attempt":
				if !exists || canonicalText(item) != "execução atual" || after != before || newEpoch != epoch {
					t.Fatal("descarte antigo alterou execução atual")
				}
			case "blocked":
				if exists || newEpoch == epoch || ok {
					t.Fatal("journal bloqueado reteve texto descartado")
				}
				if _, ok, err := sub.Replay(f.ctx, epoch, before, f.reader); ok || !errors.Is(err, ErrTimelinePublication) {
					t.Fatal("bloqueador não suspendeu replay", err)
				}
				_, err := p.ReadWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation}, windowRecoveryReader(func(context.Context, database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
					version, err := f.reader.ReadVersion(f.ctx, f.conversation)
					return &TimelineWindowSnapshot{Version: version, TurnIDs: []string{"blocker"}}, err
				}))
				if err != nil {
					t.Fatal(err)
				}
				fresh, err := sub.Refresh(f.ctx)
				if err != nil {
					t.Fatal(err)
				}
				for _, item := range fresh.Items {
					if strings.Contains(canonicalText(item), "descartado") {
						t.Fatal("texto descartado reapareceu após recuperação")
					}
				}
				c.mu.Lock()
				after = c.publication.sequence
				c.mu.Unlock()
			}
			if err := p.DiscardRetained(f.ctx, f.conversation, f.question, e.executionID, f.reader); err != nil {
				t.Fatal(err)
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.publication.sequence != after {
				t.Fatal("descarte repetido não foi idempotente")
			}
		})
	}
}
