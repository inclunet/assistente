package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/toolinvocations"
	commandtool "assistente/internal/tools/command"
	"assistente/internal/tools/invocationctx"
	"github.com/google/uuid"
)

// Even a context carrying the desktop stamp cannot lend its authority to a
// different persisted origin. Exercise both public tool ports, not only the
// identity helper; denied requests must not reach a presenter or create work.
func TestCommandAgentAdmissionRejectsBorrowedDesktopAuthority(t *testing.T) {
	cases := []struct {
		name, table, column string
		value               any
	}{
		{"job run", "tool_invocations", "origin_type", toolinvocations.OriginJobRun},
		{"tool catalog", "tool_invocations", "origin_type", toolinvocations.OriginToolCatalog},
		{"nested command", "tool_invocations", "origin_type", toolinvocations.OriginCommandInvocation},
		{"subagent conversation", "conversations", "kind", database.ConversationKindSubagent},
		{"external channel", "conversations", "channel", "telegram"},
		{"foreign conversation owner", "conversations", "user_id", uuid.Must(uuid.NewV7()).String()},
		{"foreign turn", "tool_invocations", "origin_id", uuid.Must(uuid.NewV7()).String()},
		{"stopped caller", "tool_invocations", "status", toolinvocations.StatusSucceeded},
		{"disabled owner", "users", "is_active", false},
	}
	for _, port := range []string{commandtool.CatalogName, commandtool.ConfigName} {
		for _, tc := range cases {
			t.Run(port+"/"+tc.name, func(t *testing.T) {
				a, decisions := settingsSecurityFixture(t)
				ctx := commandAgentTestContext(t, a, port)
				backend := commandAgentTools{app: a}
				// Positive control: the original local caller really is admissible.
				if _, err := a.commandAgentAccess(ctx, port); err != nil {
					t.Fatal(err)
				}
				var request commandtool.Request
				if port == commandtool.ConfigName {
					view := agentConfigView(t, backend, ctx)
					payload, err := json.Marshal(CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", ExpectedRevision: view.Revision, ExpectedFingerprint: view.Fingerprint, Layer: &CommandSettingsLayerInput{Name: "borrowed authority", Enabled: true}})
					if err != nil {
						t.Fatal(err)
					}
					request = commandtool.Request{Action: "layer_create", Scope: "global", Payload: payload}
				} else {
					request = commandtool.Request{Action: "execute", CommandID: commandProductWorkspaceListID, Arguments: json.RawMessage(`{}`)}
				}
				id := toolinvocations.CurrentInvocationID(ctx)
				if tc.table == "conversations" {
					inv, _ := invocationctx.Get(ctx)
					id = inv.ConversationID
				}
				if tc.table == "users" {
					id = a.currentUserID
				}
				result := database.DB().Table(tc.table).Where("id = ?", id).Update(tc.column, tc.value)
				if result.Error != nil || result.RowsAffected != 1 {
					t.Fatalf("fixture mutation: %v rows=%d", result.Error, result.RowsAffected)
				}
				// Bound a mistakenly opened prompt so the test fails rather than hangs.
				callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				done := make(chan error, 1)
				go func() {
					var value any
					var err error
					if port == commandtool.ConfigName {
						value, err = backend.Config(callCtx, request)
					} else {
						value, err = backend.Catalog(callCtx, request)
					}
					if value != nil {
						done <- errors.New("unauthorized request returned a value")
						return
					}
					done <- err
				}()
				select {
				case err := <-done:
					if !errors.Is(err, commandexecution.ErrDenied) {
						t.Fatalf("want denial, got %v", err)
					}
				case <-decisions:
					cancel()
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Fatal("unauthorized call did not terminate after cancellation")
					}
					t.Fatal("borrowed authority opened an interactive decision")
				case <-callCtx.Done():
					t.Fatal("unauthorized call did not terminate promptly")
				}
				select {
				case <-decisions:
					t.Fatal("denied call also emitted a decision")
				default:
				}
				for table, query := range map[string]string{"command_invocations": "1 = 1", "command_layers": "name = 'borrowed authority'"} {
					var count int64
					if err := database.DB().Table(table).Where(query).Count(&count).Error; err != nil || count != 0 {
						t.Fatalf("unexpected side effect in %s: %d %v", table, count, err)
					}
				}
			})
		}
	}
}

func TestCommandAgentLayerActivationRevalidatesCallerAfterDecision(t *testing.T) {
	for _, revoked := range []bool{false, true} {
		name := "job origin"
		if revoked {
			name = "ended invocation"
		}
		t.Run(name, func(t *testing.T) {
			a, decisions := settingsSecurityFixture(t)
			layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Protected", Enabled: true}})
			rule := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "manual", Lifecycle: "persistent", Enabled: true}})
			ctx := commandAgentTestContext(t, a, commandtool.CatalogName)
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			done := make(chan struct {
				value any
				err   error
			}, 1)
			go func() {
				value, err := (commandAgentTools{app: a}).Catalog(ctx, commandtool.Request{Action: "execute", CommandID: commandLayerActivateID, Locale: "pt-BR", Arguments: json.RawMessage(`{"scope":"global","rule_id":"` + rule.ID + `","duration_seconds":0}`)})
				done <- struct {
					value any
					err   error
				}{value, err}
			}()
			decision := agentConfigDecision(t, decisions)
			column, value := "origin_type", toolinvocations.OriginJobRun
			if revoked {
				column, value = "status", toolinvocations.StatusSucceeded
			}
			if err := database.DB().Model(&database.ToolInvocation{}).Where("id = ?", toolinvocations.CurrentInvocationID(ctx)).Update(column, value).Error; err != nil {
				t.Fatal(err)
			}
			finishCommandDecision(t, a.questionnaireMgr, decision, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
			select {
			case result := <-done:
				if result.err != nil {
					t.Fatal(result.err)
				}
				// A caller invalidated after admission terminates the durable
				// invocation as stale, rather than returning an ingress denial.
				if execution := commandAgentExecutionJSON(t, result.value); execution.Status != string(commandledger.CancelledStale) {
					t.Fatalf("invalidated caller result: %+v", execution)
				}
			case <-ctx.Done():
				t.Fatal("execution did not terminate")
			}
			var active int64
			if err := database.DB().Table("command_layer_activation_state").Where("user_id = ? AND rule_ref = ? AND state = ?", a.currentUserID, rule.ID, "active").Count(&active).Error; err != nil || active != 0 {
				t.Fatalf("unauthorized activation: %d %v", active, err)
			}
		})
	}
}
