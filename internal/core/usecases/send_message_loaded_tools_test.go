package usecases

import (
	"context"
	"testing"

	"assistente/internal/database"
	"assistente/internal/tools"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAvailableLoadedRuntimeToolsFiltersUnavailable(t *testing.T) {
	previousDB := database.DB()
	t.Cleanup(func() { database.SetDB(previousDB) })

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("open sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&database.ToolCatalog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.SetDB(db)

	userID := "user-a"
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
		UserID:             &userID,
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
	available, unavailable, err := loadedRuntimeToolAvailability(ctx, []string{"mcp_srv__down", "read_file"}, "mcp_srv__down")
	if err != nil || len(available) != 2 || len(unavailable) != 0 {
		t.Fatalf("recoverable tool was forgotten: %v %v %v", available, unavailable, err)
	}

}

func TestLoadedRuntimeToolAvailabilityPreservesUnknownCatalogEntries(t *testing.T) {
	previousDB := database.DB()
	t.Cleanup(func() { database.SetDB(previousDB) })

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("open sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&database.ToolCatalog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.SetDB(db)

	ctx := database.WithUserID(context.Background(), "user-a")
	available, unavailable, err := loadedRuntimeToolAvailability(ctx, []string{"read_file", "legacy_tool"})
	if err != nil {
		t.Fatalf("loadedRuntimeToolAvailability returned error: %v", err)
	}
	if len(available) != 2 || available[0] != "read_file" || available[1] != "legacy_tool" {
		t.Fatalf("available = %#v, want both known and unknown tools", available)
	}
	if len(unavailable) != 0 {
		t.Fatalf("unavailable = %#v, want none", unavailable)
	}
}

func TestLoadedRuntimeToolAvailabilityReturnsErrorForCatalogFailure(t *testing.T) {
	previousDB := database.DB()
	t.Cleanup(func() { database.SetDB(previousDB) })

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("open sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	database.SetDB(db)

	ctx := database.WithUserID(context.Background(), "user-a")
	want := []string{"read_file", "mcp_srv__search"}
	got := availableLoadedRuntimeTools(ctx, want)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("availableLoadedRuntimeTools = %#v, want preserved names %#v", got, want)
	}
}
