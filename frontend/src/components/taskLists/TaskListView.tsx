import { useEffect, useRef, useCallback, useMemo, useState, lazy, Suspense } from 'react';
import { AppstoreOutlined, ClearOutlined, CopyOutlined, DeleteOutlined, MessageOutlined, PlusOutlined, ThunderboltOutlined, UnorderedListOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
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
import { Button } from '../ui/Button';
import { openTaskLink } from '../../lib/deepLinks';
import { buildChatSurfaceParams, createSurfaceSnapshotVersion, type SurfaceContext } from '../../lib/chatSurface';
import TasksTable, { type TasksTableRef } from './TasksTable';
import KanbanBoard, { type KanbanBoardRef } from './KanbanBoard';
import { useCustomActions } from './useCustomActions';
import type { ViewMode, TaskListWorkflowStatus, WorkflowTransitions, CustomActionView } from '../../types/tasklist';
import { readTaskListSurfaceContext } from '../../lib/commandTaskListSurface';
import { useWorkspaceCommandSurface } from '../workspace/useWorkspaceCommandSurface';
import { readTaskListCommandTarget } from '../../lib/commandPageMutationWails';
import { usePagePresentationCommands } from '../../lib/commandPagePresentation';
import { usePageMutationCommands, type PageMutationID, type PageMutationRequest, type PageMutationResult } from '../../lib/commandPageMutation';

const WorkflowEditor = lazy(() => import('./WorkflowEditor'));
const CustomActionsEditor = lazy(() => import('./CustomActionsEditor'));

type TaskListStoreSnapshot = ReturnType<typeof useTaskListStore.getState>;

interface TaskListSurfaceFacts {
  readonly taskListRef: unknown;
  readonly updatedAt: string | undefined;
  readonly viewMode: ViewMode | undefined;
  readonly available: boolean;
  readonly loading: boolean;
  readonly loadError: string | undefined;
}

function taskListSurfaceFacts(
  state: TaskListStoreSnapshot,
  taskListId: string,
): TaskListSurfaceFacts {
  const taskList = state.taskLists.get(taskListId);
  return {
    taskListRef: taskList,
    updatedAt: taskList?.updatedAt,
    viewMode: taskList?.preferredViewMode,
    available: state.taskPages?.has(taskListId) ?? false,
    loading: state.loadingTaskPagesByListId?.has(taskListId) ?? false,
    loadError: state.taskPageLoadErrors?.get(taskListId),
  };
}

function sameTaskListSurfaceFacts(left: TaskListSurfaceFacts, right: TaskListSurfaceFacts): boolean {
  return left.taskListRef === right.taskListRef &&
    left.updatedAt === right.updatedAt &&
    left.viewMode === right.viewMode &&
    left.available === right.available &&
    left.loading === right.loading &&
    left.loadError === right.loadError;
}

interface TaskListViewProps {
  taskListId: string;
}

/**
 * Renderiza o conteúdo de uma TaskList individual (toolbar + table/kanban).
 * Usado dentro de uma aba do workspace.
 */
export default function TaskListView({ taskListId }: TaskListViewProps) {
  const { t } = useTranslation();
  const { pathname } = useLocation();
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
  const { loadTaskList, loadMoreTasks, loadAllTasksForBoard, cancelBoardTaskLoad, clearError, setViewMode, deleteTaskList, updateWorkflowFull, getTaskCountsByStatus, listBoardCustomActions, setTaskListConversation } = useTaskListStore();
  const { runCustomAction } = useCustomActions();

  const readTaskListSurface = useCallback(() => {
    const currentWorkspace = useWorkspaceStore.getState().workspace;
    const currentPanelTab = currentWorkspace?.tabs.find((tab) => tab.id === panelTab?.id);
    const currentPanelTabTaskListId = currentPanelTab?.type === 'tasklist'
      ? currentPanelTab.state?.tasklistId
      : undefined;
    const currentTaskListStore = useTaskListStore.getState();
    const currentTaskList = currentTaskListStore.taskLists.get(taskListId);
    return readTaskListSurfaceContext({
      surfaceType: 'tasklist',
      surfaceId: panelTab?.id ?? '',
      panelTabId: panelTab?.id ?? '',
      panelTabTaskListId: typeof currentPanelTabTaskListId === 'string'
        ? currentPanelTabTaskListId
        : undefined,
      taskListId,
      taskList: currentTaskList,
      taskPageAvailable: currentTaskListStore.taskPages?.has(taskListId) ?? false,
      loading: currentTaskListStore.loadingTaskPagesByListId?.has(taskListId) ?? false,
      loadError: currentTaskListStore.taskPageLoadErrors?.get(taskListId),
    });
  }, [panelTab?.id, panelTab?.state, panelTab?.type, taskListId]);

  const subscribeTaskListSurface = useCallback((invalidate: () => void) => {
    let previous = taskListSurfaceFacts(useTaskListStore.getState(), taskListId);
    return useTaskListStore.subscribe((state) => {
      const next = taskListSurfaceFacts(state, taskListId);
      if (sameTaskListSurfaceFacts(previous, next)) return;
      previous = next;
      invalidate();
    });
  }, [taskListId]);

  useWorkspaceCommandSurface('tasklist', readTaskListSurface, subscribeTaskListSurface);

  const tasksRef = useRef<TasksTableRef | KanbanBoardRef | null>(null);
  const commandRootRef = useRef<HTMLDivElement>(null);
  const [isWorkflowEditorOpen, setIsWorkflowEditorOpen] = useState(false);
  const [isCustomActionsEditorOpen, setIsCustomActionsEditorOpen] = useState(false);
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
    if (board) { board.focus(); return document.activeElement === board; }
    // DataGrid: focus a cell with tabindex=0, or the grid container
    const cell = area.querySelector<HTMLElement>('[role="gridcell"][tabindex="0"]');
    if (cell) { cell.focus(); return document.activeElement === cell; }
    const grid = area.querySelector<HTMLElement>('[role="grid"]');
    if (grid) { grid.focus(); return document.activeElement === grid; }
    return false;
  }, []);

  const tasks = useMemo(() => taskList?.tasks || [], [taskList?.tasks]);
  const currentViewMode: ViewMode = taskList?.preferredViewMode || 'list';
  const hasTasks = tasks.length > 0;
  const hasAnyTasks = hasTasks || (taskList?.taskCount ?? 0) > 0;
  const hasTaskPage = taskPage !== undefined;
  const lastBoardProgressAnnouncementRef = useRef('');
  const isMountedRef = useRef(false);
  const activeTaskListIdRef = useRef(taskListId);
  const isPanelActiveRef = useRef(isActive);
  const taskListReadyRef = useRef(Boolean(taskList && taskPage));
  const boardLoadObserverGenerationRef = useRef(0);
  activeTaskListIdRef.current = taskListId;
  isPanelActiveRef.current = isActive;
  taskListReadyRef.current = Boolean(taskList && taskPage);

  const canFocusWorkspacePanelImmediately = useCallback(() => {
    if (
      !isPanelActiveRef.current
      || isModalOpen()
      || useWorkspaceChatModalStore.getState().isOpen
      || !taskListReadyRef.current
    ) return false;
    const area = contentAreaRef.current;
    if (!area?.isConnected) return false;
    return Boolean(
      area.querySelector<HTMLElement>('.kanban-board[tabindex="0"]')
      || area.querySelector<HTMLElement>('[role="gridcell"][tabindex="0"]')
      || area.querySelector<HTMLElement>('[role="grid"]'),
    );
  }, []);

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
    }, () => {
      if (!canFocusWorkspacePanelImmediately()) return false;
      return focusContentArea();
    }, canFocusWorkspacePanelImmediately);
  }, [canFocusWorkspacePanelImmediately, focusContentArea, panelTabId]);

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
    if (!tasksRef.current) return false;
    tasksRef.current.openCreateModal();
    return true;
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
    try {
      await updateWorkflowFull(taskListId, statuses, transitions, initialStatusId, statusMigration);
      setIsWorkflowEditorOpen(false);
      addToast(t('tasklist.workflow.saved', 'Workflow atualizado com sucesso'), 'success', undefined, undefined, {
        suppressAnnounce: true,
      });
      announce(t('tasklist.workflow.saved', 'Workflow atualizado com sucesso'));
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      throw new Error(msg || t('tasklist.workflow.saveFailed', 'Erro ao salvar workflow'));
    }
  }, [taskListId, updateWorkflowFull, addToast, announce, t]);

  const readActiveTaskListTarget = useCallback(() => {
    const workspace = useWorkspaceStore.getState().workspace;
    const currentTab = panelTab?.id
      ? workspace?.tabs.find((tab) => tab.id === panelTab.id)
      : undefined;
    if (!isActive || !panelTab?.id || workspace?.activeTabId !== panelTab.id ||
      currentTab?.type !== 'tasklist' || currentTab.state?.tasklistId !== taskListId) return null;
    return useTaskListStore.getState().taskLists.get(taskListId) ?? null;
  }, [isActive, panelTab?.id, taskListId]);

  const isTaskListSurfaceCurrent = useCallback(() => {
    return readActiveTaskListTarget() !== null;
  }, [readActiveTaskListTarget]);

  const handlePageMutationSucceeded = useCallback(async (commandId: PageMutationID, result: PageMutationResult) => {
    if (!isTaskListSurfaceCurrent()) return;
    if (commandId === 'tasklists.clear') {
      await loadTaskList(taskListId);
    } else if (commandId === 'tasklists.duplicate' && result.id) {
      await loadTaskList(result.id);
    }
    if (!isTaskListSurfaceCurrent()) return;
    const message = commandId === 'tasklists.clear'
      ? t('tasklist.clearedSuccess', 'Lista limpa com sucesso')
      : t('tasklist.clonedSuccess', 'Lista clonada com sucesso');
    addToast(message, 'success', undefined, undefined, { suppressAnnounce: true });
    announce(message);
  }, [addToast, announce, isTaskListSurfaceCurrent, loadTaskList, t, taskListId]);

  const { request: requestPageMutation } = usePageMutationCommands({
    root: commandRootRef,
    pathname,
    tabId: panelTab?.id,
    allowedCommands: ['tasklists.clear', 'tasklists.duplicate'],
    canStart: (commandId) => {
      const target = readActiveTaskListTarget();
      if (!target || isModalOpen()) return false;
      return commandId === 'tasklists.clear' ? (target.taskCount ?? target.tasks.length) > 0 : true;
    },
    prepare: (commandId) => {
      const captured = readActiveTaskListTarget();
      if (!captured) return undefined;
      const targetId = taskListId;
      return {
        readRequest: async (): Promise<PageMutationRequest> => {
          const target = await readTaskListCommandTarget(targetId);
          const title = commandId === 'tasklists.duplicate'
            ? `${target.taskList.title} ${t('tasklist.cloneTitleSuffix', '(Cópia)')}`
            : '';
          return {
            targetId,
            expectedFingerprint: target.fingerprint,
            title,
            description: target.taskList.description || '',
          };
        },
        isCurrent: () => isTaskListSurfaceCurrent() &&
          useTaskListStore.getState().taskLists.get(targetId) === captured,
        canPresent: () => isTaskListSurfaceCurrent(),
        succeeded: (result: PageMutationResult) => handlePageMutationSucceeded(commandId, result),
      };
    },
    subscribe: (changed) => {
      let previous = useTaskListStore.getState().taskLists.get(taskListId);
      return useTaskListStore.subscribe((state) => {
        const next = state.taskLists.get(taskListId);
        if (next === previous) return;
        previous = next;
        changed();
      });
    },
  });

  const runPageMutation = useCallback(async (commandId: PageMutationID) => {
    const outcome = await requestPageMutation(commandId);
    if (outcome.status === 'succeeded') return;
    if (outcome.status === 'cancelled' || outcome.status === 'denied') return;
    if (!isTaskListSurfaceCurrent()) return;
    addToast(t('common.error', 'Erro ao alterar lista'), 'error');
  }, [addToast, isTaskListSurfaceCurrent, requestPageMutation, t]);

  const { request: requestPagePresentationCommand } = usePagePresentationCommands({
    root: commandRootRef,
    pathname,
    tabId: panelTab?.id,
    allowedCommands: ['tasklist.task.create.open'],
    readTarget: readActiveTaskListTarget,
    subscribe: (changed) => {
      let previous = useTaskListStore.getState().taskLists.get(taskListId);
      return useTaskListStore.subscribe((state) => {
        const next = state.taskLists.get(taskListId);
        if (next === previous) return;
        previous = next;
        changed();
      });
    },
    isCurrent: isTaskListSurfaceCurrent,
    canOpen: () => Boolean(tasksRef.current && isTaskListSurfaceCurrent()),
    open: () => handleOpenCreateTask(),
  });

  const requestCreateTask = useCallback(() => {
    return requestPagePresentationCommand('tasklist.task.create.open');
  }, [requestPagePresentationCommand]);

  const handleClone = useCallback(() => {
    void runPageMutation('tasklists.duplicate');
  }, [runPageMutation]);

  const handleClear = useCallback(() => {
    void runPageMutation('tasklists.clear');
  }, [runPageMutation]);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.repeat || e.isComposing || e.keyCode === 229 || !isActive || isModalOpen()) return;
      const root = commandRootRef.current;
      if (!root || !(e.target instanceof Node) || !root.contains(e.target)) return;

      const key = e.key.toLowerCase();
      if (e.ctrlKey && !e.altKey && !e.metaKey && !e.shiftKey && key === 'l') {
        e.preventDefault();
        e.stopPropagation();
        handleClear();
        return;
      }
      if (e.ctrlKey || e.altKey || e.metaKey || e.shiftKey) return;
      if (e.target instanceof Element && e.target.closest('input,textarea,select,[contenteditable="true"]')) return;

      if (key === 'n') {
        if (requestCreateTask()) {
          e.preventDefault();
          e.stopPropagation();
        }
        return;
      }
      if (key === 'd') {
        e.preventDefault();
        e.stopPropagation();
        handleClone();
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [handleClear, handleClone, isActive, requestCreateTask]);

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
    <div ref={commandRootRef} className="tasklist-detail">
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
            {
              key: 'custom-actions',
              label: t('tasklist.customActions.configure', 'Ações customizadas'),
              icon: <ThunderboltOutlined />,
              onClick: () => setIsCustomActionsEditorOpen(true),
              variant: 'secondary' as const,
            },
            {
              key: 'edit-workflow',
              label: t('tasklist.workflow.editWorkflow', 'Editar Workflow'),
              icon: '⚙️',
              onClick: handleOpenWorkflowEditor,
              variant: 'secondary' as const,
            },
            {
              key: 'clone-list',
              label: t('tasklist.duplicate', 'Duplicar'),
              icon: <CopyOutlined />,
              shortcut: 'D',
              onClick: handleClone,
              variant: 'secondary' as const,
            },
            {
              key: 'clear-list',
              label: t('tasklist.clear', 'Limpar'),
              icon: <ClearOutlined />,
              shortcut: 'Ctrl+L',
              onClick: () => void handleClear(),
              variant: 'danger' as const,
              disabled: !hasAnyTasks,
            },
            {
              key: 'delete-list',
              label: t('tasklist.delete', 'Apagar'),
              icon: <DeleteOutlined />,
              onClick: handleDelete,
              variant: 'danger' as const,
            },
          ]}
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
        >
          <Suspense fallback={<div>{t('tasklist.loading', 'Carregando...')}</div>}>
            <CustomActionsEditor
              taskListId={taskListId}
              onClose={() => setIsCustomActionsEditorOpen(false)}
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
        >
          <Suspense fallback={<div>{t('tasklist.loading', 'Carregando...')}</div>}>
            <WorkflowEditor
              workflow={taskList.workflow}
              taskCountsByStatus={taskCountsByStatus}
              onSave={handleSaveWorkflow}
              onCancel={() => setIsWorkflowEditorOpen(false)}
            />
          </Suspense>
        </Modal>
      )}
    </div>
  );
}
