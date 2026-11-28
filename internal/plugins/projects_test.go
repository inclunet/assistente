package plugins

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/database"
	"gorm.io/gorm"
)

func TestBuilderProjectsPersistSelectionsByAccount(t *testing.T) {
	db := managedJobTestDB(t)
	if err := db.AutoMigrate(&database.PluginProject{}); err != nil {
		t.Fatal(err)
	}
	inventory := NewInventory(db)
	owner := database.WithUserID(t.Context(), "author")
	other := database.WithUserID(t.Context(), "reader")
	busyOnce := true
	admit := func(_ *gorm.DB, commit func() error) error {
		if busyOnce {
			busyOnce = false
			return errors.New("database is locked (SQLITE_BUSY)")
		}
		return commit()
	}
	original, err := inventory.SaveProject(owner, BuilderProject{Selection: BuilderRequest{Name: "kit"}}, admit)
	if err != nil || original.ID == "" || original.Revision != 1 {
		t.Fatalf("criar: %+v %v", original, err)
	}
	updated := original
	updated.Selection.Skills = []string{"revisor"}
	busyOnce = true
	current, err := inventory.SaveProject(owner, updated, admit)
	if err != nil || current.Revision != 2 {
		t.Fatalf("salvar: %+v %v", current, err)
	}
	if _, err = inventory.SaveProject(owner, original, admit); err == nil {
		t.Fatal("sobrescreveu edição concorrente")
	}
	if _, err = inventory.SaveProject(other, current, admit); err == nil {
		t.Fatal("outra conta alterou o projeto")
	}
	if _, err = inventory.SaveProject(owner, current, func(_ *gorm.DB, _ func() error) error { return errors.New("sessão encerrada") }); err == nil {
		t.Fatal("admissão recusada")
	}
	for _, tc := range []struct {
		ctx   context.Context
		count int
	}{{owner, 1}, {other, 0}} {
		rows, err := NewInventory(db).Projects(tc.ctx)
		if err != nil || len(rows) != tc.count {
			t.Fatalf("listar: %v %v", rows, err)
		}
		if len(rows) > 0 && (rows[0].ID != original.ID || rows[0].Revision != 2 || len(rows[0].Selection.Skills) != 1 || rows[0].Selection.Skills[0] != "revisor") {
			t.Fatalf("reabrir: %+v", rows)
		}
	}
}
