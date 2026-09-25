package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/questionnaire"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
)

type desktopCommandTool struct {
	calls atomic.Int32
	seen  chan string
}

func (*desktopCommandTool) Name() string        { return "test.command_tool" }
func (*desktopCommandTool) Description() string { return "efeito controlado da delegação" }
func (*desktopCommandTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"secret":{"type":"string"}},"required":["secret"]}`)
}

// O binding reativo não persiste um segredo de teste como argumento literal.
// Usa uma tool de entrada vazia; o cenário direto mantém o schema obrigatório.
type reactiveDesktopCommandTool struct{ *desktopCommandTool }

func (*reactiveDesktopCommandTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"secret":{"type":"string"}}}`)
}
func (p *desktopCommandTool) Execute(_ context.Context, args json.RawMessage) (tools.ToolResult, error) {
	p.calls.Add(1)
	var input map[string]string
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.ToolResult{}, err
	}
	p.seen <- input["secret"]
	raw, _ := json.Marshal(map[string]string{"echo": input["secret"]})
	return tools.ToolResult{Content: string(raw)}, nil
}

func TestCommandToolDesktopDecisionAndRevalidation(t *testing.T) {
	for _, scenario := range []string{"success", "denied", "cancelled", "replacement", "unavailable", "schema_changed", "session_revoked", "reactive", "reactive_retired"} {
		t.Run(scenario, func(t *testing.T) {
			reactive := strings.HasPrefix(scenario, "reactive")
			a := commandJobPublicationApp(t)
			ctx, cancel := context.WithTimeout(database.WithUserID(context.Background(), a.currentUserID), 15*time.Second)
			defer cancel()
			effect := &desktopCommandTool{seen: make(chan string, 1)}
			var registered tools.Tool = effect
			if reactive {
				registered = &reactiveDesktopCommandTool{effect}
			}
			a.toolRegistry.MustRegister(registered)
			row := database.ToolCatalog{Name: effect.Name(), DisplayName: effect.Name(), Origin: "builtin", Schema: string(registered.Parameters()), AvailabilityStatus: "available"}
			if err := database.DB().Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			if err := a.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
				t.Fatal(err)
			}
			definition := appCommandJobHandlerDefinition()
			definition.ID, definition.HandlerClassification, definition.HandlerRoute = "app.command_tool_test", commandcatalog.HandlerTool, "app/test-tool"
			definition.ArgumentsSchema = &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{"secret": {Type: commandcatalog.SchemaString}}, Required: []string{"secret"}}
			definition.ResultSchema = &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{"echo": {Type: commandcatalog.SchemaString}}, Required: []string{"echo"}}
			definition.SensitivePaths = commandcatalog.SensitivePaths{Input: []string{"/secret"}, Output: []string{"/echo"}}
			if reactive {
				definition.ArgumentsSchema.Required = nil
				definition.ArgumentsSchema.Properties["secret"] = commandcatalog.Schema{Type: commandcatalog.SchemaString, Optional: true}
			}
			contract := commandcatalog.HandlerContract{Effect: definition.Effect, HasMutableTarget: true, Classification: commandcatalog.HandlerTool, Route: definition.HandlerRoute}
			handler, err := a.newCommandToolHandler(ctx, definition, contract, row.ID, commandcatalog.SensitivePaths{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{{Definition: definition, Handler: contract}})
			if err != nil {
				t.Fatal(err)
			}
			var parent *liveCommandJobControl
			var parentRun string
			if reactive {
				control := startCommandMaintenanceLiveJob(t, a)
				parent, parentRun = &control, <-control.RunID
				claim, _ := waitLiveClaimAndLease(t, parentRun)
				if err := a.jobMgr.CloseCommandMaintenance(ctx); err != nil {
					t.Fatal(err)
				}
				publishReactiveJobTestBinding(t, a, registry, row.ID, definition.ID, claim, `{}`)
			}
			events := make(chan map[string]any, 2)
			a.questionnaireMgr = questionnaire.NewManager(func(event string, data any) {
				if event == questionnaire.EventQuestionnaire {
					events <- data.(map[string]any)
				}
			})
			config := completeFactoryConfig(registry, a.commandHost, make(chan struct{}, 1), a.currentUserID)
			config.RegistryVersion = commandProductRegistryVersion
			config.Handlers = map[string]commandexecution.Handler{definition.ID: handler}
			config.Store, err = commandledger.New(database.DB(), time.Now)
			if err != nil {
				t.Fatal(err)
			}
			config.Envelope.Authorize = func(check context.Context, principal auth.LocalSessionPrincipal, _ commandcontract.Envelope, d commandcatalog.Definition) error {
				current, err := a.sessionSvc.RevalidateLocalSession(check, principal)
				if err != nil || current != principal || principal.UserID != a.currentUserID || d.ID != definition.ID {
					return commandexecution.ErrDenied
				}
				return nil
			}
			snapshot := config.Envelope.Snapshot
			config.Envelope.Snapshot = func(check context.Context, principal auth.LocalSessionPrincipal, candidate commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
				envelope, err := snapshot(check, principal, candidate)
				workspace := a.workspaceMgr.ActiveID()
				envelope.WorkspaceID, envelope.WorkspaceConfigGeneration = &workspace, envelope.GlobalConfigGeneration
				return envelope, err
			}
			if reactive {
				config.Envelope.Resolve = reactiveJobTestResolver(a)
			}
			service, err := a.newCommandDesktopExecutor(config, a.commandHost)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
			secret := "sensitive-value-command-tool-47"
			candidate := completeCandidateFor(definition.ID)
			candidate.Arguments = json.RawMessage(`{"secret":"` + secret + `"}`)
			if reactive {
				secret = ""
				candidate, err = commandPaletteCandidate(candidate.InvocationID, candidate.CorrelationID, definition.ID, json.RawMessage(`{}`))
				if err != nil {
					t.Fatal(err)
				}
			}
			type execution struct {
				record commandledger.FullRecord
				output json.RawMessage
				err    error
			}
			done := make(chan execution, 1)
			go func() {
				record, output, err := service.ExecuteEnvelopeWithResult(ctx, "", candidate)
				done <- execution{record, output, err}
			}()
			var payload map[string]any
			select {
			case payload = <-events:
			case early := <-done:
				t.Fatalf("antes do diálogo: %s %v", early.record.Status, early.err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if effect.calls.Load() != 0 {
				t.Fatal("efeito antes da confirmação")
			}
			action, cancelled := commanddecision.ApplyAction, false
			var replacement *desktopCommandTool
			switch scenario {
			case "reactive_retired":
				parent.Release()
				if run, err := parent.Join(); err != nil || run == nil || run.Status != jobs.RunStatusCompleted {
					t.Fatalf("encerrar fonte: %+v %v", run, err)
				}
			case "denied":
				action = commanddecision.DenyAction
			case "cancelled":
				cancelled = true
			case "replacement":
				a.toolRegistry.Unregister(effect.Name())
				replacement = &desktopCommandTool{seen: make(chan string, 1)}
				a.toolRegistry.MustRegister(replacement)
			case "unavailable":
				if err := database.DB().Model(&database.ToolCatalog{}).Where("id = ?", row.ID).Update("availability_status", "unavailable").Error; err != nil {
					t.Fatal(err)
				}
			case "schema_changed":
				if err := database.DB().Model(&database.ToolCatalog{}).Where("id = ?", row.ID).Update("schema", `{"type":"object"}`).Error; err != nil {
					t.Fatal(err)
				}
			case "session_revoked":
				if err := database.DB().Model(&database.Session{}).Where("id = ?", a.currentAuthUser.SessionID).Update("revoked_at", time.Now().UTC()).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := a.questionnaireMgr.Respond(payload["id"].(string), map[string]any{questionnaire.AnswerActionID: action}, cancelled); err != nil {
				t.Fatal(err)
			}
			var got execution
			select {
			case got = <-done:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			var rows []database.ToolInvocation
			if err := database.DB().Where("origin_type = ? AND origin_id = ? AND user_id = ?", toolinvocations.OriginCommandInvocation, candidate.InvocationID, a.currentUserID).Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			if scenario != "success" && scenario != "reactive" {
				wantStatus, wantRows := commandledger.Failed, 1
				switch scenario {
				case "denied", "cancelled":
					wantStatus, wantRows = commandledger.Denied, 0
				case "session_revoked", "reactive_retired":
					wantStatus, wantRows = commandledger.CancelledStale, 0
				}
				if got.err != nil || got.record.Status != wantStatus || len(rows) != wantRows || effect.calls.Load() != 0 || replacement != nil && replacement.calls.Load() != 0 {
					t.Fatalf("recusa executou: %s %v", got.record.Status, got.err)
				}
				for _, inv := range rows {
					if inv.Status != toolinvocations.StatusFailed || inv.ErrorCode != "execution_guard_denied" {
						t.Fatalf("invocação recusada pendente/sucesso: %+v", inv)
					}
				}
				return
			}
			if got.err != nil || got.record.Status != commandledger.Succeeded || effect.calls.Load() != 1 || len(rows) != 1 {
				t.Fatalf("tool: status=%s err=%v calls=%d rows=%d", got.record.Status, got.err, effect.calls.Load(), len(rows))
			}
			if <-effect.seen != secret || !strings.Contains(string(got.output), secret) {
				t.Fatal("valor de execução/resultado efêmero foi redigido")
			}
			persisted, _ := json.Marshal(rows[0])
			commandRecord, _ := json.Marshal(got.record)
			if secret != "" && (strings.Contains(string(persisted), secret) || strings.Contains(string(commandRecord), secret)) {
				t.Fatal("segredo persistido no ledger")
			}
			if rows[0].ToolCatalogID != row.ID || rows[0].Status != toolinvocations.StatusSucceeded || got.record.Envelope.AuthorizationDecisionID == nil {
				t.Fatalf("correlação/decisão inválida: %+v", rows[0])
			}
			replay, output, err := service.ExecuteEnvelopeWithResult(ctx, "", candidate)
			if err != nil || replay.Status != commandledger.Succeeded || len(output) != 0 || effect.calls.Load() != 1 {
				t.Fatalf("replay duplicou efeito/output: %s %v", replay.Status, err)
			}
			var count int64
			if err := database.DB().Model(&database.ToolInvocation{}).Where("origin_id = ?", candidate.InvocationID).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("invocações no replay: %d %v", count, err)
			}
			if reactive {
				var audit struct{ Provenance *string }
				if err := database.DB().Table("command_invocations").Select("provenance").Where("invocation_id = ? AND user_id = ?", candidate.InvocationID, a.currentUserID).Take(&audit).Error; err != nil {
					t.Fatal(err)
				}
				var provenance map[string]json.RawMessage
				if audit.Provenance == nil || json.Unmarshal([]byte(*audit.Provenance), &provenance) != nil {
					t.Fatal("proveniência reativa ausente")
				}
				var chainID string
				if json.Unmarshal(provenance["_chain_id"], &chainID) != nil || chainID != parentRun {
					t.Fatal("tool reiniciou a cadeia reativa")
				}
				parent.Release()
				if run, err := parent.Join(); err != nil || run == nil || run.Status != jobs.RunStatusCompleted {
					t.Fatalf("fonte: %+v %v", run, err)
				}
			}
		})
	}
}
