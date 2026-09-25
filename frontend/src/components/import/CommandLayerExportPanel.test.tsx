import { render, screen, waitFor } from '@testing-library/react';
import { StrictMode } from 'react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const mockExportData = vi.fn();
const mockDownloadJSON = vi.fn();
const mockAnnounce = vi.fn();
const mockTranslation = vi.hoisted(() => (key: string, fallbackOrOptions?: string | Record<string, unknown>) => {
  const options = typeof fallbackOrOptions === 'string' ? { defaultValue: fallbackOrOptions } : (fallbackOrOptions || {});
  return String(options.defaultValue || key);
});

vi.mock('@wailsjs/go/wailsapi/ExportImport', () => ({ ExportData: (request: unknown) => mockExportData(request) }));
vi.mock('../../lib/exportImport', () => ({
  downloadJSON: (data: string, filename: string) => mockDownloadJSON(data, filename),
  generateFilename: () => 'camadas-comandos_test.json',
}));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: mockAnnounce }) }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: mockTranslation }) }));

import { CommandLayerExportPanel } from './CommandLayerExportPanel';

describe('CommandLayerExportPanel', () => {
  beforeEach(() => {
    mockExportData.mockReset().mockResolvedValue('{"resources":{"commandLayers":[]}}');
    mockDownloadJSON.mockReset();
    mockAnnounce.mockReset();
  });

  it.each([
    ['command_export.empty', 'Não há camadas ou personalizações para exportar neste escopo.'],
    ['command_export.limit', 'A exportação excede o limite de 64 entradas ou 64 KiB. Nenhum arquivo foi gerado.'],
  ])('explica a recusa segura %s sem baixar arquivo', async (code, message) => {
    mockExportData.mockRejectedValue(code);
    render(<CommandLayerExportPanel />);
    await userEvent.setup().click(screen.getByRole('button', { name: 'Exportar camadas de comandos' }));
    expect(await screen.findByText(message)).toBeInTheDocument();
    expect(mockDownloadJSON).not.toHaveBeenCalled();
    expect(mockAnnounce).toHaveBeenCalledWith(message, 'assertive');
  });

  it('envia o escopo explícito e baixa o JSON exportado', async () => {
    const user = userEvent.setup();
    render(<CommandLayerExportPanel />);
    await user.click(screen.getByRole('checkbox', { name: 'Incluir workspaces acessíveis' }));
    await user.click(screen.getByRole('button', { name: 'Exportar camadas de comandos' }));

    await waitFor(() => expect(mockDownloadJSON).toHaveBeenCalledWith('{"resources":{"commandLayers":[]}}', 'camadas-comandos_test.json'));
    expect(mockExportData).toHaveBeenCalledWith(expect.objectContaining({
      explicitSelection: true,
      includeCommandLayers: true,
      includeWorkspace: true,
      outputFormat: 'json',
    }));
    expect(mockExportData.mock.calls[0][0].commandLayerIds).toBeUndefined();
    expect(mockExportData.mock.calls[0][0].includeCredentials).toBeUndefined();
    const wireRequest = JSON.parse(JSON.stringify(mockExportData.mock.calls[0][0])) as Record<string, unknown>;
    expect(wireRequest).not.toHaveProperty('commandLayerIds');
    expect(wireRequest).not.toHaveProperty('includeCredentials');
  });

  it('mostra erro genérico sem baixar', async () => {
    const user = userEvent.setup();
    mockExportData.mockRejectedValue(new Error('payload secreto'));
    render(<CommandLayerExportPanel />);
    await user.click(screen.getByRole('button', { name: 'Exportar camadas de comandos' }));
    expect(await screen.findByText('Não foi possível exportar as camadas de comandos.')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(mockDownloadJSON).not.toHaveBeenCalled();
  });

  it('mantém o download de sucesso sob StrictMode', async () => {
    const user = userEvent.setup();
    render(<StrictMode><CommandLayerExportPanel /></StrictMode>);
    await user.click(screen.getByRole('button', { name: 'Exportar camadas de comandos' }));
    await waitFor(() => expect(mockDownloadJSON).toHaveBeenCalledTimes(1));
  });

  it('não duplica a exportação durante uma chamada pendente', async () => {
    const user = userEvent.setup();
    mockExportData.mockReturnValue(new Promise(() => {}));
    render(<CommandLayerExportPanel />);
    const button = screen.getByRole('button', { name: 'Exportar camadas de comandos' });
    await user.click(button);
    await user.click(button);
    expect(mockExportData).toHaveBeenCalledTimes(1);
  });

  it('ignora download tardio após desmontagem', async () => {
    let resolveExport: (value: string) => void = () => {};
    mockExportData.mockReturnValue(new Promise((resolve) => { resolveExport = resolve; }));
    const view = render(<CommandLayerExportPanel />);
    await userEvent.setup().click(screen.getByRole('button', { name: 'Exportar camadas de comandos' }));
    view.unmount();
    resolveExport('{"late":true}');
    await Promise.resolve();
    expect(mockDownloadJSON).not.toHaveBeenCalled();
  });
});
