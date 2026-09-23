package app

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontext"
	"assistente/internal/commandexecution"
	"assistente/internal/commandforeground"
	"assistente/internal/workspace"
)

type commandOriginContext struct {
	facts      commandbindings.Facts
	version    string
	foreground *commandforeground.Snapshot
}

const commandPhysicalOriginMaxAge = 5 * time.Minute

// commandOriginFacts compõe somente fatos que têm provider autoritativo no
// backend. A surface e o foco pertencem ao contrato da UI e não podem ser
// inferidos da aba ativa para um acionamento externo.
func (p *commandProductRuntime) commandOriginFacts(ctx context.Context, source commandcatalog.Source, required []commandbindings.Field) (commandOriginContext, error) {
	return p.commandOriginFactsWithDevice(ctx, source, required, "")
}

func (p *commandProductRuntime) commandOriginFactsWithDevice(ctx context.Context, source commandcatalog.Source, required []commandbindings.Field, device string) (commandOriginContext, error) {
	result := commandOriginContext{facts: commandbindings.Facts{}}
	for _, field := range required {
		if field != commandbindings.Profile && field != commandbindings.Process && field != commandbindings.Device {
			return commandOriginContext{}, commandexecution.ErrDenied
		}
	}
	if len(required) == 0 {
		return result, nil
	}
	if source != commandcatalog.Palette && source != commandcatalog.StreamDeck && source != commandcatalog.KeyboardGlobal {
		return commandOriginContext{}, commandexecution.ErrDenied
	}
	for _, field := range required {
		if field == commandbindings.Process && source != commandcatalog.StreamDeck && source != commandcatalog.KeyboardGlobal {
			return commandOriginContext{}, commandexecution.ErrDenied
		}
		if field == commandbindings.Device && source != commandcatalog.StreamDeck {
			return commandOriginContext{}, commandexecution.ErrDenied
		}
	}
	if p == nil || p.app == nil || p.workspaceMgr == nil || ctx == nil {
		return commandOriginContext{}, commandexecution.ErrDenied
	}
	if err := ctx.Err(); err != nil {
		return commandOriginContext{}, err
	}
	p.app.authMu.RLock()
	if p.app.workspaceMgr != p.workspaceMgr {
		p.app.authMu.RUnlock()
		return commandOriginContext{}, commandexecution.ErrStale
	}
	current := p.app.currentAuthUser
	if current == nil || current.UserID != p.principal.UserID || current.SessionID != p.principal.SessionID || p.app.currentUserID != p.principal.UserID {
		p.app.authMu.RUnlock()
		return commandOriginContext{}, commandexecution.ErrDenied
	}
	snapshot, err := p.workspaceMgr.CommandSnapshot()
	p.app.authMu.RUnlock()
	if err != nil || snapshot.WorkspaceID != p.workspaceID {
		return commandOriginContext{}, commandexecution.ErrStale
	}
	if containsOriginField(required, commandbindings.Profile) {
		profile := snapshot.Tab.ProfileOverrideSlug
		if profile == "" {
			profile = snapshot.WorkspaceProfile
		}
		if profile == "" {
			return commandOriginContext{}, commandexecution.ErrDenied
		}
		result.facts[commandbindings.Profile] = profile
	}
	result.version = snapshot.Version
	for _, field := range required {
		switch field {
		case commandbindings.Device:
			if strings.TrimSpace(device) == "" {
				return commandOriginContext{}, commandexecution.ErrDenied
			}
			result.facts[commandbindings.Device] = device
		case commandbindings.Process:
			foreground, captureErr := p.capturePhysicalForeground(ctx)
			if captureErr != nil {
				return commandOriginContext{}, captureErr
			}
			result.foreground = &foreground
			result.facts[commandbindings.Process] = foreground.Summary.Executable
		}
	}
	return result, nil
}

func (p *commandProductRuntime) capturePhysicalForeground(ctx context.Context) (commandforeground.Snapshot, error) {
	if p == nil || p.app == nil || ctx == nil {
		return commandforeground.Snapshot{}, commandexecution.ErrDenied
	}
	reader := p.foregroundReader
	if reader == nil {
		reader = commandforeground.NewNative()
	}
	p.app.authMu.RLock()
	defer p.app.authMu.RUnlock()
	if p.app.workspaceMgr != p.workspaceMgr || p.app.currentAuthUser == nil ||
		p.app.currentAuthUser.UserID != p.principal.UserID || p.app.currentAuthUser.SessionID != p.principal.SessionID ||
		p.app.currentUserID != p.principal.UserID {
		return commandforeground.Snapshot{}, commandexecution.ErrDenied
	}
	snapshot, err := reader.Capture(ctx)
	if err != nil {
		return commandforeground.Snapshot{}, err
	}
	if commandforeground.ValidateSnapshot(snapshot, commandPhysicalOriginMaxAge) != nil {
		return commandforeground.Snapshot{}, commandexecution.ErrStale
	}
	return snapshot, nil
}

func (p *commandProductRuntime) commandOriginFactsFromSnapshot(ctx context.Context, source commandcatalog.Source, required []commandbindings.Field, device string, foreground *commandforeground.Snapshot) (commandOriginContext, error) {
	if ctx == nil || ctx.Err() != nil || (source != commandcatalog.KeyboardGlobal && source != commandcatalog.StreamDeck) {
		return commandOriginContext{}, commandexecution.ErrDenied
	}
	if containsOriginField(required, commandbindings.Device) && source != commandcatalog.StreamDeck {
		return commandOriginContext{}, commandexecution.ErrDenied
	}
	if containsOriginField(required, commandbindings.Process) && (foreground == nil || commandforeground.ValidateSnapshot(*foreground, commandPhysicalOriginMaxAge) != nil) {
		return commandOriginContext{}, commandexecution.ErrStale
	}
	result, err := p.commandOriginFactsWithDevice(ctx, source, filterOriginFields(required, commandbindings.Process), device)
	if err != nil {
		return commandOriginContext{}, err
	}
	// Resolve/revalidation only reuses this immutable event snapshot; it never
	// calls the native reader again after the physical ingress.
	if containsOriginField(required, commandbindings.Process) {
		result.foreground = foreground
		result.facts[commandbindings.Process] = foreground.Summary.Executable
	}
	if result.version == "" && containsPhysicalOriginField(required) {
		version, versionErr := p.commandWorkspaceOriginVersion(ctx)
		if versionErr != nil {
			return commandOriginContext{}, versionErr
		}
		result.version = version
	}
	return result, nil
}

func (p *commandProductRuntime) commandWorkspaceOriginVersion(ctx context.Context) (string, error) {
	if p == nil || p.app == nil || p.workspaceMgr == nil || ctx == nil {
		return "", commandexecution.ErrDenied
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	p.app.authMu.RLock()
	defer p.app.authMu.RUnlock()
	if p.app.workspaceMgr != p.workspaceMgr || p.app.currentAuthUser == nil || p.app.currentAuthUser.UserID != p.principal.UserID || p.app.currentAuthUser.SessionID != p.principal.SessionID || p.app.currentUserID != p.principal.UserID {
		return "", commandexecution.ErrDenied
	}
	snapshot, err := p.workspaceMgr.CommandSnapshot()
	if err != nil || snapshot.WorkspaceID != p.workspaceID || snapshot.Version == "" {
		return "", commandexecution.ErrStale
	}
	return snapshot.Version, nil
}

func containsOriginField(fields []commandbindings.Field, want commandbindings.Field) bool {
	for _, field := range fields {
		if field == want {
			return true
		}
	}
	return false
}

func containsPhysicalOriginField(fields []commandbindings.Field) bool {
	return containsOriginField(fields, commandbindings.Process) || containsOriginField(fields, commandbindings.Device)
}

func filterOriginFields(fields []commandbindings.Field, omit commandbindings.Field) []commandbindings.Field {
	result := make([]commandbindings.Field, 0, len(fields))
	for _, field := range fields {
		if field != omit {
			result = append(result, field)
		}
	}
	return result
}

func foregroundSummaryRaw(snapshot *commandforeground.Snapshot) (*json.RawMessage, error) {
	if snapshot == nil {
		return nil, nil
	}
	raw, err := json.Marshal(snapshot.Summary)
	if err != nil {
		return nil, err
	}
	value := json.RawMessage(raw)
	return &value, nil
}

// commandWorkspaceProvider liga o Manager real a UMA sessão desktop. Não usa
// IDs recebidos de UI como ownership, nem infere uma surface da aba ativa.
// A fábrica é interna; a publicação do bus/executor pertence ao bootstrap.
type commandWorkspaceProvider struct {
	// manager e principal são capturados juntos sob authMu. Snapshot mantém
	// authMu até terminar a leitura autoritativa, impedindo logout/troca de
	// sessão entre a validação do owner e o CommandSnapshot.
	app       *App
	manager   *workspace.Manager
	principal auth.LocalSessionPrincipal
}

func (a *App) newCommandWorkspaceProvider(principal auth.LocalSessionPrincipal) (commandcontext.ScopedProvider, error) {
	if a == nil {
		return nil, commandcontext.ErrProviderUnavailable
	}
	if err := validateCommandPrincipal(principal); err != nil {
		return nil, err
	}

	a.authMu.RLock()
	defer a.authMu.RUnlock()
	if a.workspaceMgr == nil || a.currentAuthUser == nil ||
		a.currentUserID != principal.UserID || a.currentAuthUser.UserID != principal.UserID || a.currentAuthUser.SessionID != principal.SessionID {
		return nil, commandcontext.ErrOwnerMismatch
	}
	return &commandWorkspaceProvider{app: a, manager: a.workspaceMgr, principal: principal}, nil
}

func (p *commandWorkspaceProvider) Snapshot(ctx context.Context, scope commandcontext.Scope, fact string) (commandcontext.OwnedSnapshot, error) {
	if p == nil || p.app == nil || p.manager == nil || ctx == nil {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
	}
	if err := ctx.Err(); err != nil {
		return commandcontext.OwnedSnapshot{}, err
	}
	if err := scope.Validate(); err != nil {
		return commandcontext.OwnedSnapshot{}, err
	}
	if scope.UserID != p.principal.UserID || scope.AuthContextID != p.principal.SessionID || scope.WorkspaceID == nil {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrOwnerMismatch
	}

	p.app.authMu.RLock()
	defer p.app.authMu.RUnlock()
	current := p.app.currentAuthUser
	if p.app.workspaceMgr != p.manager || current == nil || current.UserID != p.principal.UserID || current.SessionID != p.principal.SessionID || p.app.currentUserID != p.principal.UserID {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrOwnerMismatch
	}

	// Este instante pertence à consulta da fonte, nunca ao payload de ingresso.
	capturedAt := time.Now().UTC()
	snapshot, err := p.manager.CommandSnapshot()
	if err != nil {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
	}
	if snapshot.WorkspaceID != *scope.WorkspaceID {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrOwnerMismatch
	}
	version := ""
	switch fact {
	case "workspace", "active_tab":
		version = snapshot.Version
	case "profile":
		if snapshot.WorkspaceProfile == "" && snapshot.Tab.ProfileOverrideSlug == "" {
			return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
		}
		version = snapshot.Version
	default:
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
	}
	if err := ctx.Err(); err != nil {
		return commandcontext.OwnedSnapshot{}, err
	}
	return commandcontext.NewOwnedSnapshot(scope, commandcontext.Snapshot{Version: version, CapturedAt: capturedAt})
}

// commandForegroundProvider liga a captura nativa à mesma sessão que criou o
// bus. O scopeBind é privado e fecha sobre o principal; não há como o
// chamador trocar o principal do provider depois da construção.
type commandForegroundProvider struct {
	app       *App
	reader    commandforeground.Reader
	scopeBind func(commandcontext.Scope) bool
}

func (a *App) newCommandForegroundProvider(principal auth.LocalSessionPrincipal) (commandcontext.ScopedProvider, error) {
	if a == nil {
		return nil, commandcontext.ErrProviderUnavailable
	}
	if err := validateCommandPrincipal(principal); err != nil {
		return nil, err
	}

	a.authMu.RLock()
	defer a.authMu.RUnlock()
	if a.currentAuthUser == nil || a.currentUserID != principal.UserID ||
		a.currentAuthUser.UserID != principal.UserID || a.currentAuthUser.SessionID != principal.SessionID {
		return nil, commandcontext.ErrOwnerMismatch
	}
	return &commandForegroundProvider{
		app:       a,
		reader:    commandforeground.NewNative(),
		scopeBind: commandForegroundScopeBind(principal),
	}, nil
}

func commandForegroundScopeBind(principal auth.LocalSessionPrincipal) func(commandcontext.Scope) bool {
	return func(scope commandcontext.Scope) bool {
		return scope.UserID == principal.UserID &&
			scope.AuthContextID == principal.SessionID
	}
}

func (p *commandForegroundProvider) Snapshot(ctx context.Context, scope commandcontext.Scope, fact string) (commandcontext.OwnedSnapshot, error) {
	if p == nil || p.app == nil || p.reader == nil || p.scopeBind == nil || ctx == nil {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
	}
	if fact != "foreground" {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
	}
	if err := ctx.Err(); err != nil {
		return commandcontext.OwnedSnapshot{}, err
	}
	if err := scope.Validate(); err != nil {
		return commandcontext.OwnedSnapshot{}, err
	}
	if !p.scopeBind(scope) {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrOwnerMismatch
	}

	// O guard permanece durante a captura: logout/troca de sessão não pode
	// ocorrer entre a validação do principal e a observação nativa.
	p.app.authMu.RLock()
	defer p.app.authMu.RUnlock()
	current := p.app.currentAuthUser
	if current == nil || p.app.currentUserID != scope.UserID ||
		current.UserID != scope.UserID || current.SessionID != scope.AuthContextID {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrOwnerMismatch
	}

	nativeSnapshot, err := p.reader.Capture(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return commandcontext.OwnedSnapshot{}, ctxErr
		}
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
	}
	if nativeSnapshot.CapturedAt.IsZero() || strings.TrimSpace(nativeSnapshot.Version) == "" {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
	}
	if err := ctx.Err(); err != nil {
		return commandcontext.OwnedSnapshot{}, err
	}
	return commandcontext.NewOwnedSnapshot(scope, commandcontext.Snapshot{
		Version:    nativeSnapshot.Version,
		CapturedAt: nativeSnapshot.CapturedAt,
	})
}

// newCommandFactBus publica somente fontes autoritativas locais do backend.
// Fatos visuais (surface/dialog/focus) pertencem à revalidação síncrona da UI
// imediatamente antes do efeito visual; nunca são consultados pelo FactBus
// sob o DispatchGate.
func (a *App) newCommandFactBus(principal auth.LocalSessionPrincipal) (*commandcontext.FactBus, error) {
	workspaceProvider, err := a.newCommandWorkspaceProvider(principal)
	if err != nil {
		return nil, err
	}
	foregroundProvider, err := a.newCommandForegroundProvider(principal)
	if err != nil {
		return nil, err
	}
	providers := map[string]commandcontext.ScopedProvider{
		"workspace":  workspaceProvider,
		"foreground": foregroundProvider,
		// Profile é derivado pelo WorkspaceManager, não pela UI.
		"profile": workspaceProvider,
	}
	return commandcontext.NewFactBus(providers)
}

func validateCommandPrincipal(principal auth.LocalSessionPrincipal) error {
	scope := commandcontext.Scope{UserID: principal.UserID, AuthContextID: principal.SessionID}
	if err := scope.Validate(); err != nil {
		return commandcontext.ErrOwnerMismatch
	}
	return nil
}
