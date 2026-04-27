import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockGetConversations = vi.fn();
const mockDeleteConversation = vi.fn();
const mockUpdateConversation = vi.fn();
const mockExportConversations = vi.fn();
const mockImportConversations = vi.fn();
const mockSearchConversationHistory = vi.fn();
const mockAddTab = vi.fn().mockResolvedValue('tab-1');
const mockMoveTabToWorkspace = vi.fn().mockResolvedValue(undefined);
const mockNavigate = vi.fn();
const mockExecuteDeepLink = vi.fn().mockResolvedValue(undefined);

let lastToolbarActions: Array<{ key: string; label: string; onClick: () => void; disabled?: boolean }> = [];

type ConversationItem = {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
  message_count: number;
};

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
}));

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
  }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetConversations: () => mockGetConversations(),
  DeleteConversation: (id: string) => mockDeleteConversation(id),
  UpdateConversation: (id: string, title: string, snippet: string) => mockUpdateConversation(id, title, snippet),
  ExportConversations: (ids: string[]) => mockExportConversations(ids),
  ImportConversations: (payload: string) => mockImportConversations(payload),
  SearchConversationHistory: (query: string, limit: number) => mockSearchConversationHistory(query, limit),
  GetLLMProvidersWithStatus: vi.fn().mockResolvedValue([]),
}));

vi.mock('../hooks/useGridFocus', () => ({
  useGridFocus: () => ({
    handleGridReady: vi.fn(),
  }),
}));

const mockRequestConfirm = vi.fn(() => Promise.resolve(true));

vi.mock('../hooks/useConfirm', () => ({
  useConfirm: () => mockRequestConfirm,
}));

vi.mock('../lib/deepLinks', () => ({
  executeDeepLink: (...args: unknown[]) => mockExecuteDeepLink(...args),
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: (selector: (state: { addTab: typeof mockAddTab; moveTabToWorkspace: typeof mockMoveTabToWorkspace; workspaces: { id: string; name: string; is_active: boolean }[] }) => unknown) =>
    selector({
      addTab: mockAddTab,
      moveTabToWorkspace: mockMoveTabToWorkspace,
      workspaces: [],
    }),
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: ({ left, actions }: { left?: ReactNode; actions?: Array<{ key: string; label: string; onClick: () => void; disabled?: boolean }> }) => {
    lastToolbarActions = actions ?? [];
    return (
      <div>
        {left}
        {actions?.map((action) => (
          <button key={action.key} onClick={action.onClick} disabled={action.disabled}>
            {action.label}
          </button>
        ))}
      </div>
    );
  },
}));

vi.mock('../components/ui/DataGrid', () => ({
  DataGrid: ({
    items,
    onSelectionChange,
    onFocusChange,
    getRowActions,
  }: {
    items?: ConversationItem[];
    onSelectionChange?: (selected: Set<string | number>) => void;
    onFocusChange?: (item: ConversationItem | null) => void;
    getRowActions?: (item: ConversationItem) => Array<{ id: string; label?: string; action?: () => void }>;
  }) => (
    <div>
      <button type="button" onClick={() => onSelectionChange?.(new Set([1, 2]))}>
        select-two
      </button>
      <button type="button" onClick={() => onSelectionChange?.(new Set([1]))}>
        select-one
      </button>
      <button type="button" onClick={() => onSelectionChange?.(new Set())}>
        clear-selection
      </button>
      <button type="button" onClick={() => onFocusChange?.(items?.[0] ?? null)}>
        focus-first
      </button>
      <button type="button" onClick={() => onFocusChange?.(items?.[1] ?? null)}>
        focus-second
      </button>
      {items?.map((item) => (
        <div key={item.id}>
          <span>{item.title}</span>
          {getRowActions?.(item)?.map((action) => (
            <button key={action.id} type="button" onClick={action.action}>
              {action.label}
            </button>
          ))}
        </div>
      ))}
    </div>
  ),
}));

const conversations: ConversationItem[] = [
  {
    id: '1',
    title: 'Conversa 1',
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    message_count: 2,
  },
  {
    id: '2',
    title: 'Conversa 2',
    created_at: '2025-01-02T00:00:00Z',
    updated_at: '2025-01-02T00:00:00Z',
    message_count: 5,
  },
];

describe('HistoryPage', { timeout: 60_000 }, () => {
  beforeEach(() => {
    mockGetConversations.mockResolvedValue(conversations);
    mockDeleteConversation.mockResolvedValue(undefined);
    mockUpdateConversation.mockResolvedValue(undefined);
    mockExportConversations.mockResolvedValue('{}');
    mockImportConversations.mockResolvedValue({ success: true, message: 'ok' });
    mockSearchConversationHistory.mockResolvedValue([]);
    mockAddTab.mockResolvedValue(undefined);
    mockNavigate.mockReset();
    lastToolbarActions = [];
    mockRequestConfirm.mockReset();
    mockRequestConfirm.mockResolvedValue(true);
  });

  it('nao duplica acao de deletar na toolbar', async () => {
    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');

    const actionKeys = lastToolbarActions.map((action) => action.key);
    expect(actionKeys).not.toContain('delete-selected');
  });

  it('deleta conversas selecionadas ao clicar em Excluir', async () => {
    const user = userEvent.setup();
    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');

    const selectTwoButtons = screen.getAllByRole('button', { name: 'select-two' });
    await user.click(selectTwoButtons[0]);

    const deleteButton = await screen.findByRole('button', { name: 'Deletar (2)' });
    await user.click(deleteButton);

    await waitFor(() => {
      expect(mockDeleteConversation).toHaveBeenCalledWith('1');
      expect(mockDeleteConversation).toHaveBeenCalledWith('2');
    });
  });

  it('deleta conversa focada quando nao ha selecao', async () => {
    const user = userEvent.setup();
    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await waitFor(() => {
      expect(screen.getByText('Conversa 2')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: 'focus-second' }));
    const deleteButtons = await screen.findAllByRole('button', { name: 'Excluir conversa' });
    await user.click(deleteButtons[0]);

    await waitFor(() => {
      expect(mockDeleteConversation).toHaveBeenCalledWith('2');
    });
  });

  it('aciona abrir conversa via menu de acoes', async () => {
    const user = userEvent.setup();
    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await waitFor(() => {
      expect(screen.getByText('Conversa 1')).toBeInTheDocument();
    });

    const openButtons = screen.getAllByRole('button', { name: 'Abrir conversa' });
    const menuOpen = openButtons.find((button) => !button.hasAttribute('disabled'));
    expect(menuOpen).toBeTruthy();
    await user.click(menuOpen!);

    await waitFor(() => {
      expect(mockExecuteDeepLink).toHaveBeenCalledWith(
        { type: 'conversation:open', conversationId: '1', title: 'Conversa 1' },
        expect.objectContaining({ navigate: expect.any(Function) }),
      );
    });
  });
});
