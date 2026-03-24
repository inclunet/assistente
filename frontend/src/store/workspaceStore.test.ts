import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('@wailsjs/go/main/App', () => ({
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
  announce: vi.fn(),
}));

import { useWorkspaceStore, registerTabRenameHandler } from './workspaceStore';
import { UpdateWorkspaceTab } from '@wailsjs/go/main/App';

const mockedUpdateWorkspaceTab = vi.mocked(UpdateWorkspaceTab);

function setStoreState(tabs: Array<{ id: string; type: string; contentId: string; title: string; position: number }>, activeTabId: string) {
  useWorkspaceStore.setState({
    workspace: {
      id: 'ws-1',
      name: 'Test',
      tabs: tabs.map(t => ({
        id: t.id,
        type: t.type as 'chat' | 'editor' | 'terminal' | 'tasklist',
        contentId: t.contentId,
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
  });

  it('atualiza titulo da aba quando conteudo é renomeado', async () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', contentId: '42', title: 'Nova conversa', position: 0 },
    ], 'tab-1');

    useWorkspaceStore.getState().handleContentRenamed('chat', '42', 'Minha conversa');

    // updateTab é async, mas handleContentRenamed dispara void
    await vi.waitFor(() => {
      const tab = useWorkspaceStore.getState().workspace?.tabs[0];
      expect(tab?.title).toBe('Minha conversa');
    });
  });

  it('não atualiza quando titulo já é igual (previne loop)', () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', contentId: '42', title: 'Mesmo título', position: 0 },
    ], 'tab-1');

    mockedUpdateWorkspaceTab.mockClear();

    useWorkspaceStore.getState().handleContentRenamed('chat', '42', 'Mesmo título');

    expect(mockedUpdateWorkspaceTab).not.toHaveBeenCalled();
  });

  it('não atualiza quando tipo não combina', () => {
    setStoreState([
      { id: 'tab-1', type: 'editor', contentId: '42', title: 'Doc', position: 0 },
    ], 'tab-1');

    mockedUpdateWorkspaceTab.mockClear();

    useWorkspaceStore.getState().handleContentRenamed('chat', '42', 'Novo título');

    expect(mockedUpdateWorkspaceTab).not.toHaveBeenCalled();
  });

  it('atualiza aba correta quando há múltiplas abas', async () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', contentId: '10', title: 'Chat A', position: 0 },
      { id: 'tab-2', type: 'chat', contentId: '20', title: 'Chat B', position: 1 },
      { id: 'tab-3', type: 'editor', contentId: 'doc-1', title: 'Editor', position: 2 },
    ], 'tab-2');

    useWorkspaceStore.getState().handleContentRenamed('chat', '20', 'Chat Renomeado');

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
  });

  it('chama handler registrado ao renomear aba', () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', contentId: '42', title: 'Chat', position: 0 },
    ], 'tab-1');

    const handler = vi.fn();
    const unregister = registerTabRenameHandler('chat', handler);

    useWorkspaceStore.getState().renameTabContent('tab-1', 'Novo nome');

    expect(handler).toHaveBeenCalledWith('42', 'Novo nome');

    unregister();
  });

  it('não chama handler quando aba não tem contentId', () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', contentId: '', title: 'Chat', position: 0 },
    ], 'tab-1');

    const handler = vi.fn();
    const unregister = registerTabRenameHandler('chat', handler);

    useWorkspaceStore.getState().renameTabContent('tab-1', 'Novo nome');

    expect(handler).not.toHaveBeenCalled();

    unregister();
  });

  it('não chama handler quando tipo não tem handler registrado', () => {
    setStoreState([
      { id: 'tab-1', type: 'terminal', contentId: 'sess-1', title: 'Terminal', position: 0 },
    ], 'tab-1');

    const chatHandler = vi.fn();
    const unregister = registerTabRenameHandler('chat', chatHandler);

    useWorkspaceStore.getState().renameTabContent('tab-1', 'Novo nome');

    expect(chatHandler).not.toHaveBeenCalled();

    unregister();
  });

  it('unregister remove o handler', () => {
    setStoreState([
      { id: 'tab-1', type: 'chat', contentId: '42', title: 'Chat', position: 0 },
    ], 'tab-1');

    const handler = vi.fn();
    const unregister = registerTabRenameHandler('chat', handler);
    unregister();

    useWorkspaceStore.getState().renameTabContent('tab-1', 'Novo nome');

    expect(handler).not.toHaveBeenCalled();
  });
});
