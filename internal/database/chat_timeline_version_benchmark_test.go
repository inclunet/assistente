package database

import (
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Compara escritas equivalentes com e sem invalidação. Ledger/catálogo têm
// 100 conversas e 1.000 invocações compartilhando a ferramenta alterada.
// Não mede streaming: não há gravação nem consulta por delta neste contrato.
func BenchmarkChatTimelineInvalidation(b *testing.B) {
	for _, operation := range []string{"message_64KiB", "ledger_1000", "catalog_1000"} {
		b.Run(operation, func(b *testing.B) {
			for _, enabled := range []bool{false, true} {
				name := "baseline"
				if enabled {
					name = "triggers"
				}
				b.Run(name, func(b *testing.B) {
					database := timelineBenchmarkDB(b, operation != "message_64KiB")
					if enabled {
						if err := migrateChatTimelineVersions(database); err != nil {
							b.Fatal(err)
						}
					}
					text := strings.Repeat("a", 64<<10)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						var err error
						switch operation {
						case "message_64KiB":
							err = database.Exec("UPDATE chat_messages SET content=?,total_tokens=? WHERE id='answer'", text, i).Error
						case "ledger_1000":
							err = database.Exec("UPDATE tool_invocations SET output_preview=? WHERE id='inv-0'", fmt.Sprint(i)).Error
						case "catalog_1000":
							err = database.Exec("UPDATE tool_catalog SET display_name=? WHERE id='tool'", fmt.Sprint(i)).Error
						}
						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

func timelineBenchmarkDB(b *testing.B, withLedger bool) *gorm.DB {
	b.Helper()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = sqlDB.Close() })
	sqlDB.SetMaxOpenConns(1)
	if err := database.AutoMigrate(&User{}, &Conversation{}, &ChatMessage{}, &ToolCatalog{}, &ToolInvocation{}); err != nil {
		b.Fatal(err)
	}
	if err := createChatTurnSchema(database); err != nil {
		b.Fatal(err)
	}
	if err := database.Create(&User{UUIDModel: UUIDModel{ID: "user"}, Username: "user"}).Error; err != nil {
		b.Fatal(err)
	}
	if err := database.Create(&Conversation{UUIDModel: UUIDModel{ID: "conv"}, UserID: "user"}).Error; err != nil {
		b.Fatal(err)
	}
	if err := database.Create(&ChatMessage{UUIDModel: UUIDModel{ID: "answer"}, ConversationID: "conv", Role: "assistant"}).Error; err != nil {
		b.Fatal(err)
	}
	if !withLedger {
		return database
	}
	if err := database.Create(&ToolCatalog{UUIDModel: UUIDModel{ID: "tool"}, Name: "tool"}).Error; err != nil {
		b.Fatal(err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		for c := 0; c < 100; c++ {
			conv, turn := fmt.Sprintf("conv-%d", c), fmt.Sprintf("turn-%d", c)
			if err := tx.Create(&Conversation{UUIDModel: UUIDModel{ID: conv}, UserID: "user"}).Error; err != nil {
				return err
			}
			if err := tx.Create(&ChatMessage{UUIDModel: UUIDModel{ID: turn}, ConversationID: conv, Role: "user"}).Error; err != nil {
				return err
			}
			for j := 0; j < 10; j++ {
				invocation := ToolInvocation{UUIDModel: UUIDModel{ID: fmt.Sprintf("inv-%d", c*10+j)}, UserID: "user", ToolCatalogID: "tool", OriginType: "chat", ConversationID: &conv, TurnID: &turn, Status: "running"}
				if err := tx.Create(&invocation).Error; err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		b.Fatal(err)
	}
	return database
}
