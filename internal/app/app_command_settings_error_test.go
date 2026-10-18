package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
)

func TestSafeCommandSettingsErrorPreservesSQLiteBusyAsTransient(t *testing.T) {
	err := safeCommandSettingsError(errors.New("command_decision.reserve: database is locked"))
	if err == nil || errors.Is(err, commandexecution.ErrInvalidConfiguration) {
		t.Fatalf("SQLite busy was mapped as invalid configuration: %v", err)
	}
	if !errors.Is(err, errCommandSettingsStorageBusy) {
		t.Fatalf("SQLite busy lost its transient classification: %v", err)
	}
	if got := safeCommandSettingsError(err); !errors.Is(got, errCommandSettingsStorageBusy) || errors.Is(got, commandexecution.ErrInvalidConfiguration) {
		t.Fatalf("repeated safe mapping lost transient classification: %v", got)
	}
}

func TestSafeCommandSettingsErrorKeepsInvalidConfigurationDistinct(t *testing.T) {
	err := safeCommandSettingsError(commandexecution.ErrInvalidConfiguration)
	if !errors.Is(err, commandexecution.ErrInvalidConfiguration) || errors.Is(err, errCommandSettingsStorageBusy) {
		t.Fatalf("invalid configuration classification changed: %v", err)
	}
}

func TestCommandSettingsMutationFailureLogOmitsPayloadAndFlags(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	const secret = "private-trigger-payload-DO-NOT-LOG"
	err := errors.New("command_decision.reserve: database is locked; SQL payload=" + secret)
	logCommandSettingsMutationFailure(context.Background(), commandconfig.BindingCreate, "apply", err)
	line := output.String()
	for _, required := range []string{"binding_create", "phase=apply", "sqlite_busy_or_locked"} {
		if !strings.Contains(line, required) {
			t.Errorf("log missing %q: %s", required, line)
		}
	}
	if strings.Contains(line, secret) || strings.Contains(line, "SQL payload") || strings.Contains(line, "flags") {
		t.Fatalf("unsafe command data appeared in log: %s", line)
	}
}

func TestCommandSettingsExpectedValidationErrorIsNotLogged(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	logCommandSettingsMutationFailure(context.Background(), commandconfig.BindingCreate, "apply", commandexecution.ErrInvalidConfiguration)
	if output.Len() != 0 {
		t.Fatalf("expected validation error generated noisy log: %s", output.String())
	}
}

func TestMutateCommandSettingsSQLiteBusyLogsSafeOperationAndPhase(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
	a.ctx = ctx
	defer cancel()
	snapshot, err := a.GetCommandSettingsForScope("pt-BR", string(CommandSettingsScopeGlobal))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB().DB()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		_ = conn.Close()
		t.Fatalf("acquire disposable SQLite writer lock: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		_ = conn.Close()
	})

	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	const payload = "private-settings-payload-must-not-appear"
	result, err := a.MutateCommandSettings(CommandSettingsMutationRequest{
		Locale: "pt-BR", Scope: CommandSettingsScopeGlobal, Operation: string(commandconfig.LayerCreate),
		ExpectedRevision: snapshot.Revision, ExpectedFingerprint: snapshot.Fingerprint,
		Layer: &CommandSettingsLayerInput{Name: payload, Enabled: true},
	})
	if err == nil || errors.Is(err, commandexecution.ErrInvalidConfiguration) || !errors.Is(err, errCommandSettingsStorageBusy) {
		t.Fatalf("MutateCommandSettings did not preserve transient busy: result=%+v err=%v", result, err)
	}
	line := output.String()
	for _, required := range []string{"\"component\":\"command.settings\"", "operation=layer_create", "phase=apply", "sqlite_busy_or_locked"} {
		if !strings.Contains(line, required) {
			t.Errorf("diagnostic missing %q: %s", required, line)
		}
	}
	if strings.Contains(line, payload) {
		t.Fatalf("request payload leaked into diagnostic: %s", line)
	}
}

func TestMutateCommandSettingsEarlyPrepareFailureUsesFixedOperationLabel(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	a.commandProduct.Store(nil)
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	const untrustedOperation = "unvalidated-operation-secret"
	result, err := a.MutateCommandSettings(CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: untrustedOperation,
		ExpectedRevision: 1, ExpectedFingerprint: "valid-shaped-fingerprint",
	})
	if err == nil || result.Committed {
		t.Fatalf("expected early prepare failure, got result=%+v err=%v", result, err)
	}
	line := output.String()
	for _, required := range []string{"settings_mutation", "phase=prepare"} {
		if !strings.Contains(line, required) {
			t.Errorf("early diagnostic missing %q: %s", required, line)
		}
	}
	if strings.Contains(line, untrustedOperation) {
		t.Fatalf("unvalidated operation leaked into diagnostic: %s", line)
	}
}

func TestCommandSettingsTimeoutAndPublicationFailuresAreAlwaysLogged(t *testing.T) {
	for _, tc := range []struct {
		phase string
		err   error
		label string
	}{
		{"prepare_inputs", context.DeadlineExceeded, "deadline_exceeded"},
		{"post_commit_publish", context.Canceled, "canceled"},
		{"post_commit_publish", commandexecution.ErrStale, "stale"},
	} {
		t.Run(tc.phase+tc.label, func(t *testing.T) {
			var output bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })
			logCommandSettingsMutationFailure(context.Background(), commandconfig.BindingCreate, tc.phase, tc.err)
			if !strings.Contains(output.String(), tc.phase) || !strings.Contains(output.String(), tc.label) {
				t.Fatalf("failure hidden: %s", output.String())
			}
		})
	}
}
