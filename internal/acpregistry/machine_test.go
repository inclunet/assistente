package acpregistry

import (
	"slices"
	"testing"

	"assistente/internal/acp"
)

func TestPlatformForTraduzOAlvoDoRegistro(t *testing.T) {
	casos := []struct {
		goos, goarch, quer string
	}{
		{"windows", "amd64", "windows-x86_64"},
		{"windows", "arm64", "windows-aarch64"},
		{"darwin", "arm64", "darwin-aarch64"},
		{"linux", "amd64", "linux-x86_64"},
		// O Go compila para mais alvos do que o formato do registro nomeia, e um
		// nome parecido escolheria o artefato de outra máquina.
		{"linux", "386", ""},
		{"freebsd", "amd64", ""},
	}
	for _, caso := range casos {
		if got := platformFor(caso.goos, caso.goarch); got != caso.quer {
			t.Errorf("platformFor(%q, %q) = %q, quer %q", caso.goos, caso.goarch, got, caso.quer)
		}
	}
}

func TestDetectableKindMapeiaSoOsAgentesQueADeteccaoConhece(t *testing.T) {
	// O mapeamento é curto por decisão (D1/D11): nenhum agente novo ganha
	// detecção própria, e é isso que faz a tela dizer "o app não sabe procurar"
	// em vez de "não encontrado" para os outros.
	if kind, ok := DetectableKind("cursor"); !ok || kind != acp.AgentKindCursor {
		t.Errorf("cursor = (%q, %v), quer o tipo do Cursor", kind, ok)
	}
	if kind, ok := DetectableKind("claude-acp"); !ok || kind != acp.AgentKindClaudeCode {
		t.Errorf("claude-acp = (%q, %v), quer o tipo do Claude Code", kind, ok)
	}
	// O `id` do registro e o tipo de provider do app são vocabulários
	// diferentes: `claude-code` é tipo de provider, e não linha do índice.
	if _, ok := DetectableKind("claude-code"); ok {
		t.Error("o tipo de provider do app foi aceito como id do registro")
	}
	if _, ok := DetectableKind("codex-acp"); ok {
		t.Error("agente sem detecção escrita à mão apareceu como detectável")
	}
}

func TestDetectableKindsNaoRepeteEEstavel(t *testing.T) {
	kinds := DetectableKinds()
	if len(kinds) != 2 {
		t.Fatalf("tipos detectáveis = %v, quer dois", kinds)
	}
	if !slices.IsSorted(kinds) {
		t.Errorf("tipos detectáveis fora de ordem: %v", kinds)
	}
	if slices.Equal(kinds, DetectableKinds()) == false {
		t.Error("a lista mudou entre duas chamadas")
	}
}

// agenteBinario monta um agente distribuído como binário nos alvos pedidos, com
// digest onde o teste pedir.
func agenteBinario(alvos map[string]string) Agent {
	binary := make(map[string]BinaryTarget, len(alvos))
	for alvo, digest := range alvos {
		binary[alvo] = BinaryTarget{
			Archive: "https://cdn.exemplo/agente.tar.gz",
			SHA256:  digest,
			Cmd:     "./agente",
		}
	}
	return Agent{ID: "agente", Name: "Agente", Distribution: Distribution{Binary: binary}}
}

const digestQualquer = "3b1f2e0a7c6d5b4a39281706f5e4d3c2b1a09f8e7d6c5b4a39281706f5e4d3c2"

func TestFitForBinarioComDigestNaoExigeRuntime(t *testing.T) {
	agent := agenteBinario(map[string]string{"windows-x86_64": digestQualquer})

	fit := FitFor(agent, "windows-x86_64")
	if fit.Integrity != IntegrityDigest {
		t.Errorf("integridade = %q, quer %q", fit.Integrity, IntegrityDigest)
	}
	if fit.Runtime != "" {
		t.Errorf("runtime = %q, quer nenhum: binário não depende de interpretador", fit.Runtime)
	}
	if fit.PlatformTarget != "windows-x86_64" {
		t.Errorf("alvo = %q, quer o desta plataforma", fit.PlatformTarget)
	}
	if !slices.Equal(fit.Distributions, []string{DistributionBinary}) {
		t.Errorf("distribuições = %v, quer só binary", fit.Distributions)
	}
}

func TestFitForBinarioSemDigestFicaSemInstalacaoAutomatica(t *testing.T) {
	// É o caso do Cursor: o agente principal do app, sem digest em nenhum alvo
	// (D4). Ele não exige runtime, e a tela não pode chamar isso de erro.
	agent := agenteBinario(map[string]string{"windows-x86_64": ""})

	fit := FitFor(agent, "windows-x86_64")
	if fit.Integrity != IntegrityNoDigest {
		t.Errorf("integridade = %q, quer %q", fit.Integrity, IntegrityNoDigest)
	}
	if fit.Runtime != "" {
		t.Errorf("runtime = %q, quer nenhum", fit.Runtime)
	}
}

func TestFitForBinarioSemAlvoParaEstaPlataforma(t *testing.T) {
	// Windows ARM: só 7 dos 17 agentes com binário publicam alvo. Dizer que não
	// há alvo é diferente de dizer que não há digest.
	agent := agenteBinario(map[string]string{"linux-x86_64": digestQualquer})

	fit := FitFor(agent, "windows-aarch64")
	if fit.Integrity != IntegrityNoPlatformTarget {
		t.Errorf("integridade = %q, quer %q", fit.Integrity, IntegrityNoPlatformTarget)
	}
	if fit.PlatformTarget != "" {
		t.Errorf("alvo = %q, quer vazio", fit.PlatformTarget)
	}
}

func TestFitForPlataformaDesconhecidaNaoCasaComAlvoNenhum(t *testing.T) {
	// Sistema que o registro não nomeia não pode casar com o primeiro alvo do
	// mapa: instalaria o artefato de outra arquitetura.
	agent := agenteBinario(map[string]string{"linux-x86_64": digestQualquer})

	fit := FitFor(agent, "")
	if fit.Integrity != IntegrityNoPlatformTarget {
		t.Errorf("integridade = %q, quer %q", fit.Integrity, IntegrityNoPlatformTarget)
	}
}

func TestFitForPacoteNPMExigeNode(t *testing.T) {
	agent := Agent{
		ID:           "codex-acp",
		Name:         "Codex",
		Distribution: Distribution{NPX: &PackageDistribution{Package: "@agentclientprotocol/codex-acp@1.1.9"}},
	}

	fit := FitFor(agent, "windows-x86_64")
	if fit.Runtime != acp.RuntimeNode {
		t.Errorf("runtime = %q, quer %q", fit.Runtime, acp.RuntimeNode)
	}
	if fit.Integrity != IntegrityNotDistributed {
		t.Errorf("integridade = %q, quer %q: não há binário de que falar", fit.Integrity, IntegrityNotDistributed)
	}
	if !slices.Equal(fit.Distributions, []string{DistributionNPX}) {
		t.Errorf("distribuições = %v, quer só npx", fit.Distributions)
	}
}

func TestFitForPacoteUVExigeUV(t *testing.T) {
	agent := Agent{
		ID:           "fast-agent",
		Name:         "fast-agent",
		Distribution: Distribution{UVX: &PackageDistribution{Package: "fast-agent-mcp"}},
	}

	if fit := FitFor(agent, "linux-x86_64"); fit.Runtime != acp.RuntimeUV {
		t.Errorf("runtime = %q, quer %q", fit.Runtime, acp.RuntimeUV)
	}
}

func TestFitForBinarioSemDigestComPacoteNPMCaiNoNode(t *testing.T) {
	// Dois agentes do catálogo publicam binário e npm. Quando o binário desta
	// plataforma não tem digest, o app não pode instalá-lo (D4), e o caminho que
	// sobra é o pacote npm — que exige Node. Dizer "nenhum requisito" aqui
	// esconderia o que de fato falta.
	agent := agenteBinario(map[string]string{"windows-x86_64": ""})
	agent.Distribution.NPX = &PackageDistribution{Package: "algum-agente@1.0.0"}

	fit := FitFor(agent, "windows-x86_64")
	if fit.Runtime != acp.RuntimeNode {
		t.Errorf("runtime = %q, quer %q", fit.Runtime, acp.RuntimeNode)
	}
	if !slices.Equal(fit.Distributions, []string{DistributionBinary, DistributionNPX}) {
		t.Errorf("distribuições = %v, quer binary e npx nessa ordem", fit.Distributions)
	}
}

func TestFitForBinarioComDigestVenceOPacoteNPM(t *testing.T) {
	agent := agenteBinario(map[string]string{"linux-x86_64": digestQualquer})
	agent.Distribution.NPX = &PackageDistribution{Package: "algum-agente@1.0.0"}

	if fit := FitFor(agent, "linux-x86_64"); fit.Runtime != "" {
		t.Errorf("runtime = %q, quer nenhum: o binário conferível é o caminho daqui", fit.Runtime)
	}
}
