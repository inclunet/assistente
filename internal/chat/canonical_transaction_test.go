package chat

import (
	"assistente/internal/database"
	"context"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func TestCanonicalRollbackPreservesPendingStreamFlush(t *testing.T) {
	for _, finished := range []bool{false, true} {
		t.Run(strconv.FormatBool(finished), func(t *testing.T) {
			f, p, stream, _ := automaticFixture(t)
			c := stream.conversation
			// Agenda de forma determinística: o teste entrega o callback após o
			// rollback, sem depender de uma corrida com os 24 ms do coalescing.
			c.mu.Lock()
			c.stopPublicationTimerLocked()
			timer := time.NewTimer(time.Hour)
			defer timer.Stop()
			c.publicationTimer = timer
			generation := c.publicationGeneration
			c.mu.Unlock()
			if !stream.StartStream(0).Append("delta pendente") {
				t.Fatal("stream recusou delta")
			}
			c.mu.Lock()
			turn := c.turns[stream.turnID]
			turn.finished = finished
			if finished {
				// Terminal sem persistência ainda tem conteúdo a publicar.
				turn.volatile, turn.state = true, database.TurnInterrupted
			}
			published := c.publication.fork()
			ctx, cancel := context.WithCancel(f.ctx)
			finish := c.beginCanonicalPublication()
			c.flushPublicationLocked(f.conversation)
			cancel() // Mesma fronteira final de SetRetention, antes do commit.
			finish(f.conversation, ctx.Err() == nil)
			preserved := turn.publicationDirty && turn.publicationBase != nil && c.publicationTimer == timer && c.publicationGeneration == generation && reflect.DeepEqual(published, c.publication)
			c.mu.Unlock()
			if !preserved {
				t.Fatal("rollback consumiu delta, base, journal ou agendamento")
			}
			p.flushScheduledPublication(stream.key, c, generation)
			item, sequence := publicationItem(t, stream)
			if canonicalText(item) != "delta pendente" || sequence <= published.sequence {
				t.Fatal("callback original não publicou delta após rollback", item, sequence)
			}
			c.mu.Lock()
			batches, ok := c.publication.replay(c.epoch, published.sequence)
			consumed := !turn.publicationDirty && c.publicationTimer == nil && (!finished || turn.publicationBase == nil)
			c.mu.Unlock()
			if !ok || len(batches) == 0 || !consumed {
				t.Fatal("delta ausente do replay ou pendência não consumida no commit")
			}
		})
	}
}
