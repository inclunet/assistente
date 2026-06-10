package app

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
	feedtool "assistente/internal/tools/feed"
	"assistente/internal/tools/filesystem"
	"assistente/internal/tools/history"
	jobtool "assistente/internal/tools/job"
	questiontool "assistente/internal/tools/questionnaire"
	"assistente/internal/tools/shell"
	subagenttool "assistente/internal/tools/subagent"
	tasklisttool "assistente/internal/tools/tasklist"
	"assistente/internal/tools/web"
)

// serviceTaskListManager adapta tasklist.Service para a interface tasklisttool.TaskListManager.
type serviceTaskListManager struct {
	svc *tasklist.Service
}

func (m *serviceTaskListManager) CreateTaskList(ctx context.Context, title, description string, templateWorkflow *database.TaskListWorkflow, slug string) (*database.TaskList, error) {
	return m.svc.CreateTaskList(ctx, title, description, templateWorkflow, slug)
}
func (m *serviceTaskListManager) GetTaskList(ctx context.Context, id string) (*database.TaskList, error) {
	return m.svc.GetTaskList(ctx, id)
}
func (m *serviceTaskListManager) GetAllTaskLists(ctx context.Context) ([]database.TaskList, error) {
	return m.svc.GetAllTaskLists(ctx)
}
func (m *serviceTaskListManager) GetTaskListStats(ctx context.Context, taskListID string) (map[string]interface{}, error) {
	return m.svc.GetTaskListStats(ctx, taskListID)
}
func (m *serviceTaskListManager) UpdateTaskListFull(ctx context.Context, id string, title, description, preferredViewMode string, slug *string) error {
	return m.svc.UpdateTaskListFull(ctx, id, title, description, preferredViewMode, slug)
}
func (m *serviceTaskListManager) ResolveTaskListRef(ctx context.Context, taskListID *string, taskListSlug string) (string, error) {
	return m.svc.ResolveTaskListRef(ctx, taskListID, taskListSlug)
}
func (m *serviceTaskListManager) SetTaskListConversation(ctx context.Context, id string, conversationID *string) error {
	return m.svc.SetTaskListConversation(ctx, id, conversationID)
}
func (m *serviceTaskListManager) SetTaskListValidationPolicy(ctx context.Context, taskListID string, policyJSON string) error {
	return m.svc.SetTaskListValidationPolicy(ctx, taskListID, policyJSON)
}
func (m *serviceTaskListManager) GetTaskListCustomActions(ctx context.Context, taskListID string) (*database.TaskListCustomActions, error) {
	return m.svc.GetTaskListCustomActions(ctx, taskListID)
}
func (m *serviceTaskListManager) SetTaskListCustomActions(ctx context.Context, taskListID string, actionsJSON string) error {
	return m.svc.SetTaskListCustomActions(ctx, taskListID, actionsJSON)
}
func (m *serviceTaskListManager) UpdateWorkflowFull(ctx context.Context, taskListID string, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error {
	return m.svc.UpdateWorkflowFull(ctx, taskListID, statuses, transitions, initialStatusID, statusMigration)
}
func (m *serviceTaskListManager) GetTaskCountsByStatus(ctx context.Context, taskListID string) (map[int]int64, error) {
	return m.svc.GetTaskCountsByStatus(ctx, taskListID)
}
func (m *serviceTaskListManager) CreateTask(ctx context.Context, taskListID string, title, description, code, link string, parentID *string) (*database.Task, error) {
	return m.svc.CreateTask(ctx, taskListID, title, description, code, link, parentID)
}
func (m *serviceTaskListManager) CreateTaskFull(ctx context.Context, taskListID string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *string) (*database.Task, error) {
	return m.svc.CreateTaskFull(ctx, taskListID, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID, parentID)
}
func (m *serviceTaskListManager) GetTask(ctx context.Context, id string) (*database.Task, error) {
	return m.svc.GetTask(ctx, id)
}
func (m *serviceTaskListManager) FindTaskByCode(ctx context.Context, taskListID string, code string) (*database.Task, error) {
	return m.svc.FindTaskByCode(ctx, taskListID, code)
}
func (m *serviceTaskListManager) ResolveTaskRef(ctx context.Context, taskListID *string, taskListSlug string, taskID *string, code string) (string, error) {
	return m.svc.ResolveTaskRef(ctx, taskListID, taskListSlug, taskID, code)
}
func (m *serviceTaskListManager) ResolveTaskIDByTaskCode(ctx context.Context, taskListID *string, taskCode string) (string, error) {
	return m.svc.ResolveTaskIDByTaskCode(ctx, taskListID, taskCode)
}
func (m *serviceTaskListManager) UpdateTask(ctx context.Context, id string, title, description, code, link string) error {
	return m.svc.UpdateTask(ctx, id, title, description, code, link)
}
func (m *serviceTaskListManager) UpdateTaskFull(ctx context.Context, id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	return m.svc.UpdateTaskFull(ctx, id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID)
}
func (m *serviceTaskListManager) UpdateTaskAssignee(ctx context.Context, id string, assigneeName, assigneeID string) error {
	return m.svc.UpdateTaskAssignee(ctx, id, assigneeName, assigneeID)
}
func (m *serviceTaskListManager) SetTaskConversation(ctx context.Context, id string, conversationID *string) error {
	return m.svc.SetTaskConversation(ctx, id, conversationID)
}
func (m *serviceTaskListManager) UpdateTaskStatus(ctx context.Context, id string, newStatusID int) error {
	return m.svc.UpdateTaskStatus(ctx, id, newStatusID)
}
func (m *serviceTaskListManager) MoveTaskToList(ctx context.Context, taskID string, targetTaskListID string) (*database.Task, error) {
	return m.svc.MoveTaskToList(ctx, taskID, targetTaskListID)
}
func (m *serviceTaskListManager) DeleteTask(ctx context.Context, id string) error {
	return m.svc.DeleteTask(ctx, id)
}
func (m *serviceTaskListManager) GetWorkflow(ctx context.Context, taskListID string) (*database.TaskListWorkflow, error) {
	return m.svc.GetWorkflow(ctx, taskListID)
}
func (m *serviceTaskListManager) CreateTaskNote(ctx context.Context, taskID string, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error) {
	return m.svc.CreateTaskNote(ctx, taskID, int(noteType), content, authorName, authorID)
}
func (m *serviceTaskListManager) UpsertTaskNoteByExternal(ctx context.Context, p database.UpsertTaskNoteByExternalParams) (*database.TaskNote, bool, error) {
	return m.svc.UpsertTaskNoteByExternal(ctx, p)
}
func (m *serviceTaskListManager) UpdateTaskNote(ctx context.Context, noteID string, content string) error {
	return m.svc.UpdateTaskNote(ctx, noteID, content)
}
func (m *serviceTaskListManager) GetTaskNotes(ctx context.Context, taskID string) ([]database.TaskNote, error) {
	return m.svc.GetTaskNotes(ctx, taskID)
}
func (m *serviceTaskListManager) GetTaskNote(ctx context.Context, noteID string) (*database.TaskNote, error) {
	return m.svc.GetTaskNote(ctx, noteID)
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

	// feed_read: RSS/Atom/JSON Feed/podcast -> JSON canônico (auth por domínio)
	a.toolRegistry.MustRegister(feedtool.NewFeedRead(a.credMgr))

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
	a.toolRegistry.MustRegister(history.NewGetConversationInfo())

	// Registra ferramentas de gerenciamento de task lists
	tlMgr := &serviceTaskListManager{svc: a.taskSvc}
	a.toolRegistry.MustRegister(tasklisttool.NewTaskList(tlMgr))
	a.toolRegistry.MustRegister(tasklisttool.NewTask(tlMgr))
	a.toolRegistry.MustRegister(tasklisttool.NewTaskNote(tlMgr))

	// Jobs são opt-in para não inflar o payload padrão, mas descobríveis
	// na UI/catálogo para enabled_tools explícito.
	jobMgr := func() jobtool.Manager {
		if a.jobMgr == nil {
			return nil
		}
		return a.jobMgr
	}
	a.toolRegistry.MustRegisterDiscoverableOptIn(jobtool.NewJobWithProvider(jobMgr))
	a.toolRegistry.MustRegisterDiscoverableOptIn(jobtool.NewPipelineWithProvider(jobMgr))

	// Sub-agentes (AEP-0068): opt-in para não inflar o payload padrão, mas
	// descobrível na UI/catálogo. O gate de profundidade é o próprio profile —
	// o sub-agente só pode criar novos sub-agentes se o profile dele habilitar
	// a tool `subagent` (EnabledTools / DisableTools). O Manager é criado mais
	// tarde no wiring (após o ChatController existir), então o provider é lazy.
	subagentRunner := func() subagenttool.Runner {
		if a.subagentMgr == nil {
			return nil
		}
		return a.subagentMgr
	}
	a.toolRegistry.MustRegisterDiscoverableOptIn(subagenttool.NewWithProvider(subagentRunner))

	// Registra ferramenta de deep links
	a.toolRegistry.MustRegister(deeplinktool.NewOpenDeepLink(&appDeepLinkEmitter{emitter: a.emitter}))

	log.Printf("[Tools] Registry inicializado com %d ferramentas: %v", a.toolRegistry.Count(), a.toolRegistry.Names())
}
