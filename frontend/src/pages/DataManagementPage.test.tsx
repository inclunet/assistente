import { describe, expect, it, vi, beforeEach } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockAnalyzeImportData = vi.fn();
const mockExportData = vi.fn();
const mockGetAllTaskLists = vi.fn();
const mockGetConversations = vi.fn();
const mockGetLLMProvidersWithStatus = vi.fn();
const mockImportData = vi.fn();
const mockListMCPServers = vi.fn();
const mockListMemoryRecords = vi.fn();
const mockDownloadJSON = vi.fn();
const mockOpenImportFileDialog = vi.fn();
const mockAnnounce = vi.fn();
const mockNavigate = vi.fn();
let mockSearch = '';
let mockPathname = '/settings/data';

vi.mock('@wailsjs/go/app/App', () => ({
  AnalyzeImportData: (payload: string, password: string) => mockAnalyzeImportData(payload, password),
  ExportData: (payload: unknown) => mockExportData(payload),
  GetAllTaskLists: () => mockGetAllTaskLists(),
  GetConversations: () => mockGetConversations(),
  GetLLMProvidersWithStatus: () => mockGetLLMProvidersWithStatus(),
  ImportData: (payload: string, password: string) => mockImportData(payload, password),
  ListMCPServers: () => mockListMCPServers(),
  ListMemoryRecords: (filter: unknown) => mockListMemoryRecords(filter),
}));

vi.mock('../lib/exportImport', async () => {
  const actual = await vi.importActual<typeof import('../lib/exportImport')>('../lib/exportImport');
  return {
    ...actual,
    downloadJSON: (data: string, filename: string) => mockDownloadJSON(data, filename),
    openImportFileDialog: (accept?: string) => mockOpenImportFileDialog(accept),
    generateFilename: () => 'dados_test.json',
  };
});

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: mockAnnounce }),
}));

vi.mock('react-router-dom', () => ({
  useLocation: () => ({ pathname: mockPathname, search: mockSearch }),
  useNavigate: () => mockNavigate,
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, fallbackOrOptions?: string | { defaultValue?: string }) => {
      if (typeof fallbackOrOptions === 'string') return fallbackOrOptions;
      return fallbackOrOptions?.defaultValue ?? _key;
    },
  }),
}));

import DataManagementPage from './DataManagementPage';

describe('DataManagementPage', () => {
  beforeEach(() => {
    mockAnalyzeImportData.mockReset();
    mockExportData.mockReset().mockResolvedValue('{}');
    mockGetAllTaskLists.mockReset().mockResolvedValue([]);
    mockGetConversations.mockReset().mockResolvedValue([
      { id: 'conversation-1' },
      { id: 'conversation-2' },
    ]);
    mockGetLLMProvidersWithStatus.mockReset().mockResolvedValue([]);
    mockImportData.mockReset().mockResolvedValue({
      success: true,
      imported: 1,
      skipped: 0,
      failed: 0,
      skippedEmptyConversations: 0,
      skippedConversationConflict: 0,
      skippedProviderConflict: 0,
      skippedMcpServerConflict: 0,
      skippedTaskListConflict: 0,
      skippedCredentialConflict: 0,
      skippedOther: 0,
      message: 'ok',
    });
    mockListMCPServers.mockReset().mockResolvedValue([]);
    mockListMemoryRecords.mockReset().mockResolvedValue({ records: [], total: 0 });
    mockDownloadJSON.mockReset();
    mockOpenImportFileDialog.mockReset();
    mockAnnounce.mockReset();
    mockNavigate.mockReset();
    mockSearch = '';
    mockPathname = '/settings/data';
  });

  it('exporta conversas do historico a partir da aba de dados', async () => {
    const user = userEvent.setup();
    render(<DataManagementPage />);

    await waitFor(() => {
      expect(screen.getByText('2')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: 'Exportar agora' }));

    await waitFor(() => {
      expect(mockExportData).toHaveBeenCalledWith(expect.objectContaining({
        explicitSelection: true,
        includeCredentials: false,
        outputFormat: 'json',
        conversationIds: ['conversation-1', 'conversation-2'],
      }));
    });
    expect(mockDownloadJSON).toHaveBeenCalledWith('{}', 'dados_test.json');
  });

  it('bloqueia exportacao enquanto conversas padrao estao carregando', async () => {
    let resolveConversations: (value: Array<{ id: string }>) => void = () => {};
    mockGetConversations.mockReturnValue(new Promise((resolve) => {
      resolveConversations = resolve;
    }));

    render(<DataManagementPage />);

    expect(screen.getByRole('button', { name: 'Exportar agora' })).toBeDisabled();

    resolveConversations([{ id: 'conversation-1' }]);
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Exportar agora' })).not.toBeDisabled();
    });
  });

  it('mostra conversas como nao incluidas quando exportacao de conversas esta desmarcada', async () => {
    const user = userEvent.setup();

    render(<DataManagementPage />);

    await waitFor(() => {
      expect(screen.getByText('2')).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText('Incluir conversas do histórico'));

    const conversationsSummary = screen.getByText('Conversas').closest('div');
    expect(conversationsSummary).toHaveTextContent('Não incluir');
  });

  it('exporta providers sem depender do historico', async () => {
    const user = userEvent.setup();
    mockGetConversations.mockResolvedValue([]);
    mockGetLLMProvidersWithStatus.mockResolvedValue([{ id: 'provider-1' }]);
    render(<DataManagementPage />);

    await user.click(await screen.findByLabelText('Incluir providers persistidos no banco'));
    await user.click(screen.getByRole('button', { name: 'Exportar agora' }));

    await waitFor(() => {
      expect(mockExportData).toHaveBeenCalledWith(expect.objectContaining({
        providerIds: ['provider-1'],
      }));
    });
    expect(mockExportData.mock.calls[0][0]).not.toHaveProperty('conversationIds');
  });

  it('exporta servidores MCP em formato mcp-json e limpa opcoes incompatíveis', async () => {
    const user = userEvent.setup();
    mockListMCPServers.mockResolvedValue([{ slug: 'server-1' }]);

    render(<DataManagementPage />);

    await waitFor(() => {
      expect(screen.getByText('2')).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText('Incluir credenciais criptografadas no export'));
    await user.type(screen.getByPlaceholderText('Digite a senha de exportação'), 'segredo');
    await user.click(screen.getByLabelText('Incluir servidores MCP persistidos no banco'));

    await waitFor(() => {
      expect(mockListMCPServers).toHaveBeenCalled();
    });

    await user.click(screen.getByLabelText('Exportar servidores MCP em formato compatível (mcp-json)'));

    expect(screen.getByLabelText('Incluir conversas do histórico')).toBeDisabled();
    expect(screen.getByLabelText('Incluir providers persistidos no banco')).toBeDisabled();
    expect(screen.getByLabelText('Incluir tasklists persistidas no banco')).toBeDisabled();
    expect(screen.getByLabelText('Incluir credenciais criptografadas no export')).toBeDisabled();
    expect(screen.queryByPlaceholderText('Digite a senha de exportação')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Exportar agora' }));

    await waitFor(() => {
      expect(mockExportData).toHaveBeenCalledWith(expect.objectContaining({
        explicitSelection: true,
        includeCredentials: false,
        outputFormat: 'mcp-json',
        mcpServerSlugs: ['server-1'],
      }));
    });
    expect(mockExportData.mock.calls[0][0]).not.toHaveProperty('conversationIds');
    expect(mockExportData.mock.calls[0][0]).not.toHaveProperty('providerIds');
    expect(mockExportData.mock.calls[0][0]).not.toHaveProperty('taskListIds');
    expect(mockExportData.mock.calls[0][0]).not.toHaveProperty('credentialExportPassword');
  });

  it('exige senha para exportar credenciais e envia senha aparada', async () => {
    const user = userEvent.setup();

    render(<DataManagementPage />);

    await waitFor(() => {
      expect(screen.getByText('2')).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText('Incluir credenciais criptografadas no export'));
    await user.click(screen.getByRole('button', { name: 'Exportar agora' }));

    expect(mockExportData).not.toHaveBeenCalled();
    expect(screen.getByText('Informe uma senha para criptografar as credenciais exportadas.')).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText('Digite a senha de exportação'), '  segredo  ');
    await user.click(screen.getByRole('button', { name: 'Exportar agora' }));

    await waitFor(() => {
      expect(mockExportData).toHaveBeenCalledWith(expect.objectContaining({
        explicitSelection: true,
        includeCredentials: true,
        credentialExportPassword: 'segredo',
        outputFormat: 'json',
        conversationIds: ['conversation-1', 'conversation-2'],
      }));
    });
  });

  it('mostra preview e confirma importacao', async () => {
    const user = userEvent.setup();
    const jsonData = JSON.stringify({
      version: 2,
      appVersion: '0.9.0',
      exportedAt: '2025-01-01T00:00:00Z',
      options: { includeCredentials: false, includeAudio: false },
      resources: {
        conversations: [],
        providers: [],
        taskLists: [],
        memoryRecords: [{ id: 'mem-1' }, { id: 'mem-2' }, { id: 'mem-3' }],
      },
    });
    mockOpenImportFileDialog.mockResolvedValue({ name: 'backup.json', content: jsonData });
    mockAnalyzeImportData.mockResolvedValue({ conflictCount: 0 });

    render(<DataManagementPage />);
    await user.click(screen.getByRole('button', { name: 'Selecionar arquivo JSON' }));

    await waitFor(() => {
      expect(mockAnalyzeImportData).toHaveBeenCalledWith(jsonData, '');
    });
    expect(screen.getByText('backup.json')).toBeInTheDocument();
    expect(screen.getByText('3')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Importar agora' }));
    await waitFor(() => {
      expect(mockImportData).toHaveBeenCalledWith(jsonData, '');
    });
  });

  it('rotula erros e avisos do resultado de importacao', async () => {
    const user = userEvent.setup();
    const jsonData = JSON.stringify({
      version: 2,
      options: { includeCredentials: false, includeAudio: false },
      resources: { conversations: [], providers: [], taskLists: [] },
    });
    mockOpenImportFileDialog.mockResolvedValue({ name: 'backup.json', content: jsonData });
    mockAnalyzeImportData.mockResolvedValue({ conflictCount: 0 });
    mockImportData.mockResolvedValue({
      success: false,
      imported: 0,
      skipped: 1,
      failed: 1,
      skippedEmptyConversations: 0,
      skippedConversationConflict: 0,
      skippedProviderConflict: 0,
      skippedMcpServerConflict: 0,
      skippedTaskListConflict: 0,
      skippedCredentialConflict: 0,
      skippedOther: 1,
      message: 'parcial',
      errors: ['Falha ao importar conversa'],
      warnings: ['Provider ignorado'],
    });

    render(<DataManagementPage />);
    await user.click(screen.getByRole('button', { name: 'Selecionar arquivo JSON' }));
    await user.click(await screen.findByRole('button', { name: 'Importar agora' }));

    await waitFor(() => {
      expect(screen.getByText('Resultado da importação')).toBeInTheDocument();
    });
    expect(screen.getByText('Erros')).toBeInTheDocument();
    expect(screen.getByRole('list', { name: 'Erros' })).toHaveTextContent('Falha ao importar conversa');
    expect(screen.getByText('Avisos')).toBeInTheDocument();
    expect(screen.getByRole('list', { name: 'Avisos' })).toHaveTextContent('Provider ignorado');
    expect(mockAnnounce).toHaveBeenCalledWith(
      expect.stringContaining('Falha ao importar conversa'),
      'assertive',
    );
    expect(mockAnnounce).toHaveBeenCalledWith(
      expect.stringContaining('Provider ignorado'),
      'assertive',
    );
    expect(mockAnnounce).toHaveBeenCalledWith(
      expect.stringContaining('Outros descartes'),
      'assertive',
    );
  });

  it('limpa preview quando analise inicial do arquivo falha', async () => {
    const user = userEvent.setup();
    const jsonData = JSON.stringify({
      version: 2,
      options: { includeCredentials: false, includeAudio: false },
      resources: { conversations: [], providers: [], taskLists: [] },
    });
    mockOpenImportFileDialog.mockResolvedValue({ name: 'backup.json', content: jsonData });
    mockAnalyzeImportData.mockRejectedValue(new Error('unsupported-version'));

    render(<DataManagementPage />);
    await user.click(screen.getByRole('button', { name: 'Selecionar arquivo JSON' }));

    await waitFor(() => {
      expect(mockAnnounce).toHaveBeenCalledWith('O arquivo selecionado não é um export canônico suportado.', 'assertive');
    });
    expect(screen.queryByText('backup.json')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Importar agora' })).not.toBeInTheDocument();
  });

  it('bloqueia importacao imediatamente ao alterar senha de credenciais', async () => {
    const user = userEvent.setup();
    const jsonData = JSON.stringify({
      version: 2,
      options: { includeCredentials: true, includeAudio: false },
      resources: {
        conversations: [],
        providers: [],
        taskLists: [],
        credentials: { mode: 'encrypted' },
      },
    });
    mockOpenImportFileDialog.mockResolvedValue({ name: 'backup.json', content: jsonData });
    mockAnalyzeImportData.mockResolvedValue({ conflictCount: 0 });

    render(<DataManagementPage />);
    await user.click(screen.getByRole('button', { name: 'Selecionar arquivo JSON' }));

    await waitFor(() => {
      expect(screen.getByText('Senha das credenciais')).toBeInTheDocument();
    });
    await user.type(screen.getByPlaceholderText('Digite a senha de exportação'), 'segredo');

    expect(screen.getByRole('button', { name: 'Importar agora' })).toBeDisabled();
  });

  it('reanalisar senha de credenciais ja vista quando analise atual foi limpa', async () => {
    const user = userEvent.setup();
    const jsonData = JSON.stringify({
      version: 2,
      options: { includeCredentials: true, includeAudio: false },
      resources: {
        conversations: [],
        providers: [],
        taskLists: [],
        credentials: { mode: 'encrypted' },
      },
    });
    mockOpenImportFileDialog.mockResolvedValue({ name: 'backup.json', content: jsonData });
    mockAnalyzeImportData.mockImplementation(async (_payload: string, password: string) => ({
      conflictCount: 0,
      ...(password === 'errada' ? { credentialAnalysisError: 'invalid-password' } : {}),
    }));

    render(<DataManagementPage />);
    await user.click(screen.getByRole('button', { name: 'Selecionar arquivo JSON' }));

    const passwordInput = await screen.findByPlaceholderText('Digite a senha de exportação');
    await user.type(passwordInput, 'errada');

    await waitFor(() => {
      expect(mockAnalyzeImportData).toHaveBeenCalledWith(jsonData, 'errada');
    });
    await waitFor(() => {
      expect(screen.queryByText('Analisando conflitos do arquivo...')).not.toBeInTheDocument();
    });

    await user.clear(passwordInput);
    await user.type(passwordInput, 'outra');
    await user.clear(passwordInput);
    await user.type(passwordInput, 'errada');
    await act(async () => {
      await new Promise((resolve) => window.setTimeout(resolve, 700));
    });

    await waitFor(() => {
      const invalidPasswordCalls = mockAnalyzeImportData.mock.calls.filter(([, password]) => password === 'errada');
      expect(invalidPasswordCalls).toHaveLength(2);
    });

    await user.click(screen.getByRole('button', { name: 'Importar agora' }));
    expect(mockImportData).not.toHaveBeenCalled();
    expect(screen.getByText('Não foi possível validar a senha informada para as credenciais.')).toBeInTheDocument();
  });

  it('nao duplica analise inicial de importacao criptografada', async () => {
    const user = userEvent.setup();
    const jsonData = JSON.stringify({
      version: 2,
      options: { includeCredentials: true, includeAudio: false },
      resources: {
        conversations: [],
        providers: [],
        taskLists: [],
        credentials: { mode: 'encrypted' },
      },
    });
    let resolveAnalysis: (value: { conflictCount: number }) => void = () => {};
    mockOpenImportFileDialog.mockResolvedValue({ name: 'backup.json', content: jsonData });
    mockAnalyzeImportData.mockReturnValue(new Promise((resolve) => {
      resolveAnalysis = resolve;
    }));

    render(<DataManagementPage />);
    await user.click(screen.getByRole('button', { name: 'Selecionar arquivo JSON' }));

    expect(await screen.findByText('backup.json')).toBeInTheDocument();
    await new Promise((resolve) => window.setTimeout(resolve, 700));

    expect(mockAnalyzeImportData).toHaveBeenCalledTimes(1);
    await act(async () => {
      resolveAnalysis({ conflictCount: 0 });
    });
  });

  it('ignora analise atrasada quando importacao e cancelada', async () => {
    const user = userEvent.setup();
    const jsonData = JSON.stringify({
      version: 2,
      options: { includeCredentials: false, includeAudio: false },
      resources: { conversations: [], providers: [], taskLists: [] },
    });
    let resolveAnalysis: (value: { conflictCount: number; warnings: string[] }) => void = () => {};
    mockOpenImportFileDialog.mockResolvedValue({ name: 'backup.json', content: jsonData });
    mockAnalyzeImportData.mockReturnValue(new Promise((resolve) => {
      resolveAnalysis = resolve;
    }));

    render(<DataManagementPage />);
    await user.click(screen.getByRole('button', { name: 'Selecionar arquivo JSON' }));

    expect(await screen.findByText('backup.json')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Cancelar' }));
    resolveAnalysis({ conflictCount: 0, warnings: ['Aviso atrasado'] });

    await waitFor(() => {
      expect(screen.queryByText('backup.json')).not.toBeInTheDocument();
    });
    expect(screen.queryByText('Aviso atrasado')).not.toBeInTheDocument();
  });

  it('abre importacao por action na URL e limpa querystring', async () => {
    const jsonData = JSON.stringify({
      version: 2,
      options: { includeCredentials: false, includeAudio: false },
      resources: { conversations: [], providers: [], taskLists: [] },
    });
    mockSearch = '?action=import';
    mockOpenImportFileDialog.mockResolvedValue({ name: 'backup.json', content: jsonData });
    mockAnalyzeImportData.mockResolvedValue({ conflictCount: 0 });

    render(<DataManagementPage />);

    await waitFor(() => {
      expect(mockAnalyzeImportData).toHaveBeenCalledWith(jsonData, '');
    });
    expect(mockNavigate).toHaveBeenCalledWith('/settings/data', { replace: true });
  });

  it('permite repetir a mesma action depois que a querystring e limpa', async () => {
    const jsonData = JSON.stringify({
      version: 2,
      options: { includeCredentials: false, includeAudio: false },
      resources: { conversations: [], providers: [], taskLists: [] },
    });
    mockSearch = '?action=import';
    mockOpenImportFileDialog.mockResolvedValue({ name: 'backup.json', content: jsonData });
    mockAnalyzeImportData.mockResolvedValue({ conflictCount: 0 });

    const { rerender } = render(<DataManagementPage />);

    await waitFor(() => {
      expect(mockOpenImportFileDialog).toHaveBeenCalledTimes(1);
    });

    mockSearch = '';
    rerender(<DataManagementPage />);
    mockSearch = '?action=import';
    rerender(<DataManagementPage />);

    await waitFor(() => {
      expect(mockOpenImportFileDialog).toHaveBeenCalledTimes(2);
    });
  });

  it('mostra marcador vazio quando exportedAt do preview e invalido', async () => {
    const user = userEvent.setup();
    const jsonData = JSON.stringify({
      version: 2,
      exportedAt: 'data-invalida',
      options: { includeCredentials: false, includeAudio: false },
      resources: { conversations: [], providers: [], taskLists: [] },
    });
    mockOpenImportFileDialog.mockResolvedValue({ name: 'backup.json', content: jsonData });
    mockAnalyzeImportData.mockResolvedValue({ conflictCount: 0 });

    render(<DataManagementPage />);
    await user.click(screen.getByRole('button', { name: 'Selecionar arquivo JSON' }));

    await waitFor(() => {
      expect(screen.getByText('backup.json')).toBeInTheDocument();
    });
    expect(screen.queryByText(/NaN/)).not.toBeInTheDocument();
    expect(screen.getAllByText('-').length).toBeGreaterThan(0);
  });

  it('aceita action de exportacao na URL e limpa querystring', async () => {
    mockSearch = '?action=export';

    render(<DataManagementPage />);

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/settings/data', { replace: true });
    });
    expect(mockOpenImportFileDialog).not.toHaveBeenCalled();
  });

  it('ignora action na URL quando a aba de dados esta oculta', async () => {
    mockPathname = '/settings/providers';
    mockSearch = '?action=import';

    render(<DataManagementPage />);

    expect(mockGetConversations).not.toHaveBeenCalled();
    expect(mockOpenImportFileDialog).not.toHaveBeenCalled();
    expect(mockNavigate).not.toHaveBeenCalled();
  });
});
