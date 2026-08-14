package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"assistente/internal/acp"
	"assistente/internal/acpinstall"
	"assistente/internal/apidto"
)

func TestACPInstallNotWired(t *testing.T) {
	t.Parallel()
	api := NewACPInstall()
	confirm := apidto.ACPInstallConfirmation{}
	if _, err := api.ACPAgentInstallPlan("codex-acp"); !errors.Is(err, ErrACPInstallNotWired) {
		t.Fatalf("ACPAgentInstallPlan: got %v", err)
	}
	if _, err := api.InstallACPAgent("codex-acp", confirm); !errors.Is(err, ErrACPInstallNotWired) {
		t.Fatalf("InstallACPAgent: got %v", err)
	}
	if _, err := api.UpdateACPAgent("codex-acp", confirm); !errors.Is(err, ErrACPInstallNotWired) {
		t.Fatalf("UpdateACPAgent: got %v", err)
	}
	if err := api.CancelACPAgentInstall("codex-acp"); !errors.Is(err, ErrACPInstallNotWired) {
		t.Fatalf("CancelACPAgentInstall: got %v", err)
	}
	if _, err := api.CanRemoveACPAgent("codex-acp"); !errors.Is(err, ErrACPInstallNotWired) {
		t.Fatalf("CanRemoveACPAgent: got %v", err)
	}
	if err := api.RemoveACPAgent("codex-acp"); !errors.Is(err, ErrACPInstallNotWired) {
		t.Fatalf("RemoveACPAgent: got %v", err)
	}
	if _, err := api.ListInstalledACPAgents(); !errors.Is(err, ErrACPInstallNotWired) {
		t.Fatalf("ListInstalledACPAgents: got %v", err)
	}
}

func TestACPInstallUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "acp_install.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("acp_install.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("acp_install.go deve chamar WithUser(session,")
	}
}

func TestPlanoTraduzidoNuncaEntregaListaAusente(t *testing.T) {
	t.Parallel()
	dto := installPlanDTO(acpinstall.Plan{AgentID: "codex-acp", Name: "Codex"}, false)

	if dto.RunArgs == nil {
		t.Error("argumentos de execução vieram nulos")
	}
	if dto.Installed != nil {
		t.Error("disse que havia instalação onde não há")
	}
}

func TestPlanoTraduzidoLevaOEstadoDeJaInstaladoEODaInstalacaoEmVoo(t *testing.T) {
	t.Parallel()
	quando := time.Date(2026, 8, 6, 15, 4, 5, 0, time.UTC)
	dto := installPlanDTO(acpinstall.Plan{
		AgentID:      "codex-acp",
		Name:         "Codex",
		Version:      "1.1.9",
		Distribution: acpinstall.DistributionNPM,
		Origin:       "@agentclientprotocol/codex-acp@1.1.9",
		Runtime:      acpinstall.RuntimeStatus{Name: acpinstall.RuntimeNode, Found: true, Path: "node"},
		Installed: &acpinstall.Installation{
			AgentID:     "codex-acp",
			Version:     "1.1.9",
			Command:     "node",
			InstalledAt: quando,
		},
	}, true)

	if dto.Installed == nil {
		t.Fatal("perdeu o estado de já instalado")
	}
	if dto.Installed.InstalledAt != "2026-08-06T15:04:05Z" {
		t.Errorf("data = %q, queria RFC 3339 para a tela formatar no idioma de quem lê", dto.Installed.InstalledAt)
	}
	if dto.Installed.Args == nil {
		t.Error("argumentos do comando instalado vieram nulos")
	}
	if !dto.Installing {
		t.Error("perdeu a instalação em voo, e então a tela não teria o que cancelar")
	}
}

func TestOPlanoTraduzidoLevaOAvisoDeVersaoNova(t *testing.T) {
	t.Parallel()
	dto := installPlanDTO(acpinstall.Plan{
		AgentID:      "codex-acp",
		Name:         "Codex",
		Version:      "1.2.0",
		Distribution: acpinstall.DistributionNPM,
		Installed:    &acpinstall.Installation{AgentID: "codex-acp", Version: "1.1.9", Command: "node"},
		Update:       true,
		CanUpdate:    false,
		UpdateReason: "o Node.js não foi encontrado nesta máquina",
	}, false)

	if !dto.Update {
		t.Error("perdeu o aviso de que o catálogo publica outra versão")
	}
	if dto.CanUpdate {
		t.Error("ofereceu atualizar contra o que o plano decidiu")
	}
	if dto.UpdateReason == "" {
		t.Error("botão indisponível sem o motivo à vista (D7)")
	}
}

func TestRuntimeAusenteChegaATelaComOndeSeProcurou(t *testing.T) {
	t.Parallel()
	dto := runtimeStatusDTO(acp.NodeRuntime{Searched: []string{`C:\Program Files\nodejs\node.exe`}})

	if dto.Found {
		t.Error("disse que achou o Node")
	}
	if dto.Name != acpinstall.RuntimeNode {
		t.Errorf("nome = %q, queria o do pré-requisito", dto.Name)
	}
	if len(dto.Searched) == 0 {
		t.Error("não disse onde procurou")
	}
}

func TestEmptyInstallPlanSempreTemRunArgs(t *testing.T) {
	t.Parallel()
	plano := emptyInstallPlan()
	if plano.RunArgs == nil {
		t.Error("run_args veio nulo")
	}
}
