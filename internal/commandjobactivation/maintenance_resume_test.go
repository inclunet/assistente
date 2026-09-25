package commandjobactivation

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/commandmaintenance"
	"gorm.io/gorm"
)

func TestMaintenanceHeartbeatResumesCommittedPrefixAfterFailure(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		name := "erro transitório"
		if canceled {
			name = "cancelamento"
		}
		t.Run(name, func(t *testing.T) {
			c, out, fact, _, now := fixture(t)
			deliver(t, c, out, fact)
			secondID := addHeartbeatLease(t, c, out, fact, *now)
			adapter, err := NewMaintenanceHeartbeatAdapter(c)
			if err != nil {
				t.Fatal(err)
			}
			policy := commandmaintenance.Policy{
				InvocationRetention: time.Hour, ActivationRetention: time.Hour,
				LeaseDuration: 45 * time.Second, InvocationsPerUser: 10,
				InvocationsSystemKeep: 10, ActivationsPerUser: 10, BatchSize: 2,
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("falha de infraestrutura na segunda lease")
			want := failure
			if canceled {
				want = context.Canceled
			}
			original := c.ports.Authorize
			calls := 0
			c.ports.Authorize = func(ctx context.Context, tx *gorm.DB, fact commandjobevents.Fact, workspace *string) (commandactivation.Owner, error) {
				calls++
				if calls == 2 {
					if canceled {
						cancel()
					}
					return commandactivation.Owner{}, want
				}
				return original(ctx, tx, fact, workspace)
			}
			first, err := adapter.Heartbeat(ctx, policy)
			if !errors.Is(err, want) || first.Processed != 1 || !first.More || adapter.cursor == "" || adapter.cursor == secondID {
				t.Fatalf("prefixo: result=%+v cursor=%q err=%v", first, adapter.cursor, err)
			}
			firstID := adapter.cursor
			var before Lease
			if err := c.db.Where("activation_id = ?", firstID).Take(&before).Error; err != nil {
				t.Fatal(err)
			}
			c.ports.Authorize = original
			*now = now.Add(time.Second)
			second, err := adapter.Heartbeat(context.Background(), policy)
			if err != nil || second.Processed != 1 || second.More || adapter.cursor != "" {
				t.Fatalf("retomada: result=%+v cursor=%q err=%v", second, adapter.cursor, err)
			}
			var after, renewed Lease
			if err := c.db.Where("activation_id = ?", firstID).Take(&after).Error; err != nil {
				t.Fatal(err)
			}
			if err := c.db.Where("activation_id = ?", secondID).Take(&renewed).Error; err != nil {
				t.Fatal(err)
			}
			if !after.ExpiresAt.Equal(before.ExpiresAt) || !renewed.ExpiresAt.Equal(now.Add(policy.LeaseDuration)) {
				t.Fatalf("retomada repetiu prefixo ou pulou falha: before=%v after=%v second=%v", before.ExpiresAt, after.ExpiresAt, renewed.ExpiresAt)
			}
			third, err := adapter.Heartbeat(context.Background(), policy)
			if err != nil || third.Processed != 2 || third.More {
				t.Fatalf("novo ciclo não revisitou todas as leases: %+v, %v", third, err)
			}
		})
	}
}
