package toolinvocations

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Measures the production repository against a disk-backed ledger. Payloads
// remain in SQLite: loading summaries must not scale with unrelated history.
func BenchmarkToolSummaryProjection(b *testing.B) {
	for _, history := range []int{1000, 100000} {
		b.Run(fmt.Sprintf("history_%d", history), func(b *testing.B) {
			db, err := gorm.Open(sqlite.Open(filepath.Join(b.TempDir(), "ledger.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				b.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = sqlDB.Close() })
			if err := db.AutoMigrate(&database.Conversation{}, &database.ChatMessage{}, &database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
				b.Fatal(err)
			}
			if err := database.MigrateChatTurns(db); err != nil {
				b.Fatal(err)
			}
			if err := database.MigrateChatExecutionContent(db); err != nil {
				b.Fatal(err)
			}
			// Same index installed by the v39 migration; migration correctness has
			// dedicated coverage in database/migration_chat_tool_links_test.go.
			if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_tool_invocations_chat_turn ON tool_invocations(user_id, origin_type, turn_id, queued_at, id)").Error; err != nil {
				b.Fatal(err)
			}
			conv := database.Conversation{UUIDModel: database.UUIDModel{ID: "bench-conversation"}, UserID: "bench-user"}
			catalog := database.ToolCatalog{Name: "bench-tool", Origin: "builtin"}
			if err := db.Create(&conv).Error; err != nil {
				b.Fatal(err)
			}
			if err := db.Create(&catalog).Error; err != nil {
				b.Fatal(err)
			}
			turns := make([]string, 20)
			for i := 0; i <= len(turns); i++ {
				id := fmt.Sprintf("turn-%d", i)
				if i < len(turns) {
					turns[i] = id
				}
				if err := db.Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: id}, ConversationID: conv.ID, Role: "user"}).Error; err != nil {
					b.Fatal(err)
				}
			}
			payload := `{"content":"` + strings.Repeat("x", 4096) + `"}`
			if err := db.Transaction(func(tx *gorm.DB) error {
				for start := 0; start < history; start += 100 {
					batch := make([]database.ToolInvocation, 0, 100)
					for i := start; i < start+100 && i < history; i++ {
						turn := "turn-20"
						if i < len(turns) {
							turn = turns[i]
						}
						batch = append(batch, database.ToolInvocation{
							UUIDModel: database.UUIDModel{ID: fmt.Sprintf("inv-%09d", i)}, UserID: conv.UserID,
							ToolCatalogID: catalog.ID, OriginType: OriginChat, OriginID: turn, ConversationID: &conv.ID, TurnID: &turn,
							ToolCallID: fmt.Sprintf("call-%d", i), Status: StatusSucceeded, Attempt: 1,
							Output: payload, OutputPreview: "resultado", OutputBytes: int64(len(payload)), ResultAvailability: "available",
							QueuedAt: time.Unix(int64(i), 0),
						})
					}
					if err := tx.Create(&batch).Error; err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				b.Fatal(err)
			}
			benchmarkSummaryRead(b, db, conv.UserID, turns, 20)
		})
	}
}

// Optional local measurement: no SQL writes, migrations or explicit checkpoint.
// SQLite may coordinate readers through WAL auxiliary files and locks.
// Results expose timings/counts, never chat content.
func BenchmarkToolSummaryProjectionReadOnly(b *testing.B) {
	path := os.Getenv("ASSISTENTE_PROJECTION_BENCH_DB")
	if path == "" {
		b.Skip("set ASSISTENTE_PROJECTION_BENCH_DB to an existing database")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		b.Fatal(err)
	}
	uriPath := filepath.ToSlash(absolute)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	uri := url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=ro"}
	db, err := gorm.Open(sqlite.Open(uri.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		b.Fatalf("cannot open read-only benchmark database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Exec("PRAGMA query_only=ON").Error; err != nil {
		b.Fatal(err)
	}
	var sample []struct {
		UserID string
		TurnID string
	}
	if err := db.Raw(`SELECT user_id, turn_id FROM tool_invocations WHERE origin_type='chat' AND turn_id IS NOT NULL AND trim(tool_call_id)<>'' GROUP BY user_id,turn_id ORDER BY max(queued_at) DESC LIMIT 20`).Scan(&sample).Error; err != nil {
		b.Fatal("cannot select benchmark window")
	}
	if len(sample) == 0 {
		b.Fatal("no chat invocations to measure")
	}
	userID := sample[0].UserID
	var turns []string
	for _, item := range sample {
		if item.UserID == userID {
			turns = append(turns, item.TurnID)
		}
	}
	benchmarkSummaryRead(b, db, userID, turns, -1)
}

func benchmarkSummaryRead(b *testing.B, db *gorm.DB, userID string, turns []string, expected int) {
	b.Helper()
	repo := NewDBRepository(db)
	ctx := database.WithUserID(context.Background(), userID)
	read := func() int {
		summaries, err := repo.LoadSummariesForTurnIDs(ctx, turns)
		if err != nil {
			b.Fatal("summary projection failed")
		}
		count := 0
		for _, items := range summaries {
			count += len(items)
		}
		if count == 0 || (expected >= 0 && count != expected) {
			b.Fatalf("unexpected result count: %d", count)
		}
		return count
	}
	start := time.Now()
	count := read()
	firstRead := time.Since(start)
	samples := make([]float64, 0, 1024)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		read()
		if len(samples) < cap(samples) {
			samples = append(samples, float64(time.Since(start))/float64(time.Millisecond))
		}
	}
	b.StopTimer()
	sort.Float64s(samples)
	b.ReportMetric(float64(firstRead)/float64(time.Millisecond), "first-ms")
	b.ReportMetric(samples[(len(samples)-1)*95/100], "p95-ms")
	b.ReportMetric(float64(count), "summaries/op")
}
