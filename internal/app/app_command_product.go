package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandbridge"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandcontext"
	"assistente/internal/commandcontract"
	"assistente/internal/commanddeck"
	"assistente/internal/commandexecution"
	"assistente/internal/commandforeground"
	"assistente/internal/commandledger"
	"assistente/internal/commandruntime"
	"assistente/internal/commandsecurity"
	"assistente/internal/commandui"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

type commandProductRuntime struct {
	app                     *App
	principal               auth.LocalSessionPrincipal
	sessionSvc              *auth.SessionService
	credMgr                 *credentials.Manager
	workspaceMgr            *workspace.Manager
	epochs                  *commandsecurity.EpochService
	host                    *commandexecution.HostState
	facts                   *commandcontext.FactBus
	workspaceID             string
	service                 *commandexecution.Service
	agentConfig             commandexecution.Config
	keyboardService         *commandexecution.Service
	globalExecution         *commandGlobalExecution
	deckExecution           *commandDeckExecution
	deckCancel              context.CancelFunc
	manualExpiryCancel      context.CancelFunc
	manualExpiryWake        chan struct{}
	manualAuthorityEpoch    commandsecurity.EpochSnapshot
	manualAuthorityWatch    context.Context
	manualAuthorityRelease  func()
	deckLocale              string
	deckDriver              commanddeck.Driver
	deckCapture             *commandDeckCapture
	deckHeld                map[string]bool
	deckDown                map[string]bool
	deckCaptureGeneration   uint64
	deckPresentedStates     map[string]string
	keyboardMu              sync.Mutex
	keyboardMap             *localCommandKeyboardState
	keyboardEvents          map[string]localCommandKeyboardOccurrence
	registry                *commandcatalog.Registry
	resolutionMu            sync.Mutex
	resolutionConfiguration *commandbindings.Configuration
	resolutionCache         *commandcontext.ResolutionCache
	resolutionStopped       bool
	persistedConfigMu       sync.RWMutex
	persistedConfigStore    *commandconfig.Store
	persistedConfigSnapshot commandconfig.Snapshot
	hasPersistedSnapshot    bool
	bridge                  *commandbridge.Bridge
	mu                      sync.Mutex
	projectionMu            sync.Mutex
	pending                 map[string]context.CancelFunc
	ui                      *commandui.Broker
	uiRuns                  map[string]*commandUIRun
	closed                  bool
	workers                 sync.WaitGroup
	closeOnce               sync.Once
	done                    chan struct{}
	capability              commandbridge.Capability
	foregroundReader        commandforeground.Reader
}

// CommandExecutionResult mantém o histórico redigido. Output, quando presente,
// é um DTO efêmero da execução iniciada nesta chamada, nunca da consulta.
// Tokens, fingerprints e ownership interno não atravessam a fachada.
type CommandExecutionResult struct {
	InvocationID  string         `json:"invocationId"`
	Status        string         `json:"status"`
	ResultSummary *string        `json:"resultSummary,omitempty"`
	ErrorCode     *string        `json:"errorCode,omitempty"`
	Output        *CommandOutput `json:"output,omitempty"`
}

func commandProductResult(record commandledger.FullRecord) CommandExecutionResult {
	return CommandExecutionResult{InvocationID: record.InvocationID, Status: string(record.Status), ResultSummary: record.ResultSummary, ErrorCode: record.ErrorCode}
}

// ExecutePaletteCommand fixa a origem e cria IDs na borda Wails. A seleção
// passa pela configuração publicada; o chamador não escolhe o binding vencedor.
func (a *App) ExecutePaletteCommand(commandID string, arguments json.RawMessage) (CommandExecutionResult, error) {
	if a == nil {
		return CommandExecutionResult{}, commandexecution.ErrDenied
	}
	p := a.commandProduct.Load()
	if p == nil {
		return CommandExecutionResult{}, commandruntime.ErrNotReady
	}
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	if definition, ok := p.registry.Lookup(commandID); ok && commandExecutionClassForDefinition(definition) != commandExecutionDurable {
		return CommandExecutionResult{}, commandexecution.ErrDenied
	}
	if !p.dependenciesMatch(a) || a.commandProduct.Load() != p {
		return CommandExecutionResult{}, commandexecution.ErrDenied
	}
	invocationID, err := uuid.NewV7()
	if err != nil {
		return CommandExecutionResult{}, err
	}
	correlationID, err := uuid.NewV7()
	if err != nil {
		return CommandExecutionResult{}, err
	}
	var candidate commandexecution.EnvelopeCandidate
	if isCommandToolExecutionID(commandID) {
		definition, ok := p.registry.Lookup(commandID)
		if !ok || definition.HandlerClassification != commandcatalog.HandlerTool || !definition.AllowsSource(commandcatalog.Palette) {
			return CommandExecutionResult{}, commandexecution.ErrDenied
		}
		canonical, validateErr := definition.ValidateArguments(arguments)
		if validateErr != nil {
			return CommandExecutionResult{}, commandexecution.ErrInvalidRequest
		}
		candidate = commandexecution.EnvelopeCandidate{InvocationID: invocationID.String(), CorrelationID: correlationID.String(), CommandID: commandID, Arguments: canonical}
	} else {
		candidate, err = commandPaletteCandidate(invocationID.String(), correlationID.String(), commandID, arguments)
		if err != nil {
			return CommandExecutionResult{}, err
		}
	}
	ctx := a.commandBridgeContext()
	if err := p.checkPersistedCommandConfiguration(ctx); err != nil {
		return CommandExecutionResult{}, err
	}
	if err := p.refreshCommandJobProjection(ctx); err != nil {
		return CommandExecutionResult{}, err
	}
	record, output, err := p.service.ExecuteEnvelopeWithResult(ctx, "", candidate)
	result := commandProductResult(record)
	if err != nil || record.Status != commandledger.Succeeded || len(output) == 0 {
		return result, err
	}
	if record.Envelope.CommandID == nil {
		return result, commandexecution.ErrExecution
	}
	return a.commandProductResultWithOutput(p, *record.Envelope.CommandID, record, output)
}

func (a *App) GetPaletteInvocation(id string) (CommandExecutionResult, error) {
	if a == nil {
		return CommandExecutionResult{}, commandexecution.ErrDenied
	}
	p := a.commandProduct.Load()
	if p == nil {
		return CommandExecutionResult{}, commandruntime.ErrNotReady
	}
	record, err := p.service.GetEnvelopeInvocation(a.commandBridgeContext(), "", id)
	return commandProductResult(record), err
}

func (p *commandProductRuntime) execute(ctx context.Context, candidate commandexecution.EnvelopeCandidate) (commandledger.FullRecord, error) {
	if p == nil || p.service == nil || p.app.commandProduct.Load() != p || !p.dependenciesMatch(p.app) {
		return commandledger.FullRecord{}, commandexecution.ErrDenied
	}
	if err := p.refreshCommandJobProjection(ctx); err != nil {
		return commandledger.FullRecord{}, err
	}
	return p.service.ExecuteEnvelope(ctx, "", candidate)
}

// Dispatch só admite trabalho; ledger, autorização e espera ficam no executor,
// fora do gate curto da bridge. A origem física continua indisponível aqui.
func (p *commandProductRuntime) Dispatch(ctx context.Context, in commandbridge.Invocation) (commandbridge.InvocationAck, error) {
	if in.Source != commandbridge.SourcePalette || in.SessionID != p.principal.SessionID || in.DialogProof != nil || !p.dependenciesMatch(p.app) {
		return commandbridge.InvocationAck{}, commandbridge.ErrCapabilityDenied
	}
	run, cancel := context.WithCancel(ctx)
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		cancel()
		return commandbridge.InvocationAck{}, commandbridge.ErrBridgeClosed
	}
	if _, exists := p.pending[in.InvocationID]; exists {
		p.mu.Unlock()
		cancel()
		return commandbridge.InvocationAck{}, commandbridge.ErrInvocationReplay
	}
	p.pending[in.InvocationID] = cancel
	p.workers.Add(1)
	p.mu.Unlock()
	go func() {
		defer p.workers.Done()
		defer cancel()
		defer func() { p.mu.Lock(); delete(p.pending, in.InvocationID); p.mu.Unlock() }()
		candidate, err := commandPaletteCandidate(in.InvocationID, in.InvocationID, in.CommandID, json.RawMessage(`{}`))
		var record commandledger.FullRecord
		if err == nil {
			record, err = p.execute(run, candidate)
		}
		status := commandbridge.ResultFailed
		if err == nil && record.Status == commandledger.Succeeded {
			status = commandbridge.ResultSucceeded
		}
		payload, _ := json.Marshal(commandProductResult(record))
		_, _ = p.bridge.AcceptResult(commandbridge.Result{
			SessionID: in.SessionID, InvocationID: in.InvocationID, CommandID: in.CommandID,
			Generation: in.Generation, CapabilityID: in.CapabilityID, Ownership: in.Ownership,
			OccurrenceID: in.OccurrenceID, SourceEventID: in.SourceEventID, EventID: in.EventID,
			DialogProof: in.DialogProof, Owner: p.owner(), Status: status, Payload: payload,
		})
	}()
	return commandbridge.InvocationAck{InvocationID: in.InvocationID, Accepted: true}, nil
}

func (p *commandProductRuntime) owner() commandbridge.Owner {
	return commandbridge.Owner{UserID: p.principal.UserID, SessionID: p.principal.SessionID, WorkspaceID: p.workspaceID}
}

func (p *commandProductRuntime) Cancel(_ context.Context, request commandbridge.CancelRequest) error {
	p.mu.Lock()
	cancel := p.pending[request.InvocationID]
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (p *commandProductRuntime) Shutdown(ctx context.Context) error {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		// Invalidate physical Deck generations before Wait begins. This closes
		// the Add-vs-Wait race for a durable Deck ingress concurrent with drain.
		p.deckCaptureGeneration++
		if p.manualAuthorityRelease != nil {
			p.manualAuthorityRelease()
		}
		if p.manualExpiryCancel != nil {
			p.manualExpiryCancel()
		}
		if p.deckCancel != nil {
			p.deckCancel()
		}
		if p.deckCapture != nil {
			p.deckCapture.cancel()
		}
		for _, cancel := range p.pending {
			cancel()
		}
		clear(p.uiRuns)
		p.mu.Unlock()
		p.app.clearExternalUIConnections()
		p.resolutionMu.Lock()
		p.resolutionStopped = true
		p.resolutionCache.Clear()
		p.resolutionConfiguration = nil
		p.resolutionMu.Unlock()
		// Retire native occurrences before waiting for any executor to drain.
		// A timeout in another source must not keep a global admission alive.
		if p.globalExecution != nil {
			p.globalExecution.cancel()
		}
		if p.ui != nil {
			p.ui.Close()
		}
		go func() { p.workers.Wait(); close(p.done) }()
	})
	p.clearLocalCommandKeyboard("")
	if p.deckExecution != nil && p.deckExecution.service != nil {
		if err := p.deckExecution.service.Shutdown(ctx); err != nil {
			return err
		}
	}
	if p.globalExecution != nil && p.globalExecution.service != nil {
		if err := p.globalExecution.service.Shutdown(ctx); err != nil {
			return err
		}
	}
	if p.keyboardService != nil {
		if err := p.keyboardService.Shutdown(ctx); err != nil {
			return err
		}
	}
	if p.ui != nil {
		if err := p.ui.Shutdown(ctx); err != nil {
			return err
		}
	}
	if err := p.service.Shutdown(ctx); err != nil {
		return err
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *App) mountCommandProduct(ctx context.Context) error {
	a.commandProductBuild.Lock()
	defer a.commandProductBuild.Unlock()
	principal, err := a.currentCommandPrincipal()
	if err != nil {
		return err
	}
	// commandLifecycleClosing é protegido pelo mesmo mutex da montagem. Esta
	// checagem precisa preceder inclusive o fast path de mesmo principal.
	a.commandLifecycleMount.Lock()
	if a.commandLifecycleClosing {
		a.commandLifecycleMount.Unlock()
		return commandruntime.ErrStopped
	}
	a.commandLifecycleMount.Unlock()
	if ctx == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	previous := a.commandProduct.Load()
	if previous != nil && previous.principal == principal && previous.dependenciesMatch(a) {
		return nil
	}
	epochs, err := a.commandSecurityService()
	if err != nil {
		return err
	}
	if err := epochs.BindInstance(ctx, database.DB()); err != nil {
		return fmt.Errorf("exclusão da instância de comandos: %w", err)
	}
	a.authMu.RLock()
	state := a.commandHost
	sessionSvc, credMgr, manager := a.sessionSvc, a.credMgr, a.workspaceMgr
	var activeWorkspaceID string
	if manager != nil {
		activeWorkspaceID = manager.ActiveID()
	}
	a.authMu.RUnlock()
	newHost := state == nil
	var mountEpoch commandsecurity.EpochSnapshot
	if state == nil {
		if a.vaultSvc == nil {
			return commandruntime.ErrMissingDependency
		}
		state, err = commandexecution.NewHostState(epochs, commandProductRegistryVersion)
		if err != nil {
			return err
		}
		if err = state.SetVaultUnlocked(ctx, true); err != nil {
			return err
		}
		mountEpoch, err = epochs.CaptureAuthenticated(ctx, func(context.Context) (string, string, error) {
			if !a.commandPrincipalMatches(sessionSvc, credMgr, principal) {
				return "", "", commandexecution.ErrDenied
			}
			return principal.UserID, principal.SessionID, nil
		})
		if err != nil {
			return err
		}
		vault, statusErr := a.vaultSvc.Status(ctx)
		if statusErr != nil {
			return statusErr
		}
		if !vault.Unlocked {
			return commandruntime.ErrNotReady
		}
	}
	registry, handlers, err := a.commandProductCatalog()
	if err != nil {
		return err
	}
	facts, err := a.newCommandFactBus(principal)
	if err != nil {
		return err
	}
	store, err := commandledger.New(database.DB(), time.Now)
	if err != nil {
		return err
	}
	allowedReadCommands := map[string][]string{
		commandProductWorkspaceListID: {database.UserRoleUser, database.UserRoleAdmin},
		commandProductShortcutsShowID: {database.UserRoleUser, database.UserRoleAdmin},
	}
	for _, navigation := range commandProductUINavigation {
		allowedReadCommands[navigation.id] = []string{database.UserRoleUser, database.UserRoleAdmin}
	}
	readPolicy, err := commandexecution.NewLocalReadAuthorizer(database.DB(), allowedReadCommands)
	if err != nil {
		return err
	}
	writePolicy := func(ctx context.Context, principal auth.LocalSessionPrincipal, commandID string, source commandcatalog.Source) error {
		if (!isWorkspaceMutationCommand(commandID) && !isAuditedUIContextualCommand(commandID) && !isCommandLayerAction(commandID) && !(isCommandToolExecutionID(commandID) && source == commandcatalog.Palette)) || (source != commandcatalog.Palette && source != commandcatalog.KeyboardLocal && source != commandcatalog.StreamDeck) || ctx == nil {
			return commandexecution.ErrDenied
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		var row struct {
			Role      string
			ExpiresAt time.Time
		}
		result := database.DB().WithContext(ctx).Table("sessions AS s").Select("u.role, s.expires_at").Joins("JOIN users AS u ON u.id = s.user_id").Where("s.id = ? AND s.user_id = ? AND s.revoked_at IS NULL AND s.expires_at > ? AND u.is_active = ?", principal.SessionID, principal.UserID, time.Now(), true).Take(&row)
		if result.Error != nil || !row.ExpiresAt.After(time.Now()) {
			return commandexecution.ErrDenied
		}
		if row.Role != database.UserRoleUser && row.Role != database.UserRoleAdmin {
			return commandexecution.ErrDenied
		}
		return nil
	}
	policy := func(ctx context.Context, principal auth.LocalSessionPrincipal, commandID string, source commandcatalog.Source) error {
		if isWorkspaceMutationCommand(commandID) || isAuditedUIContextualCommand(commandID) || isCommandLayerAction(commandID) || isCommandToolExecutionID(commandID) && source == commandcatalog.Palette {
			return writePolicy(ctx, principal, commandID, source)
		}
		return readPolicy(ctx, principal, commandID, source)
	}
	if manager == nil || activeWorkspaceID == "" {
		return commandexecution.ErrInvalidConfiguration
	}
	p := &commandProductRuntime{app: a, principal: principal, sessionSvc: sessionSvc, credMgr: credMgr, workspaceMgr: manager, epochs: epochs, host: state, facts: facts, workspaceID: activeWorkspaceID, pending: make(map[string]context.CancelFunc), done: make(chan struct{}), foregroundReader: commandforeground.NewNative()}
	p.ui, err = commandui.New(64, 45*time.Second)
	if err != nil {
		return err
	}
	p.uiRuns = make(map[string]*commandUIRun)
	p.registry = registry
	p.resolutionCache = commandcontext.MustNewResolutionCache(256)
	config := commandexecution.Config{Epochs: epochs, Store: store, Registry: registry, RegistryVersion: commandProductRegistryVersion, Source: commandcatalog.Palette, Authorize: policy, KeyVersion: "v1", Now: time.Now, Retention: 24 * time.Hour, ExecutionTimeout: 30 * time.Second, FinalizationTimeout: 5 * time.Second, Handlers: handlers}
	config.Envelope = &commandexecution.EnvelopeConfig{
		DecisionTTL: 5 * time.Minute,
		DecisionBody: func(d commandcatalog.Definition, envelope commandcontract.Envelope) (string, error) {
			if isCommandToolExecutionID(d.ID) {
				return commandToolDecisionBody(a, p, d, envelope)
			}
			if d.ID != commandConversationClearID && d.ID != commandMessageDeleteID && d.ID != commandTerminalSessionCloseID && !pageMutationDestructive(d.ID) {
				return "", commandexecution.ErrDenied
			}
			p.mu.Lock()
			var captured workspace.CommandSnapshot
			var targetTitle string
			for _, run := range p.uiRuns {
				if run.reservation.InvocationID == envelope.InvocationID {
					captured = run.snapshot
					if run.pageMutation != nil {
						targetTitle = run.pageMutation.targetTitle
					}
					break
				}
			}
			p.mu.Unlock()
			current, err := p.workspaceMgr.CommandSnapshot()
			if err != nil || captured.WorkspaceID == "" || current != captured {
				return "", commandexecution.ErrStale
			}
			locale := p.getDeckLocale()
			metadata := d.Presentation.Locales[locale]
			if metadata.Name == "" {
				metadata = d.Presentation.Locales["en"]
			}
			body := metadata.Name + "\n" + metadata.Description
			if pageMutationDestructive(d.ID) {
				body += "\n\n“" + targetTitle + "”"
			}
			return body, nil
		},
		Context: facts,
		Snapshot: func(ctx context.Context, owner auth.LocalSessionPrincipal, candidate commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
			v, err := state.Snapshot(ctx, owner)
			if err != nil {
				return commandcontract.Envelope{}, err
			}
			snapshot, err := CommandLifecycleSnapshot(a)
			if err != nil || snapshot.State != commandruntime.StateReady || !snapshot.Published || !v.Unlocked || a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
				return commandcontract.Envelope{}, commandruntime.ErrNotReady
			}
			envelope := commandcontract.Envelope{RegistryVersion: v.Registry, GlobalConfigGeneration: commandStringPointer(v.GlobalConfig), ActiveLayersGeneration: commandStringPointer(v.ActiveLayers)}
			if candidate.TriggerType != "" || isCommandToolExecutionID(candidate.CommandID) {
				// O host publica atomicamente a união global+workspace. Seu stamp
				// invalida ambos os escopos quando qualquer parte é reconstruída.
				envelope.WorkspaceID = commandStringPointer(p.workspaceID)
				envelope.WorkspaceConfigGeneration = commandStringPointer(v.GlobalConfig)
			}
			return envelope, nil
		},
		Resolve: p.resolvePersistedTrigger,
		Authorize: func(ctx context.Context, owner auth.LocalSessionPrincipal, _ commandcontract.Envelope, d commandcatalog.Definition) error {
			if !p.dependenciesMatch(a) {
				return commandexecution.ErrStale
			}
			err := policy(ctx, owner, d.ID, commandcatalog.Palette)
			if err != nil {
				return err
			}
			if !p.dependenciesMatch(a) {
				return commandexecution.ErrStale
			}
			return nil
		},
		AuthorizeLookup: func(ctx context.Context, owner auth.LocalSessionPrincipal, r commandledger.FullRecord) error {
			if !p.dependenciesMatch(a) || !commandRecordMatchesPrincipal(r, owner) {
				return commandexecution.ErrDenied
			}
			if r.Envelope.CommandID == nil {
				// Supressão/stale não criam auditoria. Sua origem e ownership
				// continuam disponíveis na chave do ledger, sem inventar envelope.
				if r.SourceType != nil && *r.SourceType == commandcontract.SourcePalette &&
					(r.Status == commandledger.Suppressed || r.Status == commandledger.RejectedStale) {
					return nil
				}
				if r.Envelope.SourceType != nil && *r.Envelope.SourceType == commandcontract.SourcePalette &&
					r.Envelope.TriggerType != nil && *r.Envelope.TriggerType == string(commandcatalog.Palette) &&
					r.Status == commandledger.Denied {
					return nil
				}
				return commandexecution.ErrDenied
			}
			err := policy(ctx, owner, *r.Envelope.CommandID, commandcatalog.Palette)
			if err != nil {
				return err
			}
			if !p.dependenciesMatch(a) {
				return commandexecution.ErrStale
			}
			return nil
		},
		Actor: func(_ context.Context, owner auth.LocalSessionPrincipal) (commandcontract.ActorType, string, error) {
			return commandcontract.ActorUser, owner.UserID, nil
		},
	}
	p.agentConfig = config
	p.service, err = a.newCommandDesktopExecutor(config, state)
	if err != nil {
		p.ui.Close()
		return err
	}
	p.keyboardService, err = a.newLocalCommandKeyboardExecutor(p, config, state, policy)
	if err != nil {
		p.ui.Close()
		closeUninstalledCommandService(p.service, config.FinalizationTimeout)
		return err
	}
	installed := false
	defer func() {
		if !installed {
			p.ui.Close()
			if p.deckExecution != nil {
				closeUninstalledCommandService(p.deckExecution.service, config.FinalizationTimeout)
			}
			if p.globalExecution != nil {
				p.globalExecution.cancel()
				closeUninstalledCommandService(p.globalExecution.service, config.FinalizationTimeout)
			}
			closeUninstalledCommandService(p.keyboardService, config.FinalizationTimeout)
			closeUninstalledCommandService(p.service, config.FinalizationTimeout)
		}
	}()
	p.globalExecution, err = a.newCommandGlobalExecutor(p, config, state)
	if err != nil {
		return err
	}
	deckRoles := make(map[string][]string)
	for _, definition := range registry.List() {
		if definition.AllowsSource(commandcatalog.StreamDeck) && (definition.HandlerClassification == commandcatalog.HandlerUI || isWorkspaceMutationCommand(definition.ID) || isCommandLayerAction(definition.ID)) {
			deckRoles[definition.ID] = []string{database.UserRoleUser, database.UserRoleAdmin}
		}
	}
	deckPolicy, err := commandexecution.NewLocalStreamDeckReadAuthorizer(database.DB(), deckRoles)
	if err != nil {
		return err
	}
	p.deckExecution, err = a.newCommandDeckExecutor(p, config, state, deckPolicy)
	if err != nil {
		return err
	}
	capability := commandbridge.Capability{ID: uuid.Must(uuid.NewV7()).String(), CommandID: commandProductWorkspaceListID, Generation: 1, Source: commandbridge.SourcePalette, Owner: p.owner()}
	p.capability = capability
	p.bridge, err = commandbridge.New(commandbridge.Config{Port: p, Capabilities: []commandbridge.Capability{capability}})
	if err != nil {
		return err
	}
	if err = p.bridge.OpenSession(commandbridge.Session{ID: principal.SessionID, Generation: 1, Owner: p.owner()}); err != nil {
		return err
	}
	publish := func() error {
		if _, ok := loadCommandLifecycle(a); !ok {
			if err := ConfigureCommandLifecycleForApp(a, CommandLifecycleMountInputs{Execution: config, Host: state, Bridge: p.bridge, Facts: facts, Adapter: p}); err != nil {
				return err
			}
		}
		a.commandLifecycleMount.Lock()
		defer a.commandLifecycleMount.Unlock()
		if a.commandLifecycleClosing {
			return commandruntime.ErrStopped
		}
		a.authMu.Lock()
		defer a.authMu.Unlock()
		if a.currentAuthUser == nil || a.currentAuthUser.UserID != principal.UserID || a.currentAuthUser.SessionID != principal.SessionID ||
			a.sessionSvc != sessionSvc || a.credMgr != credMgr || a.workspaceMgr != manager || a.commandHost != state {
			return commandexecution.ErrStale
		}
		a.commandRegistry = registry
		a.commandProduct.Store(p)
		a.commandBridge.Store(p.bridge)
		return nil
	}
	if newHost {
		err = epochs.Admit(ctx, mountEpoch, func(context.Context) error {
			if !a.commandPrincipalMatches(sessionSvc, credMgr, principal) {
				return commandexecution.ErrStale
			}
			a.authMu.RLock()
			defer a.authMu.RUnlock()
			if a.commandHost != nil || a.workspaceMgr != manager || a.sessionSvc != sessionSvc || a.credMgr != credMgr {
				return commandexecution.ErrStale
			}
			return nil
		}, publish)
	} else {
		err = publish()
	}
	if err != nil {
		return err
	}
	installed = true
	a.wireCommandCatalog()
	if previous != nil {
		if err := previous.bridge.Shutdown(ctx); err != nil {
			return err
		}
	}
	// Only a running desktop owns hardware. Unit fixtures do not run Startup;
	// integration tests explicitly supply a controlled Driver to startDeck.
	if a.cancel != nil && a.chatDesktopIngress {
		p.startDeck(a.ctx, commanddeck.NewStreamDeckDriver())
	}
	return nil
}

// refreshCommandProductCatalog faz o hot-swap fail-closed dos snapshots de
// executor quando MCP publica adição/remoção/alteração de tools. A geração
// antiga é primeiro desabilitada e drenada; nenhuma autorização ou receipt
// passa para o novo catálogo.
func (a *App) refreshCommandProductCatalog(ctx context.Context) error {
	if a == nil || ctx == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	if err := a.lockCommandBootstrap(ctx); err != nil {
		return err
	}
	defer a.unlockCommandBootstrap()
	p := a.commandProduct.Load()
	if p == nil {
		return nil // mudança pré-login: o próximo mount lê o catálogo atual.
	}
	if !p.dependenciesMatch(a) {
		return commandexecution.ErrStale
	}
	lifecycle := a.commandLifecycle.Load()
	if lifecycle == nil {
		return commandruntime.ErrNotReady
	}
	if err := ResetCommandLifecycle(ctx, a, "tool_catalog_changed"); err != nil {
		return err
	}
	if err := p.Shutdown(ctx); err != nil {
		return err
	}
	if err := a.shutdownMountedCommandLifecycle(ctx, lifecycle); err != nil {
		return err
	}
	if p.bridge != nil {
		if err := p.bridge.Shutdown(ctx); err != nil {
			return err
		}
	}
	a.commandLifecycleMount.Lock()
	a.authMu.Lock()
	if a.commandProduct.Load() != p || a.commandLifecycle.Load() != nil {
		a.authMu.Unlock()
		a.commandLifecycleMount.Unlock()
		return commandexecution.ErrStale
	}
	a.commandProduct.Store(nil)
	a.commandBridge.Store(nil)
	a.commandRegistry = nil
	a.authMu.Unlock()
	a.commandLifecycleMount.Unlock()
	if err := a.mountCommandProduct(ctx); err != nil {
		return err
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		return err
	}
	return BootstrapCommandLifecycle(ctx, a)
}

func (p *commandProductRuntime) dependenciesMatch(a *App) bool {
	if p == nil || a == nil {
		return false
	}
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	if a.sessionSvc != p.sessionSvc || a.credMgr != p.credMgr ||
		a.workspaceMgr != p.workspaceMgr || a.commandHost != p.host ||
		a.commandEpochs != p.epochs {
		return false
	}
	if a.currentUserID != p.principal.UserID || a.currentAuthUser == nil ||
		a.currentAuthUser.UserID != p.principal.UserID ||
		a.currentAuthUser.SessionID != p.principal.SessionID {
		return false
	}
	return p.workspaceMgr.ActiveID() == p.workspaceID
}
