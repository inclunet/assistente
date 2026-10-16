package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/commandcli"

	"github.com/google/uuid"
)

type mockCommandCLI struct {
	listResult      []commandcli.Description
	listErr         error
	listLocale      string
	describeResult  commandcli.Description
	describeErr     error
	describeID      string
	describeLocale  string
	executeResult   commandcli.Result
	executeErr      error
	executeRequest  commandcli.Request
	retryResult     commandcli.Result
	retryErr        error
	retryRequest    commandcli.Request
	statusResult    commandcli.Result
	statusErr       error
	statusRequestID string
}

func (m *mockCommandCLI) List(_ context.Context, locale string) ([]commandcli.Description, error) {
	m.listLocale = locale
	return m.listResult, m.listErr
}

func (m *mockCommandCLI) Describe(_ context.Context, id, locale string) (commandcli.Description, error) {
	m.describeID = id
	m.describeLocale = locale
	return m.describeResult, m.describeErr
}

func (m *mockCommandCLI) Execute(_ context.Context, request commandcli.Request) (commandcli.Result, error) {
	m.executeRequest = request
	return m.executeResult, m.executeErr
}

func (m *mockCommandCLI) Retry(_ context.Context, request commandcli.Request) (commandcli.Result, error) {
	m.retryRequest = request
	return m.retryResult, m.retryErr
}

func (m *mockCommandCLI) Status(_ context.Context, requestID string) (commandcli.Result, error) {
	m.statusRequestID = requestID
	return m.statusResult, m.statusErr
}

func TestRunCommandsListEmitsJSONInDefaultLocale(t *testing.T) {
	backend := &mockCommandCLI{listResult: []commandcli.Description{{
		ID:              "workspace.list",
		Name:            "Listar workspace",
		Executable:      true,
		ArgumentsSchema: map[string]any{"type": "object"},
	}}}
	var out bytes.Buffer

	if err := runCommandsList(context.Background(), backend, &out); err != nil {
		t.Fatalf("runCommandsList: %v", err)
	}
	var got []commandcli.Description
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("saída não é JSON: %v; saída=%q", err, out.String())
	}
	if backend.listLocale != commandCLILocale || len(got) != 1 || got[0].ID != "workspace.list" {
		t.Fatalf("locale/lista inesperados: locale=%q got=%+v", backend.listLocale, got)
	}
}

func TestRunCommandsDescribeEmitsJSON(t *testing.T) {
	backend := &mockCommandCLI{describeResult: commandcli.Description{ID: "chat.model.select", Name: "Selecionar modelo"}}
	var out bytes.Buffer

	if err := runCommandsDescribe(context.Background(), backend, &out, "chat.model.select"); err != nil {
		t.Fatalf("runCommandsDescribe: %v", err)
	}
	var got commandcli.Description
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("saída não é JSON: %v", err)
	}
	if backend.describeID != "chat.model.select" || backend.describeLocale != commandCLILocale || got.ID != "chat.model.select" {
		t.Fatalf("describe inesperado: id=%q locale=%q got=%+v", backend.describeID, backend.describeLocale, got)
	}
}

func TestRunCommandsExecuteDoesNotSendRequestID(t *testing.T) {
	backend := &mockCommandCLI{executeResult: commandcli.Result{RequestID: "01900000-0000-7000-8000-000000000001", Status: "queued"}}
	arguments := json.RawMessage(`{"tab":"next"}`)
	var out bytes.Buffer

	if err := runCommandsExecute(context.Background(), backend, &out, "workspace.tab.next", arguments); err != nil {
		t.Fatalf("runCommandsExecute: %v", err)
	}
	if backend.executeRequest.RequestID != "" || backend.executeRequest.CommandID != "workspace.tab.next" || string(backend.executeRequest.Arguments) != string(arguments) {
		t.Fatalf("request inesperado: %+v", backend.executeRequest)
	}
	var got commandcli.Result
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.Status != "queued" {
		t.Fatalf("resultado inesperado: got=%+v err=%v", got, err)
	}
}

func TestRunCommandsRetryRequiresUUIDv7AndPreservesResultOnBackendError(t *testing.T) {
	backendErr := errors.New("comando indisponível")
	backend := &mockCommandCLI{
		retryResult: commandcli.Result{RequestID: "01900000-0000-7000-8000-000000000002", Status: "failed"},
		retryErr:    backendErr,
	}
	arguments := json.RawMessage(`{"force":true}`)

	if err := runCommandsRetry(context.Background(), backend, &bytes.Buffer{}, "job.run", "not-a-uuid", arguments); err == nil {
		t.Fatal("request-id inválido deveria ser rejeitado")
	}
	if backend.retryRequest.RequestID != "" {
		t.Fatal("backend não deveria ser chamado para UUID inválido")
	}

	requestID := uuid.Must(uuid.NewV7()).String()
	var out bytes.Buffer
	err := runCommandsRetry(context.Background(), backend, &out, "job.run", requestID, arguments)
	if !errors.Is(err, backendErr) {
		t.Fatalf("erro do backend não preservado: %v", err)
	}
	if backend.retryRequest.RequestID != requestID || string(backend.retryRequest.Arguments) != string(arguments) {
		t.Fatalf("retry request inesperado: %+v", backend.retryRequest)
	}
	var got commandcli.Result
	if decodeErr := json.Unmarshal(out.Bytes(), &got); decodeErr != nil || got.Status != "failed" || got.RequestID == "" {
		t.Fatalf("resultado de falha não foi emitido: got=%+v err=%v saída=%q", got, decodeErr, out.String())
	}
}

func TestRunCommandsStatusRequiresUUIDv7(t *testing.T) {
	backend := &mockCommandCLI{statusResult: commandcli.Result{Status: "running"}}
	if err := runCommandsStatus(context.Background(), backend, &bytes.Buffer{}, "1"); err == nil {
		t.Fatal("status deveria rejeitar request-id inválido")
	}

	requestID := uuid.Must(uuid.NewV7()).String()
	var out bytes.Buffer
	if err := runCommandsStatus(context.Background(), backend, &out, requestID); err != nil {
		t.Fatalf("runCommandsStatus: %v", err)
	}
	if backend.statusRequestID != requestID {
		t.Fatalf("status request-id=%q, want %q", backend.statusRequestID, requestID)
	}
}

func TestParseCommandArgumentsRejectsMissingOrInvalidJSON(t *testing.T) {
	for _, value := range []string{"", "{"} {
		if _, err := parseCommandArguments(value); err == nil {
			t.Fatalf("JSON inválido %q foi aceito", value)
		}
	}
	got, err := parseCommandArguments(` {"ok":true} `)
	if err != nil || string(got) != `{"ok":true}` {
		t.Fatalf("JSON válido inesperado: %q, err=%v", got, err)
	}
}

func TestCommandsListAndStatusRejectPositionalArguments(t *testing.T) {
	if err := commandsListCmd.Args(commandsListCmd, []string{"extra"}); err == nil {
		t.Fatal("commands list deveria exigir Args NoArgs")
	}
	if err := commandsStatusCmd.Args(commandsStatusCmd, []string{"extra"}); err == nil {
		t.Fatal("commands status deveria exigir Args NoArgs")
	}
}
