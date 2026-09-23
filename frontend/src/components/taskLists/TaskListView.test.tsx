import { forwardRef, useImperativeHandle, type ReactNode } from 'react';
import { describe, expect, it, beforeEach, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { WorkspaceTab } from '../../store/workspaceStore';
import TaskListView from './TaskListView';
import {
  canFocusWorkspacePanelImmediately,
  getWorkspacePanelImmediateFocusHandler,
  requestWorkspacePanelFocus,
} from '../workspace/workspacePanelFocusRegistry';

const openCreateModalMock = vi.fn();
const registerWorkspaceChatAdapterMock = vi.hoisted(() => vi.fn());
const commandSurfaceGetterMock = vi.hoisted(() => vi.fn());
const requestPagePresentationMock = vi.hoisted(() => vi.fn(() => true));
const requestPageMutationMock = vi.hoisted(() => vi.fn());
const pagePresentationOptionsMock = vi.hoisted(() => vi.fn());
const pageMutationOptionsMock = vi.hoisted(() => vi.fn());
const readTaskListCommandTargetMock = vi.hoisted(() => vi.fn());
const announceMock = vi.hoisted(() => vi.fn());
const chatModalState = vi.hoisted(() => ({
  isOpen: false,
  boundTabId: null as string | null,
  boundConversationId: null as string | null,
}));
const workspacePanelState = vi.hoisted(() => ({
  isActive: false,
  tab: {
    id: 'tasklist-tab',
    type: 'tasklist' as const,
    title: 'Lista',
    position: 0,
    state: { tasklistId: 'tasklist-1' },
  } satisfies WorkspaceTab,
}));

const workspaceStoreState = vi.hoisted(() => ({
  workspace: {
    id: 'workspace-1',
    profile: 'default',
    activeTabId: 'tasklist-tab',
    tabs: [workspacePanelState.tab],
  },
}));

const taskListStoreState = vi.hoisted(() => ({
  taskLists: new Map<string, unknown>(),
  taskPages: new Map<string, { nextCursor: string; hasMore: boolean; totalCount: number }>(),
  loadingByTaskListId: new Map<string, boolean>(),
  loadingTaskPagesByListId: new Map<string, boolean>(),
  taskPageLoadErrors: new Map<string, string>(),
  errors: new Map<string, string>(),
  loadTaskList: vi.fn(),
  loadMoreTasks: vi.fn(),
  loadAllTasksForBoard: vi.fn(),
  cancelBoardTaskLoad: vi.fn(),
  clearError: vi.fn(),
  setViewMode: vi.fn(),
  cloneTaskList: vi.fn(),
  clearTaskList: vi.fn(),
  deleteTaskList: vi.fn(),
  updateWorkflowFull: vi.fn(),
  getTaskCountsByStatus: vi.fn(),
  listBoardCustomActions: vi.fn(),
  triggerCustomAction: vi.fn(),
  setTaskListConversation: vi.fn(),
}));

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
  useLocation: () => ({ pathname: '/' }),
}));

vi.mock('../../lib/commandPagePresentation', () => ({
  usePagePresentationCommands: (options: unknown) => {
    pagePresentationOptionsMock(options);
    return { request: requestPagePresentationMock };
  },
}));

vi.mock('../../lib/commandPageMutation', () => ({
  usePageMutationCommands: (options: unknown) => {
    pageMutationOptionsMock(options);
    return { request: requestPageMutationMock };
  },
}));

vi.mock('../../lib/commandPageMutationWails', () => ({
  readTaskListCommandTarget: (...args: unknown[]) => readTaskListCommandTargetMock(...args),
}));

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
  }),
}));

vi.mock('../workspace/WorkspacePanelContext', async () => {
  const actual = await vi.importActual<typeof import('../workspace/WorkspacePanelContext')>(
    '../workspace/WorkspacePanelContext',
  );
  return {
    ...actual,
    useWorkspacePanel: () => workspacePanelState,
    useOptionalWorkspacePanel: () => ({ ...workspacePanelState, rootRef: undefined }),
  };
});

vi.mock('../workspace/useWorkspaceCommandSurface', () => ({
  useWorkspaceCommandSurface: (_surfaceType: string, getter: () => unknown) => {
    commandSurfaceGetterMock(getter);
  },
}));

vi.mock('../../store/workspaceStore', () => ({
  useWorkspaceStore: Object.assign(
    (selector: (state: typeof workspaceStoreState) => unknown) => selector(workspaceStoreState),
    { getState: () => workspaceStoreState },
  ),
}));

vi.mock('../../store/taskListStore', () => ({
  useTaskListStore: Object.assign((selector?: (state: typeof taskListStoreState) => unknown) => (
    typeof selector === 'function' ? selector(taskListStoreState) : taskListStoreState
  ), {
    getState: () => taskListStoreState,
  }),
}));

vi.mock('../../store/workspaceChatModalStore', () => {
  const useStore = (selector?: (s: typeof chatModalState) => unknown) => (
    typeof selector === 'function' ? selector(chatModalState) : chatModalState
  );
  (useStore as unknown as { getState: () => unknown }).getState = () => ({
    ...chatModalState,
    requestOpen: vi.fn(),
  });
  return { useWorkspaceChatModalStore: useStore };
});

vi.mock('../../store/uiStore', () => ({
  useUIStore: (selector: (state: { addToast: ReturnType<typeof vi.fn> }) => unknown) => selector({
    addToast: vi.fn(),
  }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceMock }),
}));

vi.mock('../../hooks/useConfirm', () => ({
  useConfirm: () => vi.fn().mockResolvedValue(false),
}));

vi.mock('../../hooks/useDefaultFocus', () => ({
  registerDefaultFocus: vi.fn(),
  unregisterDefaultFocus: vi.fn(),
}));

vi.mock('../../hooks/useRegisterWorkspaceChatAdapter', () => ({
  useRegisterWorkspaceChatAdapter: registerWorkspaceChatAdapterMock,
}));

vi.mock('../ui/Modal', () => ({
  isModalOpen: () => false,
  Modal: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock('../ui/Toolbar', () => ({
  Toolbar: ({ actions }: { actions?: Array<{ key: string; label: string; onClick?: () => void; disabled?: boolean }> }) => (
    <div>
      {actions?.map((action) => (
        <button key={action.key} type="button" onClick={action.onClick} disabled={action.disabled}>
          {action.label}
        </button>
      ))}
    </div>
  ),
}));

vi.mock('./TasksTable', () => ({
  default: forwardRef((_props, ref) => {
    useImperativeHandle(ref, () => ({
      openCreateModal: openCreateModalMock,
    }));
    return (
      <div data-testid="tasklist-surface-focus" tabIndex={0}>
        tasks-table
        <input aria-label="campo da lista" />
      </div>
    );
  }),
}));

vi.mock('./KanbanBoard', () => ({
  default: forwardRef((_props, ref) => {
    useImperativeHandle(ref, () => ({
      openCreateModal: openCreateModalMock,
    }));
    // Espelha o contrato de foco real: container focável com role="grid".
    return (
      <div className="kanban-board" role="grid" tabIndex={0}>
        kanban-board
      </div>
    );
  }),
}));

vi.mock('./useCustomActions', () => ({
  useCustomActions: () => ({ runCustomAction: vi.fn() }),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((fulfill) => {
    resolve = fulfill;
  });
  return { promise, resolve };
}

describe('TaskListView', () => {
  beforeEach(() => {
    workspacePanelState.isActive = false;
    chatModalState.isOpen = false;
    chatModalState.boundTabId = null;
    chatModalState.boundConversationId = null;
    openCreateModalMock.mockReset();
    registerWorkspaceChatAdapterMock.mockReset();
    commandSurfaceGetterMock.mockReset();
    requestPagePresentationMock.mockReset();
    requestPagePresentationMock.mockReturnValue(true);
    requestPageMutationMock.mockReset();
    requestPageMutationMock.mockResolvedValue({
      status: 'succeeded',
      result: { id: 'tasklist-copy', title: 'Lista (Cópia)' },
    });
    pagePresentationOptionsMock.mockReset();
    pageMutationOptionsMock.mockReset();
    readTaskListCommandTargetMock.mockReset();
    readTaskListCommandTargetMock.mockResolvedValue({
      taskList: { title: 'Lista atômica', description: 'Descrição atual' },
      fingerprint: 'fingerprint-1',
    });
    workspaceStoreState.workspace = {
      id: 'workspace-1',
      profile: 'default',
      activeTabId: 'tasklist-tab',
      tabs: [workspacePanelState.tab],
    };
    announceMock.mockReset();
    taskListStoreState.loadTaskList.mockReset();
    taskListStoreState.loadMoreTasks.mockReset();
    taskListStoreState.loadAllTasksForBoard.mockReset();
    taskListStoreState.loadAllTasksForBoard.mockResolvedValue(205);
    taskListStoreState.cancelBoardTaskLoad.mockReset();
    taskListStoreState.taskPages = new Map([
      ['tasklist-1', { nextCursor: '', hasMore: false, totalCount: 0 }],
    ]);
    taskListStoreState.loadingByTaskListId = new Map();
    taskListStoreState.loadingTaskPagesByListId = new Map();
    taskListStoreState.taskPageLoadErrors = new Map();
    taskListStoreState.errors = new Map();
    taskListStoreState.clearError.mockReset();
    taskListStoreState.listBoardCustomActions.mockReset();
    taskListStoreState.listBoardCustomActions.mockResolvedValue([]);
    taskListStoreState.setTaskListConversation.mockReset();
    taskListStoreState.setTaskListConversation.mockResolvedValue(undefined);
    taskListStoreState.taskLists = new Map([
      ['tasklist-1', {
        id: 'tasklist-1',
        title: 'Lista',
        preferredViewMode: 'list',
        tasks: [],
        workflow: { id: 'workflow-1', taskListId: 'tasklist-1', statuses: [], allowedTransitions: {}, initialStatusId: 1 },
      }],
    ]);
  });

  it('getter relê retarget da mesma aba sem rerender ou Notify', () => {
    render(<TaskListView taskListId="tasklist-1" />);

    const getter = commandSurfaceGetterMock.mock.calls[
      commandSurfaceGetterMock.mock.calls.length - 1
    ]?.[0] as (() => unknown) | undefined;
    expect(getter).toBeDefined();
    expect(getter?.()).not.toBeNull();

    workspaceStoreState.workspace.tabs = [{
      ...workspacePanelState.tab,
      state: { tasklistId: 'tasklist-2' },
    }];

    expect(getter?.()).toBeNull();
  });

  it('carrega a primeira página quando o cache contém apenas metadados', async () => {
    taskListStoreState.taskPages = new Map();
    render(<TaskListView taskListId="tasklist-1" />);

    expect(screen.getByText('Carregando...')).toBeInTheDocument();
    await waitFor(() => {
      expect(taskListStoreState.loadTaskList).toHaveBeenCalledTimes(1);
      expect(taskListStoreState.loadTaskList).toHaveBeenCalledWith('tasklist-1');
    });
  });

  it('expõe erro da primeira página e permite retry acessível', async () => {
    const user = userEvent.setup();
    taskListStoreState.taskLists = new Map();
    taskListStoreState.taskPages = new Map();
    taskListStoreState.errors = new Map([
      ['loadTaskList:tasklist-1', 'falha transitória'],
    ]);
    render(<TaskListView taskListId="tasklist-1" />);

    expect(screen.getByText('falha transitória')).toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledWith('falha transitória', 'assertive');
    await user.click(screen.getByRole('button', { name: 'Tentar novamente' }));

    expect(taskListStoreState.clearError).toHaveBeenCalledWith('loadTaskList:tasklist-1');
    expect(taskListStoreState.loadTaskList).toHaveBeenCalledWith('tasklist-1');
  });

  it('não responde a atalhos globais quando o painel está inativo', async () => {
    const user = userEvent.setup();
    render(<TaskListView taskListId="tasklist-1" />);

    await user.keyboard('n');

    expect(openCreateModalMock).not.toHaveBeenCalled();
  });

  it('emite SurfaceContext canônico para o chat da tasklist', async () => {
    render(<TaskListView taskListId="tasklist-1" />);

    await waitFor(() => expect(registerWorkspaceChatAdapterMock).toHaveBeenCalled());
    const calls = registerWorkspaceChatAdapterMock.mock.calls;
    const adapter = calls[calls.length - 1]?.[1] as {
      send: (instruction: string) => Promise<{ paramsOverride?: { surfaceContextJson?: string } }>;
    };
    const plan = await adapter.send('Resuma a lista');
    const surfaceContext = JSON.parse(String(plan.paramsOverride?.surfaceContextJson || '{}'));

    expect(surfaceContext.surfaceType).toBe('tasklist');
    expect(surfaceContext.surfaceId).toBe('tasklist-tab');
    expect(surfaceContext.snapshotVersion).toMatch(/^tasklist:tasklist-tab:/);
  });

  it('libera o Kanban na primeira página enquanto carrega as demais', async () => {
    workspacePanelState.isActive = true;
    const backgroundLoad = deferred<number>();
    taskListStoreState.loadAllTasksForBoard.mockReturnValueOnce(backgroundLoad.promise);
    taskListStoreState.taskLists = new Map([
      ['tasklist-1', {
        id: 'tasklist-1',
        title: 'Board grande',
        preferredViewMode: 'kanban',
        tasks: Array.from({ length: 100 }, (_, index) => ({
          id: `task-${index + 1}`,
          taskListId: 'tasklist-1',
          title: `Card ${index + 1}`,
          statusId: 1,
          order: index,
        })),
        workflow: {
          id: 'workflow-1',
          taskListId: 'tasklist-1',
          statuses: [{ id: 1, order: 0, label: 'A fazer' }],
          allowedTransitions: {},
          initialStatusId: 1,
        },
      }],
    ]);
    taskListStoreState.taskPages = new Map([
      ['tasklist-1', { nextCursor: 'cursor-100', hasMore: true, totalCount: 205 }],
    ]);
    taskListStoreState.loadingTaskPagesByListId = new Map([
      ['tasklist-1', true],
    ]);

    render(<TaskListView taskListId="tasklist-1" />);

    expect(document.body).toHaveTextContent('kanban-board');
    await waitFor(() => {
      expect(taskListStoreState.loadAllTasksForBoard).toHaveBeenCalledTimes(1);
      expect(taskListStoreState.loadAllTasksForBoard).toHaveBeenCalledWith('tasklist-1');
    });
    expect(taskListStoreState.loadAllTasksForBoard.mock.results[0]?.value).toBe(backgroundLoad.promise);
    expect(announceMock).toHaveBeenCalledWith(
      '{{loaded}} de {{total}} cards carregados; o quadro já está navegável',
      'polite',
    );

    backgroundLoad.resolve(205);
    await waitFor(() => {
      expect(announceMock).toHaveBeenCalledWith('Quadro completo com {{count}} cards', 'polite');
    });
  });

  it('expõe retry sem remover o Kanban após falha de página posterior', async () => {
    workspacePanelState.isActive = true;
    taskListStoreState.taskLists = new Map([
      ['tasklist-1', {
        id: 'tasklist-1',
        title: 'Board parcial',
        preferredViewMode: 'kanban',
        tasks: [{ id: 'task-1', taskListId: 'tasklist-1', title: 'Card disponível', statusId: 1, order: 0 }],
        workflow: {
          id: 'workflow-1',
          taskListId: 'tasklist-1',
          statuses: [{ id: 1, order: 0, label: 'A fazer' }],
          allowedTransitions: {},
          initialStatusId: 1,
        },
      }],
    ]);
    taskListStoreState.taskPages = new Map([
      ['tasklist-1', { nextCursor: 'cursor-100', hasMore: true, totalCount: 205 }],
    ]);
    taskListStoreState.taskPageLoadErrors = new Map([
      ['tasklist-1', 'falha transitória'],
    ]);
    taskListStoreState.loadAllTasksForBoard.mockRejectedValueOnce(new Error('falha transitória'));

    render(<TaskListView taskListId="tasklist-1" />);

    expect(document.body).toHaveTextContent('kanban-board');
    expect(document.body).toHaveTextContent('Não foi possível carregar todos os cards');
    expect(document.body).toHaveTextContent('Tentar carregar cards restantes');
    await waitFor(() => {
      expect(announceMock).toHaveBeenCalledWith(
        'Não foi possível carregar todos os cards. Os cards disponíveis continuam navegáveis.',
        'polite',
      );
    });
  });

  it('não anuncia conclusão depois que o Kanban é desmontado', async () => {
    workspacePanelState.isActive = true;
    const backgroundLoad = deferred<number>();
    taskListStoreState.loadAllTasksForBoard.mockReturnValueOnce(backgroundLoad.promise);
    taskListStoreState.taskLists = new Map([
      ['tasklist-1', {
        id: 'tasklist-1',
        title: 'Board',
        preferredViewMode: 'kanban',
        tasks: [{ id: 'task-1', taskListId: 'tasklist-1', title: 'Card', statusId: 1, order: 0 }],
        workflow: {
          id: 'workflow-1',
          taskListId: 'tasklist-1',
          statuses: [{ id: 1, order: 0, label: 'A fazer' }],
          allowedTransitions: {},
          initialStatusId: 1,
        },
      }],
    ]);
    taskListStoreState.taskPages = new Map([
      ['tasklist-1', { nextCursor: 'cursor-100', hasMore: true, totalCount: 205 }],
    ]);

    const { unmount } = render(<TaskListView taskListId="tasklist-1" />);
    await waitFor(() => expect(taskListStoreState.loadAllTasksForBoard).toHaveBeenCalledTimes(1));
    announceMock.mockClear();

    unmount();
    backgroundLoad.resolve(205);
    await backgroundLoad.promise;
    await Promise.resolve();

    expect(announceMock).not.toHaveBeenCalled();
  });

  it('não anuncia progresso ou conclusão enquanto o painel está inativo', async () => {
    const backgroundLoad = deferred<number>();
    taskListStoreState.loadAllTasksForBoard.mockReturnValueOnce(backgroundLoad.promise);
    taskListStoreState.taskLists = new Map([
      ['tasklist-1', {
        id: 'tasklist-1',
        title: 'Board oculto',
        preferredViewMode: 'kanban',
        tasks: [{ id: 'task-1', taskListId: 'tasklist-1', title: 'Card', statusId: 1, order: 0 }],
        workflow: {
          id: 'workflow-1',
          taskListId: 'tasklist-1',
          statuses: [{ id: 1, order: 0, label: 'A fazer' }],
          allowedTransitions: {},
          initialStatusId: 1,
        },
      }],
    ]);
    taskListStoreState.taskPages = new Map([
      ['tasklist-1', { nextCursor: 'cursor-100', hasMore: true, totalCount: 205 }],
    ]);
    taskListStoreState.loadingTaskPagesByListId = new Map([['tasklist-1', true]]);

    render(<TaskListView taskListId="tasklist-1" />);
    await Promise.resolve();

    expect(taskListStoreState.loadAllTasksForBoard).not.toHaveBeenCalled();
    expect(announceMock).not.toHaveBeenCalled();
    backgroundLoad.resolve(205);
  });

  it('não anuncia conclusão se o usuário trocar de painel durante a carga', async () => {
    workspacePanelState.isActive = true;
    const backgroundLoad = deferred<number>();
    taskListStoreState.loadAllTasksForBoard.mockReturnValueOnce(backgroundLoad.promise);
    taskListStoreState.taskLists = new Map([
      ['tasklist-1', {
        id: 'tasklist-1',
        title: 'Board',
        preferredViewMode: 'kanban',
        tasks: [{ id: 'task-1', taskListId: 'tasklist-1', title: 'Card', statusId: 1, order: 0 }],
        workflow: {
          id: 'workflow-1',
          taskListId: 'tasklist-1',
          statuses: [{ id: 1, order: 0, label: 'A fazer' }],
          allowedTransitions: {},
          initialStatusId: 1,
        },
      }],
    ]);
    taskListStoreState.taskPages = new Map([
      ['tasklist-1', { nextCursor: 'cursor-100', hasMore: true, totalCount: 205 }],
    ]);

    const { rerender } = render(<TaskListView taskListId="tasklist-1" />);
    await waitFor(() => expect(taskListStoreState.loadAllTasksForBoard).toHaveBeenCalledTimes(1));
    announceMock.mockClear();

    workspacePanelState.isActive = false;
    rerender(<TaskListView taskListId="tasklist-1" />);
    backgroundLoad.resolve(205);
    await backgroundLoad.promise;
    await Promise.resolve();

    expect(announceMock).not.toHaveBeenCalled();
  });

  it('encaminha os gestos locais N e D aos requests comuns quando o painel está ativo', async () => {
    const user = userEvent.setup();
    workspacePanelState.isActive = true;
    taskListStoreState.taskLists = new Map([[
      'tasklist-1', {
        id: 'tasklist-1',
        title: 'Lista',
        preferredViewMode: 'list',
        tasks: [{ id: 'task-1', title: 'Tarefa' }],
        taskCount: 1,
        workflow: { id: 'workflow-1', taskListId: 'tasklist-1', statuses: [], allowedTransitions: {}, initialStatusId: 1 },
      },
    ]]);
    render(<TaskListView taskListId="tasklist-1" />);

    screen.getByTestId('tasklist-surface-focus').focus();
    await user.keyboard('n');
    await user.keyboard('d');

    expect(openCreateModalMock).not.toHaveBeenCalled();
    expect(requestPagePresentationMock).toHaveBeenCalledWith('tasklist.task.create.open');
    expect(requestPageMutationMock).toHaveBeenCalledWith('tasklists.duplicate');
  });

  it('encaminha Ctrl+L também em editável interno, mas respeita consumo prévio e escopo do root', () => {
    workspacePanelState.isActive = true;
    taskListStoreState.taskLists = new Map([[
      'tasklist-1', {
        id: 'tasklist-1',
        title: 'Lista',
        preferredViewMode: 'list',
        tasks: [{ id: 'task-1', title: 'Tarefa' }],
        taskCount: 1,
        workflow: { id: 'workflow-1', taskListId: 'tasklist-1', statuses: [], allowedTransitions: {}, initialStatusId: 1 },
      },
    ]]);
    render(<TaskListView taskListId="tasklist-1" />);

    const input = screen.getByRole('textbox', { name: 'campo da lista' });
    input.focus();
    const consumed = new KeyboardEvent('keydown', { key: 'l', ctrlKey: true, bubbles: true, cancelable: true });
    consumed.preventDefault();
    input.dispatchEvent(consumed);
    expect(requestPageMutationMock).not.toHaveBeenCalled();

    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'l', ctrlKey: true, bubbles: true, cancelable: true }));
    expect(requestPageMutationMock).toHaveBeenCalledWith('tasklists.clear');

    requestPageMutationMock.mockClear();
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'l', ctrlKey: true, bubbles: true, cancelable: true }));
    expect(requestPageMutationMock).not.toHaveBeenCalled();
  });

  it('mantém Limpar elegível quando o catálogo informa tarefas fora da página carregada', () => {
    taskListStoreState.taskLists = new Map([[
      'tasklist-1', {
        id: 'tasklist-1',
        title: 'Lista paginada',
        preferredViewMode: 'list',
        tasks: [],
        taskCount: 4,
        workflow: { id: 'workflow-1', taskListId: 'tasklist-1', statuses: [], allowedTransitions: {}, initialStatusId: 1 },
      },
    ]]);
    render(<TaskListView taskListId="tasklist-1" />);

    expect(screen.getByRole('button', { name: 'Limpar' })).toBeEnabled();
  });

  it('registra canStart/prepare contra aba ativa, retarget e reload sem perder o fingerprint', async () => {
    workspacePanelState.isActive = true;
    taskListStoreState.taskLists = new Map([[
      'tasklist-1', {
        id: 'tasklist-1',
        title: 'Lista cacheada',
        preferredViewMode: 'list',
        tasks: [],
        taskCount: 4,
        workflow: { id: 'workflow-1', taskListId: 'tasklist-1', statuses: [], allowedTransitions: {}, initialStatusId: 1 },
      },
    ]]);
    render(<TaskListView taskListId="tasklist-1" />);

    const options = pageMutationOptionsMock.mock.calls[pageMutationOptionsMock.mock.calls.length - 1]?.[0] as {
      canStart: (id: 'tasklists.clear' | 'tasklists.duplicate') => boolean;
      prepare: (id: 'tasklists.clear' | 'tasklists.duplicate') => {
        readRequest: () => Promise<unknown>;
        isCurrent: () => boolean;
      } | undefined;
    };
    expect(options.canStart('tasklists.clear')).toBe(true);
    const prepared = options.prepare('tasklists.duplicate');
    expect(prepared).toBeDefined();
    await expect(prepared?.readRequest()).resolves.toEqual({
      targetId: 'tasklist-1',
      expectedFingerprint: 'fingerprint-1',
      title: 'Lista atômica (Cópia)',
      description: 'Descrição atual',
    });
    expect(readTaskListCommandTargetMock).toHaveBeenCalledWith('tasklist-1');

    workspaceStoreState.workspace = {
      ...workspaceStoreState.workspace,
      tabs: [{ ...workspacePanelState.tab, state: { tasklistId: 'tasklist-2' } }],
    };
    expect(options.canStart('tasklists.duplicate')).toBe(false);
    expect(prepared?.isCurrent()).toBe(false);

    workspaceStoreState.workspace = {
      ...workspaceStoreState.workspace,
      tabs: [workspacePanelState.tab],
    };
    taskListStoreState.taskLists = new Map([[
      'tasklist-1', {
        id: 'tasklist-1',
        title: 'Lista recarregada',
        preferredViewMode: 'list',
        tasks: [],
        taskCount: 4,
        workflow: { id: 'workflow-1', taskListId: 'tasklist-1', statuses: [], allowedTransitions: {}, initialStatusId: 1 },
      },
    ]]);
    expect(prepared?.isCurrent()).toBe(false);
  });

  it('auto-vincula a lista à conversa do chat embutido quando o modal abre nesta aba', async () => {
    chatModalState.isOpen = true;
    chatModalState.boundTabId = 'tasklist-tab';
    chatModalState.boundConversationId = '9';
    render(<TaskListView taskListId="tasklist-1" />);

    await waitFor(() =>
      expect(taskListStoreState.setTaskListConversation).toHaveBeenCalledWith('tasklist-1', '9'),
    );
  });

  it('não auto-vincula quando o chat embutido está atrelado a outra aba', async () => {
    chatModalState.isOpen = true;
    chatModalState.boundTabId = 'outra-aba';
    chatModalState.boundConversationId = '9';
    render(<TaskListView taskListId="tasklist-1" />);

    await Promise.resolve();
    expect(taskListStoreState.setTaskListConversation).not.toHaveBeenCalled();
  });

  function kanbanListReady() {
    taskListStoreState.taskLists = new Map([
      ['tasklist-1', {
        id: 'tasklist-1',
        title: 'Board',
        preferredViewMode: 'kanban',
        tasks: [{ id: 'task-1', taskListId: 'tasklist-1', title: 'Card', statusId: 1, order: 0 }],
        workflow: {
          id: 'workflow-1',
          taskListId: 'tasklist-1',
          statuses: [{ id: 1, order: 0, label: 'A fazer' }],
          allowedTransitions: {},
          initialStatusId: 1,
        },
      }],
    ]);
    taskListStoreState.taskPages = new Map([
      ['tasklist-1', { nextCursor: '', hasMore: false, totalCount: 1 }],
    ]);
  }

  it('registra handler de foco de painel e foca a área default do board quando pronto', async () => {
    workspacePanelState.isActive = true;
    kanbanListReady();
    render(<TaskListView taskListId="tasklist-1" />);

    // Simula o WorkspaceLayout roteando o foco para este painel.
    let accepted = false;
    act(() => {
      accepted = requestWorkspacePanelFocus('tasklist-tab');
    });
    expect(accepted).toBe(true);

    await waitFor(() => {
      expect(document.querySelector('.kanban-board')).toHaveFocus();
    });
  });

  it('expõe capability imediata real, foca no mesmo tick e recusa modal, inatividade e readiness', () => {
    workspacePanelState.isActive = true;
    kanbanListReady();
    const { rerender } = render(<TaskListView taskListId="tasklist-1" />);
    const immediate = getWorkspacePanelImmediateFocusHandler('tasklist-tab');
    const board = document.querySelector('.kanban-board');
    expect(immediate).toBeDefined();
    expect(board).toBeTruthy();
    expect(canFocusWorkspacePanelImmediately('tasklist-tab')).toBe(true);

    expect(immediate?.()).toBe(true);
    expect(document.activeElement).toBe(board);

    chatModalState.isOpen = true;
    expect(canFocusWorkspacePanelImmediately('tasklist-tab')).toBe(false);
    expect(immediate?.()).toBe(false);
    chatModalState.isOpen = false;

    workspacePanelState.isActive = false;
    rerender(<TaskListView taskListId="tasklist-1" />);
    expect(canFocusWorkspacePanelImmediately('tasklist-tab')).toBe(false);
    expect(immediate?.()).toBe(false);
  });

  it('recusa capability imediata enquanto a lista ainda não está pronta', () => {
    workspacePanelState.isActive = true;
    taskListStoreState.taskLists = new Map();
    taskListStoreState.taskPages = new Map();
    render(<TaskListView taskListId="tasklist-1" />);
    const immediate = getWorkspacePanelImmediateFocusHandler('tasklist-tab');
    expect(immediate).toBeDefined();
    expect(canFocusWorkspacePanelImmediately('tasklist-tab')).toBe(false);
    expect(immediate?.()).toBe(false);
  });

  it('adia o foco do painel até o board terminar de carregar', async () => {
    workspacePanelState.isActive = true;
    // Estado inicial: sem taskList/taskPage → tela "Carregando...".
    taskListStoreState.taskLists = new Map();
    taskListStoreState.taskPages = new Map();
    const { rerender } = render(<TaskListView taskListId="tasklist-1" />);

    expect(screen.getByText('Carregando...')).toBeInTheDocument();

    // Pedido de foco chega durante o carregamento: aceito, mas ainda sem board.
    let accepted = false;
    act(() => {
      accepted = requestWorkspacePanelFocus('tasklist-tab');
    });
    expect(accepted).toBe(true);
    await Promise.resolve();
    expect(document.querySelector('.kanban-board')).toBeNull();

    // Board carrega: o pedido pendente é atendido sem nova interação.
    kanbanListReady();
    rerender(<TaskListView taskListId="tasklist-1" />);

    await waitFor(() => {
      expect(document.querySelector('.kanban-board')).toHaveFocus();
    });
  });

  it('não atende pedido de foco quando o painel está inativo', async () => {
    workspacePanelState.isActive = false;
    kanbanListReady();
    render(<TaskListView taskListId="tasklist-1" />);

    let accepted = true;
    act(() => {
      accepted = requestWorkspacePanelFocus('tasklist-tab');
    });
    expect(accepted).toBe(false);
    await Promise.resolve();
    expect(document.querySelector('.kanban-board')).not.toHaveFocus();
  });

  it('não re-vincula quando a lista já aponta para a conversa do chat', async () => {
    taskListStoreState.taskLists = new Map([
      ['tasklist-1', {
        id: 'tasklist-1',
        title: 'Lista',
        preferredViewMode: 'list',
        conversationId: '9',
        tasks: [],
        workflow: { id: 'workflow-1', taskListId: 'tasklist-1', statuses: [], allowedTransitions: {}, initialStatusId: 1 },
      }],
    ]);
    chatModalState.isOpen = true;
    chatModalState.boundTabId = 'tasklist-tab';
    chatModalState.boundConversationId = '9';
    render(<TaskListView taskListId="tasklist-1" />);

    await Promise.resolve();
    expect(taskListStoreState.setTaskListConversation).not.toHaveBeenCalled();
  });
});
