package app

import (
	"context"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandledger"
	"assistente/internal/database"
)

func TestCommandJobMultiSourceRejectsDivergentChainsAndKeepsSurvivor(t *testing.T) {
	a := commandJobPublicationApp(t)
	first := startCommandJobWithConditionNamed(t, a, `{"version":1,"clauses":[]}`, liveCommandTool+".first")
	firstRunID := <-first.RunID
	firstClaim, _ := waitLiveClaimAndLease(t, firstRunID)
	firstClaims := waitLiveClaimsForRun(t, firstRunID, 1)

	second := startCommandJobWithConditionNamed(t, a, `{"version":1,"clauses":[]}`, liveCommandTool+".second")
	secondRunID := <-second.RunID
	secondClaim, _ := waitLiveClaimAndLease(t, secondRunID)
	if firstRunID == secondRunID || firstClaim.ActivationID == secondClaim.ActivationID {
		t.Fatalf("runs/claims não são independentes: first=(%s,%s) second=(%s,%s)", firstRunID, firstClaim.ActivationID, secondRunID, secondClaim.ActivationID)
	}
	secondClaims := waitLiveClaimsForRun(t, secondRunID, 2)
	firstIDs, secondIDs := make([]string, 0), make([]string, 0)
	firstLayers, secondLayers := map[string]bool{}, map[string]bool{}
	for _, claim := range firstClaims {
		firstIDs = append(firstIDs, claim.ActivationID)
		firstLayers[claim.LayerRef] = true
	}
	for _, claim := range secondClaims {
		secondIDs = append(secondIDs, claim.ActivationID)
		secondLayers[claim.LayerRef] = true
	}
	if len(secondLayers) < 2 {
		t.Fatalf("segundo run não ativou as duas camadas equivalentes: first=%v second=%v", firstLayers, secondLayers)
	}
	var survivorPrimary, survivorSecondary *commandactivation.Claim
	for i := range secondClaims {
		claim := &secondClaims[i]
		if claim.LayerRef == firstClaim.LayerRef && survivorPrimary == nil {
			survivorPrimary = claim
			continue
		}
		if survivorSecondary == nil && (survivorPrimary == nil || claim.LayerRef != survivorPrimary.LayerRef) {
			survivorSecondary = claim
		}
	}
	if survivorPrimary == nil || survivorSecondary == nil {
		t.Fatalf("claims do segundo run não cobrem layer compartilhada e layer distinta: first=%q claims=%+v", firstClaim.LayerRef, secondClaims)
	}

	// Congela somente a cadência enquanto as duas camadas equivalentes são
	// publicadas; os dois runtimes e suas leases continuam vivos.
	if err := a.jobMgr.CloseCommandMaintenance(context.Background()); err != nil {
		t.Fatal(err)
	}
	installJobPaletteExecute(t, a, firstClaim)
	installJobPaletteExecute(t, a, *survivorSecondary)

	divergent := executePaletteResolution(t, a)
	if divergent.Status != commandledger.Denied {
		t.Fatalf("cadeias divergentes foram aceitas: status=%s envelope=%+v", divergent.Status, divergent.Envelope)
	}
	if persisted := readInvocationProvenance(t, divergent.Envelope.InvocationID); persisted != nil {
		t.Fatalf("recusa divergente persistiu provenance de execução: %s", *persisted)
	}

	first.Release()
	if result, err := first.Join(); err != nil || result == nil || result.Status != "completed" {
		t.Fatalf("primeiro job não terminou: result=%+v err=%v", result, err)
	}
	drainCommandJobOutbox(t, a)
	for _, activationID := range firstIDs {
		if err := waitForNoLiveLease(activationID); err != nil {
			t.Fatal(err)
		}
	}

	survivor := executePaletteResolution(t, a)
	if survivor.Status != commandledger.Succeeded || survivor.Envelope.CommandID == nil || *survivor.Envelope.CommandID != commandProductWorkspaceListID {
		t.Fatalf("sobrevivente não resolveu workspace.list: status=%s envelope=%+v", survivor.Status, survivor.Envelope)
	}
	if survivor.Envelope.Provenance != nil {
		t.Fatal("FullRecord público expôs provenance da fonte sobrevivente")
	}
	provenance := requireJobEnvelopeProvenance(t, readInvocationProvenance(t, survivor.Envelope.InvocationID))
	if provenance["_chain_id"] != secondRunID {
		t.Fatalf("chain id do sobrevivente=%v, want %s", provenance["_chain_id"], secondRunID)
	}
	history, ok := provenance["_chain_history"].([]any)
	if !ok || len(history) == 0 || history[len(history)-1] != *secondClaim.SourceJobSlug {
		t.Fatalf("histórico do sobrevivente=%#v claim=%+v", provenance["_chain_history"], secondClaim)
	}
	chain, ok := provenance["command_chain_history"].([]any)
	if !ok || len(chain) != 1 {
		t.Fatalf("cadeia do sobrevivente=%#v", provenance["command_chain_history"])
	}
	entry, ok := chain[0].(map[string]any)
	if !ok || entry["command_id"] != commandProductWorkspaceListID || entry["invocation_id"] != survivor.Envelope.InvocationID {
		t.Fatalf("entrada da cadeia sobrevivente=%#v", entry)
	}
	layerRefs, ok := entry["layer_refs"].([]any)
	if !ok || len(layerRefs) != len(secondLayers) {
		t.Fatalf("layer_refs ausentes na cadeia sobrevivente: %#v", entry["layer_refs"])
	}
	selectedSurvivorLayer := false
	for layerRef := range secondLayers {
		if !containsStringValue(layerRefs, layerRef) {
			t.Fatalf("layer_ref sobrevivente %q ausente na cadeia: %#v", layerRef, layerRefs)
		}
		selectedSurvivorLayer = true
	}
	if !selectedSurvivorLayer {
		t.Fatalf("nenhuma layer do run sobrevivente foi selecionada na cadeia: layers=%v chain=%#v", secondLayers, layerRefs)
	}

	second.Release()
	if result, err := second.Join(); err != nil || result == nil || result.Status != "completed" {
		t.Fatalf("segundo job não terminou: result=%+v err=%v", result, err)
	}
	drainCommandJobOutbox(t, a)
	for _, activationID := range secondIDs {
		if err := waitForNoLiveLease(activationID); err != nil {
			t.Fatal(err)
		}
	}
}

func drainCommandJobOutbox(t *testing.T, a *App) {
	t.Helper()
	mounted := a.commandMaintenance.Load()
	if mounted == nil || mounted.ports.Outbox == nil {
		t.Fatal("manutenção sem adapter real de outbox")
	}
	result, err := mounted.ports.Outbox.Drain(context.Background(), 100)
	if err != nil {
		t.Fatalf("drenar outbox real: %v", err)
	}
	if result.More {
		t.Fatal("outbox real excedeu o lote único do teste")
	}
}

func waitLiveClaimsForRun(t *testing.T, runID string, minimum int) []commandactivation.Claim {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var claims []commandactivation.Claim
		if err := database.DB().Where("source_type = ? AND source_correlation_id = ? AND state = ?", "job", runID, commandactivation.StateActive).Find(&claims).Error; err == nil {
			layers := map[string]bool{}
			for _, claim := range claims {
				layers[claim.LayerRef] = true
			}
			if len(layers) >= minimum {
				return claims
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("run %s não publicou %d claims ativas independentes", runID, minimum)
	return nil
}
