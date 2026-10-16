package commandactivation

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestRevokeInvalidRuleTxDisablesRuleAndNeverReactivatesClaim(t *testing.T) {
	db := activationDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	owner := activationOwner(t)
	layer := Ref{Kind: UserRef, ID: activationID(t)}
	rule := manualRule(t, owner, layer, LifecyclePersistent)
	if err := store.CreateRule(context.Background(), rule); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	claim := Claim{ActivationID: activationID(t), LayerRefKind: layer.Kind, LayerRef: layer.ID, RuleRefKind: UserRef, RuleRef: rule.ID,
		UserID: owner.UserID, AuthContextType: owner.AuthContextType, AuthContextID: owner.AuthContextID, AuthGeneration: owner.AuthGeneration,
		SecurityGeneration: owner.SecurityGeneration, SourceType: "manual", State: StateActive, ManualStackKey: stringPtr("stack"), ActivatedAt: now, UpdatedAt: now}
	if err := db.Create(&claim).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := store.RevokeInvalidRuleTx(context.Background(), tx, owner, Ref{Kind: UserRef, ID: rule.ID}, "grant_stale")
		return err
	}); err != nil {
		t.Fatalf("RevokeInvalidRuleTx: %v", err)
	}
	loaded, err := store.ResolveRule(context.Background(), owner, Ref{Kind: UserRef, ID: rule.ID})
	if err != nil || loaded.Enabled {
		t.Fatalf("regra após invalidação = %+v, erro=%v", loaded, err)
	}
	loadedClaim, err := store.GetClaim(context.Background(), owner, claim.ActivationID)
	if err != nil || loadedClaim.State != StateStale || loadedClaim.TerminalReason == nil || *loadedClaim.TerminalReason != "grant_stale" {
		t.Fatalf("claim após invalidação = %+v, erro=%v", loadedClaim, err)
	}
}

func stringPtr(value string) *string { return &value }
