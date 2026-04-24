import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockGetConversations = vi.fn();
const mockDeleteConversation = vi.fn();
const mockUpdateConversation = vi.fn();
const mockAnalyzeImportData = vi.fn();
const mockExportConversations = vi.fn();
const mockExportData = vi.fn();
const mockExportConversationsToFile = vi.fn();
const mockImportConversations = vi.fn();
const mockImportData = vi.fn();
const mockSearchConversationHistory = vi.fn();
const mockOpenImportFileDialog = vi.fn();
const mockDownloadJSON = vi.fn();
const mockAnnounce = vi.fn();
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
    t: (_key: string, fallbackOrOptions?: string | Record<string, string | number | undefined>) => {
      if (typeof fallbackOrOptions === 'string') {
        return fallbackOrOptions;
      }
      if (fallbackOrOptions && typeof fallbackOrOptions === 'object') {
        const template = String(fallbackOrOptions.defaultValue ?? _key);
        return Object.entries(fallbackOrOptions).reduce((result, [key, value]) => {
          if (key === 'defaultValue' || value === undefined) {
            return result;
          }
          return result.split(`{{${key}}}`).join(String(value));
        }, template);
      }
      return _key;
    },
  }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  AnalyzeImportData: (payload: string, password: string) => mockAnalyzeImportData(payload, password),
  GetConversations: () => mockGetConversations(),
  DeleteConversation: (id: number) => mockDeleteConversation(id),
  UpdateConversation: (id: number, title: string, snippet: string) => mockUpdateConversation(id, title, snippet),
  ExportConversations: (ids: number[]) => mockExportConversations(ids),
  ExportData: (payload: unknown) => mockExportData(payload),
  ExportConversationsToFile: (ids: number[], format: string) => mockExportConversationsToFile(ids, format),
  ImportConversations: (payload: string) => mockImportConversations(payload),
  ImportData: (payload: string, password: string) => mockImportData(payload, password),
  SearchConversationHistory: (query: string, limit: number) => mockSearchConversationHistory(query, limit),
  GetLLMProvidersWithStatus: vi.fn().mockResolvedValue([]),
}));

vi.mock('../lib/exportImport', () => ({
  downloadJSON: (...args: unknown[]) => mockDownloadJSON(...args),
  generateFilename: vi.fn(() => 'conversas.json'),
  openFileDialog: vi.fn(),
  openImportFileDialog: (...args: unknown[]) => mockOpenImportFileDialog(...args),
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
    mockAnalyzeImportData.mockResolvedValue({
      conflictCount: 0,
      warnings: [],
      conversationConflicts: [],
      credentialConflicts: [],
      credentialAnalysisError: '',
    });
    mockExportConversations.mockResolvedValue('{}');
    mockExportData.mockResolvedValue('{}');
    mockExportConversationsToFile.mockResolvedValue('C:/tmp/conversas.html');
    mockImportConversations.mockResolvedValue({ success: true, message: 'ok' });
    mockImportData.mockResolvedValue({
      success: true,
      message: 'ok',
      imported: 2,
      skipped: 0,
      failed: 0,
      skippedEmptyConversations: 0,
      skippedConversationConflict: 0,
      skippedCredentialConflict: 0,
      skippedOther: 0,
      warnings: [],
      errors: [],
    });
    mockSearchConversationHistory.mockResolvedValue([]);
    mockOpenImportFileDialog.mockReset();
    mockDownloadJSON.mockReset();
    mockAnnounce.mockReset();
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

  it('abre modal de exportacao JSON pela toolbar', async () => {
    const user = userEvent.setup();
    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    const exportButtons = screen.getAllByRole('button', { name: 'Exportar JSON' });
    await user.click(exportButtons[0]);

    await screen.findByRole('heading', { name: 'Exportar conversas' });
    expect(screen.getByText('Exportar agora')).toBeInTheDocument();
  });

  it('exige senha ao exportar credenciais', async () => {
    const user = userEvent.setup();
    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    const exportButtons = screen.getAllByRole('button', { name: 'Exportar JSON' });
    await user.click(exportButtons[0]);
    await screen.findByRole('heading', { name: 'Exportar conversas' });
    await user.click(screen.getByRole('checkbox', { name: 'Incluir credenciais criptografadas no export' }));
    await user.click(screen.getByRole('button', { name: 'Exportar agora' }));

    expect(mockExportData).not.toHaveBeenCalled();
    expect(await screen.findByText('Informe uma senha para criptografar as credenciais exportadas.')).toBeInTheDocument();
  });

  it('exporta JSON usando ExportData e inclui senha de credenciais', async () => {
    const user = userEvent.setup();
    mockExportData.mockResolvedValue('{"version":1}');

    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    await user.click(screen.getByRole('button', { name: 'select-two' }));
    await user.click(screen.getByRole('button', { name: 'Exportar JSON (2)' }));

    await screen.findByRole('heading', { name: 'Exportar conversas' });
    await user.click(screen.getByRole('checkbox', { name: 'Incluir credenciais criptografadas no export' }));
    const passwordInput = await screen.findByPlaceholderText('Digite a senha de exportação');
    await user.type(passwordInput, 'segredo');
    await user.click(screen.getByRole('button', { name: 'Exportar agora' }));

    await waitFor(() => {
      expect(mockExportData).toHaveBeenCalledWith({
        conversationIds: ['1', '2'],
        includeCredentials: true,
        credentialExportPassword: 'segredo',
        outputFormat: 'json',
      });
    });
    expect(mockDownloadJSON).toHaveBeenCalledWith('{"version":1}', 'conversas.json');
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
    await waitFor(() => {
      expect(mockAnalyzeImportData).toHaveBeenCalledWith(expect.any(String), '');
    });
  });

  it('mostra conflitos detectados na analise da importacao', async () => {
    const user = userEvent.setup();
    mockOpenImportFileDialog.mockResolvedValue({
      name: 'backup.json',
      content: JSON.stringify({
        version: 1,
        options: { includeCredentials: true },
        resources: {
          conversations: [{ messages: [{}] }],
          credentials: { mode: 'encrypted' },
        },
      }),
    });
    mockAnalyzeImportData
      .mockResolvedValueOnce({
        conflictCount: 1,
        warnings: ['Informe a senha de exportação para analisar conflitos de credenciais.'],
        conversationConflicts: [
          { resourceType: 'conversation', identifier: 'x', reason: 'Já existe uma conversa com o mesmo título, canal e data de criação.' },
        ],
        credentialConflicts: [],
        credentialAnalysisError: '',
      });

    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    await user.click(screen.getByRole('button', { name: 'Importar' }));

    await screen.findByText('Conflitos detectados');
    expect(screen.getByText('1 conflito(s)')).toBeInTheDocument();
    expect(screen.getByText('Já existe uma conversa com o mesmo título, canal e data de criação.')).toBeInTheDocument();
  });

  it('avisa quando o arquivo inclui recursos fora do escopo atual', async () => {
    const user = userEvent.setup();
    mockOpenImportFileDialog.mockResolvedValue({
      name: 'backup.json',
      content: JSON.stringify({
        version: 1,
        resources: {
          conversations: [{ messages: [{}] }],
          profiles: [{ slug: 'perfil-demo' }],
          taskLists: [{ title: 'Sprint 42' }],
        },
      }),
    });
    mockAnalyzeImportData.mockResolvedValueOnce({
      conflictCount: 0,
      warnings: [],
      unsupportedResourceTypes: ['profiles', 'taskLists'],
      conversationConflicts: [],
      credentialConflicts: [],
      credentialAnalysisError: '',
    });

    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    await user.click(screen.getByRole('button', { name: 'Importar' }));

    await screen.findByText(/profiles, taskLists/);
    expect(screen.getByText(/AEP-0046, AEP-0048, AEP-0050, AEP-0051 e AEP-0052/)).toBeInTheDocument();
  });

  it('exige senha ao importar arquivo com credenciais criptografadas', async () => {
    const user = userEvent.setup();
    mockOpenImportFileDialog.mockResolvedValue({
      name: 'backup.json',
      content: JSON.stringify({
        version: 1,
        options: { includeCredentials: true },
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
      options: { includeCredentials: true },
      resources: {
        conversations: [{ messages: [{}] }],
        credentials: { mode: 'encrypted' },
      },
    });
    mockOpenImportFileDialog.mockResolvedValue({
      name: 'backup.json',
      content: payload,
    });
    mockImportData.mockResolvedValue({
      success: true,
      message: 'ok',
      imported: 1,
      skipped: 0,
      failed: 0,
      skippedEmptyConversations: 0,
      skippedConversationConflict: 0,
      skippedCredentialConflict: 0,
      skippedOther: 0,
      warnings: [],
      errors: [],
    });

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
    expect(mockAnnounce).toHaveBeenCalledWith(expect.stringContaining('ok'), 'polite');
  });

  it('mostra detalhamento do resultado apos importar', async () => {
    const user = userEvent.setup();
    const payload = JSON.stringify({
      version: 1,
      resources: {
        conversations: [{ messages: [{}] }],
      },
    });
    mockOpenImportFileDialog.mockResolvedValue({
      name: 'backup.json',
      content: payload,
    });
    mockImportData.mockResolvedValue({
      success: true,
      message: 'ok',
      imported: 1,
      skipped: 3,
      failed: 0,
      skippedEmptyConversations: 1,
      skippedConversationConflict: 1,
      skippedCredentialConflict: 1,
      skippedOther: 0,
      warnings: [],
      errors: [],
    });

    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    await user.click(screen.getByRole('button', { name: 'Importar' }));
    await user.click(screen.getByRole('button', { name: 'Importar agora' }));

    await screen.findByText('Resultado da importação');
    expect(screen.getByText('Conversas vazias')).toBeInTheDocument();
    expect(screen.getByText('Conflitos de conversa')).toBeInTheDocument();
    expect(screen.getByText('Credenciais duplicadas')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Fechar' })).toBeInTheDocument();
  });

  it('reanalisar importacao quando a senha de credenciais muda', async () => {
    const user = userEvent.setup();
    mockOpenImportFileDialog.mockResolvedValue({
      name: 'backup.json',
      content: JSON.stringify({
        version: 1,
        options: { includeCredentials: true },
        resources: {
          conversations: [],
          credentials: { mode: 'encrypted' },
        },
      }),
    });
    mockAnalyzeImportData
      .mockResolvedValueOnce({
        conflictCount: 0,
        warnings: ['Informe a senha de exportação para analisar conflitos de credenciais.'],
        conversationConflicts: [],
        credentialConflicts: [],
        credentialAnalysisError: '',
      })
      .mockResolvedValueOnce({
        conflictCount: 1,
        warnings: [],
        conversationConflicts: [],
        credentialConflicts: [
          { resourceType: 'credential', identifier: 'api.openai.com', reason: 'Já existe uma credencial registrada com o mesmo pattern.' },
        ],
        credentialAnalysisError: '',
      });

    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    await user.click(screen.getByRole('button', { name: 'Importar' }));
    const passwordInput = await screen.findByPlaceholderText('Digite a senha de exportação');
    await user.type(passwordInput, 'segredo');

    await waitFor(() => {
      expect(mockAnalyzeImportData).toHaveBeenLastCalledWith(expect.any(String), 'segredo');
    }, { timeout: 1500 });
  });

  it('nao exige senha quando credentials existe mas includeCredentials esta desabilitado', async () => {
    const user = userEvent.setup();
    mockOpenImportFileDialog.mockResolvedValue({
      name: 'backup.json',
      content: JSON.stringify({
        version: 1,
        options: { includeCredentials: false },
        resources: {
          conversations: [{ messages: [{}] }],
          credentials: { mode: 'encrypted' },
        },
      }),
    });
    mockImportData.mockResolvedValue({
      success: true,
      message: 'ok',
      imported: 1,
      skipped: 0,
      failed: 0,
      skippedEmptyConversations: 0,
      skippedConversationConflict: 0,
      skippedCredentialConflict: 0,
      skippedOther: 0,
      warnings: [],
      errors: [],
    });

    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    await user.click(screen.getByRole('button', { name: 'Importar' }));
    expect(screen.queryByPlaceholderText('Digite a senha de exportação')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Importar agora' }));

    await waitFor(() => {
      expect(mockImportData).toHaveBeenCalledWith(expect.any(String), '');
    });
  });

  it('mostra avisos no resultado final da importacao', async () => {
    const user = userEvent.setup();
    const payload = JSON.stringify({
      version: 1,
      resources: {
        conversations: [{ messages: [{}] }],
      },
    });
    mockOpenImportFileDialog.mockResolvedValue({
      name: 'backup.json',
      content: payload,
    });
    mockImportData.mockResolvedValue({
      success: true,
      message: 'ok',
      imported: 1,
      skipped: 0,
      failed: 0,
      skippedEmptyConversations: 0,
      skippedConversationConflict: 0,
      skippedCredentialConflict: 0,
      skippedOther: 0,
      unsupportedResourceTypes: ['profiles'],
      warnings: ['Este arquivo inclui recursos fora do escopo atual (profiles).'],
      errors: [],
    });

    const { default: HistoryPage } = await import('./HistoryPage');
    render(<HistoryPage />);

    await screen.findByText('Conversa 1');
    await user.click(screen.getByRole('button', { name: 'Importar' }));
    await user.click(screen.getByRole('button', { name: 'Importar agora' }));

    await screen.findByText('Resultado da importação');
    expect(screen.getByText('Avisos')).toBeInTheDocument();
    expect(screen.getByText('Este arquivo inclui recursos fora do escopo atual (profiles).')).toBeInTheDocument();
  });
});
