import { describe, expect, it, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

/* ── mock fns (definidas antes dos vi.mock) ────────────────── */

const mockLoadTabs = vi.fn();
const mockCreateTab = vi.fn();
const mockCloseTab = vi.fn();
const mockCreateTaskList = vi.fn();
const mockDeleteTaskList = vi.fn();
const mockCloneTaskList = vi.fn();
const mockSetViewMode = vi.fn();
const mockGetCachedTaskList = vi.fn();
const mockLoadTaskList = vi.fn();
const mockConsumeResourceEdit = vi.fn();
const mockAddToast = vi.fn();
const mockAnnounce = vi.fn();
const mockRequestConfirm = vi.fn();

/* ── mocks de módulos ──────────────────────────────────────── */

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
  }),
}));

let storeOpenTabs: Array<{ id: number; taskListId: number; title: string; position: number; isActive: boolean }> = [];
let storeTaskLists: Map<number, unknown> = new Map();

vi.mock('../store/taskListStore', () => ({
  useTaskListStore: Object.assign(
    (selector?: (state: Record<string, unknown>) => unknown) => {
      const state: Record<string, unknown> = {
        openTabs: storeOpenTabs,
        taskLists: storeTaskLists,
        loadTabs: mockLoadTabs,
        createTab: mockCreateTab,
        closeTab: mockCloseTab,
        createTaskList: mockCreateTaskList,
        deleteTaskList: mockDeleteTaskList,
        cloneTaskList: mockCloneTaskList,
        setViewMode: mockSetViewMode,
        getCachedTaskList: mockGetCachedTaskList,
        loadTaskList: mockLoadTaskList,
      };
      if (selector) return selector(state);
      return state;
    },
    {
      getState: () => ({
        openTabs: storeOpenTabs,
        taskLists: storeTaskLists,
        getCachedTaskList: mockGetCachedTaskList,
      }),
    },
  ),
}));

vi.mock('../store/navigationStore', () => ({
  useNavigationStore: Object.assign(
    () => ({ consumeResourceEdit: mockConsumeResourceEdit }),
    {
      getState: () => ({ consumeResourceEdit: mockConsumeResourceEdit }),
    },
  ),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: () => ({
    addToast: mockAddToast,
  }),
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

vi.mock('../hooks/useAnchoredContextMenu', () => ({
  useAnchoredContextMenu: () => ({
    menu: { visible: false, x: 0, y: 0, items: [], ariaLabel: '' },
    triggerElementRef: { current: null },
    openForTrigger: vi.fn(),
    openAtPoint: vi.fn(),
    closeMenu: vi.fn(),
    onSelectItem: vi.fn(),
  }),
}));

/* ── mocks de componentes UI ───────────────────────────────── */

vi.mock('../components/ui/tabs', () => ({
  Tabs: ({ children }: { children: ReactNode }) => <div data-testid="tabs">{children}</div>,
  TabList: ({ children }: { children: ReactNode }) => <div role="tablist">{children}</div>,
  Tab: ({ children, value }: { children: ReactNode; value: string }) => (
    <button role="tab" data-value={value}>
      {children}
    </button>
  ),
  TabPanel: ({ children }: { children: ReactNode }) => <div role="tabpanel">{children}</div>,
}));

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

vi.mock('../components/taskLists/TasksTable', () => ({
  default: () => <div data-testid="tasks-table">TasksTable</div>,
}));

vi.mock('../components/pickers/TaskListHistoryPicker', () => ({
  TaskListHistoryPicker: () => <div data-testid="history-picker" />,
}));

vi.mock('../components/menu', () => ({
  ContextMenu: () => null,
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

const makeTaskList = (id: number, title: string) => ({
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

describe('TaskListsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();

    // estado padrão: sem abas abertas, duas listas em cache
    storeOpenTabs = [];
    storeTaskLists = new Map([
      [1, makeTaskList(1, 'Lista Alfa')],
      [2, makeTaskList(2, 'Lista Beta')],
    ]);

    mockLoadTabs.mockResolvedValue(undefined);
    mockConsumeResourceEdit.mockReturnValue(null);
    mockCreateTaskList.mockResolvedValue(makeTaskList(3, 'Lista Nova'));
    mockCreateTab.mockResolvedValue(undefined);
    mockCloseTab.mockResolvedValue(undefined);
    mockDeleteTaskList.mockResolvedValue(undefined);
    mockCloneTaskList.mockResolvedValue(makeTaskList(4, 'Lista Alfa (Cópia)'));
    mockSetViewMode.mockResolvedValue(undefined);
    mockGetCachedTaskList.mockImplementation((id: number) => storeTaskLists.get(id));
    mockLoadTaskList.mockImplementation(async (id: number) => storeTaskLists.get(id) ?? null);
    mockRequestConfirm.mockResolvedValue(true);
  });

  async function renderPage() {
    const { default: TaskListsPage } = await import('./TaskListsPage');
    return render(<TaskListsPage />);
  }

  // ── Home Tab ────────────────────────────────────────────────

  it('renderiza aba Home com a listagem de listas', async () => {
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

  it('chama loadTabs na inicialização', async () => {
    await renderPage();

    await waitFor(() => {
      expect(mockLoadTabs).toHaveBeenCalledOnce();
    });
  });

  // ── Criar lista ─────────────────────────────────────────────

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

  it('cria lista e abre em nova aba ao salvar', async () => {
    const user = userEvent.setup();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Lista Alfa')).toBeInTheDocument();
    });

    // Abre modal
    await user.click(screen.getByRole('button', { name: 'Nova Lista' }));

    // Preenche título
    const titleInput = screen.getByPlaceholderText('Título da lista');
    await user.type(titleInput, 'Lista Nova');

    // Salva
    const saveBtn = screen.getByRole('button', { name: 'Salvar' });
    await user.click(saveBtn);

    await waitFor(() => {
      expect(mockCreateTaskList).toHaveBeenCalledWith('Lista Nova', undefined);
    });

    await waitFor(() => {
      expect(mockCreateTab).toHaveBeenCalledWith(3);
    });

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith(
        expect.stringContaining('Lista Nova'),
        'success',
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

    // Salva sem preencher título
    const saveBtn = screen.getByRole('button', { name: 'Salvar' });
    await user.click(saveBtn);

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith(
        expect.stringContaining('vazio'),
        'error',
      );
    });

    // Não deve chamar o backend
    expect(mockCreateTaskList).not.toHaveBeenCalled();
  });

  // ── Clonar lista ────────────────────────────────────────────

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
      expect(mockCloneTaskList).toHaveBeenCalledWith(1, 'Lista Alfa (Cópia)');
    });

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith(
        expect.stringContaining('clonada'),
        'success',
      );
    });
  });

  // ── Deletar lista ───────────────────────────────────────────

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
      expect(mockDeleteTaskList).toHaveBeenCalledWith(1);
    });

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith(
        expect.stringContaining('deletada'),
        'success',
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

  // ── Abrir lista em aba ──────────────────────────────────────

  it('abre lista em nova aba via ação Abrir', async () => {
    const user = userEvent.setup();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Lista Alfa')).toBeInTheDocument();
    });

    const row = screen.getByTestId('row-1');
    const openBtn = within(row).getByRole('button', { name: 'Abrir' });
    await user.click(openBtn);

    await waitFor(() => {
      expect(mockCreateTab).toHaveBeenCalledWith(1);
    });
  });
});
