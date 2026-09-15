package commandtoolbridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"

	"github.com/google/uuid"
)

const (
	bridgeCommandID = "workspace.file.inspect"
	bridgeToolName  = "file_inspect"
	bridgeCatalogID = "catalog-file-inspect"
)

type bridgeRepository struct {
	mu       sync.Mutex
	inv      toolinvocations.Invocation
	visible  bool
	resolved string
}

func (r *bridgeRepository) Create(ctx context.Context, inv *toolinvocations.Invocation) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if inv.ID == "" {
		id, _ := uuid.NewV7()
		inv.ID = id.String()
	}
	inv.UserID, _ = database.UserIDFromContext(ctx)
	r.inv = *inv
	return nil
}

func (r *bridgeRepository) MarkRunning(_ context.Context, id string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inv.ID != id {
		return errors.New("invocation not found")
	}
	r.inv.Status = toolinvocations.StatusRunning
	return nil
}

func (r *bridgeRepository) Complete(_ context.Context, id string, inv *toolinvocations.Invocation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inv.ID != id {
		return errors.New("invocation not found")
	}
	r.inv = *inv
	return nil
}

func (r *bridgeRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inv.ID != id {
		return errors.New("invocation not found")
	}
	r.inv = toolinvocations.Invocation{}
	return nil
}

func (r *bridgeRepository) Get(_ context.Context, _ string) (*toolinvocations.Invocation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := r.inv
	return &copy, nil
}

func (r *bridgeRepository) List(context.Context, toolinvocations.Filter) ([]toolinvocations.Invocation, error) {
	return nil, nil
}

func (r *bridgeRepository) CleanOldDryRuns(context.Context, time.Duration) (int, error) {
	return 0, nil
}
func (r *bridgeRepository) CleanOldChat(context.Context, time.Duration) (int, error) { return 0, nil }
func (r *bridgeRepository) CleanOrphanChat(context.Context) (int, error)             { return 0, nil }
func (r *bridgeRepository) ValidateChatOrigin(context.Context, string) error         { return nil }

func (r *bridgeRepository) ResolveToolCatalogID(context.Context, string) (string, error) {
	return r.resolved, nil
}

func (r *bridgeRepository) IsToolCatalogIDVisible(context.Context, string) (bool, error) {
	return r.visible, nil
}

type bridgeTool struct {
	started  chan struct{}
	release  chan struct{}
	received chan json.RawMessage
	content  string
}

func (t *bridgeTool) Name() string        { return bridgeToolName }
func (t *bridgeTool) Description() string { return "test tool" }
func (t *bridgeTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t *bridgeTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	t.received <- append(json.RawMessage(nil), args...)
	select {
	case <-t.started:
	default:
		close(t.started)
	}
	select {
	case <-t.release:
		content := t.content
		if content == "" {
			content = `{"visible":"ok","secret":"output-secret"}`
		}
		return tools.ToolResult{
			Content:  content,
			Metadata: map[string]any{"resource": "output-secret"},
			Annotations: &tools.ResultAnnotations{HTTPResponse: &tools.HTTPResponseAnnotation{
				URL: "https://example.invalid/output-secret",
			}},
		}, nil
	case <-ctx.Done():
		return tools.ToolResult{Content: "cancelled", IsError: true}, ctx.Err()
	}
}

func newBridgeFixture(t *testing.T) (*Bridge, *bridgeRepository, *bridgeTool, string, string) {
	t.Helper()
	registry := tools.NewRegistry()
	tool := &bridgeTool{started: make(chan struct{}), release: make(chan struct{}), received: make(chan json.RawMessage, 1)}
	if err := registry.Register(tool); err != nil {
		t.Fatal(err)
	}
	repo := &bridgeRepository{visible: true, resolved: bridgeCatalogID}
	service := toolinvocations.NewService(repo, tools.NewExecutor(registry, tools.ExecutorConfig{ToolTimeout: time.Second, MaxResultSize: 1024}))
	bridge, err := New(Config{Service: service, Routes: []Route{{
		CommandID: bridgeCommandID, ToolName: bridgeToolName, ToolCatalogID: bridgeCatalogID,
		SensitivePaths: commandcatalog.SensitivePaths{Input: []string{"/secret"}, Output: []string{"/secret"}},
		Contract:       commandcatalog.HandlerContract{Effect: commandcatalog.Read, Classification: commandcatalog.HandlerTool},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	userID, _ := uuid.NewV7()
	invocationID, _ := uuid.NewV7()
	return bridge, repo, tool, userID.String(), invocationID.String()
}

func commandInvocation(userID, invocationID string) commandexecution.Invocation {
	commandID := bridgeCommandID
	envelopeUserID := userID
	args := json.RawMessage(`{"path":"/tmp/demo","secret":"input-secret"}`)
	return commandexecution.Invocation{
		ID: invocationID, CommandID: bridgeCommandID,
		Principal: auth.LocalSessionPrincipal{UserID: userID},
		Source:    commandcatalog.Palette,
		Envelope:  &commandcontract.Envelope{InvocationID: invocationID, UserID: &envelopeUserID, CommandID: &commandID, Arguments: &args},
	}
}

func TestBridgeDelegatesThroughCommonServiceAndRedactsBothDirections(t *testing.T) {
	bridge, repo, tool, userID, invocationID := newBridgeFixture(t)
	handler, ok := bridge.Handler(bridgeCommandID)
	if !ok {
		t.Fatal("handler não encontrado pelo ID canônico")
	}

	started := time.Now()
	handle, err := handler.Start(context.Background(), commandInvocation(userID, invocationID))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("Start bloqueou por %s", elapsed)
	}
	select {
	case <-tool.started:
	case <-time.After(time.Second):
		t.Fatal("a tool comum não foi iniciada")
	}
	select {
	case received := <-tool.received:
		if !strings.Contains(string(received), "input-secret") {
			t.Fatalf("a execução não recebeu os argumentos originais: %s", received)
		}
	case <-time.After(time.Second):
		t.Fatal("argumentos não chegaram à tool comum")
	}
	select {
	case <-handle.Done:
		t.Fatal("Done foi concluído antes do handoff terminar")
	case <-time.After(20 * time.Millisecond):
	}
	close(tool.release)
	select {
	case outcome := <-handle.Done:
		if outcome.Status != commandledger.Succeeded {
			t.Fatalf("status = %s, want succeeded", outcome.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("Done não recebeu outcome")
	}

	repo.mu.Lock()
	persisted := repo.inv
	repo.mu.Unlock()
	if persisted.OriginType != toolinvocations.OriginCommandInvocation || persisted.OriginID != invocationID {
		t.Fatalf("origem = %q/%q", persisted.OriginType, persisted.OriginID)
	}
	if strings.Contains(string(persisted.Input), "input-secret") {
		t.Fatalf("input sensível persistido: %s", persisted.Input)
	}
	var input struct {
		ToolCall tools.ToolCall `json:"tool_call"`
	}
	if err := json.Unmarshal(persisted.Input, &input); err != nil {
		t.Fatal(err)
	}
	var inputArgs map[string]any
	if err := json.Unmarshal([]byte(input.ToolCall.Function.Arguments), &inputArgs); err != nil {
		t.Fatal(err)
	}
	if inputArgs["secret"] != "[redacted]" {
		t.Fatalf("secret de input = %#v", inputArgs["secret"])
	}
	var output struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(persisted.Output, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.Content, "output-secret") {
		t.Fatalf("output sensível persistido: %s", persisted.Output)
	}
	var outputJSON map[string]any
	if err := json.Unmarshal([]byte(output.Content), &outputJSON); err != nil {
		t.Fatal(err)
	}
	if outputJSON["secret"] != "[redacted]" {
		t.Fatalf("secret de output = %#v", outputJSON["secret"])
	}
}

func TestBridgeRejectsOwnerlessAndSystemDelegation(t *testing.T) {
	bridge, _, _, userID, invocationID := newBridgeFixture(t)
	handler, ok := bridge.Handler(bridgeCommandID)
	if !ok {
		t.Fatal("handler ausente")
	}
	ownerless := commandInvocation("", invocationID)
	if _, err := handler.Start(context.Background(), ownerless); !errors.Is(err, ErrOwnerRequired) {
		t.Fatalf("ownerless err = %v", err)
	}
	system := commandInvocation(userID, invocationID)
	system.Source = commandcatalog.System
	if _, err := handler.Start(context.Background(), system); !errors.Is(err, ErrSystemDelegation) {
		t.Fatalf("system err = %v", err)
	}
	system = commandInvocation(userID, invocationID)
	system.Envelope.AuthContextType = commandcontract.AuthSystem
	system.Source = commandcatalog.UI
	if _, err := handler.Start(context.Background(), system); !errors.Is(err, ErrSystemDelegation) {
		t.Fatalf("system auth context err = %v", err)
	}
}

func TestBridgeRejectsCanonicalCatalogMismatchWithoutRunningTool(t *testing.T) {
	bridge, _, tool, userID, invocationID := newBridgeFixture(t)
	// A rota exige o ID canônico, enquanto o catálogo resolve o nome para outro.
	registry := tools.NewRegistry()
	if err := registry.Register(tool); err != nil {
		t.Fatal(err)
	}
	repo := &bridgeRepository{visible: true, resolved: "another-catalog"}
	service := toolinvocations.NewService(repo, tools.NewExecutor(registry, tools.ExecutorConfig{ToolTimeout: time.Second, MaxResultSize: 1024}))
	bridge, err := New(Config{Service: service, Routes: []Route{{
		CommandID: bridgeCommandID, ToolName: bridgeToolName, ToolCatalogID: bridgeCatalogID,
		Contract: commandcatalog.HandlerContract{Effect: commandcatalog.Read, Classification: commandcatalog.HandlerTool},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	handler, _ := bridge.Handler(bridgeCommandID)
	handle, err := handler.Start(context.Background(), commandInvocation(userID, invocationID))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case outcome := <-handle.Done:
		if outcome.Status != commandledger.Failed {
			t.Fatalf("status = %s, want failed", outcome.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("resultado não recebido")
	}
	select {
	case <-tool.started:
		t.Fatal("tool foi executada após divergência de catálogo")
	default:
	}
}

func TestBridgeCancellationWithoutZeroEffectProofIsUnknown(t *testing.T) {
	bridge, _, tool, userID, invocationID := newBridgeFixture(t)
	handler, _ := bridge.Handler(bridgeCommandID)
	handle, err := handler.Start(context.Background(), commandInvocation(userID, invocationID))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-tool.started:
	case <-time.After(time.Second):
		t.Fatal("tool não iniciou")
	}
	handle.Cancel()
	select {
	case outcome := <-handle.Done:
		if outcome.Status != commandledger.OutcomeUnknown {
			t.Fatalf("cancelled status = %s, want outcome_unknown", outcome.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelamento não concluiu")
	}
}

func TestBridgeNonJSONResultAfterExecutionIsUnknown(t *testing.T) {
	bridge, _, tool, userID, invocationID := newBridgeFixture(t)
	tool.content = "efeito aplicado; resposta textual"
	handler, _ := bridge.Handler(bridgeCommandID)
	handle, err := handler.Start(context.Background(), commandInvocation(userID, invocationID))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-tool.started:
	case <-time.After(time.Second):
		t.Fatal("tool não iniciou")
	}
	close(tool.release)
	select {
	case outcome := <-handle.Done:
		if outcome.Status != commandledger.OutcomeUnknown {
			t.Fatalf("non-JSON status = %s, want outcome_unknown", outcome.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("resultado não recebido")
	}
}

func TestBridgeDefinitionRequiresExplicitCompleteContractAndUnionsSensitivePaths(t *testing.T) {
	definition := completeBridgeDefinition()
	if _, err := New(Config{Service: newBridgeService(t), Routes: []Route{{
		CommandID: bridgeCommandID, ToolName: bridgeToolName, ToolCatalogID: bridgeCatalogID,
		Definition: definition,
	}}}); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("contrato ausente err = %v", err)
	}
	bridge, err := New(Config{Service: newBridgeService(t), Routes: []Route{{
		CommandID: bridgeCommandID, ToolName: bridgeToolName, ToolCatalogID: bridgeCatalogID,
		Definition: definition,
		Contract: commandcatalog.HandlerContract{
			Effect: commandcatalog.Read, Route: "tools/file_inspect", Classification: commandcatalog.HandlerTool,
		},
	}}})
	if err != nil {
		t.Fatalf("contrato completo: %v", err)
	}
	if got := bridge.routes[bridgeCommandID].SensitivePaths.Input; len(got) != 1 || got[0] != "/secret" {
		t.Fatalf("paths de input = %#v, want definição propagada", got)
	}
	if got := bridge.routes[bridgeCommandID].SensitivePaths.Output; len(got) != 1 || got[0] != "/secret" {
		t.Fatalf("paths de output = %#v, want definição propagada", got)
	}

	badContract := commandcatalog.HandlerContract{Effect: commandcatalog.Write, Route: "tools/file_inspect", Classification: commandcatalog.HandlerTool}
	if _, err := New(Config{Service: newBridgeService(t), Routes: []Route{{
		CommandID: bridgeCommandID, ToolName: bridgeToolName, ToolCatalogID: bridgeCatalogID,
		Definition: definition, Contract: badContract,
	}}}); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("contrato divergente err = %v", err)
	}
}

func newBridgeService(t *testing.T) *toolinvocations.Service {
	t.Helper()
	registry := tools.NewRegistry()
	if err := registry.Register(&bridgeTool{started: make(chan struct{}), release: make(chan struct{}), received: make(chan json.RawMessage, 1)}); err != nil {
		t.Fatal(err)
	}
	return toolinvocations.NewService(&bridgeRepository{visible: true, resolved: bridgeCatalogID}, tools.NewExecutor(registry, tools.ExecutorConfig{ToolTimeout: time.Second, MaxResultSize: 1024}))
}

func completeBridgeDefinition() commandcatalog.Definition {
	stringSchema := commandcatalog.Schema{Type: commandcatalog.SchemaString}
	return commandcatalog.Definition{
		ID: bridgeCommandID, Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
		AllowedSources: []commandcatalog.Source{commandcatalog.Palette},
		Context:        commandcatalog.ContextPolicy{None: true},
		ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{
			"path": stringSchema, "secret": stringSchema,
		}, Required: []string{"path", "secret"}},
		ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{
			"visible": stringSchema, "secret": stringSchema,
		}, Required: []string{"visible", "secret"}},
		Risk:           commandcatalog.RiskLow,
		SensitivePaths: commandcatalog.SensitivePaths{Input: []string{"/secret"}, Output: []string{"/secret"}},
		Presentation: &commandcatalog.Presentation{Version: "1", Locales: map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: "Inspecionar arquivo", Description: "Inspeciona um arquivo", Category: "workspace"},
			"en":    {Name: "Inspect file", Description: "Inspects a file", Category: "workspace"},
			"es":    {Name: "Inspeccionar archivo", Description: "Inspecciona un archivo", Category: "workspace"},
		}},
		Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceRedacted, Audit: commandcatalog.PersistenceRedacted},
		Scopes:      []commandcatalog.Scope{commandcatalog.ScopeSession}, Availability: commandcatalog.Availability{Status: commandcatalog.Available},
		HandlerRoute: "tools/file_inspect", HandlerClassification: commandcatalog.HandlerTool,
	}
}
