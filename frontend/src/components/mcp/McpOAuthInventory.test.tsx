import { afterEach, describe, expect, it, vi } from 'vitest';
import { useState } from 'react';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import axe from 'axe-core';
import { McpOAuthInventory } from './McpOAuthInventory';
import { InspectMCPOAuthInventory } from '@wailsjs/go/wailsapi/MCP';
import type { mcp } from '../../../wailsjs/go/models';
import { registerDefaultFocus, unregisterDefaultFocus } from '../../hooks/useDefaultFocus';

const { announce } = vi.hoisted(() => ({ announce: vi.fn() }));
vi.mock('@wailsjs/go/wailsapi/MCP', () => ({ InspectMCPOAuthInventory: vi.fn() }));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce }) }));

afterEach(() => { cleanup(); vi.clearAllMocks(); });

describe('McpOAuthInventory', () => {
  it('apresenta os diagnósticos com semântica de leitura e fecha por botão', async () => {
    vi.mocked(InspectMCPOAuthInventory).mockResolvedValue([
      { id: 'one', name: 'Servidor legado', kind: 'legacy', issues: ['conflicting_client', 'unreadable'] } as mcp.OAuthInventoryItem,
    ]);
    const close = vi.fn();
    render(<McpOAuthInventory isOpen onClose={close} />);
    expect(await screen.findByRole('heading', { name: 'Servidor legado' })).toBeInTheDocument();
    expect(screen.getByText('mcp.inventory.issues.conflicting_client')).toBeInTheDocument();
    expect(screen.getByText('mcp.inventory.issues.unreadable')).toBeInTheDocument();
    expect(screen.getByRole('document')).toBeInTheDocument();
    expect(announce).toHaveBeenCalledWith('mcp.inventory.loaded');
    expect((await axe.run(screen.getByRole('dialog'))).violations).toEqual([]);
    fireEvent.click(screen.getByRole('button', { name: 'common.close' }));
    expect(close).toHaveBeenCalledOnce();
    expect(InspectMCPOAuthInventory).toHaveBeenCalledOnce();
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
