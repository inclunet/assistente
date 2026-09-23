package app

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandjobactivation"
	"assistente/internal/commandjson"
	"assistente/internal/database"
	"assistente/internal/jobs"
)

// A origem de uma delegação reativa é relida no worker, fora do DispatchGate.
// Nenhuma raiz do JSON recebido é autoridade: somente claims/leases e fatos
// da outbox verificados pelo Consumer podem sustentar a cadeia herdada.
func (a *App) resolveCommandJobOrigin(ctx context.Context, in commandexecution.Invocation, manager *jobs.Manager) (jobs.CommandJobOrigin, error) {
	denied := jobs.CommandJobOrigin{}
	if a == nil || ctx == nil || ctx.Err() != nil || in.Envelope == nil || in.Envelope.WorkspaceID == nil {
		return denied, commandexecution.ErrDenied
	}
	principal, err := a.currentCommandPrincipal()
	ownerID, ownerErr := database.RequireUserID(ctx)
	if err != nil || ownerErr != nil || principal != in.Principal || ownerID != principal.UserID {
		return denied, commandexecution.ErrDenied
	}
	mounted := a.commandMaintenance.Load()
	a.authMu.RLock()
	workspace, currentManager := a.workspaceMgr, a.jobMgr
	a.authMu.RUnlock()
	if mounted == nil || mounted.consumer == nil || mounted.manager != manager || currentManager != manager || workspace == nil {
		return denied, commandexecution.ErrDenied
	}
	before, err := workspace.CommandSnapshot()
	if err != nil || before.WorkspaceID != *in.Envelope.WorkspaceID {
		return denied, commandexecution.ErrDenied
	}
	core, err := a.commandSecurityService()
	if err != nil {
		return denied, err
	}
	epoch, err := core.Capture(ctx, principal.UserID, principal.SessionID)
	if err != nil || epoch.AuthGeneration != in.Envelope.AuthGeneration || epoch.SecurityGeneration != in.Envelope.SecurityGeneration {
		return denied, commandexecution.ErrDenied
	}
	owner := commandactivation.Owner{Scope: commandactivation.Scope{UserID: principal.UserID, WorkspaceID: cloneCommandWorkspace(in.Envelope.WorkspaceID)},
		AuthContextType: "local_session", AuthContextID: principal.SessionID, AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration}
	proof, err := mounted.consumer.Projection(ctx, owner)
	if err != nil {
		return denied, err
	}
	origin, err := commandJobOriginFromProjection(in, proof)
	if err != nil {
		return denied, err
	}
	// Revalida as fontes em memória após a projeção transacional.
	for _, entry := range proof.Claims {
		if err := manager.ValidateCommandRuntimeProjection(ctx, entry.RunID, entry.Runtime); err != nil {
			return denied, commandexecution.ErrDenied
		}
	}
	a.authMu.RLock()
	same := a.workspaceMgr == workspace && a.jobMgr == manager
	a.authMu.RUnlock()
	after, snapshotErr := workspace.CommandSnapshot()
	if !same || snapshotErr != nil || after.Version != before.Version || a.commandMaintenance.Load() != mounted ||
		mounted.consumer.ProjectionRevision() != proof.Revision || !proof.ValidUntil.IsZero() && !time.Now().Before(proof.ValidUntil) {
		return denied, commandexecution.ErrDenied
	}
	current, err := a.currentCommandPrincipal()
	if err != nil || current != principal || ctx.Err() != nil {
		return denied, commandexecution.ErrDenied
	}
	return origin, nil
}

// Combina apenas camadas selecionadas pelo executor. Ciclos de raiz diferente
// nunca são fundidos, mesmo se alguém reproduzir o mesmo JSON estrutural.
func commandJobOriginFromProjection(in commandexecution.Invocation, proof commandjobactivation.ProjectionSnapshot) (jobs.CommandJobOrigin, error) {
	denied := jobs.CommandJobOrigin{}
	if in.Envelope == nil || in.Envelope.Provenance == nil {
		return denied, commandexecution.ErrDenied
	}
	actual, err := commandjson.Canonicalize(*in.Envelope.Provenance)
	var document map[string]json.RawMessage
	if err != nil || json.Unmarshal(actual, &document) != nil {
		return denied, commandexecution.ErrDenied
	}
	history, err := commandcontract.DecodeCommandChainHistory(document["command_chain_history"])
	if err != nil || len(history) == 0 {
		return denied, commandexecution.ErrDenied
	}
	last := history[len(history)-1]
	if last.CommandID != in.CommandID || last.InvocationID != in.ID || len(last.LayerRefs) == 0 {
		return denied, commandexecution.ErrDenied
	}
	var sources []commandbindings.LayerProvenance
	var origin jobs.CommandJobOrigin
	for _, entry := range proof.Claims {
		if entry.Claim.LayerRefKind != commandactivation.UserRef || !slices.Contains(last.LayerRefs, entry.Claim.LayerRef) {
			continue
		}
		root := jobs.CommandJobOrigin{RootOriginType: entry.RootOriginType, RootOriginID: entry.RootOriginID}
		if root.RootOriginType == "" || root.RootOriginID == "" || len(sources) > 0 && root != origin {
			return denied, commandexecution.ErrDenied
		}
		origin = root
		sources = append(sources, commandbindings.LayerProvenance{SourceID: entry.Claim.ActivationID, Provenance: entry.Provenance})
	}
	selected, err := commandSelectedJobProvenance(sources)
	if err != nil || selected == nil {
		return denied, commandexecution.ErrDenied
	}
	var expected map[string]json.RawMessage
	if json.Unmarshal(*selected, &expected) != nil {
		return denied, commandexecution.ErrDenied
	}
	var prefix []commandcontract.CommandChainEntry
	if raw, exists := expected["command_chain_history"]; exists {
		prefix, err = commandcontract.DecodeCommandChainHistory(raw)
		if err != nil {
			return denied, commandexecution.ErrDenied
		}
	}
	prefixJSON, _ := commandjson.Marshal(append([]commandcontract.CommandChainEntry{}, prefix...))
	actualPrefixJSON, _ := commandjson.Marshal(history[:len(history)-1])
	if !bytes.Equal(prefixJSON, actualPrefixJSON) {
		return denied, commandexecution.ErrDenied
	}
	expected["command_chain_history"], err = commandjson.Marshal(history)
	if err != nil {
		return denied, commandexecution.ErrDenied
	}
	canonical, err := commandjson.Marshal(expected)
	if err != nil || !bytes.Equal(canonical, actual) {
		return denied, commandexecution.ErrDenied
	}
	return origin, nil
}
