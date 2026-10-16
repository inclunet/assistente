package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandportability"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/portability"
	"gorm.io/gorm"
)

const commandDesktopImportPolicyVersion = "command-import-policy-v1"

// importCommandLayers é a entrada desktop da fachada. A autenticação é sempre
// a sessão local já montada no App; JSONData nunca fornece JWT, owner, catálogo
// ou workspace autorizado.
func (a *App) importCommandLayers(ctx context.Context, req portability.ImportRequest) (*portability.ImportResult, error) {
	if a == nil || ctx == nil {
		return nil, commandexecution.ErrDenied
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return nil, safeCommandImportError(err)
	}
	if userID, ok := database.UserIDFromContext(bounded); !ok || userID != p.principal.UserID {
		return nil, commandexecution.ErrDenied
	}
	if err := bounded.Err(); err != nil {
		return nil, err
	}
	layers, err := portability.ParseCommandImportEnvelope([]byte(req.JSONData))
	if err != nil {
		return nil, safeCommandImportError(err)
	}
	options, err := portability.CommandImportOptions(req)
	if err != nil {
		return nil, safeCommandImportError(err)
	}
	refs, err := a.commandDesktopImportReferences(p, bounded)
	if err != nil {
		return nil, safeCommandImportError(err)
	}
	inputs, err := a.commandDesktopMutationInputs(p, bounded)
	if err != nil {
		return nil, safeCommandImportError(err)
	}
	applier, err := a.newCommandDesktopMutationApplier(inputs)
	if err != nil {
		return nil, safeCommandImportError(err)
	}
	// O token é propositalmente vazio: a fábrica desktop fixa e revalida a
	// sessão local antes de cada fase, sem JWT no envelope ou na fachada.
	batch, err := applier.ImportEnvelopeBatch(bounded, "", append([]byte(nil), req.JSONData...), options, refs)
	return commandDesktopImportOutcome(batch, layers, err)
}

func commandDesktopImportOutcome(batch commandMutationBatchResult, layers []commandportability.LayerExport, err error) (*portability.ImportResult, error) {
	result := commandImportResult(batch, layers)
	// Depois do commit, o relatório é a única forma segura de explicar o
	// estado persistido. Nunca o descarte por erro ou cancelamento do rebuild.
	if batch.Committed && !batch.Rebuilt {
		result.Success = false
		result.Warnings = append(result.Warnings, commandImportCommittedNotPublished())
		return &result, nil
	}
	if err != nil && !errors.Is(err, commandportability.ErrNoChanges) {
		return nil, safeCommandImportError(err)
	}
	if errors.Is(err, commandportability.ErrNoChanges) || batch.Report != nil && batch.Report.NoChanges {
		result.Success = true
		result.Imported = 0
		result.Skipped = len(layers)
		result.Message = "Nenhuma alteração de command layers foi necessária."
		return &result, nil
	}
	if !batch.Committed || !batch.Rebuilt {
		return nil, commandexecution.ErrStale
	}
	result.Success = true
	result.Message = "Command layers importadas e configuração reconstruída."
	return &result, nil
}

func commandImportCommittedNotPublished() portability.LocalizedMessage {
	return portability.LocalizedMessage{Code: "commandImport.committedNotPublished", Message: "A importação foi confirmada, mas a configuração local não foi publicada; o relatório foi preservado."}
}

func (a *App) commandDesktopMutationInputs(p *commandProductRuntime, ctx context.Context) (commandCompleteMutationInputs, error) {
	if a == nil || p == nil || p.registry == nil || p.sessionSvc == nil || p.credMgr == nil || p.epochs == nil {
		return commandCompleteMutationInputs{}, commandexecution.ErrInvalidConfiguration
	}
	db := database.DB()
	store, err := commandconfig.New(db)
	if err != nil {
		return commandCompleteMutationInputs{}, commandexecution.ErrInvalidConfiguration
	}
	epoch, err := p.epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		current, err := p.sessionSvc.RevalidateLocalSession(ctx, p.principal)
		if err != nil || current != p.principal || !a.commandPrincipalMatches(p.sessionSvc, p.credMgr, current) {
			return "", "", commandexecution.ErrDenied
		}
		return current.UserID, current.SessionID, nil
	})
	if err != nil {
		return commandCompleteMutationInputs{}, err
	}

	workspaceAuthorized := func(ctx context.Context, scope commandconfig.Scope) error {
		if a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
			return commandexecution.ErrStale
		}
		if scope.UserID != p.principal.UserID || strings.TrimSpace(scope.UserID) != scope.UserID {
			return commandexecution.ErrDenied
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		a.authMu.RLock()
		managerOK := a.workspaceMgr == p.workspaceMgr
		manager := a.workspaceMgr
		a.authMu.RUnlock()
		if !managerOK || manager == nil || !a.commandPrincipalMatches(p.sessionSvc, p.credMgr, p.principal) {
			return commandexecution.ErrDenied
		}
		if scope.WorkspaceID == nil {
			return nil
		}
		items, listErr := manager.List()
		if listErr != nil {
			return listErr
		}
		for _, item := range items {
			if item.ID == *scope.WorkspaceID {
				return nil
			}
		}
		return commandportability.ErrWorkspaceResolution
	}

	projection := func(ctx context.Context, scope commandconfig.Scope) (commandconfig.CompleteProjection, error) {
		if err := workspaceAuthorized(ctx, scope); err != nil {
			return commandconfig.CompleteProjection{}, err
		}
		if err := store.EnsureScope(ctx, scope); err != nil {
			return commandconfig.CompleteProjection{}, err
		}
		snapshot, err := store.Load(ctx, scope)
		if err != nil {
			return commandconfig.CompleteProjection{}, err
		}
		active := commandLifecycleActiveUserLayerIDs(snapshot, p.principal, time.Now())
		return commandProductProjection(p.registry, active)
	}
	authorize := func(ctx context.Context, principal auth.LocalSessionPrincipal, scope commandconfig.Scope, _ commandconfig.Operation) error {
		if principal != p.principal || principal.UserID != p.principal.UserID || !a.commandPrincipalMatches(p.sessionSvc, p.credMgr, principal) {
			return commandexecution.ErrDenied
		}
		current, err := p.sessionSvc.RevalidateLocalSession(ctx, p.principal)
		if err != nil || current != p.principal {
			return commandexecution.ErrDenied
		}
		return workspaceAuthorized(ctx, scope)
	}
	owner := func(ctx context.Context, scope commandconfig.Scope) (commandactivation.Owner, error) {
		if scope.UserID != p.principal.UserID || !validDesktopImportWorkspace(scope.WorkspaceID) || !a.commandPrincipalMatches(p.sessionSvc, p.credMgr, p.principal) {
			return commandactivation.Owner{}, commandexecution.ErrDenied
		}
		return commandactivation.Owner{Scope: commandactivation.Scope{UserID: p.principal.UserID, WorkspaceID: cloneCommandWorkspace(scope.WorkspaceID)}, AuthContextType: "local_session", AuthContextID: p.principal.SessionID, AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration}, nil
	}
	activation, err := a.newDesktopImportActivationService(owner)
	if err != nil {
		return commandCompleteMutationInputs{}, err
	}
	grants, err := commandautomation.New(db, time.Now)
	if err != nil {
		return commandCompleteMutationInputs{}, commandexecution.ErrInvalidConfiguration
	}
	baseHook, err := commandconfig.NewActivationMutationHook(activation, grants, owner)
	if err != nil {
		return commandCompleteMutationInputs{}, err
	}
	hook := func(ctx context.Context, tx *gorm.DB, diff commandconfig.MutationDiff) error {
		if tx == nil || a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
			return commandexecution.ErrDenied
		}
		root, err := db.DB()
		if err != nil {
			return commandexecution.ErrInvalidConfiguration
		}
		txDB, err := tx.DB()
		if err != nil || root == nil || txDB == nil || root != txDB {
			return commandexecution.ErrDenied
		}
		current, err := p.sessionSvc.RevalidateLocalSessionTx(ctx, tx, p.principal)
		if err != nil || current != p.principal || !a.commandPrincipalMatches(p.sessionSvc, p.credMgr, current) {
			return commandexecution.ErrDenied
		}
		return baseHook(ctx, tx, diff)
	}

	return commandCompleteMutationInputs{
		Projection: projection, Authorize: authorize,
		Version: func(context.Context) (string, error) {
			if a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
				return "", commandexecution.ErrStale
			}
			return commandProductRegistryVersion + ":" + commandDesktopImportPolicyVersion, nil
		},
		Render: renderDesktopImportDiff, OnMutationTx: hook, DecisionTTL: 2 * time.Minute,
		BuildConfiguration: func(ctx context.Context, scope commandconfig.Scope, snapshot commandconfig.Snapshot, options commandconfig.CompleteProjection) (*commandbindings.Configuration, []string, func(context.Context) error, error) {
			epoch, manualGuard, err := p.commandManualClaimAuthority(ctx)
			if err != nil {
				return nil, nil, nil, err
			}
			active := commandCurrentManualLayerIDs(snapshot, p.principal, time.Now(), epoch)
			options.ActiveUserLayerIDs = active
			// Settings/import edit only their own persisted scope. Publication
			// must retain the authoritative voice/job hotkeys of the live product.
			global, err := a.commandProductGlobalProjection(ctx, p.registry, active)
			if err != nil {
				return nil, nil, nil, err
			}
			options.BuiltinLayers = global.BuiltinLayers
			var proof *commandJobProjection
			var guard func(context.Context) error
			if scope.WorkspaceID != nil {
				jobProof, jobGuard, jobErr := a.commandJobLayerProjection(ctx, p.principal, scope)
				if jobErr != nil {
					return nil, nil, nil, jobErr
				}
				proof, guard = jobProof, jobGuard
				if proof != nil {
					active = mergeCommandLayerIDs(active, proof.layers)
					options.ActiveUserLayerIDs = active
				}
			}
			configuration, buildErr := commandconfig.ProjectComplete(ctx, snapshot, options)
			if buildErr != nil {
				return nil, nil, nil, buildErr
			}
			if proof != nil {
				withProvenance, provenanceErr := configuration.WithLayerProvenance(proof.sources)
				if provenanceErr != nil {
					return nil, nil, nil, provenanceErr
				}
				configuration = withProvenance
			}
			deadline := commandManualClaimsDeadline(snapshot, p.principal, time.Now())
			if commandManualClaimsDue(snapshot, time.Now()) {
				deadline = time.Now()
			}
			configuration = configuration.WithValidityDeadline(deadline)
			previousGuard := guard
			guard = func(ctx context.Context) error {
				if err := manualGuard(ctx); err != nil {
					return err
				}
				if !deadline.IsZero() && !time.Now().Before(deadline) {
					return commandexecution.ErrStale
				}
				if previousGuard != nil {
					return previousGuard(ctx)
				}
				return ctx.Err()
			}
			return configuration, active, guard, nil
		},
	}, nil
}

func (a *App) newDesktopImportActivationService(owner commandconfig.ActivationOwner) (*commandactivation.Service, error) {
	store, err := commandactivation.NewStore(database.DB())
	if err != nil {
		return nil, err
	}
	return commandactivation.New(database.DB(), &commandsecurity.DispatchGate{}, commandactivation.Ports{
		Owner: ownerPortForImport(owner), Layer: commandLifecycleActivationLayerPort{},
		Origin: commandactivation.OriginPortFunc(func(_ context.Context, _ commandactivation.Owner, origin commandactivation.Origin) (commandactivation.Origin, error) {
			return origin, nil
		}),
		Rule: store, GenerationTx: store,
	}, time.Now)
}

func ownerPortForImport(owner commandconfig.ActivationOwner) commandactivation.OwnerPort {
	return commandactivation.OwnerPortFunc(func(ctx context.Context, asserted commandactivation.Owner) (commandactivation.Owner, error) {
		if owner == nil || ctx == nil {
			return commandactivation.Owner{}, commandexecution.ErrDenied
		}
		return owner(ctx, commandconfig.Scope{UserID: asserted.UserID, WorkspaceID: cloneCommandWorkspace(asserted.WorkspaceID)})
	})
}

func (a *App) commandDesktopImportReferences(p *commandProductRuntime, ctx context.Context) (commandportability.ReferencePort, error) {
	if p == nil || p.registry == nil {
		return commandportability.ReferencePort{}, commandexecution.ErrInvalidConfiguration
	}
	projection, err := commandProductProjection(p.registry, nil)
	if err != nil {
		return commandportability.ReferencePort{}, err
	}
	builtinLayers := map[string]bool{}
	builtinDefaults := map[string]bool{}
	for _, layer := range projection.BuiltinLayers {
		builtinLayers[layer.ID] = true
		for _, item := range layer.Defaults {
			builtinDefaults[item.Candidate.ID] = true
		}
	}
	return commandportability.ReferencePort{
		Catalog: p.registry,
		Command: func(ctx context.Context, id string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, ok := p.registry.Lookup(id); !ok {
				return commandportability.ErrMissingReference
			}
			return nil
		},
		BuiltinLayer: func(_ context.Context, id string) error {
			if !builtinLayers[id] {
				return commandportability.ErrMissingReference
			}
			return nil
		},
		BuiltinDefault: func(_ context.Context, id string) error {
			if !builtinDefaults[id] {
				return commandportability.ErrMissingReference
			}
			return nil
		},
		BuiltinRuleReference: func(context.Context, string) error { return commandportability.ErrMissingReference },
		BuiltinRule:          func(context.Context, string, string, string) (bool, error) { return false, nil },
		Workspace: func(ctx context.Context, id string) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			a.authMu.RLock()
			manager := a.workspaceMgr
			same := manager == p.workspaceMgr
			a.authMu.RUnlock()
			if !same || manager == nil || !a.commandPrincipalMatches(p.sessionSvc, p.credMgr, p.principal) {
				return "", commandexecution.ErrDenied
			}
			items, listErr := manager.List()
			if listErr != nil {
				return "", listErr
			}
			for _, item := range items {
				if item.ID == id {
					return id, nil
				}
			}
			return "", commandportability.ErrWorkspaceResolution
		},
		Trigger: func(ctx context.Context, triggerType, raw string) (string, error) {
			var source commandcatalog.Source
			switch triggerType {
			case string(commandcatalog.KeyboardLocal):
				source = commandcatalog.KeyboardLocal
			case string(commandcatalog.Palette):
				source = commandcatalog.Palette
			default:
				return "", commandportability.ErrMissingReference
			}
			port, ok := projection.TriggerPorts[source]
			if !ok || port == nil {
				return "", commandportability.ErrMissingReference
			}
			return port.Normalize(ctx, []byte(raw))
		},
	}, nil
}

func validDesktopImportWorkspace(id *string) bool {
	return id == nil || *id != "" && strings.TrimSpace(*id) == *id && !strings.ContainsRune(*id, '\x00')
}

func commandImportResult(batch commandMutationBatchResult, layers []commandportability.LayerExport) portability.ImportResult {
	result := portability.ImportResult{Imported: len(batch.Diffs), Message: "Importação de command layers concluída."}
	if batch.Report != nil {
		result.Imported = len(batch.Report.Layers)
		for _, item := range batch.Report.Layers {
			scope := "global"
			if item.Scope.WorkspaceID != "" {
				scope = item.Scope.WorkspaceID
			}
			result.Warnings = append(result.Warnings, portability.LocalizedMessage{Code: "commandImport.layerResult", Params: map[string]string{"targetId": item.TargetID, "scope": scope, "action": string(item.Action)}, Message: fmt.Sprintf("Layer %s: %s (%s).", item.TargetID, item.Action, scope)})
		}
		for _, warning := range batch.Report.Warnings {
			result.Warnings = append(result.Warnings, portability.LocalizedMessage{Code: "commandImport.warning", Params: map[string]string{"code": warning.Code, "count": strconv.Itoa(warning.Count)}, Message: fmt.Sprintf("Aviso %s (%d).", warning.Code, warning.Count)})
		}
	}
	if result.Imported == 0 {
		result.Imported = len(layers)
	}
	return result
}

func safeCommandImportError(err error) error {
	if err == nil {
		return nil
	}
	// Não devolva wrappers: uma causa de SQL ou caminho interno não pode
	// atravessar a fachada mesmo quando carrega um sentinel conhecido.
	known := []error{
		context.Canceled, context.DeadlineExceeded,
		commandportability.ErrInvalid, commandportability.ErrUnsupported,
		commandportability.ErrForeignOwner, commandportability.ErrWorkspaceResolution,
		commandportability.ErrNameConflict, commandportability.ErrMissingReference,
		commandportability.ErrSensitiveValue, commandportability.ErrBuiltinRuleConflict,
		commandportability.ErrNoChanges, commandexecution.ErrDenied,
		commandexecution.ErrStale, commandconfig.ErrInvalid, commandconfig.ErrStale,
	}
	for _, sentinel := range known {
		if errors.Is(err, sentinel) {
			return sentinel
		}
	}
	return commandexecution.ErrInvalidConfiguration
}

func renderDesktopImportDiff(diff commandconfig.MutationDiff) (string, error) {
	if diff.Scope.UserID == "" {
		return "", commandconfig.ErrInvalid
	}
	return fmt.Sprintf("Escopo user=%s workspace=%s operação=%s; %s; %s; %s.", diff.Scope.UserID, desktopScopeName(diff.Scope.WorkspaceID), diff.Operation, summarizeDesktopLayers(diff.BeforeLayers, diff.AfterLayers), summarizeDesktopBindings(diff.BeforeBindings, diff.AfterBindings), summarizeDesktopRules(diff.BeforeActivationRules, diff.AfterActivationRules)), nil
}

func desktopScopeName(id *string) string {
	if id == nil {
		return "global"
	}
	return *id
}

func summarizeDesktopLayers(before, after []commandconfig.Layer) string {
	left, right := map[string]desktopSummaryEntry{}, map[string]desktopSummaryEntry{}
	for _, item := range before {
		left[item.ID] = desktopSummaryEntry{value: item}
	}
	for _, item := range after {
		right[item.ID] = desktopSummaryEntry{value: item}
	}
	return summarizeLayerEntries(left, right)
}

func summarizeDesktopBindings(before, after []commandconfig.Binding) string {
	left, right := map[string]desktopSummaryEntry{}, map[string]desktopSummaryEntry{}
	for _, item := range before {
		left[item.ID] = desktopSummaryEntry{value: item}
	}
	for _, item := range after {
		right[item.ID] = desktopSummaryEntry{value: item}
	}
	return summarizeBindingEntries(left, right)
}

func summarizeDesktopRules(before, after []commandactivation.Rule) string {
	left, right := map[string]desktopSummaryEntry{}, map[string]desktopSummaryEntry{}
	for _, item := range before {
		left[item.ID] = desktopSummaryEntry{value: item}
	}
	for _, item := range after {
		right[item.ID] = desktopSummaryEntry{value: item}
	}
	return summarizeRuleEntries(left, right)
}

type desktopSummaryEntry struct {
	value any
}

func summarizeLayerEntries(before, after map[string]desktopSummaryEntry) string {
	return summarizeEntries("layers", before, after, func(id string, old, current desktopSummaryEntry) string {
		beforeLayer, beforeOK := old.value.(commandconfig.Layer)
		afterLayer, afterOK := current.value.(commandconfig.Layer)
		if !beforeOK || !afterOK {
			return id
		}
		fields := changedLayerFields(beforeLayer, afterLayer)
		if len(fields) == 0 {
			return ""
		}
		return id + "[" + strings.Join(fields, ",") + "]"
	})
}

func summarizeBindingEntries(before, after map[string]desktopSummaryEntry) string {
	return summarizeEntries("bindings", before, after, func(id string, old, current desktopSummaryEntry) string {
		beforeBinding, beforeOK := old.value.(commandconfig.Binding)
		afterBinding, afterOK := current.value.(commandconfig.Binding)
		if !beforeOK || !afterOK {
			return id
		}
		fields := changedBindingFields(beforeBinding, afterBinding)
		if len(fields) == 0 {
			return ""
		}
		return id + "[" + strings.Join(fields, ",") + "]"
	})
}

func summarizeRuleEntries(before, after map[string]desktopSummaryEntry) string {
	return summarizeEntries("regras", before, after, func(id string, old, current desktopSummaryEntry) string {
		beforeRule, beforeOK := old.value.(commandactivation.Rule)
		afterRule, afterOK := current.value.(commandactivation.Rule)
		if !beforeOK || !afterOK {
			return id
		}
		fields := changedRuleFields(beforeRule, afterRule)
		if len(fields) == 0 {
			return ""
		}
		return id + "[" + strings.Join(fields, ",") + "]"
	})
}

func summarizeEntries(label string, before, after map[string]desktopSummaryEntry, changedFunc func(string, desktopSummaryEntry, desktopSummaryEntry) string) string {
	var added, removed, altered []string
	for id, current := range after {
		old, ok := before[id]
		if !ok {
			added = append(added, desktopCreatedEntry(id, current.value))
		} else if !reflect.DeepEqual(old.value, current.value) {
			if entry := changedFunc(id, old, current); entry != "" {
				altered = append(altered, entry)
			}
		}
	}
	for id := range before {
		if _, ok := after[id]; !ok {
			removed = append(removed, id)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(altered)
	return fmt.Sprintf("%s +%s -%s ~%s", label, strings.Join(added, ","), strings.Join(removed, ","), strings.Join(altered, ","))
}

func desktopCreatedEntry(id string, value any) string {
	switch item := value.(type) {
	case commandconfig.Layer:
		return fmt.Sprintf("%s[enabled=%t]", id, item.Enabled)
	case commandconfig.Binding:
		command := ""
		if item.CommandID != nil {
			command = ",command=" + *item.CommandID
		}
		return fmt.Sprintf("%s[enabled=%t,effect=%s%s]", id, item.Enabled, item.Effect, command)
	case commandactivation.Rule:
		return fmt.Sprintf("%s[enabled=%t,mode=%s,lifecycle=%s]", id, item.Enabled, item.Mode, item.Lifecycle)
	default:
		return id
	}
}

func changedLayerFields(before, after commandconfig.Layer) []string {
	var fields []string
	if before.WorkspaceID == nil != (after.WorkspaceID == nil) || before.WorkspaceID != nil && after.WorkspaceID != nil && *before.WorkspaceID != *after.WorkspaceID {
		fields = append(fields, "scope")
	}
	if before.Enabled != after.Enabled {
		fields = append(fields, "enabled")
	}
	if before.Name != after.Name || before.Description != after.Description || before.Source != after.Source || before.ResolutionPriority != after.ResolutionPriority {
		fields = append(fields, "metadata")
	}
	return fields
}

func changedBindingFields(before, after commandconfig.Binding) []string {
	var fields []string
	if before.LayerRefKind != after.LayerRefKind || before.LayerRef != after.LayerRef {
		fields = append(fields, "layer")
	}
	if before.TriggerType != after.TriggerType || before.TriggerSpec != after.TriggerSpec {
		fields = append(fields, "trigger")
	}
	if !reflect.DeepEqual(before.CommandID, after.CommandID) {
		fields = append(fields, "command")
	}
	if before.Arguments != after.Arguments {
		fields = append(fields, "arguments")
	}
	if before.Condition != after.Condition {
		fields = append(fields, "condition")
	}
	if before.Effect != after.Effect {
		fields = append(fields, "effect")
	}
	if before.Enabled != after.Enabled {
		fields = append(fields, "enabled")
	}
	if before.ResolutionPriority != after.ResolutionPriority {
		fields = append(fields, "priority")
	}
	if before.Source != after.Source || before.ReviewStatus != after.ReviewStatus {
		fields = append(fields, "review")
	}
	if before.Presentation != after.Presentation {
		fields = append(fields, "presentation")
	}
	if !reflect.DeepEqual(before.ReplacesDefaultID, after.ReplacesDefaultID) || !reflect.DeepEqual(before.ReplacesDefaultVersion, after.ReplacesDefaultVersion) || !reflect.DeepEqual(before.ReplacesDefaultFingerprint, after.ReplacesDefaultFingerprint) {
		fields = append(fields, "default")
	}
	return fields
}

func changedRuleFields(before, after commandactivation.Rule) []string {
	var fields []string
	if before.WorkspaceID == nil != (after.WorkspaceID == nil) || before.WorkspaceID != nil && after.WorkspaceID != nil && *before.WorkspaceID != *after.WorkspaceID {
		fields = append(fields, "scope")
	}
	if before.LayerRefKind != after.LayerRefKind || before.LayerRef != after.LayerRef {
		fields = append(fields, "layer")
	}
	if before.RuleRefKind != after.RuleRefKind || before.RuleRef != after.RuleRef {
		fields = append(fields, "rule")
	}
	if before.Mode != after.Mode {
		fields = append(fields, "mode")
	}
	if before.Condition != after.Condition {
		fields = append(fields, "condition")
	}
	if before.Lifecycle != after.Lifecycle || !reflect.DeepEqual(before.EventName, after.EventName) || !reflect.DeepEqual(before.AllowedInternalProducerTypes, after.AllowedInternalProducerTypes) {
		fields = append(fields, "lifecycle")
	}
	if before.Enabled != after.Enabled {
		fields = append(fields, "enabled")
	}
	if before.ReviewStatus != after.ReviewStatus {
		fields = append(fields, "review")
	}
	if !reflect.DeepEqual(before.ReplacesDefaultID, after.ReplacesDefaultID) || !reflect.DeepEqual(before.ReplacesDefaultVersion, after.ReplacesDefaultVersion) || !reflect.DeepEqual(before.ReplacesDefaultFingerprint, after.ReplacesDefaultFingerprint) {
		fields = append(fields, "default")
	}
	return fields
}
