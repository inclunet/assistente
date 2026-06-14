import { describe, expect, it, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';

/* ── mock fns (definidas antes dos vi.mock) ────────────────── */

const mockCreateTaskList = vi.fn();
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

vi.mock('@wailsjs/go/app/App', () => ({
  GetLLMProvidersWithStatus: vi.fn().mockResolvedValue([]),
}));

let storeTaskLists: Map<string, unknown> = new Map();

vi.mock('../store/taskListStore', () => ({
  useTaskListStore: Object.assign(
    (selector?: (state: Record<string, unknown>) => unknown) => {
      const state: Record<string, unknown> = {
        taskLists: storeTaskLists,
        createTaskList: mockCreateTaskList,
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
    },
  ),
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: (selector?: (state: Record<string, unknown>) => unknown) => {
    const state: Record<string, unknown> = {
      addTab: mockAddTab,
      moveTabToWorkspace: mockMoveTabToWorkspace,
      workspaces: [],
    };
    if (selector) return selector(state);
    return state;
  },
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
    ({ left, actions }: { left?: ReactNode; actions?: Array<{ key: string; label: string; onClick: () => void; disabled?: boolean }> }) => (
      <div data-testid="toolbar">
        {left}
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
  beforeEach(() => {
    cleanup();
    vi.clearAllMocks();

    storeTaskLists = new Map([
      ['1', makeTaskList('1', 'Lista Alfa')],
      ['2', makeTaskList('2', 'Lista Beta')],
    ]);

    mockCreateTaskList.mockResolvedValue(makeTaskList('3', 'Lista Nova'));
    mockDeleteTaskList.mockResolvedValue(undefined);
    mockCloneTaskList.mockResolvedValue(makeTaskList('4', 'Lista Alfa (Cópia)'));
    mockGetCachedTaskList.mockImplementation((id: string) => storeTaskLists.get(id));
    mockLoadTaskList.mockImplementation(async (id: string) => storeTaskLists.get(id) ?? null);
    mockFetchAllTaskLists.mockResolvedValue([]);
    mockAddTab.mockResolvedValue('tab-1');
    mockRequestConfirm.mockResolvedValue(true);
  });

  async function renderPage() {
    return render(
      <MemoryRouter>
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
      expect(mockCreateTaskList).toHaveBeenCalledWith('Lista Nova', undefined);
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
      expect(mockCloneTaskList).toHaveBeenCalledWith('1', 'Lista Alfa (Cópia)');
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

  it('deleta lista via ação de row com confirmação', async () => {
    const user = userEvent.setup();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Lista Alfa')).toBeInTheDocument();
    });

    const row = screen.getByTestId('row-1');
    const deleteBtn = within(row).getByRole('button', { name: 'Deletar' });
    await user.click(deleteBtn);

    await waitFor(() => {
      expect(mockRequestConfirm).toHaveBeenCalled();
    });

    await waitFor(() => {
      expect(mockDeleteTaskList).toHaveBeenCalledWith('1');
    });

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

  it('não deleta se confirmação for negada', async () => {
    mockRequestConfirm.mockResolvedValue(false);
    const user = userEvent.setup();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Lista Alfa')).toBeInTheDocument();
    });

    const row = screen.getByTestId('row-1');
    const deleteBtn = within(row).getByRole('button', { name: 'Deletar' });
    await user.click(deleteBtn);

    await waitFor(() => {
      expect(mockRequestConfirm).toHaveBeenCalled();
    });

    expect(mockDeleteTaskList).not.toHaveBeenCalled();
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
});
