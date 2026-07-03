import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockGetConversationsPage = vi.fn();
const mockGetConversationsByIDs = vi.fn();
const mockDeleteConversation = vi.fn();
const mockUpdateConversation = vi.fn();
const mockExportConversations = vi.fn();
const mockExportConversationsToFile = vi.fn();
const mockSearchConversationHistory = vi.fn();
const mockDownloadJSON = vi.fn();
const mockAddTab = vi.fn().mockResolvedValue('tab-1');
const mockMoveTabToWorkspace = vi.fn().mockResolvedValue(undefined);
const mockNavigate = vi.fn();
const mockExecuteDeepLink = vi.fn().mockResolvedValue(undefined);

let mockLocationSearch = '';

let lastToolbarActions: Array<{ key: string; label: string; onClick: () => void; disabled?: boolean }> = [];

const stableT = (key: string, fallback?: string | { defaultValue?: string; count?: number }) => {
  if (typeof fallback === 'string') return fallback;
  if (fallback?.defaultValue) {
    return fallback.defaultValue.replace('{{count}}', String(fallback.count ?? ''));
  }
  return key;
};

type ConversationItem = {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
  message_count: number;
  kind?: string;
  latestStatus?: string;
};

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>();
  return {
    ...actual,
    useNavigate: () => mockNavigate,
    useLocation: () => ({
      pathname: '/history',
      search: mockLocationSearch,
      hash: '',
      state: null,
      key: 'test',
    }),
  };
});

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: stableT,
  }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetConversationsByIDs: (ids: string[]) => mockGetConversationsByIDs(ids),
  GetConversationsPage: (limit: number, offset: number) => mockGetConversationsPage(limit, offset),
  DeleteConversation: (id: string) => mockDeleteConversation(id),
  UpdateConversation: (id: string, title: string, snippet: string) => mockUpdateConversation(id, title, snippet),
  ExportConversations: (ids: string[]) => mockExportConversations(ids),
  ExportConversationsToFile: (ids: string[], format: string, options: unknown) => mockExportConversationsToFile(ids, format, options),
  SearchConversationHistory: (query: string, limit: number) => mockSearchConversationHistory(query, limit),
}));

vi.mock('../lib/exportImport', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/exportImport')>();
  return {
    ...actual,
    downloadJSON: (data: string, filename: string) => mockDownloadJSON(data, filename),
    generateFilename: () => 'conversas_test.json',
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
  Toolbar: ({
    left,
    actions,
    searchValue,
    onSearchChange,
  }: {
    left?: ReactNode;
    actions?: Array<{ key: string; label: string; onClick: () => void; disabled?: boolean }>;
    searchValue?: string;
    onSearchChange?: (value: string) => void;
  }) => {
    lastToolbarActions = actions ?? [];
    return (
      <div>
        {left}
        <input
          aria-label="history-search"
          value={searchValue ?? ''}
          onChange={(event) => onSearchChange?.(event.target.value)}
        />
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
    onNearEnd,
    onCellEdit,
  }: {
    items?: ConversationItem[];
    onSelectionChange?: (selected: Set<string | number>) => void;
    onFocusChange?: (item: ConversationItem | null) => void;
    getRowActions?: (item: ConversationItem) => Array<{ id: string; label?: string; action?: () => void }>;
    onNearEnd?: () => void;
    onCellEdit?: (item: ConversationItem, column: { key: string }, newValue: string, rowIndex: number, colIndex: number) => void;
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
      <button type="button" onClick={() => onNearEnd?.()}>
        near-end
      </button>
      <button type="button" onClick={() => items?.[1] && onCellEdit?.(items[1], { key: 'title' }, 'Conversa 2 renomeada', 1, 0)}>
        edit-second-title
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

// Em ordem decrescente por updatedAt (como o backend retorna e como a listagem
// unificada reordena após mesclar sub-agentes): Conversa 1 é a mais recente.
const conversations: ConversationItem[] = [
  {
    id: '01926b90-7a5a-7c4e-8d3f-000000000001',
    title: 'Conversa 1',
    createdAt: '2025-01-02T00:00:00Z',
    updatedAt: '2025-01-02T00:00:00Z',
    message_count: 2,
  },
  {
    id: '01926b90-7a5a-7c4e-8d3f-000000000002',
    title: 'Conversa 2',
    createdAt: '2025-01-01T00:00:00Z',
    updatedAt: '2025-01-01T00:00:00Z',
    message_count: 5,
  },
];

import HistoryPage from './HistoryPage';

describe('HistoryPage', { timeout: 60_000 }, () => {
  beforeEach(() => {
    mockLocationSearch = '';
    mockGetConversationsPage.mockResolvedValue({ conversations, total: conversations.length });
    mockGetConversationsByIDs.mockResolvedValue([]);
    mockDeleteConversation.mockResolvedValue(undefined);
    mockUpdateConversation.mockResolvedValue(undefined);
    mockExportConversations.mockResolvedValue('{}');
    mockExportConversationsToFile.mockResolvedValue('');
    mockSearchConversationHistory.mockResolvedValue([]);
    mockDownloadJSON.mockReset();
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
    mockNavigate.mockImplementation((to: unknown, options?: unknown) => {
      if (to === '/history' && typeof options === 'object' && options !== null && 'replace' in options && (options as { replace?: boolean }).replace === true) {
        mockLocationSearch = '';
      }
    });
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

  it('mescla sub-agentes na lista e o toggle os oculta', async () => {
    const user = userEvent.setup();
    // Listagem unificada: GetConversationsPage já retorna sub-agentes (kind=subagent).
    mockGetConversationsPage.mockResolvedValue({
      conversations: [
      {
        id: '01926b90-7a5a-7c4e-8d3f-0000000000aa',
        title: 'Sub-conversa A',
        kind: 'subagent',
        latestStatus: 'running',
        message_count: 3,
        createdAt: '2025-01-03T00:00:00Z',
        updatedAt: '2025-01-03T00:00:00Z',
      },
      ...conversations,
      ],
      total: conversations.length + 1,
    });

    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    expect(screen.getByText('Sub-conversa A')).toBeInTheDocument();

    const toggle = lastToolbarActions.find((action) => action.key === 'toggle-subagents');
    expect(toggle).toBeTruthy();
    await user.click(screen.getByRole('button', { name: toggle!.label }));

    await waitFor(() => {
      expect(screen.queryByText('Sub-conversa A')).not.toBeInTheDocument();
    });
    expect(screen.getByText('Conversa 1')).toBeInTheDocument();
  });

  it('carrega mais conversas quando o grid chega perto do fim', async () => {
    const user = userEvent.setup();
    mockGetConversationsPage
      .mockResolvedValueOnce({ conversations, total: 3 })
      .mockResolvedValueOnce({
        conversations: [{
          id: '01926b90-7a5a-7c4e-8d3f-000000000003',
          title: 'Conversa 3',
          createdAt: '2024-12-31T00:00:00Z',
          updatedAt: '2024-12-31T00:00:00Z',
          message_count: 1,
        }],
        total: 3,
      });

    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    await user.click(screen.getByRole('button', { name: 'near-end' }));

    await waitFor(() => {
      expect(screen.getByText('Conversa 3')).toBeInTheDocument();
    });
    expect(mockGetConversationsPage).toHaveBeenNthCalledWith(1, 100, 0);
    expect(mockGetConversationsPage).toHaveBeenNthCalledWith(2, 100, 2);
  });

  it('carrega outra pagina quando ocultar sub-agentes esvazia a grade', async () => {
    const user = userEvent.setup();
    mockGetConversationsPage
      .mockResolvedValueOnce({
        conversations: [{
          id: '01926b90-7a5a-7c4e-8d3f-0000000000aa',
          title: 'Sub-conversa única',
          kind: 'subagent',
          latestStatus: 'running',
          message_count: 1,
          createdAt: '2025-01-03T00:00:00Z',
          updatedAt: '2025-01-03T00:00:00Z',
        }],
        total: 2,
      })
      .mockResolvedValueOnce({
        conversations: [{
          id: '01926b90-7a5a-7c4e-8d3f-000000000004',
          title: 'Conversa normal seguinte',
          message_count: 1,
          createdAt: '2025-01-02T00:00:00Z',
          updatedAt: '2025-01-02T00:00:00Z',
        }],
        total: 2,
      });

    render(<HistoryPage />);

    await screen.findByText('Sub-conversa única');
    const toggle = lastToolbarActions.find((action) => action.key === 'toggle-subagents');
    expect(toggle).toBeTruthy();
    await user.click(screen.getByRole('button', { name: toggle!.label }));

    await waitFor(() => {
      expect(mockGetConversationsPage).toHaveBeenNthCalledWith(2, 100, 1);
      expect(screen.getByText('Conversa normal seguinte')).toBeInTheDocument();
    });
  });

  it('continua auto-fill ate encontrar conversa comum quando sub-agentes ocultos esvaziam a grade', async () => {
    const user = userEvent.setup();
    const subAgentPages = Array.from({ length: 4 }, (_, index) => ({
      id: `01926b90-7a5a-7c4e-8d3f-0000000000a${index}`,
      title: `Sub-conversa ${index + 1}`,
      kind: 'subagent',
      latestStatus: 'running',
      message_count: 1,
      createdAt: `2025-01-0${5 - index}T00:00:00Z`,
      updatedAt: `2025-01-0${5 - index}T00:00:00Z`,
    }));
    mockGetConversationsPage
      .mockResolvedValueOnce({ conversations: [subAgentPages[0]], total: 5 })
      .mockResolvedValueOnce({ conversations: [subAgentPages[1]], total: 5 })
      .mockResolvedValueOnce({ conversations: [subAgentPages[2]], total: 5 })
      .mockResolvedValueOnce({ conversations: [subAgentPages[3]], total: 5 })
      .mockResolvedValueOnce({
        conversations: [{
          id: '01926b90-7a5a-7c4e-8d3f-000000000005',
          title: 'Conversa comum distante',
          message_count: 1,
          createdAt: '2025-01-01T00:00:00Z',
          updatedAt: '2025-01-01T00:00:00Z',
        }],
        total: 5,
      });

    render(<HistoryPage />);

    await screen.findByText('Sub-conversa 1');
    const toggle = lastToolbarActions.find((action) => action.key === 'toggle-subagents');
    expect(toggle).toBeTruthy();
    await user.click(screen.getByRole('button', { name: toggle!.label }));

    await waitFor(() => {
      expect(mockGetConversationsPage).toHaveBeenNthCalledWith(5, 100, 4);
      expect(screen.getByText('Conversa comum distante')).toBeInTheDocument();
    });
  });

  it('renderiza resultado de busca fora da pagina inicial', async () => {
    const user = userEvent.setup();
    const olderConversation = {
      id: '01926b90-7a5a-7c4e-8d3f-000000000099',
      title: 'Conversa antiga encontrada',
      createdAt: '2024-01-01T00:00:00Z',
      updatedAt: '2024-01-01T00:00:00Z',
      message_count: 9,
    };
    mockSearchConversationHistory.mockResolvedValue([{
      conversation_id: olderConversation.id,
      snippet: 'resultado >>>especial<<<',
    }]);
    mockGetConversationsByIDs.mockResolvedValue([olderConversation]);

    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    await user.type(screen.getByLabelText('history-search'), 'especial');

    await waitFor(() => {
      expect(mockGetConversationsByIDs).toHaveBeenCalledWith([olderConversation.id]);
      expect(screen.getByText('Conversa antiga encontrada')).toBeInTheDocument();
    });
  });

  it('exporta conversa encontrada pela busca mesmo fora da pagina carregada', async () => {
    const user = userEvent.setup();
    const olderConversation = {
      id: '01926b90-7a5a-7c4e-8d3f-000000000088',
      title: 'Conversa exportável encontrada',
      createdAt: '2024-01-01T00:00:00Z',
      updatedAt: '2024-01-01T00:00:00Z',
      message_count: 3,
    };
    mockSearchConversationHistory.mockResolvedValue([{
      conversation_id: olderConversation.id,
      snippet: 'resultado exportável',
    }]);
    mockGetConversationsByIDs.mockResolvedValue([olderConversation]);

    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    await user.type(screen.getByLabelText('history-search'), 'exportável');
    await screen.findByText('Conversa exportável encontrada');
    await user.click(screen.getByRole('button', { name: 'focus-first' }));
    await user.click(screen.getByRole('button', { name: 'Exportar JSON' }));

    await waitFor(() => {
      expect(mockExportConversations).toHaveBeenCalledWith([olderConversation.id]);
    });
  });

  it('ignora resultado de busca que termina depois do campo ser limpo', async () => {
    const user = userEvent.setup();
    const staleConversation = {
      id: '01926b90-7a5a-7c4e-8d3f-000000000077',
      title: 'Busca obsoleta',
      createdAt: '2024-01-01T00:00:00Z',
      updatedAt: '2024-01-01T00:00:00Z',
      message_count: 1,
    };
    let resolveSearchRows: ((value: unknown) => void) | undefined;
    mockSearchConversationHistory.mockResolvedValue([{
      conversation_id: staleConversation.id,
      snippet: 'resultado antigo',
    }]);
    mockGetConversationsByIDs.mockImplementationOnce(() => new Promise((resolve) => {
      resolveSearchRows = resolve;
    }));

    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    const search = screen.getByLabelText('history-search');
    await user.type(search, 'antigo');
    await waitFor(() => {
      expect(mockGetConversationsByIDs).toHaveBeenCalledWith([staleConversation.id]);
    });
    await user.clear(search);

    await act(async () => {
      resolveSearchRows?.([staleConversation]);
    });

    await waitFor(() => {
      expect(screen.queryByText('Busca obsoleta')).not.toBeInTheDocument();
      expect(screen.getByText('Conversa 1')).toBeInTheDocument();
    });
  });

  it('limpa indicador de busca ao apagar termo durante requisicao pendente', async () => {
    const user = userEvent.setup();
    let resolveSearch: ((value: unknown) => void) | undefined;
    mockSearchConversationHistory.mockImplementationOnce(() => new Promise((resolve) => {
      resolveSearch = resolve;
    }));

    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    const search = screen.getByLabelText('history-search');
    await user.type(search, 'pendente');
    await waitFor(() => {
      expect(screen.getByText('Buscando...')).toBeInTheDocument();
    });

    await user.clear(search);

    await waitFor(() => {
      expect(screen.queryByText('Buscando...')).not.toBeInTheDocument();
    });
    await act(async () => {
      resolveSearch?.([]);
    });
    expect(screen.queryByText('Buscando...')).not.toBeInTheDocument();
  });

  it('nao mostra importacao administrativa no historico', async () => {
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');

    expect(screen.queryByRole('button', { name: 'Importar' })).not.toBeInTheDocument();
  });

  it('exporta JSON apenas para conversas selecionadas', async () => {
    const user = userEvent.setup();

    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    await user.click(screen.getByRole('button', { name: 'select-one' }));
    const exportButton = await screen.findByRole('button', { name: 'Exportar JSON (1)' });
    await user.click(exportButton);

    await waitFor(() => {
      expect(mockExportConversations).toHaveBeenCalledWith(['01926b90-7a5a-7c4e-8d3f-000000000001']);
    });
    expect(mockDownloadJSON).toHaveBeenCalledWith('{}', 'conversas_test.json');
  });

  it('exporta Markdown com toggles via modal de opcoes', async () => {
    const user = userEvent.setup();

    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    await user.click(screen.getByRole('button', { name: 'select-one' }));
    await user.click(await screen.findByRole('button', { name: 'Exportar Markdown' }));

    const dialog = await screen.findByRole('dialog');
    // Desmarca o toggle de reasoning antes de confirmar.
    await user.click(within(dialog).getByLabelText('Incluir raciocínio (reasoning)'));
    await user.click(within(dialog).getByRole('button', { name: 'Exportar' }));

    await waitFor(() => {
      expect(mockExportConversationsToFile).toHaveBeenCalledWith(
        ['01926b90-7a5a-7c4e-8d3f-000000000001'],
        'md',
        expect.objectContaining({
          includeTimestamps: true,
          includeReasoning: false,
          includeMetadata: true,
        }),
      );
    });
  });

  it('nao exporta conversa focada que ja foi removida da lista', async () => {
    const user = userEvent.setup();
    render(<HistoryPage />);

    await waitFor(() => {
      expect(screen.getByText('Conversa 2')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: 'focus-second' }));
    await user.click(screen.getAllByRole('button', { name: 'Excluir conversa' })[0]);

    await waitFor(() => {
      expect(mockDeleteConversation).toHaveBeenCalledWith('01926b90-7a5a-7c4e-8d3f-000000000002');
    });
    expect(screen.queryByText('Conversa 2')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Exportar JSON' }));

    expect(mockExportConversations).not.toHaveBeenCalled();
  });

  it('atualiza updatedAt e reordena conversa após editar título', async () => {
    const user = userEvent.setup();
    const toISOStringSpy = vi.spyOn(Date.prototype, 'toISOString').mockReturnValueOnce('2026-01-01T00:00:00.000Z');
    mockGetConversationsPage.mockResolvedValue({
      conversations: [
        { ...conversations[0], updatedAt: '' },
        conversations[1],
      ],
      total: conversations.length,
    });

    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    await user.click(screen.getByRole('button', { name: 'edit-second-title' }));

    await waitFor(() => {
      expect(mockUpdateConversation).toHaveBeenCalledWith(
        '01926b90-7a5a-7c4e-8d3f-000000000002',
        'Conversa 2 renomeada',
        '',
      );
    });
    const first = screen.getByText('Conversa 2 renomeada');
    const second = screen.getByText('Conversa 1');
    expect(first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    toISOStringSpy.mockRestore();
  });
});
