package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandjson"
	"assistente/internal/database"
	"assistente/internal/logging"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
)

const commandToolIDPrefix = "tool.execute.t_"

const maxCommandToolArgumentsBytes = 48 << 10

// commandToolExecutionID vincula o comando ao UUIDv7 canônico do catálogo.
// Nenhum identificador da tool é aceito nos argumentos da invocação.
func commandToolExecutionID(catalogID string) (string, bool) {
	id, err := uuid.Parse(catalogID)
	if err != nil || id.Version() != 7 || id.Variant() != uuid.RFC4122 || id.String() != catalogID {
		return "", false
	}
	return commandToolIDPrefix + strings.ReplaceAll(catalogID, "-", ""), true
}

func isCommandToolExecutionID(commandID string) bool {
	if !strings.HasPrefix(commandID, commandToolIDPrefix) {
		return false
	}
	raw := strings.TrimPrefix(commandID, commandToolIDPrefix)
	if len(raw) != 32 || strings.ToLower(raw) != raw {
		return false
	}
	catalogID := raw[:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:]
	canonical, ok := commandToolExecutionID(catalogID)
	return ok && canonical == commandID
}

func commandToolCatalogID(commandID string) (string, bool) {
	if !isCommandToolExecutionID(commandID) {
		return "", false
	}
	raw := strings.TrimPrefix(commandID, commandToolIDPrefix)
	return raw[:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:], true
}

// commandToolRegistrations projeta alvos fixos por tool visível ao usuário
// atual. A projeção não torna uma tool executável: o handler volta a conferir
// catálogo, owner, schema, disponibilidade e geração no instante da execução.
func (a *App) commandToolRegistrations() ([]commandcatalog.Registration, map[string]commandexecution.Handler) {
	if a == nil || database.DB() == nil {
		return nil, nil
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil || principal.UserID == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(database.WithUserID(context.Background(), principal.UserID), 5*time.Second)
	defer cancel()
	a.authMu.RLock()
	service, registry, sessions, credentials, manager := a.toolInvocationSvc, a.toolRegistry, a.sessionSvc, a.credMgr, a.jobMgr
	a.authMu.RUnlock()
	db := database.DB()
	if service == nil || registry == nil || sessions == nil || credentials == nil || manager == nil || !service.IsBoundTo(db, registry) {
		return nil, nil
	}
	const catalogPageSize = 128
	const catalogProjectionLimit = 4096
	baseQuery := db.WithContext(ctx).Where("availability_status = ? AND (tool_catalog.user_id IS NULL OR EXISTS (SELECT 1 FROM mcp_servers WHERE mcp_servers.id = tool_catalog.mcp_server_id AND mcp_servers.user_id = ?))", "available", principal.UserID)
	rows := make([]database.ToolCatalog, 0, catalogPageSize)
	lastID := ""
	for len(rows) < catalogProjectionLimit {
		page := make([]database.ToolCatalog, 0, catalogPageSize)
		query := baseQuery
		if lastID != "" {
			query = query.Where("id > ?", lastID)
		}
		limit := min(catalogPageSize, catalogProjectionLimit-len(rows))
		if err := query.Order("id ASC").Limit(limit).Find(&page).Error; err != nil || ctx.Err() != nil {
			return nil, nil
		}
		if len(page) == 0 {
			break
		}
		rows = append(rows, page...)
		lastID = page[len(page)-1].ID
		if len(page) < limit {
			break
		}
	}
	if len(rows) == catalogProjectionLimit {
		var probe database.ToolCatalog
		if err := baseQuery.Where("id > ?", lastID).Order("id ASC").Limit(1).Take(&probe).Error; err == nil {
			logging.Warnf(ctx, "app.commands", "tool catalog projection truncated (command_tool_catalog_limit)")
		}
	}
	repo := toolinvocations.NewDBRepository(db)
	registrations := make([]commandcatalog.Registration, 0, len(rows))
	handlers := make(map[string]commandexecution.Handler)
	for _, row := range rows {
		commandID, valid := commandToolExecutionID(row.ID)
		if !valid || row.Origin == "archival" || row.Name == "subagent" || row.Name == "job" {
			continue
		}
		visible, visibilityErr := repo.IsToolCatalogIDVisible(ctx, row.ID)
		if visibilityErr != nil || !visible {
			continue
		}
		tool, generation, found := registry.GetWithGeneration(row.Name)
		if !found || tool == nil || generation == 0 {
			continue
		}
		toolSchema, canonicalErr := commandjson.Canonicalize(tool.Parameters())
		catalogSchema, catalogErr := commandjson.Canonicalize([]byte(row.Schema))
		if canonicalErr != nil || catalogErr != nil || !bytes.Equal(toolSchema, catalogSchema) {
			continue
		}
		if _, schemaErr := resolveCommandToolSchema(catalogSchema); schemaErr != nil {
			continue
		}
		semanticVersion, versionErr := commandToolSemanticVersion(row, tool, generation)
		if versionErr != nil {
			continue
		}
		definition, contract := commandToolExecutionRegistration(commandID, row.DisplayName, semanticVersion)
		handler, handlerErr := a.newCommandCatalogToolHandler(ctx, definition, contract, row.ID)
		if handlerErr != nil {
			continue
		}
		registrations = append(registrations, commandcatalog.Registration{Definition: definition, Handler: contract})
		handlers[commandID] = handler
	}
	return registrations, handlers
}

func commandToolExecutionRegistration(commandID, displayName, semanticVersion string) (commandcatalog.Definition, commandcatalog.HandlerContract) {
	if strings.TrimSpace(displayName) == "" {
		displayName = "ferramenta"
	}
	maxArgumentLength := maxCommandToolArgumentsBytes
	locales := map[string]commandcatalog.LocalizedMetadata{
		"pt-BR": {Name: "Executar: " + displayName, Description: "Executa a ferramenta escolhida após confirmação interativa", Category: "Ferramentas"},
		"en":    {Name: "Run: " + displayName, Description: "Runs the selected tool after interactive confirmation", Category: "Tools"},
		"es":    {Name: "Ejecutar: " + displayName, Description: "Ejecuta la herramienta seleccionada tras confirmación interactiva", Category: "Herramientas"},
	}
	route := "tools/catalog/" + strings.TrimPrefix(commandID, commandToolIDPrefix) + "/" + semanticVersion
	contract := commandcatalog.HandlerContract{Effect: commandcatalog.Destructive, HasMutableTarget: true, Route: route, Classification: commandcatalog.HandlerTool}
	return commandcatalog.Definition{
		ID: commandID, Effect: commandcatalog.Destructive, Decision: commandcatalog.Interactive,
		AllowedSources: []commandcatalog.Source{commandcatalog.Palette},
		Context: commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{
			Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion,
		}}},
		Presentation: &commandcatalog.Presentation{Version: "tool-execute-v1/" + semanticVersion, Locales: locales},
		ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{
			"arguments_json": {Type: commandcatalog.SchemaString, MaxLength: &maxArgumentLength},
		}, Required: []string{"arguments_json"}},
		ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{
			"result": {Type: commandcatalog.SchemaString},
		}, Required: []string{"result"}},
		Risk:        commandcatalog.RiskHigh,
		Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceNever, Result: commandcatalog.PersistenceNever, Audit: commandcatalog.PersistenceRedacted},
		Scopes:      []commandcatalog.Scope{commandcatalog.ScopeSession}, Availability: commandcatalog.Availability{Status: commandcatalog.Available},
		HandlerRoute: route, HandlerClassification: commandcatalog.HandlerTool, HasMutableTarget: true,
		SensitivePaths: commandcatalog.SensitivePaths{Input: []string{""}, Output: []string{""}},
	}, contract
}

// commandToolSemanticVersion faz parte da rota semântica do comando: mudanças
// no schema/identidade/generation da tool invalidam configuração e receipts,
// mesmo que o wrapper ArgumentsSchema permaneça igual.
func commandToolSemanticVersion(row database.ToolCatalog, tool tools.Tool, generation uint64) (string, error) {
	if tool == nil || generation == 0 {
		return "", commandexecution.ErrInvalidConfiguration
	}
	toolSchema, err := commandjson.Canonicalize(tool.Parameters())
	if err != nil {
		return "", err
	}
	catalogSchema, err := commandjson.Canonicalize([]byte(row.Schema))
	if err != nil || !bytes.Equal(toolSchema, catalogSchema) {
		return "", commandexecution.ErrInvalidConfiguration
	}
	canonical, err := commandjson.Marshal(map[string]any{
		"id": row.ID, "name": row.Name, "origin": row.Origin, "user": row.UserID,
		"mcp_server": row.MCPServerID, "schema": json.RawMessage(catalogSchema),
		"risk": row.Risk, "class": row.Class, "package": row.Package, "generation": generation,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func (a *App) newCommandCatalogToolHandler(ctx context.Context, definition commandcatalog.Definition, contract commandcatalog.HandlerContract, catalogID string) (commandexecution.Handler, error) {
	row, err := loadCommandToolSchemaRow(ctx, catalogID)
	if err != nil {
		return commandexecution.Handler{}, commandexecution.ErrDenied
	}
	resolved, err := resolveCommandToolSchema([]byte(row.Schema))
	if err != nil {
		return commandexecution.Handler{}, commandexecution.ErrDenied
	}
	a.authMu.RLock()
	semanticRegistry := a.toolRegistry
	a.authMu.RUnlock()
	if semanticRegistry == nil {
		return commandexecution.Handler{}, commandexecution.ErrDenied
	}
	semanticTool, semanticGeneration, semanticFound := semanticRegistry.GetWithGeneration(row.Name)
	if !semanticFound {
		return commandexecution.Handler{}, commandexecution.ErrDenied
	}
	semanticVersion, err := commandToolSemanticVersion(row, semanticTool, semanticGeneration)
	if err != nil || !strings.HasSuffix(definition.HandlerRoute, "/"+semanticVersion) {
		return commandexecution.Handler{}, commandexecution.ErrDenied
	}
	inputAdapter := func(raw json.RawMessage) (json.RawMessage, error) {
		canonical, canonicalErr := decodeCommandToolInput(raw)
		if canonicalErr != nil || resolved.Validate(decodeJSONNumber(canonical)) != nil {
			return nil, commandexecution.ErrDenied
		}
		return json.RawMessage(append([]byte(nil), canonical...)), nil
	}
	base, err := a.newCommandToolHandlerWithInputAdapter(ctx, definition, contract, catalogID, definition.SensitivePaths, func(result tools.ToolResult) (json.RawMessage, error) {
		encoded, marshalErr := json.Marshal(map[string]string{"result": result.Content})
		if marshalErr != nil {
			return nil, fmt.Errorf("tool result encoding failed")
		}
		return encoded, nil
	}, inputAdapter)
	if err != nil {
		return commandexecution.Handler{}, err
	}
	return base, nil
}

// commandToolDecisionBody confirma uma instância identificada e o schema que
// compôs o comando. Argumentos são validados antes da decisão, mas valores
// nunca são incorporados ao texto do recibo.
func commandToolDecisionBody(a *App, p *commandProductRuntime, definition commandcatalog.Definition, envelope commandcontract.Envelope) (string, error) {
	if a == nil || p == nil || a.commandProduct.Load() != p || !p.dependenciesMatch(a) || envelope.CommandID == nil || *envelope.CommandID != definition.ID || envelope.Arguments == nil {
		return "", commandexecution.ErrDenied
	}
	catalogID, ok := commandToolCatalogID(definition.ID)
	if !ok {
		return "", commandexecution.ErrDenied
	}
	ctx, cancel := context.WithTimeout(database.WithUserID(context.Background(), p.principal.UserID), 3*time.Second)
	defer cancel()
	row, err := loadCommandToolSchemaRow(ctx, catalogID)
	if err != nil {
		return "", commandexecution.ErrDenied
	}
	a.authMu.RLock()
	registry := a.toolRegistry
	a.authMu.RUnlock()
	if registry == nil {
		return "", commandexecution.ErrDenied
	}
	tool, generation, found := registry.GetWithGeneration(row.Name)
	if !found {
		return "", commandexecution.ErrDenied
	}
	semanticVersion, err := commandToolSemanticVersion(row, tool, generation)
	if err != nil || !strings.HasSuffix(definition.HandlerRoute, "/"+semanticVersion) {
		return "", commandexecution.ErrStale
	}
	repo := toolinvocations.NewDBRepository(database.DB())
	visible, err := repo.IsToolCatalogIDVisible(ctx, catalogID)
	if err != nil || !visible {
		return "", commandexecution.ErrDenied
	}
	if _, err := decodeCommandToolInput(*envelope.Arguments); err != nil {
		return "", commandexecution.ErrDenied
	}
	args, _ := decodeCommandToolInput(*envelope.Arguments)
	resolved, err := resolveCommandToolSchema([]byte(row.Schema))
	if err != nil || resolved.Validate(decodeJSONNumber(args)) != nil {
		return "", commandexecution.ErrDenied
	}
	name := strings.TrimSpace(row.DisplayName)
	if name == "" {
		name = row.Name
	}
	name = safeCommandToolLabel(name)
	targetName := safeCommandToolLabel(row.Name)
	return "Ferramenta: " + name + "\nAlvo fixo: " + targetName + " (catálogo " + row.ID + ")\nArgumentos: [redigidos]", nil
}

func safeCommandToolLabel(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 160 {
		runes = runes[:160]
	}
	return string(runes)
}

func decodeCommandToolInput(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 || len(raw) > maxCommandToolArgumentsBytes+1024 {
		return nil, commandexecution.ErrInvalidRequest
	}
	canonical, err := commandjson.Canonicalize(raw)
	if err != nil {
		return nil, commandexecution.ErrInvalidRequest
	}
	var wrapper struct {
		ArgumentsJSON string `json:"arguments_json"`
	}
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&wrapper) != nil || wrapper.ArgumentsJSON == "" || len(wrapper.ArgumentsJSON) > maxCommandToolArgumentsBytes {
		return nil, commandexecution.ErrInvalidRequest
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, commandexecution.ErrInvalidRequest
	}
	args, err := commandjson.Canonicalize([]byte(wrapper.ArgumentsJSON))
	if err != nil {
		return nil, commandexecution.ErrInvalidRequest
	}
	return args, nil
}

func loadCommandToolSchemaRow(ctx context.Context, catalogID string) (database.ToolCatalog, error) {
	var row database.ToolCatalog
	db := database.DB()
	if db == nil || ctx == nil || ctx.Err() != nil || !commandToolUUID(catalogID) {
		return row, commandexecution.ErrDenied
	}
	if err := db.WithContext(ctx).Where("id = ? AND availability_status = ?", catalogID, "available").Take(&row).Error; err != nil {
		return row, commandexecution.ErrDenied
	}
	return row, nil
}

func resolveCommandToolSchema(raw []byte) (*jsonschema.Resolved, error) {
	if len(raw) == 0 || len(raw) > 64<<10 || rejectUnsupportedCommandToolSchema(raw) != nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	return schema.Resolve(&jsonschema.ResolveOptions{})
}

// O validador atual ignora format/content e palavras desconhecidas. Como isso
// não prova a semântica exigida por uma tool, qualquer ocorrência é recusada.
func rejectUnsupportedCommandToolSchema(raw []byte) error {
	var root any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&root) != nil {
		return commandexecution.ErrInvalidConfiguration
	}
	allowed := map[string]bool{}
	for _, key := range strings.Fields(`$schema $id $ref $anchor $dynamicRef $dynamicAnchor $defs definitions title description default deprecated readOnly writeOnly examples $comment type enum const multipleOf maximum exclusiveMaximum minimum exclusiveMinimum maxLength minLength pattern maxItems minItems uniqueItems contains maxContains minContains maxProperties minProperties required properties patternProperties additionalProperties dependencies propertyNames if then else allOf anyOf oneOf not unevaluatedItems unevaluatedProperties items prefixItems dependentSchemas dependentRequired`) {
		allowed[key] = true
	}
	var walkSchema func(any) error
	var walkMap func(any) error
	walkSchema = func(node any) error {
		if _, ok := node.(bool); ok {
			return nil
		}
		object, ok := node.(map[string]any)
		if !ok {
			return commandexecution.ErrInvalidConfiguration
		}
		for key := range object {
			if !allowed[key] || key == "format" || strings.HasPrefix(key, "content") || strings.HasPrefix(key, "$dynamic") || key == "$anchor" {
				return commandexecution.ErrInvalidConfiguration
			}
		}
		for _, key := range []string{"additionalProperties", "unevaluatedItems", "unevaluatedProperties", "items", "contains", "propertyNames", "not", "if", "then", "else"} {
			if child, ok := object[key]; ok {
				if key == "items" {
					if list, ok := child.([]any); ok {
						for _, item := range list {
							if err := walkSchema(item); err != nil {
								return err
							}
						}
						continue
					}
				}
				if err := walkSchema(child); err != nil {
					return err
				}
			}
		}
		for _, key := range []string{"properties", "patternProperties", "$defs", "definitions", "dependentSchemas"} {
			if child, ok := object[key]; ok {
				if err := walkMap(child); err != nil {
					return err
				}
			}
		}
		if dependencies, ok := object["dependencies"].(map[string]any); ok {
			for _, dependency := range dependencies {
				if _, isArray := dependency.([]any); !isArray {
					if err := walkSchema(dependency); err != nil {
						return err
					}
				}
			}
		}
		for _, key := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
			if schemas, ok := object[key].([]any); ok {
				for _, child := range schemas {
					if err := walkSchema(child); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	walkMap = func(node any) error {
		object, ok := node.(map[string]any)
		if !ok {
			return commandexecution.ErrInvalidConfiguration
		}
		for _, child := range object {
			if err := walkSchema(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walkSchema(root)
}

func decodeJSONNumber(raw []byte) any {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil
	}
	return value
}
