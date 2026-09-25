package commandconfig

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestBumpGenerationTxCASRequiresExistingScope(t *testing.T) {
	db := storeTestDB(t)
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	user := storeTestUUID7(t)
	if err := store.EnsureScope(context.Background(), Scope{UserID: user}); err != nil {
		t.Fatal(err)
	}
	var got Generation
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		got, err = store.BumpGenerationTx(context.Background(), tx, Scope{UserID: user})
		return err
	}); err != nil {
		t.Fatalf("BumpGenerationTx: %v", err)
	}
	if got.Generation != 2 {
		t.Fatalf("generation = %d", got.Generation)
	}
	workspace := "ws-missing"
	if _, err := store.BumpGenerationTx(context.Background(), db, Scope{UserID: user, WorkspaceID: &workspace}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("TX/root ou scope ausente = %v", err)
	}
}
