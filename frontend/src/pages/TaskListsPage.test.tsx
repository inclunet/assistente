import { afterEach, describe, expect, it, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { capturePagePresentationTarget, PAGE_PRESENTATION_COMMAND_EVENT } from '../lib/commandPagePresentation';
import type { PageMutationID, usePageMutationCommands } from '../lib/commandPageMutation';
type PageMutationOptions = Parameters<typeof usePageMutationCommands>[0];

/* ── mock fns (definidas antes dos vi.mock) ────────────────── */

const mockCreateTaskList = vi.fn();
const mockUpdateTaskList = vi.fn();
const mockDeleteTaskList = vi.fn();
const mockCloneTaskList = vi.fn();
const mockGetCachedTaskList = vi.fn();
const mockLoadTaskList = vi.fn();
const mockFetchAllTaskLists = vi.fn();
const mockAddTab = vi.fn().mockResolvedValue('tab-1');
const mockMoveTabToWorkspace = vi.fn().mockResolvedValue(undefined);
const mockAddToast = vi.fn();
const mockAnnounce = vi.fn();
const mockRequestConfirm = vi.fn();
const mockExecuteDeepLink = vi.fn().mockResolvedValue(undefined);
const mockReadTaskListCommandTarget = vi.fn();
const mockRequestFormPageMutation = vi.fn();
const mockRequestRootPageMutation = vi.fn();
let formMutationOptions: PageMutationOptions;
let rootMutationOptions: PageMutationOptions;

/* ── mocks de módulos ──────────────────────────────────────── */

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
  }),
}));

vi.mock('../lib/deepLinks', () => ({
  executeDeepLink: (...args: unknown[]) => mockExecuteDeepLink(...args),
}));

vi.mock('../lib/commandPageMutationWails', () => ({
  readTaskListCommandTarget: (...args: unknown[]) => mockReadTaskListCommandTarget(...args),
}));

vi.mock('../lib/commandPageMutation', () => ({
  usePageMutationCommands: (options: PageMutationOptions) => {
    if (options.allowedCommands.includes('tasklists.create')) {
      formMutationOptions = options;
      return { request: mockRequestFormPageMutation };
    }
    rootMutationOptions = options;
    return { request: mockRequestRootPageMutation };
  },
}));

vi.mock('@wailsjs/go/wailsapi/LLMProviders', () => ({
  GetLLMProvidersWithStatus: vi.fn().mockResolvedValue([]),
}));

vi.mock('../store/authStore', () => ({
  useAuthStore: Object.assign((selector?: (state: Record<string, unknown>) => unknown) => {
    const state = { isAuthenticated: true, user: { userId: 'user-1', sessionId: 'session-1' } };
    return selector ? selector(state) : state;
  }, {
    getState: () => ({ isAuthenticated: true, user: { userId: 'user-1', sessionId: 'session-1' } }),
    subscribe: () => () => {},
  }),
}));

let storeTaskLists: Map<string, unknown> = new Map();

vi.mock('../store/taskListStore', () => ({
  useTaskListStore: Object.assign(
    (selector?: (state: Record<string, unknown>) => unknown) => {
      const state: Record<string, unknown> = {
        taskLists: storeTaskLists,
        createTaskList: mockCreateTaskList,
        updateTaskList: mockUpdateTaskList,
        deleteTaskList: mockDeleteTaskList,
        cloneTaskList: mockCloneTaskList,
        getCachedTaskList: mockGetCachedTaskList,
        loadTaskList: mockLoadTaskList,
        fetchAllTaskLists: mockFetchAllTaskLists,
      };
      if (selector) return selector(state);
      return state;
    },
    {
      getState: () => ({
        taskLists: storeTaskLists,
        getCachedTaskList: mockGetCachedTaskList,
        loadTaskList: mockLoadTaskList,
      }),
      subscribe: () => () => {},
    },
  ),
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: Object.assign((selector?: (state: Record<string, unknown>) => unknown) => {
    const state: Record<string, unknown> = {
      addTab: mockAddTab,
      moveTabToWorkspace: mockMoveTabToWorkspace,
      workspaces: [],
      workspace: { id: 'workspace-1', activeTabId: null },
    };
    if (selector) return selector(state);
    return state;
  }, {
    getState: () => ({ workspace: { id: 'workspace-1', activeTabId: null } }),
    subscribe: () => () => {},
  }),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: (selector?: (s: Record<string, unknown>) => unknown) => {
    const s = { addToast: mockAddToast };
    return selector ? selector(s) : s;
  },
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: mockAnnounce,
  }),
}));

vi.mock('../hooks/useGridFocus', () => ({
  useGridFocus: () => ({
    handleGridReady: vi.fn(),
  }),
}));

vi.mock('../hooks/useGridPageLandmarks', () => ({
  useGridPageLandmarks: vi.fn(),
}));

vi.mock('../hooks/useConfirm', () => ({
  useConfirm: () => mockRequestConfirm,
}));

/* ── mocks de componentes UI ───────────────────────────────── */

vi.mock('../components/ui/Modal', () => ({
  Modal: ({ isOpen, children, title }: { isOpen: boolean; children: ReactNode; title?: string }) =>
    isOpen ? (
      <div data-testid="modal">
        {title && <h2>{title}</h2>}
        {children}
      </div>
    ) : null,
  isModalOpen: () => false,
}));

vi.mock('../components/ui/Button', () => ({
  Button: ({ onClick, children, loading, ...rest }: { onClick?: () => void; children?: ReactNode; loading?: boolean }) => (
    <button onClick={onClick} disabled={loading} {...rest}>
      {children}
    </button>
  ),
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: Object.assign(
    ({ left, actions, searchValue, onSearchChange, searchPlaceholder }: {
      left?: ReactNode;
      actions?: Array<{ key: string; label: string; onClick: () => void; disabled?: boolean }>;
      searchValue?: string;
      onSearchChange?: (value: string) => void;
      searchPlaceholder?: string;
    }) => (
      <div data-testid="toolbar">
        {left}
        {onSearchChange && (
          <input
            className="toolbar__search"
            aria-label={searchPlaceholder}
            value={searchValue}
            onChange={(e) => onSearchChange(e.target.value)}
          />
        )}
        <div data-testid="toolbar-actions">
          {actions?.map((a) => (
            <button key={a.key} onClick={a.onClick} disabled={a.disabled}>
              {a.label}
            </button>
          ))}
        </div>
      </div>
    ),
    { displayName: 'Toolbar' },
  ),
}));

vi.mock('../components/ui/DataGrid', () => ({
  DataGrid: ({
    items,
    getRowActions,
    onActivate,
    onFocusChange,
  }: {
    items?: Array<{ id: number; title: string }>;
    getRowActions?: (item: { id: number; title: string }) => Array<{ id: string; label: string; onClick?: () => void }>;
    onActivate?: (item: { id: number; title: string }) => void;
    onFocusChange?: (item: { id: number; title: string } | null) => void;
  }) => (
    <div data-testid="data-grid">
      {items?.map((item) => (
        <div key={item.id} data-testid={`row-${item.id}`}>
          <span
            onClick={() => {
              onFocusChange?.(item);
              onActivate?.(item);
            }}
          >
            {item.title}
          </span>
          {getRowActions?.(item)?.map((a) => (
            <button key={a.id} onClick={a.onClick}>
              {a.label}
            </button>
          ))}
        </div>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/EditorPanel', () => ({
  EditorPanelFooter: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock('../components/ui/FormField', () => ({
  FormField: ({ children, label }: { children: ReactNode; label: string }) => (
    <div>
      <label>{label}</label>
      {children}
    </div>
  ),
}));

vi.mock('../components/ui/Input', () => ({
  Input: (props: React.InputHTMLAttributes<HTMLInputElement>) => <input {...props} />,
}));

vi.mock('../components/ui/Textarea', () => ({
  Textarea: (props: React.TextareaHTMLAttributes<HTMLTextAreaElement>) => <textarea {...props} />,
}));

vi.mock('../components/layout/MenuButton', () => ({
  MenuButton: () => <button data-testid="menu-button">⋮</button>,
}));

/* ── dados de teste ─────────────────────────────────────────── */

const baseWorkflow = {
  id: 1,
  taskListId: 1,
  statuses: [
    { id: 1, order: 0, label: 'Todo', color: 'gray', icon: '⬜' },
  ],
  allowedTransitions: {},
  initialStatusId: 1,
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-01T00:00:00Z',
};

const makeTaskList = (id: string, title: string) => ({
  id,
  title,
  description: `Descrição de ${title}`,
  preferredViewMode: 'list' as const,
  createdAt: '2024-06-01T10:00:00Z',
  updatedAt: '2024-06-01T10:00:00Z',
  workflow: { ...baseWorkflow, id, taskListId: id },
  tasks: [],
});

/* ── suíte ──────────────────────────────────────────────────── */

import TaskListsPage from './TaskListsPage';

describe('TaskListsPage', { timeout: 60_000 }, () => {
  const realPresentationRegistry = (event: Event) => {
    const detail = (event as CustomEvent<{ commandID?: unknown; instanceId?: unknown }>).detail;
    if (typeof detail?.commandID !== 'string' || typeof detail.instanceId !== 'string') return;
    const target = capturePagePresentationTarget(() => '/tasklists', detail.commandID, detail.instanceId);
    if (!target) return;
    try {
      if (target.open(detail.commandID)) event.preventDefault();
    } finally {
      target.dispose();
    }
  };

  beforeEach(() => {
    cleanup();
    vi.clearAllMocks();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    window.addEventListener(PAGE_PRESENTATION_COMMAND_EVENT, realPresentationRegistry);

    storeTaskLists = new Map([
      ['1', makeTaskList('1', 'Lista Alfa')],
      ['2', makeTaskList('2', 'Lista Beta')],
    ]);

    mockCreateTaskList.mockResolvedValue(makeTaskList('3', 'Lista Nova'));
    mockUpdateTaskList.mockResolvedValue(undefined);
    mockDeleteTaskList.mockResolvedValue(undefined);
    mockCloneTaskList.mockResolvedValue(makeTaskList('4', 'Lista Alfa (Cópia)'));
    mockGetCachedTaskList.mockImplementation((id: string) => storeTaskLists.get(id));
    mockLoadTaskList.mockImplementation(async (id: string) => storeTaskLists.get(id) ?? null);
    mockFetchAllTaskLists.mockResolvedValue([]);
    mockAddTab.mockResolvedValue('tab-1');
    mockRequestConfirm.mockResolvedValue(true);
    mockReadTaskListCommandTarget.mockImplementation(async (id: string) => ({
      taskList: storeTaskLists.get(id),
      fingerprint: `fingerprint-${id}`,
    }));
    const executePrepared = async (options: PageMutationOptions, commandId: PageMutationID) => {
      if (!options?.canStart(commandId)) return { status: 'failed' };
      const prepared = options?.prepare(commandId);
      if (!prepared) return { status: 'failed' };
      const request = await prepared.readRequest();
      if (!prepared.isCurrent()) return { status: 'cancelled' };
      const result = commandId === 'tasklists.create'
        ? { id: '3', title: request.title }
        : commandId === 'tasklists.duplicate'
          ? { id: '4', title: request.title }
          : { id: request.targetId, title: request.title };
      await prepared.succeeded?.(result);
      return { status: 'succeeded', result };
    };
    mockRequestFormPageMutation.mockImplementation((commandId: PageMutationID) => executePrepared(formMutationOptions, commandId));
    mockRequestRootPageMutation.mockImplementation((commandId: PageMutationID) => executePrepared(rootMutationOptions, commandId));
  });

  afterEach(() => {
    window.removeEventListener(PAGE_PRESENTATION_COMMAND_EVENT, realPresentationRegistry);
    vi.restoreAllMocks();
  });

  async function renderPage() {
    return render(
      <MemoryRouter initialEntries={['/tasklists']}>
        <TaskListsPage />
      </MemoryRouter>
    );
  }

  it('renderiza a listagem de listas', async () => {
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Lista Alfa')).toBeInTheDocument();
      expect(screen.getByText('Lista Beta')).toBeInTheDocument();
    });
    expect(mockFetchAllTaskLists).toHaveBeenCalledTimes(1);
    expect(mockLoadTaskList).not.toHaveBeenCalled();
  });

  it('mostra estado vazio quando não há listas', async () => {
    storeTaskLists = new Map();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Nenhuma lista de tarefas criada')).toBeInTheDocument();
    });
  });

  it('abre modal de criação ao clicar em Nova Lista', async () => {
    const user = userEvent.setup();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Lista Alfa')).toBeInTheDocument();
    });

    const newBtn = screen.getByRole('button', { name: 'Nova Lista' });
    await user.click(newBtn);

    expect(screen.getByTestId('modal')).toBeInTheDocument();
    expect(screen.getByText('Criar Nova Lista')).toBeInTheDocument();
  });

  it('cria lista e abre em nova aba do workspace ao salvar', async () => {
    const user = userEvent.setup();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Lista Alfa')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: 'Nova Lista' }));

    const titleInput = screen.getByPlaceholderText('Título da lista');
    await user.type(titleInput, 'Lista Nova');

    const saveBtn = screen.getByRole('button', { name: 'Salvar' });
    await user.click(saveBtn);

    await waitFor(() => {
      expect(mockRequestFormPageMutation).toHaveBeenCalledWith('tasklists.create');
      expect(mockReadTaskListCommandTarget).not.toHaveBeenCalled();
    });

    await waitFor(() => {
      expect(mockExecuteDeepLink).toHaveBeenCalledWith(
        { type: 'tab:open', tabType: 'tasklist', contentId: '3', title: 'Lista Nova' },
        expect.objectContaining({ navigate: expect.any(Function) }),
      );
    });

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith(
        expect.stringContaining('Lista Nova'),
        'success',
        undefined,
        undefined,
        { suppressAnnounce: true },
      );
    });
  });

  it('atualiza metadata da lista em edição e mostra sucesso', async () => {
    const user = userEvent.setup();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Lista Beta')).toBeInTheDocument();
    });

    await user.click(within(screen.getByTestId('row-2')).getByRole('button', { name: 'Editar' }));
    await waitFor(() => expect(screen.getByDisplayValue('Lista Beta')).toBeInTheDocument());
    const titleInput = screen.getByDisplayValue('Lista Beta');
    await user.clear(titleInput);
    await user.type(titleInput, 'Lista Beta Atualizada');
    await user.click(screen.getByRole('button', { name: 'Salvar' }));

    await waitFor(() => {
      expect(mockRequestFormPageMutation).toHaveBeenCalledWith('tasklists.update');
    });
    expect(mockReadTaskListCommandTarget).toHaveBeenCalledWith('2');
    expect(mockCreateTaskList).not.toHaveBeenCalled();
    expect(mockAddToast).toHaveBeenCalledWith(
      'Salvo com sucesso',
      'success',
      undefined,
      undefined,
      { suppressAnnounce: true },
    );
  });

  it('mostra erro ao falhar a atualização e não emite sucesso nem cria lista', async () => {
    mockRequestFormPageMutation.mockRejectedValueOnce(new Error('Falha ao atualizar lista'));
    const user = userEvent.setup();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Lista Beta')).toBeInTheDocument();
    });

    await user.click(within(screen.getByTestId('row-2')).getByRole('button', { name: 'Editar' }));
    await waitFor(() => expect(screen.getByDisplayValue('Lista Beta')).toBeInTheDocument());
    await user.click(screen.getByRole('button', { name: 'Salvar' }));

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith(
        'Falha ao atualizar lista',
        'error',
        undefined,
        undefined,
        { suppressAnnounce: true },
      );
    });
    expect(mockRequestFormPageMutation).toHaveBeenCalledWith('tasklists.update');
    expect(mockUpdateTaskList).not.toHaveBeenCalled();
    expect(mockAddToast).not.toHaveBeenCalledWith(
      'Salvo com sucesso',
      'success',
      undefined,
      undefined,
      { suppressAnnounce: true },
    );
  });

  it('mostra erro ao salvar com título vazio', async () => {
    const user = userEvent.setup();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Lista Alfa')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: 'Nova Lista' }));

    const saveBtn = screen.getByRole('button', { name: 'Salvar' });
    await user.click(saveBtn);

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith(
        expect.stringContaining('vazio'),
        'error',
        undefined,
        undefined,
        { suppressAnnounce: true },
      );
    });

    expect(mockCreateTaskList).not.toHaveBeenCalled();
  });

  it('fecha e anuncia o sucesso de save vindo do comando de página', async () => {
    const user = userEvent.setup();
    await renderPage();
    await user.click(screen.getByRole('button', { name: 'Nova Lista' }));
    await user.type(screen.getByPlaceholderText('Título da lista'), 'Salvo pela paleta');

    await act(async () => {
      await mockRequestFormPageMutation('tasklists.create');
    });

    await waitFor(() => expect(screen.queryByTestId('modal')).not.toBeInTheDocument());
    expect(mockAddToast).toHaveBeenCalledWith(
      expect.stringContaining('Salvo pela paleta'),
      'success',
      undefined,
      undefined,
      { suppressAnnounce: true },
    );
  });

  it('clona lista via ação de row', async () => {
    const user = userEvent.setup();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Lista Alfa')).toBeInTheDocument();
    });

    const row = screen.getByTestId('row-1');
    const cloneBtn = within(row).getByRole('button', { name: 'Clonar' });
    await user.click(cloneBtn);

    await waitFor(() => {
      expect(mockRequestRootPageMutation).toHaveBeenCalledWith('tasklists.duplicate');
      expect(mockReadTaskListCommandTarget).toHaveBeenCalledWith('1');
    });

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith(
        expect.stringContaining('clonada'),
        'success',
        undefined,
        undefined,
        { suppressAnnounce: true },
      );
    });
  });

  it('deleta lista via ação de row e delega a confirmação ao backend', async () => {
    const user = userEvent.setup();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Lista Alfa')).toBeInTheDocument();
    });

    const row = screen.getByTestId('row-1');
    const deleteBtn = within(row).getByRole('button', { name: 'Deletar' });
    await user.click(deleteBtn);

    await waitFor(() => {
      expect(mockRequestRootPageMutation).toHaveBeenCalledWith('tasklists.delete');
      expect(mockReadTaskListCommandTarget).toHaveBeenCalledWith('1');
    });
    expect(mockRequestConfirm).not.toHaveBeenCalled();

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith(
        expect.stringContaining('deletada'),
        'success',
        undefined,
        undefined,
        { suppressAnnounce: true },
      );
    });
  });

  it('não usa confirmação local para deletar', async () => {
    const user = userEvent.setup();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Lista Alfa')).toBeInTheDocument();
    });

    const row = screen.getByTestId('row-1');
    const deleteBtn = within(row).getByRole('button', { name: 'Deletar' });
    await user.click(deleteBtn);

    await waitFor(() => expect(mockRequestRootPageMutation).toHaveBeenCalledWith('tasklists.delete'));
    expect(mockRequestConfirm).not.toHaveBeenCalled();
  });

  it('invalida um draft de edição quando título ou descrição muda', async () => {
    const user = userEvent.setup();
    await renderPage();
    await user.click(within(screen.getByTestId('row-2')).getByRole('button', { name: 'Editar' }));
    await waitFor(() => expect(screen.getByDisplayValue('Lista Beta')).toBeInTheDocument());

    const prepared = formMutationOptions.prepare('tasklists.update');
    expect(prepared?.isCurrent()).toBe(true);
    await user.type(screen.getByDisplayValue('Lista Beta'), ' alterada');
    expect(prepared?.isCurrent()).toBe(false);
  });

  it('invalida o comando root quando a seleção muda ou o alvo desaparece antes do commit', async () => {
    const user = userEvent.setup();
    await renderPage();
    await user.click(screen.getByText('Lista Alfa'));
    const prepared = rootMutationOptions.prepare('tasklists.delete');
    expect(prepared?.isCurrent()).toBe(true);

    await user.click(screen.getByText('Lista Beta'));
    expect(prepared?.isCurrent()).toBe(false);

    await user.click(screen.getByText('Lista Alfa'));
    const secondPrepared = rootMutationOptions.prepare('tasklists.delete');
    storeTaskLists.delete('1');
    expect(secondPrepared?.isCurrent()).toBe(false);
    expect(secondPrepared?.canPresent?.()).toBe(true);
  });

  it('abre lista em aba do workspace via ação Abrir', async () => {
    const user = userEvent.setup();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Lista Alfa')).toBeInTheDocument();
    });

    const row = screen.getByTestId('row-1');
    const openBtn = within(row).getByRole('button', { name: 'Abrir' });
    await user.click(openBtn);

    await waitFor(() => {
      expect(mockExecuteDeepLink).toHaveBeenCalledWith(
        { type: 'tab:open', tabType: 'tasklist', contentId: '1', title: 'Lista Alfa' },
        expect.objectContaining({ navigate: expect.any(Function) }),
      );
    });
  });

  it('encaminha criação e edição pelos comandos de apresentação', async () => {
    const user = userEvent.setup();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Lista Alfa')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: 'Nova Lista' }));
    expect(screen.getByText('Criar Nova Lista')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Cancelar' }));
    await user.click(within(screen.getByTestId('row-2')).getByRole('button', { name: 'Editar' }));
    await waitFor(() => expect(screen.getByDisplayValue('Lista Beta')).toBeInTheDocument());

    expect(screen.getByText('Editar Lista')).toBeInTheDocument();
    expect(screen.getByDisplayValue('Lista Beta')).toBeInTheDocument();
  });

  it('captura o alvo da linha antes de solicitar edição e foca a busca pelo comando', async () => {
    const user = userEvent.setup();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Lista Alfa')).toBeInTheDocument();
    });

    await user.click(within(screen.getByTestId('row-2')).getByRole('button', { name: 'Editar' }));
    await waitFor(() => expect(screen.getByDisplayValue('Lista Beta')).toBeInTheDocument());
    expect(screen.getByText('Editar Lista')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Nova Lista' }));
    expect(screen.getByText('Editar Lista')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Cancelar' }));

    const search = screen.getByRole('textbox', { name: 'Buscar listas...' });
    await user.type(search, 'sem correspondência');
    expect(screen.getByText('Nenhuma lista de tarefas criada')).toBeInTheDocument();
    expect(search).toBeInTheDocument();

    const target = capturePagePresentationTarget(() => '/tasklists', 'tasklists.search.focus');
    expect(target?.open('tasklists.search.focus')).toBe(true);
    target?.dispose();
    expect(document.activeElement).toBe(screen.getByRole('textbox', { name: 'Buscar listas...' }));
  });

  it('invalida edição quando o store troca o objeto selecionado sem rerender', async () => {
    const user = userEvent.setup();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Lista Beta')).toBeInTheDocument();
    });
    await user.click(screen.getByText('Lista Beta'));

    const lease = capturePagePresentationTarget(() => '/tasklists', 'tasklists.edit.open');
    expect(lease).toBeDefined();

    storeTaskLists = new Map(storeTaskLists);
    storeTaskLists.set('2', makeTaskList('2', 'Lista Beta recriada'));

    expect(lease?.isCurrent()).toBe(false);
    expect(lease?.open('tasklists.edit.open')).toBe(false);
  });
});
