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

import { useWorkspaceStore, registerTabRenameHandler } from './workspaceStore';
import { GetActiveWorkspace, ListWorkspaces, SetActiveWorkspaceTab, UpdateWorkspaceTab } from '@wailsjs/go/wailsapi/Workspace';
import { waitForWailsBridge } from '../lib/waitForWailsBridge';
import { workspace } from '../../wailsjs/go/models';
import i18next from 'i18next';

const mockedGetActiveWorkspace = vi.mocked(GetActiveWorkspace);
const mockedListWorkspaces = vi.mocked(ListWorkspaces);
const mockedSetActiveWorkspaceTab = vi.mocked(SetActiveWorkspaceTab);
const mockedUpdateWorkspaceTab = vi.mocked(UpdateWorkspaceTab);
const mockedWaitForWailsBridge = vi.mocked(waitForWailsBridge);

function setStoreState(
  tabs: Array<{ id: string; type: string; conversationId?: string; state?: Record<string, unknown>; title: string; position: number }>,
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
        title: t.title,
        position: t.position,
      })),
      activeTabId,
    },
    isInitialized: true,
    workspaces: [],
  });
}

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
      expect(mockedSetActiveWorkspaceTab).toHaveBeenCalledWith('tab-2');
    });
    expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-2');
  });

  it('faz rollback quando backend falha e aba ativa não mudou', async () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", title: 'Chat 1', position: 0 },
      { id: 'tab-2', type: 'chat', conversationId: "01926b90-7a5a-7c4e-8d3f-000000000002", title: 'Chat 2', position: 1 },
    ], 'tab-1');

    mockedSetActiveWorkspaceTab.mockRejectedValue(new Error('backend error'));

    // setActiveTab é síncrono; o rollback ocorre no .catch assíncrono
    useWorkspaceStore.getState().setActiveTab('tab-2');

    // Aguarda o .catch processar o rollback
    await vi.waitFor(() => {
      expect(useWorkspaceStore.getState().workspace?.activeTabId).toBe('tab-1');
    });
    expect(mockedAnnounce).toHaveBeenCalled();
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
});

describe('initialize', () => {
  beforeEach(() => {
    vi.useRealTimers();
    useWorkspaceStore.setState({ workspace: null, isInitialized: false, workspaces: [] });
    mockedGetActiveWorkspace.mockReset();
    mockedListWorkspaces.mockReset();
    mockedWaitForWailsBridge.mockReset();
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
      tabs: {
        active: 'tab-default',
        items: [{
          id: 'tab-default',
          type: 'chat',
          title: '',
          position: 0,
        }],
      },
    } as workspace.Workspace);
    mockedListWorkspaces.mockResolvedValueOnce([]);

    await useWorkspaceStore.getState().initialize();

    expect(useWorkspaceStore.getState().workspace?.tabs[0]?.title)
      .toBe(i18next.t('chat.newConversation'));
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
