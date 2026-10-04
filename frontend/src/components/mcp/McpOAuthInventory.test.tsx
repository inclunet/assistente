import { afterEach, describe, expect, it, vi } from 'vitest';
import { StrictMode, useState } from 'react';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import axe from 'axe-core';
import { McpOAuthInventory } from './McpOAuthInventory';
import { InspectMCPOAuthInventory, ListMCPOAuthSnapshots, ReconnectMCPOAuthSnapshot } from '@wailsjs/go/wailsapi/MCP';
import type { mcp, credentials } from '../../../wailsjs/go/models';
import { registerDefaultFocus, unregisterDefaultFocus } from '../../hooks/useDefaultFocus';

const { announce } = vi.hoisted(() => ({ announce: vi.fn() }));
vi.mock('@wailsjs/go/wailsapi/MCP', () => ({ InspectMCPOAuthInventory: vi.fn(), ListMCPOAuthSnapshots: vi.fn(async () => []), CreateMCPOAuthSnapshot: vi.fn(), RestoreMCPOAuthSnapshot: vi.fn(), DiscardMCPOAuthSnapshot: vi.fn(), ReconnectMCPOAuthSnapshot: vi.fn() }));
vi.mock('../../hooks/useConfirm', () => ({ useConfirm: () => vi.fn(async () => true) }));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce }) }));

afterEach(() => { cleanup(); vi.clearAllMocks(); vi.mocked(ListMCPOAuthSnapshots).mockResolvedValue([]); });

describe('McpOAuthInventory', () => {
  it('apresenta os diagnósticos com semântica de formulário e fecha por botão', async () => {
    vi.mocked(InspectMCPOAuthInventory).mockResolvedValue([
      { id: 'one', name: 'Servidor legado', kind: 'legacy', issues: ['conflicting_client', 'unreadable'] } as mcp.OAuthInventoryItem,
    ]);
    const close = vi.fn();
    render(<McpOAuthInventory isOpen onClose={close} />);
    expect(await screen.findByRole('heading', { name: 'Servidor legado' })).toBeInTheDocument();
    expect(screen.getByText('mcp.inventory.issues.conflicting_client')).toBeInTheDocument();
    expect(screen.getByText('mcp.inventory.issues.unreadable')).toBeInTheDocument();
    expect(screen.getByRole('application')).toBeInTheDocument();
    expect(screen.getByLabelText('mcp.snapshots.consumer')).toBeInTheDocument();
    expect(announce).toHaveBeenCalledWith('mcp.inventory.loaded');
    expect((await axe.run(screen.getByRole('dialog'))).violations).toEqual([]);
    fireEvent.click(screen.getByRole('button', { name: 'common.close' }));
    expect(close).toHaveBeenCalledOnce();
    expect(InspectMCPOAuthInventory).toHaveBeenCalledOnce();
  });

  it('notifica a ação principal após concluir migração e atualizar inventário', async () => {
    vi.mocked(InspectMCPOAuthInventory).mockResolvedValueOnce([
      { id: 'legacy', name: 'Legado', kind: 'legacy', issues: [] } as mcp.OAuthInventoryItem,
    ]).mockResolvedValue([{ id: 'legacy', name: 'Legado', kind: 'managed', issues: [] } as mcp.OAuthInventoryItem]);
    vi.mocked(ListMCPOAuthSnapshots).mockResolvedValue([{
      id: 'snapshot', consumerId: 'legacy', name: 'Legado', createdAt: '2026-10-03', retainUntil: '2026-11-03', expired: false, location: '/snapshot',
    } as credentials.OAuthSnapshotInfo]);
    vi.mocked(ReconnectMCPOAuthSnapshot).mockResolvedValue();
    const changed = vi.fn();
    render(<McpOAuthInventory isOpen onClose={vi.fn()} onInventoryChanged={changed} />);
    await screen.findByLabelText('mcp.connection.tokenAuthMethod');
    fireEvent.change(screen.getByLabelText('mcp.connection.tokenAuthMethod'), { target: { value: 'none' } });
    fireEvent.click(screen.getByRole('button', { name: 'mcp.snapshots.reconnectNamed' }));
    await waitFor(() => expect(changed).toHaveBeenCalledOnce());
    expect(InspectMCPOAuthInventory).toHaveBeenCalledTimes(2);
  });

  it('não mostra detalhes internos em falhas de leitura', async () => {
    vi.mocked(InspectMCPOAuthInventory).mockRejectedValue(new Error('SECRET-IN-ERROR'));
    render(<McpOAuthInventory isOpen onClose={vi.fn()} />);
    expect(await screen.findByText('mcp.inventory.failed')).toBeInTheDocument();
    expect(announce).toHaveBeenCalledWith('mcp.inventory.failed', 'assertive');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.queryByText(/SECRET-IN-ERROR/)).not.toBeInTheDocument();
  });

  it('ignora resposta de uma consulta depois de fechar o diálogo', async () => {
    let finish!: (items: mcp.OAuthInventoryItem[]) => void;
    vi.mocked(InspectMCPOAuthInventory).mockReturnValue(new Promise((resolve) => { finish = resolve; }));
    const view = render(<McpOAuthInventory isOpen onClose={vi.fn()} />);
    expect(screen.getByText('mcp.inventory.loading')).toBeInTheDocument();
    expect(announce).toHaveBeenCalledWith('mcp.inventory.loading');
    view.unmount();
    announce.mockClear();
    finish([]);
    await waitFor(() => expect(InspectMCPOAuthInventory).toHaveBeenCalledOnce());
    expect(announce).not.toHaveBeenCalled();
  });

  it('explica quando não existem entradas', async () => {
    vi.mocked(InspectMCPOAuthInventory).mockResolvedValue([]);
    render(<McpOAuthInventory isOpen onClose={vi.fn()} />);
    expect(await screen.findByText('mcp.inventory.empty')).toBeInTheDocument();
  });

  it('reutiliza a consulta e anuncia carregamento uma vez em StrictMode', async () => {
    vi.mocked(InspectMCPOAuthInventory).mockResolvedValue([]);
    render(<StrictMode><McpOAuthInventory isOpen onClose={vi.fn()} /></StrictMode>);
    await screen.findByText('mcp.inventory.empty');
    expect(InspectMCPOAuthInventory).toHaveBeenCalledOnce();
    expect(announce.mock.calls.filter(([message]) => message === 'mcp.inventory.loading')).toHaveLength(1);
    expect(announce.mock.calls.filter(([message]) => message === 'mcp.inventory.loaded')).toHaveLength(1);
  });

  it('restaura foco no fechamento e consulta novamente somente ao reabrir', async () => {
    vi.mocked(InspectMCPOAuthInventory).mockResolvedValue([]);
    function Page() {
      const [open, setOpen] = useState(false);
      return <><button onClick={() => setOpen(true)}>Abrir diagnóstico</button>
        <McpOAuthInventory isOpen={open} onClose={() => setOpen(false)} /></>;
    }
    render(<Page />);
    const trigger = screen.getByRole('button', { name: 'Abrir diagnóstico' });
    const restore = () => { trigger.focus(); return true; };
    registerDefaultFocus(restore);
    try {
      expect(InspectMCPOAuthInventory).not.toHaveBeenCalled();
      fireEvent.click(trigger);
      await screen.findByText('mcp.inventory.empty');
      const close = screen.getByRole('button', { name: 'common.close' });
      close.focus();
      fireEvent.click(close);
      await waitFor(() => expect(trigger).toHaveFocus());
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
      expect(InspectMCPOAuthInventory).toHaveBeenCalledOnce();
      fireEvent.click(trigger);
      await screen.findByText('mcp.inventory.empty');
      expect(InspectMCPOAuthInventory).toHaveBeenCalledTimes(2);
      fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' });
      await waitFor(() => expect(trigger).toHaveFocus());
    } finally { unregisterDefaultFocus(restore); }
  });
});
