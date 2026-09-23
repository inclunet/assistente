package app

import (
	"context"
	"testing"

	"assistente/internal/commandactivation"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"github.com/google/uuid"
)

// installJobPaletteSuppress instala o delta na camada que pertence à claim,
// sem reconstruir manualmente o HostState. O execute seguinte deve detectar a
// geração persistida e fazer o preflight/refresh da projeção de claims.
func installJobPaletteSuppress(t *testing.T, a *App, claim commandactivation.Claim) string {
	t.Helper()
	p := a.commandProduct.Load()
	if p == nil {
		t.Fatal("produto de comandos ausente")
	}
	projection, err := commandProductProjection(p.registry, nil)
	if err != nil || len(projection.BuiltinLayers) != 2 {
		t.Fatalf("projeção builtin inválida: err=%v", err)
	}
	var defaultBindingID, defaultVersion, defaultFingerprint string
	for _, binding := range projection.BuiltinLayers[0].Defaults {
		if binding.Candidate.CommandID == commandProductWorkspaceListID {
			defaultBindingID = binding.Candidate.ID
			defaultVersion = binding.Version
			defaultFingerprint = binding.Fingerprint
			break
		}
	}
	if defaultBindingID == "" {
		t.Fatal("default workspace.list ausente")
	}

	binding := commandconfig.Binding{
		ID:           uuid.Must(uuid.NewV7()).String(),
		UserID:       claim.UserID,
		WorkspaceID:  cloneCommandWorkspace(claim.WorkspaceID),
		LayerRefKind: string(claim.LayerRefKind),
		LayerRef:     claim.LayerRef,
		TriggerType:  string(commandcatalog.Palette),
		TriggerSpec:  `{"version":1,"selection":"workspace.list"}`,
		Arguments:    `{}`,
		Condition:    `{"version":1,"clauses":[]}`,
		Effect:       "suppress",
		Enabled:      true,
		Source:       "user",
		ReviewStatus: "active",
		Presentation: `{"version":1}`,
	}
	binding.ReplacesDefaultID = paletteStringPtr(defaultBindingID)
	binding.ReplacesDefaultVersion = paletteStringPtr(defaultVersion)
	binding.ReplacesDefaultFingerprint = paletteStringPtr(defaultFingerprint)
	if err := database.DB().Create(&binding).Error; err != nil {
		t.Fatalf("inserir suppress na camada da claim: %v", err)
	}
	var generation commandconfig.Generation
	query := database.DB().Where("user_id = ?", claim.UserID)
	if claim.WorkspaceID == nil {
		query = query.Where("workspace_id IS NULL")
	} else {
		query = query.Where("workspace_id = ?", *claim.WorkspaceID)
	}
	if err := query.First(&generation).Error; err != nil {
		t.Fatalf("ler geração da configuração: %v", err)
	}
	if err := database.DB().Model(&commandconfig.Generation{}).Where("id = ?", generation.ID).
		Update("generation", generation.Generation+1).Error; err != nil {
		t.Fatalf("avançar geração da configuração: %v", err)
	}
	return binding.ID
}

func executePaletteResolution(t *testing.T, a *App) commandledger.FullRecord {
	t.Helper()
	p := a.commandProduct.Load()
	if p == nil {
		t.Fatal("produto de comandos ausente")
	}
	record, err := p.execute(context.Background(), paletteResolutionCandidate(t))
	if err != nil {
		t.Fatalf("resolver/executar palette: %v", err)
	}
	return record
}

func TestCommandJobClaimRefreshesPaletteResolutionAfterRuntimeTerminal(t *testing.T) {
	a := commandJobPublicationApp(t)
	// O mapa inicial é exercitado antes de existir uma claim. A alteração SQL
	// do fixture acontece antes do primeiro refresh que observa a claim; o teste
	// não atribui ao resolvedor detecção de escrita externa após publicação.
	first := executePaletteResolution(t, a)
	if first.Status != commandledger.Succeeded || first.Envelope.CommandID == nil || *first.Envelope.CommandID != commandProductWorkspaceListID {
		t.Fatalf("default inicial não executou: status=%s command=%v", first.Status, first.Envelope.CommandID)
	}

	control := startCommandMaintenanceLiveJob(t, a)
	runID := <-control.RunID
	claim, _ := waitLiveClaimAndLease(t, runID)
	// Este caso verifica refresh por configuração e encerramento do runtime,
	// não disputa com uma nova revisão de claim durante a admissão. Drene a
	// manutenção antes de instalar o delta; o job permanece vivo e bloqueado.
	if err := a.jobMgr.CloseCommandMaintenance(context.Background()); err != nil {
		t.Fatal(err)
	}
	installJobPaletteSuppress(t, a, claim)

	suppressed := executePaletteResolution(t, a)
	if suppressed.Status != commandledger.Suppressed || suppressed.Envelope.CommandID != nil {
		t.Fatalf("claim não projetou suppress sem rebuild explícito: status=%s command=%v", suppressed.Status, suppressed.Envelope.CommandID)
	}

	control.Release()
	if result, err := control.Join(); err != nil || result == nil || result.Status != jobs.RunStatusCompleted {
		t.Fatalf("job não chegou ao terminal: result=%+v err=%v", result, err)
	}
	var current commandactivation.Claim
	if err := database.DB().Where("activation_id = ?", claim.ActivationID).Take(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.State != commandactivation.StateActive {
		t.Fatalf("Consumer reconciliou a claim apesar da manutenção fechada: %+v", current)
	}
	// Join prova o fim do runtime. A manutenção está fechada, portanto não
	// aguardamos Consumer, lease nem outbox: o preflight deve invalidar a claim
	// pela prova runtime e devolver o default.
	last := executePaletteResolution(t, a)
	if last.Status != commandledger.Succeeded || last.Envelope.CommandID == nil || *last.Envelope.CommandID != commandProductWorkspaceListID {
		t.Fatalf("runtime terminal manteve suppress sem Consumer: status=%s command=%v", last.Status, last.Envelope.CommandID)
	}
}

func TestCommandJobConditionGuardRefreshesPaletteResolution(t *testing.T) {
	a := commandJobPublicationApp(t)
	if err := a.workspaceMgr.SetProfile("job-required-profile"); err != nil {
		t.Fatal(err)
	}
	control := startCommandJobWithCondition(t, a, `{"version":1,"clauses":[{"field":"profile","op":"eq","value":"job-required-profile"}]}`)
	runID := <-control.RunID
	claim, _ := waitLiveClaimAndLease(t, runID)
	// Como no caso de runtime terminal acima, este teste prova o refresh do
	// guard, não uma admissão concorrente com nova revisão da claim. Drene
	// antes de instalar o delta e executar a primeira resolução.
	if err := a.jobMgr.CloseCommandMaintenance(context.Background()); err != nil {
		t.Fatal(err)
	}
	installJobPaletteSuppress(t, a, claim)

	suppressed := executePaletteResolution(t, a)
	if suppressed.Status != commandledger.Suppressed || suppressed.Envelope.CommandID != nil {
		t.Fatalf("condição satisfeita não projetou suppress: status=%s command=%v", suppressed.Status, suppressed.Envelope.CommandID)
	}
	if err := a.workspaceMgr.SetProfile("different-profile"); err != nil {
		t.Fatal(err)
	}
	control.Release()
	if result, err := control.Join(); err != nil || result == nil || result.Status != jobs.RunStatusCompleted {
		t.Fatalf("job condicionado não encerrou: result=%+v err=%v", result, err)
	}
	var current commandactivation.Claim
	if err := database.DB().Where("activation_id = ?", claim.ActivationID).Take(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.State != commandactivation.StateActive {
		t.Fatalf("Consumer reconciliou a claim condicionada apesar da manutenção fechada: %+v", current)
	}
	// A troca de perfil altera o guard do workspace. Com a manutenção parada,
	// somente o refresh/preflight pode retirar a camada da resolução.
	last := executePaletteResolution(t, a)
	if last.Status != commandledger.Succeeded || last.Envelope.CommandID == nil || *last.Envelope.CommandID != commandProductWorkspaceListID {
		t.Fatalf("condição alterada manteve suppress: status=%s command=%v", last.Status, last.Envelope.CommandID)
	}
}
