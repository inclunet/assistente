package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	commandtool "assistente/internal/tools/command"
	"gorm.io/gorm"
)

func TestCommandAgentAuthorityUsesCommitTransactionAndSeesCancellation(t *testing.T) {
	a := readyCommandProduct(t)
	ctx := commandAgentTestContext(t, a, commandtool.ConfigName)
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	caller, err := a.commandAgentAccess(ctx, commandtool.ConfigName)
	if err != nil {
		t.Fatal(err)
	}
	db := database.DB()
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	previous := pool.Stats().MaxOpenConnections
	pool.SetMaxOpenConns(1)
	defer pool.SetMaxOpenConns(previous)
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := caller.revalidateTx(ctx, tx); err != nil {
			return err
		}
		if err := tx.Model(&database.ToolInvocation{}).Where("id = ?", toolinvocations.CurrentInvocationID(ctx)).Update("status", toolinvocations.StatusCancelled).Error; err != nil {
			return err
		}
		if err := caller.revalidateTx(ctx, tx); !errors.Is(err, commandexecution.ErrDenied) {
			t.Fatalf("uncommitted cancellation notobserved: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
