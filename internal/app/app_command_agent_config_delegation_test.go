package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/toolinvocations"
	commandtool "assistente/internal/tools/command"
	"assistente/internal/tools/invocationctx"
	"gorm.io/gorm"
)

func TestCommandAgentConfigRevalidatesCallerDuringLayerCreateDecision(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, context.Context, *App)
	}{
		{
			name: "invocation ended",
			mutate: func(t *testing.T, ctx context.Context, _ *App) {
				id := toolinvocations.CurrentInvocationID(ctx)
				if err := database.DB().Model(&database.ToolInvocation{}).Where("id = ?", id).Update("status", toolinvocations.StatusSucceeded).Error; err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "origin changed to job",
			mutate: func(t *testing.T, ctx context.Context, _ *App) {
				id := toolinvocations.CurrentInvocationID(ctx)
				if err := database.DB().Model(&database.ToolInvocation{}).Where("id = ?", id).Update("origin_type", toolinvocations.OriginJobRun).Error; err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "conversation changed to channel",
			mutate: func(t *testing.T, ctx context.Context, _ *App) {
				inv, ok := invocationctx.Get(ctx)
				if !ok {
					t.Fatal("invocation context ausente")
				}
				if err := database.DB().Model(&database.Conversation{}).Where("id = ?", inv.ConversationID).Update("channel", "telegram").Error; err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "conversation changed to subagent",
			mutate: func(t *testing.T, ctx context.Context, _ *App) {
				inv, ok := invocationctx.Get(ctx)
				if !ok {
					t.Fatal("invocation context ausente")
				}
				if err := database.DB().Model(&database.Conversation{}).Where("id = ?", inv.ConversationID).Update("kind", database.ConversationKindSubagent).Error; err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "user disabled",
			mutate: func(t *testing.T, _ context.Context, a *App) {
				if err := database.DB().Model(&database.User{}).Where("id = ?", a.currentUserID).Update("is_active", false).Error; err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, decisions := settingsSecurityFixture(t)
			backend := commandAgentTools{app: a}
			ctx := commandAgentTestContext(t, a, commandtool.ConfigName)
			view := agentConfigView(t, backend, ctx)
			const name = "must not persist during decision"
			request := CommandSettingsMutationRequest{
				Scope:               CommandSettingsScopeGlobal,
				Operation:           "layer_create",
				ExpectedRevision:    view.Revision,
				ExpectedFingerprint: view.Fingerprint,
				Layer:               &CommandSettingsLayerInput{Name: name, Enabled: true},
			}
			payload, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}

			done := make(chan struct {
				value any
				err   error
			}, 1)
			go func() {
				value, err := backend.Config(ctx, commandtool.Request{
					Action:  request.Operation,
					Scope:   string(request.Scope),
					Locale:  "pt-BR",
					Payload: payload,
				})
				done <- struct {
					value any
					err   error
				}{value, err}
			}()

			decision := agentConfigDecision(t, decisions)
			tc.mutate(t, ctx, a)
			finishCommandDecision(t, a.questionnaireMgr, decision, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)

			result := waitAgentConfigDecisionResult(t, done)
			if result.err == nil || !errors.Is(result.err, commandexecution.ErrDenied) || result.value != nil {
				t.Fatalf("caller invalidado durante decisão foi aceito: value=%v err=%v", result.value, result.err)
			}
			assertAgentConfigLayerAbsent(t, name)
		})
	}
}

func TestCommandAgentConfigRevalidatesCancelledContextAfterLayerCreateConfirmation(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	backend := commandAgentTools{app: a}
	baseCtx := commandAgentTestContext(t, a, commandtool.ConfigName)
	ctx, cancel := context.WithCancel(baseCtx)
	t.Cleanup(cancel)
	view := agentConfigView(t, backend, ctx)
	const name = "must not persist after context cancellation"
	request := CommandSettingsMutationRequest{
		Scope:               CommandSettingsScopeGlobal,
		Operation:           "layer_create",
		ExpectedRevision:    view.Revision,
		ExpectedFingerprint: view.Fingerprint,
		Layer:               &CommandSettingsLayerInput{Name: name, Enabled: true},
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct {
		value any
		err   error
	}, 1)
	go func() {
		value, err := backend.Config(ctx, commandtool.Request{
			Action:  request.Operation,
			Scope:   string(request.Scope),
			Locale:  "pt-BR",
			Payload: payload,
		})
		done <- struct {
			value any
			err   error
		}{value, err}
	}()

	decision := agentConfigDecision(t, decisions)
	blocked := make(chan struct{})
	release := make(chan struct{})
	var armed atomic.Bool
	var intercepted atomic.Bool
	armed.Store(true)
	const callbackName = "test:command-agent-config-context-cancel"
	if err := database.DB().Callback().Query().Before("gorm:query").Register(callbackName, func(_ *gorm.DB) {
		if !armed.Load() || !intercepted.CompareAndSwap(false, true) {
			return
		}
		close(blocked)
		<-release
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		armed.Store(false)
		select {
		case <-release:
		default:
			close(release)
		}
		if err := database.DB().Callback().Query().Remove(callbackName); err != nil {
			t.Errorf("remove callback: %v", err)
		}
	})

	if err := a.questionnaireMgr.Respond(decision["id"].(string), map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocked:
		cancel()
		close(release)
	case <-time.After(5 * time.Second):
		t.Fatal("nenhuma consulta foi interceptada durante a confirmação")
	}

	result := waitAgentConfigDecisionResult(t, done)
	if result.err == nil || !errors.Is(result.err, commandexecution.ErrDenied) || result.value != nil {
		t.Fatalf("contexto cancelado após confirmação foi aceito: value=%v err=%v", result.value, result.err)
	}
	assertAgentConfigLayerAbsent(t, name)
}

func waitAgentConfigDecisionResult(t *testing.T, done <-chan struct {
	value any
	err   error
}) struct {
	value any
	err   error
} {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("Config não encerrou após a revalidação")
		return struct {
			value any
			err   error
		}{}
	}
}

func assertAgentConfigLayerAbsent(t *testing.T, name string) {
	t.Helper()
	var count int64
	if err := database.DB().Model(&commandconfig.Layer{}).Where("name = ?", name).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("camada persistiu apesar da revalidação: %d", count)
	}
}
