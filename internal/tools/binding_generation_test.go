package tools

import (
	"context"
	"encoding/json"
	"math"
	"testing"
)

func generationTool(name, content string, calls *int) *mockTool {
	return &mockTool{name: name, exec: func(context.Context, json.RawMessage) (ToolResult, error) {
		if calls != nil {
			*calls++
		}
		return ToolResult{Content: content}, nil
	}}
}

func generationCall(name string) ToolCall {
	return ToolCall{ID: "call-generation", Type: "function", Function: FunctionCall{Name: name, Arguments: `{}`}}
}

func TestRegistryGenerationRejectsReplacedToolBinding(t *testing.T) {
	registry := NewRegistry()
	var oldCalls, newCalls int
	oldTool := generationTool("replace_me", "old", &oldCalls)
	newTool := generationTool("replace_me", "new", &newCalls)
	if err := registry.Register(oldTool); err != nil {
		t.Fatal(err)
	}
	oldGeneration, ok := registry.Generation("replace_me")
	if !ok || oldGeneration == 0 {
		t.Fatalf("geração inicial=%d presente=%v", oldGeneration, ok)
	}
	if !registry.Unregister("replace_me") {
		t.Fatal("unregister não removeu a tool")
	}
	if _, ok := registry.Generation("replace_me"); ok {
		t.Fatal("geração removida continuou publicada")
	}
	if err := registry.Register(newTool); err != nil {
		t.Fatal(err)
	}
	newGeneration, ok := registry.Generation("replace_me")
	if !ok || newGeneration <= oldGeneration {
		t.Fatalf("geração após replace=%d, anterior=%d, presente=%v", newGeneration, oldGeneration, ok)
	}

	result := NewExecutor(registry, ExecutorConfig{ExpectedToolGeneration: oldGeneration}).ExecuteOne(context.Background(), generationCall("replace_me"))
	if result.ErrorKind != ErrorKindAuthorization || result.Result.Failure == nil || result.Result.Failure.Code != "tool_generation_mismatch" {
		t.Fatalf("replace não recusado por autorização: %+v", result)
	}
	if oldCalls != 0 || newCalls != 0 {
		t.Fatalf("tool executada apesar do mismatch: old=%d new=%d", oldCalls, newCalls)
	}
}

func TestRegistryGenerationIgnoresOtherToolChangesAndZeroIsLegacy(t *testing.T) {
	registry := NewRegistry()
	var targetCalls int
	if err := registry.Register(generationTool("target", "target", &targetCalls)); err != nil {
		t.Fatal(err)
	}
	targetGeneration, ok := registry.Generation("target")
	if !ok {
		t.Fatal("geração da target ausente")
	}
	if err := registry.Register(generationTool("other", "other", nil)); err != nil {
		t.Fatal(err)
	}
	if !registry.Unregister("other") || registry.Unregister("other") {
		t.Fatal("ciclo unregister de other inconsistente")
	}
	if err := registry.Register(generationTool("other", "other-new", nil)); err != nil {
		t.Fatal(err)
	}

	boundConfig := DefaultExecutorConfig()
	boundConfig.ExpectedToolGeneration = targetGeneration
	bound := NewExecutor(registry, boundConfig).ExecuteOne(context.Background(), generationCall("target"))
	if bound.Error != nil || bound.ErrorKind != ErrorKindNone || targetCalls != 1 {
		t.Fatalf("mudança de outra tool invalidou target: %+v calls=%d", bound, targetCalls)
	}
	legacy := NewExecutor(registry, DefaultExecutorConfig()).ExecuteOne(context.Background(), generationCall("target"))
	if legacy.Error != nil || legacy.ErrorKind != ErrorKindNone || targetCalls != 2 {
		t.Fatalf("ExpectedToolGeneration=0 não preservou legado: %+v calls=%d", legacy, targetCalls)
	}
}

func TestRegistryGenerationRejectsCounterOverflow(t *testing.T) {
	registry := NewRegistry()
	registry.generationCounter = math.MaxUint64
	if err := registry.Register(generationTool("overflow", "", nil)); err == nil {
		t.Fatal("overflow de geração foi aceito")
	}
	if registry.Has("overflow") {
		t.Fatal("tool foi publicada após overflow")
	}
}

func TestRegistryOptInPublicationIsAtomicWithGeneration(t *testing.T) {
	registry := NewRegistry()
	optIn := generationTool("opt_in_atomic", "opt-in", nil)
	if err := registry.RegisterOptIn(optIn); err != nil {
		t.Fatal(err)
	}
	if generation, ok := registry.Generation(optIn.Name()); !ok || generation == 0 {
		t.Fatalf("geração opt-in não publicada: generation=%d ok=%v", generation, ok)
	}
	if got := registry.All(); len(got) != 0 {
		t.Fatalf("tool opt-in apareceu no conjunto padrão: %+v", got)
	}
	if got := registry.Discoverable(); len(got) != 0 {
		t.Fatalf("opt-in não descobrível apareceu no catálogo: %+v", got)
	}

	discoverable := generationTool("discoverable_atomic", "discoverable", nil)
	if err := registry.RegisterDiscoverableOptIn(discoverable); err != nil {
		t.Fatal(err)
	}
	if generation, ok := registry.Generation(discoverable.Name()); !ok || generation == 0 {
		t.Fatalf("geração discoverable não publicada: generation=%d ok=%v", generation, ok)
	}
	if got := registry.All(); len(got) != 0 {
		t.Fatalf("discoverable opt-in apareceu no conjunto padrão: %+v", got)
	}
	got := registry.Discoverable()
	if len(got) != 1 || got[0].Name() != discoverable.Name() {
		t.Fatalf("discoverable não foi publicado junto das flags: %+v", got)
	}
}
