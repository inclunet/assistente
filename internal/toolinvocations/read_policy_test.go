package toolinvocations

import (
	"context"
	"testing"

	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestLegacyReadPolicyAutorizaSomentePendingDoUsuario(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB.AutoMigrate(&database.ToolLedgerMigrationState{}); err != nil {
		t.Fatal(err)
	}
	previous := database.DB()
	database.SetDB(testDB)
	t.Cleanup(func() { database.SetDB(previous) })

	states := []database.ToolLedgerMigrationState{
		{UserID: "user-a", ResourceType: "conversation", ResourceID: "pending-a", State: "pending"},
		{UserID: "user-a", ResourceType: "conversation", ResourceID: "backfilled-a", State: "backfilled"},
		{UserID: "user-b", ResourceType: "conversation", ResourceID: "pending-b", State: "pending"},
	}
	if err := testDB.Create(&states).Error; err != nil {
		t.Fatal(err)
	}

	policy, err := LoadLegacyReadPolicyWithUser(
		context.Background(),
		"user-a",
		[]string{"pending-a", "backfilled-a", "pending-b", "new-a", "pending-a", ""},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Allows("pending-a") {
		t.Fatal("conversa pending deveria autorizar leitura legada")
	}
	for _, id := range []string{"backfilled-a", "pending-b", "new-a"} {
		if policy.Allows(id) {
			t.Fatalf("conversa %s autorizou legado indevidamente", id)
		}
	}
}

func TestLegacyReadPolicyPermiteSchemaPreV18(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	previous := database.DB()
	database.SetDB(testDB)
	t.Cleanup(func() { database.SetDB(previous) })

	policy, err := LoadLegacyReadPolicyWithUser(context.Background(), "user-a", []string{"legacy"})
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Allows("legacy") {
		t.Fatal("schema pré-v18 deveria preservar leitura legada")
	}
}
