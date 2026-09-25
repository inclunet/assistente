package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandportability"
)

func TestRenderDesktopImportDiffDetectaCamposSemExporArgumentos(t *testing.T) {
	const (
		beforeArguments = `{"secret":"before-secret"}`
		afterArguments  = `{"secret":"after-secret"}`
	)

	before := commandconfig.Binding{
		ID:          "binding-1",
		TriggerType: "hotkey",
		TriggerSpec: "ctrl+o",
		Arguments:   beforeArguments,
		Condition:   "workspace == old",
		Enabled:     false,
	}
	after := before
	after.TriggerSpec = "ctrl+n"
	after.Arguments = afterArguments
	after.Condition = "workspace == new"
	after.Enabled = true

	rendered, err := renderDesktopImportDiff(commandconfig.MutationDiff{
		Scope:          commandconfig.Scope{UserID: "user-1"},
		BeforeBindings: []commandconfig.Binding{before},
		AfterBindings:  []commandconfig.Binding{after},
	})
	if err != nil {
		t.Fatalf("renderDesktopImportDiff() error = %v", err)
	}

	for _, field := range []string{"trigger", "condition", "enabled"} {
		if !strings.Contains(rendered, field) {
			t.Errorf("diff não registrou o campo alterado %q: %q", field, rendered)
		}
	}
	for _, secret := range []string{beforeArguments, afterArguments, "before-secret", "after-secret"} {
		if strings.Contains(rendered, secret) {
			t.Errorf("diff expôs argumento/segredo %q: %q", secret, rendered)
		}
	}
}

func TestRenderDesktopImportDiffDetectaEnabledDaCamada(t *testing.T) {
	before := commandconfig.Layer{ID: "layer-1", Enabled: false}
	after := before
	after.Enabled = true

	rendered, err := renderDesktopImportDiff(commandconfig.MutationDiff{
		Scope:        commandconfig.Scope{UserID: "user-1"},
		BeforeLayers: []commandconfig.Layer{before},
		AfterLayers:  []commandconfig.Layer{after},
	})
	if err != nil {
		t.Fatalf("renderDesktopImportDiff() error = %v", err)
	}
	if !strings.Contains(rendered, "~layer-1[enabled]") {
		t.Fatalf("diff não registrou enabled da camada: %q", rendered)
	}
}

func TestRenderDesktopImportDiffNaoMarcaPonteirosIguaisComoAlteracao(t *testing.T) {
	beforeCommand := "command-1"
	afterCommand := "command-1"
	beforeWorkspace := "workspace-1"
	afterWorkspace := "workspace-1"

	beforeLayer := commandconfig.Layer{ID: "layer-1", WorkspaceID: &beforeWorkspace}
	afterLayer := commandconfig.Layer{ID: "layer-1", WorkspaceID: &afterWorkspace}
	beforeBinding := commandconfig.Binding{ID: "binding-1", CommandID: &beforeCommand}
	afterBinding := commandconfig.Binding{ID: "binding-1", CommandID: &afterCommand}

	rendered, err := renderDesktopImportDiff(commandconfig.MutationDiff{
		Scope:          commandconfig.Scope{UserID: "user-1"},
		BeforeLayers:   []commandconfig.Layer{beforeLayer},
		AfterLayers:    []commandconfig.Layer{afterLayer},
		BeforeBindings: []commandconfig.Binding{beforeBinding},
		AfterBindings:  []commandconfig.Binding{afterBinding},
	})
	if err != nil {
		t.Fatalf("renderDesktopImportDiff() error = %v", err)
	}
	for _, changed := range []string{"~layer-1", "~binding-1"} {
		if strings.Contains(rendered, changed) {
			t.Fatalf("ponteiros com mesmo conteúdo geraram alteração %q: %q", changed, rendered)
		}
	}
}

func TestSafeCommandImportErrorRedactsWrappedErrors(t *testing.T) {
	const secret = "wrapped-secret"

	known := fmt.Errorf("detalhe interno %s: %w", secret, commandportability.ErrInvalid)
	if got := safeCommandImportError(known); got != commandportability.ErrInvalid {
		t.Fatalf("erro conhecido embrulhado = %v, want %v", got, commandportability.ErrInvalid)
	}
	if got := safeCommandImportError(known); strings.Contains(got.Error(), secret) {
		t.Fatalf("erro conhecido expôs detalhe interno: %v", got)
	}

	unknown := fmt.Errorf("falha de armazenamento %s: %w", secret, errors.New("database failure"))
	if got := safeCommandImportError(unknown); got != commandexecution.ErrInvalidConfiguration {
		t.Fatalf("erro desconhecido embrulhado = %v, want %v", got, commandexecution.ErrInvalidConfiguration)
	}
	if got := safeCommandImportError(unknown); strings.Contains(got.Error(), secret) {
		t.Fatalf("erro desconhecido expôs detalhe interno: %v", got)
	}
	if safeCommandImportError(nil) != nil {
		t.Fatal("safeCommandImportError(nil) deve retornar nil")
	}
}

func TestCommandImportResultReportRedacted(t *testing.T) {
	const secret = "payload-secret"

	result := commandImportResult(commandMutationBatchResult{
		Diffs: []commandconfig.MutationDiff{{
			BeforeBindings: []commandconfig.Binding{{Arguments: secret}},
			AfterBindings:  []commandconfig.Binding{{Arguments: secret}},
		}},
		Report: &commandportability.ImportReport{
			Version: 2,
			Layers: []commandportability.ImportLayerReport{{
				TargetID: "layer-1",
				Scope: commandportability.PortableScope{
					Kind:        commandportability.WorkspaceScope,
					WorkspaceID: "workspace-1",
				},
				Action: commandportability.ReplaceMode,
			}},
			Warnings: []commandportability.ImportWarningCount{{Code: "warning.code", Count: 1}},
		},
	}, []commandportability.LayerExport{{
		ID:   "layer-1",
		Name: secret,
		Bindings: []commandportability.BindingExport{{
			Arguments: secret,
			Condition: secret,
		}},
	}})

	serialized, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal(commandImportResult()) error = %v", err)
	}
	if strings.Contains(string(serialized), secret) {
		t.Fatalf("resultado expôs payload bruto: %s", serialized)
	}
	if !strings.Contains(string(serialized), `"targetId":"layer-1"`) {
		t.Fatalf("resultado perdeu o resumo seguro do relatório: %s", serialized)
	}
}
