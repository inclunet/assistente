package filesystem

import (
	"assistente/internal/fstrust"
	"assistente/internal/testutil/authorizationfixture"
	"assistente/internal/tools/invocationctx"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileMantemWorkspaceDaOrigemDepoisDeTrocarATela(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	for dir, value := range map[string]string{a: "conteudo-A", b: "conteudo-B"} {
		if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	previousRoot, previousAuth := sandboxRootFunc, pathAuthorizer
	t.Cleanup(func() { sandboxRootFunc = previousRoot; pathAuthorizer = previousAuth })
	active := a
	SetSandboxRootFunc(func() string { return active })
	m := fstrust.NewManager(authorizationfixture.Options(t, t.TempDir(), filepath.Join(a, ".assistente")))
	SetPathAuthorizer(fstrust.NewAuthorizer(m, nil))
	ctx := invocationctx.WithWorkspace(authorizationfixture.Context(), a)
	active = b
	tool := NewReadFile(b)
	result, err := tool.Execute(ctx, json.RawMessage(`{"path":"note.txt","raw":true}`))
	if err != nil || result.IsError || result.Content != "conteudo-A" {
		t.Fatalf("leitura seguiu interface: %+v %v", result, err)
	}
	args, err := json.Marshal(map[string]any{"path": filepath.Join(b, "note.txt"), "raw": true})
	if err != nil {
		t.Fatal(err)
	}
	result, err = tool.Execute(ctx, args)
	if err != nil || !result.IsError {
		t.Fatalf("workspace da interface virou sandbox da operação: %+v %v", result, err)
	}
	// Canais sem raiz capturada não herdam nem a interface nem o cwd do boot.
	channel := invocationctx.WithWorkspace(invocationctx.With(authorizationfixture.Context(), invocationctx.InvocationContext{Source: "channel"}), "")
	result, err = tool.Execute(channel, json.RawMessage(`{"path":"note.txt"}`))
	if err != nil || !result.IsError {
		t.Fatalf("canal herdou raiz relativa: %+v %v", result, err)
	}
	result, err = tool.Execute(channel, args)
	if err != nil || !result.IsError {
		t.Fatalf("canal herdou sandbox: %+v %v", result, err)
	}
}
