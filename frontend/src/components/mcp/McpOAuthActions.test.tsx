import { StrictMode } from 'react';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { InspectMCPOAuthInventory } from '@wailsjs/go/wailsapi/MCP';
import type { mcp } from '../../../wailsjs/go/models';
import { McpOAuthActions } from './McpOAuthActions';
import { Toolbar } from '../ui/Toolbar';

const state = vi.hoisted(() => ({
  user: { userId: 'one', sessionId: 'session-one' } as { userId: string; sessionId: string } | null,
  servers: [] as unknown[],
}));
vi.mock('../../store/authStore', () => ({ useAuthStore: (selector: (s: typeof state) => unknown) => selector(state) }));
vi.mock('../../store/mcpStore', () => ({ useMCPStore: (selector: (s: typeof state) => unknown) => selector(state) }));
vi.mock('@wailsjs/go/wailsapi/MCP', () => ({ InspectMCPOAuthInventory: vi.fn() }));
vi.mock('./McpOAuthInventory', () => ({ McpOAuthInventory: ({ isOpen, onClose, onInventoryChanged }: { isOpen: boolean; onClose: () => void; onInventoryChanged: () => void }) => isOpen ? <div role="dialog" aria-label="inventory">
  <button onClick={onInventoryChanged}>Converted</button><button onClick={onClose}>Close</button>
</div> : null }));
const items = (kind: string) => [{ id: 'one', name: 'Server', kind, issues: [] }] as mcp.OAuthInventoryItem[];
const page = () => <Toolbar ariaLabel="MCP" right={<McpOAuthActions />} />;

beforeEach(() => {
  vi.clearAllMocks();
  state.user = { userId: 'one', sessionId: 'session-one' };
  state.servers = [];
  vi.mocked(InspectMCPOAuthInventory).mockResolvedValue([]);
});

describe('McpOAuthActions', () => {
  it.each(['legacy', 'client_credentials'])('mostra migração para %s e atualiza após converter', async (kind) => {
    vi.mocked(InspectMCPOAuthInventory).mockResolvedValueOnce(items(kind)).mockResolvedValue(items('managed'));
    render(page());
    await userEvent.click(await screen.findByRole('button', { name: 'mcp.inventory.migrate' }));
    fireEvent.click(screen.getByRole('button', { name: 'Converted' }));
    await waitFor(() => expect(InspectMCPOAuthInventory).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole('button', { name: 'mcp.inventory.migrate' })).not.toBeInTheDocument();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it.each(['managed', 'hostname', 'unassociated'])('não oferece migração para %s; diagnóstico continua acessível por teclado', async (kind) => {
    vi.mocked(InspectMCPOAuthInventory).mockResolvedValue(items(kind));
    render(page());
    await waitFor(() => expect(InspectMCPOAuthInventory).toHaveBeenCalledOnce());
    expect(screen.queryByRole('button', { name: 'mcp.inventory.migrate' })).not.toBeInTheDocument();
    const advanced = screen.getByRole('button', { name: 'mcp.inventory.advancedActions' });
    advanced.focus();
    await userEvent.keyboard('{Enter}');
    const diagnostic = await screen.findByRole('menuitem', { name: 'mcp.inventory.title' });
    diagnostic.focus();
    await userEvent.keyboard('{Enter}');
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('reconsulta quando a lista muda e ao fechar o diagnóstico, sem consultas duplicadas em StrictMode', async () => {
    const view = render(<StrictMode>{page()}</StrictMode>);
    await waitFor(() => expect(InspectMCPOAuthInventory).toHaveBeenCalledOnce());
    vi.mocked(InspectMCPOAuthInventory).mockResolvedValue(items('legacy'));
    state.servers = [{}];
    view.rerender(<StrictMode>{page()}</StrictMode>);
    await userEvent.click(await screen.findByRole('button', { name: 'mcp.inventory.migrate' }));
    vi.mocked(InspectMCPOAuthInventory).mockResolvedValue([]);
    fireEvent.click(screen.getByRole('button', { name: 'Close' }));
    await waitFor(() => expect(InspectMCPOAuthInventory).toHaveBeenCalledTimes(3));
    expect(screen.queryByRole('button', { name: 'mcp.inventory.migrate' })).not.toBeInTheDocument();
  });

  it('ignora resposta atrasada da sessão anterior e fecha o diálogo ao sair', async () => {
    let finish!: (value: mcp.OAuthInventoryItem[]) => void;
    vi.mocked(InspectMCPOAuthInventory).mockReturnValueOnce(new Promise((resolve) => { finish = resolve; }));
    const view = render(page());
    await waitFor(() => expect(InspectMCPOAuthInventory).toHaveBeenCalledOnce());
    state.user = { userId: 'two', sessionId: 'session-two' };
    view.rerender(page());
    await waitFor(() => expect(InspectMCPOAuthInventory).toHaveBeenCalledTimes(2));
    await act(async () => finish(items('legacy')));
    expect(screen.queryByRole('button', { name: 'mcp.inventory.migrate' })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'mcp.inventory.advancedActions' }));
    await userEvent.click(await screen.findByRole('menuitem', { name: 'mcp.inventory.title' }));
    state.user = null;
    view.rerender(page());
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('invalida pendência anterior quando a reconsulta falha', async () => {
    vi.mocked(InspectMCPOAuthInventory).mockResolvedValueOnce(items('legacy')).mockRejectedValue(new Error('PRIVATE'));
    const view = render(page());
    await screen.findByRole('button', { name: 'mcp.inventory.migrate' });
    state.servers = [{}];
    view.rerender(page());
    await waitFor(() => expect(screen.queryByRole('button', { name: 'mcp.inventory.migrate' })).not.toBeInTheDocument());
    expect(InspectMCPOAuthInventory).toHaveBeenCalledTimes(2);
    expect(screen.getByRole('button', { name: 'mcp.inventory.advancedActions' })).toBeInTheDocument();
    expect(screen.queryByText(/PRIVATE/)).not.toBeInTheDocument();
  });

  it('mantém acesso avançado se a consulta falhar sem expor detalhes', async () => {
    vi.mocked(InspectMCPOAuthInventory).mockRejectedValue(new Error('PRIVATE'));
    render(page());
    await waitFor(() => expect(InspectMCPOAuthInventory).toHaveBeenCalledOnce());
    expect(screen.getByRole('button', { name: 'mcp.inventory.advancedActions' })).toBeInTheDocument();
    expect(screen.queryByText(/PRIVATE/)).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'mcp.inventory.migrate' })).not.toBeInTheDocument();
  });
});
