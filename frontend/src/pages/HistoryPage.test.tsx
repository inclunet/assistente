import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockGetConversations = vi.fn();
const mockDeleteConversation = vi.fn();
const mockUpdateConversation = vi.fn();
const mockExportConversations = vi.fn();
const mockExportConversationsToFile = vi.fn();
const mockImportConversations = vi.fn();
const mockImportData = vi.fn();
const mockSearchConversationHistory = vi.fn();
const mockOpenImportFileDialog = vi.fn();
const mockAddTab = vi.fn().mockResolvedValue('tab-1');
const mockMoveTabToWorkspace = vi.fn().mockResolvedValue(undefined);
const mockNavigate = vi.fn();
const mockExecuteDeepLink = vi.fn().mockResolvedValue(undefined);

let lastToolbarActions: Array<{ key: string; label: string; onClick: () => void; disabled?: boolean }> = [];

type RowAction = {
  id: string;
  label?: string;
  action?: () => void;
  submenu?: RowAction[];
};

type ConversationItem = {
  id: number;
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
  DeleteConversation: (id: number) => mockDeleteConversation(id),
  UpdateConversation: (id: number, title: string, snippet: string) => mockUpdateConversation(id, title, snippet),
  ExportConversations: (ids: number[]) => mockExportConversations(ids),
  ExportConversationsToFile: (ids: number[], format: string) => mockExportConversationsToFile(ids, format),
  ImportConversations: (payload: string) => mockImportConversations(payload),
  ImportData: (payload: string, password: string) => mockImportData(payload, password),
  SearchConversationHistory: (query: string, limit: number) => mockSearchConversationHistory(query, limit),
  GetLLMProvidersWithStatus: vi.fn().mockResolvedValue([]),
}));

vi.mock('../lib/exportImport', () => ({
  downloadJSON: vi.fn(),
  generateFilename: vi.fn(() => 'conversas.json'),
  openFileDialog: vi.fn(),
  openImportFileDialog: (...args: unknown[]) => mockOpenImportFileDialog(...args),
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
          {getRowActions?.(item)?.flatMap((action: RowAction) => {
            const buttons = [];
            if (action.action) {
              buttons.push(
                <button key={action.id} type="button" onClick={action.action}>
                  {action.label}
                </button>
              );
            }
            action.submenu?.forEach((submenuItem) => {
              buttons.push(
                <button key={submenuItem.id} type="button" onClick={submenuItem.action}>
                  {submenuItem.label}
                </button>
              );
            });
            return buttons;
          })}
        </div>
      ))}
    </div>
  ),
}));

const conversations: ConversationItem[] = [
  {
    id: 1,
    title: 'Conversa 1',
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    message_count: 2,
  },
  {
    id: 2,
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
    mockExportConversationsToFile.mockResolvedValue('C:/tmp/conversas.html');
    mockImportConversations.mockResolvedValue({ success: true, message: 'ok' });
    mockImportData.mockResolvedValue({ success: true, message: 'ok', imported: 2, skipped: 0, errors: [] });
    mockSearchConversationHistory.mockResolvedValue([]);
    mockOpenImportFileDialog.mockReset();
    mockAddTab.mockResolvedValue(undefined);
    mockNavigate.mockReset();
    lastToolbarActions = [];
    mockRequestConfirm.mockReset();
    mockRequestConfirm.mockResolvedValue(true);
    vi.stubGlobal('alert', vi.fn());
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
      expect(mockDeleteConversation).toHaveBeenCalledWith(1);
      expect(mockDeleteConversation).toHaveBeenCalledWith(2);
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
      expect(mockDeleteConversation).toHaveBeenCalledWith(2);
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
        { type: 'conversation:open', conversationId: 1, title: 'Conversa 1' },
        expect.objectContaining({ navigate: expect.any(Function) }),
      );
    });
  });

  it('exporta HTML das conversas selecionadas pela toolbar', async () => {
    const user = userEvent.setup();
    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');

    await user.click(screen.getByRole('button', { name: 'select-two' }));
    const exportHtmlAction = lastToolbarActions.find((action) => action.key === 'export-html');
    expect(exportHtmlAction).toBeTruthy();
    exportHtmlAction?.onClick();

    await waitFor(() => {
      expect(mockExportConversationsToFile).toHaveBeenCalledWith([1, 2], 'html');
    });
  });

  it('exporta PDF de uma unica conversa pelo menu da linha', async () => {
    const user = userEvent.setup();
    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');

    const pdfButtons = screen.getAllByRole('button', { name: 'Exportar PDF' });
    await user.click(pdfButtons[1]);

    await waitFor(() => {
      expect(mockExportConversationsToFile).toHaveBeenCalledWith([1], 'pdf');
    });
  });

  it('abre modal de importacao com resumo do arquivo', async () => {
    const user = userEvent.setup();
    mockOpenImportFileDialog.mockResolvedValue({
      name: 'backup.json',
      content: JSON.stringify({
        version: 1,
        exportedAt: '2025-01-03T00:00:00Z',
        appVersion: '1.2.3',
        options: { includeAudio: false },
        resources: {
          conversations: [
            { messages: [{}, {}] },
          ],
        },
      }),
    });

    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    await user.click(screen.getByRole('button', { name: 'Importar' }));

    await screen.findByRole('heading', { name: 'Importar conversas' });
    expect(screen.getByText('backup.json')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Importar agora' })).toBeInTheDocument();
  });

  it('exige senha ao importar arquivo com credenciais criptografadas', async () => {
    const user = userEvent.setup();
    mockOpenImportFileDialog.mockResolvedValue({
      name: 'backup.json',
      content: JSON.stringify({
        version: 1,
        resources: {
          conversations: [],
          credentials: { mode: 'encrypted' },
        },
      }),
    });

    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    await user.click(screen.getByRole('button', { name: 'Importar' }));
    await screen.findByRole('heading', { name: 'Importar conversas' });
    await user.click(screen.getByRole('button', { name: 'Importar agora' }));

    expect(mockImportData).not.toHaveBeenCalled();
    expect(await screen.findByText('Informe a senha usada para exportar as credenciais.')).toBeInTheDocument();
  });

  it('confirma importacao usando ImportData e recarrega a lista', async () => {
    const user = userEvent.setup();
    const payload = JSON.stringify({
      version: 1,
      resources: {
        conversations: [{ messages: [{}] }],
        credentials: { mode: 'encrypted' },
      },
    });
    mockOpenImportFileDialog.mockResolvedValue({
      name: 'backup.json',
      content: payload,
    });
    mockImportData.mockResolvedValue({ success: true, message: 'ok', imported: 1, skipped: 0, errors: [] });

    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    await user.click(screen.getByRole('button', { name: 'Importar' }));
    const passwordInput = await screen.findByPlaceholderText('Digite a senha de exportação');
    await user.type(passwordInput, 'segredo');
    await user.click(screen.getByRole('button', { name: 'Importar agora' }));

    await waitFor(() => {
      expect(mockImportData).toHaveBeenCalledWith(payload, 'segredo');
    });
    await waitFor(() => {
      expect(mockGetConversations).toHaveBeenCalledTimes(2);
    });
    expect(globalThis.alert).toHaveBeenCalledWith(expect.stringContaining('ok'));
  });
});
