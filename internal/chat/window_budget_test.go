package chat

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"assistente/internal/database"
	"gorm.io/gorm"
)

func TestCanonicalWindowBudgetBeforeMaterialization(t *testing.T) {
	for _, ledger := range []bool{false, true} {
		t.Run(map[bool]string{false: "messages", true: "ledger"}[ledger], func(t *testing.T) {
			f := newSnapshotFixture(t)
			large := strings.Repeat("x", 5*1024*1024)
			if err := f.db.Model(&database.ChatMessage{}).Where("id=?", f.question).Update("content", large).Error; err != nil {
				t.Fatal(err)
			}
			if ledger {
				if err := f.db.Model(&database.ToolInvocation{}).Where("id=?", f.invocation).Update("output_preview", large).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				if err := f.db.Model(&database.ChatMessage{}).Where("id=?", f.answer).Update("content", large).Error; err != nil {
					t.Fatal(err)
				}
			}
			payloadReads := 0
			if err := f.db.Callback().Query().After("gorm:query").Register("budget_payload", func(tx *gorm.DB) {
				if strings.Contains(tx.Statement.SQL.String(), "selected_payload") {
					payloadReads++
				}
			}); err != nil {
				t.Fatal(err)
			}
			query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 20}
			if snapshot, err := f.reader.ReadCanonicalWindow(f.ctx, query); !errors.Is(err, ErrTimelineSourceLimit) || snapshot != nil {
				t.Fatalf("excesso agregado aceito: %v", err)
			}
			if payloadReads != 0 {
				t.Fatal("conteúdo materializado antes da recusa", payloadReads)
			}
			// Cada item cabe sozinho. Uma pergunta não deve carregar ledger/resposta
			// fora da página, mesmo que o turno tenha conteúdo volumoso.
			query.Limit = 1
			first, err := f.reader.ReadCanonicalWindow(f.ctx, query)
			if err != nil || !reflect.DeepEqual(first.TurnIDs, []string{f.question}) || len(first.Responses) != 0 || len(first.Items) != 1 {
				t.Fatalf("pergunta isolada: %v", err)
			}
			query.Anchor, query.Direction = "end", "before"
			if _, err := f.reader.ReadCanonicalWindow(f.ctx, query); err != nil {
				t.Fatal("resposta isolada", err)
			}
			query.Limit = 20
			if _, err := f.reader.ReadWindow(f.ctx, query); err != nil {
				t.Fatal("legado mudou", err)
			}
		})
	}
}

func TestWindowBudgetRefusalPreservesSubscription(t *testing.T) {
	for _, mode := range []string{"persisted", "live", "mixed"} {
		t.Run(mode, func(t *testing.T) {
			live := mode != "persisted"
			f, p, e, _ := automaticFixture(t)
			query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}
			sub, initial, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "budget"}, "surface", query, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			size := canonicalSourceBytes + 1
			if mode == "mixed" {
				size = canonicalSourceBytes/2 + 1
			}
			large := strings.Repeat("x", size)
			if mode == "mixed" {
				if err := f.db.Model(&database.ChatMessage{}).Where("id=?", f.question).Update("content", large).Error; err != nil {
					t.Fatal(err)
				}
				if _, err := f.reader.ReadCanonicalWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}); err != nil {
					t.Fatal("fontes persistidas cabem antes de somar live", err)
				}
			}
			c := sub.conversation
			c.mu.Lock()
			publishedBefore, err := json.Marshal(c.publication.items)
			sequenceBefore := c.publication.sequence
			c.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			if live {
				// Alimenta o buffer real sob a barreira sem agendar publicação;
				// o teste isola a leitura da janela do timer.
				c := e.conversation
				c.mu.Lock()
				part := &livePart{}
				part.text.WriteString(large)
				c.turns[e.turnID].streams[0] = &liveStream{parts: []*livePart{part}}
				c.mu.Unlock()
			} else {
				if err := f.db.Model(&database.ChatMessage{}).Where("id=?", f.question).Update("content", large).Error; err != nil {
					t.Fatal(err)
				}
			}
			beforeQuery := sub.query
			request := navigationFrom(initial)
			request.Limit = 20
			if _, err := sub.Navigate(f.ctx, request); !errors.Is(err, ErrTimelineSourceLimit) {
				t.Fatalf("navegação: %v", err)
			}
			position := request.TimelineWindowPosition
			if _, err := sub.Refresh(f.ctx, initial.WindowRevision, &position); !errors.Is(err, ErrTimelineSourceLimit) {
				t.Fatalf("refresh: %v", err)
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			w := c.windows[sub.windowID]
			if w.revision != 1 || len(w.itemIDs) != 1 || !w.itemIDs[initial.OrderedItemIDs[0]] || !reflect.DeepEqual(sub.query, beforeQuery) {
				t.Fatal("recusa alterou inscrição")
			}
			publishedAfter, err := json.Marshal(c.publication.items)
			if err != nil || string(publishedBefore) != string(publishedAfter) || c.publication.sequence != sequenceBefore {
				t.Fatal("recusa alterou janela publicada", err)
			}
			if c.publicationErrorLocked() != nil {
				t.Fatal("recusa local suspendeu publicação global")
			}
			if live && c.turns[e.turnID].streams[0].parts[0].text.String() != large {
				t.Fatal("recusa descartou buffer")
			}
		})
	}
}

func TestWindowCompositionBudgetAggregatesAndEscapes(t *testing.T) {
	c := &projectionConversation{turns: make(map[string]*liveTurn)}
	first := &TimelineItem{ItemID: "one", Kind: "message", Segments: []ResponseSegment{{Kind: "text", Text: strings.Repeat("\x00", 800000)}}}
	second := &TimelineItem{ItemID: "two", Kind: "message", Segments: []ResponseSegment{{Kind: "text", Text: strings.Repeat("\x00", 800000)}}}
	if !windowCompositionWithinBudget(c, []*TimelineItem{first}) || !windowCompositionWithinBudget(c, []*TimelineItem{second}) {
		t.Fatal("item isolado recusado")
	}
	if windowCompositionWithinBudget(c, []*TimelineItem{first, second}) {
		t.Fatal("cota reiniciada por item")
	}
	second.Segments = nil
	second.ItemID = strings.Repeat("i", canonicalSourceBytes)
	if windowCompositionWithinBudget(c, []*TimelineItem{second}) {
		t.Fatal("identificador não contabilizado")
	}
	// Buffers fora da página e tentativas retidas não entram na leitura.
	part := &livePart{}
	part.text.WriteString(strings.Repeat("x", canonicalSourceBytes+1))
	c.turns["outside"] = &liveTurn{streams: map[int]*liveStream{0: {parts: []*livePart{part}}}}
	c.retained = map[string]*liveTurn{"old": c.turns["outside"]}
	if !windowCompositionWithinBudget(c, []*TimelineItem{first}) {
		t.Fatal("buffer fora da página contaminou orçamento")
	}
}

func TestWindowBudgetCountsJSONEscapesWithoutTextCopy(t *testing.T) {
	for _, value := range []string{"plain", "ação", "<>&", "\x00\b\n\r\t\f", "\u2028\u2029", string([]byte{0xff, 0xfe}), "\"\\"} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		size := len(encoded) - 2
		budget := projectionBudget{remaining: size, exactStrings: true}
		if !budget.stringValue(value) || budget.remaining != 0 {
			t.Fatalf("escape %q: esperado %d, restante %d", value, size, budget.remaining)
		}
		budget.remaining = size - 1
		if budget.stringValue(value) {
			t.Fatalf("escape excedente aceito: %q", value)
		}
	}
}
