package commandjobactivation

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandjobevents"
	"gorm.io/gorm"
)

func TestHeartbeatPassPropagatesPortErrorsWithoutRenewingOrAdvancing(t *testing.T) {
	ports := map[string]func(*Consumer, error){
		"layer": func(c *Consumer, want error) {
			c.ports.Layer = func(context.Context, *gorm.DB, commandactivation.Owner, commandactivation.Rule) (bool, error) {
				return false, want
			}
		},
		"condition": func(c *Consumer, want error) {
			c.ports.Condition = func(context.Context, *gorm.DB, commandactivation.Owner, commandactivation.Rule, commandjobevents.Fact) (bool, error) {
				return false, want
			}
		},
		"runtime": func(c *Consumer, want error) {
			c.ports.Runtime = func(context.Context, *gorm.DB, commandjobevents.Fact) (RuntimeIdentity, error) {
				return RuntimeIdentity{}, want
			}
		},
	}

	for name, configure := range ports {
		t.Run(name, func(t *testing.T) {
			c, out, fact, _, _ := fixture(t)
			deliver(t, c, out, fact)
			var before Lease
			if err := c.db.Take(&before).Error; err != nil {
				t.Fatal(err)
			}
			want := errors.New(name + " infrastructure failure")
			configure(c, want)

			cursor, result, err := c.HeartbeatPass(context.Background(), "", 1, 45*time.Second)
			if !errors.Is(err, want) || cursor != "" || result.Scanned != 1 || result.Renewed != 0 || result.Rejected != 0 || !result.More {
				t.Fatalf("erro da porta=(%q,%+v,%v), esperado falha sem avanço", cursor, result, err)
			}
			var after Lease
			if err := c.db.Take(&after).Error; err != nil {
				t.Fatal(err)
			}
			if !after.ExpiresAt.Equal(before.ExpiresAt) || !after.UpdatedAt.Equal(before.UpdatedAt) {
				t.Fatalf("lease alterada após erro da porta: antes=%+v depois=%+v", before, after)
			}
		})
	}
}

func TestHeartbeatPassPropagatesPortCancellationWithoutRenewingOrAdvancing(t *testing.T) {
	ports := map[string]func(*Consumer, context.CancelFunc){
		"layer": func(c *Consumer, cancel context.CancelFunc) {
			c.ports.Layer = func(ctx context.Context, _ *gorm.DB, _ commandactivation.Owner, _ commandactivation.Rule) (bool, error) {
				cancel()
				return false, ctx.Err()
			}
		},
		"condition": func(c *Consumer, cancel context.CancelFunc) {
			c.ports.Condition = func(ctx context.Context, _ *gorm.DB, _ commandactivation.Owner, _ commandactivation.Rule, _ commandjobevents.Fact) (bool, error) {
				cancel()
				return false, ctx.Err()
			}
		},
		"runtime": func(c *Consumer, cancel context.CancelFunc) {
			c.ports.Runtime = func(ctx context.Context, _ *gorm.DB, _ commandjobevents.Fact) (RuntimeIdentity, error) {
				cancel()
				return RuntimeIdentity{}, ctx.Err()
			}
		},
	}

	for name, configure := range ports {
		t.Run(name, func(t *testing.T) {
			c, out, fact, _, _ := fixture(t)
			deliver(t, c, out, fact)
			var before Lease
			if err := c.db.Take(&before).Error; err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			configure(c, cancel)

			cursor, result, err := c.HeartbeatPass(ctx, "", 1, 45*time.Second)
			if !errors.Is(err, context.Canceled) || cursor != "" || result.Scanned != 1 || result.Renewed != 0 || result.Rejected != 0 || !result.More {
				t.Fatalf("cancelamento da porta=(%q,%+v,%v), esperado falha sem avanço", cursor, result, err)
			}
			var after Lease
			if err := c.db.Take(&after).Error; err != nil {
				t.Fatal(err)
			}
			if !after.ExpiresAt.Equal(before.ExpiresAt) || !after.UpdatedAt.Equal(before.UpdatedAt) {
				t.Fatalf("lease alterada após cancelamento: antes=%+v depois=%+v", before, after)
			}
		})
	}
}

func TestReconcileBatchPropagatesPortFailureWithoutInvalidatingLease(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	deliver(t, c, out, fact)
	var before Lease
	if err := c.db.Take(&before).Error; err != nil {
		t.Fatal(err)
	}
	want := errors.New("layer infrastructure failure")
	c.ports.Layer = func(context.Context, *gorm.DB, commandactivation.Owner, commandactivation.Rule) (bool, error) {
		return false, want
	}

	cursor, done, err := c.ReconcileBatch(context.Background(), "", 1)
	if !errors.Is(err, want) || cursor != "" || done {
		t.Fatalf("reconciliação=(%q,%v,%v), esperado fail-closed sem cursor", cursor, done, err)
	}
	var after Lease
	if err := c.db.Take(&after).Error; err != nil {
		t.Fatal(err)
	}
	if !after.ExpiresAt.Equal(before.ExpiresAt) || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("lease alterada após falha de infraestrutura: antes=%+v depois=%+v", before, after)
	}
	var claim commandactivation.Claim
	if err := c.db.Take(&claim).Error; err != nil {
		t.Fatal(err)
	}
	if claim.State != commandactivation.StateActive {
		t.Fatalf("claim invalidada após falha de infraestrutura: %s", claim.State)
	}
}
