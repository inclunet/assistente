import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi, beforeEach } from 'vitest';

const mockImportDataWithResolutions = vi.fn();
const mockListWorkspaces = vi.fn();
const mockAnnounce = vi.fn();
const mockTranslation = vi.hoisted(() => (key: string, fallbackOrOptions?: string | Record<string, unknown>) => {
  const options = typeof fallbackOrOptions === 'string' ? { defaultValue: fallbackOrOptions } : (fallbackOrOptions || {});
  let value = String(options.defaultValue || key);
  const replacements = { ...options, ...((options.replace as Record<string, unknown> | undefined) || {}) };
  Object.entries(replacements).forEach(([name, replacement]) => {
    if (name === 'defaultValue' || name === 'replace') return;
    value = value.split(`{{${name}}}`).join(String(replacement));
  });
  return value;
});

vi.mock('@wailsjs/go/wailsapi/ExportImport', () => ({
  ImportDataWithResolutions: (request: unknown) => mockImportDataWithResolutions(request),
}));

vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({
  ListWorkspaces: () => mockListWorkspaces(),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: mockAnnounce }),
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: mockTranslation,
  }),
}));

import { CommandLayerImportPanel, inspectCommandLayerImport } from './CommandLayerImportPanel';

const globalLayer = {
  id: 'layer-1',
  scope: { kind: 'global' },
  name: 'Global shortcuts',
  enabled: true,
};

const workspaceLayer = {
  id: 'layer-2',
  scope: { kind: 'workspace', workspaceId: 'source-workspace' },
  name: 'Workspace shortcuts',
  enabled: true,
};

function commandFile(layers: unknown[], extraResources: Record<string, unknown> = {}) {
  return JSON.stringify({
    version: 2,
    resources: { commandLayers: layers, ...extraResources },
  });
}

describe('CommandLayerImportPanel', () => {
  beforeEach(() => {
    mockImportDataWithResolutions.mockReset().mockResolvedValue({
      success: true,
      imported: 1,
      skipped: 0,
      failed: 0,
      warnings: [],
      errors: [],
      message: '',
    });
    mockListWorkspaces.mockReset().mockResolvedValue([
      { id: 'destination-workspace', name: 'Destino' },
    ]);
    mockAnnounce.mockReset();
  });

  it('classifica commandLayers e recusa recursos mistos na classificação inicial', () => {
    expect(inspectCommandLayerImport(commandFile([globalLayer]))).toEqual({ kind: 'commandLayers', layerCount: 1 });
    expect(inspectCommandLayerImport(commandFile([globalLayer], { conversations: [{ id: 'conversation-1' }] })).kind).toBe('mixed');
    expect(inspectCommandLayerImport('{quebrado').kind).toBe('invalid');
  });

  it('envia política, mapa explícito de workspace e nome opcional de cópia', async () => {
    const user = userEvent.setup();
    render(<CommandLayerImportPanel selection={{ fileName: 'commands.json', jsonData: commandFile([workspaceLayer]) }} onChangeFile={vi.fn()} />);

    await screen.findByText('Destino (destination-workspace)');
    await user.selectOptions(screen.getByRole('combobox', { name: /^Política de conflito/ }), 'copy');
    await user.type(screen.getByLabelText('Nome para Workspace shortcuts'), 'Cópia dos atalhos');
    await user.selectOptions(screen.getByRole('combobox', { name: /^Destino para source-workspace/ }), 'destination-workspace');
    await user.click(screen.getByRole('button', { name: 'Aplicar importação' }));

    await waitFor(() => expect(mockImportDataWithResolutions).toHaveBeenCalled());
    const request = mockImportDataWithResolutions.mock.calls[0][0] as {
      jsonData: string;
      resolutions: Array<Record<string, unknown>>;
    };
    expect(request.jsonData).toBe(commandFile([workspaceLayer]));
    expect(request.resolutions.map(({ resourceType, identifier, strategy, renameValue }) => ({ resourceType, identifier, strategy, renameValue }))).toEqual([
      { resourceType: 'commandLayers', identifier: '*', strategy: 'rename', renameValue: undefined },
      { resourceType: 'commandLayerName', identifier: 'layer-2', strategy: 'rename', renameValue: 'Cópia dos atalhos' },
      { resourceType: 'commandWorkspace', identifier: 'source-workspace', strategy: 'rename', renameValue: 'destination-workspace' },
    ]);
  });

  it('exige política sem selecionar opção destrutiva e bloqueia nova submissão após relatório', async () => {
    const user = userEvent.setup();
    mockImportDataWithResolutions.mockResolvedValue({
      success: false,
      imported: 1,
      skipped: 0,
      failed: 0,
      warnings: [{ code: 'commandImport.committedNotPublished', params: {}, message: 'gravado' }],
      errors: [],
      message: '',
    });
    render(<CommandLayerImportPanel selection={{ fileName: 'commands.json', jsonData: commandFile([globalLayer]) }} onChangeFile={vi.fn()} />);

    const apply = screen.getByRole('button', { name: 'Aplicar importação' });
    await user.click(apply);
    expect(mockImportDataWithResolutions).not.toHaveBeenCalled();
    expect(screen.getByText('Selecione uma política de conflito antes de importar.')).toBeInTheDocument();

    await user.selectOptions(screen.getByRole('combobox', { name: /^Política de conflito/ }), 'replace');
    await user.click(apply);
    await screen.findByText('Este arquivo não pode ser aplicado novamente. Troque o arquivo para iniciar outra importação.');
    expect(screen.getByText('gravado')).toBeInTheDocument();
    expect(apply).toBeDisabled();
    expect(mockImportDataWithResolutions).toHaveBeenCalledTimes(1);
  });

  it('impede duplicação enquanto a confirmação do backend está pendente', async () => {
    const user = userEvent.setup();
    let resolveImport: (value: unknown) => void = () => {};
    mockImportDataWithResolutions.mockReturnValue(new Promise((resolve) => { resolveImport = resolve; }));
    render(<CommandLayerImportPanel selection={{ fileName: 'commands.json', jsonData: commandFile([globalLayer]) }} onChangeFile={vi.fn()} />);

    await user.selectOptions(screen.getByRole('combobox', { name: /^Política de conflito/ }), 'keep');
    const apply = screen.getByRole('button', { name: 'Aplicar importação' });
    await user.click(apply);
    expect(apply).toBeDisabled();
    await user.click(apply);
    expect(mockImportDataWithResolutions).toHaveBeenCalledTimes(1);

    await waitFor(() => expect(mockImportDataWithResolutions).toHaveBeenCalledTimes(1));
    resolveImport({ success: true, imported: 1, skipped: 0, failed: 0, warnings: [], errors: [], message: '' });
    await waitFor(() => expect(apply).toBeDisabled());
  });

  it('bloqueia arquivo acima de 64 KiB no próprio painel', () => {
    const oversizedFile = commandFile([{ ...globalLayer, name: 'x'.repeat(65 * 1024) }]);
    render(<CommandLayerImportPanel selection={{ fileName: 'large-commands.json', jsonData: oversizedFile }} onChangeFile={vi.fn()} />);

    expect(screen.getByText(/O arquivo excede o limite de 64 KiB/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Aplicar importação' })).toBeDisabled();
  });

  it('limpa o relatório e a política quando o parent troca a seleção', async () => {
    const user = userEvent.setup();
    const firstSelection = { fileName: 'commands-1.json', jsonData: commandFile([globalLayer]) };
    const secondSelection = { fileName: 'commands-2.json', jsonData: commandFile([{ ...globalLayer, id: 'layer-2' }]) };
    const view = render(<CommandLayerImportPanel key="first" selection={firstSelection} onChangeFile={vi.fn()} />);

    await user.selectOptions(screen.getByRole('combobox', { name: /^Política de conflito/ }), 'keep');
    await user.click(screen.getByRole('button', { name: 'Aplicar importação' }));
    await screen.findByText('Este arquivo não pode ser aplicado novamente. Troque o arquivo para iniciar outra importação.');

    view.rerender(<CommandLayerImportPanel key="second" selection={secondSelection} onChangeFile={vi.fn()} />);

    expect(screen.queryByText('Este arquivo não pode ser aplicado novamente. Troque o arquivo para iniciar outra importação.')).not.toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: /^Política de conflito/ })).toHaveValue('');
    expect(screen.getByRole('button', { name: 'Aplicar importação' })).not.toBeDisabled();
  });
});
