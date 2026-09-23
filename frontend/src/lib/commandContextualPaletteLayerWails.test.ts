import { describe, expect, it, vi } from 'vitest';
import { createContextualPaletteLayerWailsPort } from './commandContextualPaletteLayerWails';
import type { ContextualPaletteCommandLease } from './commandWorkspaceTabWails';

function fixture() {
  let current = true;
  const lease: ContextualPaletteCommandLease = {
    generation: 'generation-a', commandId: 'layer.activate',
    observed: { surfaceType: 'chat', surfaceId: 'tab-a', profile: 'focused' },
    isCurrent: () => current,
  };
  const execute = vi.fn(async () => ({ invocationId: 'invocation', status: 'succeeded' }));
  const old = vi.fn();
  const app = { ExecuteContextualPaletteLayerCommand: execute, ExecutePaletteCommand: old };
  const target = { go: { app: { App: app } } } as unknown as Window;
  const authorize = vi.fn(() => true);
  return { lease, execute, old, app, target, authorize, invalidate: () => { current = false; } };
}

describe('contextual palette layer Wails submission', () => {
  it.each(['layer.activate', 'layer.toggle', 'layer.back'])('%s sends captured scalars once and returns status without output', async commandId => {
    const f = fixture(); const lease = { ...f.lease, commandId };
    const port = createContextualPaletteLayerWailsPort(lease, f.authorize, f.target);
    const result = await port.executeCommand(commandId, {});
    expect(result).toEqual({ invocationId: 'invocation', status: 'succeeded' });
    expect(f.execute).toHaveBeenCalledExactlyOnceWith('generation-a', commandId, lease.observed);
    expect(f.old).not.toHaveBeenCalled();
  });
  it.each(['lease', 'authorization', 'api-getter'])('rejects %s drift after asynchronous bridge readiness', async drift => {
    const f = fixture();
    if (drift === 'api-getter') Object.defineProperty(f.app, 'ExecuteContextualPaletteLayerCommand', {
      get: () => { f.invalidate(); return f.execute; },
    });
    const pending = createContextualPaletteLayerWailsPort(f.lease, f.authorize, f.target).executeCommand(f.lease.commandId, {});
    if (drift === 'lease') f.invalidate();
    if (drift === 'authorization') f.authorize.mockReturnValue(false);
    await expect(pending).rejects.toThrow('contextual-layer-stale');
    expect(f.execute).not.toHaveBeenCalled(); expect(f.old).not.toHaveBeenCalled();
  });
  it('rejects missing new API without legacy fallback', async () => {
    const f = fixture(); Reflect.deleteProperty(f.app, 'ExecuteContextualPaletteLayerCommand');
    await expect(createContextualPaletteLayerWailsPort(f.lease, f.authorize, f.target).executeCommand(f.lease.commandId, {})).rejects.toThrow('unavailable');
    expect(f.old).not.toHaveBeenCalled();
  });
  it('does not convert invalidation after synchronous submission into failure or retry', async () => {
    const f = fixture();
    f.execute.mockImplementation(async () => { f.invalidate(); return { invocationId: 'invocation', status: 'succeeded' }; });
    await expect(createContextualPaletteLayerWailsPort(f.lease, f.authorize, f.target).executeCommand(f.lease.commandId, {})).resolves.toMatchObject({ status: 'succeeded' });
    expect(f.execute).toHaveBeenCalledOnce(); expect(f.old).not.toHaveBeenCalled();
  });
  it.each(['profiles', 'tasklists', 'toolbar'])('rejects non-workspace surface %s', async surfaceType => {
    const f = fixture();
    const lease = { ...f.lease, observed: { ...f.lease.observed, surfaceType } };
    await expect(createContextualPaletteLayerWailsPort(lease, f.authorize, f.target).executeCommand(lease.commandId, {})).rejects.toThrow('stale');
    expect(f.execute).not.toHaveBeenCalled();
  });
});
