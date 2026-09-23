package wailsapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"assistente/internal/core/ports"
	"assistente/internal/portability"
)

func commandExportAPI(session Session) *ExportImport {
	api := NewExportImport()
	AttachExportImport(api, session, nil, func() ports.SystemDialogPort { return nil }, "dev", nil)
	return api
}

func TestCommandExportCallbackRecebeRequestEContextoAutenticado(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxUID, "user-1")
	api := commandExportAPI(stubSession{ctx: ctx})
	req := portability.ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, OutputFormat: portability.FormatJSON}
	called := false
	AttachCommandExport(api, func(gotCtx context.Context, gotReq portability.ExportRequest) (string, error) {
		called = true
		if gotCtx.Value(ctxUID) != "user-1" {
			t.Fatalf("contexto autenticado não foi preservado")
		}
		if !reflect.DeepEqual(gotReq, req) {
			t.Fatalf("request recebido=%+v, want=%+v", gotReq, req)
		}
		return "command-export", nil
	})

	got, err := api.ExportData(req)
	if err != nil || got != "command-export" {
		t.Fatalf("resultado=%q erro=%v", got, err)
	}
	if !called {
		t.Fatal("callback não foi chamado")
	}
}

func TestCommandExportSemSessaoNaoChamaCallback(t *testing.T) {
	api := commandExportAPI(stubSession{err: errors.New("sessão ausente")})
	called := false
	AttachCommandExport(api, func(context.Context, portability.ExportRequest) (string, error) {
		called = true
		return "não deveria", nil
	})

	if _, err := api.ExportData(portability.ExportRequest{IncludeCommandLayers: true}); err == nil {
		t.Fatal("export sem sessão foi aceito")
	}
	if called {
		t.Fatal("callback executou sem sessão")
	}
}

func TestCommandExportSemCallbackNaoFazFallback(t *testing.T) {
	api := commandExportAPI(stubSession{ctx: context.Background()})
	if _, err := api.ExportData(portability.ExportRequest{
		ExplicitSelection:    true,
		IncludeCommandLayers: true,
	}); !errors.Is(err, ErrExportImportNotWired) {
		t.Fatalf("erro=%v, want ErrExportImportNotWired", err)
	}
}

func TestCommandExportToFileEscreveSomenteOResultadoDoCallback(t *testing.T) {
	api := commandExportAPI(stubSession{ctx: context.Background()})
	want := "conteúdo produzido pelo callback"
	AttachCommandExport(api, func(context.Context, portability.ExportRequest) (string, error) {
		return want, nil
	})
	path := filepath.Join(t.TempDir(), "export.json")
	gotPath, err := api.ExportDataToFile(portability.ExportRequest{
		ExplicitSelection:    true,
		IncludeCommandLayers: true,
	}, path)
	if err != nil || gotPath != path {
		t.Fatalf("caminho=%q erro=%v", gotPath, err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("conteúdo=%q, want=%q", got, want)
	}
}

func TestCommandExportIDsSemIncludeNaoCaiNoGenerico(t *testing.T) {
	api := commandExportAPI(stubSession{ctx: context.Background()})
	called := false
	AttachCommandExport(api, func(context.Context, portability.ExportRequest) (string, error) {
		called = true
		return "não deveria passar pela validação", nil
	})
	_, err := api.ExportData(portability.ExportRequest{
		ExplicitSelection: true,
		CommandLayerIDs:   []string{"01926b90-7a5a-7c4e-8d3f-000000000001"},
	})
	if err == nil {
		t.Fatal("request sem includeCommandLayers foi aceito")
	}
	if called {
		t.Fatal("callback executou antes da validação dedicada")
	}
}

func TestCommandExportRequestsInvalidosNaoChamamCallbackNemAlteramArquivo(t *testing.T) {
	tests := []struct {
		name string
		req  portability.ExportRequest
	}{
		{name: "malformed", req: portability.ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, CommandLayerIDs: []string{"não-uuid"}}},
		{name: "mixed", req: portability.ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, ConversationIDs: []string{"conversation-1"}}},
		{name: "credential", req: portability.ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true, IncludeCredentials: true, CredentialExportPassword: "secret"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := commandExportAPI(stubSession{ctx: context.Background()})
			called := false
			AttachCommandExport(api, func(context.Context, portability.ExportRequest) (string, error) {
				called = true
				return "novo", nil
			})
			path := filepath.Join(t.TempDir(), "export.json")
			if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := api.ExportDataToFile(tt.req, path); err == nil {
				t.Fatal("request inválido foi aceito")
			}
			if called {
				t.Fatal("callback executou para request inválido")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "original" {
				t.Fatalf("arquivo foi alterado: %q", got)
			}
		})
	}
}

func TestCommandExportCallbackErrorNaoCriaArquivo(t *testing.T) {
	api := commandExportAPI(stubSession{ctx: context.Background()})
	wantErr := errors.New("falha no exportador")
	AttachCommandExport(api, func(context.Context, portability.ExportRequest) (string, error) {
		return "não gravar", wantErr
	})
	path := filepath.Join(t.TempDir(), "export.json")
	if _, err := api.ExportDataToFile(portability.ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true}, path); !errors.Is(err, wantErr) {
		t.Fatalf("erro=%v, want=%v", err, wantErr)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("arquivo criado após erro: %v", err)
	}
}

func TestCommandExportToFileSemCallbackFalhaFechado(t *testing.T) {
	api := commandExportAPI(stubSession{ctx: context.Background()})
	path := filepath.Join(t.TempDir(), "export.json")
	if _, err := api.ExportDataToFile(portability.ExportRequest{
		ExplicitSelection:    true,
		IncludeCommandLayers: true,
	}, path); !errors.Is(err, ErrExportImportNotWired) {
		t.Fatalf("erro=%v, want ErrExportImportNotWired", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("arquivo criado sem callback: %v", err)
	}
}
