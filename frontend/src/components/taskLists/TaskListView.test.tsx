import { forwardRef, useImperativeHandle, type ReactNode } from 'react';
import { describe, expect, it, beforeEach, vi } from 'vitest';
import { act, render, screen, fireEvent, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { WorkspaceTab } from '../../store/workspaceStore';
import TaskListView from './TaskListView';
import { requestWorkspacePanelFocus } from '../workspace/workspacePanelFocusRegistry';
import { DATAGRID_ENTRY_SELECTOR } from '../ui/DataGrid';
import { enqueueSave, taskListWorkflowSaveKey } from '../../lib/serialSaveQueue';

const openCreateModalMock = vi.fn();
const registerWorkspaceChatAdapterMock = vi.hoisted(() => vi.fn());
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
  updateTaskList: vi.fn(),
}));

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
}));

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
  }),
}));

vi.mock('../workspace/WorkspacePanelContext', () => ({
  useWorkspacePanel: () => workspacePanelState,
}));

vi.mock('../../store/workspaceStore', () => ({
  useWorkspaceStore: (selector: (state: { workspace: { profile: string } }) => unknown) => selector({
    workspace: { profile: 'default' },
  }),
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
  (useStore as unknown as { getState: () => unknown }).getState = () => ({ requestOpen: vi.fn() });
  return { useWorkspaceChatModalStore: useStore };
});

vi.mock('../../store/uiStore', () => ({
  useUIStore: (selector: (state: { addToast: ReturnType<typeof vi.fn> }) => unknown) => selector({
    addToast: toastMock,
  }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceMock }),
}));

const confirmMock = vi.hoisted(() => vi.fn());
const toastMock = vi.hoisted(() => vi.fn());

vi.mock('../../hooks/useConfirm', () => ({
  useConfirm: () => confirmMock,
}));

vi.mock('../../hooks/useDefaultFocus', () => ({
  registerDefaultFocus: vi.fn(),
  unregisterDefaultFocus: vi.fn(),
  restoreDefaultFocus: vi.fn(),
}));

vi.mock('../../hooks/useRegisterWorkspaceChatAdapter', () => ({
  useRegisterWorkspaceChatAdapter: registerWorkspaceChatAdapterMock,
}));

vi.mock('../ui/Modal', () => ({
  isModalOpen: () => false,
  Modal: ({ children, title, initialFocusSelector }: {
    children: ReactNode;
    title?: string;
    initialFocusSelector?: string;
  }) => (
    <div role="dialog" aria-label={title} data-initial-focus={initialFocusSelector}>{children}</div>
  ),
}));

vi.mock('./CustomActionsEditor', () => ({ default: () => <div>custom-actions-editor</div> }));
vi.mock('./WorkflowEditor', () => ({ default: () => <div>workflow-editor</div> }));

vi.mock('../ui/Toolbar', () => ({
  Toolbar: ({
    actions,
    rightEnd,
  }: {
    actions?: Array<{ key: string; label: string; onClick?: () => void }>;
    rightEnd?: ReactNode;
  }) => (
    <div>
      {actions?.map((action) => (
        <button key={action.key} type="button" onClick={action.onClick}>
          {action.label}
        </button>
      ))}
      {rightEnd}
    </div>
  ),
}));

vi.mock('./TasksTable', () => ({
  default: forwardRef((_props, ref) => {
    useImperativeHandle(ref, () => ({
      openCreateModal: openCreateModalMock,
    }));
    return <div>tasks-table</div>;
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
    announceMock.mockReset();
    toastMock.mockReset();
    confirmMock.mockReset();
    confirmMock.mockResolvedValue(false);
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
    taskListStoreState.updateTaskList.mockReset();
    taskListStoreState.updateTaskList.mockResolvedValue(undefined);
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

  it('responde a atalhos globais quando o painel está ativo', async () => {
    const user = userEvent.setup();
    workspacePanelState.isActive = true;
    render(<TaskListView taskListId="tasklist-1" />);

    await user.keyboard('n');

    expect(openCreateModalMock).toHaveBeenCalledTimes(1);
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

  it('menu Configurações reúne edição, workflow, ações, duplicar, limpar e apagar', async () => {
    const user = userEvent.setup();
    render(<TaskListView taskListId="tasklist-1" />);
    await user.click(screen.getByRole('button', { name: 'Configurações' }));

    for (const name of ['Editar Lista', 'Editar Workflow', 'Ações customizadas', /Duplicar/, /Limpar/, 'Apagar']) {
      expect(await screen.findByRole('menuitem', { name })).toBeInTheDocument();
    }
    // Sem vínculo manual (AEP-0073): o vínculo da lista é só via chat embutido.
    expect(screen.queryByRole('menuitem', { name: /conversa/i })).not.toBeInTheDocument();
    // Ações movidas para o menu não poluem mais a toolbar.
    expect(screen.queryByRole('button', { name: 'Editar Workflow' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Ações customizadas' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Duplicar' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Nova Tarefa' })).toBeInTheDocument();
  });

  it.each([
    ['Ações customizadas', 'custom-actions-editor'],
    ['Editar Workflow', 'workflow-editor'],
  ])('modal "%s" aberto pelo menu pede foco inicial no grid', async (name, content) => {
    taskListStoreState.getTaskCountsByStatus.mockResolvedValue({});
    const user = userEvent.setup();
    render(<TaskListView taskListId="tasklist-1" />);
    await user.click(screen.getByRole('button', { name: 'Configurações' }));
    await user.click(await screen.findByRole('menuitem', { name }));

    const dialog = await screen.findByRole('dialog', { name });
    expect(dialog).toHaveAttribute('data-initial-focus', DATAGRID_ENTRY_SELECTOR);
    expect(await screen.findByText(content)).toBeInTheDocument();
  });

  it('reabrir o workflow com um salvamento em voo espera ele terminar antes de ler os dados', async () => {
    taskListStoreState.getTaskCountsByStatus.mockResolvedValue({});
    let resolveSave!: () => void;
    void enqueueSave(taskListWorkflowSaveKey('tasklist-1'), () => new Promise<void>((res) => { resolveSave = res; }));
    const user = userEvent.setup();
    render(<TaskListView taskListId="tasklist-1" />);
    await user.click(screen.getByRole('button', { name: 'Configurações' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Editar Workflow' }));

    await new Promise<void>((r) => { window.setTimeout(r, 20); });
    expect(taskListStoreState.getTaskCountsByStatus).not.toHaveBeenCalled();
    expect(screen.queryByText('workflow-editor')).not.toBeInTheDocument();

    resolveSave();
    expect(await screen.findByText('workflow-editor')).toBeInTheDocument();
    expect(taskListStoreState.getTaskCountsByStatus).toHaveBeenCalledTimes(1);
  });

  it('edita título e descrição da lista pelo menu', async () => {
    const user = userEvent.setup();
    render(<TaskListView taskListId="tasklist-1" />);
    await user.click(screen.getByRole('button', { name: 'Configurações' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Editar Lista' }));

    const titleInput = await screen.findByLabelText(/Título/);
    expect(titleInput).toHaveValue('Lista');
    fireEvent.change(titleInput, { target: { value: 'Lista Nova' } });
    fireEvent.change(screen.getByLabelText('Descrição'), { target: { value: 'Desc' } });
    await user.click(screen.getByRole('button', { name: 'Salvar' }));

    await waitFor(() => expect(taskListStoreState.updateTaskList).toHaveBeenCalledWith('tasklist-1', 'Lista Nova', 'Desc'));
    expect(announceMock).toHaveBeenCalledWith('Lista atualizada');
  });

  it('mostra erro e mantém o modal aberto quando salvar a lista falha', async () => {
    const user = userEvent.setup();
    taskListStoreState.updateTaskList.mockRejectedValueOnce(new Error('falha no backend'));
    render(<TaskListView taskListId="tasklist-1" />);
    await user.click(screen.getByRole('button', { name: 'Configurações' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Editar Lista' }));

    fireEvent.change(await screen.findByLabelText(/Título/), { target: { value: 'Outro' } });
    await user.click(screen.getByRole('button', { name: 'Salvar' }));

    await waitFor(() => expect(toastMock).toHaveBeenCalledWith('falha no backend', 'error'));
    expect(toastMock).not.toHaveBeenCalledWith('Lista atualizada', expect.anything(), expect.anything(), expect.anything(), expect.anything());
    // Sem sucesso: o modal segue aberto para corrigir e tentar de novo.
    expect(screen.getByLabelText(/Título/)).toBeInTheDocument();
  });

  it('apaga a lista pelo menu com confirmação', async () => {
    const user = userEvent.setup();
    confirmMock.mockResolvedValue(true);
    render(<TaskListView taskListId="tasklist-1" />);
    await user.click(screen.getByRole('button', { name: 'Configurações' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Apagar' }));

    await waitFor(() => expect(taskListStoreState.deleteTaskList).toHaveBeenCalledWith('tasklist-1'));
  });
});
