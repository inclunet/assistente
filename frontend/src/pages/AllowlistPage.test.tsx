import type { ReactNode } from 'react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockLoadItems = vi.fn();
const mockOpenNew = vi.fn();
const mockOpenEdit = vi.fn();
const mockDeleteItem = vi.fn();
const mockAddToast = vi.fn();
const mockAnnounce = vi.fn();

const mockCreateAllowlist = vi.fn<(payload: Record<string, unknown>) => Promise<string>>(async () => 'new-slug');
const mockUpdateAllowlist = vi.fn<(id: string, payload: Record<string, unknown>) => Promise<void>>(async () => undefined);
const mockGetAllowlist = vi.fn<(slug: string) => Promise<Record<string, unknown> | null>>(async () => null);
const mockGetAllowlists = vi.fn<() => Promise<unknown[]>>(async () => []);
const mockDeleteAllowlist = vi.fn<(id: string) => Promise<void>>(async () => undefined);

vi.mock('@wailsjs/go/app/App', () => ({
  GetAllowlists: () => mockGetAllowlists(),
  GetAllowlist: (slug: string) => mockGetAllowlist(slug),
  CreateAllowlist: (payload: Record<string, unknown>) => mockCreateAllowlist(payload),
  UpdateAllowlist: (id: string, payload: Record<string, unknown>) => mockUpdateAllowlist(id, payload),
  DeleteAllowlist: (id: string) => mockDeleteAllowlist(id),
}));

// Captura as `operations` passadas ao useEditableList para que os testes
// possam exercitar createItem/updateItem diretamente (sem precisar abrir o
// formulario na UI). E o jeito mais barato de validar que command_rules e
// preservado no payload enviado ao backend.
let capturedOperations: {
  createItem?: (data: Record<string, unknown>) => Promise<unknown>;
  updateItem?: (id: string, data: Record<string, unknown>) => Promise<unknown>;
} = {};

const mockCrud = {
  items: [
    {
      id: 'allow-1',
      slug: 'allow-1',
      name: 'Allowlist 1',
      description: 'Descricao',
      auto_approve: [],
      always_deny: [],
      default_action: 'confirm',
      ruleCount: 1,
    },
  ],
  loadItems: mockLoadItems,
  openNew: mockOpenNew,
  openEdit: mockOpenEdit,
  deleteItem: mockDeleteItem,
  updateField: vi.fn(),
  closeEditor: vi.fn(),
  save: vi.fn(),
  isNew: false,
  editingItem: null,
  editingId: null,
  saving: false,
};

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
  }),
}));

vi.mock('../hooks/useEditableList', () => ({
  useEditableList: (operations: typeof capturedOperations) => {
    capturedOperations = operations;
    return mockCrud;
  },
}));

vi.mock('../hooks/useGridFocus', () => ({
  useGridFocus: () => ({
    handleGridReady: vi.fn(),
  }),
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: mockAnnounce,
  }),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: (selector?: (state: { addToast: typeof mockAddToast }) => unknown) => {
    const state = { addToast: mockAddToast };
    return selector ? selector(state) : state;
  },
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: ({ left, actions }: { left?: ReactNode; actions?: Array<{ key: string; label: string; onClick: () => void; disabled?: boolean }> }) => (
    <div>
      {left}
      {actions?.map((action) => (
        <button
          key={action.key}
          data-testid={`toolbar-action-${action.key}`}
          onClick={action.onClick}
          disabled={action.disabled}
        >
          {action.label}
        </button>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/DataGrid', () => ({
  DataGrid: ({
    items,
    onFocusChange,
    getRowActions,
  }: {
    items?: Array<{ id: string; name: string }>;
    onFocusChange?: (item: { id: string; name: string } | null) => void;
    getRowActions?: (item: { id: string; name: string }) => Array<{ id: string; label?: string; onClick?: () => void }>;
  }) => (
    <div>
      <button type="button" onClick={() => onFocusChange?.(items?.[0] ?? null)}>
        focus-first
      </button>
      {items?.map((item) => (
        <div key={item.id}>
          <span>{item.name}</span>
          {getRowActions?.(item)?.map((action) => (
            <button key={action.id} type="button" onClick={action.onClick}>
              {action.label}
            </button>
          ))}
        </div>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/Modal', () => ({
  Modal: ({ isOpen, children }: { isOpen: boolean; children?: ReactNode }) => (isOpen ? <div>{children}</div> : null),
  isModalOpen: () => false,
}));

vi.mock('../components/ui/EditorPanel', () => ({
  EditorPanelFooter: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
}));

vi.mock('../components', () => ({
  Button: ({ children, onClick }: { children?: ReactNode; onClick?: () => void }) => (
    <button onClick={onClick}>{children}</button>
  ),
}));

import AllowlistPage from './AllowlistPage';

describe('AllowlistPage', () => {
  beforeEach(() => {
    mockLoadItems.mockReset();
    mockOpenNew.mockReset();
    mockOpenEdit.mockReset();
    mockDeleteItem.mockReset();
    mockAddToast.mockReset();
    mockAnnounce.mockReset();
    mockCreateAllowlist.mockReset().mockResolvedValue('new-slug');
    mockUpdateAllowlist.mockReset().mockResolvedValue(undefined);
    mockGetAllowlist.mockReset();
    mockGetAllowlists.mockReset().mockResolvedValue([]);
    mockDeleteAllowlist.mockReset().mockResolvedValue(undefined);
  });

  it('abre edicao pela toolbar quando ha foco', async () => {
    const user = userEvent.setup();
    render(<AllowlistPage />);

    await waitFor(() => {
      expect(screen.getByText('Allowlist 1')).toBeInTheDocument();
    });

    const editButton = screen.getByTestId('toolbar-action-edit');
    expect(editButton).toBeDisabled();

    await user.click(screen.getByRole('button', { name: 'focus-first' }));
    await user.click(editButton);

    expect(mockOpenEdit).toHaveBeenCalledWith(expect.objectContaining({ id: 'allow-1' }));
  });

  it('executa excluir via menu de acoes', async () => {
    const user = userEvent.setup();
    render(<AllowlistPage />);

    await waitFor(() => {
      expect(screen.getByText('Allowlist 1')).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByRole('button', { name: 'Excluir' });
    await user.click(deleteButtons[deleteButtons.length - 1]);

    expect(mockDeleteItem).toHaveBeenCalledWith(expect.objectContaining({ id: 'allow-1' }));
  });

  it('createItem encaminha command_rules ao backend', async () => {
    render(<AllowlistPage />);

    expect(capturedOperations.createItem).toBeDefined();

    const commandRules = [
      { program: 'kubectl', subcommands: ['get'], args: ['*'], decision: 'approve' },
    ];

    await capturedOperations.createItem!({
      name: 'Nova',
      description: 'desc',
      auto_approve: ['ls'],
      always_deny: [],
      command_rules: commandRules,
      default_action: 'confirm',
    });

    expect(mockCreateAllowlist).toHaveBeenCalledTimes(1);
    const payload = mockCreateAllowlist.mock.calls[0][0];
    expect(payload.command_rules).toEqual(commandRules);
  });

  it('updateItem encaminha command_rules ao backend', async () => {
    render(<AllowlistPage />);

    expect(capturedOperations.updateItem).toBeDefined();

    const commandRules = [
      { program: 'kubectl', subcommands: ['delete'], decision: 'deny' },
    ];

    await capturedOperations.updateItem!('allow-1', {
      name: 'Editada',
      description: 'desc',
      auto_approve: [],
      always_deny: ['rm -rf /'],
      command_rules: commandRules,
      default_action: 'confirm',
    });

    expect(mockUpdateAllowlist).toHaveBeenCalledTimes(1);
    const [calledId, calledPayload] = mockUpdateAllowlist.mock.calls[0];
    expect(calledId).toBe('allow-1');
    expect(calledPayload.command_rules).toEqual(commandRules);
  });

  it('duplica preservando command_rules e contando-as no ruleCount', async () => {
    const user = userEvent.setup();

    const commandRules = [
      { program: 'kubectl', subcommands: ['get'], args: ['*'], decision: 'approve' },
      { program: 'kubectl', subcommands: ['delete'], decision: 'deny' },
    ];
    mockGetAllowlist.mockResolvedValue({
      name: 'Allowlist 1',
      description: 'Descricao',
      auto_approve: ['ls'],
      always_deny: ['rm -rf /'],
      command_rules: commandRules,
      default_action: 'confirm',
    });
    mockCreateAllowlist.mockResolvedValue('allow-1-copia');

    render(<AllowlistPage />);

    await waitFor(() => {
      expect(screen.getByText('Allowlist 1')).toBeInTheDocument();
    });

    const duplicateButtons = screen.getAllByRole('button', { name: 'Duplicar' });
    await user.click(duplicateButtons[duplicateButtons.length - 1]);

    await waitFor(() => {
      expect(mockCreateAllowlist).toHaveBeenCalledTimes(1);
    });
    const payload = mockCreateAllowlist.mock.calls[0][0];
    expect(payload.command_rules).toEqual(commandRules);
    expect(payload.auto_approve).toEqual(['ls']);
    expect(payload.always_deny).toEqual(['rm -rf /']);

    await waitFor(() => {
      expect(mockOpenEdit).toHaveBeenCalled();
    });
    const openedRow = mockOpenEdit.mock.calls[mockOpenEdit.mock.calls.length - 1][0] as {
      ruleCount: number;
    };
    // 1 auto_approve + 1 always_deny + 2 command_rules = 4
    expect(openedRow.ruleCount).toBe(4);
  });
});
