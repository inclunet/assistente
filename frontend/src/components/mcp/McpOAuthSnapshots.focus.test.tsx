import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { DiscardMCPOAuthSnapshot, ListMCPOAuthSnapshots } from '@wailsjs/go/wailsapi/MCP';
import type { credentials } from '../../../wailsjs/go/models';
import { Modal } from '../ui/Modal';
import { ConfirmHost } from '../ui/ConfirmHost';
import { McpOAuthSnapshots } from './McpOAuthSnapshots';

vi.mock('@wailsjs/go/wailsapi/MCP', () => ({ CreateMCPOAuthSnapshot: vi.fn(), ListMCPOAuthSnapshots: vi.fn(), RestoreMCPOAuthSnapshot: vi.fn(), DiscardMCPOAuthSnapshot: vi.fn() }));
beforeEach(() => vi.clearAllMocks());

describe('foco após descarte de snapshot', () => {
  it.each([1, 2])('restaura foco no seletor com %i snapshot(s) e decisão real', async (count) => {
    const items = Array.from({ length: count }, (_, index) => ({ id: String(index), consumerId: 'server', name: `Servidor ${index}`, createdAt: '2026-10-02', retainUntil: '2026-11-01', location: '/private', expired: false } as credentials.OAuthSnapshotInfo));
    vi.mocked(ListMCPOAuthSnapshots).mockResolvedValue(items);
    let finish!: () => void;
    vi.mocked(DiscardMCPOAuthSnapshot).mockReturnValue(new Promise<void>((resolve) => { finish = resolve; }));
    render(<><Modal isOpen onClose={vi.fn()} title="Diagnóstico"><McpOAuthSnapshots consumers={[]} /></Modal><ConfirmHost /></>);
    const trigger = (await screen.findAllByRole('button', { name: 'mcp.snapshots.discardNamed' }))[0];
    trigger.focus();
    fireEvent.click(trigger);
    const decision = await screen.findByRole('alertdialog');
    fireEvent.click(within(decision).getByRole('button', { name: 'mcp.snapshots.discard' }));
    await waitFor(() => expect(DiscardMCPOAuthSnapshot).toHaveBeenCalledWith('0', true));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
    expect(trigger).toBeDisabled();
    vi.mocked(ListMCPOAuthSnapshots).mockResolvedValue(items.slice(1));
    await act(async () => { finish(); });
    await waitFor(() => expect(screen.getByLabelText('mcp.snapshots.consumer')).toHaveFocus());
    expect(screen.queryByText('Servidor 0')).not.toBeInTheDocument();
  });
});
