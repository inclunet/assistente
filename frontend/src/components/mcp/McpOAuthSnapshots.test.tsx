import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import i18n from 'i18next';
import axe from 'axe-core';
import { CreateMCPOAuthSnapshot, ListMCPOAuthSnapshots, RestoreMCPOAuthSnapshot, DiscardMCPOAuthSnapshot } from '@wailsjs/go/wailsapi/MCP';
import type { credentials, mcp } from '../../../wailsjs/go/models';
import { McpOAuthSnapshots } from './McpOAuthSnapshots';

const { confirm, announce } = vi.hoisted(() => ({ confirm: vi.fn(), announce: vi.fn() }));
vi.mock('@wailsjs/go/wailsapi/MCP', () => ({ CreateMCPOAuthSnapshot: vi.fn(), ListMCPOAuthSnapshots: vi.fn(), RestoreMCPOAuthSnapshot: vi.fn(), DiscardMCPOAuthSnapshot: vi.fn() }));
vi.mock('../../hooks/useConfirm', () => ({ useConfirm: () => confirm }));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce }) }));

const snapshot = { id: 'snapshot', consumerId: 'legacy', name: 'Servidor', createdAt: '2026-10-02T00:00:00Z', retainUntil: '2026-11-01T00:00:00Z', location: '/private/snapshot.oauth', expired: false } as credentials.OAuthSnapshotInfo;
const consumers = [{ id: 'legacy', name: 'Legado', kind: 'legacy', issues: [] }, { id: 'managed', name: 'Composto', kind: 'managed', issues: [] }] as mcp.OAuthInventoryItem[];

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(ListMCPOAuthSnapshots).mockResolvedValue([snapshot]);
  vi.mocked(CreateMCPOAuthSnapshot).mockResolvedValue(snapshot);
  vi.mocked(RestoreMCPOAuthSnapshot).mockResolvedValue();
  vi.mocked(DiscardMCPOAuthSnapshot).mockResolvedValue();
  confirm.mockResolvedValue(true);
});

describe('McpOAuthSnapshots', () => {
  it('permite criar snapshot de Client Credentials e inclui credenciais compartilhadas', async () => {
    render(<McpOAuthSnapshots consumers={[...consumers, { id: 'cc', name: 'Aplicação', kind: 'client_credentials', issues: [] }, { id: 'host', name: 'Compartilhada', kind: 'hostname', issues: [] }] as mcp.OAuthInventoryItem[]} />);
    await screen.findByText('Servidor');
    expect(screen.getByRole('option', { name: 'Compartilhada' })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('mcp.snapshots.consumer'), { target: { value: 'cc' } });
    fireEvent.click(screen.getByRole('button', { name: 'mcp.snapshots.create' }));
    await waitFor(() => expect(CreateMCPOAuthSnapshot).toHaveBeenCalledWith('cc'));
    expect(RestoreMCPOAuthSnapshot).not.toHaveBeenCalled();
  });
  it.each([false, true])('confirma explicitamente a restauração dos tokens por hostname (%s)', async (accepted) => {
    confirm.mockResolvedValue(accepted);
    vi.mocked(ListMCPOAuthSnapshots).mockResolvedValue([{ ...snapshot, consumerId: 'credential:shared.example' } as credentials.OAuthSnapshotInfo]);
    render(<McpOAuthSnapshots consumers={consumers} />);
    fireEvent.click(await screen.findByRole('button', { name: 'mcp.snapshots.restoreNamed' }));
    await waitFor(() => expect(confirm).toHaveBeenCalledWith(expect.objectContaining({ message: 'mcp.snapshots.restoreHostnameConfirm' })));
    if (accepted) {
      await waitFor(() => expect(RestoreMCPOAuthSnapshot).toHaveBeenCalledWith('snapshot'));
      await waitFor(() => expect(announce).toHaveBeenCalledWith('mcp.snapshots.hostnameRestored'));
    } else expect(RestoreMCPOAuthSnapshot).not.toHaveBeenCalled();
  });
  it('cria snapshot pelo identificador do hostname e exclui fontes externas', async () => {
    render(<McpOAuthSnapshots consumers={[
      { id: 'credential:shared.example', name: 'Compartilhada', kind: 'hostname', issues: [] },
      { id: 'credential:command.example', name: 'Comando', kind: 'hostname', issues: ['external_source'] },
      { id: 'credential:shared.example/private', name: 'Caminho inelegível', kind: 'hostname', issues: ['snapshot_ineligible'] },
    ] as mcp.OAuthInventoryItem[]} />);
    await screen.findByText('Servidor');
    expect(screen.queryByRole('option', { name: 'Comando' })).not.toBeInTheDocument();
    expect(screen.queryByRole('option', { name: 'Caminho inelegível' })).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('mcp.snapshots.consumer'), { target: { value: 'credential:shared.example' } });
    fireEvent.click(screen.getByRole('button', { name: 'mcp.snapshots.create' }));
    await waitFor(() => expect(CreateMCPOAuthSnapshot).toHaveBeenCalledWith('credential:shared.example'));
  });
  it('preserva a lista atual após descarte e troca de idioma', async () => {
    render(<McpOAuthSnapshots consumers={consumers} />);
    fireEvent.click(await screen.findByRole('button', { name: 'mcp.snapshots.discardNamed' }));
    vi.mocked(ListMCPOAuthSnapshots).mockResolvedValue([]);
    await screen.findByText('mcp.snapshots.empty');
    await act(() => i18n.changeLanguage('es'));
    expect(screen.queryByText('Servidor')).not.toBeInTheDocument();
    await act(() => i18n.changeLanguage('en'));
  });
  it('apresenta ações acessíveis e cria somente para o consumidor legado selecionado', async () => {
    const view = render(<McpOAuthSnapshots consumers={consumers} />);
    await screen.findByText('Servidor');
    expect(screen.queryByRole('option', { name: 'Composto' })).not.toBeInTheDocument();
    expect((await axe.run(view.container)).violations).toEqual([]);
    fireEvent.change(screen.getByLabelText('mcp.snapshots.consumer'), { target: { value: 'legacy' } });
    fireEvent.click(screen.getByRole('button', { name: 'mcp.snapshots.create' }));
    await waitFor(() => expect(CreateMCPOAuthSnapshot).toHaveBeenCalledWith('legacy'));
    await waitFor(() => expect(announce).toHaveBeenCalledWith('mcp.snapshots.created'));
  });

  it.each([false, true])('restaura somente após a decisão explícita (%s)', async (accepted) => {
    confirm.mockResolvedValue(accepted);
    render(<McpOAuthSnapshots consumers={consumers} />);
    fireEvent.click(await screen.findByRole('button', { name: 'mcp.snapshots.restoreNamed' }));
    await waitFor(() => expect(confirm).toHaveBeenCalledWith(expect.objectContaining({ message: 'mcp.snapshots.restoreConfirm' })));
    if (accepted) await waitFor(() => expect(RestoreMCPOAuthSnapshot).toHaveBeenCalledWith('snapshot'));
    else expect(RestoreMCPOAuthSnapshot).not.toHaveBeenCalled();
  });

  it('confirma o fim do rollback antes de descartar e impede recuperação expirada', async () => {
    vi.mocked(ListMCPOAuthSnapshots).mockResolvedValue([{ ...snapshot, expired: true } as credentials.OAuthSnapshotInfo]);
    render(<McpOAuthSnapshots consumers={consumers} />);
    expect(await screen.findByRole('button', { name: 'mcp.snapshots.restoreNamed' })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: 'mcp.snapshots.discardNamed' }));
    await waitFor(() => expect(DiscardMCPOAuthSnapshot).toHaveBeenCalledWith('snapshot', true));
    expect(confirm).toHaveBeenCalledWith(expect.objectContaining({ message: 'mcp.snapshots.discardConfirm' }));
  });

  it('não revela conteúdo de erros do cofre', async () => {
    vi.mocked(ListMCPOAuthSnapshots).mockRejectedValue(new Error('SECRET'));
    render(<McpOAuthSnapshots consumers={consumers} />);
    expect(await screen.findByText('mcp.snapshots.failed')).toBeInTheDocument();
    expect(screen.queryByText(/SECRET/)).not.toBeInTheDocument();
    expect(announce).toHaveBeenCalledWith('mcp.snapshots.failed', 'assertive');
  });

  it('não inicia restauração se o diagnóstico foi fechado durante a confirmação', async () => {
    let answer!: (value: boolean) => void;
    confirm.mockReturnValue(new Promise<boolean>((resolve) => { answer = resolve; }));
    const view = render(<McpOAuthSnapshots consumers={consumers} />);
    fireEvent.click(await screen.findByRole('button', { name: 'mcp.snapshots.restoreNamed' }));
    await waitFor(() => expect(confirm).toHaveBeenCalled());
    view.unmount();
    answer(true);
    await Promise.resolve();
    expect(RestoreMCPOAuthSnapshot).not.toHaveBeenCalled();
  });
});
