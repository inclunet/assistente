import { afterEach, describe, expect, it, vi } from 'vitest';

const stores = vi.hoisted(() => {
  const authListeners = new Set<() => void>();
  const workspaceListeners = new Set<() => void>();
  return {
    auth: {
      isAuthenticated: true,
      user: { userId: 'u', sessionId: 's', role: 'user' } as {
        userId: string;
        sessionId: string;
        role: string;
      } | null,
    },
    workspace: { workspace: { id: 'w', profile: 'p', tabs: [], activeTabId: null } },
    authListeners,
    workspaceListeners,
    notify(listeners: Set<() => void>) {
      listeners.forEach((listener) => listener());
    },
  };
});

vi.mock('../store/authStore', () => ({
  useAuthStore: {
    getState: () => stores.auth,
    subscribe: (listener: () => void) => {
      stores.authListeners.add(listener);
      return () => stores.authListeners.delete(listener);
    },
  },
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: {
    getState: () => stores.workspace,
    subscribe: (listener: () => void) => {
      stores.workspaceListeners.add(listener);
      return () => stores.workspaceListeners.delete(listener);
    },
  },
}));

import {
  createCommandUIExecution,
  type CommandExecutionResult,
  type CommandUIExecutionPort,
} from './commandUIExecution';
import type { CommandUIEffect } from './commandUIEffect';
import { createTrustedCommandContextSession } from './commandContextSession';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function focusReady(): void {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  const button = document.createElement('button');
  document.body.appendChild(button);
  button.focus();
}

function portFor(commandID = 'help.shortcuts.show'): CommandUIExecutionPort & {
  begin: ReturnType<typeof vi.fn>;
  take: ReturnType<typeof vi.fn>;
  complete: ReturnType<typeof vi.fn>;
  getResult: ReturnType<typeof vi.fn>;
  cancelUI: ReturnType<typeof vi.fn>;
} {
  const ticket = '01900000-0000-7000-8000-000000000001';
  const invocationId = '01900000-0000-7000-8000-000000000002';
  const handoffId = '01900000-0000-7000-8000-000000000003';
  const port = {
    begin: vi.fn(async () => ({ ticket, invocationId, commandId: commandID })),
    take: vi.fn(async () => ({ ticket, invocationId, commandId: commandID, handoffId })),
    complete: vi.fn(async () => undefined),
    getResult: vi.fn(
      async (): Promise<CommandExecutionResult> => ({ invocationId, status: 'succeeded' })
    ),
    cancelUI: vi.fn(async () => undefined),
  };
  return {
    beginUICommand: port.begin,
    takeUICommand: port.take,
    completeUICommand: port.complete,
    getUICommandResult: port.getResult,
    cancelUICommand: port.cancelUI,
    ...port,
  };
}

const effect: CommandUIEffect = () => undefined;

afterEach(() => {
  stores.auth.isAuthenticated = true;
  stores.auth.user = { userId: 'u', sessionId: 's', role: 'user' };
  stores.workspace.workspace = { id: 'w', profile: 'p', tabs: [], activeTabId: null };
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

describe('commandUIExecution', () => {
  it('recusa alvo contextual indisponível antes de reservar no backend', async () => {
    focusReady();
    const port = portFor('workspace.panel.focus');
    const handler = vi.fn(() => undefined);
    const execution = createCommandUIExecution(port, new Map([['workspace.panel.focus', handler]]), {
      prepareEffect: () => undefined,
    });
    await expect(execution.execute('workspace.panel.focus')).resolves.toMatchObject({ status: 'cancelled', errorCode: 'ui-context-stale' });
    expect(port.begin).not.toHaveBeenCalled();
    expect(handler).not.toHaveBeenCalled();
    execution.dispose();
  });

  it('executa o alvo capturado uma única vez e libera sua preparação', async () => {
    focusReady();
    const port = portFor('workspace.panel.focus');
    const fallback = vi.fn(() => undefined);
    const prepared = { effect: vi.fn(() => undefined), isCurrent: vi.fn(() => true), dispose: vi.fn() };
    const prepareEffect = vi.fn(() => prepared);
    const execution = createCommandUIExecution(port, new Map([['workspace.panel.focus', fallback]]), { prepareEffect });
    const pending = execution.execute('workspace.panel.focus');
    expect(execution.execute('workspace.panel.focus')).toBe(pending);
    await expect(pending).resolves.toMatchObject({ status: 'succeeded' });
    expect(prepareEffect).toHaveBeenCalledTimes(1);
    expect(prepared.effect).toHaveBeenCalledTimes(1);
    expect(fallback).not.toHaveBeenCalled();
    expect(prepared.dispose).toHaveBeenCalledTimes(1);
    execution.dispose();
  });

  it.each(['stale', 'throw'] as const)('recusa alvo %s durante Take sem procurar substituto', async (mode) => {
    focusReady();
    const port = portFor('workspace.panel.focus');
    const take = deferred<Awaited<ReturnType<typeof port.take>>>();
    port.take.mockReturnValue(take.promise);
    port.getResult.mockResolvedValue({ invocationId: '01900000-0000-7000-8000-000000000002', status: 'cancelled' });
    const prepared = { effect: vi.fn(() => undefined), isCurrent: vi.fn(() => true), dispose: vi.fn() };
    const prepareEffect = vi.fn(() => prepared);
    const execution = createCommandUIExecution(port, new Map([['workspace.panel.focus', effect]]), { prepareEffect });
    const pending = execution.execute('workspace.panel.focus');
    await vi.waitFor(() => expect(port.take).toHaveBeenCalledTimes(1));
    if (mode === 'stale') prepared.isCurrent.mockReturnValue(false);
    else prepared.isCurrent.mockImplementation(() => { throw new Error('provider gone'); });
    take.resolve({ ticket: '01900000-0000-7000-8000-000000000001', invocationId: '01900000-0000-7000-8000-000000000002', commandId: 'workspace.panel.focus', handoffId: '01900000-0000-7000-8000-000000000003' });
    await expect(pending).resolves.toMatchObject({ status: 'cancelled' });
    expect(prepared.effect).not.toHaveBeenCalled();
    expect(prepareEffect).toHaveBeenCalledTimes(1);
    expect(port.complete).toHaveBeenCalledWith(expect.any(String), expect.any(String), 'cancelled');
    expect(prepared.dispose).toHaveBeenCalledTimes(1);
    execution.dispose();
  });

  it('libera alvo preparado mesmo quando Begin falha', async () => {
    focusReady();
    const port = portFor('workspace.panel.focus');
    port.begin.mockRejectedValue(new Error('bridge unavailable'));
    const prepared = { effect: vi.fn(() => undefined), isCurrent: () => true, dispose: vi.fn() };
    const execution = createCommandUIExecution(port, new Map([['workspace.panel.focus', effect]]), { prepareEffect: () => prepared });
    await expect(execution.execute('workspace.panel.focus')).resolves.toMatchObject({ status: 'outcome_unknown' });
    expect(prepared.dispose).toHaveBeenCalledTimes(1);
    expect(prepared.effect).not.toHaveBeenCalled();
    execution.dispose();
  });

  it('confirma a navegação mesmo quando seu efeito desmonta a superfície', async () => {
    focusReady();
    const port = portFor('navigation.settings.open');
    let execution: ReturnType<typeof createCommandUIExecution>;
    const navigate = vi.fn(() => {
      execution.dispose();
      return undefined;
    });
    execution = createCommandUIExecution(port, new Map([['navigation.settings.open', navigate]]));
    await expect(execution.execute('navigation.settings.open')).resolves.toMatchObject({ status: 'succeeded' });
    expect(navigate).toHaveBeenCalledTimes(1);
    expect(port.complete).toHaveBeenCalledWith(
      '01900000-0000-7000-8000-000000000001',
      '01900000-0000-7000-8000-000000000003',
      'succeeded',
    );
    expect(port.cancelUI).not.toHaveBeenCalled();
    await expect(execution.execute('navigation.settings.open')).resolves.toMatchObject({ errorCode: 'disposed' });
  });

  it('desmontagem depois do efeito não concorre com a confirmação pendente', async () => {
    focusReady();
    const port = portFor('navigation.settings.open');
    const confirmation = deferred<void>();
    port.complete.mockReturnValue(confirmation.promise);
    const navigate = vi.fn(() => undefined);
    const execution = createCommandUIExecution(port, new Map([['navigation.settings.open', navigate]]));
    const pending = execution.execute('navigation.settings.open');
    await vi.waitFor(() => expect(port.complete).toHaveBeenCalledTimes(1));
    execution.dispose();
    expect(port.cancelUI).not.toHaveBeenCalled();
    confirmation.resolve();
    await expect(pending).resolves.toMatchObject({ status: 'succeeded' });
    expect(navigate).toHaveBeenCalledTimes(1);
  });

  it('ainda cancela efeito que desmonta e lança, sem inventar sucesso', async () => {
    focusReady();
    const port = portFor('navigation.settings.open');
    port.getResult.mockResolvedValue({ invocationId: '01900000-0000-7000-8000-000000000002', status: 'outcome_unknown' });
    let execution: ReturnType<typeof createCommandUIExecution>;
    execution = createCommandUIExecution(port, new Map([['navigation.settings.open', () => {
      execution.dispose();
      throw new Error('navigation failed');
    }]]));
    await expect(execution.execute('navigation.settings.open')).resolves.toMatchObject({ status: 'outcome_unknown' });
    expect(port.cancelUI).toHaveBeenCalledTimes(1);
    expect(port.complete).not.toHaveBeenCalled();
  });

  it('não faz Begin quando a sessão real não possui a superfície solicitada', async () => {
    focusReady();
    const port = portFor();
    const session = createTrustedCommandContextSession();
    const execution = createCommandUIExecution(port, new Map([['help.shortcuts.show', effect]]), {
      trustedSession: session,
      surfaceID: 'surface-real',
    });

    await expect(execution.execute('help.shortcuts.show')).resolves.toMatchObject({
      status: 'cancelled',
      errorCode: 'ui-context-stale',
    });
    expect(port.begin).not.toHaveBeenCalled();
    execution.dispose();
    expect(session.readOwnedCommandContextFrame()).toBeDefined();
    session.dispose();
  });

  it('não comita efeito quando o snapshot da superfície muda durante o handoff', async () => {
    focusReady();
    const port = portFor();
    const take = deferred<Awaited<ReturnType<typeof port.take>>>();
    port.take.mockReturnValue(take.promise);
    port.getResult.mockResolvedValue({
      invocationId: '01900000-0000-7000-8000-000000000002',
      status: 'cancelled',
    });
    const session = createTrustedCommandContextSession();
    let snapshotVersion = 'surface-real-v1';
    const cleanup = session.registerSurfaceContext('surface-real', () => ({
      surfaceType: 'editor',
      surfaceId: 'surface-real',
      snapshotVersion,
    }));
    const handler = vi.fn(() => undefined) as unknown as CommandUIEffect;
    const execution = createCommandUIExecution(port, new Map([['help.shortcuts.show', handler]]), {
      trustedSession: session,
      surfaceID: 'surface-real',
    });
    const promise = execution.execute('help.shortcuts.show');
    await vi.waitFor(() => expect(port.take).toHaveBeenCalled());

    // O getter real da superfície muda durante o handoff; não há Notify nem
    // dispose. A captura/commit deve reler a mesma superfície e falhar fechado.
    snapshotVersion = 'surface-real-v2';
    take.resolve({
      ticket: '01900000-0000-7000-8000-000000000001',
      invocationId: '01900000-0000-7000-8000-000000000002',
      commandId: 'help.shortcuts.show',
      handoffId: '01900000-0000-7000-8000-000000000003',
    });

    await expect(promise).resolves.toMatchObject({ status: 'cancelled' });
    expect(handler).not.toHaveBeenCalled();
    expect(port.complete).toHaveBeenCalledWith(
      '01900000-0000-7000-8000-000000000001',
      '01900000-0000-7000-8000-000000000003',
      'cancelled'
    );
    cleanup();
    execution.dispose();
    session.dispose();
  });

  it('executa com surfaceID registrada na sessão compartilhada', async () => {
    focusReady();
    const port = portFor();
    const session = createTrustedCommandContextSession();
    const cleanup = session.registerSurfaceContext('surface-real', () => ({
      surfaceType: 'editor',
      surfaceId: 'surface-real',
      snapshotVersion: 'surface-real-v1',
    }));
    const handler = vi.fn(() => undefined) as unknown as CommandUIEffect;
    const execution = createCommandUIExecution(port, new Map([['help.shortcuts.show', handler]]), {
      trustedSession: session,
      surfaceID: 'surface-real',
    });

    await expect(execution.execute('help.shortcuts.show')).resolves.toMatchObject({
      status: 'succeeded',
    });
    expect(handler).toHaveBeenCalledTimes(1);
    expect(port.complete).toHaveBeenCalledWith(
      '01900000-0000-7000-8000-000000000001',
      '01900000-0000-7000-8000-000000000003',
      'succeeded'
    );
    expect(session.readSurfaceContext('surface-real')).toMatchObject({
      surfaceId: 'surface-real',
    });
    cleanup();
    execution.dispose();
    session.dispose();
  });

  it('dispose do executor não destrói sessão real emprestada', () => {
    focusReady();
    const port = portFor();
    const session = createTrustedCommandContextSession();
    const cleanup = session.registerSurfaceContext('surface-real', () => ({
      surfaceType: 'editor',
      surfaceId: 'surface-real',
      snapshotVersion: 'surface-real-v1',
    }));
    const execution = createCommandUIExecution(port, new Map([['help.shortcuts.show', effect]]), {
      trustedSession: session,
      surfaceID: 'surface-real',
    });

    execution.dispose();
    expect(session.readSurfaceContext('surface-real')).toMatchObject({
      surfaceId: 'surface-real',
    });
    cleanup();
    session.dispose();
  });

  it('faz Begin/Take, executa efeito uma vez e só resolve após GetResult', async () => {
    focusReady();
    const port = portFor();
    const result = deferred<CommandExecutionResult>();
    port.getResult.mockReturnValue(result.promise);
    let calls = 0;
    const handlers = new Map([
      [
        'help.shortcuts.show',
        (() => {
          calls += 1;
          return undefined;
        }) as CommandUIEffect,
      ],
    ]);
    const execution = createCommandUIExecution(port, handlers);

    let settled = false;
    const promise = execution.execute('help.shortcuts.show').then((value) => {
      settled = true;
      return value;
    });
    await vi.waitFor(() =>
      expect(port.complete).toHaveBeenCalledWith(
        '01900000-0000-7000-8000-000000000001',
        '01900000-0000-7000-8000-000000000003',
        'succeeded'
      )
    );
    expect(calls).toBe(1);
    expect(settled).toBe(false);
    result.resolve({ invocationId: '01900000-0000-7000-8000-000000000002', status: 'succeeded' });
    await expect(promise).resolves.toMatchObject({
      invocationId: '01900000-0000-7000-8000-000000000002',
      status: 'succeeded',
    });
    execution.dispose();
  });

  it('deduplica tentativas do mesmo comando e não faz retry após confirmação perdida', async () => {
    focusReady();
    const port = portFor();
    port.complete.mockRejectedValue(new Error('confirmation lost'));
    port.getResult.mockResolvedValue({
      invocationId: '01900000-0000-7000-8000-000000000002',
      status: 'outcome_unknown',
    });
    const execution = createCommandUIExecution(port, new Map([['help.shortcuts.show', effect]]));
    const first = execution.execute('help.shortcuts.show');
    const second = execution.execute('help.shortcuts.show');
    expect(second).toBe(first);
    await expect(first).resolves.toMatchObject({
      status: 'outcome_unknown',
      invocationId: '01900000-0000-7000-8000-000000000002',
    });
    expect(port.begin).toHaveBeenCalledTimes(1);
    expect(port.complete).toHaveBeenCalledTimes(1);
    execution.dispose();
  });

  it('cancela depois do Take sem executar o efeito e consulta o estado backend', async () => {
    focusReady();
    const port = portFor();
    const take = deferred<Awaited<ReturnType<typeof port.take>>>();
    port.take.mockReturnValue(take.promise);
    port.getResult.mockResolvedValue({
      invocationId: '01900000-0000-7000-8000-000000000002',
      status: 'outcome_unknown',
    });
    port.complete.mockRejectedValue(new Error('cancel raced with Take'));
    const handler = vi.fn(() => undefined) as unknown as CommandUIEffect;
    const execution = createCommandUIExecution(port, new Map([['help.shortcuts.show', handler]]));
    const promise = execution.execute('help.shortcuts.show');
    await vi.waitFor(() => expect(port.take).toHaveBeenCalled());
    await execution.cancel('help.shortcuts.show');
    take.resolve({
      ticket: '01900000-0000-7000-8000-000000000001',
      invocationId: '01900000-0000-7000-8000-000000000002',
      commandId: 'help.shortcuts.show',
      handoffId: '01900000-0000-7000-8000-000000000003',
    });
    await expect(promise).resolves.toMatchObject({
      status: 'outcome_unknown',
      invocationId: '01900000-0000-7000-8000-000000000002',
    });
    expect(handler).not.toHaveBeenCalled();
    expect(port.cancelUI).toHaveBeenCalledWith('01900000-0000-7000-8000-000000000001');
    expect(port.complete).toHaveBeenCalledWith(
      '01900000-0000-7000-8000-000000000001',
      '01900000-0000-7000-8000-000000000003',
      'cancelled'
    );
    execution.dispose();
  });

  it.each(['suppressed', 'denied', 'rejected_stale'] as const)(
    'preserva resultado autoritativo %s quando Take recusa antes do handoff',
    async (status) => {
      focusReady();
      const port = portFor();
      port.take.mockRejectedValue(new Error(`executor-${status}`));
      const result = deferred<CommandExecutionResult>();
      port.getResult.mockReturnValue(result.promise);
      port.cancelUI.mockImplementation(async () => {
        result.resolve({
          invocationId: '01900000-0000-7000-8000-000000000002',
          status,
        });
      });
      const handler = vi.fn(() => undefined) as unknown as CommandUIEffect;
      const execution = createCommandUIExecution(port, new Map([['help.shortcuts.show', handler]]));

      await expect(execution.execute('help.shortcuts.show')).resolves.toMatchObject({
        invocationId: '01900000-0000-7000-8000-000000000002',
        status,
      });
      expect(handler).not.toHaveBeenCalled();
      expect(port.cancelUI).toHaveBeenCalledWith('01900000-0000-7000-8000-000000000001');
      execution.dispose();
    }
  );

  it('não converte falha de transporte ou resultado ausente em sucesso após Take', async () => {
    focusReady();
    const port = portFor();
    port.take.mockRejectedValue(new Error('transport lost'));
    port.getResult.mockRejectedValue(new Error('result unavailable'));
    const handler = vi.fn(() => undefined) as unknown as CommandUIEffect;
    const execution = createCommandUIExecution(port, new Map([['help.shortcuts.show', handler]]));

    await expect(execution.execute('help.shortcuts.show')).resolves.toMatchObject({
      status: 'outcome_unknown',
      errorCode: 'take-confirmation-unknown',
    });
    expect(handler).not.toHaveBeenCalled();
    expect(port.cancelUI).toHaveBeenCalledWith('01900000-0000-7000-8000-000000000001');
    execution.dispose();
  });

  it('não aceita succeeded após falha de Take sem handoff confirmado', async () => {
    focusReady();
    const port = portFor();
    port.take.mockRejectedValue(new Error('transport lost'));
    const result = deferred<CommandExecutionResult>();
    port.getResult.mockReturnValue(result.promise);
    port.cancelUI.mockImplementation(async () => {
      result.resolve({ invocationId: '01900000-0000-7000-8000-000000000002', status: 'succeeded' });
    });
    const handler = vi.fn(() => undefined) as unknown as CommandUIEffect;
    const execution = createCommandUIExecution(port, new Map([['help.shortcuts.show', handler]]));

    await expect(execution.execute('help.shortcuts.show')).resolves.toMatchObject({
      status: 'outcome_unknown',
      errorCode: 'succeeded-without-handoff',
    });
    expect(handler).not.toHaveBeenCalled();
    expect(port.cancelUI).toHaveBeenCalledTimes(1);
    execution.dispose();
  });

  it('mantém resultado autoritativo quando cancelamento e owner stale chegam durante a consulta', async () => {
    focusReady();
    const port = portFor();
    const result = deferred<CommandExecutionResult>();
    port.getResult.mockReturnValue(result.promise);
    const handler = vi.fn(() => undefined) as unknown as CommandUIEffect;
    const execution = createCommandUIExecution(port, new Map([['help.shortcuts.show', handler]]));
    const promise = execution.execute('help.shortcuts.show');

    await vi.waitFor(() =>
      expect(port.complete).toHaveBeenCalledWith(
        '01900000-0000-7000-8000-000000000001',
        '01900000-0000-7000-8000-000000000003',
        'succeeded'
      )
    );
    stores.auth.isAuthenticated = false;
    stores.auth.user = null;
    stores.notify(stores.authListeners);
    await execution.cancel('help.shortcuts.show');
    result.resolve({ invocationId: '01900000-0000-7000-8000-000000000002', status: 'denied' });

    await expect(promise).resolves.toMatchObject({ status: 'denied' });
    expect(handler).toHaveBeenCalledTimes(1);
    // Cancelamento explícito após o efeito não desfaz uma navegação já iniciada.
    expect(port.cancelUI).not.toHaveBeenCalled();
    execution.dispose();
  });

  it('recusa logout entre captura e commit e não inicia Begin antes da captura válida', async () => {
    focusReady();
    const port = portFor();
    const begin = deferred<Awaited<ReturnType<typeof port.begin>>>();
    const take = deferred<Awaited<ReturnType<typeof port.take>>>();
    port.begin.mockReturnValue(begin.promise);
    port.take.mockReturnValue(take.promise);
    port.getResult.mockResolvedValue({
      invocationId: '01900000-0000-7000-8000-000000000002',
      status: 'outcome_unknown',
    });
    const handler = vi.fn(() => undefined) as unknown as CommandUIEffect;
    const execution = createCommandUIExecution(port, new Map([['help.shortcuts.show', handler]]));
    const promise = execution.execute('help.shortcuts.show');
    expect(port.begin).toHaveBeenCalledTimes(1);
    stores.auth.isAuthenticated = false;
    stores.auth.user = null;
    stores.notify(stores.authListeners);
    begin.resolve({
      ticket: '01900000-0000-7000-8000-000000000001',
      invocationId: '01900000-0000-7000-8000-000000000002',
      commandId: 'help.shortcuts.show',
    });
    take.resolve({
      ticket: '01900000-0000-7000-8000-000000000001',
      invocationId: '01900000-0000-7000-8000-000000000002',
      commandId: 'help.shortcuts.show',
      handoffId: '01900000-0000-7000-8000-000000000003',
    });
    await expect(promise).resolves.toMatchObject({ status: 'outcome_unknown' });
    expect(handler).not.toHaveBeenCalled();
    expect(port.complete).toHaveBeenCalledWith(
      '01900000-0000-7000-8000-000000000001',
      '01900000-0000-7000-8000-000000000003',
      'cancelled'
    );
    execution.dispose();
  });

  it('recusa correlação Take e efeito que lança sem reportar failed', async () => {
    focusReady();
    const port = portFor();
    port.take.mockResolvedValue({
      ticket: '01900000-0000-7000-8000-000000000001',
      invocationId: '01900000-0000-7000-8000-000000000004',
      commandId: 'help.shortcuts.show',
      handoffId: '01900000-0000-7000-8000-000000000003',
    });
    port.getResult.mockResolvedValue({
      invocationId: '01900000-0000-7000-8000-000000000002',
      status: 'outcome_unknown',
    });
    const execution = createCommandUIExecution(port, new Map([['help.shortcuts.show', effect]]));
    await expect(execution.execute('help.shortcuts.show')).resolves.toMatchObject({
      status: 'outcome_unknown',
    });
    expect(port.cancelUI).toHaveBeenCalledWith('01900000-0000-7000-8000-000000000001');
    expect(port.complete).not.toHaveBeenCalled();
    execution.dispose();

    const throwingPort = portFor();
    throwingPort.getResult.mockResolvedValue({
      invocationId: '01900000-0000-7000-8000-000000000002',
      status: 'outcome_unknown',
    });
    const throwing = vi.fn(() => {
      throw new Error('effect failed');
    }) as unknown as CommandUIEffect;
    const second = createCommandUIExecution(
      throwingPort,
      new Map([['help.shortcuts.show', throwing]])
    );
    await expect(second.execute('help.shortcuts.show')).resolves.toMatchObject({
      status: 'outcome_unknown',
    });
    expect(throwing).toHaveBeenCalledTimes(1);
    expect(throwingPort.cancelUI).toHaveBeenCalledWith('01900000-0000-7000-8000-000000000001');
    expect(throwingPort.complete).not.toHaveBeenCalled();
    second.dispose();
  });

  it('cancela Begin tardio depois de dispose', async () => {
    focusReady();
    const port = portFor();
    const begin = deferred<Awaited<ReturnType<typeof port.begin>>>();
    port.begin.mockReturnValue(begin.promise);
    port.getResult.mockResolvedValue({
      invocationId: '01900000-0000-7000-8000-000000000002',
      status: 'outcome_unknown',
    });
    const execution = createCommandUIExecution(port, new Map([['help.shortcuts.show', effect]]));
    const promise = execution.execute('help.shortcuts.show');
    execution.dispose();
    begin.resolve({
      ticket: '01900000-0000-7000-8000-000000000001',
      invocationId: '01900000-0000-7000-8000-000000000002',
      commandId: 'help.shortcuts.show',
    });
    await expect(promise).resolves.toMatchObject({ status: 'outcome_unknown' });
    expect(port.cancelUI).toHaveBeenCalledWith('01900000-0000-7000-8000-000000000001');
  });

  it('rejeita foco stale sem logout ou cancel explícito e completa como cancelled', async () => {
    focusReady();
    const port = portFor();
    port.take.mockImplementation(async () => {
      window.dispatchEvent(new Event('blur'));
      return {
        ticket: '01900000-0000-7000-8000-000000000001',
        invocationId: '01900000-0000-7000-8000-000000000002',
        commandId: 'help.shortcuts.show',
        handoffId: '01900000-0000-7000-8000-000000000003',
      };
    });
    port.getResult.mockResolvedValue({
      invocationId: '01900000-0000-7000-8000-000000000002',
      status: 'cancelled',
    });
    const handler = vi.fn(() => undefined) as unknown as CommandUIEffect;
    const execution = createCommandUIExecution(port, new Map([['help.shortcuts.show', handler]]));

    await expect(execution.execute('help.shortcuts.show')).resolves.toMatchObject({
      invocationId: '01900000-0000-7000-8000-000000000002',
      status: 'cancelled',
    });
    expect(handler).not.toHaveBeenCalled();
    expect(port.cancelUI).not.toHaveBeenCalled();
    expect(port.complete).toHaveBeenCalledWith(
      '01900000-0000-7000-8000-000000000001',
      '01900000-0000-7000-8000-000000000003',
      'cancelled'
    );
    execution.dispose();
  });

  it('trata Begin null como resposta malformada sem lançar', async () => {
    focusReady();
    const port = portFor();
    port.begin.mockResolvedValue(null);
    const execution = createCommandUIExecution(port, new Map([['help.shortcuts.show', effect]]));

    await expect(execution.execute('help.shortcuts.show')).resolves.toMatchObject({
      status: 'outcome_unknown',
      errorCode: 'malformed-begin',
    });
    expect(port.begin).toHaveBeenCalledTimes(1);
    expect(port.cancelUI).not.toHaveBeenCalled();
    execution.dispose();
  });

  it('não chama Begin quando o handler não está registrado', async () => {
    focusReady();
    const port = portFor();
    const execution = createCommandUIExecution(port, new Map());

    await expect(execution.execute('help.shortcuts.show')).resolves.toMatchObject({
      status: 'cancelled',
      errorCode: 'ui-handler-missing',
    });
    expect(port.begin).not.toHaveBeenCalled();
    execution.dispose();
  });
});
