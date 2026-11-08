package app

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/sqlite"
	"gorm.io/gorm"
)

// Complementa a fixture histórica sem mudar sua preparação nem seus limites.
// Cada amostra abre outro pool e outro leitor: inclui compilação inicial das
// consultas, primeira leitura e serialização. Não mede cache frio do SO/disco.
func BenchmarkCanonicalTimelineColdOpen(b *testing.B) {
	for _, count := range []int{100, 500, 1000} {
		b.Run(fmt.Sprintf("messages_%d", count), func(b *testing.B) {
			setupMessageWindowAppTestDB(b)
			conversation := seedMessageWindowBenchmark(b, count, 1024)
			path := filepath.Join(b.TempDir(), "cold.db")
			if err := database.DB().Exec("VACUUM INTO ?", path).Error; err != nil {
				b.Fatal(err)
			}
			query := database.MessageWindowQuery{ConversationID: conversation, Anchor: "end", Direction: "before", Limit: database.MaxMessageWindowRows}
			b.ResetTimer()
			for range b.N {
				db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
				if err != nil {
					b.Fatal(err)
				}
				raw, err := db.DB()
				if err != nil {
					b.Fatal(err)
				}
				controller := &messageWindowTestReader{chat.NewLiveProjection(nil), chat.NewDBTimelineReader(db)}
				window, err := controller.Read(messageWindowTestCtx(), query)
				if err != nil {
					_ = raw.Close()
					b.Fatal(err)
				}
				payload, err := json.Marshal(window)
				if err != nil {
					_ = raw.Close()
					b.Fatal(err)
				}
				b.ReportMetric(float64(len(payload)), "window_bytes")
				if err := raw.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
