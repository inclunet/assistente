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
  useEditableList: () => mockCrud,
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
  useUIStore: () => ({
    addToast: mockAddToast,
  }),
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
});
