package commandsecurity

import (
	"context"
	"testing"
)

// Mede apenas a coordenação: não inclui autenticação, SQLite ou handler.
func BenchmarkDispatchGate(b *testing.B) {
	ctx := context.Background()
	noop := func() error { return nil }
	b.Run("admission", func(b *testing.B) {
		var gate DispatchGate
		b.ReportAllocs()
		for b.Loop() {
			if err := gate.WithAdmission(ctx, noop); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("parallel_admission", func(b *testing.B) {
		var gate DispatchGate
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				if err := gate.WithAdmission(ctx, noop); err != nil {
					b.Error(err)
					return
				}
			}
		})
	})
	b.Run("parallel_admission_and_mutation", func(b *testing.B) {
		var gate DispatchGate
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			n := 0
			for pb.Next() {
				n++
				var err error
				if n%10 == 0 {
					err = gate.WithMutation(ctx, noop)
				} else {
					err = gate.WithAdmission(ctx, noop)
				}
				if err != nil {
					b.Error(err)
					return
				}
			}
		})
	})
}
