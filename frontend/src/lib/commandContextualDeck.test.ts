import { afterEach, describe, expect, it, vi } from 'vitest';
import { createContextualDeckLease, isContextualDeckCommand, selectContextualDeckCommand } from './commandContextualDeck';
import { createCommandWorkspaceTabWailsPort } from './commandWorkspaceTabWails';

function fixture() {
  const commandId = 'workspace.tab.chat.create';
  const reservation = { ticket: 'ticket', invocationId: 'invocation', commandId };
  const api = { BeginContextualDeckUICommand: vi.fn(async () => reservation), BeginUICommand: vi.fn(), BeginContextualPaletteUICommand: vi.fn(),
    CancelUICommand: vi.fn(async () => {}), TakeUICommand: vi.fn(), CompleteUICommand: vi.fn(), GetUICommandResult: vi.fn(), CommitWorkspaceTabCommand: vi.fn() };
  const target = { go: { app: { App: api } } } as unknown as Window;
  const isCurrent = vi.fn(() => true);
  const lease = createContextualDeckLease({ offerId: 'offer', generation: 'map', commandId,
    observed: { surfaceId: 'tab-a', surfaceType: 'chat', profile: 'focused' }, isCurrent, target });
  const port = createCommandWorkspaceTabWailsPort({ contextualPalette: lease, target });
  return { api, isCurrent, lease, port, commandId, reservation };
}
afterEach(() => vi.restoreAllMocks());
describe('physical Deck lease and closed projection selection', () => {
  it('sends only offer/generation/observation and consumes once even across ports', async () => {
    const f = fixture();
    await expect(f.port.beginUICommand(f.commandId)).resolves.toEqual(f.reservation);
    await expect(f.port.beginUICommand(f.commandId)).rejects.toThrow('consumed');
    expect(f.api.BeginContextualDeckUICommand).toHaveBeenCalledExactlyOnceWith('offer', 'map', { surfaceId: 'tab-a', surfaceType: 'chat', profile: 'focused' });
    expect(f.api.BeginContextualPaletteUICommand).not.toHaveBeenCalled(); expect(f.api.BeginUICommand).not.toHaveBeenCalled();
  });
  it.each(['expired', 'bridge-drift', 'missing-api', 'wrong-command'])('rejects %s without fallback', async mode => {
    const now = Date.now(); const f = fixture();
    if (mode === 'expired') vi.spyOn(Date, 'now').mockReturnValue(now + 10001);
    if (mode === 'missing-api') Reflect.deleteProperty(f.api, 'BeginContextualDeckUICommand');
    const promise = f.port.beginUICommand(mode === 'wrong-command' ? 'workspace.tab.close' : f.commandId);
    if (mode === 'bridge-drift') f.isCurrent.mockReturnValue(false);
    await expect(promise).rejects.toThrow();
    if (mode !== 'missing-api') expect(f.api.BeginContextualDeckUICommand).not.toHaveBeenCalled();
    expect(f.api.BeginContextualPaletteUICommand).not.toHaveBeenCalled(); expect(f.api.BeginUICommand).not.toHaveBeenCalled();
  });
  it('does not impose the offer TTL again on a consumed reservation', async () => {
    const now = Date.now(); const f = fixture();
    await f.port.beginUICommand(f.commandId);
    vi.spyOn(Date, 'now').mockReturnValue(now + 15000);
    await f.port.commitBackendCommand('ticket', 'handoff');
    expect(f.api.CommitWorkspaceTabCommand).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff');
  });
  it('cancels a mismatched reserved command and cannot reuse the offer', async () => {
    const f = fixture(); f.api.BeginContextualDeckUICommand.mockResolvedValue({ ...f.reservation, commandId: 'workspace.tab.close' });
    await expect(f.port.beginUICommand(f.commandId)).rejects.toThrow('mismatch');
    expect(f.api.CancelUICommand).toHaveBeenCalledExactlyOnceWith('ticket');
    await expect(f.port.beginUICommand(f.commandId)).rejects.toThrow('consumed');
  });
  it.each(['layer.unknown', 'profiles.update', 'tasklists.create', 'editor.mermaid.open', 'editor.mermaid.reopen'])('does not expand durable allowlist to %s', id => {
    expect(isContextualDeckCommand(id)).toBe(false);
  });
  it.each(['layer.activate', 'layer.toggle', 'layer.back'])('allows selection but prohibits UI reservation for %s', commandId => {
    expect(isContextualDeckCommand(commandId)).toBe(true);
    expect(() => createContextualDeckLease({ offerId: 'offer', generation: 'map', commandId,
      observed: { surfaceType: 'chat', surfaceId: 'tab-a' }, isCurrent: () => true })).toThrow('requires-backend');
  });
  it('selects only one result and rejects malformed/ambiguous payloads entirely', () => {
    const context = { surfaceType: 'chat', surfaceId: 'tab-a', profile: 'focused' };
    const local = { commandId: 'navigation.settings.open', bySurface: { chat: true }, fallback: false };
    const durable = { commandId: 'workspace.tab.close', bySurface: { chat: false }, fallback: false };
    expect(selectContextualDeckCommand([local, durable], context)).toBe(local.commandId);
    expect(selectContextualDeckCommand([local, { ...durable, fallback: 'true' }], context)).toBeNull();
    expect(selectContextualDeckCommand([local, { ...durable, bySurface: { chat: true } }], context)).toBeNull();
  });
});
