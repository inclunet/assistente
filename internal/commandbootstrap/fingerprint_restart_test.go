package commandbootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/commandjson"
	"assistente/internal/commandledger"
	"github.com/google/uuid"
)

type restartDecisionPresenter struct{}

func (restartDecisionPresenter) Present(_ context.Context, r commanddecision.Request) (commanddecision.Response, error) {
	return commanddecision.Response{DecisionID: r.DecisionID, ActionID: commanddecision.ApplyAction}, nil
}

func TestKeysRestartRecognizesStoredFingerprintFormats(t *testing.T) {
	for _, format := range []string{"canonical", "versioned", "unknown-version", "malformed", "canonical-missing-key"} {
		t.Run(format, func(t *testing.T) {
			db, manager, dek := preparedKeysFixture(t)
			ctx := context.Background()
			provider, err := commandledger.NewCredentialKeyProvider(manager)
			if err != nil {
				t.Fatal(err)
			}
			key, err := provider(ctx, "command-request-hmac:v1")
			if err != nil {
				t.Fatal(err)
			}
			fingerprint, err := commandjson.HMAC(key, "assistente.command.config-mutation.v2:v1", []byte(`{"version":2}`))
			clear(key)
			if err != nil {
				t.Fatal(err)
			}
			switch format {
			case "versioned":
				fingerprint = "v1:" + fingerprint
			case "unknown-version":
				fingerprint = "v999:" + fingerprint
			case "malformed":
				fingerprint = strings.Repeat("z", 64)
			}
			store, err := commanddecision.New(db, restartDecisionPresenter{}, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			request := commanddecision.Request{DecisionID: uuid.Must(uuid.NewV7()).String(), MutationID: uuid.Must(uuid.NewV7()).String(), UserID: uuid.Must(uuid.NewV7()).String(), SessionID: uuid.Must(uuid.NewV7()).String(), Fingerprint: fingerprint, AuthGeneration: "a1", SecurityGeneration: "s1", ExpiresAt: time.Now().Add(time.Hour), Body: "Teste"}
			if _, err := store.Decide(ctx, request); err != nil {
				t.Fatal(err)
			}
			manager = loadedKeysManager(t, dek)
			if format == "canonical-missing-key" {
				if err := db.Exec("DELETE FROM credential_entries WHERE pattern = ?", commandKeyPattern("v1")).Error; err != nil {
					t.Fatal(err)
				}
				manager = loadedKeysManager(t, dek)
			}
			version, err := PrepareKeys(ctx, db, manager)
			if format == "unknown-version" || format == "malformed" || format == "canonical-missing-key" {
				if !errors.Is(err, ErrKeys) {
					t.Fatalf("referência inválida aceita: version=%q err=%v", version, err)
				}
				return
			}
			if err != nil || version != "v1" {
				t.Fatalf("reinício rejeitou fingerprint %s produzido pelo signer: %v", format, err)
			}
			var stored string
			if err := db.Table("command_decision_receipts").Where("decision_id = ?", request.DecisionID).Pluck("request_fingerprint", &stored).Error; err != nil {
				t.Fatal(err)
			}
			if stored != fingerprint {
				t.Fatal("bootstrap reescreveu receipt")
			}
		})
	}
}
