package app

import (
	"testing"

	"assistente/internal/commandconfig"
)

func TestCommandMutationAuditRequiresExactScopeGeneration(t *testing.T) {
	workspace := "workspace"
	global := commandconfig.Generation{ID: "global-generation", Generation: 4}
	local := commandconfig.Generation{ID: "workspace-generation", WorkspaceID: &workspace, Generation: 7}
	union := commandconfig.Snapshot{
		Scope:       commandconfig.Scope{UserID: "user", WorkspaceID: &workspace},
		Generations: []commandconfig.Generation{global, local},
	}
	if commandMutationAuditMatchesSnapshot(union, global.ID, global.Generation) {
		t.Fatal("auditoria global herdada não prova o commit do workspace")
	}
	if !commandMutationAuditMatchesSnapshot(union, local.ID, local.Generation) {
		t.Fatal("geração exata do workspace recusada")
	}
	if commandMutationAuditMatchesSnapshot(union, local.ID, local.Generation-1) {
		t.Fatal("auditoria obsoleta aceita")
	}
	union.Scope.WorkspaceID = nil
	if !commandMutationAuditMatchesSnapshot(union, global.ID, global.Generation) {
		t.Fatal("geração exata global recusada")
	}
	if commandMutationAuditMatchesSnapshot(union, local.ID, local.Generation) {
		t.Fatal("auditoria de workspace aceita como prova global")
	}
}
