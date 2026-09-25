package wailsapi

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/core/ports"
	"assistente/internal/credentials"
	"assistente/internal/portability"
)

const commandImportWailsEnvelope = `{"version":2,"options":{},"resources":{"commandLayers":[{"id":"01926b90-7a5a-7c4e-8d3f-000000000001","scope":{"kind":"global"},"name":"Camada","enabled":true}]}}`

func TestCommandImportRespeitaAuthAntesDoCallback(t *testing.T) {
	wantErr := errors.New("sessão não autenticada")
	api := NewExportImport()
	AttachExportImport(api, stubSession{err: wantErr}, nil, func() ports.SystemDialogPort { return nil }, "dev", nil)
	called := false
	AttachCommandImport(api, func(context.Context, portability.ImportRequest) (*portability.ImportResult, error) {
		called = true
		return &portability.ImportResult{Success: true}, nil
	})

	if _, err := api.ImportData(commandImportWailsEnvelope, ""); !errors.Is(err, wantErr) {
		t.Fatalf("erro de autenticação=%v, want=%v", err, wantErr)
	}
	if called {
		t.Fatal("callback de commandLayers executou sem autenticação")
	}
}

func TestCommandImportSemCallbackNaoFazFallback(t *testing.T) {
	api := NewExportImport()
	AttachExportImport(api, stubSession{ctx: context.Background()}, nil, func() ports.SystemDialogPort { return nil }, "dev", nil)
	genericCalls := 0
	api.importConversations = func(context.Context, string, *credentials.Manager, string, []portability.ImportResolution, func([]string)) (*portability.ImportResult, error) {
		genericCalls++
		return nil, errors.New("fallback indevido")
	}
	if _, err := api.ImportData(commandImportWailsEnvelope, ""); !errors.Is(err, ErrExportImportNotWired) {
		t.Fatalf("callback ausente: erro=%v, want ErrExportImportNotWired", err)
	}
	if genericCalls != 0 {
		t.Fatal("import genérico foi usado sem callback de commandLayers")
	}
}

func TestCommandImportDuplicatedResourcesNaoInvocaGenericoNemRestore(t *testing.T) {
	raw := `{"resources":{"commandLayers":[{"id":"01926b90-7a5a-7c4e-8d3f-000000000001","scope":{"kind":"global"},"name":"Camada","enabled":true}]},"resources":{"conversations":[]}}`
	api := NewExportImport()
	AttachExportImport(api, stubSession{ctx: context.Background()}, nil, func() ports.SystemDialogPort { return nil }, "dev", func(context.Context) (func([]string), error) {
		t.Fatal("prepareConversationRestore foi chamado para commandLayers")
		return nil, nil
	})
	genericCalls := 0
	api.importConversations = func(context.Context, string, *credentials.Manager, string, []portability.ImportResolution, func([]string)) (*portability.ImportResult, error) {
		genericCalls++
		return nil, errors.New("fallback indevido")
	}
	callbackCalls := 0
	AttachCommandImport(api, func(_ context.Context, req portability.ImportRequest) (*portability.ImportResult, error) {
		callbackCalls++
		_, err := portability.ParseCommandImportEnvelope([]byte(req.JSONData))
		return nil, err
	})

	if _, err := api.ImportData(raw, ""); err == nil {
		t.Fatal("envelope duplicado foi aceito")
	}
	if callbackCalls != 1 || genericCalls != 0 {
		t.Fatalf("roteamento incorreto: callback=%d genérico=%d", callbackCalls, genericCalls)
	}
}
