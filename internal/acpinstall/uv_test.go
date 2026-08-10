package acpinstall

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestUVDescribeMostraAsDuasEtapas(t *testing.T) {
	uv := NewUV(runtimeComUV())
	dir := filepath.Join("C:", "dados", "agents", fastAgentID, fastAgentVersao)
	spec := fastAgentPacote + "==" + fastAgentVersao

	linha := uv.Describe(dir, spec)

	if !strings.Contains(linha, "venv") || !strings.Contains(linha, "pip install") {
		t.Errorf("linha = %q, queria uv venv e uv pip install", linha)
	}
	if !strings.Contains(linha, spec) {
		t.Errorf("linha = %q, queria a spec pinada", linha)
	}
	if !strings.Contains(linha, "&&") {
		t.Errorf("linha = %q, queria as duas etapas na mesma frase", linha)
	}
}

func TestUVDescribeVazioSemRuntime(t *testing.T) {
	if linha := NewUV(runtimeSemUV()).Describe("dir", "pacote==1.0.0"); linha != "" {
		t.Errorf("linha = %q, queria vazio sem uv", linha)
	}
}

func TestUVInstallSemRuntimeDizQueFaltaOUV(t *testing.T) {
	err := NewUV(runtimeSemUV()).Install(context.Background(), t.TempDir(), "pacote==1.0.0")
	if !errors.Is(err, ErrNoUV) {
		t.Errorf("erro = %v, queria a falta do uv", err)
	}
}

func TestPinnedUVSpecUsaVersaoDoItemQuandoPacoteNaoTraz(t *testing.T) {
	spec, name, version, err := pinnedUVSpec(agenteFastAgent())
	if err != nil {
		t.Fatalf("pinnedUVSpec: %v", err)
	}
	if name != fastAgentPacote || version != fastAgentVersao {
		t.Errorf("name/version = %s/%s, queria %s/%s", name, version, fastAgentPacote, fastAgentVersao)
	}
	if spec != fastAgentPacote+"=="+fastAgentVersao {
		t.Errorf("spec = %q, queria pinada com ==", spec)
	}
}

func TestPinnedUVSpecRecusaQuemNaoEUVX(t *testing.T) {
	_, _, _, err := pinnedUVSpec(agenteCodex())
	if !errors.Is(err, ErrNotUVX) {
		t.Errorf("erro = %v, queria ErrNotUVX", err)
	}
}

func TestPinnedUVSpecRespeitaVersaoJaNoPacote(t *testing.T) {
	agente := agenteFastAgent()
	agente.Distribution.UVX.Package = "goose-acp==1.0.0"
	agente.Version = "9.9.9"
	spec, _, version, err := pinnedUVSpec(agente)
	if err != nil {
		t.Fatalf("pinnedUVSpec: %v", err)
	}
	if version != "1.0.0" || spec != "goose-acp==1.0.0" {
		t.Errorf("spec/version = %s/%s, queria a do pacote e não a do item", spec, version)
	}
}
