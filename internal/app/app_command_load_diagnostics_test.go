package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandautomation"
	"assistente/internal/commandexecution"
	"assistente/internal/commandjobactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/logging"
)

type commandLoadLogBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *commandLoadLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}
func (b *commandLoadLogBuffer) text() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.String()
}
func captureCommandLoadLogs(t *testing.T) *commandLoadLogBuffer {
	t.Helper()
	b := &commandLoadLogBuffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(b, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return b
}
func commandLoadRecords(t *testing.T, b *commandLoadLogBuffer) []map[string]any {
	t.Helper()
	var result []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(b.text()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record["component"] == "app.command-load" {
			result = append(result, record)
		}
	}
	return result
}

type commandLoadCodedError struct{}

func (commandLoadCodedError) Error() string { return "SECRET SQL arguments and user data" }
func (commandLoadCodedError) Code() int     { return 517 }

func TestCommandLoadDiagnosticsDefaultAndNoSensitiveData(t *testing.T) {
	b := captureCommandLoadLogs(t)
	ctx := logging.WithAttrs(context.Background(), slog.String("user_id", "SECRET"), slog.String("session_id", "SECRET"))
	ctx, trace := beginCommandLoad(ctx, "settings_load")
	defer trace.finish(nil)
	commandLoadStage(ctx, "store_load")
	if len(commandLoadRecords(t, b)) != 0 {
		t.Fatal("início e etapas não devem gerar ruído")
	}
	commandLoadCause(ctx, fmt.Errorf("SECRET wrapped: %w", commandLoadCodedError{}))
	trace.finish(commandexecution.ErrDenied)
	records := commandLoadRecords(t, b)
	if len(records) != 1 {
		t.Fatalf("esperado um resultado sem opt-in, obtido %d", len(records))
	}
	last := records[len(records)-1]
	if last["status"] != "failed" || last["stage"] != "store_load" || last["error_class"] != "sqlite_busy" || last["error_code"] != float64(517) {
		t.Fatalf("causa perdida: %+v", last)
	}
	if strings.Contains(b.text(), "SECRET") {
		t.Fatal("diagnóstico vazou conteúdo/identidade")
	}
	for _, record := range records {
		if record["load_id"] != last["load_id"] {
			t.Fatal("correlação perdida")
		}
	}
}

func TestCommandLoadDiagnosticsSlowAndFinishAreBounded(t *testing.T) {
	b := captureCommandLoadLogs(t)
	_, trace := beginCommandLoad(context.Background(), "keyboard_map_load")
	defer trace.finish(nil)
	trace.stage("job_projection")
	trace.slow()
	trace.finish(nil, slog.Int("bindings", 3))
	n := len(commandLoadRecords(t, b))
	trace.slow()
	trace.finish(errors.New("late"))
	trace.stage("late")
	if len(commandLoadRecords(t, b)) != n {
		t.Fatal("log após conclusão")
	}
	records := commandLoadRecords(t, b)
	if records[n-2]["status"] != "slow" || records[n-2]["stage"] != "job_projection" || records[n-1]["status"] != "succeeded" || records[n-1]["bindings"] != float64(3) {
		t.Fatalf("eventos inválidos: %+v", records)
	}
}

func TestCommandLoadDiagnosticsEndpointsPreserveOriginalFailure(t *testing.T) {
	b := captureCommandLoadLogs(t)
	a := &App{}
	if _, err := a.GetCommandSettingsForScope("pt-BR", "global"); !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
		t.Fatalf("contrato settings alterado: %v", err)
	}
	if _, err := a.GetLocalCommandKeyboardMap(); !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
		t.Fatalf("contrato teclado alterado: %v", err)
	}
	failed := map[string]bool{}
	for _, r := range commandLoadRecords(t, b) {
		if r["status"] == "failed" {
			if r["error_class"] != "not_ready" {
				t.Fatalf("erro original perdido: %+v", r)
			}
			failed[r["operation"].(string)] = true
		}
	}
	if !failed["settings_load"] || !failed["keyboard_map_load"] {
		t.Fatalf("endpoints sem diagnóstico: %+v", failed)
	}
}

func TestCommandLoadDiagnosticsSuccessfulReads(t *testing.T) {
	a := commandJobPublicationApp(t)
	b := captureCommandLoadLogs(t)
	if _, err := a.GetCommandSettingsForScope("pt-BR", "global"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatal(err)
	}
	passed := map[string]bool{}
	for _, r := range commandLoadRecords(t, b) {
		if r["status"] == "succeeded" {
			passed[r["operation"].(string)] = true
		}
	}
	if !passed["settings_load"] || !passed["keyboard_map_load"] {
		t.Fatalf("leituras não registradas: %+v", passed)
	}
}

func TestCommandLoadDiagnosticsTimerReportsWaitingStage(t *testing.T) {
	b := captureCommandLoadLogs(t)
	_, trace := beginCommandLoad(context.Background(), "settings_load")
	defer trace.finish(nil)
	trace.stage("store_load")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, r := range commandLoadRecords(t, b) {
			if r["status"] == "slow" {
				if r["stage"] != "store_load" {
					t.Fatalf("etapa errada: %+v", r)
				}
				trace.finish(context.DeadlineExceeded)
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("chamada sem conclusão não gerou aviso")
}

func TestCommandLoadDiagnosticsErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{auth.ErrSessionExpired, "session_expired"}, {auth.ErrSessionRevoked, "session_revoked"},
		{context.DeadlineExceeded, "deadline"}, {context.Canceled, "cancelled"},
		{commandexecution.ErrStale, "stale"}, {errors.New("SECRET"), "other"},
		{commandLoadCodedError{}, "sqlite_busy"},
	} {
		if got := commandLoadErrorClass(fmt.Errorf("SECRET: %w", tc.err)); got != tc.want {
			t.Fatalf("classe=%s want=%s", got, tc.want)
		}
	}
}

func TestCommandLoadDiagnosticsHostAndJobProjectionSentinels(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"invalid host state", commandexecution.ErrInvalidHostState, "invalid_host_state"},
		{"disabled host", commandexecution.ErrHostStateDisabled, "host_state_disabled"},
		{"invalid host user", commandexecution.ErrInvalidHostUser, "invalid_host_user"},
		{"invalid host principal", commandexecution.ErrInvalidHostPrincipal, "invalid_host_principal"},
		{"unpublished host user", commandexecution.ErrHostUserNotPublished, "host_user_not_published"},
		{"invalid host layers", commandexecution.ErrInvalidHostLayers, "invalid_host_layers"},
		{"host generation overflow", commandexecution.ErrHostGenerationOverflow, "host_generation_overflow"},
		{"projection base changed", commandexecution.ErrJobProjectionBaseChanged, "job_projection_base_changed"},
		{"projection unavailable", commandjobactivation.ErrUnavailable, "job_projection_unavailable"},
		{"projection schema unavailable", commandjobevents.ErrSchemaUnavailable, "job_projection_schema_unavailable"},
		{"projection bootstrap incomplete", commandjobevents.ErrBootstrapIncomplete, "job_projection_bootstrap_incomplete"},
		{"projection invalid fact", commandjobevents.ErrInvalidFact, "job_projection_invalid_fact"},
		{"projection fingerprint conflict", commandjobevents.ErrFingerprintConflict, "job_projection_fingerprint_conflict"},
		{"grant invalid", commandautomation.ErrInvalid, "job_projection_grant_invalid"},
		{"grant stale", commandautomation.ErrStale, "job_projection_grant_stale"},
		{"grant missing", commandautomation.ErrNotFound, "job_projection_grant_not_found"},
		{"grant foreign scope", commandautomation.ErrForeignScope, "job_projection_grant_foreign_scope"},
		{"grant fingerprint unavailable", commandautomation.ErrFingerprint, "job_projection_fingerprint_unavailable"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := fmt.Errorf("private detail: %w", tc.err)
			if got := commandLoadErrorClass(wrapped); got != tc.want {
				t.Fatalf("classe wrapped=%s want=%s", got, tc.want)
			}
			joined := errors.Join(errors.New("SECRET SQL and identifiers"), wrapped)
			if got := commandLoadErrorClass(joined); got != tc.want {
				t.Fatalf("classe joined=%s want=%s", got, tc.want)
			}
			if strings.Contains(commandLoadErrorClass(joined), "SECRET") {
				t.Fatal("classe de erro expôs conteúdo arbitrário")
			}
		})
	}
}
