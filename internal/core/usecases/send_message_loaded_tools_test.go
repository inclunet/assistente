package usecases

import (
	"context"
	"testing"

	"assistente/internal/database"
	"assistente/internal/tools"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestAvailableLoadedRuntimeToolsFiltersUnavailable(t *testing.T) {
	previousDB := database.DB()
	t.Cleanup(func() { database.SetDB(previousDB) })

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&database.ToolCatalog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.SetDB(db)

	if err := db.Create(&database.ToolCatalog{
		Name:               "read_file",
		DisplayName:        "read_file",
		Origin:             tools.ToolOriginBuiltin,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}).Error; err != nil {
		t.Fatalf("create available tool: %v", err)
	}
	if err := db.Create(&database.ToolCatalog{
		Name:               "mcp_srv__down",
		DisplayName:        "mcp_srv__down",
		Origin:             tools.ToolOriginMCPBridge,
		AvailabilityStatus: tools.ToolAvailabilityUnavailable,
	}).Error; err != nil {
		t.Fatalf("create unavailable tool: %v", err)
	}

	ctx := database.WithUserID(context.Background(), "user-a")
	got := availableLoadedRuntimeTools(ctx, []string{"mcp_srv__down", "read_file"})
	if len(got) != 1 || got[0] != "read_file" {
		t.Fatalf("availableLoadedRuntimeTools = %#v, want read_file only", got)
	}
}
