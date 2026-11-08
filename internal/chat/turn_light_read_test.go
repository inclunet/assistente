package chat

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"assistente/internal/database"
	"gorm.io/gorm"
)

func TestTurnReadersDoNotMaterializeMedia(t *testing.T) {
	f := newSnapshotFixture(t)
	before, err := f.reader.ReadRecoveryTurn(f.ctx, f.conversation, f.question)
	if err != nil {
		t.Fatal(err)
	}
	large := strings.Repeat("x", 2*1024*1024)

	for _, id := range []string{f.question, f.answer} {
		attachFixtureMedia(t, f.db, id, `[{"type":"application/octet-stream","data":"`+large+`"}]`)
		if err := database.SaveMessageAudioWithContext(f.ctx, id, large, "audio/wav"); err != nil {
			t.Fatal(err)
		}
	}
	for name, read := range map[string]func(context.Context, string, string) (*TimelineTurnSnapshot, error){"publication": f.reader.ReadPublicationTurn, "recovery": f.reader.ReadRecoveryTurn} {
		t.Run(name, func(t *testing.T) {
			queries := 0
			hook := "test:turn-light"
			if err := f.db.Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
				if messages, ok := tx.Statement.Dest.(*[]database.ChatMessage); ok {
					queries++
					sql := tx.Statement.SQL.String()
					if strings.Contains(sql, "audio") || strings.Contains(sql, "media") {
						t.Error("consulta de turno materializa blobs", sql)
					}
					for _, m := range *messages {
						if m.Audio != "" || m.Media != "" || m.AudioMimeType != "" {
							t.Error("blob carregado antes da projeção")
						}
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.db.Callback().Query().Remove(hook) }()
			if err := f.db.Callback().Row().After("gorm:row").Register(hook, func(tx *gorm.DB) {
				sql := tx.Statement.SQL.String()
				if strings.Contains(sql, "admitted_sources") {
					queries++
					if strings.Contains(sql, "audio") || strings.Contains(sql, "media") {
						t.Error("consulta de turno materializa blobs", sql)
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.db.Callback().Row().Remove(hook) }()
			after, err := read(f.ctx, f.conversation, f.question)
			if err != nil {
				t.Fatal(err)
			}
			if queries == 0 {
				t.Fatal("leitura de mensagens não observada")
			}
			for _, message := range after.Response.messages {
				if message.Audio != "" || message.Media != "" || message.AudioMimeType != "" {
					t.Error("blob carregado antes da projeção")
				}
			}
			if !reflect.DeepEqual(canonicalItem(before.Response), canonicalItem(after.Response)) || !reflect.DeepEqual(before.Response.TerminalPatch(), after.Response.TerminalPatch()) {
				t.Fatal("leitura leve alterou texto, ferramentas ou metadados")
			}
			if _, err := read(database.WithUserID(context.Background(), "outro"), f.conversation, f.question); err == nil {
				t.Fatal("leitura ignorou owner")
			}
			cancelled, cancel := context.WithCancel(f.ctx)
			cancel()
			if _, err := read(cancelled, f.conversation, f.question); !errors.Is(err, context.Canceled) {
				t.Fatal("cancelamento ignorado", err)
			}
		})
	}
	audio, mime, err := database.GetMessageAudioWithContext(f.ctx, f.answer)
	if err != nil || audio != large || mime != "audio/wav" {
		t.Fatal("playback perdeu o áudio", err)
	}
	stored := []database.ChatMessage{{UUIDModel: database.UUIDModel{ID: f.answer}}}
	err = database.LoadMessageMedia(f.ctx, f.db, stored)
	if err != nil || !equalFixtureMedia(stored[0].Media, `[{"type":"application/octet-stream","data":"`+large+`"}]`) {
		t.Fatal("mídia persistida alterada", err)
	}
}

func TestRecoveryTurnKeepsTextBeyondPublicationBudget(t *testing.T) {
	f := newSnapshotFixture(t)
	text := strings.Repeat("x", canonicalSourceBytes+1)
	if err := f.db.Model(&database.ChatMessage{}).Where("id=?", f.answer).Update("content", text).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadPublicationTurn(f.ctx, f.conversation, f.question); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal("publicação ignorou cota", err)
	}
	recovered, err := f.reader.ReadRecoveryTurn(f.ctx, f.conversation, f.question)
	if err != nil || recovered.Response == nil || recovered.Response.TerminalPatch().Message.Content != text {
		t.Fatal("recuperação explícita perdeu conteúdo confirmado", err)
	}
}

func BenchmarkRecoveryTurnMedia(b *testing.B) {
	for _, size := range []int{0, 2 * 1024 * 1024} {
		name := "without_media"
		if size > 0 {
			name = "with_media"
		}
		b.Run(name, func(b *testing.B) {
			f := newSnapshotFixture(b)
			payload := strings.Repeat("x", size)
			for _, id := range []string{f.question, f.answer} {
				attachFixtureMedia(b, f.db, id, `[{"type":"image/png","data":"`+payload+`"}]`)
				if err := database.SaveMessageAudioWithContext(f.ctx, id, payload, "audio/wav"); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				snapshot, err := f.reader.ReadRecoveryTurn(f.ctx, f.conversation, f.question)
				if err != nil {
					b.Fatal(err)
				}
				data, err := json.Marshal(snapshot.Response.TerminalPatch())
				if err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(float64(len(data)), "DTO-bytes")
			}
		})
	}
}
