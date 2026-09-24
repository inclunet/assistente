package app

import (
	"context"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandcontract"
	"assistente/internal/commanddecision"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/questionnaire"
	"github.com/google/uuid"
)

func TestProductJobRunEventCommandReplayEndToEnd(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	ctx, cancel := context.WithTimeout(database.WithUserID(context.Background(), a.currentUserID), 30*time.Second)
	defer cancel()

	registrar := &globalJobTestHotkeyRegistrar{}
	var admissionInvocation atomic.Value
	var admissionErr atomic.Value
	a.emitter = globalJobTestEmitter(func(event string, data any) {
		if event != "command:global-job-admission" {
			return
		}
		payload, ok := data.(map[string]string)
		if !ok {
			admissionErr.Store(fmt.Errorf("admission payload = %T", data))
			return
		}
		admissionInvocation.Store(payload["invocationId"])
		if !a.AdmitGlobalCommandOccurrence(payload["invocationId"], true) {
			admissionErr.Store(fmt.Errorf("admission rejected"))
		}
	})
	dispatchDone := make(chan error, 1)
	dispatchFinished := make(chan struct{})
	dispatch := func(dispatchCtx context.Context, occurrence jobs.CommandHotkeyOccurrence) error {
		defer close(dispatchFinished)
		err := a.dispatchCommandJobHotkey(dispatchCtx, occurrence)
		dispatchDone <- err
		return err
	}
	decisionManager, decisionEvents := newCommandDecisionManager(t)
	a.questionnaireMgr = decisionManager
	ctx = configureGlobalJobHotkeyTestManager(t, a, registrar, dispatch)
	ctx, cancelHotkey := context.WithTimeout(ctx, 25*time.Second)
	defer cancelHotkey()
	layerID := createJobRunEventLayer(t, a)
	// O binding pertence à camada de destino, mas não exige nem fabrica uma
	// claim. Persista-o antes do job.run para que a atualização de configuração
	// não concorra com uma claim/lease já ativa.
	selectedBindingID := installJobPaletteExecute(t, a, commandactivation.Claim{
		UserID: a.currentUserID, LayerRefKind: commandactivation.UserRef, LayerRef: layerID,
	})
	createJobRunEventGrant(t, a, ctx, layerID)
	runTool := &liveCommandToolImpl{name: "test.product_job_run_event_" + uuid.Must(uuid.NewV7()).String(), started: make(chan struct{}), startedContext: make(chan context.Context, 1), release: make(chan struct{})}
	a.toolRegistry.MustRegister(runTool)
	catalogID := uuid.Must(uuid.NewV7()).String()
	if err := database.DB().Create(&database.ToolCatalog{
		UUIDModel: database.UUIDModel{ID: catalogID}, Name: runTool.Name(), DisplayName: runTool.Name(),
		Description: runTool.Description(), Origin: "builtin", Schema: string(runTool.Parameters()), AvailabilityStatus: "available",
	}).Error; err != nil {
		t.Fatal(err)
	}
	job := &jobs.Job{
		ID: "product-job-run-event-" + uuid.Must(uuid.NewV7()).String(), Name: "Product job.run event replay", Enabled: true, Tool: runTool.Name(),
		Inputs: map[string]any{}, Triggers: []jobs.Trigger{{Type: jobs.TriggerHotkey, Keys: "Ctrl+Alt+G"}},
		ErrorPolicy: jobs.ErrorPolicy{Strategy: jobs.ErrorStop},
	}
	if err := a.jobMgr.CreateJobContext(ctx, job); err != nil {
		t.Fatal(err)
	}
	toolRegistrations, _ := a.commandToolRegistrations()
	expectedToolCommandID, validToolID := commandToolExecutionID(catalogID)
	if !validToolID {
		t.Fatalf("UUID de catálogo da tool fake não gera id de comando canônico: %s", catalogID)
	}
	foundToolRegistration := false
	var invalidToolRegistrations []string
	for _, registration := range toolRegistrations {
		if registration.Definition.ID == expectedToolCommandID {
			foundToolRegistration = true
		}
		if err := commandcatalog.ValidateDefinitionComplete(registration.Definition, registration.Handler); err != nil {
			invalidToolRegistrations = append(invalidToolRegistrations, fmt.Sprintf("%s route=%q class=%q risk=%q scopes=%v context=%+v: %v", registration.Definition.ID, registration.Definition.HandlerRoute, registration.Definition.HandlerClassification, registration.Definition.Risk, registration.Definition.Scopes, registration.Definition.Context, err))
		}
	}
	if !foundToolRegistration {
		t.Fatalf("tool fake disponível %s não apareceu nas %d registrations dinâmicas", expectedToolCommandID, len(toolRegistrations))
	}
	if len(invalidToolRegistrations) != 0 {
		t.Fatalf("commandToolRegistrations contém definições inválidas: %v", invalidToolRegistrations)
	}
	if _, _, err := a.commandProductCatalog(); err != nil {
		t.Fatalf("commandProductCatalog com tool fake disponível %s: %v", expectedToolCommandID, err)
	}
	if err := a.rebuildCommandLifecycleProjection(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := a.jobMgr.Start(); err != nil {
		t.Fatal(err)
	}
	defer runTool.releaseOnce.Do(func() { close(runTool.release) })

	callbackDone := make(chan struct{})
	go func() {
		defer close(callbackDone)
		registrar.callback(t, 0)()
	}()
	t.Cleanup(func() {
		runTool.releaseOnce.Do(func() { close(runTool.release) })
		select {
		case <-callbackDone:
		case <-time.After(5 * time.Second):
			t.Error("cleanup não conseguiu aguardar o callback do hotkey")
		}
		select {
		case <-dispatchFinished:
		case <-time.After(5 * time.Second):
			t.Error("cleanup não conseguiu aguardar o dispatch do hotkey")
		}
	})
	var decision map[string]any
	select {
	case decision = <-decisionEvents:
	case err := <-dispatchDone:
		t.Fatalf("ingresso product job.run terminou antes da decisão: %v", err)
	case <-ctx.Done():
		t.Fatal("ingresso product job.run não pediu decisão", ctx.Err())
	}
	finishCommandDecision(t, decisionManager, decision, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
	select {
	case <-runTool.started:
	case <-ctx.Done():
		t.Fatal("job.run real não alcançou a tool bloqueada", ctx.Err())
	}

	runs, err := a.jobMgr.GetJobRunsContext(ctx, job.ID, 10)
	if err != nil || len(runs) != 1 || runs[0].Status != jobs.RunStatusRunning {
		t.Fatalf("run do comando product job.run: runs=%+v err=%v", runs, err)
	}
	commandStartedRunID := runs[0].RunID
	invocation, ok := admissionInvocation.Load().(string)
	if !ok || invocation == "" || runs[0].RootOriginType != "user_hotkey" || runs[0].RootOriginID != invocation {
		t.Fatalf("origem real do run: root=%s/%s invocation=%q", runs[0].RootOriginType, runs[0].RootOriginID, invocation)
	}
	runtimeIdentity, ok := runs[0].Provenance["command_runtime_identity"].(map[string]any)
	generation, generationOK := runtimeIdentity["generation"].(string)
	if !ok || !generationOK || "run_"+generation != commandStartedRunID {
		t.Fatalf("runtime identity não liga o run da claim ao job.run: runtime=%v run=%s", runs[0].Provenance["command_runtime_identity"], commandStartedRunID)
	}
	if value := admissionErr.Load(); value != nil {
		t.Fatal(value)
	}

	// O consumer montado no App consome o fato/outbox gravado pelo runtime do
	// próprio job.run. Nenhum snapshot/memento de origem é anexado pelo teste.
	drainCommandJobOutbox(t, a)
	claims := waitLiveClaimsForRun(t, commandStartedRunID, 1)
	var eventClaim *commandactivation.Claim
	for i := range claims {
		if claims[i].SourceCorrelationID != nil && *claims[i].SourceCorrelationID == commandStartedRunID {
			eventClaim = &claims[i]
			break
		}
	}
	if eventClaim == nil {
		t.Fatalf("consumer não ativou claim para o run product job.run %s: claims=%+v", commandStartedRunID, claims)
	}
	// Keep command maintenance mounted while the blocked job remains active:
	// its runtime projection is part of the live authority witness, not merely
	// a source of background cadence that can be shut down during this proof.
	if eventClaim.LayerRef != layerID || eventClaim.LayerRefKind != commandactivation.UserRef {
		t.Fatalf("claim real não corresponde à camada preparada: claim=(%s,%s) prepared=(user,%s)", eventClaim.LayerRefKind, eventClaim.LayerRef, layerID)
	}
	product := a.commandProduct.Load()
	if product == nil {
		t.Fatal("produto de comandos ausente")
	}
	scope, err := a.commandMutationCurrentScope(product.principal)
	if err != nil {
		t.Fatalf("capturar escopo de configuração corrente: %v", err)
	}
	jobProjection, _, err := a.commandJobLayerProjection(ctx, product.principal, scope)
	if err != nil {
		t.Fatalf("consultar projeção produtiva do consumer para a claim: %v", err)
	}
	if jobProjection == nil || !slices.Contains(jobProjection.layers, eventClaim.LayerRef) {
		t.Fatalf("claim ativa não está na projeção produtiva: activation=%s run=%s layer=%s workspace=%v scopeWorkspace=%v projected=%v", eventClaim.ActivationID, commandStartedRunID, eventClaim.LayerRef, eventClaim.WorkspaceID, scope.WorkspaceID, func() []string {
			if jobProjection == nil {
				return nil
			}
			return jobProjection.layers
		}())
	}
	candidate := paletteResolutionCandidate(t)
	var unprovenContext context.Context
	epoch, err := product.host.Epochs().Capture(ctx, product.principal.UserID, product.principal.SessionID)
	if err != nil {
		t.Fatalf("capturar epoch do watch sem prova: %v", err)
	}
	unprovenRelease, err := product.host.Epochs().AdmitExecution(ctx, epoch,
		func(context.Context) error { return nil },
		func(watch context.Context) error { unprovenContext = watch; return nil })
	if err != nil {
		t.Fatalf("inscrever watch de execução sem prova: %v", err)
	}
	defer unprovenRelease()
	if err := product.refreshCommandJobProjection(ctx); err != nil {
		t.Fatalf("publicar claim do evento no host: %v", err)
	}
	select {
	case <-unprovenContext.Done():
	case <-ctx.Done():
		t.Fatal("publicação não cancelou o watch sem prova", ctx.Err())
	}
	var causalContext context.Context
	select {
	case causalContext = <-runTool.startedContext:
	case <-ctx.Done():
		t.Fatal("tool do job.run não publicou seu contexto causal", ctx.Err())
	}
	select {
	case <-causalContext.Done():
		t.Fatal("publicação de claim cancelou o contexto causal vivo do job.run")
	default:
	}
	_, activeLayers, versions, err := product.host.ResolutionSnapshot(ctx, product.principal)
	if err != nil {
		t.Fatalf("snapshot do host antes da seleção downstream: %v", err)
	}
	if !slices.Contains(activeLayers, eventClaim.LayerRef) {
		t.Fatalf("claim ativa do evento ausente da projeção host: activation=%s layer=%s active=%v", eventClaim.ActivationID, eventClaim.LayerRef, activeLayers)
	}
	runs, err = a.jobMgr.GetJobRunsContext(ctx, job.ID, 10)
	if err != nil || len(runs) != 1 || runs[0].RunID != commandStartedRunID || runs[0].Status != jobs.RunStatusRunning {
		t.Fatalf("execução causal não permaneceu running após publicação: runs=%+v err=%v", runs, err)
	}
	source := commandcontract.SourcePalette
	resolution, err := product.resolvePersistedTrigger(ctx, product.principal, candidate, commandcontract.Envelope{
		SourceType: &source, RegistryVersion: versions.Registry,
		GlobalConfigGeneration: &versions.GlobalConfig, ActiveLayersGeneration: &versions.ActiveLayers,
	})
	if err != nil || resolution.Mode != commandcontract.ResolutionExecute || resolution.CommandID != commandProductWorkspaceListID {
		t.Fatalf("seleção downstream real: mode=%s command=%s bindingIDs=%v layerRefs=%v err=%v", resolution.Mode, resolution.CommandID, resolution.BindingIDs, resolution.LayerRefs, err)
	}
	if len(resolution.BindingIDs) != 1 || resolution.BindingIDs[0] != selectedBindingID || len(resolution.LayerRefs) != 1 || resolution.LayerRefs[0] != eventClaim.LayerRef || resolution.Provenance == nil {
		t.Fatalf("seleção não corresponde à claim/binding ativa: claim=(%s,%s) want binding=%s; resolved bindings=%v layers=%v provenance=%v", eventClaim.ActivationID, eventClaim.LayerRef, selectedBindingID, resolution.BindingIDs, resolution.LayerRefs, resolution.Provenance != nil)
	}

	downstream, err := product.execute(ctx, candidate)
	if err != nil || downstream.Status != commandledger.Succeeded || downstream.Envelope.CommandID == nil || *downstream.Envelope.CommandID != commandProductWorkspaceListID {
		t.Fatalf("comando downstream do evento: status=%s envelope=%+v err=%v", downstream.Status, downstream.Envelope, err)
	}
	if len(downstream.Envelope.BindingIDs) != 1 || downstream.Envelope.BindingIDs[0] != selectedBindingID {
		t.Fatalf("envelope downstream perdeu o binding selecionado: got=%v want=%s", downstream.Envelope.BindingIDs, selectedBindingID)
	}
	provenance := requireJobEnvelopeProvenance(t, readInvocationProvenance(t, downstream.InvocationID))
	if provenance["_chain_id"] != invocation || provenance["_source"] != "job" || provenance["_source_job_id"] != job.ID {
		t.Fatalf("memento não preservou raiz causal e job de origem: provenance=%v want chain=%s source=job/%s", provenance, invocation, job.ID)
	}
	replayed, err := product.execute(ctx, candidate)
	if err != nil || replayed.ID != downstream.ID || replayed.InvocationID != downstream.InvocationID || replayed.Status != commandledger.Succeeded {
		t.Fatalf("replay downstream não reutilizou a invocação: first=(%s,%s,%s) replay=(%s,%s,%s) err=%v", downstream.ID, downstream.InvocationID, downstream.Status, replayed.ID, replayed.InvocationID, replayed.Status, err)
	}

	runTool.releaseOnce.Do(func() { close(runTool.release) })
	select {
	case <-callbackDone:
	case <-ctx.Done():
		t.Fatal("callback do hotkey product job.run não concluiu", ctx.Err())
	}
	select {
	case err := <-dispatchDone:
		if err != nil {
			t.Fatalf("dispatch product job.run: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("dispatch product job.run não concluiu", ctx.Err())
	}
	runs, err = a.jobMgr.GetJobRunsContext(ctx, job.ID, 10)
	if err != nil || len(runs) != 1 || runs[0].RunID != commandStartedRunID || runs[0].Status != jobs.RunStatusCompleted {
		t.Fatalf("run foi duplicado/alterado pelo replay downstream: runs=%+v err=%v", runs, err)
	}
	drainCommandJobOutbox(t, a)
}

func createJobRunEventLayer(t *testing.T, a *App) string {
	t.Helper()
	layerID := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC()
	if err := database.DB().Create(&commandconfig.Layer{ID: layerID, UserID: a.currentUserID, Name: "job-run-event-layer-" + layerID, Enabled: true, Source: "user", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	return layerID
}

func createJobRunEventGrant(t *testing.T, a *App, ctx context.Context, layerID string) {
	t.Helper()
	db := database.DB()
	now := time.Now().UTC()
	if err := commandautomation.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	ruleID := uuid.Must(uuid.NewV7()).String()
	eventName, producers := commandautomation.JobRunStateEvent, `["jobs.runtime"]`
	rule := commandactivation.Rule{ID: ruleID, UserID: a.currentUserID, LayerRefKind: commandactivation.UserRef, LayerRef: layerID, RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, Mode: commandactivation.ModeEvent, Condition: `{"version":1,"clauses":[]}`, Lifecycle: commandactivation.LifecyclePersistent, EventName: &eventName, AllowedInternalProducerTypes: &producers, Enabled: false, Source: "user", ReviewStatus: "active"}
	if err := db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	decisions, err := commanddecision.New(db, liveCommandDecisionPresenter{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	automation, err := commandautomation.New(db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := a.commandEpochs.Capture(ctx, a.currentUserID, a.currentAuthUser.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	change, err := automation.PrepareRule(ctx, commandautomation.Owner{UserID: a.currentUserID}, ruleID)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := commandautomationKeyProvider(a)
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := automation.ConfirmGrant(ctx, change, epoch, decisions, a.commandStorageVersion, keys, now.Add(time.Hour), func(commandautomation.Rule) (string, error) {
		return "authorize job.run event integration", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := automation.CommitConfirmedGrant(ctx, confirmed, epoch); err != nil {
		t.Fatal(err)
	}
}

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

	// Reutilize o candidato para provar replay idempotente após a ativação real
	// do evento. Gerar um candidato novo aqui testaria uma segunda execução,
	// não a repetição da mesma invocação.
	candidate := paletteResolutionCandidate(t)
	product := a.commandProduct.Load()
	if product == nil {
		t.Fatal("produto de comandos ausente")
	}
	survivor, err := product.execute(context.Background(), candidate)
	if err != nil {
		t.Fatalf("executar comando na fonte sobrevivente: %v", err)
	}
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
	replayed, err := product.execute(context.Background(), candidate)
	if err != nil || replayed.Status != commandledger.Succeeded || replayed.ID != survivor.ID || replayed.InvocationID != survivor.InvocationID {
		t.Fatalf("replay do comando da fonte sobrevivente não reutilizou a invocação: first=(%s,%s,%s) replay=(%s,%s,%s) err=%v",
			survivor.ID, survivor.InvocationID, survivor.Status, replayed.ID, replayed.InvocationID, replayed.Status, err)
	}
	if persisted := readInvocationProvenance(t, replayed.InvocationID); persisted == nil {
		t.Fatal("replay perdeu a proveniência durável da execução original")
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
	for batch := 0; batch < 10; batch++ {
		result, err := mounted.ports.Outbox.Drain(context.Background(), 100)
		if err != nil {
			t.Fatalf("drenar outbox real: %v", err)
		}
		if !result.More {
			return
		}
	}
	t.Fatal("outbox real não esvaziou após dez lotes")
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
