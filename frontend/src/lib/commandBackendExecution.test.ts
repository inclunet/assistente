import { afterEach, describe, expect, it, vi } from 'vitest';

const stores = vi.hoisted(() => {
  const authListeners = new Set<() => void>();
  const workspaceListeners = new Set<() => void>();
  return {
    auth: { isAuthenticated: true, user: { userId: 'u', sessionId: 's', role: 'user' } },
    workspace: { workspace: { id: 'w', profile: 'p', tabs: [], activeTabId: null } },
    authListeners, workspaceListeners,
  };
});
vi.mock('../store/authStore', () => ({ useAuthStore: { getState: () => stores.auth, subscribe: (listener: () => void) => { stores.authListeners.add(listener); return () => stores.authListeners.delete(listener); } } }));
vi.mock('../store/workspaceStore', () => ({ useWorkspaceStore: { getState: () => stores.workspace, subscribe: (listener: () => void) => { stores.workspaceListeners.add(listener); return () => stores.workspaceListeners.delete(listener); } } }));
import { createCommandBackendExecution } from './commandBackendExecution';
import { createTrustedCommandContextSession } from './commandContextSession';

const result = (status = 'succeeded', output: unknown = { kind: 'workspace.list', workspaces: [] }) => ({ invocationId: '01900000-0000-7000-8000-000000000002', status, output });
function ready(): void { vi.spyOn(document, 'hasFocus').mockReturnValue(true); const b = document.createElement('button'); document.body.appendChild(b); b.focus(); }

describe('commandBackendExecution', () => {
  let disposeSession: (() => void) | undefined;
  afterEach(() => { disposeSession?.(); disposeSession = undefined; stores.auth.isAuthenticated = true; stores.auth.user = { userId: 'u', sessionId: 's', role: 'user' }; document.body.replaceChildren(); vi.restoreAllMocks(); });

  function realSession(surfaceID = 'surface-a') {
    const session = createTrustedCommandContextSession();
    const cleanup = session.registerSurfaceContext(surfaceID, () => ({
      surfaceType: 'editor', surfaceId: surfaceID, snapshotVersion: `${surfaceID}-v1`,
    }));
    disposeSession = () => { cleanup(); session.dispose(); };
    return session;
  }
  it.each(['layer.activate', 'layer.toggle', 'layer.back'])('reconhece %s sem repetir efeito na interface', async (command) => {
    ready();
    const port = { executeCommand: vi.fn(async () => ({ invocationId: 'invocation-layer', status: 'succeeded' })) };
    const apply = vi.fn(() => undefined);
    const executor = createCommandBackendExecution(port);
    await expect(executor.execute(command, apply)).resolves.toMatchObject({ presented: true, execution: { status: 'succeeded' } });
    expect(port.executeCommand).toHaveBeenCalledExactlyOnceWith(command, {});
    expect(apply).not.toHaveBeenCalled();
    executor.dispose();
  });
  it('não anuncia ativação após perda do contexto nem transforma recusa em sucesso', async () => {
    ready();
    const port = { executeCommand: vi.fn(async () => {
      window.dispatchEvent(new Event('blur'));
      return { invocationId: 'invocation-layer', status: 'succeeded' };
    }) };
    const executor = createCommandBackendExecution(port);
    await expect(executor.execute('layer.toggle', vi.fn(() => undefined))).resolves.toMatchObject({ presented: false, reason: 'context-stale' });
    executor.dispose();
    ready();
    const denied = createCommandBackendExecution({ executeCommand: vi.fn(async () => ({ invocationId: 'invocation-denied', status: 'denied' })) });
    await expect(denied.execute('layer.activate', vi.fn(() => undefined))).resolves.toMatchObject({ presented: false, execution: { status: 'denied' } });
    denied.dispose();
  });
  it('apresenta uma lista válida uma vez e preserva a execução', async () => {
    ready(); const port = { executeCommand: vi.fn(async () => result()) }; const apply = vi.fn(() => undefined);
    const e = createCommandBackendExecution(port); await expect(e.execute('workspace.list', apply)).resolves.toMatchObject({ presented: true, execution: { status: 'succeeded' } }); expect(apply).toHaveBeenCalledTimes(1); e.dispose();
  });
  it('descarta resposta após mudança de foco sem adulterar status', async () => {
    ready(); const port = { executeCommand: vi.fn(async () => { window.dispatchEvent(new Event('blur')); return result(); }) }; const apply = vi.fn(() => undefined); const e = createCommandBackendExecution(port);
    await expect(e.execute('workspace.list', apply)).resolves.toMatchObject({ presented: false, execution: { status: 'succeeded' } }); expect(apply).not.toHaveBeenCalled(); e.dispose();
  });
  it('não apresenta replay, payload inválido ou comando não correspondente', async () => {
    ready(); const port = { executeCommand: vi.fn(async () => result('outcome_unknown', { kind: 'workspace.list', workspaces: [] })) }; const apply = vi.fn(() => undefined); const e = createCommandBackendExecution(port);
    await expect(e.execute('workspace.list', apply)).resolves.toMatchObject({ presented: false, execution: { status: 'outcome_unknown' } }); expect(apply).not.toHaveBeenCalled(); e.dispose();
  });
  it('deduplica pendência e não repete aplicação', async () => {
    ready(); let resolve!: (v: unknown) => void; const port = { executeCommand: vi.fn(() => new Promise(r => { resolve = r; })) }; const apply = vi.fn(() => undefined); const e = createCommandBackendExecution(port); const a = e.execute('workspace.list', apply); const b = e.execute('workspace.list', apply); expect(a).toBe(b); resolve(result()); await a; expect(apply).toHaveBeenCalledTimes(1); e.dispose();
  });
  it('cancela somente a apresentação pendente sem cancelar o backend', async () => {
    ready(); let resolve!: (v: unknown) => void; const port = { executeCommand: vi.fn(() => new Promise(r => { resolve = r; })) }; const apply = vi.fn(() => undefined); const e = createCommandBackendExecution(port);
    const pending = e.execute('workspace.list', apply); expect(e.execute('workspace.list', apply)).toBe(pending); e.cancelPresentation(); resolve(result());
    await expect(pending).resolves.toMatchObject({ presented: false, reason: 'context-stale', execution: { status: 'succeeded' } }); expect(apply).not.toHaveBeenCalled(); e.dispose();
  });
  it('não lança com resultado malformado e captura throw do apply', async () => {
    ready(); const port = { executeCommand: vi.fn().mockResolvedValueOnce(null).mockResolvedValueOnce(result()) }; const apply = vi.fn(() => { throw new Error('render'); }); const e = createCommandBackendExecution(port);
    await expect(e.execute('workspace.list', apply)).resolves.toMatchObject({ presented: false }); await expect(e.execute('workspace.list', apply)).resolves.toMatchObject({ execution: { status: 'succeeded' }, presented: false }); expect(apply).toHaveBeenCalledTimes(1); e.dispose();
  });
  it('recusa comando desconhecido antes de chamar a porta', async () => {
    ready(); const port = { executeCommand: vi.fn(async () => result()) }; const e = createCommandBackendExecution(port);
    await expect(e.execute('workspace.other', vi.fn(() => undefined))).resolves.toMatchObject({ presented: false, reason: 'unsupported-command' });
    expect(port.executeCommand).not.toHaveBeenCalled(); e.dispose();
  });
  it('não chama o backend quando a superfície exigida não tem provider', async () => {
    ready(); const session = createTrustedCommandContextSession(); disposeSession = () => session.dispose();
    const port = { executeCommand: vi.fn(async () => result()) };
    const e = createCommandBackendExecution(port, { trustedSession: session, surfaceID: 'surface-a' });

    await expect(e.execute('workspace.list', vi.fn(() => undefined))).resolves.toMatchObject({
      presented: false, reason: 'context-stale', execution: { status: 'outcome_unknown' },
    });
    expect(port.executeCommand).not.toHaveBeenCalled();
    e.dispose();
  });
  it('preserva o status do backend e não apresenta após troca da superfície durante await', async () => {
    ready(); let resolve!: (value: unknown) => void;
    const session = realSession();
    const port = { executeCommand: vi.fn(() => new Promise(r => { resolve = r; })) };
    const apply = vi.fn(() => undefined);
    const e = createCommandBackendExecution(port, { trustedSession: session, surfaceID: 'surface-a' });
    const pending = e.execute('workspace.list', apply);

    // A superfície capturada deixa de existir antes do resultado chegar.
    session.dispose();
    resolve(result());
    await expect(pending).resolves.toMatchObject({
      presented: false, reason: 'context-stale', execution: { status: 'succeeded' },
    });
    expect(apply).not.toHaveBeenCalled();
    e.dispose();
    disposeSession = undefined;
  });
  it('recusa snapshot de superfície alterado durante await sem notify', async () => {
    ready(); let resolve!: (value: unknown) => void;
    const session = createTrustedCommandContextSession();
    let snapshotVersion = 'surface-a-v1';
    const cleanup = session.registerSurfaceContext('surface-a', () => ({
      surfaceType: 'editor', surfaceId: 'surface-a', snapshotVersion,
    }));
    disposeSession = () => { cleanup(); session.dispose(); };
    const port = { executeCommand: vi.fn(() => new Promise(r => { resolve = r; })) };
    const apply = vi.fn(() => undefined);
    const e = createCommandBackendExecution(port, { trustedSession: session, surfaceID: 'surface-a' });
    const pending = e.execute('workspace.list', apply);

    snapshotVersion = 'surface-a-v2';
    resolve(result());
    await expect(pending).resolves.toMatchObject({
      presented: false, reason: 'context-stale', execution: { status: 'succeeded' },
    });
    expect(apply).not.toHaveBeenCalled();
    e.dispose();
  });
  it('apresenta usando a surface registrada na sessão compartilhada', async () => {
    ready(); const session = createTrustedCommandContextSession();
    const reads = vi.fn(() => ({
      surfaceType: 'editor' as const, surfaceId: 'surface-a', snapshotVersion: 'surface-a-v1',
    }));
    const cleanup = session.registerSurfaceContext('surface-b', () => ({
      surfaceType: 'editor', surfaceId: 'surface-b', snapshotVersion: 'surface-b-v1',
    }));
    const observedCleanup = session.registerSurfaceContext('surface-a', reads);
    disposeSession = () => { cleanup(); observedCleanup(); session.dispose(); };
    const port = { executeCommand: vi.fn(async () => result()) };
    const apply = vi.fn(() => undefined);
    const e = createCommandBackendExecution(port, { trustedSession: session, surfaceID: 'surface-b' });

    await expect(e.execute('workspace.list', apply)).resolves.toMatchObject({ presented: true, execution: { status: 'succeeded' } });
    expect(reads).not.toHaveBeenCalled();
    expect(apply).toHaveBeenCalledTimes(1);
    cleanup();
    e.dispose();
  });
  it('não dispõe uma sessão emprestada ao encerrar o executor', () => {
    ready(); const session = realSession();
    const e = createCommandBackendExecution({ executeCommand: vi.fn(async () => result()) }, { trustedSession: session, surfaceID: 'surface-a' });

    e.dispose();
    expect(session.readOwnedCommandContextFrame('surface-a')).toBeDefined();
    session.dispose();
    disposeSession = undefined;
  });
  it.each(['logout', 'session-aba', 'workspace-aba', 'dispose'])('descarta resultado após %s sem repetir o backend', async (change) => {
    ready();
    let resolve!: (value: unknown) => void;
    const port = { executeCommand: vi.fn(() => new Promise(r => { resolve = r; })) };
    const e = createCommandBackendExecution(port);
    const apply = vi.fn(() => undefined);
    const pending = e.execute('workspace.list', apply);
    if (change === 'logout') {
      stores.auth.isAuthenticated = false;
      stores.authListeners.forEach(listener => listener());
    } else if (change === 'session-aba') {
      stores.auth.user.sessionId = 'other-session';
      stores.authListeners.forEach(listener => listener());
      stores.auth.user.sessionId = 's';
      stores.authListeners.forEach(listener => listener());
    } else if (change === 'workspace-aba') {
      stores.workspace.workspace.id = 'other-workspace';
      stores.workspaceListeners.forEach(listener => listener());
      stores.workspace.workspace.id = 'w';
      stores.workspaceListeners.forEach(listener => listener());
    } else {
      e.dispose();
    }
    resolve(result());
    await expect(pending).resolves.toMatchObject({ presented: false, execution: { status: 'succeeded' } });
    expect(apply).not.toHaveBeenCalled();
    expect(port.executeCommand).toHaveBeenCalledTimes(1);
    e.dispose();
  });
  it.each([
    { kind: 'workspace.list', workspaces: [], path: 'private' },
    { kind: 'workspace.list', workspaces: [{ id: 'a', name: 'A', profile: '', tab_count: 0, is_active: true, path: 'private' }] },
    { kind: 'workspace.list', workspaces: [
      { id: 'a', name: 'A', profile: '', tab_count: 0, is_active: false },
      { id: 'a', name: 'B', profile: '', tab_count: 0, is_active: false },
    ] },
    { kind: 'workspace.list', workspaces: [
      { id: 'a', name: 'A', profile: '', tab_count: 0, is_active: true },
      { id: 'b', name: 'B', profile: '', tab_count: 0, is_active: true },
    ] },
    null,
    { kind: 'different.command', workspaces: [] },
  ])('recusa payload inválido sem apresentar: %j', async (output) => {
    ready();
    const port = { executeCommand: vi.fn(async () => result('succeeded', output)) };
    const e = createCommandBackendExecution(port);
    const apply = vi.fn(() => undefined);
    await expect(e.execute('workspace.list', apply)).resolves.toMatchObject({ presented: false, reason: 'missing-output', execution: { status: 'succeeded' } });
    expect(apply).not.toHaveBeenCalled();
    expect(port.executeCommand).toHaveBeenCalledTimes(1);
    e.dispose();
  });
});
