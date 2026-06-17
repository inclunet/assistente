import type { ButtonHTMLAttributes, ReactNode } from 'react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { act, render, screen, waitFor, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockListMemoryRecords = vi.fn();
const mockGetMemoryRecord = vi.fn();
const mockCreateMemoryRecord = vi.fn();
const mockUpdateMemoryRecord = vi.fn();
const mockArchiveMemoryRecord = vi.fn();
const mockUnarchiveMemoryRecord = vi.fn();
const mockDeleteMemoryRecord = vi.fn();
const mockConfirm = vi.fn();
const mockAnnounce = vi.fn();
const mockAddToast = vi.fn();

vi.mock('@wailsjs/go/app/App', () => ({
  ListMemoryRecords: (...args: unknown[]) => mockListMemoryRecords(...args),
  GetMemoryRecord: (...args: unknown[]) => mockGetMemoryRecord(...args),
  CreateMemoryRecord: (...args: unknown[]) => mockCreateMemoryRecord(...args),
  UpdateMemoryRecord: (...args: unknown[]) => mockUpdateMemoryRecord(...args),
  ArchiveMemoryRecord: (...args: unknown[]) => mockArchiveMemoryRecord(...args),
  UnarchiveMemoryRecord: (...args: unknown[]) => mockUnarchiveMemoryRecord(...args),
  DeleteMemoryRecord: (...args: unknown[]) => mockDeleteMemoryRecord(...args),
}));

vi.mock('react-i18next', () => {
  const t = (key: string, fallback?: string | Record<string, unknown>) => (typeof fallback === 'string' ? fallback : key);
  return {
    useTranslation: () => ({ t }),
  };
});

vi.mock('@ant-design/icons', () => ({
  PlusOutlined: () => <span data-testid="plus-icon" />,
  DeleteOutlined: () => <span data-testid="delete-icon" />,
  InboxOutlined: () => <span data-testid="inbox-icon" />,
  RollbackOutlined: () => <span data-testid="rollback-icon" />,
}));

vi.mock('../hooks/useConfirm', () => ({
  useConfirm: () => mockConfirm,
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: mockAnnounce }),
}));

vi.mock('../hooks/useGridFocus', () => ({
  useGridFocus: () => ({ handleGridReady: vi.fn() }),
}));

vi.mock('../hooks/useGridPageLandmarks', () => ({
  useGridPageLandmarks: vi.fn(),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: (selector: (state: { addToast: typeof mockAddToast }) => unknown) => selector({ addToast: mockAddToast }),
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: ({ actions = [], searchValue, onSearchChange, rightEnd }: {
    actions?: Array<{ key: string; label: string; onClick: () => void }>;
    searchValue?: string;
    onSearchChange?: (value: string) => void;
    rightEnd?: ReactNode;
  }) => (
    <div>
      <input
        aria-label="search"
        value={searchValue}
        onChange={(event) => onSearchChange?.(event.target.value)}
      />
      {rightEnd}
      {actions.map((action) => (
        <button key={action.key} type="button" onClick={action.onClick}>{action.label}</button>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/DataGrid', () => ({
  DataGrid: ({ items, columns, onActivate, getRowActions }: {
    items: Array<Record<string, unknown>>;
    columns: Array<{ key: string; format?: (value: unknown, item: Record<string, unknown>) => ReactNode }>;
    onActivate?: (item: Record<string, unknown>, index: number) => void;
    getRowActions?: (item: Record<string, unknown>) => Array<{ id: string; label?: string; action?: () => void }>;
  }) => (
    <div role="grid">
      {items.map((item, index) => (
        <div key={String(item.id)} role="row">
          <button type="button" onClick={() => onActivate?.(item, index)}>
            {columns.map((column) => (
              <span key={column.key}>
                {column.format ? column.format(item[column.key], item) : String(item[column.key] ?? '')}
              </span>
            ))}
          </button>
          {getRowActions?.(item).map((action) => (
            <button key={action.id} type="button" onClick={action.action}>{action.label}</button>
          ))}
        </div>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/Modal', () => ({
  Modal: ({ isOpen, children, title }: { isOpen: boolean; children: ReactNode; title: string }) =>
    isOpen ? <div role="dialog" aria-label={title}>{children}</div> : null,
}));

vi.mock('../components/ui/Button', () => ({
  Button: ({ children, ...props }: ButtonHTMLAttributes<HTMLButtonElement>) => <button {...props}>{children}</button>,
}));

import MemoriesPage from './MemoriesPage';

describe('MemoriesPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockConfirm.mockResolvedValue(true);
    mockListMemoryRecords.mockResolvedValue({
      records: [{
        id: 'mem-1',
        content: 'Usuário prefere respostas curtas.',
        summary: '',
        loadPolicy: 'core',
        kind: 'user_preference',
        scope: 'user',
        importance: 5,
      }],
      total: 1,
    });
    mockGetMemoryRecord.mockResolvedValue({
      id: 'mem-1',
      content: 'Usuário prefere respostas curtas.',
      loadPolicy: 'core',
      kind: 'user_preference',
      scope: 'user',
      importance: 5,
    });
    mockCreateMemoryRecord.mockResolvedValue({ id: 'mem-2' });
  });

  it('carrega e renderiza memórias salvas', async () => {
    render(<MemoriesPage />);

    await waitFor(() => {
      expect(mockListMemoryRecords).toHaveBeenCalled();
      expect(screen.getByText('Usuário prefere respostas curtas.')).toBeInTheDocument();
    });
  });

  it('cria memória pelo modal', async () => {
    const user = userEvent.setup();
    render(<MemoriesPage />);

    await user.click(await screen.findByText('memories.actions.new'));
    fireEvent.change(screen.getByLabelText('memories.fields.content'), {
      target: { value: 'Nova memória importante' },
    });
    await user.click(screen.getByText('common.save'));

    await waitFor(() => {
      expect(mockCreateMemoryRecord).toHaveBeenCalledWith(expect.objectContaining({
        content: 'Nova memória importante',
        loadPolicy: 'retrievable',
      }));
    });
  });

  it('troca filtro para archived ao salvar memoria arquivada', async () => {
    const user = userEvent.setup();
    render(<MemoriesPage />);

    await waitFor(() => {
      expect(mockListMemoryRecords).toHaveBeenCalled();
    });
    fireEvent.change(screen.getByLabelText('memories.filters.policy'), {
      target: { value: 'core' },
    });
    await user.click(screen.getByText('memories.actions.new'));
    fireEvent.change(screen.getByLabelText('memories.fields.content'), {
      target: { value: 'Memória arquivada' },
    });
    fireEvent.change(screen.getByLabelText('memories.fields.loadPolicy'), {
      target: { value: 'archived' },
    });
    await user.click(screen.getByText('common.save'));

    await waitFor(() => {
      expect(mockListMemoryRecords.mock.calls.some(([filter]) => (
        filter && typeof filter === 'object' &&
        (filter as { includeArchived?: boolean; loadPolicies?: string[] }).includeArchived === true &&
        (filter as { loadPolicies?: string[] }).loadPolicies?.[0] === 'archived'
      ))).toBe(true);
    });
  });

  it('bloqueia submit duplicado enquanto salva', async () => {
    const user = userEvent.setup();
    let resolveCreate: ((value: unknown) => void) | undefined;
    mockCreateMemoryRecord.mockImplementationOnce(() => new Promise((resolve) => {
      resolveCreate = resolve;
    }));
    render(<MemoriesPage />);

    await user.click(await screen.findByText('memories.actions.new'));
    fireEvent.change(screen.getByLabelText('memories.fields.content'), {
      target: { value: 'Nova memória importante' },
    });
    const saveButton = screen.getByText('common.save');
    await user.dblClick(saveButton);

    expect(mockCreateMemoryRecord).toHaveBeenCalledTimes(1);
    await act(async () => {
      resolveCreate?.({ id: 'mem-2' });
    });
  });

  it('ignora respostas antigas de busca quando uma consulta mais recente termina antes', async () => {
    let resolveFirst: ((value: unknown) => void) | undefined;
    let resolveSecond: ((value: unknown) => void) | undefined;
    mockListMemoryRecords
      .mockImplementationOnce(() => new Promise((resolve) => {
        resolveFirst = resolve;
      }))
      .mockImplementationOnce(() => new Promise((resolve) => {
        resolveSecond = resolve;
      }));

    render(<MemoriesPage />);
    fireEvent.change(screen.getByLabelText('search'), { target: { value: 'nova' } });

    await waitFor(() => {
      expect(mockListMemoryRecords).toHaveBeenCalledTimes(2);
    });

    await act(async () => {
      resolveSecond?.({
        records: [{ id: 'latest', content: 'resultado recente', loadPolicy: 'core', kind: 'user_preference', scope: 'user' }],
        total: 1,
      });
    });

    await waitFor(() => {
      expect(screen.getByText('resultado recente')).toBeInTheDocument();
    });

    await act(async () => {
      resolveFirst?.({
        records: [{ id: 'stale', content: 'resultado antigo', loadPolicy: 'core', kind: 'user_preference', scope: 'user' }],
        total: 1,
      });
    });

    await waitFor(() => {
      expect(screen.queryByText('resultado antigo')).not.toBeInTheDocument();
      expect(screen.getByText('resultado recente')).toBeInTheDocument();
    });
  });

  it('reajusta a pagina quando o total diminui apos exclusao', async () => {
    const user = userEvent.setup();
    let resolvePageCorrection: ((value: unknown) => void) | undefined;
    let resolveReload: ((value: unknown) => void) | undefined;
    mockListMemoryRecords
      .mockResolvedValueOnce({
        records: [{ id: 'mem-1', content: 'primeira página', loadPolicy: 'core', kind: 'user_preference', scope: 'user' }],
        total: 251,
      })
      .mockResolvedValueOnce({
        records: [{ id: 'mem-251', content: 'última página', loadPolicy: 'core', kind: 'user_preference', scope: 'user' }],
        total: 251,
      })
      .mockImplementationOnce(() => new Promise((resolve) => {
        resolvePageCorrection = resolve;
      }))
      .mockImplementationOnce(() => new Promise((resolve) => {
        resolveReload = resolve;
      }));

    render(<MemoriesPage />);

    await screen.findByText('primeira página');
    await user.click(screen.getByText('memories.pagination.next'));
    await screen.findByText('última página');
    await user.click(screen.getByText('memories.actions.delete'));

    await waitFor(() => {
      expect(mockListMemoryRecords).toHaveBeenCalledTimes(3);
    });
    await act(async () => {
      resolvePageCorrection?.({
        records: [],
        total: 249,
      });
    });
    await waitFor(() => {
      expect(screen.queryByText('última página')).not.toBeInTheDocument();
      expect(mockListMemoryRecords).toHaveBeenCalledTimes(4);
    });
    await act(async () => {
      resolveReload?.({
        records: [{ id: 'mem-1', content: 'primeira página', loadPolicy: 'core', kind: 'user_preference', scope: 'user' }],
        total: 249,
      });
    });
    expect(mockListMemoryRecords.mock.calls[3][0]).toEqual(expect.objectContaining({
      offset: 0,
    }));
    expect(screen.getByText('primeira página')).toBeInTheDocument();
  });
});

