import { describe, it, expect, vi, beforeEach } from 'vitest';

const { mockedAnnounce, mockedIsModalOpen } = vi.hoisted(() => ({
  mockedAnnounce: vi.fn(),
  mockedIsModalOpen: vi.fn(() => false),
}));

vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({
  GetActiveWorkspace: vi.fn(),
  ListWorkspaces: vi.fn(),
  CreateWorkspace: vi.fn(),
  SwitchWorkspace: vi.fn(),
  RenameWorkspace: vi.fn(),
  DeleteWorkspace: vi.fn(),
  SetWorkspaceProfile: vi.fn(),
  AddWorkspaceTab: vi.fn(),
  RemoveWorkspaceTab: vi.fn(),
  SetActiveWorkspaceTab: vi.fn(),
  UpdateWorkspaceTab: vi.fn().mockResolvedValue(undefined),
  ReorderWorkspaceTabs: vi.fn(),
  MoveWorkspaceTabTo: vi.fn(),
  ExportWorkspace: vi.fn(),
  ImportWorkspace: vi.fn(),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(() => vi.fn()),
}));

vi.mock('../../wailsjs/go/models', () => ({
  workspace: { Tab: class { constructor(data: Record<string, unknown>) { Object.assign(this, data); } } },
}));

vi.mock('../hooks/useAnnouncer', () => ({
  announce: mockedAnnounce,
}));

vi.mock('../lib/modalRegistry', () => ({
  isModalOpen: mockedIsModalOpen,
}));

vi.mock('../lib/waitForWailsBridge', () => ({
  waitForWailsBridge: vi.fn(),
}));

vi.mock('../lib/workspaceNavigationWails', () => ({
  setActiveWorkspaceTabForWorkspace: vi.fn(),
}));

import {
  useWorkspaceStore,
  registerTabRenameHandler,
  flushWorkspaceNavigation,
  type WorkspaceTabUpdatedEvent,
} from './workspaceStore';
import { GetActiveWorkspace, ListWorkspaces, UpdateWorkspaceTab } from '@wailsjs/go/wailsapi/Workspace';
import { setActiveWorkspaceTabForWorkspace } from '../lib/workspaceNavigationWails';
import { waitForWailsBridge } from '../lib/waitForWailsBridge';
import { workspace } from '../../wailsjs/go/models';
import { EventsOn } from '@wailsjs/runtime/runtime';
import i18next from 'i18next';
import { useAuthStore } from './authStore';

const mockedGetActiveWorkspace = vi.mocked(GetActiveWorkspace);
const mockedListWorkspaces = vi.mocked(ListWorkspaces);
const mockedSetActiveWorkspaceTab = vi.mocked(setActiveWorkspaceTabForWorkspace);
const mockedUpdateWorkspaceTab = vi.mocked(UpdateWorkspaceTab);
const mockedWaitForWailsBridge = vi.mocked(waitForWailsBridge);
const mockedEventsOn = vi.mocked(EventsOn);

function versioned<T extends Record<string, unknown>>(payload: T, sequence = '1'): T & Record<string, unknown> {
  return { ...payload, snapshot_epoch: 'epoch-test', snapshot_sequence: sequence };
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function setStoreState(
  tabs: Array<{ id: string; type: string; conversationId?: string; state?: Record<string, unknown>; profileOverride?: Record<string, unknown>; title: string; position: number }>,
  activeTabId: string,
) {
  useWorkspaceStore.setState({
    workspace: {
      id: 'ws-1',
      name: 'Test',
      tabs: tabs.map(t => ({
        id: t.id,
        type: t.type as 'chat' | 'editor' | 'terminal' | 'tasklist',
        conversationId: t.conversationId,
        state: t.state,
        profileOverride: t.profileOverride,
        title: t.title,
        position: t.position,
      })),
      activeTabId,
    },
    isInitialized: true,
    workspaces: [],
  });
}

describe('eventos de workspace', () => {
  beforeEach(async () => {
    useWorkspaceStore.setState({ workspace: null, isInitialized: false, workspaces: [] });
    mockedAnnounce.mockClear();
    mockedEventsOn.mockClear();
    mockedGetActiveWorkspace.mockReset().mockResolvedValue(versioned({
      id: 'ws-1', name: 'Test', tabs: {
        active: 'tab-1', items: [{ id: 'tab-1', type: 'chat', title: 'Chat', position: 0 }],
      },
    }, '3') as never);
    mockedListWorkspaces.mockReset().mockResolvedValue([]);
    await useWorkspaceStore.getState().initialize();
  });

  it('workspace:created atualiza somente a lista, sem ativar o workspace recebido', async () => {
    const activeBefore = useWorkspaceStore.getState().workspace;
    const listed = [
      { id: 'ws-1', name: 'Test', path: '', profile: '', tab_count: 1, is_active: true },
      { id: 'ws-new', name: 'Novo', path: '', profile: '', tab_count: 1, is_active: false },
    ];
    mockedListWorkspaces.mockResolvedValueOnce(listed);
    const cleanup = useWorkspaceStore.getState().setupEventListeners();
    try {
      const registration = mockedEventsOn.mock.calls.find(([event]) => event === 'workspace:created');
      expect(registration).toBeDefined();
      const handler = registration![1] as (payload: unknown) => void;
      handler(versioned({ id: 'ws-new', name: 'Novo', tabs: {
        active: 'new-tab', items: [{ id: 'new-tab', type: 'chat' }],
      } }, '100'));
      await vi.waitFor(() => expect(useWorkspaceStore.getState().workspaces).toEqual(listed));
      expect(useWorkspaceStore.getState().workspace).toBe(activeBefore);
      expect(mockedAnnounce).not.toHaveBeenCalled();
    } finally {
      cleanup();
    }
  });

  it('workspace:terminal_session_bound aplica o vínculo sem anunciar perfil nem navegar', () => {
    const cleanup = useWorkspaceStore.getState().setupEventListeners();
    try {
      const registration = mockedEventsOn.mock.calls.find(([event]) => event === 'workspace:terminal_session_bound');
      expect(registration).toBeDefined();
      const handler = registration![1] as (payload: unknown) => void;
      handler(versioned({ id: 'ws-1', name: 'Test', tabs: {
        active: 'tab-1', items: [{ id: 'tab-1', type: 'terminal', state: { sessionId: 'session-bound' }, position: 0 }],
      } }, '4'));
      expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-1');
      expect(useWorkspaceStore.getState().workspace?.tabs[0].state?.sessionId).toBe('session-bound');
      expect(mockedAnnounce).not.toHaveBeenCalled();
    } finally { cleanup(); }
  });

  it('workspace:conversation_bound aplica o vínculo sem anunciar perfil nem navegar', () => {
    const cleanup = useWorkspaceStore.getState().setupEventListeners();
    try {
      const registration = mockedEventsOn.mock.calls.find(([event]) => event === 'workspace:conversation_bound');
      expect(registration).toBeDefined();
      const handler = registration![1] as (payload: unknown) => void;
      handler(versioned({ id: 'ws-1', name: 'Test', tabs: {
        active: 'tab-1', items: [{ id: 'tab-1', type: 'editor', conversation_id: 'conversation-bound', position: 0 }],
      } }, '4'));
      expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-1');
      expect(useWorkspaceStore.getState().workspace?.tabs[0].conversationId).toBe('conversation-bound');
      expect(mockedAnnounce).not.toHaveBeenCalled();
    } finally { cleanup(); }
  });

  it('aplica modo do editor por snapshot versionado sem anunciar perfil ou repetir persistência', () => {
    const cleanup = useWorkspaceStore.getState().setupEventListeners();
    try {
      const registration = mockedEventsOn.mock.calls.find(([event]) => event === 'workspace:editor_mode_changed');
      expect(registration).toBeDefined();
      const handler = registration![1] as (payload: unknown) => void;
      const snapshot = (mode: string, version: string) => versioned({ id: 'ws-1', name: 'Test', tabs: {
        active: 'tab-1', items: [{ id: 'tab-1', type: 'editor', state: { displayMode: mode }, position: 0 }],
      } }, version);
      handler({ workspace: snapshot('view', '4'), tabId: 'tab-1', mode: 'view' });
      expect(useWorkspaceStore.getState().workspace?.tabs[0].state?.displayMode).toBe('view');
      handler({ workspace: snapshot('rich', '3'), tabId: 'tab-1', mode: 'rich' });
      expect(useWorkspaceStore.getState().workspace?.tabs[0].state?.displayMode).toBe('view');
      handler({ workspace: snapshot('rich', '5'), tabId: 'tab-1', mode: 'invalid' });
      expect(useWorkspaceStore.getState().workspace?.tabs[0].state?.displayMode).toBe('view');
      expect(mockedAnnounce).not.toHaveBeenCalled();
    } finally { cleanup(); }
  });

  it('reconcilia e anuncia o profile alterado pela tool', () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', title: 'Chat', position: 0 },
    ], 'tab-1');
    const cleanup = useWorkspaceStore.getState().setupEventListeners();
    const registration = mockedEventsOn.mock.calls.find(([event]) => event === 'workspace:tab_updated');
    expect(registration).toBeDefined();

    const handler = registration?.[1] as (payload: WorkspaceTabUpdatedEvent) => void;
    handler({
      workspace: {
        id: 'ws-1',
        name: 'Workspace',
        profile: 'geral',
        tabs: {
          active: 'tab-1',
          items: [{
            id: 'tab-1',
            type: 'chat',
            conversation_id: 'conversation-1',
            title: 'Chat',
            position: 0,
            profile_override: { slug: 'custom' },
          }],
        },
        snapshot_epoch: 'epoch-test',
        snapshot_sequence: '4',
      },
      tabId: 'tab-1',
      profileSlug: 'custom',
    });

    expect(useWorkspaceStore.getState().workspace?.tabs[0]?.profileOverride).toEqual({
      slug: 'custom',
    });
    expect(mockedAnnounce).toHaveBeenCalledWith(
      `${i18next.t('workspace.profileChanged')}: custom`,
    );
    cleanup();
  });

  it('aplica tab_added do mesmo workspace sem perder abas já conhecidas', () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', title: 'Chat', position: 0 },
    ], 'tab-1');
    const cleanup = useWorkspaceStore.getState().setupEventListeners();
    const registration = mockedEventsOn.mock.calls.find(([event]) => event === 'workspace:tab_added');
    const handler = registration?.[1] as (payload: workspace.Workspace) => void;

    handler({
      id: 'ws-1',
      name: 'Test',
      tabs: {
        active: 'tab-1',
        items: [
          { id: 'tab-1', type: 'chat', title: 'Chat', position: 0 },
          { id: 'tab-2', type: 'chat', title: 'Novo chat', position: 1 },
        ],
      },
      snapshot_epoch: 'epoch-test',
      snapshot_sequence: '4',
    } as unknown as workspace.Workspace);

    expect(useWorkspaceStore.getState().workspace?.tabs.map((tab) => tab.id)).toEqual(['tab-1', 'tab-2']);
    cleanup();
  });

  it('ignora tab_added atrasado de outro workspace', () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', title: 'Chat', position: 0 },
    ], 'tab-1');
    const before = useWorkspaceStore.getState().workspace;
    const cleanup = useWorkspaceStore.getState().setupEventListeners();
    const registration = mockedEventsOn.mock.calls.find(([event]) => event === 'workspace:tab_added');
    const handler = registration?.[1] as (payload: workspace.Workspace) => void;

    handler({
      id: 'ws-2',
      name: 'Outro workspace',
      tabs: {
        active: 'other-tab',
        items: [{ id: 'other-tab', type: 'chat', title: 'Outro', position: 0 }],
      },
      snapshot_epoch: 'epoch-test',
      snapshot_sequence: '2',
    } as unknown as workspace.Workspace);

    expect(useWorkspaceStore.getState().workspace).toEqual(before);
    cleanup();
  });

  it('ignora tab_removed de outro workspace', () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', title: 'Chat', position: 0 },
    ], 'tab-1');
    const before = useWorkspaceStore.getState().workspace;
    const cleanup = useWorkspaceStore.getState().setupEventListeners();
    const registration = mockedEventsOn.mock.calls.find(([event]) => event === 'workspace:tab_removed');
    const handler = registration?.[1] as (payload: workspace.Workspace) => void;

    handler({
      id: 'ws-2',
      name: 'Outro workspace',
      tabs: {
        active: 'other-tab',
        items: [{ id: 'other-tab', type: 'chat', title: 'Outro', position: 0 }],
      },
      snapshot_epoch: 'epoch-test',
      snapshot_sequence: '2',
    } as unknown as workspace.Workspace);

    expect(useWorkspaceStore.getState().workspace).toEqual(before);
    cleanup();
  });

  it('ignora tab_updated de outro workspace', () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', title: 'Chat', position: 0 },
    ], 'tab-1');
    const before = useWorkspaceStore.getState().workspace;
    const cleanup = useWorkspaceStore.getState().setupEventListeners();
    const registration = mockedEventsOn.mock.calls.find(([event]) => event === 'workspace:tab_updated');
    const handler = registration?.[1] as (payload: WorkspaceTabUpdatedEvent) => void;

    handler({
      workspace: {
        id: 'ws-2',
        name: 'Outro workspace',
        tabs: {
          active: 'other-tab',
          items: [{ id: 'other-tab', type: 'chat', title: 'Outro', position: 0 }],
        },
        snapshot_epoch: 'epoch-test',
        snapshot_sequence: '2',
      },
      tabId: 'other-tab',
      profileSlug: '',
    });

    expect(useWorkspaceStore.getState().workspace).toEqual(before);
    expect(mockedAnnounce).not.toHaveBeenCalled();
    cleanup();
  });

  it('ordena A/B/A, descarta epoch diferente e torna duplicata idempotente', () => {
    const cleanup = useWorkspaceStore.getState().setupEventListeners();
    const registration = mockedEventsOn.mock.calls.find(([event]) => event === 'workspace:switched');
    const handler = registration?.[1] as (payload: unknown) => void;
    const snapshot = (id: string, sequence: string, epoch = 'epoch-test') => ({
      id,
      name: id,
      snapshot_epoch: epoch,
      snapshot_sequence: sequence,
      tabs: { active: `${id}-tab`, items: [{ id: `${id}-tab`, type: 'chat', title: id, position: 0 }] },
    });

    handler(snapshot('ws-b', '5'));
    expect(useWorkspaceStore.getState().workspace?.id).toBe('ws-b');
    handler(snapshot('ws-a', '4'));
    expect(useWorkspaceStore.getState().workspace?.id).toBe('ws-b');
    handler(snapshot('ws-b', '5'));
    expect(useWorkspaceStore.getState().workspace?.id).toBe('ws-b');
    handler(snapshot('ws-c', '6', 'epoch-new'));
    expect(useWorkspaceStore.getState().workspace?.id).toBe('ws-b');
    cleanup();
  });

});

describe('handleContentRenamed', () => {
  beforeEach(() => {
    useWorkspaceStore.setState({ workspace: null, isInitialized: false, workspaces: [] });
    mockedAnnounce.mockClear();
    mockedIsModalOpen.mockReturnValue(false);
    mockedSetActiveWorkspaceTab.mockReset();
  });

  it('atualiza titulo da aba quando conversa é renomeada', async () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000042", title: 'Nova conversa', position: 0 },
    ], 'tab-1');

    useWorkspaceStore.getState().handleContentRenamed('chat', '01926b90-7a5a-7c4e-8d3f-000000000042', 'Minha conversa');

    // updateTab é async, mas handleContentRenamed dispara void
    await vi.waitFor(() => {
      const tab = useWorkspaceStore.getState().workspace?.tabs[0];
      expect(tab?.title).toBe('Minha conversa');
    });
  });

  it('não atualiza quando titulo já é igual (previne loop)', () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000042", title: 'Mesmo título', position: 0 },
    ], 'tab-1');

    mockedUpdateWorkspaceTab.mockClear();

    useWorkspaceStore.getState().handleContentRenamed('chat', '01926b90-7a5a-7c4e-8d3f-000000000042', 'Mesmo título');

    expect(mockedUpdateWorkspaceTab).not.toHaveBeenCalled();
  });

  it('não atualiza quando tipo não combina', () => {
    setStoreState([
      { id: 'tab-1', type: 'editor', title: 'Doc', position: 0 },
    ], 'tab-1');

    mockedUpdateWorkspaceTab.mockClear();

    useWorkspaceStore.getState().handleContentRenamed('chat', '01926b90-7a5a-7c4e-8d3f-000000000042', 'Novo título');

    expect(mockedUpdateWorkspaceTab).not.toHaveBeenCalled();
  });

  it('atualiza aba correta quando há múltiplas abas', async () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000010", title: 'Chat A', position: 0 },
      { id: 'tab-2', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000020", title: 'Chat B', position: 1 },
      { id: 'tab-3', type: 'editor', title: 'Editor', position: 2 },
    ], 'tab-2');

    useWorkspaceStore.getState().handleContentRenamed('chat', '01926b90-7a5a-7c4e-8d3f-000000000020', 'Chat Renomeado');

    await vi.waitFor(() => {
      const tabs = useWorkspaceStore.getState().workspace?.tabs;
      expect(tabs?.[0]?.title).toBe('Chat A');
      expect(tabs?.[1]?.title).toBe('Chat Renomeado');
      expect(tabs?.[2]?.title).toBe('Editor');
    });
  });
});

describe('renameTabContent + registerTabRenameHandler', () => {
  beforeEach(() => {
    useWorkspaceStore.setState({ workspace: null, isInitialized: false, workspaces: [] });
    mockedAnnounce.mockClear();
    mockedIsModalOpen.mockReturnValue(false);
    mockedSetActiveWorkspaceTab.mockReset();
  });

  it('chama handler registrado ao renomear aba de chat', () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000042", title: 'Chat', position: 0 },
    ], 'tab-1');

    const handler = vi.fn();
    const unregister = registerTabRenameHandler('chat', handler);

    useWorkspaceStore.getState().renameTabContent('tab-1', 'Novo nome');

    expect(handler).toHaveBeenCalledWith('01926b90-7a5a-7c4e-8d3f-000000000042', 'Novo nome');

    unregister();
  });

  it('chama handler de editor com o tabId ao renomear aba de editor', () => {
    setStoreState([
      { id: 'editor-tab', type: 'editor', title: 'Editor', position: 0 },
    ], 'editor-tab');

    const handler = vi.fn();
    const unregister = registerTabRenameHandler('editor', handler);

    useWorkspaceStore.getState().renameTabContent('editor-tab', 'Novo documento');

    expect(handler).toHaveBeenCalledWith('editor-tab', 'Novo documento');

    unregister();
  });

  it('não chama handler quando aba de chat não tem conversationId', () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', title: 'Chat', position: 0 },
    ], 'tab-1');

    const handler = vi.fn();
    const unregister = registerTabRenameHandler('chat', handler);

    useWorkspaceStore.getState().renameTabContent('tab-1', 'Novo nome');

    expect(handler).not.toHaveBeenCalled();

    unregister();
  });

  it('não chama handler quando tipo não tem handler registrado', () => {
    setStoreState([
      { id: 'tab-1', type: 'terminal', state: { sessionId: 'sess-1' }, title: 'Terminal', position: 0 },
    ], 'tab-1');

    const chatHandler = vi.fn();
    const unregister = registerTabRenameHandler('chat', chatHandler);

    useWorkspaceStore.getState().renameTabContent('tab-1', 'Novo nome');

    expect(chatHandler).not.toHaveBeenCalled();

    unregister();
  });

  it('unregister remove o handler', () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000042", title: 'Chat', position: 0 },
    ], 'tab-1');

    const handler = vi.fn();
    const unregister = registerTabRenameHandler('chat', handler);
    unregister();

    useWorkspaceStore.getState().renameTabContent('tab-1', 'Novo nome');

    expect(handler).not.toHaveBeenCalled();
  });
});

describe('setActiveTab', () => {
  beforeEach(() => {
    useWorkspaceStore.setState({ workspace: null, isInitialized: false, workspaces: [] });
    mockedAnnounce.mockClear();
    mockedIsModalOpen.mockReturnValue(false);
    mockedSetActiveWorkspaceTab.mockReset();
  });

  it('bloqueia troca de aba quando há qualquer modal aberto', async () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", title: 'Chat 1', position: 0 },
      { id: 'tab-2', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000002", title: 'Chat 2', position: 1 },
    ], 'tab-1');

    mockedIsModalOpen.mockReturnValue(true);

    useWorkspaceStore.getState().setActiveTab('tab-2');

    expect(mockedSetActiveWorkspaceTab).not.toHaveBeenCalled();
    expect(mockedAnnounce).toHaveBeenCalledTimes(1);
  });

  it('aplica optimistic update imediatamente e persiste em background', async () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", title: 'Chat 1', position: 0 },
      { id: 'tab-2', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000002", title: 'Chat 2', position: 1 },
    ], 'tab-1');

    mockedSetActiveWorkspaceTab.mockResolvedValue(undefined as never);

    // setActiveTab é síncrono (fire-and-forget para backend)
    useWorkspaceStore.getState().setActiveTab('tab-2');

    // Optimistic: UI atualizada imediatamente
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-2');

    // Aguarda backend fire-and-forget completar
    await vi.waitFor(() => {
      expect(mockedSetActiveWorkspaceTab).toHaveBeenCalledWith('ws-1', 'tab-2');
    });
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-2');
  });

  it('reconcilia e faz rollback para o snapshot autoritativo quando a API falha', async () => {
    const rollbackListener = vi.fn();
    window.addEventListener('workspace:tab-activation-rollback', rollbackListener);
    try {
      mockedGetActiveWorkspace.mockReset().mockResolvedValueOnce(versioned({
        id: 'ws-1',
        name: 'Test',
        tabs: {
          active: 'tab-1',
          items: [
            { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
            { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
          ],
        },
      }) as never);
      mockedListWorkspaces.mockResolvedValueOnce([]);
      await useWorkspaceStore.getState().initialize();
      setStoreState([
        { id: 'tab-1', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", title: 'Chat 1', position: 0 },
        { id: 'tab-2', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000002", title: 'Chat 2', position: 1 },
      ], 'tab-1');

      mockedGetActiveWorkspace.mockResolvedValueOnce(versioned({
        id: 'ws-1',
        name: 'Test',
        tabs: {
          active: 'tab-1',
          items: [
            { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
            { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
          ],
        },
      }, '2') as never);
      mockedSetActiveWorkspaceTab.mockRejectedValue(new Error('backend error'));

      // setActiveTab é síncrono; a reconciliação autoritativa ocorre no .catch assíncrono
      useWorkspaceStore.getState().setActiveTab('tab-2');

      await vi.waitFor(() => expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-1'));
      await vi.waitFor(() => expect(mockedAnnounce).toHaveBeenCalled());
      expect(rollbackListener).toHaveBeenCalledWith(expect.objectContaining({
        detail: { failedTabId: 'tab-2', rollbackTabId: 'tab-1' },
      }));
    } finally {
      window.removeEventListener('workspace:tab-activation-rollback', rollbackListener);
    }
  });

  it('remove overlay quando a persistência e a reconciliação falham', async () => {
    mockedGetActiveWorkspace.mockReset().mockResolvedValueOnce(versioned({
      id: 'ws-1', name: 'Test', tabs: {
        active: 'tab-1',
        items: [
          { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
          { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
        ],
      },
    }, '1') as never);
    mockedListWorkspaces.mockResolvedValueOnce([]);
    await useWorkspaceStore.getState().initialize();
    setStoreState([
      { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
      { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
    ], 'tab-1');
    mockedGetActiveWorkspace.mockRejectedValueOnce(new Error('reconcile failed'));
    mockedSetActiveWorkspaceTab.mockRejectedValueOnce(new Error('set failed'));

    useWorkspaceStore.getState().setActiveTab('tab-2');
    await expect(flushWorkspaceNavigation()).resolves.toBe(false);
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-1');
  });

  it('remove overlay quando reconcile retorna snapshot duplicate/older', async () => {
    mockedGetActiveWorkspace.mockReset().mockResolvedValueOnce(versioned({
      id: 'ws-1', name: 'Test', tabs: {
        active: 'tab-1',
        items: [
          { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
          { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
        ],
      },
    }, '2') as never);
    mockedListWorkspaces.mockResolvedValueOnce([]);
    await useWorkspaceStore.getState().initialize();
    setStoreState([
      { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
      { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
    ], 'tab-1');
    mockedGetActiveWorkspace.mockResolvedValueOnce(versioned({
      id: 'ws-1', name: 'Test', tabs: {
        active: 'tab-1',
        items: [
          { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
          { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
        ],
      },
    }, '1') as never);
    mockedSetActiveWorkspaceTab.mockRejectedValueOnce(new Error('set failed'));

    useWorkspaceStore.getState().setActiveTab('tab-2');
    await expect(flushWorkspaceNavigation()).resolves.toBe(false);
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-1');
  });

  it('não reprojeta o anchor antigo quando nova intenção começa durante reconcile', async () => {
    mockedGetActiveWorkspace.mockReset().mockResolvedValueOnce(versioned({
      id: 'ws-1', name: 'Test', tabs: {
        active: 'tab-1',
        items: [
          { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
          { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
          { id: 'tab-3', type: 'chat', title: 'Chat 3', position: 2 },
        ],
      },
    }, '1') as never);
    mockedListWorkspaces.mockResolvedValueOnce([]);
    await useWorkspaceStore.getState().initialize();
    setStoreState([
      { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
      { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
      { id: 'tab-3', type: 'chat', title: 'Chat 3', position: 2 },
    ], 'tab-1');
    const reconcile = deferred<unknown>();
    mockedGetActiveWorkspace.mockReturnValueOnce(reconcile.promise as never);
    mockedSetActiveWorkspaceTab
      .mockRejectedValueOnce(new Error('set failed'))
      .mockResolvedValueOnce(versioned({
        id: 'ws-1', name: 'Test', tabs: {
          active: 'tab-3',
          items: [
            { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
            { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
            { id: 'tab-3', type: 'chat', title: 'Chat 3', position: 2 },
          ],
        },
      }, '2') as never);

    useWorkspaceStore.getState().setActiveTab('tab-2');
    await vi.waitFor(() => expect(mockedGetActiveWorkspace).toHaveBeenCalledTimes(2));
    useWorkspaceStore.getState().setActiveTab('tab-3');
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-3');
    reconcile.resolve(versioned({
      id: 'ws-1', name: 'Test', tabs: {
        active: 'tab-1',
        items: [
          { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
          { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
          { id: 'tab-3', type: 'chat', title: 'Chat 3', position: 2 },
        ],
      },
    }, '1'));
    await vi.waitFor(() => expect(mockedSetActiveWorkspaceTab).toHaveBeenCalledWith('ws-1', 'tab-3'));
    await expect(flushWorkspaceNavigation()).resolves.toBe(false);
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-3');
  });

  it('não faz rollback quando outra troca já ocorreu antes do erro', async () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", title: 'Chat 1', position: 0 },
      { id: 'tab-2', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000002", title: 'Chat 2', position: 1 },
      { id: 'tab-3', type: 'editor', title: 'Editor', position: 2 },
    ], 'tab-1');

    // Primeira troca falha, mas com delay para dar tempo de outra troca
    let rejectFirst: (err: Error) => void;
    mockedSetActiveWorkspaceTab
      .mockImplementationOnce(() => new Promise((_, rej) => { rejectFirst = rej; }) as never)
      .mockResolvedValueOnce(undefined as never);

    // Inicia troca para tab-2 (vai falhar eventualmente)
    useWorkspaceStore.getState().setActiveTab('tab-2');
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-2');

    // Antes do erro, outra troca ocorre para tab-3
    useWorkspaceStore.getState().setActiveTab('tab-3');
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-3');

    // Agora a primeira falha
    rejectFirst!(new Error('backend error'));

    // tab-3 deve permanecer (rollback não deve sobrescrever)
    await vi.waitFor(() => {
      expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-3');
    });
  });

  it('não faz rollback stale quando usuário volta para mesma aba do request falhado', async () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", title: 'Chat 1', position: 0 },
      { id: 'tab-2', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000002", title: 'Chat 2', position: 1 },
      { id: 'tab-3', type: 'editor', title: 'Editor', position: 2 },
    ], 'tab-1');

    let rejectFirst: (err: Error) => void;
    mockedSetActiveWorkspaceTab
      .mockImplementationOnce(() => new Promise((_, rej) => { rejectFirst = rej; }) as never)
      .mockResolvedValueOnce(undefined as never)
      .mockResolvedValueOnce(undefined as never);

    // Ativação A: tab-1 → tab-2 (vai falhar)
    useWorkspaceStore.getState().setActiveTab('tab-2');
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-2');

    // Ativação B: tab-2 → tab-3
    useWorkspaceStore.getState().setActiveTab('tab-3');
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-3');

    // Ativação C: tab-3 → tab-2 (volta para mesma aba de A)
    useWorkspaceStore.getState().setActiveTab('tab-2');
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-2');

    // A falha agora — activeTabId === 'tab-2' (mesma de A), mas seqId é diferente
    rejectFirst!(new Error('backend error'));

    // tab-2 deve permanecer (rollback stale de A não deve sobrescrever C)
    await vi.waitFor(() => {
      expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-2');
    });
    // announce NÃO deve ser chamado (rollback não executou)
    expect(mockedAnnounce).not.toHaveBeenCalled();
  });

  it('não faz rollback quando workspace mudou antes do erro', async () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", title: 'Chat 1', position: 0 },
      { id: 'tab-2', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000002", title: 'Chat 2', position: 1 },
    ], 'tab-1');

    let rejectFirst: (err: Error) => void;
    mockedSetActiveWorkspaceTab
      .mockImplementationOnce(() => new Promise((_, rej) => { rejectFirst = rej; }) as never);

    // Troca para tab-2 (vai falhar eventualmente)
    useWorkspaceStore.getState().setActiveTab('tab-2');
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-2');

    // Simula troca de workspace (id diferente)
    useWorkspaceStore.setState({
      workspace: {
        id: 'ws-2',
        name: 'Outro Workspace',
        tabs: [
          { id: 'tab-a', type: 'chat' as const, conversationId: "01926b90-7a5a-7c4e-8d3f-000000000010", title: 'Chat A', position: 0 },
        ],
        activeTabId: 'tab-a',
      },
    });

    // Falha do backend chega agora — workspace é outro
    rejectFirst!(new Error('backend error'));

    // tab-a deve permanecer (rollback ignorado pois workspace mudou)
    await vi.waitFor(() => {
      expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-a');
    });
    expect(mockedAnnounce).not.toHaveBeenCalled();
  });

  it('ignora quando tabId já é a aba ativa', async () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", title: 'Chat 1', position: 0 },
    ], 'tab-1');

    useWorkspaceStore.getState().setActiveTab('tab-1');

    expect(mockedSetActiveWorkspaceTab).not.toHaveBeenCalled();
  });

  it('coalesce somente pendências ainda não submetidas durante burst CtrlTab', async () => {
    mockedGetActiveWorkspace.mockReset().mockResolvedValueOnce(versioned({
      id: 'ws-1', name: 'Test', tabs: {
        active: 'tab-1',
        items: [
          { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
          { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
          { id: 'tab-3', type: 'chat', title: 'Chat 3', position: 2 },
          { id: 'tab-4', type: 'chat', title: 'Chat 4', position: 3 },
        ],
      },
    }, '1') as never);
    mockedListWorkspaces.mockResolvedValueOnce([]);
    await useWorkspaceStore.getState().initialize();
    setStoreState([
      { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
      { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
      { id: 'tab-3', type: 'chat', title: 'Chat 3', position: 2 },
      { id: 'tab-4', type: 'chat', title: 'Chat 4', position: 3 },
    ], 'tab-1');
    const first = deferred<unknown>();
    const second = deferred<unknown>();
    mockedSetActiveWorkspaceTab
      .mockImplementationOnce(() => first.promise as never)
      .mockImplementationOnce(() => second.promise as never);

    useWorkspaceStore.getState().setActiveTab('tab-2');
    useWorkspaceStore.getState().setActiveTab('tab-3');
    useWorkspaceStore.getState().setActiveTab('tab-4');
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-4');

    await vi.waitFor(() => expect(mockedSetActiveWorkspaceTab).toHaveBeenCalledWith('ws-1', 'tab-2'));
    expect(mockedSetActiveWorkspaceTab).toHaveBeenCalledTimes(1);
    first.resolve(versioned({
      id: 'ws-1', name: 'Test', tabs: {
        active: 'tab-2',
        items: [
          { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
          { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
          { id: 'tab-3', type: 'chat', title: 'Chat 3', position: 2 },
          { id: 'tab-4', type: 'chat', title: 'Chat 4', position: 3 },
        ],
      },
    }, '1'));
    await vi.waitFor(() => expect(mockedSetActiveWorkspaceTab).toHaveBeenCalledWith('ws-1', 'tab-4'));
    expect(mockedSetActiveWorkspaceTab).not.toHaveBeenCalledWith('ws-1', 'tab-3');
    second.resolve(versioned({
      id: 'ws-1', name: 'Test', tabs: {
        active: 'tab-4',
        items: [
          { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
          { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
          { id: 'tab-3', type: 'chat', title: 'Chat 3', position: 2 },
          { id: 'tab-4', type: 'chat', title: 'Chat 4', position: 3 },
        ],
      },
    }, '2'));
    await expect(flushWorkspaceNavigation()).resolves.toBe(true);
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-4');
  });

  it('continua a fila coalescida após duas falhas e faz rollback ao snapshot canônico', async () => {
    mockedGetActiveWorkspace.mockReset().mockResolvedValueOnce(versioned({
      id: 'ws-1', name: 'Test', tabs: {
        active: 'tab-1',
        items: [
          { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
          { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
          { id: 'tab-3', type: 'chat', title: 'Chat 3', position: 2 },
          { id: 'tab-4', type: 'chat', title: 'Chat 4', position: 3 },
        ],
      },
    }, '1') as never);
    mockedListWorkspaces.mockResolvedValueOnce([]);
    await useWorkspaceStore.getState().initialize();
    setStoreState([
      { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
      { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
      { id: 'tab-3', type: 'chat', title: 'Chat 3', position: 2 },
      { id: 'tab-4', type: 'chat', title: 'Chat 4', position: 3 },
    ], 'tab-1');
    const first = deferred<unknown>();
    const second = deferred<unknown>();
    mockedSetActiveWorkspaceTab
      .mockImplementationOnce(() => first.promise as never)
      .mockImplementationOnce(() => second.promise as never);

    useWorkspaceStore.getState().setActiveTab('tab-2');
    useWorkspaceStore.getState().setActiveTab('tab-3');
    useWorkspaceStore.getState().setActiveTab('tab-4');
    first.reject(new Error('first failure'));
    await vi.waitFor(() => expect(mockedSetActiveWorkspaceTab).toHaveBeenCalledWith('ws-1', 'tab-4'));
    second.reject(new Error('second failure'));
    await expect(flushWorkspaceNavigation()).resolves.toBe(false);
    expect(mockedSetActiveWorkspaceTab).toHaveBeenCalledTimes(2);
    expect(mockedSetActiveWorkspaceTab).not.toHaveBeenCalledWith('ws-1', 'tab-3');
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-1');
  });

  it('desfaz seleção otimista quando confirmação inválida e reconciliação falham', async () => {
    const tabs = [
      { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
      { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
    ];
    mockedGetActiveWorkspace.mockReset().mockResolvedValueOnce(versioned({
      id: 'ws-1', name: 'Test', tabs: { active: 'tab-1', items: tabs },
    }, '1') as never);
    mockedListWorkspaces.mockResolvedValueOnce([]);
    await useWorkspaceStore.getState().initialize();
    mockedGetActiveWorkspace.mockRejectedValueOnce(new Error('read failed'));
    mockedSetActiveWorkspaceTab.mockResolvedValueOnce({ id: 'ws-1' } as never);

    useWorkspaceStore.getState().setActiveTab('tab-2');
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-2');
    await expect(flushWorkspaceNavigation()).resolves.toBe(false);
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-1');
    expect(mockedAnnounce).toHaveBeenCalledWith(expect.stringContaining('workspace.tabSwitchFailed'));
  });

  it.each([
    ['tab-1', true],
    ['tab-2', false],
  ] as const)('nova tentativa após falha confirma seleção real %s sem reutilizar alvo antigo', async (confirmedTab, expectedResult) => {
    const tabs = [
      { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
      { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
    ];
    mockedGetActiveWorkspace.mockReset().mockResolvedValueOnce(versioned({
      id: 'ws-1', name: 'Test', tabs: { active: 'tab-1', items: tabs },
    }, '1') as never);
    mockedListWorkspaces.mockResolvedValueOnce([]);
    await useWorkspaceStore.getState().initialize();
    mockedGetActiveWorkspace.mockRejectedValueOnce(new Error('reconciliation unavailable'));
    mockedSetActiveWorkspaceTab.mockRejectedValueOnce(new Error('acknowledgement lost'));
    useWorkspaceStore.getState().setActiveTab('tab-2');
    await expect(flushWorkspaceNavigation()).resolves.toBe(false);
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-1');

    mockedGetActiveWorkspace.mockRejectedValueOnce(new Error('still unavailable'));
    await expect(flushWorkspaceNavigation()).resolves.toBe(false);
    mockedGetActiveWorkspace.mockResolvedValueOnce(versioned({
      id: 'ws-1', name: 'Test', tabs: { active: confirmedTab, items: tabs },
    }, '2') as never);
    await expect(flushWorkspaceNavigation()).resolves.toBe(expectedResult);
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe(confirmedTab);
    const reads = mockedGetActiveWorkspace.mock.calls.length;
    await expect(flushWorkspaceNavigation()).resolves.toBe(true);
    expect(mockedGetActiveWorkspace).toHaveBeenCalledTimes(reads);
  });

  it('não confirma resposta antiga quando evento mais novo já projetou outra seleção', async () => {
    const tabs = [
      { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
      { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
      { id: 'tab-3', type: 'chat', title: 'Chat 3', position: 2 },
    ];
    mockedGetActiveWorkspace.mockReset().mockResolvedValueOnce(versioned({
      id: 'ws-1', name: 'Test', tabs: { active: 'tab-1', items: tabs },
    }, '1') as never);
    mockedListWorkspaces.mockResolvedValueOnce([]);
    await useWorkspaceStore.getState().initialize();
    mockedGetActiveWorkspace.mockResolvedValue(undefined as never);
    const cleanup = useWorkspaceStore.getState().setupEventListeners();
    const reconcileCalls = mockedGetActiveWorkspace.mock.calls.length;
    const eventRegistration = mockedEventsOn.mock.calls.find(([event]) => event === 'workspace:tab_navigated');
    const navigate = eventRegistration?.[1] as (payload: unknown) => void;
    const reply = deferred<unknown>();
    mockedSetActiveWorkspaceTab.mockReturnValueOnce(reply.promise as never);

    useWorkspaceStore.getState().setActiveTab('tab-2');
    navigate(versioned({
      id: 'ws-1', name: 'Test', tabs: { active: 'tab-3', items: tabs },
    }, '4'));
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-2');
    reply.resolve(versioned({
      id: 'ws-1', name: 'Test', tabs: { active: 'tab-2', items: tabs },
    }, '3'));

    await expect(flushWorkspaceNavigation()).resolves.toBe(false);
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-3');
    expect(mockedGetActiveWorkspace).toHaveBeenCalledTimes(reconcileCalls);
    cleanup();
  });

  it('flush funciona como barreira e só resolve após a confirmação canônica', async () => {
    mockedGetActiveWorkspace.mockReset().mockResolvedValueOnce(versioned({
      id: 'ws-1', name: 'Test', tabs: {
        active: 'tab-1',
        items: [
          { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
          { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
        ],
      },
    }, '1') as never);
    mockedListWorkspaces.mockResolvedValueOnce([]);
    await useWorkspaceStore.getState().initialize();
    setStoreState([
      { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
      { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
    ], 'tab-1');
    const reply = deferred<unknown>();
    mockedSetActiveWorkspaceTab.mockReturnValueOnce(reply.promise as never);
    useWorkspaceStore.getState().setActiveTab('tab-2');
    let settled = false;
    const barrier = flushWorkspaceNavigation().then(result => {
      settled = true;
      return result;
    });
    await Promise.resolve();
    expect(settled).toBe(false);
    reply.resolve(versioned({
      id: 'ws-1', name: 'Test', tabs: {
        active: 'tab-2',
        items: [
          { id: 'tab-1', type: 'chat', title: 'Chat 1', position: 0 },
          { id: 'tab-2', type: 'chat', title: 'Chat 2', position: 1 },
        ],
      },
    }, '2'));
    await expect(barrier).resolves.toBe(true);
  });
});

describe('initialize', () => {
  beforeEach(() => {
    vi.useRealTimers();
    useWorkspaceStore.setState({ workspace: null, isInitialized: false, workspaces: [] });
    mockedGetActiveWorkspace.mockReset();
    mockedListWorkspaces.mockReset();
    mockedWaitForWailsBridge.mockReset();
  });

  it('permite flush sem âncora quando não há seleção pendente', async () => {
    mockedWaitForWailsBridge.mockResolvedValueOnce(undefined);
    mockedGetActiveWorkspace.mockResolvedValueOnce(null as never);
    mockedListWorkspaces.mockResolvedValueOnce([]);

    await useWorkspaceStore.getState().initialize();

    await expect(flushWorkspaceNavigation()).resolves.toBe(true);
  });

  it('libera flush da nova sessão quando a ativação antiga fica pendurada', async () => {
    const oldRequest = deferred<unknown>();
    const newRequest = deferred<unknown>();
    mockedWaitForWailsBridge.mockResolvedValue(undefined);
    mockedGetActiveWorkspace.mockResolvedValueOnce(versioned({
      id: 'ws-old', name: 'Old', tabs: {
        active: 'old-tab', items: [
          { id: 'old-tab', type: 'chat', title: 'Old', position: 0 },
          { id: 'old-target', type: 'chat', title: 'Old target', position: 1 },
        ],
      },
    }, '1') as never);
    mockedListWorkspaces.mockResolvedValue([]);
    useAuthStore.setState({ isAuthenticated: true, user: { userId: 'user-old', sessionId: 'session-old', role: 'user' } });
    await useWorkspaceStore.getState().initialize();
    mockedSetActiveWorkspaceTab
      .mockReturnValueOnce(oldRequest.promise as never)
      .mockReturnValueOnce(newRequest.promise as never);
    useWorkspaceStore.getState().setActiveTab('old-target');
    await vi.waitFor(() => expect(mockedSetActiveWorkspaceTab).toHaveBeenCalledWith('ws-old', 'old-target'));

    useAuthStore.setState({ isAuthenticated: false, user: null });
    useAuthStore.setState({ isAuthenticated: true, user: { userId: 'user-new', sessionId: 'session-new', role: 'user' } });
    mockedGetActiveWorkspace.mockResolvedValueOnce(versioned({
      id: 'ws-new', name: 'New', tabs: {
        active: 'new-tab', items: [{ id: 'new-tab', type: 'chat', title: 'New', position: 0 }],
      },
    }, '1') as never).mockResolvedValue(versioned({
      id: 'ws-new', name: 'New', tabs: {
        active: 'new-tab', items: [{ id: 'new-tab', type: 'chat', title: 'New', position: 0 }],
      },
    }, '2') as never);
    mockedListWorkspaces.mockResolvedValue([]);
    await useWorkspaceStore.getState().initialize();
    await expect(flushWorkspaceNavigation()).resolves.toBe(true);

    useWorkspaceStore.setState(state => ({
      workspace: state.workspace
        ? {
            ...state.workspace,
            tabs: [...state.workspace.tabs, { id: 'new-target', type: 'chat', title: 'New target', position: 1 }],
          }
        : state.workspace,
    }));
    useWorkspaceStore.getState().setActiveTab('new-target');
    await vi.waitFor(() => expect(mockedSetActiveWorkspaceTab).toHaveBeenCalledWith('ws-new', 'new-target'));
    let newFlushSettled = false;
    const newFlush = flushWorkspaceNavigation().then(result => {
      newFlushSettled = true;
      return result;
    });
    oldRequest.resolve(versioned({
      id: 'ws-old', name: 'Old', tabs: {
        active: 'old-target', items: [{ id: 'old-target', type: 'chat', title: 'Old target', position: 0 }],
      },
    }, '2'));
    await Promise.resolve();
    expect(newFlushSettled).toBe(false);
    expect(useWorkspaceStore.getState().workspace).toMatchObject({ id: 'ws-new', activeTabId: 'new-target' });
    newRequest.resolve(versioned({
      id: 'ws-new', name: 'New', tabs: {
        active: 'new-target', items: [
          { id: 'new-tab', type: 'chat', title: 'New', position: 0 },
          { id: 'new-target', type: 'chat', title: 'New target', position: 1 },
        ],
      },
    }, '2'));
    await expect(newFlush).resolves.toBe(true);
    expect(useWorkspaceStore.getState().workspace).toMatchObject({ id: 'ws-new', activeTabId: 'new-target' });
    useAuthStore.setState({ isAuthenticated: false, user: null });
  });

  it('não invalida o bootstrap quando listeners são instalados enquanto a ponte aguarda', async () => {
    let releaseBridge!: () => void;
    const bridge = new Promise<void>(resolve => { releaseBridge = resolve; });
    mockedWaitForWailsBridge.mockReturnValueOnce(bridge);
    mockedGetActiveWorkspace.mockResolvedValueOnce({
      id: 'ws-startup',
      name: 'Startup',
      snapshot_epoch: 'epoch-startup',
      snapshot_sequence: '1',
      tabs: { active: 'tab-startup', items: [{ id: 'tab-startup', type: 'chat', title: 'Chat', position: 0 }] },
    } as unknown as workspace.Workspace);
    mockedListWorkspaces.mockResolvedValueOnce([]);

    const initialization = useWorkspaceStore.getState().initialize();
    const cleanup = useWorkspaceStore.getState().setupEventListeners();
    releaseBridge();
    await initialization;

    expect(useWorkspaceStore.getState().workspace?.id).toBe('ws-startup');
    expect(useWorkspaceStore.getState().isInitialized).toBe(true);
    cleanup();
  });

  it('faz retry quando waitForWailsBridge expira e só marca isInitialized após sucesso', async () => {
    vi.useFakeTimers();
    mockedWaitForWailsBridge
      .mockRejectedValueOnce(new Error('Timed out waiting for Wails bridge after 10000ms'))
      .mockResolvedValueOnce(undefined);
    mockedGetActiveWorkspace.mockResolvedValueOnce(null as unknown as workspace.Workspace);
    mockedListWorkspaces.mockResolvedValueOnce([]);

    await useWorkspaceStore.getState().initialize();

    expect(useWorkspaceStore.getState().isInitialized).toBe(false);
    expect(mockedGetActiveWorkspace).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1000);

    await vi.waitFor(() => {
      expect(mockedWaitForWailsBridge).toHaveBeenCalledTimes(2);
      expect(mockedGetActiveWorkspace).toHaveBeenCalledTimes(1);
      expect(mockedListWorkspaces).toHaveBeenCalledTimes(1);
      expect(useWorkspaceStore.getState().isInitialized).toBe(true);
    });
  });

  it('apresenta titulo traduzido para aba padrao sem titulo persistido', async () => {
    mockedWaitForWailsBridge.mockResolvedValueOnce(undefined);
    mockedGetActiveWorkspace.mockResolvedValueOnce({
      id: 'ws-default',
      name: 'Default',
      snapshot_epoch: 'epoch-default',
      snapshot_sequence: '1',
      tabs: {
        active: 'tab-default',
        items: [{
          id: 'tab-default',
          type: 'chat',
          title: '',
          position: 0,
        }],
      },
    } as unknown as workspace.Workspace);
    mockedListWorkspaces.mockResolvedValueOnce([]);

    await useWorkspaceStore.getState().initialize();

    expect(useWorkspaceStore.getState().workspace?.tabs[0]?.title)
      .toBe(i18next.t('chat.newConversation'));
  });

  it.each([
    ['editor', 'editor.fallback.newDoc'],
    ['tasklist', 'workspace.newTasklist'],
  ] as const)('apresenta titulo traduzido para aba %s sem titulo persistido', async (type, key) => {
    mockedWaitForWailsBridge.mockResolvedValueOnce(undefined);
    mockedGetActiveWorkspace.mockResolvedValueOnce({
      id: 'ws-fallback',
      name: 'Fallbacks',
      snapshot_epoch: 'epoch-fallback',
      snapshot_sequence: '1',
      tabs: {
        active: `tab-${type}`,
        items: [{
          id: `tab-${type}`,
          type,
          title: '',
          position: 0,
        }],
      },
    } as unknown as workspace.Workspace);
    mockedListWorkspaces.mockResolvedValueOnce([]);

    await useWorkspaceStore.getState().initialize();

    expect(useWorkspaceStore.getState().workspace?.tabs[0]?.title).toBe(i18next.t(key));
  });

  it('preserva titulo não vazio vindo do backend', async () => {
    mockedWaitForWailsBridge.mockResolvedValueOnce(undefined);
    mockedGetActiveWorkspace.mockResolvedValueOnce({
      id: 'ws-title',
      name: 'Titles',
      snapshot_epoch: 'epoch-title',
      snapshot_sequence: '1',
      tabs: {
        active: 'tab-editor',
        items: [{ id: 'tab-editor', type: 'editor', title: 'README.md', position: 0 }],
      },
    } as unknown as workspace.Workspace);
    mockedListWorkspaces.mockResolvedValueOnce([]);

    await useWorkspaceStore.getState().initialize();

    expect(useWorkspaceStore.getState().workspace?.tabs[0]?.title).toBe('README.md');
  });
});

describe('updateTab — filePath no state', () => {
  beforeEach(() => {
    useWorkspaceStore.setState({ workspace: null, isInitialized: false, workspaces: [] });
    mockedUpdateWorkspaceTab.mockReset();
    mockedUpdateWorkspaceTab.mockResolvedValue(undefined);
  });

  it('propaga filePath para state da aba editor', async () => {
    setStoreState([
      { id: 'tab-e1', type: 'editor', title: 'Novo documento', position: 0, state: {} },
    ], 'tab-e1');

    await useWorkspaceStore.getState().updateTab('tab-e1', {
      state: { filePath: '/home/user/readme.md' },
    });

    const tab = useWorkspaceStore.getState().workspace?.tabs[0];
    expect(tab?.state?.filePath).toBe('/home/user/readme.md');
    expect(mockedUpdateWorkspaceTab).toHaveBeenCalledWith('tab-e1', {
      state: { filePath: '/home/user/readme.md' },
    });
  });

  it('preserva state existente ao adicionar filePath', async () => {
    setStoreState([
      { id: 'tab-e2', type: 'editor', title: 'Doc', position: 0, state: { draftId: 'draft-1', scrollTop: 100 } },
    ], 'tab-e2');

    await useWorkspaceStore.getState().updateTab('tab-e2', {
      state: { draftId: 'draft-1', scrollTop: 100, filePath: '/saved.md' },
    });

    const tab = useWorkspaceStore.getState().workspace?.tabs[0];
    expect(tab?.state).toEqual({ draftId: 'draft-1', scrollTop: 100, filePath: '/saved.md' });
  });

  it('atualiza titulo e state juntos', async () => {
    setStoreState([
      { id: 'tab-e3', type: 'editor', title: 'Novo documento', position: 0, state: {} },
    ], 'tab-e3');

    await useWorkspaceStore.getState().updateTab('tab-e3', {
      title: 'saved-file.md',
      state: { filePath: '/saved-file.md' },
    });

    const tab = useWorkspaceStore.getState().workspace?.tabs[0];
    expect(tab?.title).toBe('saved-file.md');
    expect(tab?.state?.filePath).toBe('/saved-file.md');
  });

  it('aba pristine (sem state) recebe filePath corretamente', async () => {
    setStoreState([
      { id: 'tab-pristine', type: 'editor', title: 'Sem state', position: 0 },
    ], 'tab-pristine');

    await useWorkspaceStore.getState().updateTab('tab-pristine', {
      state: { filePath: '/new.md' },
    });

    const tab = useWorkspaceStore.getState().workspace?.tabs[0];
    expect(tab?.state).toEqual({ filePath: '/new.md' });
  });

  it('faz merge de state parcial sem perder filePath existente', async () => {
    setStoreState([
      {
        id: 'tab-e2-merge',
        type: 'editor',
        title: 'Doc com arquivo',
        position: 0,
        state: { filePath: '/persisted.md', sessionId: 'session-1' },
      },
    ], 'tab-e2-merge');

    await useWorkspaceStore.getState().updateTab('tab-e2-merge', {
      state: { scrollTop: 240 },
    });

    const tab = useWorkspaceStore.getState().workspace?.tabs[0];
    expect(tab?.state).toEqual({
      filePath: '/persisted.md',
      sessionId: 'session-1',
      scrollTop: 240,
    });
    expect(mockedUpdateWorkspaceTab).toHaveBeenCalledWith('tab-e2-merge', {
      state: { scrollTop: 240 },
    });
  });
});

describe('updateTab — ProfileOverride por patch', () => {
  beforeEach(() => {
    useWorkspaceStore.setState({ workspace: null, isInitialized: false, workspaces: [] });
    mockedUpdateWorkspaceTab.mockReset().mockResolvedValue(undefined);
  });

  it('preserva slug ao trocar somente o modelo', async () => {
    setStoreState([{
      id: 'tab-chat',
      type: 'chat',
      title: 'Chat',
      position: 0,
      profileOverride: { slug: 'programacao' },
    }], 'tab-chat');

    await useWorkspaceStore.getState().updateTab('tab-chat', {
      profile_override: { model: 'modelo-b' },
    });

    expect(useWorkspaceStore.getState().workspace?.tabs[0]?.profileOverride).toEqual({
      slug: 'programacao',
      model: 'modelo-b',
    });
  });

  it('remove modelo com nil sem apagar o slug', async () => {
    setStoreState([{
      id: 'tab-chat',
      type: 'chat',
      title: 'Chat',
      position: 0,
      profileOverride: { slug: 'programacao', model: 'modelo-b' },
    }], 'tab-chat');

    await useWorkspaceStore.getState().updateTab('tab-chat', {
      profile_override: { model: null },
    });

    expect(useWorkspaceStore.getState().workspace?.tabs[0]?.profileOverride).toEqual({
      slug: 'programacao',
    });
  });

  it('ignora undefined porque somente null remove uma chave', async () => {
    setStoreState([{
      id: 'tab-chat',
      type: 'chat',
      title: 'Chat',
      position: 0,
      profileOverride: { slug: 'programacao', model: 'modelo-b' },
    }], 'tab-chat');

    await useWorkspaceStore.getState().updateTab('tab-chat', {
      profile_override: { model: undefined },
    });

    expect(useWorkspaceStore.getState().workspace?.tabs[0]?.profileOverride).toEqual({
      slug: 'programacao',
      model: 'modelo-b',
    });
  });

  it('não altera o store quando a persistência falha', async () => {
    setStoreState([{
      id: 'tab-chat',
      type: 'chat',
      title: 'Chat',
      position: 0,
      profileOverride: { slug: 'programacao', model: 'modelo-a' },
    }], 'tab-chat');
    mockedUpdateWorkspaceTab.mockRejectedValueOnce(new Error('falha'));

    await expect(useWorkspaceStore.getState().updateTab('tab-chat', {
      profile_override: { model: 'modelo-b' },
    })).rejects.toThrow('falha');

    expect(useWorkspaceStore.getState().workspace?.tabs[0]?.profileOverride).toEqual({
      slug: 'programacao',
      model: 'modelo-a',
    });
  });
});
