package app

import (
	"errors"
	"strings"
	"testing"

	"assistente/internal/commandruntime"
)

func TestCommandMountIdentifiesMissingDependency(t *testing.T) {
	a, baseline := appLifecycleProductMountFixture(t)
	for _, tc := range []struct {
		name   string
		remove func(*CommandLifecycleMountInputs)
	}{
		{"host", func(i *CommandLifecycleMountInputs) { i.Host = nil }},
		{"bridge", func(i *CommandLifecycleMountInputs) { i.Bridge = nil }},
		{"context-fact-bus", func(i *CommandLifecycleMountInputs) { i.Facts = nil }},
		{"adapter", func(i *CommandLifecycleMountInputs) { i.Adapter = (*appLifecycleBridgePort)(nil) }},
		{"registry", func(i *CommandLifecycleMountInputs) { i.Execution.Registry = nil }},
		{"ledger-store", func(i *CommandLifecycleMountInputs) { i.Execution.Store = nil }},
		{"authorize", func(i *CommandLifecycleMountInputs) { i.Execution.Authorize = nil }},
		{"envelope", func(i *CommandLifecycleMountInputs) { i.Execution.Envelope = nil }},
		{"handlers", func(i *CommandLifecycleMountInputs) { i.Execution.Handlers = nil }},
		{"registry-version", func(i *CommandLifecycleMountInputs) { i.Execution.RegistryVersion = "" }},
		{"retention", func(i *CommandLifecycleMountInputs) { i.Execution.Retention = 0 }},
		{"execution-timeout", func(i *CommandLifecycleMountInputs) { i.Execution.ExecutionTimeout = 0 }},
		{"finalization-timeout", func(i *CommandLifecycleMountInputs) { i.Execution.FinalizationTimeout = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inputs := baseline
			tc.remove(&inputs)
			_, err := a.commandLifecycleMountSpec(inputs)
			if !errors.Is(err, commandruntime.ErrMissingDependency) || !strings.HasSuffix(err.Error(), ": "+tc.name) {
				t.Fatalf("dependência %s não identificada: %v", tc.name, err)
			}
			if a.commandLifecycle.Load() != nil {
				t.Fatal("diagnóstico publicou runtime")
			}
		})
	}
	if _, err := a.commandLifecycleMountSpec(baseline); err != nil {
		t.Fatal(err)
	}
	presenter := a.questionnaireMgr
	a.questionnaireMgr = nil
	_, err := a.commandLifecycleMountSpec(baseline)
	a.questionnaireMgr = presenter
	if !errors.Is(err, commandruntime.ErrMissingDependency) || !strings.HasSuffix(err.Error(), ": decision-presenter") {
		t.Fatalf("presenter: %v", err)
	}
	version := a.commandStorageVersion
	a.commandStorageVersion = ""
	_, err = a.commandLifecycleMountSpec(baseline)
	a.commandStorageVersion = version
	if !errors.Is(err, commandruntime.ErrMissingDependency) || !strings.HasSuffix(err.Error(), ": command-storage") {
		t.Fatalf("storage: %v", err)
	}
}
