package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestLLMProviderServerProfileIDAutoMigrates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("abrir SQLite em memória: %v", err)
	}
	if err := db.AutoMigrate(&LLMProvider{}); err != nil {
		t.Fatalf("AutoMigrate(LLMProvider): %v", err)
	}

	want := LLMProvider{
		ID: "openrouter", Name: "OpenRouter", Type: "openrouter",
		BaseURL: "https://openrouter.ai/api/v1", ServerProfileID: "openrouter",
	}
	if err := db.Create(&want).Error; err != nil {
		t.Fatalf("criar provider: %v", err)
	}
	var got LLMProvider
	if err := db.First(&got, "id = ?", want.ID).Error; err != nil {
		t.Fatalf("buscar provider: %v", err)
	}
	if got.ServerProfileID != want.ServerProfileID {
		t.Fatalf("server profile id = %q, esperado %q", got.ServerProfileID, want.ServerProfileID)
	}
}
