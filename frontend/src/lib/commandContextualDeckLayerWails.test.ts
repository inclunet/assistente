import { afterEach, describe, expect, it, vi } from 'vitest';
import { createContextualDeckLayerWailsPort } from './commandContextualDeckLayerWails';
import type { LocalCommandKeyboardContext } from './commandLocalKeyboardWails';

function fixture(commandId = 'layer.activate') {
  const execute = vi.fn(async () => ({ invocationId: 'invocation', status: 'succeeded' }));
  const app = { ExecuteContextualDeckLayerCommand: execute, ExecutePaletteCommand: vi.fn(), BeginContextualDeckUICommand: vi.fn() };
  const target = { go: { app: { App: app } } } as unknown as Window;
  const current = vi.fn(() => true);
  const observed: LocalCommandKeyboardContext = { surfaceType: 'chat', surfaceId: 'tab-a', appPage: 'workspace', profile: 'focused' };
  const port = createContextualDeckLayerWailsPort({ offerId: 'offer', generation: 'map', commandId, observed, isCurrent: current, target });
  return { port, execute, app, current, observed, commandId };
}
afterEach(() => vi.restoreAllMocks());
describe('Deck layer submission port', () => {
  it.each(['layer.activate', 'layer.toggle', 'layer.back'])('%s sends no command, arguments or physical identity and snapshots observation', async id => {
    const f = fixture(id); f.observed.surfaceId = 'forged-later'; f.observed.appPage = 'settings';
    await expect(f.port.executeCommand(id, { ignored: true })).resolves.toMatchObject({ status: 'succeeded' });
    expect(f.execute).toHaveBeenCalledExactlyOnceWith('offer', 'map', { surfaceType: 'chat', surfaceId: 'tab-a', appPage: 'workspace', profile: 'focused' });
    expect(f.app.ExecutePaletteCommand).not.toHaveBeenCalled(); expect(f.app.BeginContextualDeckUICommand).not.toHaveBeenCalled();
    await expect(f.port.executeCommand(id, {})).rejects.toThrow('stale');
  });
  it.each(['lease', 'deadline', 'getter', 'missing-api'])('rejects %s after bridge wait with no fallback', async mode => {
    const now = Date.now();
    // Anchor creation and expiry to the same clock. Under full-suite load,
    // fixture creation can take longer than the 1ms expiry margin.
    const clock = mode === 'deadline' ? vi.spyOn(Date, 'now').mockReturnValue(now) : undefined;
    const f = fixture();
    if (mode === 'missing-api') Reflect.deleteProperty(f.app, 'ExecuteContextualDeckLayerCommand');
    if (mode === 'getter') Object.defineProperty(f.app, 'ExecuteContextualDeckLayerCommand', { get: () => { f.current.mockReturnValue(false); return f.execute; } });
    const result = f.port.executeCommand(f.commandId, {});
    if (mode === 'lease') f.current.mockReturnValue(false);
    clock?.mockReturnValue(now + 10001);
    await expect(result).rejects.toThrow();
    expect(f.execute).not.toHaveBeenCalled(); expect(f.app.ExecutePaletteCommand).not.toHaveBeenCalled();
    expect(f.app.BeginContextualDeckUICommand).not.toHaveBeenCalled();
  });
  it('keeps a succeeded result after the submission invalidates the source', async () => {
    const f = fixture(); f.execute.mockImplementation(async () => { f.current.mockReturnValue(false); return { invocationId: 'invocation', status: 'succeeded' }; });
    await expect(f.port.executeCommand(f.commandId, {})).resolves.toMatchObject({ status: 'succeeded' });
    expect(f.execute).toHaveBeenCalledOnce();
  });
  it('never retries an uncertain transport failure', async () => {
    const f = fixture(); f.execute.mockRejectedValue(new Error('transport lost'));
    await expect(f.port.executeCommand(f.commandId, {})).rejects.toThrow('transport lost');
    await expect(f.port.executeCommand(f.commandId, {})).rejects.toThrow('stale');
    expect(f.execute).toHaveBeenCalledOnce(); expect(f.app.ExecutePaletteCommand).not.toHaveBeenCalled();
  });
});
