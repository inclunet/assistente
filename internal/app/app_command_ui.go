package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"assistente/controllers"
	"assistente/internal/chat"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontext"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/commandruntime"
	"assistente/internal/commandsecurity"
	"assistente/internal/commandui"
	"assistente/internal/terminal"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

// As reservas são transitórias e limitadas. O ledger do executor é a fonte
// persistente do desfecho; este mapa apenas correlaciona o transporte Wails.
type commandUIRun struct {
	paletteContext             *localCommandKeyboardContextProof
	deckPage                   bool
	pageMutation               *commandPageMutationPreparation
	reservation                commandui.Reservation
	done                       chan struct{}
	cancel                     context.CancelFunc
	expiresAt                  time.Time
	result                     CommandExecutionResult
	err                        error
	admission                  *commandUIAdmission
	sourceValid                func() bool
	contextual                 bool
	snapshot                   workspace.CommandSnapshot
	conversationFingerprint    string
	chatGeneration             uint64
	chatStreamMgr              *chat.StreamingManager
	chatController             *controllers.ChatController
	messagePreparation         *chatMessagePreparation
	terminalMgr                *terminal.Manager
	terminalInterrupt          terminal.InterruptSnapshot
	terminalClose              terminal.CloseSnapshot
	terminalSessionID          string
	terminalPreparationReady   chan struct{}
	terminalPreparationClaimed bool
	terminalPrepared           bool
	editorFilePreparing        bool
	editorFile                 *editorFilePreparation
}

type commandUIAdmission struct {
	epoch    commandsecurity.EpochSnapshot
	versions commandexecution.Versions
}

func (p *commandProductRuntime) uiOwner() commandui.Owner {
	return commandui.Owner{UserID: p.principal.UserID, SessionID: p.principal.SessionID, WorkspaceID: p.workspaceID}
}

func commandUIRunTimeout(commandID string) time.Duration {
	if isPageMutationCommand(commandID) {
		return 5 * time.Minute
	}
	if isEditorFileCommand(commandID) || isEditorFormatDialogCommand(commandID) || commandID == commandConversationClearID || commandID == commandMessageDeleteID || commandID == commandTerminalSessionCloseID {
		return 5 * time.Minute
	}
	return 30 * time.Second
}

func commandUITakeTimeout(commandID string) time.Duration {
	if pageMutationDestructive(commandID) {
		return 5 * time.Minute
	}
	if commandID == commandConversationClearID || commandID == commandMessageDeleteID || commandID == commandTerminalSessionCloseID {
		return 5 * time.Minute
	}
	return 35 * time.Second
}

func commandUIResultTTL(commandID string) time.Duration {
	if isPageMutationCommand(commandID) {
		return 6 * time.Minute
	}
	if isEditorFileCommand(commandID) || isEditorFormatDialogCommand(commandID) || commandID == commandConversationClearID || commandID == commandMessageDeleteID || commandID == commandTerminalSessionCloseID {
		// A consulta do desfecho precisa sobreviver ao prazo total do diálogo.
		return commandUIRunTimeout(commandID) + time.Minute
	}
	return time.Minute
}

// A identidade nunca é recebida da UI. Até respostas e cancelamentos exigem
// a sessão local ainda válida e a mesma composição publicada pelo App.
func (a *App) authenticatedCommandProduct() (*commandProductRuntime, error) {
	if a == nil {
		return nil, commandexecution.ErrDenied
	}
	p := a.commandProduct.Load()
	if p == nil || !p.dependenciesMatch(a) {
		return nil, commandruntime.ErrNotReady
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil || principal != p.principal {
		return nil, commandexecution.ErrDenied
	}
	current, err := p.sessionSvc.RevalidateLocalSession(a.commandBridgeContext(), principal)
	if err != nil || current != principal || !a.commandPrincipalMatches(p.sessionSvc, p.credMgr, principal) {
		return nil, commandexecution.ErrDenied
	}
	if err := p.refreshCommandJobProjection(a.commandBridgeContext()); err != nil {
		return nil, err
	}
	state, err := p.host.Snapshot(a.commandBridgeContext(), principal)
	if err != nil || !state.Unlocked {
		return nil, commandruntime.ErrNotReady
	}
	snapshot, err := CommandLifecycleSnapshot(a)
	if err != nil || snapshot.State != commandruntime.StateReady || !snapshot.Published || a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
		return nil, commandruntime.ErrNotReady
	}
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return nil, commandruntime.ErrStopped
	}
	return p, nil
}

// BeginUICommand só confirma recepção. A execução continua no serviço único:
// autenticação, autorização, ledger e CAS precedem Start e qualquer handoff.
func (a *App) BeginUICommand(commandID string) (commandui.Reservation, error) {
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return commandui.Reservation{}, err
	}
	if p.ui == nil {
		return commandui.Reservation{}, commandruntime.ErrNotReady
	}
	definition, ok := p.registry.Lookup(commandID)
	if !ok || commandExecutionClassForDefinition(definition) == commandExecutionLocalUI || (commandExecutionClassForDefinition(definition) != commandExecutionAuditedUI && !isWorkspaceMutationCommand(commandID)) || !definition.AllowsSource(commandcatalog.Palette) {
		return commandui.Reservation{}, commandexecution.ErrDenied
	}
	return a.beginCommandUI(p, commandID, func(reservation commandui.Reservation) (commandexecution.EnvelopeCandidate, error) {
		return commandPaletteCandidate(reservation.InvocationID, reservation.InvocationID, commandID, json.RawMessage(`{}`))
	}, p.execute, nil)
}

// Paleta e teclado compartilham reserva, lifetime e handoff. Apenas a fábrica
// hostside de candidato e o executor da origem diferem. Não ligar runCtx ao
// mapa DOM: a própria navegação invalida esse mapa antes de confirmar o efeito.
// A fonte é revalidada no Take, antes de entregar autoridade à UI.
func (a *App) beginCommandUI(p *commandProductRuntime, commandID string,
	prepare func(commandui.Reservation) (commandexecution.EnvelopeCandidate, error),
	execute func(context.Context, commandexecution.EnvelopeCandidate) (commandledger.FullRecord, error),
	sourceValid func() bool,
) (commandui.Reservation, error) {
	return a.beginCommandUIWithPaletteContext(p, commandID, prepare, execute, sourceValid, nil)
}

func (a *App) beginCommandUIWithPaletteContext(p *commandProductRuntime, commandID string,
	prepare func(commandui.Reservation) (commandexecution.EnvelopeCandidate, error),
	execute func(context.Context, commandexecution.EnvelopeCandidate) (commandledger.FullRecord, error),
	sourceValid func() bool,
	paletteContext *localCommandKeyboardContextProof,
) (commandui.Reservation, error) {
	return a.beginCommandUIWithCleanup(p, commandID, prepare, execute, sourceValid, paletteContext, nil)
}

// Cleanup belongs to the reservation lifetime, including preparation failures
// that never reach execute. Synchronous admission errors remain caller-owned.
func (a *App) beginCommandUIWithCleanup(p *commandProductRuntime, commandID string,
	prepare func(commandui.Reservation) (commandexecution.EnvelopeCandidate, error),
	execute func(context.Context, commandexecution.EnvelopeCandidate) (commandledger.FullRecord, error),
	sourceValid func() bool,
	paletteContext *localCommandKeyboardContextProof,
	cleanup func(),
) (commandui.Reservation, error) {
	if p == nil || p.ui == nil || prepare == nil || execute == nil {
		return commandui.Reservation{}, commandexecution.ErrDenied
	}
	if (commandID == commandWorkspaceTabTerminalCreateID || commandID == commandTerminalSessionCreateID || commandID == commandTerminalSessionCloseID) && a.currentTerminalManager() == nil {
		return commandui.Reservation{}, commandruntime.ErrNotReady
	}
	var reservation commandui.Reservation
	var err error
	var clearSnapshot workspace.CommandSnapshot
	var clearFingerprint string
	var chatGeneration uint64
	var messageVersions commandexecution.Versions
	var terminalManager *terminal.Manager
	var terminalTarget terminal.InterruptSnapshot
	var terminalCloseTarget terminal.CloseSnapshot
	var terminalSessionID string
	var terminalPreparationReady chan struct{}
	if commandID == commandTerminalSessionCloseID {
		clearSnapshot, terminalManager, terminalCloseTarget, err = a.captureTerminalClose(a.commandBridgeContext(), p)
		if err != nil {
			return commandui.Reservation{}, err
		}
	} else if commandID == commandTerminalSessionCreateID {
		clearSnapshot, terminalManager, terminalSessionID, err = a.captureTerminalSessionCreate(a.commandBridgeContext(), p)
		if err != nil {
			return commandui.Reservation{}, err
		}
	}
	if commandID == commandTerminalSessionCreateID || commandID == commandTerminalSessionCloseID {
		terminalPreparationReady = make(chan struct{})
	}
	if isPageMutationCommand(commandID) {
		a.authMu.RLock()
		available := a.taskSvc != nil
		if isProfileMutationCommand(commandID) {
			available = a.profileManager != nil && a.profilesCtrl != nil
		}
		a.authMu.RUnlock()
		if !available {
			return commandui.Reservation{}, commandexecution.ErrDenied
		}
		clearSnapshot, err = p.workspaceMgr.CommandSnapshot()
		if err != nil {
			return commandui.Reservation{}, err
		}
		messageVersions, err = p.host.Snapshot(a.commandBridgeContext(), p.principal)
		if err != nil {
			return commandui.Reservation{}, err
		}
	}
	if commandID == commandTerminalInterruptID {
		clearSnapshot, terminalManager, terminalTarget, err = a.captureTerminalInterrupt(a.commandBridgeContext(), p)
		if err != nil {
			return commandui.Reservation{}, err
		}
	}
	if isChatMessageCommand(commandID) {
		clearSnapshot, err = a.captureChatMessageTarget(a.commandBridgeContext(), p)
		if err != nil {
			return commandui.Reservation{}, err
		}
		messageVersions, err = p.host.Snapshot(a.commandBridgeContext(), p.principal)
		if err != nil {
			return commandui.Reservation{}, err
		}
	}
	if isChatActionCommand(commandID) {
		clearSnapshot, chatGeneration, err = a.captureChatAction(a.commandBridgeContext(), p, commandID)
		if err != nil {
			return commandui.Reservation{}, err
		}
	}
	if commandID == commandConversationClearID {
		clearSnapshot, clearFingerprint, err = a.captureConversationClear(a.commandBridgeContext(), p)
		if err != nil {
			return commandui.Reservation{}, err
		}
	}
	if isPageMutationCommand(commandID) || isEditorFileCommand(commandID) || isEditorFormatDialogCommand(commandID) || commandID == commandConversationClearID || commandID == commandMessageDeleteID || commandID == commandTerminalSessionCloseID {
		reservation, err = p.ui.ReserveWithTTL(p.uiOwner(), commandID, 5*time.Minute)
	} else {
		reservation, err = p.ui.Reserve(p.uiOwner(), commandID)
	}
	if err != nil {
		return commandui.Reservation{}, err
	}
	candidate, err := prepare(reservation)
	if err != nil {
		_ = p.ui.Cancel(p.uiOwner(), reservation.Ticket)
		return commandui.Reservation{}, err
	}
	runTimeout := commandUIRunTimeout(commandID)
	runCtx, cancel := context.WithTimeout(a.commandBridgeContext(), runTimeout)
	deckOccurrence, _ := commandDeckOccurrenceFor(p, candidate.InvocationID)
	run := &commandUIRun{
		deckPage:                candidate.TriggerType == string(commandcatalog.StreamDeck) && deckOccurrence.page,
		paletteContext:          paletteContext,
		reservation:             reservation,
		done:                    make(chan struct{}),
		cancel:                  cancel,
		expiresAt:               time.Now().Add(commandUIResultTTL(commandID)),
		sourceValid:             sourceValid,
		contextual:              isWorkspaceMutationCommand(commandID),
		snapshot:                clearSnapshot,
		conversationFingerprint: clearFingerprint,
		chatGeneration:          chatGeneration,
		chatStreamMgr:           a.streamMgr,
		chatController:          a.chatCtrl,
		terminalMgr:             terminalManager,
		terminalInterrupt:       terminalTarget,
		terminalClose:           terminalCloseTarget,
		terminalSessionID: func() string {
			if terminalSessionID != "" {
				return terminalSessionID
			}
			return terminalCloseTarget.SessionID()
		}(),
		terminalPreparationReady: terminalPreparationReady,
	}
	if isChatMessageCommand(commandID) {
		a.authMu.RLock()
		run.messagePreparation = &chatMessagePreparation{ready: make(chan struct{}), ctx: runCtx, versions: messageVersions, controller: a.conversationsCtrl}
		a.authMu.RUnlock()
	}
	if isPageMutationCommand(commandID) {
		a.authMu.RLock()
		run.pageMutation = &commandPageMutationPreparation{ready: make(chan struct{}), ctx: runCtx, versions: messageVersions, taskService: a.taskSvc}
		if isProfileMutationCommand(commandID) {
			run.pageMutation.profileManager, run.pageMutation.profileController = a.profileManager, a.profilesCtrl
			run.pageMutation.ownership, _ = commandexecution.NewCommitOwnership(35 * time.Second)
		}
		a.authMu.RUnlock()
	}
	p.mu.Lock()
	p.expireUIResultsLocked(time.Now())
	if p.closed || len(p.uiRuns) >= 64 {
		p.mu.Unlock()
		cancel()
		_ = p.ui.Cancel(p.uiOwner(), reservation.Ticket)
		return commandui.Reservation{}, commandruntime.ErrNotReady
	}
	p.uiRuns[reservation.Ticket] = run
	p.pending[reservation.InvocationID] = cancel
	p.workers.Add(1)
	p.mu.Unlock()
	go func() {
		defer p.workers.Done()
		defer cancel()
		var record commandledger.FullRecord
		var executeErr error
		if prep := run.pageMutation; prep != nil {
			select {
			case <-prep.ready:
				executeErr = a.validatePageMutationOrigin(runCtx, p, run)
			case <-runCtx.Done():
				executeErr = runCtx.Err()
			}
		}
		if prep := run.messagePreparation; prep != nil {
			select {
			case <-prep.ready:
				executeErr = a.validatePreparedChatMessage(runCtx, p, run)
			case <-runCtx.Done():
				executeErr = runCtx.Err()
			}
		}
		if prep := run.terminalPreparationReady; prep != nil && executeErr == nil {
			select {
			case <-prep:
				p.mu.Lock()
				if !run.terminalPrepared {
					executeErr = commandexecution.ErrStale
				}
				p.mu.Unlock()
			case <-runCtx.Done():
				executeErr = runCtx.Err()
			}
		}
		if executeErr == nil {
			record, executeErr = execute(runCtx, candidate)
		}
		run.result, run.err = commandProductResult(record), executeErr
		p.mu.Lock()
		if run.pageMutation != nil {
			run.pageMutation.mutation = nil
			run.pageMutation.profileMutation = nil
		}
		if prep := run.messagePreparation; prep != nil {
			prep.editedContent = nil
		}
		delete(p.pending, reservation.InvocationID)
		p.mu.Unlock()
		if isProfileMutationCommand(commandID) {
			a.rebuildAfterProfileCommand(p, run)
		}
		if cleanup != nil {
			cleanup()
		}
		close(run.done)
		// Wake a Take waiting for an admission that was refused before Start.
		_ = p.ui.Cancel(p.uiOwner(), reservation.Ticket)
	}()
	return reservation, nil
}

// Retenção limitada com descarte preguiçoso: consultas expiradas são recusadas
// e novas reservas liberam as posições vencidas. Não há timer/goroutine por
// resultado nem tarefa de limpeza que sobreviva ao shutdown do runtime.
func (p *commandProductRuntime) expireUIResultsLocked(now time.Time) {
	for ticket, run := range p.uiRuns {
		if !now.Before(run.expiresAt) {
			run.cancel()
			delete(p.uiRuns, ticket)
		}
	}
}

func (a *App) commandUIRun(ticket string) (*commandProductRuntime, *commandUIRun, error) {
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return nil, nil, err
	}
	if p.ui == nil {
		return nil, nil, commandruntime.ErrNotReady
	}
	p.mu.Lock()
	p.expireUIResultsLocked(time.Now())
	run := p.uiRuns[ticket]
	p.mu.Unlock()
	if run == nil {
		return nil, nil, commandexecution.ErrDenied
	}
	return p, run, nil
}

// TakeUICommand aguarda fora do DispatchGate e consome o handoff uma única vez.
// O ticket só localiza uma reserva; não substitui a autenticação acima.
func (a *App) TakeUICommand(ticket string) (commandui.Handoff, error) {
	p, run, err := a.commandUIRun(ticket)
	if err != nil {
		return commandui.Handoff{}, err
	}
	if run.sourceValid != nil && !run.sourceValid() {
		run.cancel()
		_ = p.ui.Cancel(p.uiOwner(), ticket)
		return commandui.Handoff{}, commandexecution.ErrStale
	}
	ctx, cancel := context.WithTimeout(a.commandBridgeContext(), commandUITakeTimeout(run.reservation.CommandID))
	defer cancel()
	return p.ui.TakeValidated(ctx, p.uiOwner(), ticket, func() error {
		if isChatMessageCommand(run.reservation.CommandID) {
			if err := a.validatePreparedChatMessage(ctx, p, run); err != nil {
				return err
			}
		}
		if _, err := a.authenticatedCommandProduct(); err != nil {
			return err
		}
		p.mu.Lock()
		admission := run.admission
		p.mu.Unlock()
		if admission == nil {
			return commandexecution.ErrDenied
		}
		// A espera pelo Take ocorreu fora do gate. Antes de entregar à UI, provar
		// que a configuração/geração admitida ainda é a atual; nunca retarget.
		return p.epochs.Admit(ctx, admission.epoch, func(ctx context.Context) error {
			if run.sourceValid != nil && !run.sourceValid() {
				return commandexecution.ErrStale
			}
			versions, err := p.host.Snapshot(ctx, p.principal)
			if err != nil || versions != admission.versions || a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
				return commandexecution.ErrStale
			}
			return nil
		}, func() error { return nil })
	})
}

func (a *App) CompleteUICommand(ticket, handoffID, status string) error {
	p, run, err := a.commandUIRun(ticket)
	if err != nil {
		return err
	}
	if run.reservation.CommandID == commandMessageSendEditorID {
		return a.completeChatEditorCommand(p, run, ticket, handoffID, status)
	}
	if run.contextual && status != string(commandledger.Cancelled) {
		return commandui.ErrCommitRequired
	}
	return p.ui.Complete(p.uiOwner(), ticket, handoffID, status)
}

// CommitWorkspaceTabCommand é a porta de transporte já publicada para escritas
// contextuais de workspace/abas. O nome do transporte não seleciona a operação:
// somente o comando da reserva admitida pode fazê-lo. O broker marca committing
// antes da callback e só publica succeeded após a persistência real.
func (a *App) CommitWorkspaceTabCommand(ticket, handoffID string) error {
	return a.commitWorkspaceTabCommand(ticket, handoffID)
}

func (a *App) commitWorkspaceTabCommand(ticket, handoffID string) error {
	p, run, err := a.commandUIRun(ticket)
	if err != nil {
		return err
	}
	if isPageMutationCommand(run.reservation.CommandID) {
		return a.commitPageMutationCommand(ticket, handoffID)
	}
	if run.reservation.CommandID == commandChatCancelID {
		return a.commitChatCancel(ticket, handoffID)
	}
	tabType, isCreate := workspaceTabTypeForCommand(run.reservation.CommandID)
	editorMode, isEditorMode := editorModeForCommand(run.reservation.CommandID)
	isTerminalCreate := run.reservation.CommandID == commandWorkspaceTabTerminalCreateID
	isTerminalSessionCreate := run.reservation.CommandID == commandTerminalSessionCreateID
	isTerminalSessionClose := run.reservation.CommandID == commandTerminalSessionCloseID
	isClose := run.reservation.CommandID == commandWorkspaceTabCloseID
	isWorkspaceCreate := run.reservation.CommandID == commandWorkspaceCreateID
	isContextChat := run.reservation.CommandID == commandWorkspaceChatOpenID
	isClear := run.reservation.CommandID == commandConversationClearID
	isTerminalInterrupt := run.reservation.CommandID == commandTerminalInterruptID
	if !run.contextual || (!isCreate && !isClose && !isWorkspaceCreate && !isContextChat && !isEditorMode && !isClear && !isTerminalInterrupt && !isTerminalSessionCreate && !isTerminalSessionClose) {
		return commandexecution.ErrDenied
	}
	ctx, cancel := context.WithTimeout(a.commandBridgeContext(), 35*time.Second)
	defer cancel()
	return p.ui.Commit(ctx, p.uiOwner(), ticket, handoffID, func(ctx context.Context) error {
		current, err := a.authenticatedCommandProduct()
		if err != nil || current != p {
			return commandexecution.ErrDenied
		}
		p.mu.Lock()
		admission := run.admission
		expected := run.snapshot
		terminalPrepared := run.terminalPrepared
		p.mu.Unlock()
		if admission == nil || expected.WorkspaceID == "" {
			return commandexecution.ErrDenied
		}
		var executionCtx context.Context
		release, err := p.epochs.AdmitExecution(ctx, admission.epoch, func(ctx context.Context) error {
			if run.sourceValid != nil && !run.sourceValid() {
				return commandexecution.ErrStale
			}
			currentVersions, err := p.host.Snapshot(ctx, p.principal)
			if err != nil || currentVersions != admission.versions || !currentVersions.Unlocked || !p.dependenciesMatch(a) || a.commandProduct.Load() != p {
				return commandexecution.ErrStale
			}
			return nil
		}, func(runCtx context.Context) error {
			executionCtx = runCtx
			return nil
		})
		if err != nil {
			return err
		}
		defer release()
		var created *workspace.Workspace
		if isTerminalInterrupt {
			if !terminalPrepared {
				return commandexecution.ErrDenied
			}
			return a.commitTerminalInterrupt(executionCtx, p, run, expected)
		}
		if isTerminalSessionClose {
			if !terminalPrepared {
				return commandexecution.ErrDenied
			}
			return a.commitTerminalSessionClose(executionCtx, p, run, expected)
		}
		if isClear {
			return a.commitConversationClear(executionCtx, p, expected, run.conversationFingerprint)
		}
		if isContextChat {
			created, err = a.commitWorkspaceContextChat(executionCtx, p, expected)
			if err != nil {
				return err
			}
		} else if isTerminalCreate {
			id, err := uuid.NewV7()
			if err != nil {
				return err
			}
			created, err = a.commitTerminalWorkspaceTab(executionCtx, p, run, expected, id.String())
			if err != nil {
				return err
			}
		} else if isTerminalSessionCreate {
			if !terminalPrepared {
				return commandexecution.ErrDenied
			}
			created, err = a.commitTerminalSessionCreate(executionCtx, p, run, expected)
			if err != nil {
				return err
			}
		} else if isEditorMode {
			// O Manager valida o tipo editor e o CAS do snapshot. A verificação
			// de identidade abaixo impede que uma troca de sessão/manager faça
			// esta confirmação atingir outro owner antes da operação durável.
			a.authMu.RLock()
			if a.workspaceMgr != p.workspaceMgr || a.currentAuthUser == nil ||
				a.currentAuthUser.UserID != p.principal.UserID || a.currentAuthUser.SessionID != p.principal.SessionID {
				a.authMu.RUnlock()
				return commandexecution.ErrDenied
			}
			updated, err := p.workspaceMgr.SetEditorModeForCommand(executionCtx, expected, editorMode)
			a.authMu.RUnlock()
			if err != nil {
				return err
			}
			created = updated
		} else {
			legacyErr := func() error {
				a.authMu.RLock()
				defer a.authMu.RUnlock()
				if a.workspaceMgr != p.workspaceMgr || a.currentAuthUser == nil || a.currentAuthUser.UserID != p.principal.UserID || a.currentAuthUser.SessionID != p.principal.SessionID || executionCtx == nil {
					return commandexecution.ErrDenied
				}
				currentSnapshot, snapshotErr := p.workspaceMgr.CommandSnapshot()
				if snapshotErr != nil || currentSnapshot.WorkspaceID != expected.WorkspaceID || currentSnapshot.Version != expected.Version || currentSnapshot.ActiveTabID != expected.ActiveTabID {
					return commandexecution.ErrStale
				}
				if isWorkspaceCreate {
					// O snapshot protege a origem da solicitação. O resultado é um
					// workspace independente; nunca substitui o workspace ativo.
					created, err = p.workspaceMgr.CreateForCommand(executionCtx, expected, "Workspace "+time.Now().Format("2006-01-02 15-04-05"))
					if err != nil {
						return err
					}
				} else if isCreate {
					id, err := uuid.NewV7()
					if err != nil {
						return err
					}
					created, err = p.workspaceMgr.AddTabForCommand(executionCtx, expected, workspace.Tab{ID: id.String(), Type: tabType})
					if err != nil {
						return err
					}
				} else if isClose {
					id, err := uuid.NewV7()
					if err != nil {
						return err
					}
					created, err = p.workspaceMgr.CloseActiveTabForCommand(executionCtx, expected, id.String())
					if err != nil {
						return err
					}
				}
				return nil
			}()
			if legacyErr != nil {
				return legacyErr
			}
		}
		if a.emitter != nil {
			if isContextChat {
				if created != nil {
					a.emitter.Emit("workspace:conversation_bound", created)
				}
			} else if isWorkspaceCreate {
				a.emitter.Emit("workspace:created", created)
			} else if isCreate {
				a.emitter.Emit("workspace:tab_added", created)
			} else if isTerminalSessionCreate {
				a.emitter.Emit("workspace:terminal_session_bound", created)
			} else if isClose {
				a.emitter.Emit("workspace:tab_removed", created)
			} else if isEditorMode {
				a.emitter.Emit("workspace:editor_mode_changed", workspaceEditorModeChangedEvent{
					Workspace: created, TabID: expected.ActiveTabID, Mode: editorMode,
				})
			}
		}
		return nil
	})
}

func (a *App) CancelUICommand(ticket string) error {
	p, run, err := a.commandUIRun(ticket)
	if err != nil {
		return err
	}
	if run.reservation.CommandID == commandMessageSendEditorID {
		if editor := chatEditorForRun(p, run); editor != nil {
			editor.mu.Lock()
			defer editor.mu.Unlock()
		}
	}
	err = p.ui.Cancel(p.uiOwner(), ticket)
	if !errors.Is(err, commandui.ErrAlreadyCommitting) {
		run.cancel()
	}
	return err
}

func (a *App) GetUICommandResult(ticket string) (CommandExecutionResult, error) {
	_, run, err := a.commandUIResultRun(ticket)
	if err != nil {
		return CommandExecutionResult{}, err
	}
	select {
	case <-run.done:
	case <-a.commandBridgeContext().Done():
		return CommandExecutionResult{}, a.commandBridgeContext().Err()
	}
	if _, current, err := a.commandUIResultRun(ticket); err != nil || current != run {
		if err == nil {
			err = commandexecution.ErrStale
		}
		return CommandExecutionResult{}, err
	}
	if run.err == nil && run.result.InvocationID != run.reservation.InvocationID {
		return CommandExecutionResult{}, commandexecution.ErrExecution
	}
	return run.result, run.err
}

// Consultar o resultado não admite efeitos. Uma mutação de perfil pode ter
// retirado o mapa por segurança; ainda assim seu dono autenticado deve poder
// conhecer o resultado, inclusive outcome_unknown. Demais comandos mantêm o
// gate de readiness; nenhum Begin/Take/Commit usa esta exceção.
func (a *App) commandUIResultRun(ticket string) (*commandProductRuntime, *commandUIRun, error) {
	if p, run, err := a.commandUIRun(ticket); err == nil {
		return p, run, nil
	}
	p := a.commandProduct.Load()
	if p == nil || !p.dependenciesMatch(a) {
		return nil, nil, commandexecution.ErrDenied
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil || principal != p.principal {
		return nil, nil, commandexecution.ErrDenied
	}
	current, err := p.sessionSvc.RevalidateLocalSession(a.commandBridgeContext(), principal)
	if err != nil || current != principal || !a.commandPrincipalMatches(p.sessionSvc, p.credMgr, principal) {
		return nil, nil, commandexecution.ErrDenied
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.expireUIResultsLocked(time.Now())
	run := p.uiRuns[ticket]
	if p.closed || a.commandProduct.Load() != p || run == nil || !isProfileMutationCommand(run.reservation.CommandID) {
		return nil, nil, commandexecution.ErrDenied
	}
	return p, run, nil
}

// Chamado exclusivamente pelo executor após o CAS running. Não aguarda UI,
// não readquire o gate e não publica evento antes da admissão no ledger.
func (a *App) startCommandUI(ctx context.Context, invocation commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
	p := a.commandProduct.Load()
	if p == nil || p.ui == nil || invocation.Principal != p.principal || !p.dependenciesMatch(a) {
		return commandexecution.ExecutionHandle{}, commandexecution.ErrDenied
	}
	if isLocalUICommand(invocation.CommandID) {
		return commandexecution.ExecutionHandle{}, commandexecution.ErrDenied
	}
	e := invocation.Envelope
	if e == nil || e.GlobalConfigGeneration == nil || e.ActiveLayersGeneration == nil {
		return commandexecution.ExecutionHandle{}, commandexecution.ErrDenied
	}
	p.mu.Lock()
	var matched bool
	for _, run := range p.uiRuns {
		if run.reservation.InvocationID == invocation.ID && run.reservation.CommandID == invocation.CommandID {
			run.admission = &commandUIAdmission{
				epoch:    commandsecurity.EpochSnapshot{UserID: p.principal.UserID, SessionID: p.principal.SessionID, AuthGeneration: e.AuthGeneration, SecurityGeneration: e.SecurityGeneration},
				versions: commandexecution.Versions{Registry: e.RegistryVersion, GlobalConfig: *e.GlobalConfigGeneration, ActiveLayers: *e.ActiveLayersGeneration, Unlocked: true},
			}
			matched = true
			break
		}
	}
	p.mu.Unlock()
	if !matched {
		return commandexecution.ExecutionHandle{}, commandexecution.ErrDenied
	}
	if isWorkspaceMutationCommand(invocation.CommandID) || isAuditedUIContextualCommand(invocation.CommandID) {
		snapshot, err := p.workspaceMgr.CommandSnapshot()
		if err != nil || snapshot.WorkspaceID != p.workspaceID || snapshot.Tab.ID == "" || e.ContextVersion == nil || p.facts == nil {
			return commandexecution.ExecutionHandle{}, commandexecution.ErrStale
		}
		workspaceID := snapshot.WorkspaceID
		proof, err := p.facts.Capture(ctx, commandcontext.Scope{UserID: p.principal.UserID, AuthContextID: p.principal.SessionID, WorkspaceID: &workspaceID}, commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion}}})
		if err != nil || proof.ContextVersion() != *e.ContextVersion {
			return commandexecution.ExecutionHandle{}, commandexecution.ErrStale
		}
		var terminalManager *terminal.Manager
		if invocation.CommandID == commandWorkspaceTabTerminalCreateID || invocation.CommandID == commandTerminalSessionCreateID || invocation.CommandID == commandTerminalSessionCloseID {
			terminalManager = a.currentTerminalManager()
			if terminalManager == nil {
				return commandexecution.ExecutionHandle{}, commandruntime.ErrNotReady
			}
		}
		p.mu.Lock()
		for _, run := range p.uiRuns {
			if run.reservation.InvocationID == invocation.ID {
				if (invocation.CommandID == commandConversationClearID && (run.snapshot != snapshot || run.conversationFingerprint == "")) || ((isPageMutationCommand(invocation.CommandID) || isChatActionCommand(invocation.CommandID) || isChatMessageCommand(invocation.CommandID) || invocation.CommandID == commandTerminalInterruptID || invocation.CommandID == commandTerminalSessionCloseID) && run.snapshot != snapshot) {
					p.mu.Unlock()
					return commandexecution.ExecutionHandle{}, commandexecution.ErrStale
				}
				if !isChatMessageCommand(invocation.CommandID) {
					run.snapshot = snapshot
				}
				if invocation.CommandID != commandTerminalInterruptID {
					run.terminalMgr = terminalManager
				}
				break
			}
		}
		p.mu.Unlock()
	}
	handle, err := p.ui.Start(ctx, p.uiOwner(), invocation)
	if err == nil && isProfileMutationCommand(invocation.CommandID) {
		p.mu.Lock()
		for _, run := range p.uiRuns {
			if run.reservation.InvocationID == invocation.ID && run.pageMutation != nil {
				handle.CommitOwnership = run.pageMutation.ownership
				break
			}
		}
		p.mu.Unlock()
	}
	return handle, err
}
