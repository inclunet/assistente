import { describe, expect, it, vi } from 'vitest';
import type {
  ExternalUICommandReadyEvent,
  ExternalUIConnectionStatus,
  ExternalUIDestination,
  ExternalUIOwnerProof,
  ExternalUICommandOutcome,
  TakeExternalUICommandResult,
} from '../services/externalUIConnection';
import { createExternalUICommandDispatcher, type ExternalUICommandFrame } from './externalUICommandDispatcher';

const owner: ExternalUIOwnerProof = { userId: 'user-1', sessionId: 'session-1', workspaceId: 'workspace-1' };
const destination: ExternalUIDestination = {
  workspaceId: owner.workspaceId,
  tabId: 'tab-1',
  surface: { surfaceId: 'tab-1', surfaceType: 'chat', snapshotVersion: 'surface-v1' },
};
const connected: ExternalUIConnectionStatus = {
  state: 'connected', owner, target: destination, connectionId: 'connection-1', generation: '4',
  targetSnapshotId: 'snapshot-1', contextVersion: 'context-1', expiresAt: new Date(Date.now() + 30_000).toISOString(),
};
const ready: ExternalUICommandReadyEvent = {
  connectionId: connected.connectionId!, generation: connected.generation!, invocationId: 'invocation-1',
  targetSnapshotId: connected.targetSnapshotId!, contextVersion: connected.contextVersion!, commandId: 'navigation.settings.open',
};
const frame: ExternalUICommandFrame = { owner, destination, localGeneration: 'keyboard-map-2', hasFocus: true };
const takeResult: TakeExternalUICommandResult = {
  invocationId: ready.invocationId, commandId: ready.commandId, arguments: {}, receiptId: 'receipt-1',
  targetSnapshotId: ready.targetSnapshotId, contextVersion: ready.contextVersion, target: destination,
};

function setup(options: {
  status?: ExternalUIConnectionStatus;
  currentFrame?: ExternalUICommandFrame | null;
  isSupported?: (id: string) => boolean;
  execute?: (id: string) => boolean;
  arguments_?: Readonly<Record<string, unknown>>;
  take?: () => Promise<TakeExternalUICommandResult>;
} = {}) {
  let listener: ((event: ExternalUICommandReadyEvent) => void) | undefined;
  let currentFrame = options.currentFrame === undefined ? frame : options.currentFrame;
  const order: string[] = [];
  const service = {
    getSnapshot: () => options.status ?? connected,
    subscribeReady: (next: (event: ExternalUICommandReadyEvent) => void) => {
      listener = next;
      return () => { listener = undefined; };
    },
    take: vi.fn(options.take ?? (async () => ({ ...takeResult, arguments: options.arguments_ ?? {} }))),
    complete: vi.fn(async (_event: ExternalUICommandReadyEvent, _take: TakeExternalUICommandResult, outcome: ExternalUICommandOutcome) => {
      order.push(`complete:${outcome}`);
      return true;
    }),
  };
  const publishCurrentContext = vi.fn(async () => { order.push('publish'); });
  const execute = vi.fn((id: string) => {
    order.push(`execute:${id}`);
    return options.execute?.(id) ?? true;
  });
  const dispatcher = createExternalUICommandDispatcher({
    service,
    readCurrentFrame: () => currentFrame,
    isSupported: options.isSupported ?? (() => true),
    execute,
    publishCurrentContext,
  });
  return { dispatcher, emit: (event = ready) => listener?.(event), service, execute, publishCurrentContext, order,
    setFrame: (next: ExternalUICommandFrame | null) => { currentFrame = next; } };
}

describe('createExternalUICommandDispatcher', () => {
  it('recusa destino/contexto stale antes de Take e pede publicação CAS atual', async () => {
    const staleFrame = { ...frame, destination: { ...destination, surface: { ...destination.surface, snapshotVersion: 'surface-v2' } } };
    const f = setup({ currentFrame: staleFrame });
    f.emit();
    await vi.waitFor(() => expect(f.publishCurrentContext).toHaveBeenCalledOnce());
    expect(f.service.take).not.toHaveBeenCalled();
    expect(f.execute).not.toHaveBeenCalled();
    expect(f.service.complete).not.toHaveBeenCalled();
    f.dispatcher.dispose();
  });

  it('recusa conexão desconectada sem Take ou efeito', async () => {
    const disconnected: ExternalUIConnectionStatus = { state: 'disconnected', owner, target: null };
    const f = setup({ status: disconnected });
    f.emit();
    await vi.waitFor(() => expect(f.publishCurrentContext).toHaveBeenCalledOnce());
    expect(f.service.take).not.toHaveBeenCalled();
    expect(f.execute).not.toHaveBeenCalled();
    f.dispatcher.dispose();
  });

  it('consome ready duplicado uma vez e completa antes de publicar navegação síncrona', async () => {
    const f = setup();
    f.emit();
    f.emit();
    await vi.waitFor(() => expect(f.service.complete).toHaveBeenCalledOnce());
    expect(f.service.take).toHaveBeenCalledOnce();
    expect(f.execute).toHaveBeenCalledTimes(1);
    expect(f.execute).toHaveBeenNthCalledWith(1, 'navigation.settings.open', ready);
    expect(f.order).toEqual(['execute:navigation.settings.open', 'complete:succeeded', 'publish']);
    f.dispatcher.dispose();
  });

  it('marca comando não suportado como failed antes do efeito', async () => {
    const f = setup({ isSupported: () => false });
    f.emit();
    await vi.waitFor(() => expect(f.service.complete).toHaveBeenCalledOnce());
    expect(f.service.complete).toHaveBeenCalledWith(ready, takeResult, 'failed');
    expect(f.execute).not.toHaveBeenCalled();
    expect(f.order).toEqual(['complete:failed', 'publish']);
    f.dispatcher.dispose();
  });

  it('recusa argumentos não vazios antes de qualquer efeito', async () => {
    const f = setup({ arguments_: { path: '/unexpected' } });
    f.emit();
    await vi.waitFor(() => expect(f.service.complete).toHaveBeenCalledTimes(1));
    expect(f.service.complete).toHaveBeenCalledWith(ready, { ...takeResult, arguments: { path: '/unexpected' } }, 'failed');
    expect(f.execute).not.toHaveBeenCalled();
    f.dispatcher.dispose();
  });

  it('cancela após Take se a geração local mudou antes do efeito', async () => {
    let releaseTake!: (result: TakeExternalUICommandResult) => void;
    const f = setup({ take: () => new Promise(resolve => { releaseTake = resolve; }) });
    f.emit();
    await vi.waitFor(() => expect(f.service.take).toHaveBeenCalledTimes(1));
    f.setFrame({ ...frame, localGeneration: 'keyboard-map-3' });
    releaseTake(takeResult);
    await vi.waitFor(() => expect(f.service.complete).toHaveBeenCalledTimes(1));
    expect(f.service.complete).toHaveBeenCalledWith(ready, takeResult, 'cancelled');
    expect(f.execute).not.toHaveBeenCalled();
    f.dispatcher.dispose();
  });

  it('cancela após Take se o dispatcher for desmontado', async () => {
    let releaseTake!: (result: TakeExternalUICommandResult) => void;
    const f = setup({ take: () => new Promise(resolve => { releaseTake = resolve; }) });
    f.emit();
    await vi.waitFor(() => expect(f.service.take).toHaveBeenCalledTimes(1));
    f.dispatcher.dispose();
    releaseTake(takeResult);
    await vi.waitFor(() => expect(f.service.complete).toHaveBeenCalledTimes(1));
    expect(f.service.complete).toHaveBeenCalledWith(ready, takeResult, 'cancelled');
    expect(f.execute).not.toHaveBeenCalled();
  });
});
