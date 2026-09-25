package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"assistente/controllers"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"assistente/internal/profiles"
	"assistente/internal/tasklist"
	"assistente/internal/workspace"
)

var commandPageMutationNames = map[string][3]string{
	"tasklists.create":    {"Salvar nova lista", "Save new task list", "Guardar nueva lista"},
	"tasklists.update":    {"Salvar alterações da lista", "Save task list changes", "Guardar cambios de la lista"},
	"tasklists.duplicate": {"Duplicar lista selecionada", "Duplicate selected task list", "Duplicar lista seleccionada"},
	"tasklists.delete":    {"Excluir lista selecionada", "Delete selected task list", "Eliminar lista seleccionada"},
	"tasklists.clear":     {"Limpar tarefas da lista aberta", "Clear tasks from the open list", "Vaciar tareas de la lista abierta"},
	"profiles.create":     {"Salvar novo perfil", "Save new profile", "Guardar nuevo perfil"},
	"profiles.update":     {"Salvar alterações do perfil", "Save profile changes", "Guardar cambios del perfil"},
	"profiles.duplicate":  {"Duplicar perfil selecionado", "Duplicate selected profile", "Duplicar perfil seleccionado"},
	"profiles.delete":     {"Excluir perfil selecionado", "Delete selected profile", "Eliminar perfil seleccionado"},
	"profiles.activate":   {"Ativar perfil selecionado", "Activate selected profile", "Activar perfil seleccionado"},
}
var commandPageMutationIDs = []string{"tasklists.create", "tasklists.update", "tasklists.duplicate", "tasklists.delete", "tasklists.clear", "profiles.create", "profiles.update", "profiles.duplicate", "profiles.delete", "profiles.activate"}

func isProfileMutationCommand(id string) bool {
	switch id {
	case "profiles.create", "profiles.update", "profiles.duplicate", "profiles.delete", "profiles.activate":
		return true
	default:
		return false
	}
}

func isPageMutationCommand(id string) bool { _, ok := commandPageMutationNames[id]; return ok }
func pageMutationDestructive(id string) bool {
	return id == "tasklists.delete" || id == "tasklists.clear" || id == "profiles.delete"
}
func commandPageMutationRegistrations() []commandcatalog.Registration {
	var result []commandcatalog.Registration
	for _, id := range commandPageMutationIDs {
		names := commandPageMutationNames[id]
		d, h := commandWorkspaceChatOpenRegistration()
		d.ID, d.HandlerRoute = id, "contextual/page/"+id
		h.Route = d.HandlerRoute
		d.Risk = commandcatalog.RiskMedium
		d.Persistence.Result = commandcatalog.PersistenceNever
		d.MutatesEffectiveCapability = isProfileMutationCommand(id)
		h.MutatesEffectiveCapability = d.MutatesEffectiveCapability
		if pageMutationDestructive(id) {
			d.Effect = commandcatalog.Destructive
			h.Effect = d.Effect
			d.Decision = commandcatalog.Interactive
		}
		d.Presentation = &commandcatalog.Presentation{Version: "page-mutation-v1", Locales: map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: names[0], Description: "Operação sobre a lista capturada; alterações concorrentes são recusadas", Category: "Listas de tarefas"},
			"en":    {Name: names[1], Description: "Operation on the captured task list; concurrent changes are rejected", Category: "Task lists"},
			"es":    {Name: names[2], Description: "Operación sobre la lista capturada; los cambios concurrentes se rechazan", Category: "Listas de tareas"},
		}}
		if isProfileMutationCommand(id) {
			d.Presentation.Locales = map[string]commandcatalog.LocalizedMetadata{
				"pt-BR": {Name: names[0], Description: "Operação sobre o perfil capturado; alterações concorrentes são recusadas", Category: "Perfis"},
				"en":    {Name: names[1], Description: "Operation on the captured profile; concurrent changes are rejected", Category: "Profiles"},
				"es":    {Name: names[2], Description: "Operación sobre el perfil capturado; los cambios concurrentes se rechazan", Category: "Perfiles"},
			}
		}
		if id == "tasklists.clear" {
			d.Presentation.Locales = map[string]commandcatalog.LocalizedMetadata{
				"pt-BR": {Name: names[0], Description: "Remove todas as tarefas e suas notas da lista capturada. Mantém a lista e o workflow; alterações concorrentes são recusadas", Category: "Listas de tarefas"},
				"en":    {Name: names[1], Description: "Removes all tasks and their notes from the captured list. Keeps the list and workflow; concurrent changes are rejected", Category: "Task lists"},
				"es":    {Name: names[2], Description: "Elimina todas las tareas y sus notas de la lista capturada. Conserva la lista y el flujo; los cambios concurrentes se rechazan", Category: "Listas de tareas"},
			}
		}
		result = append(result, commandcatalog.Registration{Definition: d, Handler: h})
	}
	return result
}

type CommandPageMutationRequest struct {
	TargetID            string            `json:"targetId"`
	ExpectedFingerprint string            `json:"expectedFingerprint"`
	Title               string            `json:"title"`
	Description         string            `json:"description"`
	Profile             *profiles.Profile `json:"profile,omitempty"`
}
type CommandProfileTarget struct {
	Profile     *profiles.Profile `json:"profile"`
	Fingerprint string            `json:"fingerprint"`
}
type CommandTaskListTarget struct {
	TaskList    *database.TaskList `json:"taskList"`
	Fingerprint string             `json:"fingerprint"`
}
type CommandPageMutationResult struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
type commandPageMutationPreparation struct {
	ready              chan struct{}
	ctx                context.Context
	claimed            bool
	versions           commandexecution.Versions
	taskService        *tasklist.Service
	mutation           *tasklist.CommandMutation
	profileManager     *profiles.Manager
	profileController  *controllers.ProfilesController
	profileMutation    *controllers.PreparedProfileMutation
	ownership          *commandexecution.CommitOwnership
	rebuildAfterCommit bool
	result             CommandPageMutationResult
	targetTitle        string
}

func (a *App) ReadProfileCommandTarget(slug string) (CommandProfileTarget, error) {
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return CommandProfileTarget{}, err
	}
	a.authMu.RLock()
	mgr := a.profileManager
	a.authMu.RUnlock()
	if mgr == nil || len(slug) > 256 {
		return CommandProfileTarget{}, commandexecution.ErrDenied
	}
	var value *profiles.Profile
	var fp string
	if slug == "" {
		fp, err = mgr.CommandMutationSnapshot("")
	} else {
		value, fp, err = mgr.ReadCommandTarget(slug)
	}
	if err != nil {
		return CommandProfileTarget{}, commandexecution.ErrDenied
	}
	if current, err := a.authenticatedCommandProduct(); err != nil || current != p {
		return CommandProfileTarget{}, commandexecution.ErrStale
	}
	return CommandProfileTarget{Profile: value, Fingerprint: fp}, nil
}

func (a *App) ReadTaskListCommandTarget(id string) (CommandTaskListTarget, error) {
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return CommandTaskListTarget{}, err
	}
	a.authMu.RLock()
	svc := a.taskSvc
	a.authMu.RUnlock()
	if svc == nil {
		return CommandTaskListTarget{}, commandexecution.ErrDenied
	}
	value, fp, err := svc.ReadCommandTarget(database.WithUserID(a.commandBridgeContext(), p.principal.UserID), id)
	if err != nil {
		return CommandTaskListTarget{}, commandexecution.ErrDenied
	}
	if current, err := a.authenticatedCommandProduct(); err != nil || current != p {
		return CommandTaskListTarget{}, commandexecution.ErrStale
	}
	return CommandTaskListTarget{TaskList: value, Fingerprint: fp}, nil
}

// IDs and content are runtime preparation, never persisted binding arguments.
// The reservation identifies the operation; the UI cannot change its kind.
func (a *App) PreparePageMutationCommand(ticket string, request CommandPageMutationRequest) error {
	p, run, err := a.commandUIRun(ticket)
	if err != nil {
		return err
	}
	if !isPageMutationCommand(run.reservation.CommandID) {
		return commandexecution.ErrDenied
	}
	p.mu.Lock()
	prep := run.pageMutation
	if prep == nil || prep.claimed {
		p.mu.Unlock()
		return commandexecution.ErrDenied
	}
	prep.claimed = true
	p.mu.Unlock()
	failed := true
	defer func() {
		if failed {
			run.cancel()
			_ = p.ui.Cancel(p.uiOwner(), ticket)
		}
	}()
	if len(request.Title) > 4096 || len(request.Description) > 1<<20 || len(request.ExpectedFingerprint) > 256 || len(request.TargetID) > 256 {
		return commandexecution.ErrDenied
	}
	if err := a.validatePageMutationOrigin(prep.ctx, p, run); err != nil {
		return err
	}
	if isProfileMutationCommand(run.reservation.CommandID) {
		data, err := json.Marshal(request.Profile)
		if err != nil || len(data) > 1<<20 || request.ExpectedFingerprint == "" {
			return commandexecution.ErrDenied
		}
		operation := strings.TrimPrefix(run.reservation.CommandID, "profiles.")
		if (operation == "create" || operation == "update") != (request.Profile != nil) {
			return commandexecution.ErrDenied
		}
		var title string
		if operation == "delete" {
			value, fp, err := prep.profileManager.ReadCommandTarget(request.TargetID)
			if err != nil || fp != request.ExpectedFingerprint {
				return commandexecution.ErrStale
			}
			title = value.Name
		}
		prepared, err := prep.profileController.PrepareCommandMutation(operation, request.TargetID, request.Profile, request.ExpectedFingerprint)
		if err != nil {
			return commandexecution.ErrStale
		}
		if err := a.validatePageMutationOrigin(prep.ctx, p, run); err != nil {
			return err
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		if prep.ctx.Err() != nil || p.closed || p.uiRuns[ticket] != run {
			return commandexecution.ErrStale
		}
		prep.profileMutation, prep.targetTitle = prepared, title
		close(prep.ready)
		failed = false
		return nil
	}
	operation := strings.TrimPrefix(run.reservation.CommandID, "tasklists.")
	if operation == "duplicate" {
		operation = "clone"
	}
	var targetTitle string
	if pageMutationDestructive(run.reservation.CommandID) {
		target, fingerprint, err := prep.taskService.ReadCommandTarget(database.WithUserID(prep.ctx, p.principal.UserID), request.TargetID)
		if err != nil || fingerprint != request.ExpectedFingerprint {
			return commandexecution.ErrStale
		}
		targetTitle = target.Title
	}
	mutation, err := prep.taskService.PrepareCommandMutation(database.WithUserID(prep.ctx, p.principal.UserID), operation, request.TargetID, request.Title, request.Description, request.ExpectedFingerprint)
	if err != nil {
		return commandexecution.ErrStale
	}
	if err := a.validatePageMutationOrigin(prep.ctx, p, run); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if prep.ctx.Err() != nil || p.closed || p.uiRuns[ticket] != run {
		return commandexecution.ErrStale
	}
	prep.mutation = mutation
	prep.targetTitle = targetTitle
	close(prep.ready)
	failed = false
	return nil
}

func (a *App) validatePageMutationOrigin(ctx context.Context, p *commandProductRuntime, run *commandUIRun) error {
	if ctx.Err() != nil || a.commandProduct.Load() != p || !p.dependenciesMatch(a) || run.sourceValid != nil && !run.sourceValid() {
		return commandexecution.ErrStale
	}
	p.mu.Lock()
	prep, expected := run.pageMutation, run.snapshot
	p.mu.Unlock()
	if prep == nil || (!isProfileMutationCommand(run.reservation.CommandID) && prep.taskService == nil) || (isProfileMutationCommand(run.reservation.CommandID) && (prep.profileManager == nil || prep.profileController == nil)) {
		return commandexecution.ErrDenied
	}
	versions, err := p.host.Snapshot(ctx, p.principal)
	if err != nil || versions != prep.versions {
		return commandexecution.ErrStale
	}
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	if (isProfileMutationCommand(run.reservation.CommandID) && (a.profileManager != prep.profileManager || a.profilesCtrl != prep.profileController)) || (!isProfileMutationCommand(run.reservation.CommandID) && a.taskSvc != prep.taskService) || a.workspaceMgr != p.workspaceMgr || a.currentAuthUser == nil || a.currentAuthUser.UserID != p.principal.UserID || a.currentAuthUser.SessionID != p.principal.SessionID {
		return commandexecution.ErrDenied
	}
	return p.workspaceMgr.WithCommandSnapshot(ctx, func(current workspace.CommandSnapshot) error {
		if current != expected {
			return commandexecution.ErrStale
		}
		return nil
	})
}

func (a *App) commitPageMutationCommand(ticket, handoff string) error {
	p, run, err := a.commandUIRun(ticket)
	if err != nil {
		return err
	}
	if !isPageMutationCommand(run.reservation.CommandID) {
		return commandexecution.ErrDenied
	}
	ctx, cancel := context.WithTimeout(a.commandBridgeContext(), 35*time.Second)
	defer cancel()
	return p.ui.Commit(ctx, p.uiOwner(), ticket, handoff, func(ctx context.Context) error {
		if err := a.validatePageMutationOrigin(ctx, p, run); err != nil {
			return err
		}
		if isProfileMutationCommand(run.reservation.CommandID) {
			return a.commitProfilePageMutation(ctx, p, run)
		}
		p.mu.Lock()
		admission, expected, prep := run.admission, run.snapshot, run.pageMutation
		mutation := prep.mutation
		p.mu.Unlock()
		if admission == nil || mutation == nil {
			return commandexecution.ErrDenied
		}
		var effectCtx context.Context
		release, err := p.epochs.AdmitExecution(ctx, admission.epoch, func(ctx context.Context) error { return a.validatePageMutationOrigin(ctx, p, run) }, func(ctx context.Context) error {
			return p.host.WithPublishedVersions(ctx, p.principal, prep.versions, func() error {
				return p.withContextualDeckPageSource(ctx, run, func() error {
					return p.workspaceMgr.WithCommandSnapshot(ctx, func(current workspace.CommandSnapshot) error {
						if current != expected {
							return commandexecution.ErrStale
						}
						effectCtx = ctx
						return nil
					})
				})
			})
		})
		if err != nil {
			return err
		}
		defer release()
		guard := func(commit func() error) error {
			a.authMu.RLock()
			defer a.authMu.RUnlock()
			if a.taskSvc != prep.taskService || a.commandProduct.Load() != p || a.workspaceMgr != p.workspaceMgr || a.currentAuthUser == nil || a.currentAuthUser.UserID != p.principal.UserID || a.currentAuthUser.SessionID != p.principal.SessionID {
				return commandexecution.ErrStale
			}
			return p.workspaceMgr.WithCommandSnapshot(effectCtx, func(current workspace.CommandSnapshot) error {
				if current != expected {
					return commandexecution.ErrStale
				}
				return commit()
			})
		}
		result, err := mutation.CommitGuarded(database.WithUserID(effectCtx, p.principal.UserID), guard)
		if err != nil {
			if errors.Is(err, tasklist.ErrCommandMutationStale) {
				return commandexecution.ErrStale
			}
			return err
		}
		p.mu.Lock()
		if result != nil {
			prep.result = CommandPageMutationResult{ID: result.ID, Title: result.Title}
		}
		p.mu.Unlock()
		return nil
	})
}

func (a *App) GetPageMutationCommandResult(ticket string) (CommandPageMutationResult, error) {
	p, run, err := a.commandUIResultRun(ticket)
	if err != nil {
		return CommandPageMutationResult{}, err
	}
	if !isPageMutationCommand(run.reservation.CommandID) {
		return CommandPageMutationResult{}, commandexecution.ErrDenied
	}
	status, err := a.GetUICommandResult(ticket)
	if err != nil || status.Status != "succeeded" {
		return CommandPageMutationResult{}, commandexecution.ErrDenied
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if run.pageMutation == nil {
		return CommandPageMutationResult{}, commandexecution.ErrDenied
	}
	return run.pageMutation.result, nil
}
