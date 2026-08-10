package acpinstall

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/acpregistry"
)

func TestInstallBinarioGravaEnvNoInstalledJSON(t *testing.T) {
	pacote := pacoteDoOpencode(t)
	agente := agenteOpencode(t, digestDe(pacote))
	plataforma := PlatformTarget()
	alvo := agente.Distribution.Binary[plataforma]
	alvo.Env = map[string]string{
		"VT_ACP_ENABLED":     "true",
		"VT_ACP_ZED_ENABLED": "true",
	}
	agente.Distribution.Binary[plataforma] = alvo

	c := montar(t, opcoes{
		agentes: []acpregistry.Agent{agente},
		runtime: runtimeSemNode,
		http:    &clienteFalso{corpo: pacote},
	})

	instalacao, err := c.instalador.Install(context.Background(), opencodeID, Confirmed{
		Distribution: DistributionBinary,
		Origin:       opencodeURL,
		SHA256:       digestDe(pacote),
	})
	if err != nil {
		t.Fatalf("instalação: %v", err)
	}
	if instalacao.Env["VT_ACP_ENABLED"] != "true" || instalacao.Env["VT_ACP_ZED_ENABLED"] != "true" {
		t.Fatalf("env na instalação = %#v", instalacao.Env)
	}

	dados, err := os.ReadFile(filepath.Join(instalacao.Dir, installedFileName))
	if err != nil {
		t.Fatal(err)
	}
	var gravado Installation
	if err := json.Unmarshal(dados, &gravado); err != nil {
		t.Fatal(err)
	}
	if gravado.Env["VT_ACP_ENABLED"] != "true" {
		t.Errorf("env no installed.json = %#v", gravado.Env)
	}
}

func TestInstallBinarioSemEnvNaoQuebra(t *testing.T) {
	pacote := pacoteDoOpencode(t)
	agente := agenteOpencode(t, digestDe(pacote))
	c := montar(t, opcoes{
		agentes: []acpregistry.Agent{agente},
		runtime: runtimeSemNode,
		http:    &clienteFalso{corpo: pacote},
	})

	instalacao, err := c.instalador.Install(context.Background(), opencodeID, Confirmed{
		Distribution: DistributionBinary,
		Origin:       opencodeURL,
		SHA256:       digestDe(pacote),
	})
	if err != nil {
		t.Fatalf("instalação sem env: %v", err)
	}
	if instalacao.Env != nil && len(instalacao.Env) != 0 {
		t.Errorf("env = %#v, queria nil ou vazio", instalacao.Env)
	}
}
