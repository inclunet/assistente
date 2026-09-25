import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createAuthenticatedCommandBridge } from './commandBridgeContext';
import {
  createCommandBridge,
  type CommandBridge,
  type CommandBridgeOwner,
  type CommandInvocation,
  type CommandResult,
  type CommandSession,
  type CommandSource,
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
  updateOpenModalScope,
} from './modalRegistry';

const stores = vi.hoisted(() => ({
  listeners: new Set<() => void>(),
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
  useAuthStore: {
    getState: () => stores.auth,
    subscribe: (listener: () => void) => {
      stores.listeners.add(listener);
      return () => stores.listeners.delete(listener);
    },
  },
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: {
    getState: () => stores.workspace,
    subscribe: (listener: () => void) => {
      stores.listeners.add(listener);
      return () => stores.listeners.delete(listener);
    },
  },
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

function invocation(generation = '1', source: CommandSource = 'ui.action'): CommandInvocation {
  invocationNumber += 1;
  return {
    sessionId: session.id,
    invocationId: `01900000-0000-7000-8000-${String(invocationNumber).padStart(12, '0')}`,
    commandId: 'command.a',
    generation,
    capabilityId:
      generation === '1' ? (source === 'keyboard.local' ? 'cap-keyboard' : 'cap-a') : 'cap-next',
    ownership: 'local',
    source,
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

function resultFor(
  invocationValue: CommandInvocation,
  resultOwner: CommandBridgeOwner
): CommandResult {
  return { ...invocationValue, owner: resultOwner, status: 'succeeded' };
}

function setup(withDialog = false) {
  const dispatch = vi.fn(async (value: CommandInvocation) => ({
    invocationId: value.invocationId,
    accepted: true,
  }));
  const cancel = vi.fn(async () => undefined);
  const shutdown = vi.fn(async () => undefined);
  const bridge = createCommandBridge({
    port: { dispatch, cancel, shutdown },
    capabilities: [
      { id: 'cap-a', commandId: 'command.a', generation: '1', source: 'ui.action', owner },
      {
        id: 'cap-keyboard',
        commandId: 'command.a',
        generation: '1',
        source: 'keyboard.local',
        owner,
      },
    ],
  });
  bridge.openSession(session);

  const context = createTrustedCommandContextSession();
  const initialSurface = surface('editor-1');
  const cleanupSurface = context.registerSurfaceContext('editor-1', () => initialSurface);
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  document.body.appendChild(overlay);
  if (withDialog) registerOpenModal('decision-a', scope);

  return {
    composed: createAuthenticatedCommandBridge({
      bridge,
      context,
      session,
      ownership: 'exclusive',
    }),
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
  stores.listeners.clear();
  unregisterOpenModal('decision-a');
  ensureModalCleanup();
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

describe('createAuthenticatedCommandBridge', () => {
  it('reserva repetição do topo antes do resolver e mantém evento para o handler existente', async () => {
    const { composed, dispatch } = setup(true);
    const resolve = vi.fn(() => invocation('1', 'keyboard.local'));
    const event = new KeyboardEvent('keydown', {
      key: 'R',
      ctrlKey: true,
      shiftKey: true,
      cancelable: true,
    });
    await expect(composed.dispatchLocalKeyboard(event, resolve)).resolves.toEqual({
      kind: 'dialog-reserved',
      scope,
    });
    expect(resolve).not.toHaveBeenCalled();
    expect(dispatch).not.toHaveBeenCalled();
    expect(event.defaultPrevented).toBe(false);
    registerOpenModal('other');
    try {
      await expect(composed.dispatchLocalKeyboard(event, resolve)).resolves.toEqual({
        kind: 'blocked',
      });
      expect(resolve).not.toHaveBeenCalled();
    } finally {
      unregisterOpenModal('other');
    }
    await expect(composed.dispatchLocalKeyboard(event, resolve)).resolves.toEqual({
      kind: 'dialog-reserved',
      scope,
    });
    unregisterOpenModal('decision-a');
    await expect(composed.dispatchLocalKeyboard(event, resolve)).resolves.toMatchObject({
      kind: 'dispatched',
      ack: { accepted: true },
    });
    expect(resolve).toHaveBeenCalledTimes(1);
    expect(dispatch).toHaveBeenCalledTimes(1);
  });

  it.each([{ repeat: true }, { isComposing: true }, { keyCode: 229 }])(
    'ignora guarda %j antes de reservar ou resolver',
    async (guard) => {
      const { composed, dispatch } = setup(true);
      const resolve = vi.fn();
      const event = new KeyboardEvent('keydown', {
        key: 'r',
        ctrlKey: true,
        shiftKey: true,
        cancelable: true,
        ...guard,
      });
      await expect(composed.dispatchLocalKeyboard(event, resolve)).resolves.toEqual({
        kind: 'ignored',
      });
      expect(resolve).not.toHaveBeenCalled();
      expect(dispatch).not.toHaveBeenCalled();
      expect(event.defaultPrevented).toBe(false);
    }
  );

  it('reserva somente o scope atual quando a fila troca o diálogo no mesmo modal', async () => {
    const { composed, dispatch } = setup(true);
    const event = new KeyboardEvent('keydown', { key: 'r', ctrlKey: true, shiftKey: true });
    const resolve = vi.fn();
    const next = { ...scope, dialogId: 'decision-b', generation: '2' };
    updateOpenModalScope('decision-a', next);
    await expect(composed.dispatchLocalKeyboard(event, resolve)).resolves.toEqual({
      kind: 'dialog-reserved',
      scope: next,
    });
    updateOpenModalScope('decision-a');
    await expect(composed.dispatchLocalKeyboard(event, resolve)).resolves.toEqual({
      kind: 'blocked',
    });
    expect(resolve).not.toHaveBeenCalled();
    expect(dispatch).not.toHaveBeenCalled();
  });

  it.each(['input', 'textarea', 'contenteditable', 'monaco'])(
    'preserva digitação em %s',
    async (kind) => {
      const { composed, dispatch } = setup(true);
      const control = document.createElement(
        kind === 'input' || kind === 'textarea' ? kind : 'div'
      );
      control.tabIndex = 0;
      if (kind === 'contenteditable') control.setAttribute('contenteditable', 'true');
      if (kind === 'monaco') control.className = 'monaco-editor';
      document.body.appendChild(control);
      control.focus();
      const event = new KeyboardEvent('keydown', {
        key: 'r',
        ctrlKey: true,
        shiftKey: true,
        cancelable: true,
      });
      const resolve = vi.fn();
      await expect(composed.dispatchLocalKeyboard(event, resolve)).resolves.toEqual({
        kind: 'ignored',
      });
      expect(event.defaultPrevented).toBe(false);
      expect(resolve).not.toHaveBeenCalled();
      expect(dispatch).not.toHaveBeenCalled();
    }
  );

  it('bloqueia chamada direta e comando de resposta sem prova de diálogo mesmo com capability', async () => {
    const { composed, bridge, dispatch } = setup(true);
    const first = invocation();
    await expect(composed.invoke(first)).resolves.toEqual({
      invocationId: first.invocationId,
      accepted: false,
      reason: 'dialog-blocked',
    });
    bridge.replaceCapabilities([
      { id: 'cap-a', commandId: 'decision.respond', generation: '1', source: 'ui.action', owner },
    ]);
    await expect(
      composed.invoke({ ...invocation(), commandId: 'decision.respond' })
    ).resolves.toMatchObject({ accepted: false, reason: 'dialog-blocked' });
    expect(dispatch).not.toHaveBeenCalled();
  });

  it('permite somente decision.respond local com prova do diálogo topmost atual', async () => {
    const { composed, bridge, dispatch } = setup(true);
    bridge.replaceCapabilities([
      {
        id: 'cap-a',
        commandId: 'decision.respond',
        generation: '1',
        source: 'keyboard.local',
        owner,
      },
    ]);
    const accepted = {
      ...invocation(),
      commandId: 'decision.respond',
      source: 'keyboard.local' as const,
      dialogProof: {
        dialogId: scope.dialogId,
        kind: 'decision' as const,
        scopeGeneration: scope.generation,
        commandId: 'decision.respond' as const,
        triggerSpec: 'keyboard.local:Ctrl+Shift+R' as const,
      },
    };

    await expect(composed.invoke(accepted)).resolves.toEqual({
      invocationId: accepted.invocationId,
      accepted: true,
    });
    expect(dispatch).toHaveBeenCalledWith(accepted);
  });

  it('bloqueia prova de diálogo stale, de outro diálogo ou origem global', async () => {
    const { composed, bridge, dispatch } = setup(true);
    bridge.replaceCapabilities([
      {
        id: 'cap-a',
        commandId: 'decision.respond',
        generation: '1',
        source: 'keyboard.local',
        owner,
      },
    ]);
    const baseProof = {
      dialogId: scope.dialogId,
      kind: 'decision' as const,
      scopeGeneration: scope.generation,
      commandId: 'decision.respond' as const,
      triggerSpec: 'keyboard.local:Ctrl+Shift+R' as const,
    };

    await expect(
      composed.invoke({
        ...invocation(),
        commandId: 'decision.respond',
        source: 'keyboard.local',
        dialogProof: { ...baseProof, scopeGeneration: '2' },
      })
    ).resolves.toMatchObject({ accepted: false, reason: 'dialog-blocked' });
    await expect(
      composed.invoke({
        ...invocation(),
        commandId: 'decision.respond',
        source: 'keyboard.local',
        dialogProof: { ...baseProof, dialogId: 'decision-b' },
      })
    ).resolves.toMatchObject({ accepted: false, reason: 'dialog-blocked' });
    await expect(
      composed.invoke({
        ...invocation(),
        commandId: 'decision.respond',
        source: 'keyboard.global',
        ownership: 'global',
        dialogProof: baseProof,
      })
    ).resolves.toMatchObject({ accepted: false, reason: 'dialog-blocked' });
    expect(dispatch).not.toHaveBeenCalled();
  });

  it('relê stack após resolver e impede candidato atravessar abertura de diálogo', async () => {
    const { composed, dispatch } = setup();
    const resolve = vi.fn(() => {
      registerOpenModal('decision-a', scope);
      return invocation('1', 'keyboard.local');
    });
    await expect(
      composed.dispatchLocalKeyboard(new KeyboardEvent('keydown', { key: 'x' }), resolve)
    ).resolves.toEqual({ kind: 'blocked' });
    expect(resolve).toHaveBeenCalledTimes(1);
    expect(dispatch).not.toHaveBeenCalled();
  });

  it('recusa capacidade alterada no mesmo controle focalizado', async () => {
    const { composed, dispatch } = setup();
    const button = document.createElement('button');
    document.body.appendChild(button);
    button.focus();
    const before = composed.readContext()?.frame.focus.control;
    const result = await composed.dispatchLocalKeyboard(
      new KeyboardEvent('keydown', { key: 'x' }),
      () => {
        button.setAttribute('aria-disabled', 'true');
        return invocation('1', 'keyboard.local');
      }
    );
    const after = composed.readContext()?.frame.focus.control;
    expect(after?.identity).toBe(before?.identity);
    expect(before?.capabilities.disabled).toBe(false);
    expect(after?.capabilities.disabled).toBe(true);
    expect(result).toEqual({ kind: 'blocked' });
    expect(dispatch).not.toHaveBeenCalled();
  });

  it('recusa perda de foco da janela sem mudança do controle', async () => {
    const { composed, dispatch } = setup();
    const focus = vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    await expect(
      composed.dispatchLocalKeyboard(new KeyboardEvent('keydown', { key: 'x' }), () => {
        focus.mockReturnValue(false);
        return invocation('1', 'keyboard.local');
      })
    ).resolves.toEqual({ kind: 'blocked' });
    expect(dispatch).not.toHaveBeenCalled();
  });

  it.each(['version', 'selection', 'freshness'])(
    'recusa mudança de surface: %s',
    async (change) => {
      const { composed, context, dispatch } = setup();
      let value = { ...surface('editor-1'), selection: { kind: 'text', text: 'before' } };
      context.registerSurfaceContext('editor-1', () => value);
      await expect(
        composed.dispatchLocalKeyboard(
          new KeyboardEvent('keydown', { key: 'x' }),
          () => {
            value =
              change === 'version'
                ? { ...value, snapshotVersion: 'snapshot-2' }
                : change === 'selection'
                  ? { ...value, selection: { kind: 'text', text: 'after' } }
                  : { ...value, staleAfterMs: 10_000 };
            return invocation('1', 'keyboard.local');
          },
          'editor-1'
        )
      ).resolves.toEqual({ kind: 'blocked' });
      expect(dispatch).not.toHaveBeenCalled();
    }
  );

  it('usa o frame revalidado sem terceira consulta e aceita JSON com ordem de chaves diferente', async () => {
    const { composed, context, dispatch } = setup();
    const stable = surface('editor-1');
    let reads = 0;
    context.registerSurfaceContext('editor-1', () => {
      reads += 1;
      return {
        ...stable,
        selection: {
          kind: 'text',
          range: reads === 1 ? { startOffset: 1, endOffset: 2 } : { endOffset: 2, startOffset: 1 },
        },
      };
    });
    await expect(
      composed.dispatchLocalKeyboard(
        new KeyboardEvent('keydown', { key: 'x' }),
        () => ({
          ...invocation('1', 'keyboard.local'),
        }),
        'editor-1'
      )
    ).resolves.toMatchObject({ kind: 'dispatched', ack: { accepted: true } });
    expect(reads).toBe(2);
    expect(dispatch).toHaveBeenCalledTimes(1);
  });

  it('caminho interno mantém validação de capability, sessão e dispose', async () => {
    const { composed, dispatch } = setup();
    const event = new KeyboardEvent('keydown', { key: 'x' });
    await expect(
      composed.dispatchLocalKeyboard(event, () => ({
        ...invocation('1', 'keyboard.local'),
        capabilityId: 'foreign',
      }))
    ).rejects.toMatchObject({ code: 'capability-denied' });
    stores.auth.isAuthenticated = false;
    const resolve = vi.fn();
    await expect(composed.dispatchLocalKeyboard(event, resolve)).rejects.toMatchObject({
      code: 'session-unavailable',
    });
    expect(resolve).not.toHaveBeenCalled();
    await composed.dispose();
    await expect(composed.dispatchLocalKeyboard(event, resolve)).rejects.toMatchObject({
      code: 'bridge-closed',
    });
    expect(dispatch).not.toHaveBeenCalled();
  });

  it('compõe modal topmost, foco e surface real em frame autenticado', () => {
    const { composed, cleanupSurface, overlay } = setup(true);
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
    const { composed, dispatch, bridge, context, cleanupSurface, overlay } = setup();
    const current = invocation();
    await expect(composed.invoke(current, 'editor-1')).resolves.toMatchObject({ accepted: true });

    stores.auth.user = { userId: 'user-b', sessionId: 'session-b' };
    expect(composed.readContext('editor-1')).toBeUndefined();
    await expect(composed.invoke(invocation(), 'editor-1')).rejects.toMatchObject({
      code: 'session-unavailable',
    });

    stores.auth.user = { userId: 'user-a', sessionId: 'session-a' };
    // Retornar ao owner anterior não ressuscita a lease da superfície.
    expect(composed.readContext('editor-1')).toBeUndefined();
    const newCleanup = context.registerSurfaceContext('editor-1', () => surface('editor-1'));
    const staleContext = createTrustedCommandContextSession();
    const staleCleanup = staleContext.registerSurfaceContext('old-surface', () =>
      surface('old-surface', new Date(Date.now() - 10_000).toISOString())
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
    await expect(
      composed.invoke(
        { ...invocation('1'), invocationId: '01900000-0000-7000-8000-000000000099' },
        'editor-1'
      )
    ).rejects.toMatchObject({
      code: 'stale-generation',
    });
    expect(dispatch).toHaveBeenCalledTimes(1);

    newCleanup();
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
      { id: 'cap-a', commandId: 'command.a', generation: '1', source: 'ui.action', owner },
      {
        id: 'cap-b',
        commandId: 'command.a',
        generation: '1',
        source: 'ui.action',
        owner: otherOwner,
      },
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

    await expect(composed.invoke(invocation(), 'editor-1')).resolves.toMatchObject({
      accepted: true,
    });
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
      lifecycle: vi.fn(
        (event: { generation?: string }) =>
          new Promise<void>((resolve) => {
            if (event.generation) releases.set(event.generation, resolve);
            else resolve();
          })
      ),
      shutdown: vi.fn(async () => undefined),
      subscribeResult: vi.fn(() => () => undefined),
    } as unknown as CommandBridge;
    const composed = createAuthenticatedCommandBridge({
      bridge: fakeBridge,
      context,
      session,
      ownership: 'exclusive',
    });

    const generationTwo = composed.lifecycle({
      kind: 'generation',
      sessionId: session.id,
      generation: '2',
    });
    const generationThree = composed.lifecycle({
      kind: 'generation',
      sessionId: session.id,
      generation: '3',
    });
    releases.get('3')?.();
    await generationThree;
    releases.get('2')?.();
    await generationTwo;

    await expect(
      composed.invoke(
        { ...invocation('2'), invocationId: '01900000-0000-7000-8000-000000000099' },
        'editor-1'
      )
    ).rejects.toMatchObject({
      code: 'stale-generation',
    });
    await expect(
      composed.invoke(
        { ...invocation('3'), invocationId: '01900000-0000-7000-8000-000000000100' },
        'editor-1'
      )
    ).resolves.toMatchObject({ accepted: true });
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
