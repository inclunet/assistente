import { describe, expect, it, vi } from 'vitest';

import { setActiveWorkspaceTabForWorkspace } from './workspaceNavigationWails';

describe('workspaceNavigationWails', () => {
  it('chama somente a API escopada e devolve o snapshot', async () => {
    const snapshot = { id: 'ws-1', snapshot_epoch: 'epoch', snapshot_sequence: '2' };
    const scoped = vi.fn().mockResolvedValue(snapshot);
    const target = { go: { wailsapi: { Workspace: { SetActiveWorkspaceTabForWorkspace: scoped } } } } as never;

    await expect(setActiveWorkspaceTabForWorkspace('ws-1', 'tab-2', { target })).resolves.toBe(snapshot);
    expect(scoped).toHaveBeenCalledWith('ws-1', 'tab-2');
  });

  it('falha fechado sem fallback para a API legada', async () => {
    const legacy = vi.fn();
    const target = { go: { wailsapi: { Workspace: { SetActiveWorkspaceTab: legacy } } } } as never;

    await expect(setActiveWorkspaceTabForWorkspace('ws-1', 'tab-2', { target }))
      .rejects.toThrow('Scoped workspace navigation Wails API is not available');
    expect(legacy).not.toHaveBeenCalled();
  });
});
