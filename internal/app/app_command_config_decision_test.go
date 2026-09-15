package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/questionnaire"
	"gorm.io/gorm"
)

// Exercita a sessão real da fixture e o presenter do App, sem janela nativa,
// secret manager real ou migração automática do banco do usuário.
func exerciseCommandBindingDecisions(t *testing.T, app *App, db *gorm.DB, store *commandconfig.Store,
	state *commandexecution.HostState, token, userID, bindingID string, options commandconfig.LocalReadProjection,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := commanddecision.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	load := func() (bool, int64) {
		t.Helper()
		snapshot, err := store.Load(ctx, commandconfig.Scope{UserID: userID})
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range snapshot.Bindings {
			if row.ID == bindingID {
				return row.Enabled, snapshot.Generations[0].Generation
			}
		}
		t.Fatal("binding ausente")
		return false, 0
	}
	initial, generation := load()
	for _, scenario := range []string{"denied", "policy_revoked", "locked", "success"} {
		var manager *questionnaire.Manager
		presented, revoked := false, false
		manager = questionnaire.NewManager(func(event string, data any) {
			if event != questionnaire.EventQuestionnaire {
				return
			}
			presented = true
			payload := data.(map[string]any)
			if payload["kind"] != questionnaire.KindDecision || payload["body"] != "fixture: enabled" {
				t.Fatal("diálogo incorreto")
			}
			if scenario == "policy_revoked" {
				revoked = true
			}
			if scenario == "locked" {
				// Deve ser possível adquirir gate enquanto a decisão está aberta.
				if err := state.SetOSSessionState(ctx, true, true); err != nil {
					t.Fatal(err)
				}
			}
			action := commanddecision.ApplyAction
			if scenario == "denied" {
				action = commanddecision.DenyAction
			}
			if err := manager.Respond(payload["id"].(string), map[string]any{questionnaire.AnswerActionID: action}, false); err != nil {
				t.Fatal(err)
			}
		})
		receipts, err := commanddecision.New(db, &commandDecisionPresenter{manager: manager}, time.Now)
		if err != nil {
			t.Fatal(err)
		}
		policyDenied := errors.New("política revogada na fixture")
		authorize := func(_ context.Context, p auth.LocalSessionPrincipal) error {
			if p.UserID != userID {
				t.Fatal("principal incorreto")
			}
			if revoked {
				return policyDenied
			}
			return nil
		}
		keys := func(_ context.Context, ref string) ([]byte, error) {
			if ref != "command-request-hmac:v1" {
				t.Fatal("referência de chave incorreta")
			}
			return []byte("fixture-key-only-0123456789abcdef"), nil
		}
		render := func(before, after commandconfig.Binding) (string, error) {
			if before.ID != bindingID || before.Enabled != initial || after.Enabled == initial {
				t.Fatal("diff divergente")
			}
			return "fixture: enabled", nil
		}
		if scenario == "denied" {
			if err := app.changeCommandBindingEnabledWithDecision(ctx, "invalid-token", store, bindingID, !initial, options, authorize, receipts, "v1", keys, render); err == nil || presented {
				t.Fatal("token inválido abriu decisão", err)
			}
		}
		err = app.changeCommandBindingEnabledWithDecision(ctx, token, store, bindingID, !initial, options, authorize, receipts, "v1", keys, render)
		if !presented {
			t.Fatal("decisão não foi apresentada", scenario, err)
		}
		if scenario == "success" && err != nil {
			t.Fatal(err)
		}
		if scenario != "success" && err == nil {
			t.Fatal("escrita indevida", scenario)
		}
		if scenario == "policy_revoked" && !errors.Is(err, policyDenied) {
			t.Fatal("política não revalidada", err)
		}
		enabled, current := load()
		var consumed int64
		if err := db.Table("command_decision_receipts").Where("status = ?", commanddecision.Consumed).Count(&consumed).Error; err != nil {
			t.Fatal(err)
		}
		if scenario == "success" {
			if enabled == initial || current != generation+1 || consumed != 1 {
				t.Fatal("efeito/receipt divergente", enabled, current, consumed)
			}
			if _, _, err := state.UserConfiguration(ctx, userID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
				t.Fatal("mapa não invalidado", err)
			}
		} else if enabled != initial || current != generation || consumed != 0 {
			t.Fatal("falha alterou banco", scenario)
		}
		if scenario == "locked" {
			if err := state.SetOSSessionState(ctx, true, false); err != nil {
				t.Fatal(err)
			}
		}
		if err := app.rebuildPersistedLocalReadConfiguration(ctx, token, store, options); err != nil {
			t.Fatal(err)
		}
	}
}
