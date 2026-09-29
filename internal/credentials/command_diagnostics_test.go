package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

type commandLogBuffer struct {
	sync.Mutex
	bytes.Buffer
}

func (b *commandLogBuffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.Buffer.Write(p)
}
func captureCommandLogs(t *testing.T) *commandLogBuffer {
	t.Helper()
	old, writer := slog.Default(), log.Writer()
	output := &commandLogBuffer{}
	slog.SetDefault(slog.New(slog.NewJSONHandler(output, nil)))
	t.Cleanup(func() { slog.SetDefault(old); log.SetOutput(writer) })
	return output
}
func commandLogRecords(t *testing.T, output *commandLogBuffer) []map[string]any {
	t.Helper()
	output.Lock()
	defer output.Unlock()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record["msg"] == "credential_command_execution" {
			records = append(records, record)
		}
	}
	return records
}

func TestCommandDiagnosticsCountExecutionsNotRequests(t *testing.T) {
	f := newCommandCacheFixture(t)
	if err := f.m.RegisterStoredCredentialWithContext(f.ctx, StoredCredential{ID: "credential-1", Pattern: "cache.example", Auth: f.config}); err != nil {
		t.Fatal(err)
	}
	output := captureCommandLogs(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); f.get(t) }()
	}
	wg.Wait()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer first" {
			f.setValue(t, "second")
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	client := NewHTTPClient(f.m, "cache.example", 0)
	defer client.CloseIdleConnections()
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequestWithContext(f.ctx, "GET", server.URL, nil)
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatal(response.StatusCode)
		}
	}
	records := commandLogRecords(t, output)
	if len(records) != 2 || f.count() != 2 {
		t.Fatalf("logs=%d comandos=%d", len(records), f.count())
	}
	for i, reason := range []string{"initial", "http_401"} {
		record := records[i]
		if record["reason"] != reason || record["outcome"] != "success" || record["component"] != "credentials.command" || record["user_id"] != "cache-user" || record["credential_id"] != "credential-1" {
			t.Fatalf("metadados inesperados: %+v", record)
		}
		if record["cache_ref"] == "" || record["cache_ref"] != records[0]["cache_ref"] || record["cache_generation"] != float64(i) {
			t.Fatalf("correlação inválida: %+v", record)
		}
		if duration, ok := record["duration_ms"].(float64); !ok || duration < 0 {
			t.Fatal("duração ausente")
		}
	}
	for _, forbidden := range []string{"Bearer first", "Bearer second", "cache.example", f.config.SourceConfig.Command, "-test.run", f.value} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatal("configuração ou material secreto no log")
		}
	}
	// Edição substitui a entrada e recebe outra referência, com resolução inicial.
	if err := f.m.RegisterPatternWithContext(f.ctx, "cache.example", f.config); err != nil {
		t.Fatal(err)
	}
	f.get(t)
	records = commandLogRecords(t, output)
	if len(records) != 3 || records[2]["reason"] != "initial" || records[2]["cache_ref"] == records[0]["cache_ref"] {
		t.Fatal("edição confundida com renovação HTTP")
	}
}

func TestCommandDiagnosticsDirectOutcomesAndRedaction(t *testing.T) {
	for _, tc := range []struct{ mode, outcome string }{{"success", "success"}, {"fail", "failure"}, {"sleep", "timeout"}, {"empty", "failure"}} {
		t.Run(tc.mode, func(t *testing.T) {
			t.Setenv("ASSISTENTE_CREDENTIAL_TEST_HELPER", "1")
			output := captureCommandLogs(t)
			config := commandTestConfig(t, tc.mode)
			_, _ = ResolveSource(context.Background(), &AuthConfig{Source: "command", Type: "bearer", SourceConfig: config})
			records := commandLogRecords(t, output)
			if len(records) != 1 || records[0]["outcome"] != tc.outcome || records[0]["reason"] != "direct" {
				t.Fatalf("registro inesperado: %+v", records)
			}
			for _, secret := range []string{"token-value", "SECRET-STDERR", "SECRET-STDOUT", config.Command, "-test.run"} {
				if strings.Contains(output.String(), secret) {
					t.Fatal("segredo/configuração no diagnóstico")
				}
			}
		})
	}
}

func TestCommandDiagnosticsCancellation(t *testing.T) {
	f := newCommandCacheFixture(t)
	output := captureCommandLogs(t)
	if err := os.WriteFile(f.hold, []byte("hold"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := f.m.getByPatternWithContext(ctx, "cache.example", true); done <- err }()
	waitCacheCalls(t, f, 1)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelamento perdido: %v", err)
	}
	records := commandLogRecords(t, output)
	if len(records) != 1 || records[0]["outcome"] != "canceled" {
		t.Fatalf("cancelamento sem diagnóstico: %+v", records)
	}
}
