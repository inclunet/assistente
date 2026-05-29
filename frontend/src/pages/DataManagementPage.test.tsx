import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockAnalyzeImportData = vi.fn();
const mockExportData = vi.fn();
const mockGetAllTaskLists = vi.fn();
const mockGetConversations = vi.fn();
const mockGetLLMProvidersWithStatus = vi.fn();
const mockImportData = vi.fn();
const mockListMCPServers = vi.fn();
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

  it('mostra preview e confirma importacao', async () => {
    const user = userEvent.setup();
    const jsonData = JSON.stringify({
      version: 2,
      appVersion: '0.9.0',
      exportedAt: '2025-01-01T00:00:00Z',
      options: { includeCredentials: false, includeAudio: false },
      resources: { conversations: [], providers: [], taskLists: [] },
    });
    mockOpenImportFileDialog.mockResolvedValue({ name: 'backup.json', content: jsonData });
    mockAnalyzeImportData.mockResolvedValue({ conflictCount: 0 });

    render(<DataManagementPage />);
    await user.click(screen.getByRole('button', { name: 'Selecionar arquivo JSON' }));

    await waitFor(() => {
      expect(mockAnalyzeImportData).toHaveBeenCalledWith(jsonData, '');
    });
    expect(screen.getByText('backup.json')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Importar agora' }));
    await waitFor(() => {
      expect(mockImportData).toHaveBeenCalledWith(jsonData, '');
    });
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

    await waitFor(() => {
      expect(mockGetConversations).toHaveBeenCalled();
    });
    expect(mockOpenImportFileDialog).not.toHaveBeenCalled();
    expect(mockNavigate).not.toHaveBeenCalled();
  });
});
