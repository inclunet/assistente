package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"gorm.io/gorm"
)

type recoveryUnexpectedPresenter struct{ calls int }

func (p *recoveryUnexpectedPresenter) Present(context.Context, commanddecision.Request) (commanddecision.Response, error) {
	p.calls++
	return commanddecision.Response{}, errors.New("recuperação não apresenta decisão")
}

func exerciseCommandDecisionRecovery(t *testing.T, app *App, db *gorm.DB, config *commandconfig.Store,
	state *commandexecution.HostState, token, userID string, options commandconfig.LocalReadProjection,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	presenter := &recoveryUnexpectedPresenter{}
	receipts, err := commanddecision.New(db, presenter, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	before, err := config.Load(ctx, commandconfig.Scope{UserID: userID})
	if err != nil {
		t.Fatal(err)
	}
	mapBefore, _, err := state.UserConfiguration(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	authorize := func(_ context.Context, p auth.LocalSessionPrincipal) error {
		if p.UserID != userID {
			return errors.New("usuário inesperado")
		}
		return nil
	}
	if result, err := app.recoverCommandDecisionSession(ctx, "invalid-token", receipts, authorize); err == nil || result.Closed != 0 {
		t.Fatal("recuperação aceitou token inválido", result, err)
	}
	denied := errors.New("política da fixture negou")
	if result, err := app.recoverCommandDecisionSession(ctx, token, receipts, func(context.Context, auth.LocalSessionPrincipal) error { return denied }); !errors.Is(err, denied) || result.Closed != 0 {
		t.Fatal("recuperação ignorou política", result, err)
	}
	if current, _, err := state.UserConfiguration(ctx, userID); err != nil || current != mapBefore {
		t.Fatal("falha de autenticação alterou mapa", err)
	}
	// O cenário policy_revoked anterior deixou accepted sob epoch antigo;
	// lock/unlock posterior garante que essa confirmação não pode sobreviver.
	result, err := app.recoverCommandDecisionSession(ctx, token, receipts, authorize)
	if err != nil || result.Closed != 1 || result.More {
		t.Fatal("recuperação incorreta", result, err)
	}
	if _, _, err := state.UserConfiguration(ctx, userID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatal("recuperação manteve mapa antigo", err)
	}
	if result, err := app.recoverCommandDecisionSession(ctx, token, receipts, authorize); err != nil || result.Closed != 0 || result.More {
		t.Fatal("recuperação não idempotente", result, err)
	}
	after, err := config.Load(ctx, commandconfig.Scope{UserID: userID})
	if err != nil || !reflect.DeepEqual(before.Bindings, after.Bindings) || !reflect.DeepEqual(before.Generations, after.Generations) {
		t.Fatal("recuperação repetiu efeito", err)
	}
	var consumed, mutations int64
	if err := db.Table("command_decision_receipts").Where("status = ?", commanddecision.Consumed).Count(&consumed).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("command_config_mutations").Count(&mutations).Error; err != nil {
		t.Fatal(err)
	}
	if consumed != 1 || mutations != 1 || presenter.calls != 0 {
		t.Fatal("efeito/diálogo repetido", consumed, mutations, presenter.calls)
	}
	if err := app.rebuildPersistedLocalReadConfiguration(ctx, token, config, options); err != nil {
		t.Fatal(err)
	}
}
