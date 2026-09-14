package commandbindings

import (
	"strconv"
	"sync/atomic"
	"testing"
)

const (
	resolverBenchmarkDistinctTriggerPrefix = "resolver-bench-trigger-"
	resolverBenchmarkCollisionTrigger      = "resolver-bench-collision"
)

func resolverBenchmarkDistinctSnapshot(total int, generalPath bool) (*Resolver, string, Facts, error) {
	candidates := make([]Candidate, total)
	for i := range candidates {
		candidates[i] = Candidate{
			ID:                "resolver-bench-binding-" + strconv.Itoa(i),
			Trigger:           resolverBenchmarkDistinctTriggerPrefix + strconv.Itoa(i),
			CommandID:         "resolver-bench-command-" + strconv.Itoa(i),
			ArgumentsKey:      "resolver-bench-args",
			ExecutionScopeKey: "resolver-bench-target",
			Scope:             Application,
			Enabled:           true,
			LayerActive:       true,
		}
	}

	trigger := resolverBenchmarkDistinctTriggerPrefix + strconv.Itoa(total-1)
	if generalPath {
		candidates = append(candidates, Candidate{
			ID:                "resolver-bench-disabled-extra",
			Trigger:           trigger,
			CommandID:         "resolver-bench-disabled-extra-command",
			ArgumentsKey:      "resolver-bench-args",
			ExecutionScopeKey: "resolver-bench-target",
			Scope:             Application,
			Enabled:           false,
			LayerActive:       true,
		})
	}
	returnResolver, err := New(candidates)
	return returnResolver, trigger, nil, err
}

func resolverBenchmarkResultOK(got Result, err error, wantStatus Status, wantCommand, wantBinding string) bool {
	if err != nil || got.Status != wantStatus || got.CommandID != wantCommand {
		return false
	}
	if wantStatus == NoMatch {
		return len(got.BindingIDs) == 0
	}
	return len(got.BindingIDs) == 1 && got.BindingIDs[0] == wantBinding
}

func BenchmarkResolveDistinctBindings(b *testing.B) {
	for _, total := range []int{1, 100, 1000} {
		for _, hit := range []bool{true, false} {
			name := "miss"
			if hit {
				name = "hit"
			}

			b.Run(strconv.Itoa(total)+"/"+name, func(b *testing.B) {
				resolver, hitTrigger, facts, err := resolverBenchmarkDistinctSnapshot(total, false)
				if err != nil {
					b.Fatal(err)
				}

				trigger := hitTrigger
				wantStatus := Selected
				wantCommand := "resolver-bench-command-" + strconv.Itoa(total-1)
				wantBinding := "resolver-bench-binding-" + strconv.Itoa(total-1)
				if !hit {
					trigger = "resolver-bench-trigger-missing"
					wantStatus = NoMatch
					wantCommand = ""
					wantBinding = ""
				}

				got, err := resolver.Resolve(trigger, facts, nil)
				if !resolverBenchmarkResultOK(got, err, wantStatus, wantCommand, wantBinding) {
					b.Fatalf("resultado de setup inesperado: %+v, %v", got, err)
				}

				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					got, err := resolver.Resolve(trigger, facts, nil)
					if !resolverBenchmarkResultOK(got, err, wantStatus, wantCommand, wantBinding) {
						b.Fatalf("resultado inesperado: %+v, %v", got, err)
					}
				}
			})
		}
	}
}

func BenchmarkResolveDistinctBindingsPath(b *testing.B) {
	for _, total := range []int{1, 100, 1000} {
		for _, generalPath := range []bool{false, true} {
			name := "fast-path"
			if generalPath {
				name = "general-path"
			}

			b.Run(strconv.Itoa(total)+"/"+name, func(b *testing.B) {
				resolver, trigger, facts, err := resolverBenchmarkDistinctSnapshot(total, generalPath)
				if err != nil {
					b.Fatal(err)
				}
				wantCommand := "resolver-bench-command-" + strconv.Itoa(total-1)
				wantBinding := "resolver-bench-binding-" + strconv.Itoa(total-1)

				got, err := resolver.Resolve(trigger, facts, nil)
				if !resolverBenchmarkResultOK(got, err, Selected, wantCommand, wantBinding) {
					b.Fatalf("resultado de setup inesperado: %+v, %v", got, err)
				}

				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					got, err := resolver.Resolve(trigger, facts, nil)
					if !resolverBenchmarkResultOK(got, err, Selected, wantCommand, wantBinding) {
						b.Fatalf("resultado inesperado: %+v, %v", got, err)
					}
				}
			})
		}
	}
}

func resolverBenchmarkCollisionSnapshot(total int) (*Resolver, string, Facts, string, error) {
	candidates := make([]Candidate, total)
	for i := range candidates {
		condition := Facts{}
		switch i % 3 {
		case 1:
			condition[Profile] = "dev"
		case 2:
			condition[Profile] = "dev"
			condition[Device] = "deck"
		}
		candidates[i] = Candidate{
			ID:                "resolver-bench-collision-binding-" + strconv.Itoa(i),
			Trigger:           resolverBenchmarkCollisionTrigger,
			CommandID:         "resolver-bench-collision-command-" + strconv.Itoa(i),
			ArgumentsKey:      "resolver-bench-collision-args",
			ExecutionScopeKey: "resolver-bench-collision-target",
			Scope:             Surface,
			Condition:         condition,
			LayerPriority:     i,
			BindingPriority:   i,
			Enabled:           true,
			LayerActive:       true,
		}
	}

	winner := total - 1
	for winner%3 != 2 {
		winner--
	}
	resolver, err := New(candidates)
	return resolver, resolverBenchmarkCollisionTrigger, Facts{Profile: "dev", Device: "deck"},
		"resolver-bench-collision-command-" + strconv.Itoa(winner), err
}

func BenchmarkResolveSameTriggerCollisions(b *testing.B) {
	for _, total := range []int{10, 100} {
		b.Run(strconv.Itoa(total), func(b *testing.B) {
			resolver, trigger, facts, wantCommand, err := resolverBenchmarkCollisionSnapshot(total)
			if err != nil {
				b.Fatal(err)
			}
			wantBinding := "resolver-bench-collision-binding-" + strconv.Itoa(total-1)
			winner := total - 1
			for winner%3 != 2 {
				winner--
			}
			wantBinding = "resolver-bench-collision-binding-" + strconv.Itoa(winner)

			got, err := resolver.Resolve(trigger, facts, nil)
			if !resolverBenchmarkResultOK(got, err, Selected, wantCommand, wantBinding) {
				b.Fatalf("resultado de setup inesperado: %+v, %v", got, err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				got, err := resolver.Resolve(trigger, facts, nil)
				if !resolverBenchmarkResultOK(got, err, Selected, wantCommand, wantBinding) {
					b.Fatalf("resultado inesperado: %+v, %v", got, err)
				}
			}
		})
	}
}

func BenchmarkResolveParallelSameSnapshot(b *testing.B) {
	resolver, trigger, facts, wantCommand, err := resolverBenchmarkCollisionSnapshot(100)
	if err != nil {
		b.Fatal(err)
	}
	winner := 98
	wantBinding := "resolver-bench-collision-binding-" + strconv.Itoa(winner)

	got, err := resolver.Resolve(trigger, facts, nil)
	if !resolverBenchmarkResultOK(got, err, Selected, wantCommand, wantBinding) {
		b.Fatalf("resultado de setup inesperado: %+v, %v", got, err)
	}

	var invalid atomic.Bool
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			got, err := resolver.Resolve(trigger, facts, nil)
			if !resolverBenchmarkResultOK(got, err, Selected, wantCommand, wantBinding) {
				invalid.Store(true)
				return
			}
		}
	})
	b.StopTimer()
	if invalid.Load() {
		b.Fatal("resultado inesperado em leitura paralela do snapshot")
	}
}
