package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"assistente/internal/allowlist"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/questionnaire"
	"assistente/internal/tasklist"
	"assistente/internal/tools"
	deeplinktool "assistente/internal/tools/deeplink"
	"assistente/internal/tools/filesystem"
	"assistente/internal/tools/history"
	questiontool "assistente/internal/tools/questionnaire"
	"assistente/internal/tools/shell"
	tasklisttool "assistente/internal/tools/tasklist"
	"assistente/internal/tools/web"
)

// serviceTaskListManager adapta tasklist.Service para a interface tasklisttool.TaskListManager.
type serviceTaskListManager struct {
	ctx context.Context
	svc *tasklist.Service
}

func (m *serviceTaskListManager) CreateTaskList(title, description string, templateWorkflow *database.TaskListWorkflow) (*database.TaskList, error) {
	return m.svc.CreateTaskList(m.ctx, title, description, templateWorkflow)
}
func (m *serviceTaskListManager) GetTaskList(id uint) (*database.TaskList, error) {
	return m.svc.GetTaskList(id)
}
func (m *serviceTaskListManager) GetAllTaskLists() ([]database.TaskList, error) {
	return m.svc.GetAllTaskLists()
}
func (m *serviceTaskListManager) GetTaskListStats(taskListID uint) (map[string]interface{}, error) {
	return m.svc.GetTaskListStats(taskListID)
}
func (m *serviceTaskListManager) UpdateTaskListFull(id uint, title, description, preferredViewMode string) error {
	return m.svc.UpdateTaskListFull(id, title, description, preferredViewMode)
}
func (m *serviceTaskListManager) UpdateWorkflowFull(taskListID uint, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error {
	return m.svc.UpdateWorkflowFull(taskListID, statuses, transitions, initialStatusID, statusMigration)
}
func (m *serviceTaskListManager) GetTaskCountsByStatus(taskListID uint) (map[int]int64, error) {
	return m.svc.GetTaskCountsByStatus(taskListID)
}
func (m *serviceTaskListManager) CreateTask(taskListID uint, title, description, code, link string, parentID *uint) (*database.Task, error) {
	return m.svc.CreateTask(taskListID, title, description, code, link, parentID)
}
func (m *serviceTaskListManager) CreateTaskFull(taskListID uint, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *uint) (*database.Task, error) {
	return m.svc.CreateTaskFull(taskListID, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID, parentID)
}
func (m *serviceTaskListManager) GetTask(id uint) (*database.Task, error) { return m.svc.GetTask(id) }
func (m *serviceTaskListManager) FindTaskByCode(taskListID uint, code string) (*database.Task, error) {
	return m.svc.FindTaskByCode(taskListID, code)
}
func (m *serviceTaskListManager) UpdateTask(id uint, title, description, code, link string) error {
	return m.svc.UpdateTask(id, title, description, code, link)
}
func (m *serviceTaskListManager) UpdateTaskFull(id uint, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	return m.svc.UpdateTaskFull(id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID)
}
func (m *serviceTaskListManager) UpdateTaskAssignee(id uint, assigneeName, assigneeID string) error {
	return m.svc.UpdateTaskAssignee(id, assigneeName, assigneeID)
}
func (m *serviceTaskListManager) UpdateTaskStatus(id uint, newStatusID int) error {
	return m.svc.UpdateTaskStatus(id, newStatusID)
}
func (m *serviceTaskListManager) MoveTaskToList(taskID uint, targetTaskListID uint) (*database.Task, error) {
	return m.svc.MoveTaskToList(taskID, targetTaskListID)
}
func (m *serviceTaskListManager) DeleteTask(id uint) error { return m.svc.DeleteTask(id) }
func (m *serviceTaskListManager) GetWorkflow(taskListID uint) (*database.TaskListWorkflow, error) {
	return m.svc.GetWorkflow(taskListID)
}
func (m *serviceTaskListManager) CreateTaskNote(taskID uint, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error) {
	return m.svc.CreateTaskNote(taskID, int(noteType), content, authorName, authorID)
}
func (m *serviceTaskListManager) UpdateTaskNote(noteID uint, content string) error {
	return m.svc.UpdateTaskNote(noteID, content)
}
func (m *serviceTaskListManager) GetTaskNotes(taskID uint) ([]database.TaskNote, error) {
	return m.svc.GetTaskNotes(taskID)
}
func (m *serviceTaskListManager) GetTaskNote(noteID uint) (*database.TaskNote, error) {
	return m.svc.GetTaskNote(noteID)
}

// appDeepLinkEmitter emite deep links para o frontend via events.Emitter.
type appDeepLinkEmitter struct {
	emitter events.Emitter
}

func (e *appDeepLinkEmitter) EmitDeepLink(uri string) {
	e.emitter.Emit("deeplink:execute", uri)
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
	a.toolRegistry.MustRegister(filesystem.NewEditFile(workDir, a.questionnaireMgr))
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

	// Registra ferramenta de busca no histórico de conversas
	a.toolRegistry.MustRegister(history.NewSearchConversations(a.msgRepo))

	// Registra ferramentas de gerenciamento de task lists
	tlMgr := &serviceTaskListManager{ctx: a.ctx, svc: a.taskSvc}
	a.toolRegistry.MustRegister(tasklisttool.NewTaskList(tlMgr))
	a.toolRegistry.MustRegister(tasklisttool.NewTask(tlMgr))
	a.toolRegistry.MustRegister(tasklisttool.NewTaskNote(tlMgr))

	// Registra ferramenta de deep links
	a.toolRegistry.MustRegister(deeplinktool.NewOpenDeepLink(&appDeepLinkEmitter{emitter: a.emitter}))

	log.Printf("[Tools] Registry inicializado com %d ferramentas: %v", a.toolRegistry.Count(), a.toolRegistry.Names())
}
