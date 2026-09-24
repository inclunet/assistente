import { useEffect, useRef, useCallback, useMemo, useState, lazy, Suspense } from 'react';
import { AppstoreOutlined, ClearOutlined, CopyOutlined, DeleteOutlined, EditOutlined, MessageOutlined, PlusOutlined, ThunderboltOutlined, UnorderedListOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useTaskListStore } from '../../store/taskListStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useWorkspaceChatModalStore } from '../../store/workspaceChatModalStore';
import type { WorkspaceChatModalAdapter } from '../../store/workspaceChatModalStore';
import { useRegisterWorkspaceChatAdapter } from '../../hooks/useRegisterWorkspaceChatAdapter';
import { useWorkspacePanel } from '../workspace/WorkspacePanelContext';
import { useUIStore } from '../../store/uiStore';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useConfirm } from '../../hooks/useConfirm';
import { registerWorkspacePanelFocus } from '../workspace/workspacePanelFocusRegistry';
import { isModalOpen, Modal } from '../ui/Modal';
import { Toolbar } from '../ui/Toolbar';
import { DATAGRID_ENTRY_SELECTOR } from '../ui/DataGrid';
import { Button } from '../ui/Button';
import { DialogActions } from '../ui/DialogActions';
import { FormField } from '../ui/FormField';
import { Input } from '../ui/Input';
import { Textarea } from '../ui/Textarea';
import { MenuButton } from '../layout/MenuButton';
import { openTaskLink } from '../../lib/deepLinks';
import { buildChatSurfaceParams, createSurfaceSnapshotVersion, type SurfaceContext } from '../../lib/chatSurface';
import TasksTable, { type TasksTableRef } from './TasksTable';
import KanbanBoard, { type KanbanBoardRef } from './KanbanBoard';
import { useCustomActions } from './useCustomActions';
import type { ViewMode, TaskListWorkflowStatus, WorkflowTransitions, CustomActionView } from '../../types/tasklist';

const WorkflowEditor = lazy(() => import('./WorkflowEditor'));
const CustomActionsEditor = lazy(() => import('./CustomActionsEditor'));

interface TaskListViewProps {
  taskListId: string;
}

/**
 * Renderiza o conteúdo de uma TaskList individual (toolbar + table/kanban).
 * Usado dentro de uma aba do workspace.
 */
export default function TaskListView({ taskListId }: TaskListViewProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const addToast = useUIStore((s) => s.addToast);
  const { announce } = useAnnouncer();
  const requestConfirm = useConfirm();

  const { tab: panelTab, isActive } = useWorkspacePanel();
  const wsProfile = useWorkspaceStore((s) => s.workspace?.profile);
  const tabProfileSlug = panelTab?.type === 'tasklist'
    ? (panelTab.profileOverride?.slug as string | undefined)
    : undefined;
  const effectiveProfileSlug = tabProfileSlug || wsProfile || '';

  const taskList = useTaskListStore((s) => s.taskLists.get(taskListId));
  const taskPage = useTaskListStore((s) => s.taskPages?.get(taskListId));
  const initialLoadErrorKey = `loadTaskList:${taskListId}`;
  const initialLoadError = useTaskListStore((s) => s.errors?.get(initialLoadErrorKey));
  const isLoadingTaskPage = useTaskListStore((s) => s.loadingTaskPagesByListId?.has(taskListId) ?? false);
  const taskPageLoadError = useTaskListStore((s) => s.taskPageLoadErrors?.get(taskListId));
  const { loadTaskList, loadMoreTasks, loadAllTasksForBoard, cancelBoardTaskLoad, clearError, setViewMode, cloneTaskList, clearTaskList, deleteTaskList, updateTaskList, updateWorkflowFull, getTaskCountsByStatus, listBoardCustomActions, setTaskListConversation } = useTaskListStore();
  const { runCustomAction } = useCustomActions();

  const tasksRef = useRef<TasksTableRef | KanbanBoardRef | null>(null);
  const [isWorkflowEditorOpen, setIsWorkflowEditorOpen] = useState(false);
  const [isCustomActionsEditorOpen, setIsCustomActionsEditorOpen] = useState(false);
  const [isEditListOpen, setIsEditListOpen] = useState(false);
  const [editTitle, setEditTitle] = useState('');
  const [editDescription, setEditDescription] = useState('');
  const [editSaving, setEditSaving] = useState(false);
  const [boardActions, setBoardActions] = useState<CustomActionView[]>([]);
  const [taskCountsByStatus, setTaskCountsByStatus] = useState<Record<number, number>>({});

  // Conversa atualmente vinculada ao chat embutido desta aba (quando o modal de
  // chat está aberto). Usada para auto-vincular a lista à conversa do chat.
  const chatBoundConversationId = useWorkspaceChatModalStore(
    (s) => (s.isOpen && s.boundTabId === panelTab?.id ? s.boundConversationId : null),
  );

  const boardActionsReqRef = useRef(0);
  const announcedInitialLoadErrorRef = useRef<string | null>(null);
  const initialLoadRequestRef = useRef<string | null>(null);
  const requestInitialLoad = useCallback(() => {
    if (initialLoadRequestRef.current === taskListId) return;
    initialLoadRequestRef.current = taskListId;
    void Promise.resolve(loadTaskList(taskListId)).finally(() => {
      if (initialLoadRequestRef.current === taskListId) {
        initialLoadRequestRef.current = null;
      }
    });
  }, [loadTaskList, taskListId]);

  const reloadBoardActions = useCallback(() => {
    // Guard por request-id: se taskListId mudar enquanto a Promise anterior ainda
    // está pendente, a resposta antiga não deve sobrescrever a lista mais recente.
    const reqId = ++boardActionsReqRef.current;
    listBoardCustomActions(taskListId)
      .then((res) => { if (boardActionsReqRef.current === reqId) setBoardActions(res); })
      .catch(() => { if (boardActionsReqRef.current === reqId) setBoardActions([]); });
  }, [listBoardCustomActions, taskListId]);

  useEffect(() => {
    reloadBoardActions();
  }, [reloadBoardActions]);

  useEffect(() => {
    if ((!taskList || !taskPage) && !initialLoadError) {
      requestInitialLoad();
    }
  }, [taskList, taskPage, initialLoadError, requestInitialLoad]);

  useEffect(() => {
    if (!initialLoadError) {
      announcedInitialLoadErrorRef.current = null;
      return;
    }
    if (announcedInitialLoadErrorRef.current === initialLoadError) return;
    announcedInitialLoadErrorRef.current = initialLoadError;
    announce(initialLoadError, 'assertive');
  }, [initialLoadError, announce]);

  const contentAreaRef = useRef<HTMLDivElement>(null);

  const focusContentArea = useCallback((): boolean => {
    const area = contentAreaRef.current;
    if (!area) return false;
    // Kanban: focus the board container which manages card focus internally
    const board = area.querySelector<HTMLElement>('.kanban-board[tabindex="0"]');
    if (board) { board.focus(); return true; }
    // DataGrid: focus a cell with tabindex=0, or the grid container
    const cell = area.querySelector<HTMLElement>('[role="gridcell"][tabindex="0"]');
    if (cell) { cell.focus(); return true; }
    const grid = area.querySelector<HTMLElement>('[role="grid"]');
    if (grid) { grid.focus(); return true; }
    return false;
  }, []);

  const tasks = useMemo(() => taskList?.tasks || [], [taskList?.tasks]);
  const currentViewMode: ViewMode = taskList?.preferredViewMode || 'list';
  const hasTasks = tasks.length > 0;
  const hasTaskPage = taskPage !== undefined;
  const lastBoardProgressAnnouncementRef = useRef('');
  const isMountedRef = useRef(false);
  const activeTaskListIdRef = useRef(taskListId);
  const isPanelActiveRef = useRef(isActive);
  const boardLoadObserverGenerationRef = useRef(0);
  activeTaskListIdRef.current = taskListId;
  isPanelActiveRef.current = isActive;

  useEffect(() => {
    isMountedRef.current = true;
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  // Foco de painel assíncrono (tasklist/kanban).
  //
  // Ao entrar na aba (Ctrl+Tab/PageUp-Down/Ctrl+N ou ao fechar outra aba), o
  // `WorkspaceLayout` roteia o foco via `workspacePanelFocusRegistry`. Como o
  // board carrega páginas de forma assíncrona (tela "Carregando..." antes de
  // `taskList`/`taskPage`), não dá para focar no instante da troca: replicamos o
  // padrão do editor (nonce + efeito "quando pronto"). O handler apenas marca um
  // pedido; um efeito refaz o foco assim que a superfície está renderizada.
  const panelTabId = panelTab?.id;
  const [panelFocusNonce, setPanelFocusNonce] = useState(0);
  const consumedPanelFocusNonceRef = useRef(0);

  useEffect(() => {
    if (!panelTabId) return;
    return registerWorkspacePanelFocus(panelTabId, () => {
      if (!isPanelActiveRef.current || isModalOpen()) return false;
      setPanelFocusNonce((nonce) => nonce + 1);
      return true;
    });
  }, [panelTabId]);

  useEffect(() => {
    if (
      panelFocusNonce === 0 ||
      consumedPanelFocusNonceRef.current === panelFocusNonce ||
      !isActive ||
      isModalOpen() ||
      !taskList ||
      !taskPage
    ) {
      return;
    }
    const nonce = panelFocusNonce;
    const raf = requestAnimationFrame(() => {
      if (consumedPanelFocusNonceRef.current === nonce) return;
      if (!isPanelActiveRef.current || isModalOpen()) return;
      // Só marca o pedido como consumido quando o foco realmente pousa na
      // superfície; se o board ainda não montou, deixamos o nonce pendente para
      // um novo commit (cards carregando, troca de modo) tentar de novo.
      if (focusContentArea()) {
        consumedPanelFocusNonceRef.current = nonce;
      }
    });
    return () => cancelAnimationFrame(raf);
  }, [panelFocusNonce, isActive, taskList, taskPage, currentViewMode, focusContentArea]);

  const handleLoadBoardPages = useCallback(async (observerGeneration: number) => {
    const shouldAnnounce = () => (
      isMountedRef.current &&
      isPanelActiveRef.current &&
      boardLoadObserverGenerationRef.current === observerGeneration &&
      activeTaskListIdRef.current === taskListId &&
      useTaskListStore.getState().taskLists.get(taskListId)?.preferredViewMode === 'kanban'
    );
    try {
      const loaded = await loadAllTasksForBoard(taskListId);
      if (!shouldAnnounce()) return;
      announce(
        t('tasklist.pagination.boardLoaded', 'Quadro completo com {{count}} cards', { count: loaded }),
        'polite',
      );
    } catch {
      if (!shouldAnnounce()) return;
      announce(
        t('tasklist.pagination.boardLoadFailed', 'Não foi possível carregar todos os cards. Os cards disponíveis continuam navegáveis.'),
        'polite',
      );
    }
  }, [loadAllTasksForBoard, taskListId, announce, t]);

  const requestBoardBackgroundLoad = useCallback(() => {
    const observerGeneration = ++boardLoadObserverGenerationRef.current;
    void handleLoadBoardPages(observerGeneration);
    return observerGeneration;
  }, [handleLoadBoardPages]);

  useEffect(() => {
    if (!isActive || currentViewMode !== 'kanban' || !hasTaskPage) return;
    const page = useTaskListStore.getState().taskPages.get(taskListId);
    if (!page?.hasMore) return;
    const observerGeneration = requestBoardBackgroundLoad();
    return () => {
      if (boardLoadObserverGenerationRef.current === observerGeneration) {
        boardLoadObserverGenerationRef.current += 1;
      }
      cancelBoardTaskLoad(taskListId);
    };
  }, [isActive, currentViewMode, hasTaskPage, taskListId, requestBoardBackgroundLoad, cancelBoardTaskLoad]);

  useEffect(() => {
    if (
      !isActive ||
      currentViewMode !== 'kanban' ||
      !isLoadingTaskPage ||
      !taskPage ||
      tasks.length >= taskPage.totalCount
    ) {
      return;
    }
    const signature = `${taskListId}:${tasks.length}:${taskPage.totalCount}`;
    if (lastBoardProgressAnnouncementRef.current === signature) return;
    lastBoardProgressAnnouncementRef.current = signature;
    announce(
      t('tasklist.pagination.boardProgress', '{{loaded}} de {{total}} cards carregados; o quadro já está navegável', {
        loaded: tasks.length,
        total: taskPage.totalCount,
      }),
      'polite',
    );
  }, [isActive, currentViewMode, isLoadingTaskPage, taskPage, tasks.length, taskListId, announce, t]);

  const handleOpenCreateTask = useCallback(() => {
    tasksRef.current?.openCreateModal();
  }, []);

  const handleLoadMore = useCallback(async () => {
    try {
      await loadMoreTasks(taskListId);
      const loaded = useTaskListStore.getState().taskLists.get(taskListId)?.tasks.length ?? 0;
      announce(t('tasklist.pagination.loaded', '{{count}} tarefas carregadas', { count: loaded }));
    } catch {
      addToast(t('tasklist.pagination.loadMoreFailed', 'Erro ao carregar mais tarefas'), 'error');
    }
  }, [loadMoreTasks, taskListId, announce, t, addToast]);

  const handleToggleViewMode = useCallback(async () => {
    const newMode: ViewMode = currentViewMode === 'list' ? 'kanban' : 'list';
    try {
      await setViewMode(taskListId, newMode);
      announce(
        t('tasklist.viewModeChanged', 'Alterado para visualização {{mode}}', {
          mode: t(newMode === 'list' ? 'tasklist.viewModeList' : 'tasklist.viewModeKanban'),
        }),
      );
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      addToast(msg || t('common.error', 'Erro ao alterar visualização'), 'error');
    }
  }, [currentViewMode, taskListId, setViewMode, announce, addToast, t]);

  const handleOpenWorkflowEditor = useCallback(async () => {
    try {
      const counts = await getTaskCountsByStatus(taskListId);
      setTaskCountsByStatus(counts);
      setIsWorkflowEditorOpen(true);
    } catch {
      addToast(t('common.error', 'Erro ao carregar dados'), 'error');
    }
  }, [taskListId, getTaskCountsByStatus, addToast, t]);

  const handleSaveWorkflow = useCallback(async (
    statuses: TaskListWorkflowStatus[],
    transitions: WorkflowTransitions,
    initialStatusId: number,
    statusMigration: Record<number, number>,
  ) => {
    // Salvamento automático a cada alteração: o editor anuncia o resultado e
    // o modal segue aberto.
    try {
      await updateWorkflowFull(taskListId, statuses, transitions, initialStatusId, statusMigration);
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      throw new Error(msg || t('tasklist.workflow.saveFailed', 'Erro ao salvar workflow'));
    }
  }, [taskListId, updateWorkflowFull, t]);

  const handleClone = useCallback(async () => {
    const newTitle = `${taskList?.title || 'Lista'} (Cópia)`;
    try {
      const cloned = await cloneTaskList(taskListId, newTitle);
      if (cloned) {
        addToast(t('tasklist.clonedSuccess', 'Lista clonada com sucesso'), 'success', undefined, undefined, {
          suppressAnnounce: true,
        });
        announce(t('tasklist.clonedSuccess', 'Lista clonada com sucesso'));
      }
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      addToast(msg || t('common.error', 'Erro ao clonar'), 'error');
    }
  }, [taskList?.title, taskListId, cloneTaskList, addToast, announce, t]);

  const handleClear = useCallback(async () => {
    const confirmed = await requestConfirm({
      title: t('tasklist.clearConfirmTitle', 'Limpar Lista'),
      message: t(
        'tasklist.clearConfirmMessage',
        `Tem certeza que deseja remover todas as tarefas de "${taskList?.title}"? Esta ação não pode ser desfeita.`
      ),
    });
    if (!confirmed) return;

    try {
      await clearTaskList(taskListId);
      addToast(t('tasklist.clearedSuccess', 'Lista limpa com sucesso'), 'success', undefined, undefined, {
        suppressAnnounce: true,
      });
      announce(t('tasklist.clearedSuccess', 'Lista limpa com sucesso'));
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      addToast(msg || t('common.error', 'Erro ao limpar'), 'error');
    }
  }, [taskList?.title, taskListId, requestConfirm, clearTaskList, addToast, announce, t]);

  useEffect(() => {
    if (!isActive) return;

    const onKeyDown = (e: KeyboardEvent) => {
      if (isModalOpen()) return;

      if (e.ctrlKey && !e.altKey && !e.metaKey && !e.shiftKey && (e.key === 'l' || e.key === 'L')) {
        e.preventDefault();
        void handleClear();
        return;
      }

      if (e.ctrlKey || e.altKey || e.metaKey || e.shiftKey) return;
      const tag = (e.target as HTMLElement)?.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || (e.target as HTMLElement)?.isContentEditable) return;

      if (e.key === 'n' || e.key === 'N') {
        e.preventDefault();
        handleOpenCreateTask();
        return;
      }

      if (e.key === 'd' || e.key === 'D') {
        e.preventDefault();
        void handleClone();
        return;
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [handleOpenCreateTask, handleClear, handleClone, isActive]);

  const tasklistChatModalAdapter = useMemo((): WorkspaceChatModalAdapter | null => {
    if (!panelTab || panelTab.type !== 'tasklist' || !taskList) return null;

    return {
      prepare: async () => {
        const taskLabel = t('tasklist.chatModalContext.taskCount', { count: tasks.length });
        const header = `${taskList.title}\n${taskLabel}\n`;
        const body = tasks
          .slice(0, 40)
          .map((x) => `- ${String(x.title || '').trim()}`)
          .join('\n');
        const contextDisplay = `${header}${body || t('tasklist.chatModalContext.noTasks')}`;
        return { ok: true, contextDisplay, meta: null };
      },
      send: async (instruction, media) => {
        const taskLabel = t('tasklist.chatModalContext.taskCount', { count: tasks.length });
        const header = `${taskList.title}\n${taskLabel}\n`;
        const previewTasks = tasks.slice(0, 40);
        const body = previewTasks
          .map((x) => `- ${String(x.title || '').trim()}`)
          .join('\n');
        const taskSnapshotSeed = previewTasks
          .map((task) => `${task.id}:${task.updatedAt}:${task.statusId}`)
          .join('|');
        const statuses = [...(taskList.workflow?.statuses ?? [])].sort((a, b) => a.order - b.order);
        const surfaceContext: SurfaceContext = {
          surfaceType: 'tasklist',
          surfaceId: panelTab.id,
          title: taskList.title,
          mode: currentViewMode,
          focus: {
            kind: 'tasklist',
            label: taskList.title,
            entity: { taskListId },
          },
          content: {
            kind: 'tasklist_summary',
            summary: `${header}${body || t('tasklist.chatModalContext.noTasks')}`,
            truncated: tasks.length > previewTasks.length,
          },
          metadata: {
            taskListId,
            slug: taskList.slug,
            taskCount: tasks.length,
            statuses: statuses.map((status) => ({
              id: status.id,
              label: status.label,
              order: status.order,
            })),
          },
          snapshotVersion: createSurfaceSnapshotVersion(
            'tasklist',
            panelTab.id,
            `${taskList.updatedAt}:${taskList.workflow?.updatedAt}:${tasks.length}:${taskSnapshotSeed}`,
          ),
          capturedAt: new Date().toISOString(),
          staleAfterMs: 60000,
        };
        return {
          content: instruction,
          mediaFiles: media,
          paramsOverride: buildChatSurfaceParams(panelTab, {
            profileSlug: effectiveProfileSlug || undefined,
            context: surfaceContext,
          }),
        };
      },
    };
  }, [panelTab, taskList, tasks, currentViewMode, effectiveProfileSlug, taskListId, t]);

  useRegisterWorkspaceChatAdapter(panelTab?.id, tasklistChatModalAdapter);

  const handleDelete = useCallback(async () => {
    const confirmed = await requestConfirm({
      title: t('tasklist.deleteConfirmTitle', 'Deletar Lista'),
      message: t(
        'tasklist.deleteConfirmMessage',
        `Tem certeza que deseja deletar "${taskList?.title}"? Esta ação não pode ser desfeita.`
      ),
    });
    if (!confirmed) return;

    try {
      await deleteTaskList(taskListId);
      addToast(t('tasklist.deletedSuccess', 'Lista deletada com sucesso'), 'success', undefined, undefined, {
        suppressAnnounce: true,
      });
      announce(t('tasklist.deletedSuccess', 'Lista deletada com sucesso'));
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      addToast(msg || t('common.error', 'Erro ao deletar'), 'error');
    }
  }, [taskList?.title, taskListId, requestConfirm, deleteTaskList, addToast, announce, t]);

  const handleOpenLinkedConversation = useCallback(() => {
    if (!taskList?.conversationId) return;
    openTaskLink(`assistente://conversation/${taskList.conversationId}`, { navigate });
  }, [taskList?.conversationId, navigate]);

  const handleOpenEditList = useCallback(() => {
    setEditTitle(taskList?.title ?? '');
    setEditDescription(taskList?.description ?? '');
    setIsEditListOpen(true);
  }, [taskList?.title, taskList?.description]);

  const handleSaveList = useCallback(async () => {
    if (!editTitle.trim()) {
      const msg = t('tasklist.emptyTitle', 'Título não pode estar vazio');
      addToast(msg, 'error');
      announce(msg);
      return;
    }
    setEditSaving(true);
    try {
      await updateTaskList(taskListId, editTitle.trim(), editDescription.trim());
      const msg = t('tasklist.listUpdated', 'Lista atualizada');
      addToast(msg, 'success', undefined, undefined, { suppressAnnounce: true });
      announce(msg);
      setIsEditListOpen(false);
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      addToast(msg || t('common.error', 'Erro ao salvar'), 'error');
    } finally {
      setEditSaving(false);
    }
  }, [editTitle, editDescription, taskListId, updateTaskList, addToast, announce, t]);

  // Auto-vínculo: quando o chat embutido desta aba abre com uma conversa, a lista
  // passa a apontar para ela (inclusive ao iniciar uma conversa nova pelo chat).
  // Sem feedback visual extra — é um efeito implícito do uso do chat.
  useEffect(() => {
    // Só auto-vincula com a lista já carregada no store: evita escrever no backend
    // antes de confirmar que a lista existe e impede chamadas espúrias ao alternar
    // rapidamente de aba/lista (quando taskList ainda é undefined).
    if (!taskList) return;
    if (!chatBoundConversationId) return;
    if (taskList.conversationId === chatBoundConversationId) return;
    void setTaskListConversation(taskListId, chatBoundConversationId).then(() => {
      announce(t('tasklist.conversationLinkSaved', 'Vínculo de conversa atualizado'));
    }).catch((error) => {
      const msg = error instanceof Error ? error.message : String(error);
      addToast(msg || t('common.error', 'Erro ao salvar'), 'error');
    });
  }, [chatBoundConversationId, taskList, taskListId, setTaskListConversation, announce, addToast, t]);

  if (!taskPage && initialLoadError) {
    return (
      <div className="tasklist-loading">
        <span>{initialLoadError}</span>
        <Button
          type="button"
          variant="secondary"
          onClick={() => {
            clearError(initialLoadErrorKey);
            requestInitialLoad();
          }}
        >
          {t('common.retry', 'Tentar novamente')}
        </Button>
      </div>
    );
  }
  if (!taskList || !taskPage) {
    return <div className="tasklist-loading">{t('tasklist.loading', 'Carregando...')}</div>;
  }

  return (
    <div className="tasklist-detail">
      <div className="ws-content-toolbar">
        <Toolbar
          left={
            <h1 className="page-toolbar__title">{taskList.title}</h1>
          }
          actions={[
            {
              key: 'chat-modal',
              label: t('editor.chatModal.title'),
              icon: <MessageOutlined />,
              shortcut: 'Ctrl+Shift+I',
              onClick: () => void useWorkspaceChatModalStore.getState().requestOpen(panelTab.id),
            },
            ...(taskList.conversationId
              ? [{
                  key: 'open-conversation',
                  label: t('tasklist.openConversation', 'Abrir conversa'),
                  icon: <MessageOutlined />,
                  onClick: handleOpenLinkedConversation,
                  variant: 'secondary' as const,
                }]
              : []),
            {
              key: 'new-task',
              label: t('tasklist.createTask', 'Nova Tarefa'),
              icon: <PlusOutlined />,
              onClick: handleOpenCreateTask,
              shortcut: 'N',
              variant: 'primary',
            },
            ...(hasTasks
              ? [
                  {
                    key: 'toggle-view',
                    label: t(currentViewMode === 'list' ? 'tasklist.viewModeKanban' : 'tasklist.viewModeList'),
                    icon: currentViewMode === 'list' ? <AppstoreOutlined /> : <UnorderedListOutlined />,
                    onClick: handleToggleViewMode,
                    variant: 'secondary' as const,
                  },
                ]
              : []),
            ...boardActions.map((ca) => ({
              key: `custom-${ca.id}`,
              label: ca.label,
              icon: ca.icon ? ca.icon : <ThunderboltOutlined />,
              onClick: () => void runCustomAction(ca, taskListId, ''),
              variant: (ca.danger ? 'danger' : 'secondary') as 'danger' | 'secondary',
            })),
          ]}
          rightEnd={
            <MenuButton
              buttonLabel={t('tasklist.settings', 'Configurações')}
              items={[
                {
                  id: 'edit-list',
                  label: t('tasklist.editList', 'Editar Lista'),
                  icon: <EditOutlined aria-hidden="true" />,
                  onClick: handleOpenEditList,
                },
                {
                  id: 'edit-workflow',
                  label: t('tasklist.workflow.editWorkflow', 'Editar Workflow'),
                  icon: <span aria-hidden="true">⚙️</span>,
                  onClick: handleOpenWorkflowEditor,
                },
                {
                  id: 'custom-actions',
                  label: t('tasklist.customActions.configure', 'Ações customizadas'),
                  icon: <ThunderboltOutlined aria-hidden="true" />,
                  onClick: () => setIsCustomActionsEditorOpen(true),
                },
                { separator: true, id: 'sep-1' },
                {
                  id: 'clone-list',
                  label: t('tasklist.duplicate', 'Duplicar'),
                  icon: <CopyOutlined aria-hidden="true" />,
                  shortcut: 'D',
                  onClick: handleClone,
                },
                {
                  id: 'clear-list',
                  label: t('tasklist.clear', 'Limpar'),
                  icon: <ClearOutlined aria-hidden="true" />,
                  shortcut: 'Ctrl+L',
                  onClick: () => void handleClear(),
                  disabled: !hasTasks,
                },
                { separator: true, id: 'sep-2' },
                {
                  id: 'delete-list',
                  label: t('tasklist.delete', 'Apagar'),
                  icon: <DeleteOutlined aria-hidden="true" />,
                  onClick: handleDelete,
                  danger: true,
                },
              ]}
            />
          }
        />
      </div>

      <div className="ws-content-area" ref={contentAreaRef}>
        {currentViewMode === 'kanban' ? (
          <KanbanBoard
            ref={(r) => { tasksRef.current = r; }}
            taskListId={taskListId}
            tasks={tasks}
            taskList={taskList}
            onTaskCreated={() => {}}
            onTaskUpdated={() => {}}
            onTaskDeleted={() => {}}
          />
        ) : (
          <TasksTable
            ref={(r) => { tasksRef.current = r; }}
            taskListId={taskListId}
            tasks={tasks}
            taskList={taskList}
            onTaskCreated={() => {}}
            onTaskUpdated={() => {}}
            onTaskDeleted={() => {}}
          />
        )}
        {taskPage?.hasMore && (
          <div className="tasklist-pagination">
            <Button
              type="button"
              variant="secondary"
              loading={isLoadingTaskPage}
              onClick={() => void (
                currentViewMode === 'kanban' && taskPageLoadError
                  ? requestBoardBackgroundLoad()
                  : handleLoadMore()
              )}
            >
              {currentViewMode === 'kanban' && taskPageLoadError
                ? t('tasklist.pagination.retryBoard', 'Tentar carregar cards restantes')
                : t('tasklist.pagination.loadMore', 'Carregar mais tarefas')}
            </Button>
            <span>
              {t('tasklist.pagination.progress', '{{loaded}} de {{total}} tarefas carregadas', {
                loaded: tasks.length,
                total: taskPage.totalCount,
              })}
            </span>
            {currentViewMode === 'kanban' && taskPageLoadError && (
              <span className="tasklist-pagination-error">
                {t('tasklist.pagination.boardLoadFailed', 'Não foi possível carregar todos os cards. Os cards disponíveis continuam navegáveis.')}
              </span>
            )}
          </div>
        )}
      </div>

      {isCustomActionsEditorOpen && (
        <Modal
          isOpen={isCustomActionsEditorOpen}
          onClose={() => setIsCustomActionsEditorOpen(false)}
          title={t('tasklist.customActions.configure', 'Ações customizadas')}
          size="lg"
          initialFocusSelector={DATAGRID_ENTRY_SELECTOR}
        >
          <Suspense fallback={<div>{t('tasklist.loading', 'Carregando...')}</div>}>
            <CustomActionsEditor
              taskListId={taskListId}
              onSaved={reloadBoardActions}
            />
          </Suspense>
        </Modal>
      )}

      {isWorkflowEditorOpen && taskList.workflow && (
        <Modal
          isOpen={isWorkflowEditorOpen}
          onClose={() => setIsWorkflowEditorOpen(false)}
          title={t('tasklist.workflow.editWorkflow', 'Editar Workflow')}
          size="lg"
          initialFocusSelector={DATAGRID_ENTRY_SELECTOR}
        >
          <Suspense fallback={<div>{t('tasklist.loading', 'Carregando...')}</div>}>
            <WorkflowEditor
              workflow={taskList.workflow}
              taskCountsByStatus={taskCountsByStatus}
              onSave={handleSaveWorkflow}
            />
          </Suspense>
        </Modal>
      )}

      {isEditListOpen && (
        <Modal
          isOpen={isEditListOpen}
          onClose={() => setIsEditListOpen(false)}
          title={t('tasklist.editList', 'Editar Lista')}
        >
          <FormField label={t('tasklist.titleLabel', 'Título')} required>
            <Input
              type="text"
              value={editTitle}
              placeholder={t('tasklist.titlePlaceholder', 'Título da lista')}
              onChange={(e) => setEditTitle(e.target.value)}
              disabled={editSaving}
              maxLength={200}
            />
          </FormField>
          <FormField label={t('tasklist.description', 'Descrição')}>
            <Textarea
              value={editDescription}
              placeholder={t('tasklist.descriptionPlaceholder', 'Adicione mais detalhes...')}
              onChange={(e) => setEditDescription(e.target.value)}
              disabled={editSaving}
              rows={4}
              maxLength={1000}
            />
          </FormField>
          <DialogActions
            primary={
              <Button type="button" variant="primary" onClick={() => void handleSaveList()} disabled={editSaving} loading={editSaving}>
                {t('common.save', 'Salvar')}
              </Button>
            }
            secondary={
              <Button type="button" variant="secondary" onClick={() => setIsEditListOpen(false)} disabled={editSaving}>
                {t('common.cancel', 'Cancelar')}
              </Button>
            }
          />
        </Modal>
      )}

    </div>
  );
}
