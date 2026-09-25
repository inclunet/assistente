import { describe, expect, it, vi } from 'vitest';
import { createContextualDeckLease, selectContextualDeckCommand, isContextualDeckCommand } from './commandContextualDeck';
import { WORKSPACE_MUTATION_COMMAND_IDS } from './commandContextualBackendExecution';
import { CHAT_MESSAGING_COMMAND_IDS } from './commandChatMessaging';
import { EDITOR_FILE_COMMAND_IDS } from './commandEditorFile';
import { EDITOR_FORMAT_COMMANDS } from './commandEditorFormatting';
import { EDITOR_SLIDE_COMMAND_IDS } from './commandEditorSlideTemplates';
import { CHAT_CLEAR_COMMAND } from './commandChatClear';
import { CONTEXTUAL_PAGE_PALETTE_COMMAND_IDS } from './commandContextualPalette';
import { createPageMutationWailsPort } from './commandPageMutationWails';

function fixture(commandId: string, surfaceType = commandId.startsWith('profiles.') ? 'profiles' : 'tasklists') {
  const reservation = { commandId, ticket: 'ticket', invocationId: 'invocation' };
  const app = { BeginContextualDeckPageUICommand: vi.fn(async () => reservation), BeginContextualDeckUICommand: vi.fn(),
    BeginContextualPagePaletteUICommand: vi.fn(), BeginUICommand: vi.fn(), CancelUICommand: vi.fn(async () => {}),
    TakeUICommand: vi.fn(), CompleteUICommand: vi.fn(), GetUICommandResult: vi.fn(), CommitWorkspaceTabCommand: vi.fn() };
  const target = { go: { app: { App: app } } } as unknown as Window;
  const current = vi.fn(() => true);
  const lease = createContextualDeckLease({ offerId: 'physical-offer', generation: 'generation', commandId,
    observed: { surfaceId: 'private-page-instance', surfaceType, profile: 'focused' }, isCurrent: current, target });
  return { app, current, reservation, port: createPageMutationWailsPort({ contextualPalette: lease, target }) };
}

describe('contextual Deck page API closed contract', () => {
  it('has a closed inventory of 81 commands, excluding Mermaid open and page create/update', () => {
    const ids = new Set([...WORKSPACE_MUTATION_COMMAND_IDS, ...CONTEXTUAL_PAGE_PALETTE_COMMAND_IDS,
      ...CHAT_MESSAGING_COMMAND_IDS.filter(id => id !== 'chat.message.edit.open'), ...EDITOR_FILE_COMMAND_IDS,
      ...Object.keys(EDITOR_FORMAT_COMMANDS), ...EDITOR_SLIDE_COMMAND_IDS, CHAT_CLEAR_COMMAND, 'editor.mermaid.apply', 'editor.mermaid.remove',
      'terminal.command.interrupt', 'terminal.session.create', 'terminal.session.close', 'layer.activate', 'layer.toggle', 'layer.back']);
    expect(ids.size).toBe(81);
    for (const id of ids) expect(isContextualDeckCommand(id), id).toBe(true);
    for (const id of ['editor.mermaid.open', 'profiles.create', 'profiles.update', 'tasklists.create', 'tasklists.update']) {
      expect(isContextualDeckCommand(id), id).toBe(false);
    }
  });
  it.each(CONTEXTUAL_PAGE_PALETTE_COMMAND_IDS)('%s passes four scalars, no command/target/surfaceID/serial', async id => {
    const f = fixture(id);
    await expect(f.port.beginUICommand(id)).resolves.toEqual(f.reservation);
    expect(f.app.BeginContextualDeckPageUICommand).toHaveBeenCalledExactlyOnceWith('physical-offer', 'generation', id.startsWith('profiles.') ? 'profiles' : 'tasklists', 'focused');
    expect(f.app.BeginUICommand).not.toHaveBeenCalled(); expect(f.app.BeginContextualDeckUICommand).not.toHaveBeenCalled(); expect(f.app.BeginContextualPagePaletteUICommand).not.toHaveBeenCalled();
    await expect(f.port.beginUICommand(id)).rejects.toThrow('consumed');
  });
  it.each(['missing-api', 'drift', 'mismatch'])('rejects %s without workspace/palette fallback', async mode => {
    const f = fixture('tasklists.clear');
    if (mode === 'missing-api') Reflect.deleteProperty(f.app, 'BeginContextualDeckPageUICommand');
    if (mode === 'mismatch') f.app.BeginContextualDeckPageUICommand.mockResolvedValue({ ...f.reservation, commandId: 'profiles.delete' });
    const pending = f.port.beginUICommand('tasklists.clear');
    if (mode === 'drift') f.current.mockReturnValue(false);
    await expect(pending).rejects.toThrow();
    if (mode === 'mismatch') expect(f.app.CancelUICommand).toHaveBeenCalledExactlyOnceWith('ticket');
    expect(f.app.BeginContextualDeckUICommand).not.toHaveBeenCalled(); expect(f.app.BeginContextualPagePaletteUICommand).not.toHaveBeenCalled(); expect(f.app.BeginUICommand).not.toHaveBeenCalled();
  });
  it.each(['tasklists.duplicate', 'tasklists.clear'])('accepts %s from workspace tasklist via the page API', async id => {
    const f = fixture(id, 'tasklist');
    await f.port.beginUICommand(id);
    expect(f.app.BeginContextualDeckPageUICommand).toHaveBeenCalledExactlyOnceWith('physical-offer', 'generation', 'tasklist', 'focused');
  });
  it.each([['tasklists.delete', 'tasklist'], ['profiles.activate', 'chat'], ['tasklists.clear', 'profiles']])('rejects wrong surface for %s / %s', async (id, type) => {
    const f = fixture(id, type);
    await expect(f.port.beginUICommand(id)).rejects.toThrow('stale');
    expect(f.app.BeginContextualDeckPageUICommand).not.toHaveBeenCalled();
  });
  it.each(['root', 'profile'])('rejects forbidden surfaceID conditions at %s even in a mixed local payload', level => {
    const condition = { commandId: 'tasklists.clear', bySurface: { tasklists: true }, fallback: false };
    const indexed = { ...condition, bySurfaceId: { tasklists: { 'page-a': true } } };
    const entry = level === 'root' ? indexed : { ...condition, byProfile: { focused: indexed } };
    expect(selectContextualDeckCommand([entry, { commandId: 'navigation.settings.open', bySurface: {}, fallback: false }], { surfaceId: 'page-a', surfaceType: 'tasklists', profile: 'focused' })).toBeNull();
  });
  it('rejects ambiguity and unknown profile without a root fallback escape', () => {
    const entry = { commandId: 'profiles.delete', bySurface: { profiles: true }, fallback: true,
      byProfile: { focused: { commandId: 'profiles.delete', bySurface: { profiles: true }, fallback: false } } };
    expect(selectContextualDeckCommand([entry], { surfaceId: 'page', surfaceType: 'profiles' })).toBeNull();
    expect(selectContextualDeckCommand([entry, { commandId: 'navigation.settings.open', bySurface: { profiles: true }, fallback: false }], { surfaceId: 'page', surfaceType: 'profiles', profile: 'focused' })).toBeNull();
  });
});
