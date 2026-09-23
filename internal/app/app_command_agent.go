package app

import (
	"context"
	"errors"
	"strings"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	commandtool "assistente/internal/tools/command"
	"assistente/internal/tools/invocationctx"
	"gorm.io/gorm"
)

// A porta não é exportada por Wails. O executor comum de tools fornece a
// identidade da invocação; argumentos do modelo nunca fornecem owner/sessão.
type commandAgentTools struct{ app *App }

type commandAgentSessionKey struct{}

// Só o ingresso local de mensagens instala este contexto, antes do turno.
// Canais/CLI/jobs não recebem a identidade da sessão desktop por proximidade.
type commandChatSession struct{ wailsSession }

func (s commandChatSession) AuthenticatedContext() (context.Context, error) {
	ctx, err := s.wailsSession.AuthenticatedContext()
	if err != nil {
		return nil, err
	}
	if !s.app.chatDesktopIngress {
		return ctx, nil
	}
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	s.app.authMu.RLock()
	defer s.app.authMu.RUnlock()
	if s.app.currentAuthUser == nil || s.app.currentAuthUser.UserID != user || s.app.currentAuthUser.SessionID == "" {
		return ctx, nil
	}
	principal := auth.LocalSessionPrincipal{UserID: user, SessionID: s.app.currentAuthUser.SessionID}
	return context.WithValue(ctx, commandAgentSessionKey{}, principal), nil
}

type commandAgentCaller struct {
	app                             *App
	product                         *commandProductRuntime
	principal                       auth.LocalSessionPrincipal
	ctx                             context.Context
	invocationID, toolName          string
	conversationID, turnID, profile string
}

func (a *App) commandAgentAccess(ctx context.Context, toolName string) (*commandAgentCaller, error) {
	if a == nil || ctx == nil || ctx.Err() != nil {
		return nil, commandexecution.ErrDenied
	}
	owner, err := database.RequireUserID(ctx)
	inv, ok := invocationctx.Get(ctx)
	p := a.commandProduct.Load()
	id := toolinvocations.CurrentInvocationID(ctx)
	started, local := ctx.Value(commandAgentSessionKey{}).(auth.LocalSessionPrincipal)
	if err != nil || !ok || p == nil || !local || started != p.principal || owner != p.principal.UserID || id == "" ||
		inv.ConversationID == "" || inv.TurnID == "" || strings.TrimSpace(inv.ProfileSlug) == "" {
		return nil, commandexecution.ErrDenied
	}
	c := &commandAgentCaller{app: a, product: p, principal: p.principal, ctx: ctx, invocationID: id,
		toolName: toolName, conversationID: inv.ConversationID, turnID: inv.TurnID, profile: inv.ProfileSlug}
	if err := c.revalidate(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *commandAgentCaller) revalidate(ctx context.Context) error {
	return c.revalidateTx(ctx, database.DB())
}

// Reutiliza a conexão do commit quando chamado pelo applier. Nunca disputa
// uma segunda conexão nem consulta um snapshot externo ao writer.
func (c *commandAgentCaller) revalidateTx(ctx context.Context, tx *gorm.DB) error {
	if c == nil || tx == nil || ctx == nil || ctx.Err() != nil || c.ctx.Err() != nil || c.app.commandProduct.Load() != c.product || !c.product.dependenciesMatch(c.app) {
		return commandexecution.ErrDenied
	}
	p, err := c.product.sessionSvc.RevalidateLocalSessionTx(ctx, tx, c.principal)
	if err != nil || p != c.principal || !c.app.commandPrincipalMatches(c.product.sessionSvc, c.product.credMgr, p) {
		return commandexecution.ErrDenied
	}
	// Identidade continua válida durante a suspensão do mapa que o applier
	// realiza antes do commit. Prontidão/versões da configuração pertencem ao
	// executor e ao serviço de mutação, não à autenticação da tool.
	ready, err := c.product.host.SourceSecurityReady(ctx)
	if err != nil || !ready {
		return commandexecution.ErrDenied
	}
	// Um canal, job, dry-run, invocação encerrada ou chamada de outra tool
	// não pode emprestar o presenter interativo da sessão desktop.
	var count int64
	err = tx.WithContext(ctx).Table("tool_invocations AS i").
		Joins("JOIN tool_catalog AS t ON t.id = i.tool_catalog_id").
		Joins("JOIN conversations AS c ON c.id = i.conversation_id AND c.user_id = i.user_id").
		Joins("JOIN sessions AS s ON s.id = ? AND s.user_id = i.user_id", p.SessionID).
		Joins("JOIN users AS u ON u.id = i.user_id AND u.is_active = ?", true).
		Where("u.role IN ?", []string{database.UserRoleUser, database.UserRoleAdmin}).
		Where("t.origin = ?", "builtin").
		Where("i.id = ? AND i.user_id = ? AND i.origin_type = ? AND i.status = ? AND i.dry_run = ? AND t.name = ?", c.invocationID, p.UserID, toolinvocations.OriginChat, toolinvocations.StatusRunning, false, c.toolName).
		Where("i.conversation_id = ? AND i.turn_id = ? AND i.origin_id = ? AND COALESCE(c.channel, '') = '' AND COALESCE(c.kind, '') = ''", c.conversationID, c.turnID, c.turnID).
		Where("i.queued_at >= s.created_at").Count(&count).Error
	if err != nil || count != 1 {
		return commandexecution.ErrDenied
	}
	return nil
}

func (c *commandAgentCaller) AuthenticateLocalAccess(ctx context.Context, token string) (auth.LocalSessionPrincipal, error) {
	if token != "" {
		return auth.LocalSessionPrincipal{}, commandexecution.ErrDenied
	}
	if err := c.revalidate(ctx); err != nil {
		return auth.LocalSessionPrincipal{}, err
	}
	return c.principal, nil
}

type commandAgentDescription struct {
	ID                         string                  `json:"id"`
	Name                       string                  `json:"name"`
	Description                string                  `json:"description"`
	AllowedSources             []commandcatalog.Source `json:"allowed_sources"`
	Effect                     commandcatalog.Effect   `json:"effect"`
	DecisionRequirement        commandcatalog.Decision `json:"decision_requirement"`
	MutatesEffectiveCapability bool                    `json:"mutates_effective_capability"`
	Arguments                  map[string]any          `json:"arguments_schema,omitempty"`
	ExecutableFromChat         bool                    `json:"executable_from_chat"`
}

func (b commandAgentTools) Catalog(ctx context.Context, req commandtool.Request) (result any, err error) {
	defer func() { err = commandAgentToolError(err) }()
	c, err := b.app.commandAgentAccess(ctx, "command_catalog")
	if err != nil {
		return nil, err
	}
	if req.Action == "execute" {
		return b.app.executeAgentCommand(ctx, c, req)
	}
	if req.Action != "list" && req.Action != "describe" {
		return nil, commandexecution.ErrInvalidRequest
	}
	items := make([]commandAgentDescription, 0)
	for _, d := range c.product.registry.List() {
		if req.Action == "describe" && req.CommandID != d.ID {
			continue
		}
		item := commandAgentDescription{ID: d.ID, AllowedSources: d.AllowedSources, Effect: d.Effect, DecisionRequirement: d.Decision, MutatesEffectiveCapability: d.MutatesEffectiveCapability, ExecutableFromChat: d.AllowsSource(commandcatalog.Chat)}
		if req.Action == "describe" {
			item.Arguments = commandcatalog.JSONSchema(d.ArgumentsSchema)
		}
		if d.Presentation != nil {
			m, ok := d.Presentation.Locales[req.Locale]
			if !ok {
				m = d.Presentation.Locales["pt-BR"]
			}
			item.Name, item.Description = m.Name, m.Description
		}
		items = append(items, item)
	}
	if req.Action == "describe" {
		if len(items) != 1 {
			return nil, commandexecution.ErrInvalidRequest
		}
		return items[0], c.revalidate(ctx)
	}
	return struct {
		Commands []commandAgentDescription `json:"commands"`
	}{items}, c.revalidate(ctx)
}

func commandAgentToolError(err error) error {
	switch {
	case errors.Is(err, commandexecution.ErrStale), errors.Is(err, commandconfig.ErrStale):
		return errors.Join(commandtool.ErrStale, err)
	case errors.Is(err, commandexecution.ErrDenied):
		return errors.Join(commandtool.ErrDenied, err)
	case errors.Is(err, commandexecution.ErrInvalidRequest), errors.Is(err, commandconfig.ErrInvalid):
		return errors.Join(commandtool.ErrInvalidRequest, err)
	default:
		return err
	}
}
