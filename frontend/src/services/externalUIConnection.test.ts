import { describe, expect, it, vi } from 'vitest';
import {
  EXTERNAL_COMMAND_READY_EVENT,
  createExternalUIConnectionService,
  parseExternalUIConnectionStatus,
  type ExternalUIConnectionPort,
  type ExternalUIConnectionStatus,
  type ExternalUIEventsOn,
  type ExternalUIOwnerProof,
  type ExternalUIDestination,
} from './externalUIConnection';

const owner: ExternalUIOwnerProof = { userId: 'user-1', sessionId: 'session-1', workspaceId: 'workspace-1' };
const target: ExternalUIDestination = {
  workspaceId: owner.workspaceId,
  tabId: 'tab-1',
  surface: { surfaceType: 'editor', surfaceId: 'editor-1', snapshotVersion: 'editor:editor-1:1' },
};
const connected: ExternalUIConnectionStatus = {
  state: 'connected',
  connectionId: '0190f7b2-7c00-7000-8000-000000000001',
  generation: '1',
  owner,
  target,
  targetSnapshotId: '0190f7b2-7c00-7000-8000-000000000002',
  contextVersion: '0190f7b2-7c00-7000-8000-000000000003',
  expiresAt: '2026-09-24T12:00:00Z',
};
const ready = {
  connectionId: connected.connectionId!,
  generation: connected.generation!,
  invocationId: '0190f7b2-7c00-7000-8000-000000000004',
  targetSnapshotId: connected.targetSnapshotId!,
  contextVersion: connected.contextVersion!,
  commandId: 'tool.execute.t_0190f7b27c0070008000000000000005',
};

function eventBus() {
  const listeners = new Map<string, Set<(payload: unknown) => void>>();
  const on: ExternalUIEventsOn = (name, listener) => {
    const group = listeners.get(name) ?? new Set();
    group.add(listener);
    listeners.set(name, group);
    return () => group.delete(listener);
  };
  return {
    on,
    emit(name: string, payload: unknown) { for (const listener of listeners.get(name) ?? []) listener(payload); },
  };
}

function port(overrides: Partial<ExternalUIConnectionPort> = {}): ExternalUIConnectionPort {
  return {
    ReadExternalUIConnection: vi.fn(async () => connected),
    BeginExternalUIConnection: vi.fn(async () => ({ invitation: 'one-time-invitation', expiresAt: '2099-01-01T00:00:00Z' })),
    PublishExternalUIContext: vi.fn(async () => ({ ...connected, target })),
    HeartbeatExternalUIConnection: vi.fn(async () => connected),
    DisconnectExternalUIConnection: vi.fn(async () => undefined),
    TakeExternalUICommand: vi.fn(async () => ({
      invocationId: ready.invocationId,
      commandId: ready.commandId,
      arguments: { query: 'hello' },
      receiptId: 'receipt-1',
      targetSnapshotId: ready.targetSnapshotId,
      contextVersion: ready.contextVersion,
      target,
    })),
    CompleteExternalUICommand: vi.fn(async () => ({ accepted: true })),
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(done => { resolve = done; });
  return { promise, resolve };
}

describe('externalUIConnection contract', () => {
  it('parses the real Go status DTO with nested target and rejects the old top-level surface shape', () => {
    expect(parseExternalUIConnectionStatus(connected)).toMatchObject({ owner, target });
    const obsolete = { ...connected, surface: target.surface } as Record<string, unknown>;
    delete obsolete.target;
    expect(parseExternalUIConnectionStatus(obsolete)).toBeUndefined();
  });

  it('normalizes the Go zero destination when disconnected and preserves waiting_claim target', () => {
    const disconnected = parseExternalUIConnectionStatus({
      state: 'disconnected', owner, target: { workspaceId: '', surface: { surfaceType: '', surfaceId: '', snapshotVersion: '' } },
      expiresAt: '0001-01-01T00:00:00Z',
    });
    expect(disconnected?.target).toBeNull();
    const waiting = parseExternalUIConnectionStatus({
      state: 'waiting_claim', owner, target, expiresAt: '2099-01-01T00:00:00Z',
    });
    expect(waiting).toMatchObject({ state: 'waiting_claim', target });
  });

  it('uses destination, CAS publication and lease DTOs exactly', async () => {
    const api = port();
    const events = eventBus();
    const service = createExternalUIConnectionService(api, () => owner, events);

    await service.begin(target);
    expect(api.BeginExternalUIConnection).toHaveBeenCalledWith(target);
    await service.refresh();
    const nextTarget = { ...target, surface: { ...target.surface, snapshotVersion: 'editor:editor-1:2' } };
    await service.publishContext(nextTarget);
    expect(api.PublishExternalUIContext).toHaveBeenCalledWith({
      owner,
      connectionId: connected.connectionId,
      generation: connected.generation,
      expectedTargetSnapshotId: connected.targetSnapshotId,
      expectedContextVersion: connected.contextVersion,
      target: nextTarget,
    });
    await service.heartbeat();
    expect(api.HeartbeatExternalUIConnection).toHaveBeenCalledWith({
      owner, connectionId: connected.connectionId, generation: connected.generation,
    });
    await service.disconnect();
    expect(api.DisconnectExternalUIConnection).toHaveBeenCalledWith({
      owner, connectionId: connected.connectionId, generation: connected.generation,
    });
    expect(service.getSnapshot()).toEqual({ state: 'disconnected', owner, target: null });
    service.dispose();
  });

  it('reads the authoritative waiting_claim status immediately after creating an invitation', async () => {
    const waiting: ExternalUIConnectionStatus = {
      state: 'waiting_claim', owner, target, expiresAt: '2099-01-01T00:00:00Z',
    };
    const api = port({
      BeginExternalUIConnection: vi.fn(async () => ({ invitation: 'pending-invitation', expiresAt: '2099-01-01T00:00:00Z' })),
      ReadExternalUIConnection: vi.fn(async () => waiting),
    });
    const service = createExternalUIConnectionService(api, () => owner, eventBus());

    await expect(service.begin(target)).resolves.toMatchObject({ invitation: 'pending-invitation' });
    expect(api.ReadExternalUIConnection).toHaveBeenCalledOnce();
    expect(service.getSnapshot()).toMatchObject({ state: 'waiting_claim', owner, target });
    service.dispose();
  });

  it('preserves publication when a following heartbeat observes the authoritative context', async () => {
    const publishResponse = deferred<unknown>();
    const newerTarget: ExternalUIDestination = {
      ...target,
      surface: { ...target.surface, snapshotVersion: 'editor:editor-1:2' },
    };
    const published: ExternalUIConnectionStatus = {
      ...connected,
      target: newerTarget,
      targetSnapshotId: '0190f7b2-7c00-7000-8000-000000000006',
      contextVersion: '0190f7b2-7c00-7000-8000-000000000007',
    };
    let authoritative: ExternalUIConnectionStatus = connected;
    const api = port({
      HeartbeatExternalUIConnection: vi.fn(async () => authoritative),
      PublishExternalUIContext: vi.fn(async () => {
        const next = await publishResponse.promise;
        authoritative = next as ExternalUIConnectionStatus;
        return next;
      }),
    });
    const service = createExternalUIConnectionService(api, () => owner, eventBus());
    await service.refresh();

    const publish = service.publishContext(newerTarget);
    const heartbeat = service.heartbeat();
    publishResponse.resolve(published);
    await expect(publish).resolves.toMatchObject({ target: newerTarget, contextVersion: published.contextVersion });
    await expect(heartbeat).resolves.toMatchObject({ target: newerTarget, contextVersion: published.contextVersion });

    expect(service.getSnapshot()).toMatchObject({
      target: newerTarget,
      targetSnapshotId: published.targetSnapshotId,
      contextVersion: published.contextVersion,
    });
    service.dispose();
  });

  it('serializes heartbeat then publication so the old heartbeat cannot make publication report stale success', async () => {
    const heartbeatResponse = deferred<unknown>();
    const publishResponse = deferred<unknown>();
    let publishCalled = false;
    const api = port({
      HeartbeatExternalUIConnection: vi.fn(() => heartbeatResponse.promise),
      PublishExternalUIContext: vi.fn(() => { publishCalled = true; return publishResponse.promise; }),
    });
    const service = createExternalUIConnectionService(api, () => owner, eventBus());
    await service.refresh();
    const newerTarget = { ...target, surface: { ...target.surface, snapshotVersion: 'editor:editor-1:2' } };
    const heartbeat = service.heartbeat();
    const publish = service.publishContext(newerTarget);

    await vi.waitFor(() => expect(api.HeartbeatExternalUIConnection).toHaveBeenCalledOnce());
    expect(api.PublishExternalUIContext).not.toHaveBeenCalled();
    heartbeatResponse.resolve(connected);
    await heartbeat;
    await vi.waitFor(() => expect(publishCalled).toBe(true));
    expect(api.PublishExternalUIContext).toHaveBeenCalledWith(expect.objectContaining({
      expectedTargetSnapshotId: connected.targetSnapshotId,
      expectedContextVersion: connected.contextVersion,
      target: newerTarget,
    }));
    const published = {
      ...connected, target: newerTarget,
      targetSnapshotId: '0190f7b2-7c00-7000-8000-000000000006',
      contextVersion: '0190f7b2-7c00-7000-8000-000000000007',
    };
    publishResponse.resolve(published);
    await expect(publish).resolves.toMatchObject({ target: newerTarget, contextVersion: published.contextVersion });
    expect(service.getSnapshot()).toMatchObject({ target: newerTarget, contextVersion: published.contextVersion });
    service.dispose();
  });

  it('does not cancel Take when only the heartbeat expiry changes', async () => {
    const takeResponse = deferred<unknown>();
    const laterHeartbeat = { ...connected, expiresAt: '2099-01-01T00:01:00Z' };
    const api = port({
      TakeExternalUICommand: vi.fn(() => takeResponse.promise),
      HeartbeatExternalUIConnection: vi.fn(async () => laterHeartbeat),
    });
    const service = createExternalUIConnectionService(api, () => owner, eventBus());
    await service.refresh();
    const take = service.take(ready);
    await service.heartbeat();
    takeResponse.resolve({
      invocationId: ready.invocationId, commandId: ready.commandId, arguments: { query: 'hello' },
      receiptId: 'receipt-1', targetSnapshotId: ready.targetSnapshotId,
      contextVersion: ready.contextVersion, target,
    });

    await expect(take).resolves.toMatchObject({ invocationId: ready.invocationId });
    service.dispose();
  });

  it('treats status events as refresh hints so a delayed event cannot regress authoritative state', async () => {
    let authoritative = connected;
    const nextTarget = { ...target, surface: { ...target.surface, snapshotVersion: 'editor:editor-1:2' } };
    const published: ExternalUIConnectionStatus = {
      ...connected, target: nextTarget,
      targetSnapshotId: '0190f7b2-7c00-7000-8000-000000000006',
      contextVersion: '0190f7b2-7c00-7000-8000-000000000007',
    };
    const api = port({
      ReadExternalUIConnection: vi.fn(async () => authoritative),
      PublishExternalUIContext: vi.fn(async () => { authoritative = published; return published; }),
    });
    const events = eventBus();
    const service = createExternalUIConnectionService(api, () => owner, events);
    await service.refresh();
    await service.publishContext(nextTarget);
    events.emit('external:ui-connection:status', connected);

    await vi.waitFor(() => expect(api.ReadExternalUIConnection).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(service.getSnapshot()).toMatchObject({
      target: nextTarget, targetSnapshotId: published.targetSnapshotId, contextVersion: published.contextVersion,
    }));
    service.dispose();
  });

  it('accepts only exact ready payloads and forwards matching invocation stamps to Take/Complete', async () => {
    const api = port();
    const events = eventBus();
    const service = createExternalUIConnectionService(api, () => owner, events);
    await service.refresh();
    const onReady = vi.fn();
    service.subscribeReady(onReady);
    events.emit(EXTERNAL_COMMAND_READY_EVENT, { ...ready, arguments: { secret: 'must-not-arrive' } });
    expect(onReady).not.toHaveBeenCalled();
    events.emit(EXTERNAL_COMMAND_READY_EVENT, ready);
    expect(onReady).toHaveBeenCalledWith(ready);

    const receipt = await service.take(ready);
    expect(api.TakeExternalUICommand).toHaveBeenCalledWith({
      owner,
      connectionId: ready.connectionId,
      generation: ready.generation,
      invocationId: ready.invocationId,
      targetSnapshotId: ready.targetSnapshotId,
      contextVersion: ready.contextVersion,
    });
    expect(await service.complete(ready, receipt, 'succeeded')).toBe(true);
    expect(api.CompleteExternalUICommand).toHaveBeenCalledWith({
      owner,
      connectionId: ready.connectionId,
      generation: ready.generation,
      invocationId: ready.invocationId,
      receiptId: 'receipt-1',
      targetSnapshotId: ready.targetSnapshotId,
      contextVersion: ready.contextVersion,
      outcome: 'succeeded',
    });
    service.dispose();
  });
});
