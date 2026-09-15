import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  createAuthenticatedCommandBridge,
} from './commandBridgeContext';
import {
  createCommandBridge,
  type CommandBridge,
  type CommandBridgeOwner,
  type CommandInvocation,
  type CommandResult,
  type CommandSession,
  type DialogCommandScope,
} from './commandBridge';
import {
  createTrustedCommandContextSession,
  type TrustedCommandContextSession,
} from './commandContextSession';
import {
  ensureModalCleanup,
  registerOpenModal,
  unregisterOpenModal,
} from './modalRegistry';

const stores = vi.hoisted(() => ({
  auth: {
    isAuthenticated: true,
    user: { userId: 'user-a', sessionId: 'session-a' } as {
      userId: string;
      sessionId: string;
    } | null,
  },
  workspace: { workspace: { id: 'workspace-a' } as { id: string } | null },
}));

vi.mock('../store/authStore', () => ({
  useAuthStore: { getState: () => stores.auth },
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: { getState: () => stores.workspace },
}));

const owner: CommandBridgeOwner = {
  userId: 'user-a',
  sessionId: 'session-a',
  workspaceId: 'workspace-a',
};
const session: CommandSession = { id: owner.sessionId, generation: '1', owner };
const scope: DialogCommandScope = {
  dialogId: 'decision-a',
  kind: 'decision',
  generation: '1',
  allowedCommandIds: ['decision.respond'],
  allowedTriggerSpecs: ['keyboard.local:Ctrl+Shift+R'],
};

let invocationNumber = 0;

function invocation(generation = '1'): CommandInvocation {
  invocationNumber += 1;
  return {
    sessionId: session.id,
    invocationId: `01900000-0000-7000-8000-${String(invocationNumber).padStart(12, '0')}`,
    commandId: 'command.a',
    generation,
    capabilityId: generation === '1' ? 'cap-a' : 'cap-next',
    ownership: 'local',
    source: 'ui.action',
  };
}

function surface(surfaceID: string, capturedAt = new Date().toISOString()) {
  return {
    surfaceType: 'editor',
    surfaceId: surfaceID,
    snapshotVersion: 'snapshot-1',
    capturedAt,
    staleAfterMs: 5_000,
  };
}

function resultFor(invocationValue: CommandInvocation, resultOwner: CommandBridgeOwner): CommandResult {
  return { ...invocationValue, owner: resultOwner, status: 'succeeded' };
}

function setup() {
  const dispatch = vi.fn(async (value: CommandInvocation) => ({
    invocationId: value.invocationId,
    accepted: true,
  }));
  const cancel = vi.fn(async () => undefined);
  const shutdown = vi.fn(async () => undefined);
  const bridge = createCommandBridge({
    port: { dispatch, cancel, shutdown },
    capabilities: [{ id: 'cap-a', commandId: 'command.a', generation: '1', owner }],
  });
  bridge.openSession(session);

  const context = createTrustedCommandContextSession();
  const cleanupSurface = context.registerSurfaceContext('editor-1', () => surface('editor-1'));
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  document.body.appendChild(overlay);
  registerOpenModal('decision-a', scope);

  return {
    composed: createAuthenticatedCommandBridge({ bridge, context, session, ownership: 'exclusive' }),
    bridge,
    context,
    dispatch,
    cancel,
    shutdown,
    cleanupSurface,
    overlay,
  };
}

beforeEach(() => {
  stores.auth.isAuthenticated = true;
  stores.auth.user = { userId: 'user-a', sessionId: 'session-a' };
  stores.workspace.workspace = { id: 'workspace-a' };
  document.body.replaceChildren();
  invocationNumber = 0;
});

afterEach(() => {
  unregisterOpenModal('decision-a');
  ensureModalCleanup();
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

describe('createAuthenticatedCommandBridge', () => {
  it('compõe modal topmost, foco e surface real em frame autenticado', () => {
    const { composed, cleanupSurface, overlay } = setup();
    const control = document.createElement('button');
    document.body.appendChild(control);
    control.focus();

    const frame = composed.readContext('editor-1');
    expect(frame?.owner).toEqual(owner);
    expect(frame?.frame.modal.topID).toBe('decision-a');
    expect(frame?.frame.modal.dialogCommandScope).toMatchObject(scope);
    expect(frame?.frame.focus.control?.capabilities.button).toBe(true);
    expect(frame?.frame.surface?.surfaceId).toBe('editor-1');

    cleanupSurface();
    overlay.remove();
  });

  it('recusa owner trocado, surface stale e geração antiga antes do dispatch', async () => {
    const { composed, dispatch, bridge, cleanupSurface, overlay } = setup();
    const current = invocation();
    await expect(composed.invoke(current, 'editor-1')).resolves.toMatchObject({ accepted: true });

    stores.auth.user = { userId: 'user-b', sessionId: 'session-b' };
    expect(composed.readContext('editor-1')).toBeUndefined();
    await expect(composed.invoke(invocation(), 'editor-1')).rejects.toMatchObject({
      code: 'session-unavailable',
    });

    stores.auth.user = { userId: 'user-a', sessionId: 'session-a' };
    const staleContext = createTrustedCommandContextSession();
    const staleCleanup = staleContext.registerSurfaceContext('old-surface', () =>
      surface('old-surface', new Date(Date.now() - 10_000).toISOString()),
    );
    const staleComposed = createAuthenticatedCommandBridge({
      bridge,
      context: staleContext,
      session,
      ownership: 'exclusive',
    });
    expect(staleComposed.readContext('old-surface')).toBeUndefined();
    await expect(staleComposed.invoke(invocation(), 'old-surface')).rejects.toMatchObject({
      code: 'session-unavailable',
    });

    await composed.lifecycle({ kind: 'generation', sessionId: session.id, generation: '2' });
    await expect(composed.invoke({ ...invocation('1'), invocationId: '01900000-0000-7000-8000-000000000099' }, 'editor-1')).rejects.toMatchObject({
      code: 'stale-generation',
    });
    expect(dispatch).toHaveBeenCalledTimes(1);

    staleCleanup();
    cleanupSurface();
    overlay.remove();
  });

  it('filtra resultados de outras sessões e resultados após logout do owner atual', async () => {
    const { composed, bridge, dispatch, cleanupSurface, overlay } = setup();
    const otherOwner: CommandBridgeOwner = {
      userId: 'user-b',
      sessionId: 'session-b',
      workspaceId: 'workspace-b',
    };
    bridge.replaceCapabilities([
      { id: 'cap-a', commandId: 'command.a', generation: '1', owner },
      { id: 'cap-b', commandId: 'command.a', generation: '1', owner: otherOwner },
    ]);
    bridge.openSession({ id: otherOwner.sessionId, generation: '1', owner: otherOwner });

    const seen: CommandResult[] = [];
    composed.subscribeResult((result) => seen.push(result));
    const first = invocation();
    const afterLogout = invocation();
    await composed.invoke(first, 'editor-1');
    await composed.invoke(afterLogout, 'editor-1');
    const other: CommandInvocation = {
      ...invocation(),
      sessionId: otherOwner.sessionId,
      capabilityId: 'cap-b',
    };
    await bridge.invoke(other, otherOwner);

    bridge.acceptResult(resultFor(first, owner));
    stores.auth.isAuthenticated = false;
    bridge.acceptResult(resultFor(afterLogout, owner));
    bridge.acceptResult(resultFor(other, otherOwner));

    expect(dispatch).toHaveBeenCalledTimes(3);
    expect(seen).toHaveLength(1);
    expect(seen[0].invocationId).toBe(first.invocationId);

    await composed.dispose();
    cleanupSurface();
    overlay.remove();
  });

  it('captura bridge e context na construção, sem seguir substituições da config', async () => {
    const first = setup();
    const config = {
      bridge: first.bridge,
      context: first.context,
      session,
      ownership: 'exclusive' as const,
    };
    const composed = createAuthenticatedCommandBridge(config);
    const replacement = setup();
    const mutableConfig = config as unknown as {
      bridge: CommandBridge;
      context: TrustedCommandContextSession;
    };
    mutableConfig.bridge = replacement.bridge;
    replacement.context.dispose();
    mutableConfig.context = replacement.context;

    await expect(composed.invoke(invocation(), 'editor-1')).resolves.toMatchObject({ accepted: true });
    expect(first.dispatch).toHaveBeenCalledTimes(1);
    expect(replacement.dispatch).not.toHaveBeenCalled();
    await composed.dispose();
    expect(first.shutdown).toHaveBeenCalledTimes(1);
    expect(replacement.shutdown).not.toHaveBeenCalled();
  });

  it('não regride a geração local quando lifecycles concorrentes completam fora de ordem', async () => {
    const { context, cleanupSurface, overlay } = setup();
    const releases = new Map<string, () => void>();
    const fakeBridge = {
      invoke: vi.fn(async (value: CommandInvocation) => ({
        invocationId: value.invocationId,
        accepted: true,
      })),
      cancel: vi.fn(async () => ({ invocationId: 'cancelled', accepted: true })),
      lifecycle: vi.fn((event: { generation?: string }) =>
        new Promise<void>((resolve) => {
          if (event.generation) releases.set(event.generation, resolve);
          else resolve();
        })),
      shutdown: vi.fn(async () => undefined),
      subscribeResult: vi.fn(() => () => undefined),
    } as unknown as CommandBridge;
    const composed = createAuthenticatedCommandBridge({
      bridge: fakeBridge,
      context,
      session,
      ownership: 'exclusive',
    });

    const generationTwo = composed.lifecycle({ kind: 'generation', sessionId: session.id, generation: '2' });
    const generationThree = composed.lifecycle({ kind: 'generation', sessionId: session.id, generation: '3' });
    releases.get('3')?.();
    await generationThree;
    releases.get('2')?.();
    await generationTwo;

    await expect(composed.invoke({ ...invocation('2'), invocationId: '01900000-0000-7000-8000-000000000099' }, 'editor-1')).rejects.toMatchObject({
      code: 'stale-generation',
    });
    await expect(composed.invoke({ ...invocation('3'), invocationId: '01900000-0000-7000-8000-000000000100' }, 'editor-1')).resolves.toMatchObject({ accepted: true });
    expect(fakeBridge.invoke).toHaveBeenCalledTimes(1);

    await composed.dispose();
    cleanupSurface();
    overlay.remove();
  });

  it('dispose é idempotente, delega cancelamento e recusa leituras posteriores', async () => {
    const { composed, context, cancel, shutdown, dispatch, cleanupSurface, overlay } = setup();
    await composed.invoke(invocation(), 'editor-1');
    await expect(composed.dispose()).resolves.toBeUndefined();
    await expect(composed.dispose()).resolves.toBeUndefined();

    expect(dispatch).toHaveBeenCalledTimes(1);
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(shutdown).toHaveBeenCalledTimes(1);
    expect(composed.readContext('editor-1')).toBeUndefined();
    expect(context.readOwnedCommandContextFrame('editor-1')).toBeUndefined();
    await expect(composed.invoke(invocation(), 'editor-1')).rejects.toMatchObject({
      code: 'bridge-closed',
    });
    expect(() => composed.subscribeResult(() => undefined)).toThrowError('bridge-closed');

    cleanupSurface();
    overlay.remove();
  });
});
