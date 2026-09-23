package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandautomation"
	"assistente/internal/commandbindings"
	"assistente/internal/commanddecision"
	"assistente/internal/commandforeground"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/questionnaire"
)

type c43ForegroundReader struct {
	snapshot     commandforeground.Snapshot
	afterCapture *commandforeground.Snapshot
	err          error
	calls        int
}

func (r *c43ForegroundReader) Capture(context.Context) (commandforeground.Snapshot, error) {
	r.calls++
	captured := r.snapshot
	if r.afterCapture != nil {
		r.snapshot = *r.afterCapture
		r.afterCapture = nil
	}
	return captured, r.err
}

func c43ForegroundSnapshot(t *testing.T, window uintptr, process uint32, created uint64, executable string) commandforeground.Snapshot {
	t.Helper()
	snapshot, err := commandforeground.NewSnapshot(window, process, created, `C:\`+executable, "EditorWindow")
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func c43EnableSettingsSecurity(t *testing.T, a *App) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	a.ctx = ctx
	if err := commandautomation.Migrate(ctx, database.DB()); err != nil {
		t.Fatal(err)
	}
}

type c43GlobalJobFixture struct {
	app          *App
	ctx          context.Context
	decisions    <-chan map[string]any
	decisionMgr  *questionnaire.Manager
	registrar    *globalJobTestHotkeyRegistrar
	dispatchDone chan error
	job          *jobs.Job
	tool         *globalJobTestTool
	layerID      string
	suppressID   string
	trigger      string
}

func c43GlobalFixture(t *testing.T) c43GlobalJobFixture {
	t.Helper()
	a := commandMaintenanceAppFixture(t)
	c43EnableSettingsSecurity(t, a)
	decisionManager, decisionEvents := newCommandDecisionManager(t)
	a.questionnaireMgr = decisionManager
	registrar := &globalJobTestHotkeyRegistrar{}
	dispatchDone := make(chan error, 4)
	dispatch := func(ctx context.Context, occurrence jobs.CommandHotkeyOccurrence) error {
		err := a.dispatchCommandJobHotkey(ctx, occurrence)
		dispatchDone <- err
		return err
	}
	ctx := configureGlobalJobHotkeyTestManager(t, a, registrar, dispatch)
	tool := &globalJobTestTool{}
	job := createGlobalJobHotkeyTestJob(t, a, ctx, tool)
	if err := a.rebuildCommandLifecycleProjection(ctx, false); err != nil {
		t.Fatal(err)
	}

	layer := settingsContractApply(t, a, decisionEvents, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "layer_create",
		Layer: &CommandSettingsLayerInput{Name: "C43 foreground job suppression", Enabled: true, ResolutionPriority: 10},
	})
	settingsContractApply(t, a, decisionEvents, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "rule_create",
		Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "context", Lifecycle: "persistent", Enabled: true,
			Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "foreground.process", Op: "eq", Value: "editor.exe"}}}},
	})
	spec, trigger, err := commandGlobalTrigger("Ctrl+Alt+G")
	if err != nil {
		t.Fatal(err)
	}
	globalBindings, err := a.commandGlobalBindings(ctx)
	if err != nil {
		t.Fatalf("bindings globais builtin: %v", err)
	}
	var target commandGlobalBinding
	for _, binding := range globalBindings {
		if binding.CommandID == commandGlobalJobID && binding.Identity == trigger {
			target = binding
			break
		}
	}
	if target.ID == "" {
		t.Fatalf("default global builtin para %s não encontrado em commandGlobalBindings", trigger)
	}
	published, err := a.GetCommandSettingsForScope("pt-BR", "global")
	if err != nil {
		t.Fatalf("ler settings globais antes da supressão: %v", err)
	}
	publicDefault := settingsFindDefaultBinding(published, target.ID)
	if publicDefault.ID != target.ID || publicDefault.DefaultID != target.ID || publicDefault.TriggerType != "keyboard.global" || publicDefault.TriggerSpec == "" {
		t.Fatalf("GetCommandSettingsForScope não expôs o default global fielmente: %+v", publicDefault)
	}
	suppression := settingsContractApply(t, a, decisionEvents, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "binding_create",
		Binding: &CommandSettingsBindingInput{LayerID: layer.ID, TriggerType: "keyboard.global", TriggerSpec: string(spec),
			Arguments: map[string]any{}, Effect: "suppress", Enabled: true,
			ReplacesDefaultID: target.ID, ReplacesDefaultVersion: "1", ReplacesDefaultFingerprint: target.Fingerprint},
	})
	return c43GlobalJobFixture{app: a, ctx: ctx, decisions: decisionEvents, decisionMgr: decisionManager,
		registrar: registrar, dispatchDone: dispatchDone, job: job, tool: tool, layerID: layer.ID,
		suppressID: suppression.ID, trigger: trigger}
}

func c43CallGlobalJob(t *testing.T, fixture c43GlobalJobFixture, wantExecution bool) string {
	t.Helper()
	go fixture.registrar.callback(t, 0)()
	if !wantExecution {
		select {
		case err := <-fixture.dispatchDone:
			if err == nil {
				t.Fatal("camada contextual não suprimiu o job no processo correspondente")
			}
		case <-fixture.decisions:
			t.Fatal("job suprimido chegou à decisão/executor")
		case <-time.After(5 * time.Second):
			t.Fatal("dispatch contextual não terminou")
		}
		return ""
	}
	var decision map[string]any
	select {
	case decision = <-fixture.decisions:
	case err := <-fixture.dispatchDone:
		t.Fatalf("hotkey global terminou antes da decisão: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("decisão do job global não foi emitida")
	}
	finishCommandDecision(t, fixture.decisionMgr, decision, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
	select {
	case err := <-fixture.dispatchDone:
		if err != nil {
			t.Fatalf("dispatch global: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("dispatch global não publicou conclusão")
	}
	return "executed"
}

func TestCommandGlobalJobContextLayerSuppressesUsingCapturedForeground(t *testing.T) {
	fixture := c43GlobalFixture(t)
	reader := &c43ForegroundReader{}
	fixture.app.commandProduct.Load().foregroundReader = reader
	if err := fixture.app.jobMgr.Start(); err != nil {
		t.Fatal(err)
	}
	admitted := make(chan string, 2)
	fixture.app.emitter = globalJobTestEmitter(func(event string, data any) {
		if event != "command:global-job-admission" {
			return
		}
		invocationID := data.(map[string]string)["invocationId"]
		if !fixture.app.AdmitGlobalCommandOccurrence(invocationID, true) {
			t.Errorf("admission nativa recusada: %s", invocationID)
		}
		admitted <- invocationID
	})

	product := fixture.app.commandProduct.Load()
	configuration, _, _, err := product.host.ResolutionSnapshot(context.Background(), product.principal)
	if err != nil {
		t.Fatal(err)
	}
	if got := configuration.RequiredFacts(fixture.trigger); len(got) != 1 || got[0] != commandbindings.Process {
		t.Fatalf("facts requeridos pelo hotkey global=%v, want foreground.process", got)
	}
	matched, err := configuration.Resolve(fixture.trigger, commandbindings.Facts{commandbindings.Process: "editor.exe"}, nil)
	if err != nil || matched.Status != commandbindings.Suppressed {
		t.Fatalf("regra contextual persistida não suprimiu o default para editor.exe: %+v err=%v", matched, err)
	}
	nonmatched, err := configuration.Resolve(fixture.trigger, commandbindings.Facts{commandbindings.Process: "other.exe"}, nil)
	if err != nil || nonmatched.Status != commandbindings.Selected || nonmatched.CommandID != commandGlobalJobID {
		t.Fatalf("hotkey não seleciona o job builtin fora da camada contextual: %+v err=%v", nonmatched, err)
	}

	// O reader muda para other.exe imediatamente depois de devolver a captura
	// original. A camada ainda suprime esse evento já ingressado.
	reader.snapshot = c43ForegroundSnapshot(t, 1, 2, 3, "editor.exe")
	reader.afterCapture = snapshotPointer(c43ForegroundSnapshot(t, 4, 5, 6, "other.exe"))
	c43CallGlobalJob(t, fixture, false)
	if reader.calls != 1 || fixture.tool.calls.Load() != 0 {
		t.Fatalf("snapshot matching foi reavaliado após mudança: captures=%d tool calls=%d", reader.calls, fixture.tool.calls.Load())
	}

	// No sentido inverso, editor.exe após a captura não pode emprestar
	// autoridade ao snapshot other.exe: o default do job segue executável.
	reader.snapshot = c43ForegroundSnapshot(t, 7, 8, 9, "other.exe")
	reader.afterCapture = snapshotPointer(c43ForegroundSnapshot(t, 10, 11, 12, "editor.exe"))
	c43CallGlobalJob(t, fixture, true)
	if reader.calls != 2 || fixture.tool.calls.Load() != 1 {
		t.Fatalf("job real não executou pelo snapshot original: captures=%d tool calls=%d", reader.calls, fixture.tool.calls.Load())
	}
	invocationID := <-admitted
	var row struct {
		BindingIDs        string
		ForegroundSummary *string
		Status            string
	}
	if err := database.DB().Table("command_invocations").Select("binding_ids,foreground_summary,status").Where("invocation_id = ?", invocationID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	var bindingIDs []string
	if err := json.Unmarshal([]byte(row.BindingIDs), &bindingIDs); err != nil {
		t.Fatal(err)
	}
	for _, id := range bindingIDs {
		if id == fixture.suppressID {
			t.Fatalf("supressão contextual de outro processo entrou na proveniência: %v", bindingIDs)
		}
	}
	var summary struct {
		Executable      string `json:"executable"`
		WindowClass     string `json:"window_class"`
		ProviderVersion string `json:"provider_version"`
	}
	if row.ForegroundSummary == nil || json.Unmarshal([]byte(*row.ForegroundSummary), &summary) != nil ||
		summary.Executable != "other.exe" || summary.WindowClass != "EditorWindow" || summary.ProviderVersion == "" {
		t.Fatalf("resumo allowlisted não corresponde ao snapshot de ingresso: raw=%v decoded=%+v", row.ForegroundSummary, summary)
	}
	if row.Status != "succeeded" {
		t.Fatalf("invocation status=%s, want succeeded", row.Status)
	}
	runs, err := fixture.app.jobMgr.GetJobRunsContext(fixture.ctx, fixture.job.ID, 10)
	if err != nil || len(runs) != 1 || runs[0].JobID != fixture.job.ID || runs[0].RootOriginID != invocationID || runs[0].RootOriginType != "user_hotkey" {
		t.Fatalf("execução desviou do alvo/hotkey autorizado: runs=%+v err=%v", runs, err)
	}
}

func snapshotPointer(snapshot commandforeground.Snapshot) *commandforeground.Snapshot {
	return &snapshot
}

func TestCommandGlobalJobContextLayerWithoutForegroundFailsClosed(t *testing.T) {
	fixture := c43GlobalFixture(t)
	reader := &c43ForegroundReader{err: errors.New("foreground provider unavailable")}
	fixture.app.commandProduct.Load().foregroundReader = reader
	if err := fixture.app.jobMgr.Start(); err != nil {
		t.Fatal(err)
	}
	fixture.app.emitter = globalJobTestEmitter(func(string, any) { t.Error("admission emitida sem foreground confiável") })
	c43CallGlobalJob(t, fixture, false)
	if reader.calls != 1 || fixture.tool.calls.Load() != 0 {
		t.Fatalf("foreground captures=%d tool calls=%d; want 1/0", reader.calls, fixture.tool.calls.Load())
	}
}
