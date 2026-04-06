package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"assistente/internal/allowlist"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/tools"
	deeplinktool "assistente/internal/tools/deeplink"
	"assistente/internal/tools/editor"
	"assistente/internal/tools/filesystem"
	"assistente/internal/tools/history"
	questiontool "assistente/internal/tools/questionnaire"
	"assistente/internal/tools/shell"
	tasklisttool "assistente/internal/tools/tasklist"
	"assistente/internal/tools/web"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// appTaskListManager adapta o App para a interface tasklisttool.TaskListManager
type appTaskListManager struct {
	ctx context.Context
}

// appDeepLinkEmitter emite deep links para o frontend via eventos Wails.
type appDeepLinkEmitter struct {
	ctx context.Context
}

func (e *appDeepLinkEmitter) EmitDeepLink(uri string) {
	runtime.EventsEmit(e.ctx, "deeplink:execute", uri)
}

func (m *appTaskListManager) CreateTaskList(title, description string, templateWorkflow *database.TaskListWorkflow) (*database.TaskList, error) {
	return database.CreateTaskList(title, description, templateWorkflow)
}

func (m *appTaskListManager) GetTaskList(id uint) (*database.TaskList, error) {
	return database.GetTaskList(id)
}

func (m *appTaskListManager) GetAllTaskLists() ([]database.TaskList, error) {
	return database.GetAllTaskLists()
}

func (m *appTaskListManager) GetTaskListStats(taskListID uint) (map[string]interface{}, error) {
	return database.GetTaskListStats(taskListID)
}

func (m *appTaskListManager) CreateTask(taskListID uint, title, description, code, link string, parentID *uint) (*database.Task, error) {
	task, err := database.CreateTask(taskListID, title, description, code, link, parentID)
	if err == nil && task != nil && m.ctx != nil {
		runtime.EventsEmit(m.ctx, "task:created", task)
	}
	return task, err
}

func (m *appTaskListManager) CreateTaskFull(taskListID uint, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *uint) (*database.Task, error) {
	task, err := database.CreateTaskFull(taskListID, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID, parentID)
	if err == nil && task != nil && m.ctx != nil {
		runtime.EventsEmit(m.ctx, "task:created", task)
	}
	return task, err
}

func (m *appTaskListManager) GetTask(id uint) (*database.Task, error) {
	return database.GetTask(id)
}

func (m *appTaskListManager) FindTaskByCode(taskListID uint, code string) (*database.Task, error) {
	return database.FindTaskByCode(taskListID, code)
}

func (m *appTaskListManager) UpdateTask(id uint, title, description, code, link string) error {
	if err := database.UpdateTask(id, title, description, code, link); err != nil {
		return err
	}
	m.emitTaskUpdated(id)
	return nil
}

func (m *appTaskListManager) UpdateTaskFull(id uint, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	if err := database.UpdateTaskFull(id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID); err != nil {
		return err
	}
	m.emitTaskUpdated(id)
	return nil
}

func (m *appTaskListManager) UpdateTaskAssignee(id uint, assigneeName, assigneeID string) error {
	if err := database.UpdateTaskAssignee(id, assigneeName, assigneeID); err != nil {
		return err
	}
	m.emitTaskUpdated(id)
	return nil
}

func (m *appTaskListManager) UpdateTaskStatus(id uint, newStatusID int) error {
	if err := database.UpdateTaskStatus(id, newStatusID); err != nil {
		return err
	}
	m.emitTaskUpdated(id)
	return nil
}

func (m *appTaskListManager) MoveTaskToList(taskID uint, targetTaskListID uint) (*database.Task, error) {
	oldTask, err := database.GetTask(taskID)
	if err != nil {
		return nil, err
	}
	oldListID := oldTask.TaskListID

	task, err := database.MoveTaskToList(taskID, targetTaskListID)
	if err != nil {
		return nil, err
	}

	if m.ctx != nil && oldListID != targetTaskListID {
		runtime.EventsEmit(m.ctx, "task:updated", task)
		runtime.EventsEmit(m.ctx, "taskList:updated", oldListID)
		runtime.EventsEmit(m.ctx, "taskList:updated", targetTaskListID)
	}
	return task, err
}

func (m *appTaskListManager) emitTaskUpdated(id uint) {
	if m.ctx == nil {
		return
	}
	task, err := database.GetTask(id)
	if err == nil && task != nil {
		runtime.EventsEmit(m.ctx, "task:updated", task)
	}
}

func (m *appTaskListManager) DeleteTask(id uint) error {
	return database.DeleteTask(id)
}

func (m *appTaskListManager) GetWorkflow(taskListID uint) (*database.TaskListWorkflow, error) {
	return database.GetWorkflow(taskListID)
}

func (m *appTaskListManager) CreateTaskNote(taskID uint, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error) {
	return database.CreateTaskNote(taskID, noteType, content, authorName, authorID)
}

func (m *appTaskListManager) UpdateTaskNote(noteID uint, content string) error {
	return database.UpdateTaskNote(noteID, content)
}

func (m *appTaskListManager) GetTaskNotes(taskID uint) ([]database.TaskNote, error) {
	return database.GetTaskNotes(taskID)
}

func (m *appTaskListManager) GetTaskNote(noteID uint) (*database.TaskNote, error) {
	return database.GetTaskNote(noteID)
}

func (m *appTaskListManager) UpdateTaskListFull(id uint, title, description, preferredViewMode string) error {
	return database.UpdateTaskListFull(id, title, description, preferredViewMode)
}

func (m *appTaskListManager) UpdateWorkflowFull(taskListID uint, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error {
	return database.UpdateWorkflowFull(taskListID, statuses, transitions, initialStatusID, statusMigration)
}

func (m *appTaskListManager) GetTaskCountsByStatus(taskListID uint) (map[int]int64, error) {
	return database.GetTaskCountsByStatus(taskListID)
}

// initToolRegistry inicializa o registro de ferramentas disponíveis
func (a *App) initToolRegistry() {
	a.toolRegistry = tools.NewRegistry()
	a.toolExecutor = tools.NewExecutor(a.toolRegistry, tools.DefaultExecutorConfig())

	// Determina diretório de trabalho para as tools de filesystem
	workDir, err := os.Getwd()
	if err != nil {
		log.Printf("[Tools] Erro ao obter diretório de trabalho: %v", err)
		workDir = "."
	}

	// Registra ferramentas de filesystem
	a.toolRegistry.MustRegister(filesystem.NewReadFile(workDir))
	a.toolRegistry.MustRegister(filesystem.NewListDirectory(workDir))
	a.toolRegistry.MustRegister(filesystem.NewSearchFiles(workDir))
	a.toolRegistry.MustRegister(filesystem.NewGrepSearch(workDir))
	a.toolRegistry.MustRegister(filesystem.NewWriteFile(workDir))
	a.toolRegistry.MustRegister(filesystem.NewEditFile(workDir))
	a.toolRegistry.MustRegister(filesystem.NewMoveFile(workDir))
	a.toolRegistry.MustRegister(filesystem.NewCopyFile(workDir))
	a.toolRegistry.MustRegister(filesystem.NewDeleteFile(workDir))
	a.toolRegistry.MustRegister(filesystem.NewMakeDirectory(workDir))

	// Registra ferramentas web (credMgr já foi inicializado antes)
	a.toolRegistry.MustRegister(web.NewWebFetch(a.credMgr)) // GET simples, foco em leitura

	// HTTPRequest com CredentialManager (autenticação automática por domínio)
	httpReqTool := web.NewHTTPRequest(a.credMgr)

	// Confirmação para operações destrutivas
	httpReqTool.SetConfirmFunc(func(ctx context.Context, method, url, body string) (bool, error) {
		if a.questionnaireMgr == nil {
			return false, fmt.Errorf("questionnaire manager não inicializado")
		}
		bodyPreview := body
		if bodyPreview == "" {
			bodyPreview = "(sem body)"
		}
		resp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
			Title:       fmt.Sprintf("Confirmar operação %s", method),
			Description: fmt.Sprintf("O assistente quer executar:\n\n%s %s\n\nBody:\n%s", method, url, bodyPreview),
			AllowCancel: true,
			SubmitLabel: "Permitir",
			CancelLabel: "Negar",
			Questions: []questionnaire.Question{
				{
					ID:       "approve",
					Type:     "boolean",
					Prompt:   fmt.Sprintf("Permitir esta operação %s?", method),
					Required: true,
				},
			},
		})
		if err != nil {
			return false, err
		}
		if resp.Cancelled {
			return false, nil
		}
		approved, ok := resp.Answers["approve"].(bool)
		if !ok {
			return false, fmt.Errorf("resposta inválida para aprovação")
		}
		return approved, nil
	})
	a.toolRegistry.MustRegister(httpReqTool)

	a.toolRegistry.MustRegister(web.NewWebSearch(a.credMgr))

	// Registra ferramenta de shell (run_command)
	confirmFn := func(ctx context.Context, cmd, wd string) (bool, error) {
		if a.questionnaireMgr == nil {
			return false, fmt.Errorf("questionnaire manager não inicializado")
		}
		resp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
			Title:       "Confirmar execução de comando",
			Description: fmt.Sprintf("O assistente quer executar:\n\n%s\n\nem: %s", cmd, wd),
			AllowCancel: true,
			SubmitLabel: "Permitir",
			CancelLabel: "Negar",
			Questions: []questionnaire.Question{
				{
					ID:       "approve",
					Type:     "boolean",
					Prompt:   "Permitir a execução deste comando?",
					Required: true,
				},
			},
		})
		if err != nil {
			return false, err
		}
		if resp.Cancelled {
			return false, nil
		}
		approved, ok := resp.Answers["approve"].(bool)
		if !ok {
			return false, fmt.Errorf("resposta inválida para aprovação de comando")
		}
		return approved, nil
	}
	getAllowlistFn := func() *allowlist.Allowlist {
		activeProfile, err := a.profileManager.GetActive()
		if err != nil || activeProfile == nil {
			// Sem perfil ativo: usa allowlist padrão
			al, err := a.allowlistMgr.Get("padrao")
			if err != nil {
				return nil // sem allowlist = tudo requer confirmação
			}
			return al
		}
		if activeProfile.Chat.CommandAllowlist == "" {
			// Perfil sem allowlist configurada: usa a padrão
			al, err := a.allowlistMgr.Get("padrao")
			if err != nil {
				return nil
			}
			return al
		}
		al, err := a.allowlistMgr.Get(activeProfile.Chat.CommandAllowlist)
		if err != nil {
			log.Printf("[Tools] Allowlist '%s' não encontrada, usando confirmação para tudo", activeProfile.Chat.CommandAllowlist)
			return nil
		}
		return al
	}
	a.toolRegistry.MustRegister(shell.NewRunCommand(a.terminalMgr, confirmFn, getAllowlistFn, workDir))

	// Registra ferramenta de questionário (collect_responses)
	a.toolRegistry.MustRegister(questiontool.NewCollectResponses(a.questionnaireMgr))

	// Registra ferramenta de edição de texto (opt-in: só disponível em perfis que a listam explicitamente)
	a.toolRegistry.MustRegisterOptIn(editor.NewTextEdit(a.questionnaireMgr))

	// Registra ferramenta de busca no histórico de conversas
	a.toolRegistry.MustRegister(history.NewSearchConversations())

	// Registra ferramentas de gerenciamento de task lists
	tlMgr := &appTaskListManager{ctx: a.ctx}
	a.toolRegistry.MustRegister(tasklisttool.NewTaskList(tlMgr))
	a.toolRegistry.MustRegister(tasklisttool.NewTask(tlMgr))
	a.toolRegistry.MustRegister(tasklisttool.NewTaskNote(tlMgr))

	// Registra ferramenta de deep links
	a.toolRegistry.MustRegister(deeplinktool.NewOpenDeepLink(&appDeepLinkEmitter{ctx: a.ctx}))

	log.Printf("[Tools] Registry inicializado com %d ferramentas: %v", a.toolRegistry.Count(), a.toolRegistry.Names())
}
