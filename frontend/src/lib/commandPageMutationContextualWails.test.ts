import { describe, expect, it, vi } from 'vitest';
import { createPageMutationWailsPort } from './commandPageMutationWails';
import { CONTEXTUAL_PAGE_PALETTE_COMMAND_IDS } from './commandContextualPalette';
import type { AppPage } from './commandAppPage';

describe('page contextual palette Wails ingress', () => {
  function fixture(id = 'tasklists.duplicate', surfaceType = 'tasklists', available = true, appPage: AppPage | null = surfaceType === 'tasklist' ? 'workspace' : surfaceType === 'profiles' ? 'profiles' : 'tasklists', surfaceId = 'private-ui-instance') {
    let current = true;
    const page = vi.fn(async () => ({ ticket: 't', invocationId: 'i', commandId: id }));
    const generic = vi.fn(); const workspace = vi.fn(); const prepare = vi.fn(); const commit = vi.fn();
    const app = { BeginUICommand: generic, BeginContextualPaletteUICommand: workspace, ...(available ? { BeginContextualPagePaletteUICommand: page } : {}), PreparePageMutationCommand: prepare, CommitWorkspaceTabCommand: commit };
    const port = createPageMutationWailsPort({ target: { go: { app: { App: app } } } as unknown as Window,
      contextualPalette: { commandId: id, generation: 'g', observed: { surfaceType, surfaceId, ...(appPage !== null ? { appPage } : {}), profile: 'focused' }, isCurrent: () => current },
    });
    return { port, page, generic, workspace, prepare, commit, invalidate: () => { current = false; } };
  }
  it.each(CONTEXTUAL_PAGE_PALETTE_COMMAND_IDS)('%s serializes the complete observed page DTO', async id => {
    const surfaceType = id.startsWith('profiles.') ? 'profiles' : 'tasklists';
    const f = fixture(id, surfaceType); await f.port.beginUICommand(id);
    expect(f.page).toHaveBeenCalledExactlyOnceWith('g', id, { surfaceId: '', surfaceType, appPage: surfaceType, profile: 'focused' });
    expect(f.generic).not.toHaveBeenCalled(); expect(f.workspace).not.toHaveBeenCalled();
  });
  it('missing API never falls back to workspace or unconditional Begin', async () => {
    const f = fixture('tasklists.duplicate', 'tasklists', false);
    await expect(f.port.beginUICommand('tasklists.duplicate')).rejects.toThrow('unavailable');
    expect(f.generic).not.toHaveBeenCalled(); expect(f.workspace).not.toHaveBeenCalled();
  });
  it('rejects domain mismatch and deletion from a workspace tasklist', async () => {
    for (const [id, surface] of [['profiles.delete', 'tasklists'], ['tasklists.delete', 'tasklist'], ['tasklists.duplicate', 'toolbar']]) {
      const f = fixture(id, surface);
      await expect(f.port.beginUICommand(id)).rejects.toThrow('surface'); expect(f.page).not.toHaveBeenCalled();
      expect(f.generic).not.toHaveBeenCalled(); expect(f.workspace).not.toHaveBeenCalled();
    }
  });
  it.each([
    ['missing page fact', 'tasklists', null, ''],
    ['wrong page fact', 'tasklists', 'workspace', ''],
  ] as const)('rejects %s before calling the page Begin', async (_name, surfaceType, appPage, surfaceId) => {
    const f = fixture('tasklists.duplicate', surfaceType, true, appPage, surfaceId);
    await expect(f.port.beginUICommand('tasklists.duplicate')).rejects.toThrow('contextual-page-palette-page');
    expect(f.page).not.toHaveBeenCalled(); expect(f.generic).not.toHaveBeenCalled(); expect(f.workspace).not.toHaveBeenCalled();
  });
  it('allows the workspace tasklist alias only with app.page=workspace', async () => {
    const f = fixture('tasklists.duplicate', 'tasklist', true, 'workspace');
    await f.port.beginUICommand('tasklists.duplicate');
    expect(f.page).toHaveBeenCalledExactlyOnceWith('g', 'tasklists.duplicate', { surfaceId: '', surfaceType: 'tasklist', appPage: 'workspace', profile: 'focused' });
  });
  it('does not forward the caller surface ID to the page-only ingress', async () => {
    const f = fixture('tasklists.duplicate', 'tasklists', true, 'tasklists', 'private-ui-instance');
    await f.port.beginUICommand('tasklists.duplicate');
    expect(f.page).toHaveBeenCalledExactlyOnceWith('g', 'tasklists.duplicate', {
      surfaceId: '', surfaceType: 'tasklists', appPage: 'tasklists', profile: 'focused',
    });
  });
  it('guards bridge wait, preparation and immediate Commit without fallback', async () => {
    const f = fixture(); const begin = f.port.beginUICommand('tasklists.duplicate'); f.invalidate();
    await expect(begin).rejects.toThrow('stale');
    await expect(f.port.preparePageMutationCommand('t', { targetId: 'a', expectedFingerprint: 'v1', title: '', description: '' })).rejects.toThrow('stale');
    expect(() => f.port.commitBackendCommand('t', 'h')).toThrow('stale');
    expect(f.page).not.toHaveBeenCalled(); expect(f.prepare).not.toHaveBeenCalled(); expect(f.commit).not.toHaveBeenCalled();
    expect(f.generic).not.toHaveBeenCalled(); expect(f.workspace).not.toHaveBeenCalled();
  });
});
