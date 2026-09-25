import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const ports = vi.hoisted(() => ({
  get: vi.fn(), list: vi.fn(), activate: vi.fn(), switch: vi.fn(), add: vi.fn(),
  bridge: vi.fn(), announce: vi.fn(),
  events: new Map<string, (payload: unknown) => void>(),
  auth: { isAuthenticated: true, user: { userId: 'user', sessionId: 'initial' } },
  authListeners: new Set<(auth: { isAuthenticated: boolean; user: { userId: string; sessionId: string } }) => void>(),
}));
vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({
  GetActiveWorkspace: ports.get, ListWorkspaces: ports.list, SetActiveWorkspaceTab: ports.activate,
  SwitchWorkspace: ports.switch, AddWorkspaceTab: ports.add,
  CreateWorkspace: vi.fn(), RenameWorkspace: vi.fn(), DeleteWorkspace: vi.fn(),
  SetWorkspaceProfile: vi.fn(), RemoveWorkspaceTab: vi.fn(), UpdateWorkspaceTab: vi.fn(),
  ReorderWorkspaceTabs: vi.fn(), MoveWorkspaceTabTo: vi.fn(), ExportWorkspace: vi.fn(), ImportWorkspace: vi.fn(),
}));
vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (name: string, callback: (payload: unknown) => void) => {
    ports.events.set(name, callback);
    return () => { ports.events.delete(name); };
  },
}));
vi.mock('../../wailsjs/go/models', () => ({ workspace: { Tab: class {
  constructor(data: Record<string, unknown>) { Object.assign(this, data); }
} } }));
vi.mock('../lib/waitForWailsBridge', () => ({ waitForWailsBridge: ports.bridge }));
vi.mock('../lib/modalRegistry', () => ({ isModalOpen: () => false }));
vi.mock('../hooks/useAnnouncer', () => ({ announce: ports.announce }));
vi.mock('../lib/workspaceNavigationWails', () => ({
  setActiveWorkspaceTabForWorkspace: ports.activate,
}));
vi.mock('./authStore', () => ({ useAuthStore: {
  getState: () => ports.auth,
  subscribe: (callback: (auth: typeof ports.auth) => void) => {
    ports.authListeners.add(callback);
    return () => ports.authListeners.delete(callback);
  },
} }));

import { useWorkspaceStore } from './workspaceStore';

function snapshot(sequence: number, id = 'a', active = 'one', epoch = 'epoch') {
  return { id, name: id, snapshot_epoch: epoch, snapshot_sequence: String(sequence), tabs: {
    active, items: ['one', 'two', 'three'].map((tab, position) => ({ id: tab, type: 'chat', title: tab, position })),
  } };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
function session(id: string) {
  ports.auth = { isAuthenticated: true, user: { userId: 'user', sessionId: id } };
  ports.authListeners.forEach((listener) => listener(ports.auth));
}
const flush = async () => { for (let n = 0; n < 12; n++) await Promise.resolve(); };
let cleanup: (() => void) | undefined;
let testSession = 0;
beforeEach(() => {
  vi.clearAllMocks();
  ports.get.mockReset().mockResolvedValue(snapshot(1));
  ports.list.mockReset().mockResolvedValue([]);
  ports.activate.mockReset().mockResolvedValue(undefined);
  ports.switch.mockReset();
  ports.add.mockReset();
  ports.bridge.mockReset().mockResolvedValue(undefined);
  ports.events.clear();
  session(`case-${++testSession}`);
});
afterEach(async () => { cleanup?.(); cleanup = undefined; await flush(); });
async function start() {
  await useWorkspaceStore.getState().initialize();
  cleanup = useWorkspaceStore.getState().setupEventListeners();
  await flush();
}

describe('snapshots produtivos e requisições concorrentes', () => {
  it('preserva estado mais novo quando um evento de B chega antes do retorno da troca', async () => {
    await start();
    const reply = deferred<ReturnType<typeof snapshot>>();
    ports.switch.mockReturnValueOnce(reply.promise);
    const pending = useWorkspaceStore.getState().switchWorkspace('b');
    ports.events.get('workspace:tab_added')?.(snapshot(3, 'b', 'three'));
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('three');
    reply.resolve(snapshot(2, 'b', 'two'));
    await pending;
    expect(useWorkspaceStore.getState().workspace).toMatchObject({ id: 'b', activeTabId: 'three' });
  });

  it('payload mais novo inválido não envenena a sequência aceita', async () => {
    await start();
    ports.events.get('workspace:tab_added')?.({ ...snapshot(100), tabs: { items: {}, active: 'one' } });
    ports.events.get('workspace:tab_navigated')?.(snapshot(2, 'a', 'two'));
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('two');
  });

  it.each(['resolve', 'reject'] as const)('resposta antiga de bootstrap (%s) não termina a inicialização da sessão nova', async (outcome) => {
    const old = deferred<ReturnType<typeof snapshot>>();
    const fresh = deferred<ReturnType<typeof snapshot>>();
    ports.get.mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise);
    const oldRun = useWorkspaceStore.getState().initialize();
    await flush();
    expect(ports.get).toHaveBeenCalledTimes(1);
    session('replacement');
    const freshRun = useWorkspaceStore.getState().initialize();
    await flush();
    expect(ports.get).toHaveBeenCalledTimes(2);
    if (outcome === 'resolve') old.resolve(snapshot(99, 'old', 'three', 'old-epoch'));
    else old.reject(new Error('old request failed'));
    await oldRun;
    expect(useWorkspaceStore.getState().isInitialized).toBe(false);
    expect(useWorkspaceStore.getState().workspace).toBeNull();
    fresh.resolve(snapshot(1, 'fresh', 'one', 'fresh-epoch'));
    await freshRun;
    expect(useWorkspaceStore.getState().workspace?.id).toBe('fresh');
    expect(useWorkspaceStore.getState().isInitialized).toBe(true);
  });

  it('resposta antiga de reconciliação não entra na sessão seguinte', async () => {
    await useWorkspaceStore.getState().initialize();
    const old = deferred<ReturnType<typeof snapshot>>();
    ports.get.mockReturnValueOnce(old.promise);
    cleanup = useWorkspaceStore.getState().setupEventListeners();
    session('reconcile-replacement');
    ports.get.mockResolvedValue(snapshot(1, 'fresh', 'one', 'fresh-epoch'));
    await useWorkspaceStore.getState().initialize();
    old.resolve(snapshot(999, 'old', 'three'));
    await flush();
    expect(useWorkspaceStore.getState().workspace?.id).toBe('fresh');
  });

  it('mantém resposta visual imediata e persiste duas ativações coalescidas', async () => {
    await start();
    const first = deferred<ReturnType<typeof snapshot>>();
    const second = deferred<ReturnType<typeof snapshot>>();
    ports.activate.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    useWorkspaceStore.getState().setActiveTab('two');
    useWorkspaceStore.getState().setActiveTab('three');
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('three');
    expect(ports.activate.mock.calls).toEqual([['a', 'two']]);
    first.resolve(snapshot(2, 'a', 'two'));
    await vi.waitFor(() => expect(ports.activate.mock.calls).toEqual([['a', 'two'], ['a', 'three']]));
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('three');
    second.resolve(snapshot(3, 'a', 'three'));
    await flush();
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('three');
  });

  it('duas falhas consecutivas reconciliam para a aba persistida, não para a intenção intermediária', async () => {
    await start();
    const first = deferred<ReturnType<typeof snapshot>>();
    const second = deferred<ReturnType<typeof snapshot>>();
    ports.activate.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    useWorkspaceStore.getState().setActiveTab('two');
    useWorkspaceStore.getState().setActiveTab('three');
    first.reject(new Error('first failure'));
    await vi.waitFor(() => expect(ports.activate).toHaveBeenCalledWith('a', 'three'));
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('three');
    ports.get.mockResolvedValue(snapshot(3, 'a', 'one'));
    second.reject(new Error('second failure'));
    await vi.waitFor(() => expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('one'));
  });

  it('não envia a intenção enfileirada depois de A→B→A', async () => {
    await start();
    const first = deferred<ReturnType<typeof snapshot>>();
    ports.activate.mockReturnValueOnce(first.promise);
    useWorkspaceStore.getState().setActiveTab('two');
    useWorkspaceStore.getState().setActiveTab('three');
    ports.events.get('workspace:switched')?.(snapshot(2, 'b'));
    ports.events.get('workspace:switched')?.(snapshot(3, 'a'));
    ports.get.mockResolvedValue(snapshot(4, 'a'));
    first.resolve(snapshot(2, 'b', 'two'));
    await flush();
    expect(ports.activate.mock.calls).toEqual([['a', 'two']]);
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('one');
  });
});
