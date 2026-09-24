package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/commandactivation"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"github.com/google/uuid"
)

func TestCommandJobResolutionPublishesRealLayerProvenanceAndDropsItAtTerminal(t *testing.T) {
	a := commandJobPublicationApp(t)
	control := startCommandMaintenanceLiveJob(t, a)
	runID := <-control.RunID
	claim, _ := waitLiveClaimAndLease(t, runID)
	// Pausa apenas a cadência antes de alterar a configuração da claim. O
	// runtime e o tool continuam vivos, mas evitamos disputar SQLite com o
	// heartbeat enquanto o binding real é instalado.
	if err := a.jobMgr.CloseCommandMaintenance(context.Background()); err != nil {
		t.Fatal(err)
	}
	installJobPaletteExecute(t, a, claim)

	record := executePaletteResolution(t, a)
	if record.Status != commandledger.Succeeded || record.Envelope.CommandID == nil || *record.Envelope.CommandID != commandProductWorkspaceListID {
		t.Fatalf("binding execute não resolveu workspace.list: status=%s envelope=%+v", record.Status, record.Envelope)
	}
	if record.Envelope.Provenance != nil {
		t.Fatal("FullRecord público expôs proveniência redigida; o retorno deve permanecer sem o payload")
	}
	provenance := requireJobEnvelopeProvenance(t, readInvocationProvenance(t, record.Envelope.InvocationID))
	if version, ok := provenance["version"].(float64); !ok || version != 1 {
		t.Fatalf("versão da proveniência=%v, want 1", provenance["version"])
	}
	if provenance["_chain_id"] != runID {
		t.Fatalf("chain id=%v, want run %s", provenance["_chain_id"], runID)
	}
	if provenance["_source"] != "job" || provenance["_source_job_id"] != *claim.SourceJobSlug || provenance["redacted"] != true {
		t.Fatalf("origem estrutural ou marcador de redação incorreto: %#v", provenance)
	}
	history, ok := provenance["_chain_history"].([]any)
	if !ok || len(history) == 0 || history[len(history)-1] != *claim.SourceJobSlug {
		t.Fatalf("histórico de jobs não corresponde à claim: %#v claim=%+v", provenance["_chain_history"], claim)
	}
	if _, exists := provenance["command_runtime_identity"]; exists {
		t.Fatal("proveniência pública vazou command_runtime_identity")
	}
	if _, exists := provenance["_command_runtime_identity"]; exists {
		t.Fatal("proveniência pública vazou _command_runtime_identity")
	}
	chain, ok := provenance["command_chain_history"].([]any)
	if !ok || len(chain) != 1 {
		t.Fatalf("cadeia de comandos ausente ou inesperada: %#v", provenance["command_chain_history"])
	}
	entry, ok := chain[0].(map[string]any)
	if !ok || entry["command_id"] != commandProductWorkspaceListID || entry["invocation_id"] != record.Envelope.InvocationID {
		t.Fatalf("entrada atual da cadeia inválida: %#v invocation=%s", entry, record.Envelope.InvocationID)
	}
	layerRefs, ok := entry["layer_refs"].([]any)
	if !ok || !containsStringValue(layerRefs, claim.LayerRef) {
		t.Fatalf("layer_refs não contém a camada da claim %q: %#v", claim.LayerRef, entry["layer_refs"])
	}

	control.Release()
	if result, err := control.Join(); err != nil || result == nil || result.Status != "completed" {
		t.Fatalf("job não terminou: result=%+v err=%v", result, err)
	}

	last := executePaletteResolution(t, a)
	if last.Status != commandledger.Succeeded || last.Envelope.CommandID == nil || *last.Envelope.CommandID != commandProductWorkspaceListID {
		t.Fatalf("refresh terminal não restaurou workspace.list: status=%s envelope=%+v", last.Status, last.Envelope)
	}
	if last.Envelope.Provenance != nil {
		t.Fatalf("refresh terminal manteve proveniência do job: %s", string(*last.Envelope.Provenance))
	}
	if persisted := readInvocationProvenance(t, last.Envelope.InvocationID); persisted != nil {
		t.Fatalf("invocação terminal persistiu proveniência: %s", *persisted)
	}
}

func containsStringValue(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func installJobPaletteExecute(t *testing.T, a *App, claim commandactivation.Claim) string {
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
	commandID := commandProductWorkspaceListID
	binding := commandconfig.Binding{
		ID:                         uuid.Must(uuid.NewV7()).String(),
		UserID:                     claim.UserID,
		WorkspaceID:                cloneCommandWorkspace(claim.WorkspaceID),
		LayerRefKind:               string(claim.LayerRefKind),
		LayerRef:                   claim.LayerRef,
		TriggerType:                string(commandcatalog.Palette),
		TriggerSpec:                `{"version":1,"selection":"workspace.list"}`,
		Arguments:                  `{}`,
		Condition:                  `{"version":1,"clauses":[]}`,
		Effect:                     "execute",
		CommandID:                  &commandID,
		Enabled:                    true,
		Source:                     "user",
		ReviewStatus:               "active",
		Presentation:               `{"version":1}`,
		ReplacesDefaultID:          paletteStringPtr(defaultBindingID),
		ReplacesDefaultVersion:     paletteStringPtr(defaultVersion),
		ReplacesDefaultFingerprint: paletteStringPtr(defaultFingerprint),
	}
	if err := database.DB().Create(&binding).Error; err != nil {
		t.Fatalf("inserir execute na camada da claim: %v", err)
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

func readInvocationProvenance(t *testing.T, invocationID string) *string {
	t.Helper()
	var row struct {
		Provenance *string `gorm:"column:provenance"`
	}
	if err := database.DB().Table("command_invocations").Select("provenance").
		Where("invocation_id = ?", invocationID).Take(&row).Error; err != nil {
		t.Fatalf("ler proveniência persistida da invocação %s: %v", invocationID, err)
	}
	return row.Provenance
}

func requireJobEnvelopeProvenance(t *testing.T, raw *string) map[string]any {
	t.Helper()
	if raw == nil {
		t.Fatal("invocação resolvida sem proveniência persistida")
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(*raw), &document); err != nil {
		t.Fatalf("proveniência inválida: %v", err)
	}
	if document == nil || strings.TrimSpace(string(*raw)) == "{}" {
		t.Fatalf("proveniência vazia: %s", string(*raw))
	}
	return document
}
