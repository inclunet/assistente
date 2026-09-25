package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandcontract"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"github.com/google/uuid"
)

type paletteCredentialEchoTool struct{ calls atomic.Int32 }

func (*paletteCredentialEchoTool) Name() string        { return "test.palette_credential_echo" }
func (*paletteCredentialEchoTool) Description() string { return "fixture" }
func (*paletteCredentialEchoTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"secret":{"type":"string"}},"required":["secret"]}`)
}
func (t *paletteCredentialEchoTool) Execute(_ context.Context, args json.RawMessage) (tools.ToolResult, error) {
	t.calls.Add(1)
	return tools.ToolResult{Content: string(args)}, nil
}

func TestCommandToolPublicPaletteExecutionDecisionAndOutputRedaction(t *testing.T) {
	for _, scenario := range []string{"confirmed", "cancelled", "schema_drift"} {
		t.Run(scenario, func(t *testing.T) {
			a := commandJobPublicationApp(t)
			ctx, cancel := context.WithTimeout(database.WithUserID(context.Background(), a.currentUserID), 20*time.Second)
			defer cancel()
			tool := &paletteCredentialEchoTool{}
			a.toolRegistry.MustRegister(tool)
			catalogID := uuid.Must(uuid.NewV7()).String()
			if err := database.DB().Create(&database.ToolCatalog{UUIDModel: database.UUIDModel{ID: catalogID}, Name: tool.Name(), DisplayName: "Fixture credential tool", Origin: "builtin", Schema: string(tool.Parameters()), AvailabilityStatus: "available"}).Error; err != nil {
				t.Fatal(err)
			}
			events := make(chan map[string]any, 2)
			a.questionnaireMgr = questionnaire.NewManager(func(event string, payload any) {
				if event == questionnaire.EventQuestionnaire {
					events <- payload.(map[string]any)
				}
			})
			if err := a.refreshCommandProductCatalog(ctx); err != nil {
				t.Fatalf("refresh catalog: %v", err)
			}
			commandID, ok := commandToolExecutionID(catalogID)
			if !ok {
				t.Fatal("command ID fixture inválido")
			}
			p := a.commandProduct.Load()
			definition, ok := p.registry.Lookup(commandID)
			if !ok || !definition.AllowsSource("palette") || definition.AllowsSource("keyboard.local") || definition.AllowsSource("streamdeck") {
				t.Fatalf("tool não está restrita à paleta ad hoc: %+v", definition.AllowedSources)
			}
			secret := "do-not-show-in-confirmation-or-result"
			invalidArgs, _ := json.Marshal(map[string]string{"unexpected": "value"})
			invalidWrapper, _ := json.Marshal(map[string]string{"arguments_json": string(invalidArgs)})
			invalid, _ := a.ExecutePaletteCommand(commandID, invalidWrapper)
			if invalid.Status != "denied" || tool.calls.Load() != 0 {
				t.Fatalf("args inválidos não falharam antes da decisão: %+v calls=%d", invalid, tool.calls.Load())
			}
			select {
			case event := <-events:
				t.Fatalf("args inválidos abriram decisão: %#v", event)
			default:
			}
			arguments, _ := json.Marshal(map[string]string{"secret": secret})
			wrapper, _ := json.Marshal(map[string]string{"arguments_json": string(arguments)})
			candidateArgs, err := definition.ValidateArguments(wrapper)
			if err != nil {
				t.Fatalf("tool args rejected by command definition: %v", err)
			}
			commandIDCopy := commandID
			candidateMessage := json.RawMessage(candidateArgs)
			if body, err := commandToolDecisionBody(a, p, definition, commandcontract.Envelope{CommandID: &commandIDCopy, Arguments: &candidateMessage}); err != nil {
				t.Fatalf("tool decision preflight: %v route=%q", err, definition.HandlerRoute)
			} else if strings.Contains(body, secret) {
				t.Fatal("decision body exposed raw arguments")
			}
			done := make(chan struct {
				result CommandExecutionResult
				err    error
			}, 1)
			go func() {
				result, err := a.ExecutePaletteCommand(commandID, wrapper)
				done <- struct {
					result CommandExecutionResult
					err    error
				}{result, err}
			}()
			var decision map[string]any
			select {
			case decision = <-events:
			case result := <-done:
				code, summary := "", ""
				if result.result.ErrorCode != nil {
					code = *result.result.ErrorCode
				}
				if result.result.ResultSummary != nil {
					summary = *result.result.ResultSummary
				}
				t.Fatalf("exec terminou antes da decisão: status=%s errorCode=%s summary=%s err=%v", result.result.Status, code, summary, result.err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			body, _ := decision["body"].(string)
			if !strings.Contains(body, tool.Name()) || !strings.Contains(body, catalogID) || !strings.Contains(body, "[redigidos]") || strings.Contains(body, secret) {
				t.Fatalf("decisão não identifica alvo fixo/redige args: %q", body)
			}
			decisionID, _ := decision["id"].(string)
			if scenario == "schema_drift" {
				if err := database.DB().Model(&database.ToolCatalog{}).Where("id = ?", catalogID).Update("schema", `{"type":"object"}`).Error; err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "cancelled" {
				if err := a.questionnaireMgr.Respond(decisionID, map[string]any{questionnaire.AnswerActionID: commanddecision.DenyAction}, true); err != nil {
					t.Fatal(err)
				}
			} else if err := a.questionnaireMgr.Respond(decisionID, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false); err != nil {
				t.Fatal(err)
			}
			var outcome struct {
				result CommandExecutionResult
				err    error
			}
			select {
			case outcome = <-done:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if scenario == "cancelled" || scenario == "schema_drift" {
				if scenario == "cancelled" && outcome.result.Status != "denied" || scenario == "schema_drift" && outcome.result.Status == "succeeded" || tool.calls.Load() != 0 {
					t.Fatalf("cancel/drift executou tool: scenario=%s status=%s calls=%d err=%v", scenario, outcome.result.Status, tool.calls.Load(), outcome.err)
				}
				return
			}
			if outcome.err != nil || outcome.result.Status != "succeeded" || tool.calls.Load() != 1 || outcome.result.Output != nil {
				t.Fatalf("resultado público após execução: %+v err=%v calls=%d", outcome.result, outcome.err, tool.calls.Load())
			}
			encoded, _ := json.Marshal(outcome.result)
			if strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), `"secret"`) {
				t.Fatalf("resultado público vazou argumentos/saída da tool: %s", encoded)
			}
			var rows []database.ToolInvocation
			if err := database.DB().Where("origin_type = ? AND origin_id = ? AND user_id = ?", toolinvocations.OriginCommandInvocation, outcome.result.InvocationID, a.currentUserID).Find(&rows).Error; err != nil || len(rows) != 1 || rows[0].Status != toolinvocations.StatusSucceeded {
				t.Fatalf("invocação real não correlacionada: rows=%+v err=%v", rows, err)
			}
			p = a.commandProduct.Load()
			persistedRecord, err := p.service.GetEnvelopeInvocation(ctx, "", outcome.result.InvocationID)
			if err != nil {
				t.Fatalf("ler auditoria persistida: %v", err)
			}
			persistedJSON, _ := json.Marshal(persistedRecord)
			if strings.Contains(string(persistedJSON), secret) {
				t.Fatalf("command_invocations persistiu argumento sensível: %s", persistedJSON)
			}
			for _, row := range rows {
				encoded, _ := json.Marshal(row)
				if strings.Contains(string(encoded), secret) {
					t.Fatalf("ToolInvocation persistiu o segredo: %s", encoded)
				}
			}
		})
	}
}

func TestCommandToolSemanticVersionChangesWithSchemaOrRegistryGeneration(t *testing.T) {
	tool := &paletteCredentialEchoTool{}
	row := database.ToolCatalog{UUIDModel: database.UUIDModel{ID: uuid.Must(uuid.NewV7()).String()}, Name: tool.Name(), Origin: "builtin", Schema: string(tool.Parameters())}
	first, err := commandToolSemanticVersion(row, tool, 1)
	if err != nil {
		t.Fatal(err)
	}
	row.Schema = `{"type":"object"}`
	if _, err := commandToolSemanticVersion(row, tool, 1); err == nil {
		t.Fatal("schema do catálogo divergente foi aceito")
	}
	row.Schema = string(tool.Parameters())
	second, err := commandToolSemanticVersion(row, tool, 2)
	if err != nil || first == second {
		t.Fatalf("generation não invalidou o contrato semântico: %s %s %v", first, second, err)
	}
}

func TestSafeCommandToolLabelRemovesUnicodeControlsAndTruncatesRunes(t *testing.T) {
	malicious := "tool\r\nALERTA\u2028\u202e" + strings.Repeat("界", 170)
	got := safeCommandToolLabel(malicious)
	if strings.ContainsAny(got, "\r\n\u2028\u2029") || strings.ContainsRune(got, '\u202e') || len([]rune(got)) > 160 || !strings.HasPrefix(got, "tool ALERTA") {
		t.Fatalf("label não sanitizado por runas: %q (%d runas)", got, len([]rune(got)))
	}
}

func TestCommandToolCatalogRefreshAddsAndRemovesFixedTargets(t *testing.T) {
	a := commandJobPublicationApp(t)
	ctx, cancel := context.WithTimeout(database.WithUserID(context.Background(), a.currentUserID), 30*time.Second)
	defer cancel()
	tool := &paletteCredentialEchoTool{}
	a.toolRegistry.MustRegister(tool)
	catalogID := uuid.Must(uuid.NewV7()).String()
	if err := database.DB().Create(&database.ToolCatalog{UUIDModel: database.UUIDModel{ID: catalogID}, Name: tool.Name(), DisplayName: "Fixture tool", Origin: "builtin", Schema: string(tool.Parameters()), AvailabilityStatus: "available"}).Error; err != nil {
		t.Fatal(err)
	}
	commandID, _ := commandToolExecutionID(catalogID)
	oldProduct := a.commandProduct.Load()
	if err := a.refreshCommandProductCatalog(ctx); err != nil {
		t.Fatalf("refresh após adicionar: %v", err)
	}
	addedProduct := a.commandProduct.Load()
	if addedProduct == nil || addedProduct == oldProduct {
		t.Fatal("refresh de adição reutilizou runtime antigo")
	}
	if _, ok := addedProduct.registry.Lookup(commandID); !ok {
		t.Fatal("tool adicionada não entrou no novo catálogo")
	}
	workspaceCandidate := commandexecution.EnvelopeCandidate{InvocationID: uuid.Must(uuid.NewV7()).String(), CorrelationID: uuid.Must(uuid.NewV7()).String(), CommandID: commandProductWorkspaceListID, Arguments: json.RawMessage(`{}`)}
	if record, err := addedProduct.execute(ctx, workspaceCandidate); err != nil || record.Status != "succeeded" {
		t.Fatalf("executor anterior não admitia candidate de paleta válido antes do refresh: status=%s err=%v", record.Status, err)
	}
	oldDefinition, _ := addedProduct.registry.Lookup(commandID)
	a.toolRegistry.Unregister(tool.Name())
	a.toolRegistry.MustRegister(tool) // Mesmo schema, nova geração de registro.
	if err := a.refreshCommandProductCatalog(ctx); err != nil {
		t.Fatalf("refresh após republicar schema/runtime: %v", err)
	}
	refreshedProduct := a.commandProduct.Load()
	newDefinition, exists := refreshedProduct.registry.Lookup(commandID)
	if !exists || refreshedProduct == addedProduct || newDefinition.HandlerRoute == oldDefinition.HandlerRoute {
		t.Fatal("novo catálogo reutilizou o contrato semântico/autorização anterior")
	}
	candidate := commandexecution.EnvelopeCandidate{InvocationID: uuid.Must(uuid.NewV7()).String(), CorrelationID: uuid.Must(uuid.NewV7()).String(), CommandID: commandID, Arguments: json.RawMessage(`{"arguments_json":"{\"secret\":\"x\"}"}`)}
	if _, err := addedProduct.execute(ctx, candidate); err == nil || tool.calls.Load() != 0 {
		t.Fatalf("executor aposentado aceitou autorização antiga: err=%v calls=%d", err, tool.calls.Load())
	}
	if err := database.DB().Model(&database.ToolCatalog{}).Where("id = ?", catalogID).Update("availability_status", "unavailable").Error; err != nil {
		t.Fatal(err)
	}
	if err := a.refreshCommandProductCatalog(ctx); err != nil {
		t.Fatalf("refresh após remover: %v", err)
	}
	removedProduct := a.commandProduct.Load()
	if removedProduct == nil || removedProduct == addedProduct {
		t.Fatal("refresh de remoção reutilizou runtime antigo")
	}
	if _, ok := removedProduct.registry.Lookup(commandID); ok {
		t.Fatal("tool removida permaneceu disponível no catálogo publicado")
	}
	args := json.RawMessage(`{"arguments_json":"{\"secret\":\"x\"}"}`)
	if result, err := a.ExecutePaletteCommand(commandID, args); err == nil && result.Status == "succeeded" || tool.calls.Load() != 0 {
		t.Fatalf("tool removida executou: %+v err=%v calls=%d", result, err, tool.calls.Load())
	}
}
