package jobs

import "testing"

func TestRenderWithRoot_TaskFields(t *testing.T) {
	task := map[string]any{"code": "PROJ-1", "title": "Login", "link": "https://x/PROJ-1"}
	data := map[string]any{"task": task}

	got, err := RenderWithRoot(`{{ .task.link }}`, data)
	if err != nil {
		t.Fatalf("render link: %v", err)
	}
	if got != "https://x/PROJ-1" {
		t.Fatalf("link = %q", got)
	}

	payload, err := RenderWithRoot(`{"code": {{ json .task.code }}, "prompt": {{ json (printf "Investigue %s: %s" .task.code .task.title) }}}`, data)
	if err != nil {
		t.Fatalf("render payload: %v", err)
	}
	want := `{"code": "PROJ-1", "prompt": "Investigue PROJ-1: Login"}`
	if payload != want {
		t.Fatalf("payload = %q, want %q", payload, want)
	}
}

func TestEvaluateConditionWithRoot(t *testing.T) {
	withCode := map[string]any{"task": map[string]any{"code": "PROJ-1"}}
	noCode := map[string]any{"task": map[string]any{"code": ""}}

	ok, err := EvaluateConditionWithRoot(`{{ ne .task.code "" }}`, withCode)
	if err != nil || !ok {
		t.Fatalf("expected truthy for non-empty code, got ok=%v err=%v", ok, err)
	}
	ok, err = EvaluateConditionWithRoot(`{{ ne .task.code "" }}`, noCode)
	if err != nil || ok {
		t.Fatalf("expected falsy for empty code, got ok=%v err=%v", ok, err)
	}
	// Condição vazia é sempre verdadeira.
	if ok, _ := EvaluateConditionWithRoot("", noCode); !ok {
		t.Fatal("empty condition should be truthy")
	}
	// Condição só-whitespace NÃO é tratada como ausente (igual a EvaluateCondition):
	// é renderizada, vira string vazia e portanto falsy.
	if ok, _ := EvaluateConditionWithRoot("   ", noCode); ok {
		t.Fatal("whitespace-only condition should be falsy, not treated as empty/truthy")
	}
}
