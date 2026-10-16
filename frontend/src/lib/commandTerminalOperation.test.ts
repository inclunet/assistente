import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { acquireCommandFocusTracking } from './commandFocusContext';
import {
  captureTerminalOperationTarget,
  executeTerminalInterrupt,
  executeTerminalSessionOperation,
  registerTerminalOperationSurface,
  requestTerminalOperation,
  TERMINAL_INTERRUPT_COMMAND,
  TERMINAL_SESSION_CLOSE_COMMAND,
  TERMINAL_SESSION_CREATE_COMMAND,
  TERMINAL_OPERATION_EVENT,
  type TerminalOperationPort,
} from './commandTerminalOperation';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';
import { useAuthStore } from '../store/authStore';
import { useTerminalStore } from '../store/terminalStore';
import { useWorkspaceStore } from '../store/workspaceStore';

const cleanups: Array<() => void> = [];
const originalAuth = useAuthStore.getState();
const originalTerminal = useTerminalStore.getState();
const originalWorkspace = useWorkspaceStore.getState();

const reservation = {
  ticket: 'terminal-ticket',
  invocationId: 'terminal-invocation',
  commandId: TERMINAL_INTERRUPT_COMMAND,
};
const handoff = { ...reservation, handoffId: 'terminal-handoff' };

function setup(instanceId = 'terminal-instance-a') {
  const root = document.createElement('main');
  document.body.append(root);
  let route = '/terminal';
  let session = {
    id: 'session-a', name: 'Terminal A', cwd: '/tmp', state: 'running', shell: 'sh',
    createdAt: '', lastUsed: '',
  };
  const command = {
    id: 'entry-a', command: 'sleep 10', output: '', exitCode: -999,
    startedAt: '2026-09-19T00:00:00.000Z', endedAt: '', source: 'user-raw',
    convertValues: (value: unknown) => value,
  };
  useAuthStore.setState({
    isAuthenticated: true,
    user: { userId: 'user-a', sessionId: 'auth-session-a', role: 'user' },
  });
  useWorkspaceStore.setState({
    workspace: {
      id: 'workspace-a',
      name: 'Workspace A',
      activeTabId: 'tab-a',
      tabs: [{ id: 'tab-a', type: 'terminal', title: 'Terminal A', position: 0, state: { sessionId: session.id } }],
    },
  });
  useTerminalStore.setState({
    sessions: [session],
    historyBySession: { [session.id]: [command] },
    activeEntryBySession: { [session.id]: command.id },
  });

  const changed = new Set<() => void>();
  const unregister = registerTerminalOperationSurface({
    root,
    instanceId,
    tabId: 'tab-a',
    isCurrent: () => route === '/terminal',
    canStart: () => true,
    subscribe: listener => {
      changed.add(listener);
      return () => changed.delete(listener);
    },
  });
  cleanups.push(unregister, () => root.remove());

  return {
    root,
    command,
    get session() { return session; },
    set session(next) { session = next; },
    setRoute(next: string) { route = next; },
    capture: () => captureTerminalOperationTarget(() => route, instanceId),
    captureWithCommand: (commandId: typeof TERMINAL_INTERRUPT_COMMAND | typeof TERMINAL_SESSION_CREATE_COMMAND | typeof TERMINAL_SESSION_CLOSE_COMMAND) =>
      captureTerminalOperationTarget(() => route, instanceId, commandId),
    changed,
  };
}

function fakePort(overrides: Partial<TerminalOperationPort> = {}) {
  const calls: string[] = [];
  let begunCommandId = reservation.commandId;
  const port: TerminalOperationPort = {
    beginUICommand: vi.fn(async commandID => {
      calls.push('begin');
      begunCommandId = commandID;
      return { ...reservation, commandId: commandID };
    }),
    prepareTerminalInterruptCommand: vi.fn(async () => { calls.push('prepare'); }),
    prepareTerminalSessionCommand: vi.fn(async () => { calls.push('prepare-session'); }),
    takeUICommand: vi.fn(async () => {
      calls.push('take');
      return { ...handoff, commandId: begunCommandId };
    }),
    commitBackendCommand: vi.fn(async () => { calls.push('commit'); }),
    getUICommandResult: vi.fn(async () => {
      calls.push('result');
      return { invocationId: reservation.invocationId, status: 'succeeded' as const };
    }),
    completeUICommand: vi.fn(async () => { calls.push('complete'); }),
    cancelUICommand: vi.fn(async () => { calls.push('cancel'); }),
    ...overrides,
  };
  return { port, calls };
}

function restoreStores() {
  useAuthStore.setState(originalAuth);
  useTerminalStore.setState(originalTerminal);
  useWorkspaceStore.setState(originalWorkspace);
}

beforeEach(() => {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
});

afterEach(() => {
  cleanups.splice(0).reverse().forEach(cleanup => cleanup());
  unregisterOpenModal('terminal-operation-modal');
  document.body.replaceChildren();
  restoreStores();
  vi.restoreAllMocks();
});

describe('registry real da operação de terminal', () => {
  it('recusa sessão sem execução conhecida em vez de enviar Ctrl+C sem alvo', () => {
    const fixture = setup();
    useTerminalStore.setState({ activeEntryBySession: { 'session-a': null } });
    expect(fixture.capture()).toBeUndefined();
    useTerminalStore.setState({ activeEntryBySession: { 'session-a': 'not-in-history' } });
    expect(fixture.capture()).toBeUndefined();
  });
  it('captura owner, sessão, workspace, aba, binding e comando atual uma vez', () => {
    const fixture = setup();
    const target = fixture.capture()!;

    expect(target).toMatchObject({
      ownerId: 'user-a',
      sessionId: 'auth-session-a',
      workspaceId: 'workspace-a',
      tabId: 'tab-a',
      activeEntryId: 'entry-a',
      currentCommand: fixture.command,
      binding: { workspaceId: 'workspace-a', tabId: 'tab-a', sessionId: 'session-a' },
    });
    expect(target.session).toBe(useTerminalStore.getState().sessions[0]);
    expect(target.isCurrent()).toBe(true);
    expect(target.canCommit()).toBe(true);
  });

  it('mantém a lease durante streaming que recria HistoryEntry e invalida nova execução', () => {
    const fixture = setup();
    const target = fixture.capture()!;
    useTerminalStore.setState({
      historyBySession: {
        'session-a': [{ ...fixture.command, output: 'chunk-1' }],
      },
    });
    expect(target.isCurrent()).toBe(true);
    expect(target.canCommit()).toBe(true);

    useTerminalStore.setState({
      activeEntryBySession: { 'session-a': 'entry-b' },
      historyBySession: {
        'session-a': [{ ...fixture.command, id: 'entry-b', command: 'other command' }],
      },
    });
    expect(target.isCurrent()).toBe(false);
  });

  it.each(['owner', 'session', 'workspace', 'tab', 'binding', 'session-object', 'active-entry', 'current-command'] as const)(
    'invalida a lease por ABA de %s',
    field => {
      const fixture = setup();
      const target = fixture.capture()!;
      if (field === 'owner') {
        useAuthStore.setState({ user: { userId: 'user-b', sessionId: 'auth-session-a', role: 'user' } });
      } else if (field === 'session') {
        useAuthStore.setState({ user: { userId: 'user-a', sessionId: 'auth-session-b', role: 'user' } });
      } else if (field === 'workspace') {
        const workspace = useWorkspaceStore.getState().workspace!;
        useWorkspaceStore.setState({ workspace: { ...workspace, id: 'workspace-b' } });
      } else if (field === 'tab') {
        const workspace = useWorkspaceStore.getState().workspace!;
        useWorkspaceStore.setState({ workspace: { ...workspace, activeTabId: 'tab-b', tabs: [{ ...workspace.tabs[0], id: 'tab-b' }] } });
      } else if (field === 'binding') {
        const workspace = useWorkspaceStore.getState().workspace!;
        useWorkspaceStore.setState({ workspace: { ...workspace, tabs: [{ ...workspace.tabs[0], state: { sessionId: 'session-b' } }] } });
      } else if (field === 'session-object') {
        useTerminalStore.setState({ sessions: [{ ...fixture.session, name: 'renamed' }] });
      } else if (field === 'active-entry') {
        useTerminalStore.setState({ activeEntryBySession: { 'session-a': 'entry-b' } });
      } else {
        useTerminalStore.setState({ historyBySession: { 'session-a': [{ ...fixture.command, command: 'changed execution' }] } });
      }
      // The subscriptions invalidate immediately, before a value can return to
      // its original snapshot (ABA).
      expect(target.isCurrent()).toBe(false);
      target.dispose();
    },
  );

  it('invalida rota, modal, blur e composição IME sem retarget', () => {
    const fixture = setup();
    const target = fixture.capture()!;
    fixture.setRoute('/other');
    expect(target.isCurrent()).toBe(false);

    const second = setup('terminal-instance-b').capture()!;
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    document.body.append(overlay);
    registerOpenModal('terminal-operation-modal');
    expect(second.isCurrent()).toBe(false);
    unregisterOpenModal('terminal-operation-modal');
    overlay.remove();

    const third = setup('terminal-instance-c').capture()!;
    window.dispatchEvent(new Event('blur'));
    expect(third.isCurrent()).toBe(false);

    const fourth = setup('terminal-instance-d').capture()!;
    const releaseTracking = acquireCommandFocusTracking(document);
    const input = document.createElement('textarea');
    document.body.append(input);
    input.focus();
    input.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    expect(fourth.canCommit()).toBe(false);
    releaseTracking();
    input.remove();
  });

  it('evento é cancelável e preserva instanceId', () => {
    const listener = vi.fn((event: Event) => {
      expect((event as CustomEvent).detail).toEqual({ instanceId: 'terminal-instance-a' });
      event.preventDefault();
    });
    window.addEventListener(TERMINAL_OPERATION_EVENT, listener);
    try {
      expect(requestTerminalOperation('terminal-instance-a')).toBe(true);
    } finally {
      window.removeEventListener(TERMINAL_OPERATION_EVENT, listener);
    }
  });
});

describe('executeTerminalInterrupt com transporte fake', () => {
  it('faz Begin/Take/commit/resultado uma vez e usa o status autoritativo', async () => {
    const target = setup().capture()!;
    const { port, calls } = fakePort();

    await expect(executeTerminalInterrupt(port, target)).resolves.toBe('succeeded');
    expect(calls).toEqual(['begin', 'prepare', 'take', 'commit', 'result']);
    expect(port.beginUICommand).toHaveBeenCalledExactlyOnceWith(TERMINAL_INTERRUPT_COMMAND);
    expect(port.prepareTerminalInterruptCommand).toHaveBeenCalledExactlyOnceWith(
      'terminal-ticket', 'workspace-a', 'tab-a', 'session-a', 'entry-a',
    );
    expect(port.commitBackendCommand).toHaveBeenCalledExactlyOnceWith('terminal-ticket', 'terminal-handoff');
  });

  it('cria sessão na aba capturada sem substituir o alvo durante o pipeline', async () => {
    const target = setup().captureWithCommand(TERMINAL_SESSION_CREATE_COMMAND)!;
    const { port, calls } = fakePort();

    await expect(executeTerminalSessionOperation(port, target)).resolves.toBe('succeeded');
    expect(calls).toEqual(['begin', 'prepare-session', 'take', 'commit', 'result']);
    expect(port.beginUICommand).toHaveBeenCalledExactlyOnceWith(TERMINAL_SESSION_CREATE_COMMAND);
    expect(port.prepareTerminalSessionCommand).toHaveBeenCalledExactlyOnceWith(
      'terminal-ticket', 'workspace-a', 'tab-a', 'session-a',
    );
  });

  it('cria sessão e substitui vínculo órfão capturado mesmo sem sessão listada', async () => {
    const fixture = setup();
    const workspace = useWorkspaceStore.getState().workspace!;
    useWorkspaceStore.setState({
      workspace: {
        ...workspace,
        tabs: [{ ...workspace.tabs[0], state: { sessionId: 'session-dangling' } }],
      },
    });
    useTerminalStore.setState({ sessions: [], historyBySession: {}, activeEntryBySession: {} });

    const target = fixture.captureWithCommand(TERMINAL_SESSION_CREATE_COMMAND)!;
    expect(target.session).toBeNull();
    expect(target.binding.sessionId).toBe('session-dangling');
    const { port } = fakePort();

    await expect(executeTerminalSessionOperation(port, target)).resolves.toBe('succeeded');
    expect(port.prepareTerminalSessionCommand).toHaveBeenCalledExactlyOnceWith(
      'terminal-ticket', 'workspace-a', 'tab-a', 'session-dangling',
    );
  });

  it('retorna cancelamento autoritativo quando Take rejeita a decisão sem commit', async () => {
    const target = setup().captureWithCommand(TERMINAL_SESSION_CLOSE_COMMAND)!;
    const { port, calls } = fakePort({
      takeUICommand: vi.fn(async () => { calls.push('take'); throw new Error('decision cancelled'); }),
      getUICommandResult: vi.fn(async () => {
        calls.push('result');
        return { invocationId: reservation.invocationId, status: 'cancelled' as const };
      }),
    });

    await expect(executeTerminalSessionOperation(port, target)).resolves.toBe('cancelled');
    expect(port.commitBackendCommand).not.toHaveBeenCalled();
    expect(calls).toContain('result');
  });

  it('não trata Begin como sucesso e não repete após resposta incerta do commit', async () => {
    const target = setup().capture()!;
    const { port, calls } = fakePort({
      getUICommandResult: vi.fn(async () => ({ invocationId: reservation.invocationId, status: 'failed' as const })),
    });
    await expect(executeTerminalInterrupt(port, target)).resolves.toBe('failed');
    expect(port.prepareTerminalInterruptCommand).toHaveBeenCalledOnce();
    expect(calls).toEqual(['begin', 'prepare', 'take', 'commit']);

    const secondTarget = setup('terminal-instance-b').capture()!;
    const uncertain = fakePort({ commitBackendCommand: vi.fn(async () => { throw new Error('lost'); }) });
    await expect(executeTerminalInterrupt(uncertain.port, secondTarget)).resolves.toBe('outcome_unknown');
    expect(uncertain.port.commitBackendCommand).toHaveBeenCalledTimes(1);
    expect(uncertain.port.getUICommandResult).not.toHaveBeenCalled();
  });

  it('aceita BeginLocalCommandUIKey sobrescrito no port sem trocar o comando', async () => {
    const target = setup().capture()!;
    const { port, calls } = fakePort({
      beginUICommand: vi.fn(async commandID => {
        calls.push('begin-local');
        return { ...reservation, commandId: commandID };
      }),
    });

    await expect(executeTerminalInterrupt(port, target)).resolves.toBe('succeeded');
    expect(calls).toEqual(['begin-local', 'prepare', 'take', 'commit', 'result']);
    expect(port.commitBackendCommand).toHaveBeenCalledOnce();
  });

  it('falha fechado sem a ponte de preparação e não reserva Begin', async () => {
    const target = setup().capture()!;
    const { port } = fakePort();
    const prepareless = { ...port, prepareTerminalInterruptCommand: undefined } as unknown as TerminalOperationPort;

    await expect(executeTerminalInterrupt(prepareless, target)).resolves.toBe('failed');
    expect(port.beginUICommand).not.toHaveBeenCalled();
  });

  it('não faz Take quando Prepare rejeita o alvo capturado', async () => {
    const target = setup().capture()!;
    const { port } = fakePort({
      prepareTerminalInterruptCommand: vi.fn(async () => { throw new Error('target-mismatch'); }),
    });

    await expect(executeTerminalInterrupt(port, target)).resolves.toBe('failed');
    expect(port.prepareTerminalInterruptCommand).toHaveBeenCalledOnce();
    expect(port.takeUICommand).not.toHaveBeenCalled();
    expect(port.cancelUICommand).toHaveBeenCalledExactlyOnceWith('terminal-ticket');
  });

  it('cancela o handoff quando a aba muda antes do commit', async () => {
    const fixture = setup();
    const target = fixture.capture()!;
    const { port, calls } = fakePort({
      takeUICommand: vi.fn(async () => {
        calls.push('take');
        const workspace = useWorkspaceStore.getState().workspace!;
        useWorkspaceStore.setState({ workspace: { ...workspace, activeTabId: 'other-tab' } });
        return handoff;
      }),
    });

    await expect(executeTerminalInterrupt(port, target)).resolves.toBe('cancelled');
    expect(calls).toEqual(['begin', 'prepare', 'take', 'complete']);
    expect(port.commitBackendCommand).not.toHaveBeenCalled();
  });
});
