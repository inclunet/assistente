package commandautomation

import (
	"context"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestGrantOpaqueWorkspaceAndDetachedOwner(t *testing.T) {
	f := newAutomationFixture(t)
	w := "ws-0123456789abcdef"
	f.owner.WorkspaceID = &w
	f.rule.Owner = f.owner
	if err := f.db.Model(&ruleRow{}).Where("id = ?", f.rule.ID).Update("workspace_id", w).Error; err != nil {
		t.Fatal(err)
	}
	confirmed := f.confirmed(t)
	if err := f.store.CommitConfirmedGrant(context.Background(), confirmed, f.epoch); err != nil {
		t.Fatal(err)
	}
	otherPointer := w
	owner := Owner{UserID: f.owner.UserID, WorkspaceID: &otherPointer}
	if _, err := f.store.LoadActive(context.Background(), owner, keyFromRule(f.rule)); err != nil {
		t.Fatalf("mesmo workspace, ponteiro distinto: %v", err)
	}
	for _, invalid := range []string{"", " ws-x", "ws-x ", "ws-\x00x"} {
		if validOwner(Owner{UserID: owner.UserID, WorkspaceID: &invalid}) {
			t.Fatalf("workspace inválido aceito: %q", invalid)
		}
	}
}

func TestGrantPreparedOwnerAndPresenterCannotMutateProposal(t *testing.T) {
	f := newAutomationFixture(t)
	workspace := "ws-exact"
	f.owner.WorkspaceID = &workspace
	f.rule.Owner = f.owner
	if err := f.db.Model(&ruleRow{}).Where("id = ?", f.rule.ID).Update("workspace_id", workspace).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	p, err := f.store.Prepare(ctx, f.owner, f.rule.RuleRef)
	if err != nil {
		t.Fatal(err)
	}
	workspace = "ws-tampered"
	c, err := f.store.ConfirmGrant(ctx, p, f.epoch, f.receipts, "v1", fixtureKeys, f.now.Add(time.Minute), func(rule Rule) (string, error) {
		*rule.Owner.WorkspaceID = "ws-presenter"
		rule.AllowedInternalProducerTypes[0] = "untrusted"
		return "confirmar", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.CommitConfirmedGrant(ctx, c, f.epoch, func(_ context.Context, _ *gorm.DB, owner Owner, rule Rule) error {
		*owner.WorkspaceID = "ws-validator"
		*rule.Owner.WorkspaceID = "ws-validator"
		rule.AllowedInternalProducerTypes[0] = "forged"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if *p.owner.WorkspaceID != "ws-exact" || p.rule.AllowedInternalProducerTypes[0] != JobsRuntime {
		t.Fatal("callback alterou proposta assinada")
	}
}
