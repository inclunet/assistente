package portability

import (
	"context"
	"strings"
	"testing"
)

// TestLegacyImportInspectAggregatesWarnings verifica que o hook Inspect roda
// apenas para itens importados e que seus avisos são agregados em Warnings,
// prefixados com tipo de recurso e nome (AEP-0072 Fase 5 / #123).
func TestLegacyImportInspectAggregatesWarnings(t *testing.T) {
	source := &memoryLegacyImportSource{
		files: []LegacyImportFile{
			{Name: "novo", Filename: "novo", Path: "novo"},
			{Name: "existente", Filename: "existente", Path: "existente"},
		},
		data: map[string][]byte{
			"novo":      []byte("conteudo-novo"),
			"existente": []byte("conteudo-existente"),
		},
	}

	res, err := ImportLegacyResourcesWithContext(context.Background(), LegacyImportRequest[string]{
		ResourceType: "skills",
		Source:       source,
		Parse: func(_ LegacyImportFile, data []byte) (string, error) {
			return string(data), nil
		},
		Import: func(_ context.Context, item string) (bool, error) {
			// "existente" é tratado como já presente (skipped).
			return item == "conteudo-novo", nil
		},
		Inspect: func(file LegacyImportFile, _ string) []string {
			return []string{"aviso de qualidade"}
		},
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Imported != 1 || res.Skipped != 1 {
		t.Fatalf("imported=%d skipped=%d want 1/1", res.Imported, res.Skipped)
	}
	// Inspect só roda para o item importado -> exatamente 1 warning.
	if len(res.Warnings) != 1 {
		t.Fatalf("warnings: got %d want 1 (%v)", len(res.Warnings), res.Warnings)
	}
	if !strings.Contains(res.Warnings[0], "skills novo:") {
		t.Errorf("warning deveria ser prefixado com tipo/nome: %q", res.Warnings[0])
	}
}
