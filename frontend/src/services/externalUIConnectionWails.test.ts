import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ExternalUIOwnerProof, ExternalUIDestination } from './externalUIConnection';
import { createWailsExternalUIConnectionService } from './externalUIConnectionWails';

const owner: ExternalUIOwnerProof = { userId: 'user-1', sessionId: 'session-1', workspaceId: 'workspace-1' };
const target: ExternalUIDestination = {
  workspaceId: owner.workspaceId,
  tabId: 'tab-1',
  surface: {
    surfaceType: 'editor', surfaceId: 'tab-1', snapshotVersion: 'editor:tab-1:2',
    selection: { kind: 'text', text: 'selected phrase', range: { startLine: 2, endLine: 2 } },
    focus: { kind: 'editor', cursor: { line: 2, column: 4 } },
    content: { kind: 'document', text: 'current document' },
    metadata: { language: 'typescript' },
  },
};
const ids = {
  connectionId: '0190f7b2-7c00-7000-8000-000000000001',
  targetSnapshotId: '0190f7b2-7c00-7000-8000-000000000002',
  contextVersion: '0190f7b2-7c00-7000-8000-000000000003',
  invocationId: '0190f7b2-7c00-7000-8000-000000000004',
  receiptId: '0190f7b2-7c00-7000-8000-000000000005',
  generation: '1',
  commandId: 'tool.execute.t_0190f7b27c0070008000000000000005',
};
const status = {
  state: 'connected', owner, target,
  connectionId: ids.connectionId,
  generation: ids.generation,
  targetSnapshotId: ids.targetSnapshotId,
  contextVersion: ids.contextVersion,
  expiresAt: '2099-01-01T00:00:00Z',
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

describe('external UI Wails JSON boundary', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('keeps RawMessage values as JSON objects through the generated wrapper roundtrip', async () => {
    const callbacks = new Map<string, (...args: unknown[]) => void>();
    const eventsOnMultiple = vi.fn((name: string, callback: (...args: unknown[]) => void) => {
      callbacks.set(name, callback);
      return () => callbacks.delete(name);
    });
    const api = {
      ReadExternalUIConnection: vi.fn().mockResolvedValue(status),
      BeginExternalUIConnection: vi.fn().mockResolvedValue({ invitation: 'invite-once', expiresAt: '2099-01-01T00:00:00Z' }),
      PublishExternalUIContext: vi.fn().mockResolvedValue(status),
      HeartbeatExternalUIConnection: vi.fn().mockResolvedValue(status),
      DisconnectExternalUIConnection: vi.fn().mockResolvedValue(undefined),
      TakeExternalUICommand: vi.fn().mockResolvedValue({
        invocationId: ids.invocationId,
        commandId: ids.commandId,
        arguments: { phrase: 'hello', options: { exact: true } },
        receiptId: ids.receiptId,
        targetSnapshotId: ids.targetSnapshotId,
        contextVersion: ids.contextVersion,
        target,
      }),
      CompleteExternalUICommand: vi.fn().mockResolvedValue({ accepted: true }),
    };
    vi.stubGlobal('go', { app: { App: api } });
    vi.stubGlobal('runtime', { EventsOnMultiple: eventsOnMultiple });

    const service = createWailsExternalUIConnectionService(() => owner);
    await service.refresh();
    await service.begin(target);

    const serializedBeginValue: unknown = JSON.parse(JSON.stringify(api.BeginExternalUIConnection.mock.calls[0][0]));
    if (!isRecord(serializedBeginValue)) throw new Error('begin-payload-not-an-object');
    const serializedBegin = serializedBeginValue;
    expect(serializedBegin).toEqual(target);
    const wireSurface = serializedBegin.surface;
    if (!isRecord(wireSurface)) throw new Error('surface-payload-not-an-object');
    expect(wireSurface).toMatchObject({
      selection: { kind: 'text', text: 'selected phrase' },
      focus: { kind: 'editor', cursor: { line: 2, column: 4 } },
      content: { kind: 'document', text: 'current document' },
      metadata: { language: 'typescript' },
    });
    for (const field of ['selection', 'focus', 'content', 'metadata']) {
      const jsonRawMessage = wireSurface[field];
      const expectedValue = target.surface[field as keyof typeof target.surface];
      if (!isRecord(expectedValue)) throw new Error(`expected-${field}-not-an-object`);
      expect(jsonRawMessage).toEqual(expectedValue);
      expect(JSON.parse(JSON.stringify(jsonRawMessage))).toMatchObject(expectedValue);
      expect(Array.isArray(jsonRawMessage)).toBe(false);
    }

    const ready = {
      connectionId: ids.connectionId,
      generation: ids.generation,
      invocationId: ids.invocationId,
      targetSnapshotId: ids.targetSnapshotId,
      contextVersion: ids.contextVersion,
      commandId: ids.commandId,
    };
    const handoff = await service.take(ready);
    expect(handoff.arguments).toEqual({ phrase: 'hello', options: { exact: true } });
    expect(handoff.target).toEqual(target);
    expect(await service.complete(ready, handoff, 'succeeded')).toBe(true);
    service.dispose();
  });
});
