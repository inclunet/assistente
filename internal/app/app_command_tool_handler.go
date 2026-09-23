package app

import (
	"bytes"
	"context"
	"encoding/json"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandjson"
	"assistente/internal/commandtoolbridge"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"github.com/google/uuid"
)

// Montagem interna de uma delegação local de alvo fixo. O ID de catálogo é
// escolhido pelo bootstrap, nunca resolvido a partir de um nome vindo da UI.
// A tool continua sujeita às próprias políticas (inclusive commandpolicy).
func (a *App) newCommandToolHandler(ctx context.Context, definition commandcatalog.Definition, contract commandcatalog.HandlerContract, catalogID string, paths commandcatalog.SensitivePaths, output func(tools.ToolResult) (json.RawMessage, error)) (commandexecution.Handler, error) {
	invalid := commandexecution.ErrInvalidConfiguration
	if a == nil || ctx == nil || ctx.Err() != nil || !commandToolUUID(catalogID) ||
		contract.Classification != commandcatalog.HandlerTool || contract.Effect != commandcatalog.Destructive || !contract.HasMutableTarget ||
		definition.Decision != commandcatalog.Interactive || commandcatalog.ValidateDefinitionComplete(definition, contract) != nil {
		return commandexecution.Handler{}, invalid
	}
	catalog, err := commandcatalog.NewComplete([]commandcatalog.Registration{{Definition: definition, Handler: contract}})
	if err != nil {
		return commandexecution.Handler{}, invalid
	}
	definition, _ = catalog.Lookup(definition.ID)
	principal, err := a.currentCommandPrincipal()
	owner, ownerErr := database.RequireUserID(ctx)
	if err != nil || ownerErr != nil || owner != principal.UserID {
		return commandexecution.Handler{}, commandexecution.ErrDenied
	}
	a.authMu.RLock()
	service, registry, sessions, credentials := a.toolInvocationSvc, a.toolRegistry, a.sessionSvc, a.credMgr
	manager := a.jobMgr
	a.authMu.RUnlock()
	db := database.DB()
	if service == nil || registry == nil || sessions == nil || credentials == nil || manager == nil || db == nil || !service.IsBoundTo(db, registry) {
		return commandexecution.Handler{}, invalid
	}
	// Repositório novo evita usar cache de resolução por nome como autorização.
	load := func(check context.Context) (database.ToolCatalog, []byte, error) {
		var row database.ToolCatalog
		user, err := database.RequireUserID(check)
		if err != nil || user != principal.UserID {
			return row, nil, commandexecution.ErrDenied
		}
		repo := toolinvocations.NewDBRepository(db)
		visible, err := repo.IsToolCatalogIDVisible(check, catalogID)
		if err != nil || !visible || db.WithContext(check).Where("id = ?", catalogID).Take(&row).Error != nil || row.AvailabilityStatus != "available" {
			return row, nil, commandexecution.ErrDenied
		}
		// Delegação de profiles/jobs tem portas próprias. Não
		// são reclassificadas como uma tool genérica para contornar essas provas.
		if row.Name == "subagent" || row.Name == "job" || row.Origin == "archival" {
			return row, nil, commandexecution.ErrDenied
		}
		resolved, err := repo.ResolveToolCatalogID(check, row.Name)
		if err != nil || resolved != catalogID {
			return row, nil, commandexecution.ErrDenied
		}
		schema, err := commandjson.Canonicalize([]byte(row.Schema))
		if err != nil {
			return row, nil, commandexecution.ErrDenied
		}
		fingerprint, err := commandjson.Marshal(map[string]any{"id": row.ID, "name": row.Name, "origin": row.Origin,
			"user": row.UserID, "mcp_server": row.MCPServerID, "schema": json.RawMessage(schema), "risk": row.Risk, "class": row.Class, "package": row.Package})
		return row, fingerprint, err
	}
	row, fingerprint, err := load(ctx)
	if err != nil {
		return commandexecution.Handler{}, err
	}
	tool, generation, found := registry.GetWithGeneration(row.Name)
	if !found || generation == 0 || tool == nil {
		return commandexecution.Handler{}, commandexecution.ErrDenied
	}
	schema, err := commandjson.Canonicalize(tool.Parameters())
	catalogSchema, catalogErr := commandjson.Canonicalize([]byte(row.Schema))
	if err != nil || catalogErr != nil || !bytes.Equal(schema, catalogSchema) {
		return commandexecution.Handler{}, commandexecution.ErrDenied
	}
	checkTarget := func(check context.Context) error {
		if check == nil || check.Err() != nil || database.DB() != db || !a.commandPrincipalMatches(sessions, credentials, principal) {
			return commandexecution.ErrDenied
		}
		a.authMu.RLock()
		same := a.toolInvocationSvc == service && a.toolRegistry == registry && a.jobMgr == manager
		a.authMu.RUnlock()
		if !same || !service.IsBoundTo(db, registry) {
			return commandexecution.ErrDenied
		}
		current, err := sessions.RevalidateLocalSession(check, principal)
		if err != nil || current != principal {
			return commandexecution.ErrDenied
		}
		_, latest, err := load(check)
		currentTool, currentGeneration, exists := registry.GetWithGeneration(row.Name)
		if err != nil || !bytes.Equal(latest, fingerprint) || !exists || currentGeneration != generation {
			return commandexecution.ErrDenied
		}
		currentSchema, err := commandjson.Canonicalize(currentTool.Parameters())
		if err != nil || !bytes.Equal(currentSchema, schema) || !a.commandPrincipalMatches(sessions, credentials, principal) {
			return commandexecution.ErrDenied
		}
		current, err = sessions.RevalidateLocalSession(check, principal)
		if err != nil || current != principal {
			return commandexecution.ErrDenied
		}
		return check.Err()
	}
	if err := checkTarget(ctx); err != nil {
		return commandexecution.Handler{}, err
	}
	checkInvocation := func(check context.Context, in commandexecution.Invocation) error {
		e := in.Envelope
		if in.Principal != principal || in.CommandID != definition.ID || !definition.AllowsSource(in.Source) ||
			e == nil || e.AuthContextType != commandcontract.AuthLocalSession || e.AuthContextID != principal.SessionID ||
			e.ActorType != commandcontract.ActorUser || e.ActorID != principal.UserID || e.UserID == nil || *e.UserID != principal.UserID ||
			e.AuthorizationDecisionID == nil || !commandToolUUID(*e.AuthorizationDecisionID) || e.Provenance == nil ||
			e.JobID != nil || e.JobSlug != nil || e.JobDefinitionFingerprint != nil || e.RunID != nil ||
			e.SourceProfileSlug != nil || e.TargetProfileSlug != nil || e.DelegationFingerprint != nil || e.GrantGeneration != nil ||
			e.ConversationID != nil || e.TurnID != nil || e.SurfaceType != nil || e.SurfaceID != nil || e.SurfaceSnapshotVersion != nil {
			return commandexecution.ErrDenied
		}
		return checkTarget(check)
	}
	resolveOrigin := func(check context.Context, in commandexecution.Invocation) (jobs.CommandJobOrigin, error) {
		return a.resolveCommandJobOrigin(check, in, manager)
	}
	// A raiz é anexada no worker por uma porta privada do runtime. Eventos
	// produzidos pela tool preservam a cadeia, sem criar um run de job fictício.
	prepare := func(check context.Context, in commandexecution.Invocation) (context.Context, func(), error) {
		return manager.CommandToolContext(check, in, resolveOrigin)
	}
	authorize := func(check context.Context, in commandexecution.Invocation) error {
		if err := checkInvocation(check, in); err != nil {
			return err
		}
		return manager.ValidateCommandToolContext(check, in)
	}
	bridge, err := commandtoolbridge.New(commandtoolbridge.Config{Service: service, Routes: []commandtoolbridge.Route{{
		CommandID: definition.ID, Definition: definition, Contract: contract, ToolName: row.Name, ToolCatalogID: catalogID,
		SensitivePaths: paths, OutputAdapter: output, Authorize: authorize, ToolGeneration: generation, PrepareContext: prepare,
	}}})
	if err != nil {
		return commandexecution.Handler{}, err
	}
	handler, ok := bridge.Handler(definition.ID)
	if !ok {
		return commandexecution.Handler{}, invalid
	}
	return handler, nil
}

func commandToolUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}
