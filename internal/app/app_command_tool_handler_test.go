package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type appCommandToolHandlerTestTool struct{}

func (*appCommandToolHandlerTestTool) Name() string { return "app.command_tool_handler_test_tool" }

func (*appCommandToolHandlerTestTool) Description() string {
	return "tool controlada da factory de command tools"
}

func (*appCommandToolHandlerTestTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (*appCommandToolHandlerTestTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: `{"ok":true}`}, nil
}

func appCommandToolHandlerTestDefinition() commandcatalog.Definition {
	definition := appCommandJobHandlerDefinition()
	definition.HandlerRoute = "app/test-command-tool-handler"
	definition.HandlerClassification = commandcatalog.HandlerTool
	return definition
}

func appCommandToolHandlerTestContract(definition commandcatalog.Definition) commandcatalog.HandlerContract {
	return commandcatalog.HandlerContract{
		Effect:           commandcatalog.Destructive,
		HasMutableTarget: true,
		Route:            definition.HandlerRoute,
		Classification:   commandcatalog.HandlerTool,
	}
}

func setupAppCommandToolHandlerTest(t *testing.T, status, schema string) (*App, context.Context, commandcatalog.Definition, commandcatalog.HandlerContract, string) {
	t.Helper()
	a := commandJobPublicationApp(t)
	ctx := database.WithUserID(context.Background(), a.currentUserID)
	definition := appCommandToolHandlerTestDefinition()
	contract := appCommandToolHandlerTestContract(definition)
	tool := &appCommandToolHandlerTestTool{}
	a.toolRegistry.MustRegister(tool)
	catalogID := uuid.Must(uuid.NewV7()).String()
	if schema == "" {
		schema = string(tool.Parameters())
	}
	if err := database.DB().WithContext(ctx).Create(&database.ToolCatalog{
		UUIDModel:          database.UUIDModel{ID: catalogID},
		Name:               tool.Name(),
		DisplayName:        tool.Name(),
		Description:        tool.Description(),
		Origin:             "builtin",
		Schema:             schema,
		AvailabilityStatus: status,
	}).Error; err != nil {
		t.Fatal(err)
	}
	return a, ctx, definition, contract, catalogID
}

func TestNewCommandToolHandlerRejectsForeignAndInvalidCatalogState(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		schema     string
		catalog    bool
		foreignCtx bool
	}{
		{name: "contexto estrangeiro", status: "available", catalog: true, foreignCtx: true},
		{name: "catalogo inexistente", status: "available", catalog: false},
		{name: "catalogo indisponivel", status: "disabled", catalog: true},
		{name: "schema divergente", status: "available", catalog: true, schema: `{"type":"object","additionalProperties":false}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				a          *App
				ctx        context.Context
				definition commandcatalog.Definition
				contract   commandcatalog.HandlerContract
				catalogID  string
			)
			if tt.catalog {
				a, ctx, definition, contract, catalogID = setupAppCommandToolHandlerTest(t, tt.status, tt.schema)
			} else {
				a, ctx, definition, contract, catalogID = setupAppCommandToolHandlerTest(t, "available", "")
				if err := database.DB().Where("id = ?", catalogID).Delete(&database.ToolCatalog{}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if tt.foreignCtx {
				foreign := database.WithUserID(context.Background(), uuid.Must(uuid.NewV7()).String())
				ctx = foreign
			}

			_, err := a.newCommandToolHandler(ctx, definition, contract, catalogID, commandcatalog.SensitivePaths{}, func(result tools.ToolResult) (json.RawMessage, error) {
				return json.RawMessage(result.Content), nil
			})
			if err == nil {
				t.Fatal("factory aceitou configuração inválida")
			}
			if !errors.Is(err, commandexecution.ErrDenied) {
				t.Fatalf("erro = %v, want ErrDenied", err)
			}
		})
	}
}

func TestNewCommandToolHandlerRejectsUnboundServiceOrRegistry(t *testing.T) {
	tests := []struct {
		name string
		bind func(*App)
	}{
		{name: "service com outro repositorio", bind: func(a *App) {
			otherDB := database.DB().Session(&gorm.Session{})
			a.authMu.Lock()
			defer a.authMu.Unlock()
			a.toolInvocationSvc = toolinvocations.NewService(
				toolinvocations.NewDBRepository(otherDB),
				tools.NewExecutor(a.toolRegistry, tools.DefaultExecutorConfig()),
			)
		}},
		{name: "service com outro registry", bind: func(a *App) {
			otherRegistry := tools.NewRegistry()
			a.authMu.Lock()
			defer a.authMu.Unlock()
			a.toolInvocationSvc = toolinvocations.NewService(
				toolinvocations.NewDBRepository(database.DB()),
				tools.NewExecutor(otherRegistry, tools.DefaultExecutorConfig()),
			)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, ctx, definition, contract, catalogID := setupAppCommandToolHandlerTest(t, "available", "")
			tt.bind(a)
			if a.toolInvocationSvc.IsBoundTo(database.DB(), a.toolRegistry) {
				t.Fatal("fixture ainda considera o service ligado ao registry do App")
			}
			_, err := a.newCommandToolHandler(ctx, definition, contract, catalogID, commandcatalog.SensitivePaths{}, func(result tools.ToolResult) (json.RawMessage, error) {
				return json.RawMessage(result.Content), nil
			})
			if !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
				t.Fatalf("factory com binding inválido = %v, want ErrInvalidConfiguration", err)
			}
		})
	}
}

func TestNewCommandToolHandlerCapturesSessionBeforeToolAdmission(t *testing.T) {
	a, ctx, definition, contract, catalogID := setupAppCommandToolHandlerTest(t, "available", "")
	if _, err := a.newCommandToolHandler(ctx, definition, contract, catalogID, commandcatalog.SensitivePaths{}, func(result tools.ToolResult) (json.RawMessage, error) {
		return json.RawMessage(result.Content), nil
	}); err != nil {
		t.Fatalf("montagem inicial: %v", err)
	}

	if err := database.DB().Model(&database.Session{}).Where("id = ?", a.currentAuthUser.SessionID).Update("revoked_at", time.Now().UTC()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := a.newCommandToolHandler(ctx, definition, contract, catalogID, commandcatalog.SensitivePaths{}, func(result tools.ToolResult) (json.RawMessage, error) {
		return json.RawMessage(result.Content), nil
	}); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("factory após revogação = %v, want ErrDenied", err)
	}
}
