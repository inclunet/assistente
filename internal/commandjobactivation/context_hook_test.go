package commandjobactivation

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"assistente/internal/commandactivation"
	"assistente/internal/commandjobevents"
	"gorm.io/gorm"
)

type contextHookKey struct{}

func TestWithContextGuardsAllTransactionalPathsAndReleasesOnError(t *testing.T) {
	tests := []struct {
		name string
		fail func(*Consumer, error)
	}{
		{
			name: "consume",
			fail: func(c *Consumer, want error) {
				c.ports.Authorize = func(ctx context.Context, _ *gorm.DB, _ commandjobevents.Fact, _ *string) (commandactivation.Owner, error) {
					if ctx.Value(contextHookKey{}) != "locked" {
						return commandactivation.Owner{}, errors.New("hook context not propagated")
					}
					return commandactivation.Owner{}, want
				}
			},
		},
		{
			name: "reconcile",
			fail: func(c *Consumer, want error) {
				c.ports.Layer = func(ctx context.Context, _ *gorm.DB, _ commandactivation.Owner, _ commandactivation.Rule) (bool, error) {
					if ctx.Value(contextHookKey{}) != "locked" {
						return false, errors.New("hook context not propagated")
					}
					return false, want
				}
			},
		},
		{
			name: "renew",
			fail: func(c *Consumer, want error) {
				c.ports.Layer = func(ctx context.Context, _ *gorm.DB, _ commandactivation.Owner, _ commandactivation.Rule) (bool, error) {
					if ctx.Value(contextHookKey{}) != "locked" {
						return false, errors.New("hook context not propagated")
					}
					return false, want
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, out, fact, _, _ := fixture(t)
			if err := c.db.Transaction(func(tx *gorm.DB) error { return out.InsertFactTx(tx, fact) }); err != nil {
				t.Fatal(err)
			}
			activationID := ""
			if test.name == "consume" {
				claimed, _, err := out.ClaimBatch(context.Background(), "consumer", 1)
				if err != nil || len(claimed) != 1 {
					t.Fatalf("claim=%d err=%v", len(claimed), err)
				}
			} else {
				deliver(t, c, out, fact)
				var lease Lease
				if err := c.db.Take(&lease).Error; err != nil {
					t.Fatal(err)
				}
				activationID = lease.ActivationID
			}

			var entered, released atomic.Int32
			c.ports.WithContext = func(ctx context.Context, fn func(context.Context) error) error {
				entered.Add(1)
				defer released.Add(1)
				return fn(context.WithValue(ctx, contextHookKey{}, "locked"))
			}
			want := errors.New(test.name + " failure")
			test.fail(c, want)
			var callErr error
			switch test.name {
			case "consume":
				callErr = func() error { _, err := c.Consume(context.Background(), fact.SourceEventID, "consumer"); return err }()
			case "reconcile":
				callErr = func() error { _, _, err := c.ReconcileBatch(context.Background(), "", 1); return err }()
			case "renew":
				callErr = c.RenewRuntime(context.Background(), activationID)
			}
			if !errors.Is(callErr, want) {
				t.Fatalf("erro=%v, want %v", callErr, want)
			}
			if entered.Load() != 1 || released.Load() != 1 {
				t.Fatalf("hook entered=%d released=%d", entered.Load(), released.Load())
			}
		})
	}
}
