package commandbindings

import (
	"strconv"
	"testing"
)

const configurationBenchTrigger = "Ctrl+Shift+B"

func configurationBenchDefault(index int, trigger string) Default {
	return Default{
		Candidate: Candidate{
			ID:                "configuration-bench-default-" + strconv.Itoa(index),
			Trigger:           trigger,
			CommandID:         "configuration-bench-command-" + strconv.Itoa(index),
			ArgumentsKey:      "configuration-bench-args",
			ExecutionScopeKey: "configuration-bench-target",
			Scope:             Application,
			Enabled:           true,
			LayerActive:       true,
		},
		Version:     "1",
		Fingerprint: "configuration-bench-fingerprint-" + strconv.Itoa(index),
	}
}

func configurationBenchDeltas(defaults []Default, trigger string) []Delta {
	if len(defaults) < 2 {
		panic("configuration benchmark exige ao menos dois defaults")
	}

	return []Delta{
		{
			ID:                 "configuration-bench-override",
			DefaultID:          defaults[0].Candidate.ID,
			DefaultVersion:     defaults[0].Version,
			DefaultFingerprint: defaults[0].Fingerprint,
			Trigger:            trigger,
			Effect:             Execute,
			CommandID:          "configuration-bench-override-command",
			ArgumentsKey:       "configuration-bench-override-args",
			Enabled:            true,
			LayerActive:        true,
			ReviewStatus:       Active,
			LayerPriority:      1,
		},
		{
			ID:                 "configuration-bench-suppress",
			DefaultID:          defaults[1].Candidate.ID,
			DefaultVersion:     defaults[1].Version,
			DefaultFingerprint: defaults[1].Fingerprint,
			Trigger:            trigger,
			Effect:             Suppress,
			Enabled:            true,
			LayerActive:        true,
			ReviewStatus:       Active,
		},
	}
}

func configurationBenchConfiguration(b *testing.B, total int, manyBuckets bool) *Configuration {
	b.Helper()
	defaults := make([]Default, total)
	for i := range defaults {
		trigger := configurationBenchTrigger
		if manyBuckets && i >= 2 {
			trigger = "configuration-bench-trigger-" + strconv.Itoa(i)
		}
		defaults[i] = configurationBenchDefault(i, trigger)
	}
	deltas := configurationBenchDeltas(defaults, configurationBenchTrigger)
	configuration, err := NewConfiguration(defaults, deltas, nil)
	if err != nil {
		b.Fatal(err)
	}

	got, err := configuration.Resolve(configurationBenchTrigger, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	if got.Status != Selected || got.CommandID != "configuration-bench-override-command" {
		b.Fatalf("resultado inesperado antes do benchmark: %+v", got)
	}
	return configuration
}

func BenchmarkConfigurationResolve(b *testing.B) {
	for _, total := range []int{10, 100, 1000} {
		for _, manyBuckets := range []bool{false, true} {
			name := "same-trigger"
			if manyBuckets {
				name = "many-buckets"
			}
			b.Run(name+"/"+strconv.Itoa(total), func(b *testing.B) {
				configuration := configurationBenchConfiguration(b, total, manyBuckets)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					got, err := configuration.Resolve(configurationBenchTrigger, nil, nil)
					if err != nil || got.CommandID != "configuration-bench-override-command" {
						b.Fatalf("resultado inesperado: %+v, %v", got, err)
					}
				}
			})
		}
	}
}
