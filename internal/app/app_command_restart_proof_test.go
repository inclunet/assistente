package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCommandRestartProofIsBoundToDatabaseRoot(t *testing.T) {
	ctx := context.Background()
	a := readyCommandProduct(t)
	core, err := a.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	proof, err := core.RestartProof(ctx, database.DB())
	if err != nil || !proof.Valid() {
		t.Fatalf("prova de restart inválida: valid=%v err=%v", proof.Valid(), err)
	}
	drained := commandsecurity.FromRestartProof(proof)

	foreign, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "foreign-restart-proof.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	foreignSQL, err := foreign.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = foreignSQL.Close() })

	ledgerStore, err := commandledger.New(foreign, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commandledger.NewCoordinatorRecovery(ledgerStore, drained); !errors.Is(err, commandledger.ErrInvalidRequest) {
		t.Fatalf("ledger aceitou prova de outra raiz: %v", err)
	}

	decisionStore, err := commanddecision.New(foreign, &commandDecisionPresenter{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commanddecision.NewCoordinatorRecovery(decisionStore, drained); !errors.Is(err, commanddecision.ErrInvalid) {
		t.Fatalf("decisions aceitou prova de outra raiz: %v", err)
	}
}
