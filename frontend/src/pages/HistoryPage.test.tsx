import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockGetConversations = vi.fn();
const mockDeleteConversation = vi.fn();
const mockUpdateConversation = vi.fn();
const mockExportConversationsToFile = vi.fn();
const mockExportData = vi.fn();
const mockImportConversations = vi.fn();
const mockImportData = vi.fn();
const mockAnalyzeImportData = vi.fn();
const mockSearchConversationHistory = vi.fn();
const mockGetLLMProvidersWithStatus = vi.fn();
const mockGetAllTaskLists = vi.fn();
const mockListMcpServers = vi.fn();
const mockOpenImportFileDialog = vi.fn();
const mockAddTab = vi.fn().mockResolvedValue('tab-1');
const mockMoveTabToWorkspace = vi.fn().mockResolvedValue(undefined);
const mockNavigate = vi.fn();
const mockExecuteDeepLink = vi.fn().mockResolvedValue(undefined);

let lastToolbarActions: Array<{ key: string; label: string; onClick: () => void; disabled?: boolean }> = [];

type ConversationItem = {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
  message_count: number;
};

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>();
  return {
    ...actual,
    useNavigate: () => mockNavigate,
    useLocation: () => ({
      pathname: '/history',
      search: '',
      hash: '',
      state: null,
      key: 'test',
    }),
  };
});

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string, fallback?: string | { defaultValue?: string; count?: number }) => {
      if (typeof fallback === 'string') return fallback;
      if (fallback?.defaultValue) {
        return fallback.defaultValue.replace('{{count}}', String(fallback.count ?? ''));
      }
      return key;
    },
  }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetConversations: () => mockGetConversations(),
  DeleteConversation: (id: string) => mockDeleteConversation(id),
  UpdateConversation: (id: string, title: string, snippet: string) => mockUpdateConversation(id, title, snippet),
  ExportConversationsToFile: (ids: string[], format: string) => mockExportConversationsToFile(ids, format),
  ExportData: (payload: unknown) => mockExportData(payload),
  ImportConversations: (payload: string) => mockImportConversations(payload),
  ImportData: (payload: string, password: string) => mockImportData(payload, password),
  AnalyzeImportData: (payload: string, password: string) => mockAnalyzeImportData(payload, password),
  ListMCPServers: () => mockListMcpServers(),
  SearchConversationHistory: (query: string, limit: number) => mockSearchConversationHistory(query, limit),
  GetLLMProvidersWithStatus: () => mockGetLLMProvidersWithStatus(),
  GetAllTaskLists: () => mockGetAllTaskLists(),
}));

vi.mock('../lib/exportImport', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/exportImport')>();
  return {
    ...actual,
    openImportFileDialog: (...args: unknown[]) => mockOpenImportFileDialog(...args),
  };
});

vi.mock('../hooks/useGridFocus', () => ({
  useGridFocus: () => ({
    handleGridReady: vi.fn(),
  }),
}));

vi.mock('../hooks/useGridPageLandmarks', () => ({
  useGridPageLandmarks: vi.fn(),
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
      <button type="button" onClick={() => onSelectionChange?.(new Set(items?.map(i => i.id) ?? []))}>
        select-two
      </button>
      <button type="button" onClick={() => onSelectionChange?.(new Set(items?.slice(0, 1).map(i => i.id) ?? []))}>
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
    id: '01926b90-7a5a-7c4e-8d3f-000000000001',
    title: 'Conversa 1',
    createdAt: '2025-01-01T00:00:00Z',
    updatedAt: '2025-01-01T00:00:00Z',
    message_count: 2,
  },
  {
    id: '01926b90-7a5a-7c4e-8d3f-000000000002',
    title: 'Conversa 2',
    createdAt: '2025-01-02T00:00:00Z',
    updatedAt: '2025-01-02T00:00:00Z',
    message_count: 5,
  },
];

import HistoryPage from './HistoryPage';

describe('HistoryPage', { timeout: 60_000 }, () => {
  beforeEach(() => {
    mockGetConversations.mockResolvedValue(conversations);
    mockDeleteConversation.mockResolvedValue(undefined);
    mockUpdateConversation.mockResolvedValue(undefined);
    mockExportConversationsToFile.mockResolvedValue('');
    mockExportData.mockResolvedValue('{}');
    mockImportConversations.mockResolvedValue({ success: true, message: 'ok' });
    mockImportData.mockResolvedValue({ success: true, message: 'ok' });
    mockAnalyzeImportData.mockResolvedValue({
      version: 2,
      conversationCount: 0,
      messageCount: 0,
      providerCount: 0,
      taskListCount: 0,
      taskCount: 0,
      taskNoteCount: 0,
      includesCredentials: false,
      requiresCredentialPassword: false,
      credentialCount: 0,
      conflictCount: 0,
    });
    mockSearchConversationHistory.mockResolvedValue([]);
    mockGetLLMProvidersWithStatus.mockResolvedValue([]);
    mockGetAllTaskLists.mockResolvedValue([]);
    mockListMcpServers.mockResolvedValue([]);
    mockOpenImportFileDialog.mockReset();
    Object.defineProperty(URL, 'createObjectURL', {
      configurable: true,
      value: vi.fn(() => 'blob:history-export-test'),
    });
    Object.defineProperty(URL, 'revokeObjectURL', {
      configurable: true,
      value: vi.fn(),
    });
    Object.defineProperty(HTMLAnchorElement.prototype, 'click', {
      configurable: true,
      value: vi.fn(),
    });
    mockAddTab.mockResolvedValue(undefined);
    mockNavigate.mockReset();
    lastToolbarActions = [];
    mockRequestConfirm.mockReset();
    mockRequestConfirm.mockResolvedValue(true);
  });

  it('nao duplica acao de deletar na toolbar', async () => {
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');

    const actionKeys = lastToolbarActions.map((action) => action.key);
    expect(actionKeys).not.toContain('delete-selected');
  });

  it('deleta conversas selecionadas ao clicar em Excluir', async () => {
    const user = userEvent.setup();
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');

    const selectTwoButtons = screen.getAllByRole('button', { name: 'select-two' });
    await user.click(selectTwoButtons[0]);

    const deleteButton = await screen.findByRole('button', { name: 'Deletar (2)' });
    await user.click(deleteButton);

    await waitFor(() => {
      expect(mockDeleteConversation).toHaveBeenCalledWith('01926b90-7a5a-7c4e-8d3f-000000000001');
      expect(mockDeleteConversation).toHaveBeenCalledWith('01926b90-7a5a-7c4e-8d3f-000000000002');
    });
  });

  it('deleta conversa focada quando nao ha selecao', async () => {
    const user = userEvent.setup();
    render(<HistoryPage />);

    await waitFor(() => {
      expect(screen.getByText('Conversa 2')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: 'focus-second' }));
    const deleteButtons = await screen.findAllByRole('button', { name: 'Excluir conversa' });
    await user.click(deleteButtons[0]);

    await waitFor(() => {
      expect(mockDeleteConversation).toHaveBeenCalledWith('01926b90-7a5a-7c4e-8d3f-000000000002');
    });
  });

  it('aciona abrir conversa via menu de acoes', async () => {
    const user = userEvent.setup();
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
        { type: 'conversation:open', conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001', title: 'Conversa 1' },
        expect.objectContaining({ navigate: expect.any(Function) }),
      );
    });
  });

  it('exporta providers sem exigir conversas selecionadas', async () => {
    const user = userEvent.setup();
    mockGetConversations.mockResolvedValue([]);
    mockGetLLMProvidersWithStatus.mockResolvedValue([
      { id: 'provider-1', name: 'OpenAI' },
    ]);

    render(<HistoryPage />);

    const exportButton = await screen.findByRole('button', { name: 'Exportar dados' });
    await user.click(exportButton);
    await user.click(screen.getByLabelText('Incluir providers persistidos no banco'));
    await user.click(screen.getByRole('button', { name: 'Exportar agora' }));

    await waitFor(() => {
      expect(mockExportData).toHaveBeenCalledWith(expect.objectContaining({
        explicitSelection: true,
        includeCredentials: false,
        outputFormat: 'json',
        providerIds: ['provider-1'],
      }));
    });
    expect(mockExportData.mock.calls[0][0]).not.toHaveProperty('conversationIds');
  });

  it('exporta tasklists e credenciais criptografadas sem conversas selecionadas', async () => {
    const user = userEvent.setup();
    mockGetConversations.mockResolvedValue([]);
    mockGetAllTaskLists.mockResolvedValue([
      { id: 'tasklist-1', name: 'Checklist' },
    ]);

    render(<HistoryPage />);

    await user.click(await screen.findByRole('button', { name: 'Exportar dados' }));
    await user.click(screen.getByLabelText('Incluir tasklists persistidas no banco'));
    await user.click(screen.getByLabelText('Incluir credenciais criptografadas no export'));
    await user.type(screen.getByPlaceholderText('Digite a senha de exportação'), ' segredo ');
    await user.click(screen.getByRole('button', { name: 'Exportar agora' }));

    await waitFor(() => {
      expect(mockExportData).toHaveBeenCalledWith(expect.objectContaining({
        explicitSelection: true,
        includeCredentials: true,
        outputFormat: 'json',
        taskListIds: ['tasklist-1'],
        credentialExportPassword: 'segredo',
      }));
    });
    expect(mockExportData.mock.calls[0][0]).not.toHaveProperty('conversationIds');
  });

  it('mostra preview de importacao com recursos DB-only', async () => {
    const user = userEvent.setup();
    const jsonData = JSON.stringify({
      version: 2,
      appVersion: '0.9.0',
      exportedAt: '2025-01-01T00:00:00Z',
      options: {
        includeCredentials: true,
        includeAudio: false,
      },
      resources: {
        conversations: [],
        providers: [{ id: 'provider-1' }],
        taskLists: [{
          id: 'tasklist-1',
          tasks: [{
            id: 'task-1',
            notes: [{ id: 'note-1' }],
            children: [{ id: 'task-2', notes: [] }],
          }],
        }],
        credentials: { mode: 'encrypted' },
      },
    });
    mockOpenImportFileDialog.mockResolvedValue({
      name: 'backup-db-only.json',
      content: jsonData,
    });
    mockAnalyzeImportData.mockResolvedValue({
      version: 2,
      conversationCount: 0,
      messageCount: 0,
      providerCount: 1,
      taskListCount: 1,
      taskCount: 2,
      taskNoteCount: 1,
      includesCredentials: true,
      requiresCredentialPassword: true,
      credentialCount: 1,
      conflictCount: 0,
    });

    render(<HistoryPage />);

    await user.click(await screen.findByRole('button', { name: 'Importar' }));

    await waitFor(() => {
      expect(mockAnalyzeImportData).toHaveBeenCalledWith(jsonData, '');
    });
    expect(await screen.findByText('backup-db-only.json')).toBeInTheDocument();
    expect(screen.getByText('Incluídas')).toBeInTheDocument();
    expect(screen.getByText('Nenhum conflito detectado')).toBeInTheDocument();
  });
});
