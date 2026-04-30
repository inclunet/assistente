import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { WorkspaceTab } from '../store/workspaceStore';

const mockCreateConversation = vi.fn();
const mockUpdateTab = vi.fn();
const mockLoadConversation = vi.fn();

const hoisted = vi.hoisted(() => {
  let tabs: Array<Pick<WorkspaceTab, 'id' | 'conversationId'>> = [];
  let activeConversationId: string | null = null;
  let activeTab: Pick<WorkspaceTab, 'id' | 'conversationId'> | null = null;
  return {
    get tabs() {
      return tabs;
    },
    setTabs(next: typeof tabs) {
      tabs = next;
    },
    get activeConversationId() {
      return activeConversationId;
    },
    get activeTab() {
      return activeTab;
    },
    setActiveId(id: string | null) {
      activeConversationId = id;
    },
    setActiveTab(tab: typeof activeTab) {
      activeTab = tab;
    },
    reset() {
      activeConversationId = null;
      activeTab = null;
      tabs = [];
    },
  };
});

vi.mock('@wailsjs/go/app/App', () => ({
  CreateConversation: (title: string, body: string) => mockCreateConversation(title, body),
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: {
    getState: () => ({
      get workspace() {
        return { tabs: hoisted.tabs };
      },
      getActiveTab: () => hoisted.activeTab,
      updateTab: mockUpdateTab,
    }),
  },
}));

vi.mock('../store/chatStore', () => ({
  useChatStore: {
    getState: () => ({
      get activeConversationId() {
        return hoisted.activeConversationId;
      },
      loadConversationSession: (id: string, options?: { activate?: boolean }) => mockLoadConversation(id, options),
    }),
  },
}));

import { ensureWorkspaceTabConversationId, ensureWorkspaceTabHasConversation } from './workspaceConversation';

describe('ensureWorkspaceTabHasConversation', () => {
  beforeEach(() => {
    hoisted.reset();
    mockCreateConversation.mockReset();
    mockUpdateTab.mockReset();
    mockLoadConversation.mockReset();
    mockCreateConversation.mockResolvedValue({ id: '01970a9e-0099-7000-8000-000000000099' });
    mockUpdateTab.mockResolvedValue(undefined);
    mockLoadConversation.mockImplementation(async (id: string) => {
      hoisted.setActiveId(id);
    });
  });

  it('dedupe inflight: duas chamadas seguidas \u00e0 mesma aba partilham uma cria\u00e7\u00e3o', async () => {
    hoisted.setTabs([{ id: 'tab-dedupe' }]);
    const tab = hoisted.tabs[0]!;

    const p1 = ensureWorkspaceTabHasConversation(tab as WorkspaceTab);
    const p2 = ensureWorkspaceTabHasConversation(tab as WorkspaceTab);

    const [a, b] = await Promise.all([p1, p2]);
    expect(a).toBe('01970a9e-0099-7000-8000-000000000099');
    expect(b).toBe('01970a9e-0099-7000-8000-000000000099');
    expect(mockCreateConversation).toHaveBeenCalledTimes(1);
  });

  it('quando a aba j\u00e1 tem conversationId, sincroniza o chatStore se necess\u00e1rio', async () => {
    hoisted.setTabs([{ id: 'tab-sync', conversationId: '01970a9e-0007-7000-8000-000000000007' }]);
    mockLoadConversation.mockClear();

    const id = await ensureWorkspaceTabHasConversation(hoisted.tabs[0] as WorkspaceTab);

    expect(id).toBe('01970a9e-0007-7000-8000-000000000007');
    expect(mockCreateConversation).not.toHaveBeenCalled();
    expect(mockUpdateTab).not.toHaveBeenCalled();
    expect(mockLoadConversation).toHaveBeenCalledWith('01970a9e-0007-7000-8000-000000000007', { activate: true });
    expect(hoisted.activeConversationId).toBe('01970a9e-0007-7000-8000-000000000007');
  });

  it('quando conversationId est\u00e1 vazio, cria conversa, atualiza aba e carrega mensagens', async () => {
    hoisted.setTabs([{ id: 'tab-new' }]);
    hoisted.setActiveTab({ id: 'tab-new' });

    const id = await ensureWorkspaceTabHasConversation(hoisted.tabs[0] as WorkspaceTab);

    expect(id).toBe('01970a9e-0099-7000-8000-000000000099');
    expect(mockCreateConversation).toHaveBeenCalledTimes(1);
    expect(mockUpdateTab).toHaveBeenCalledWith('tab-new', { conversation_id: '01970a9e-0099-7000-8000-000000000099' });
    expect(mockLoadConversation).toHaveBeenCalledWith('01970a9e-0099-7000-8000-000000000099', { activate: true });
    expect(hoisted.activeConversationId).toBe('01970a9e-0099-7000-8000-000000000099');
  });

  it('lan\u00e7a se a aba n\u00e3o existe no workspace', async () => {
    hoisted.setTabs([]);
    await expect(ensureWorkspaceTabHasConversation({ id: 'missing' } as WorkspaceTab)).rejects.toThrow(
      /Aba n\u00e3o encontrada/,
    );
  });

  it('n\u00e3o chama loadConversation quando o chat j\u00e1 est\u00e1 na conversa da aba', async () => {
    hoisted.setTabs([{ id: 'tab-skip-load', conversationId: '01970a9e-0007-7000-8000-000000000007' }]);
    hoisted.setActiveId('01970a9e-0007-7000-8000-000000000007');
    mockLoadConversation.mockClear();

    const id = await ensureWorkspaceTabHasConversation(hoisted.tabs[0] as WorkspaceTab);

    expect(id).toBe('01970a9e-0007-7000-8000-000000000007');
    expect(mockLoadConversation).not.toHaveBeenCalled();
  });

  it('ensureWorkspaceTabConversationId não sincroniza o chatStore quando só precisa do id', async () => {
    hoisted.setTabs([{ id: 'tab-id-only', conversationId: '01970a9e-0007-7000-8000-000000000007' }]);
    mockLoadConversation.mockClear();

    const id = await ensureWorkspaceTabConversationId(hoisted.tabs[0] as WorkspaceTab);

    expect(id).toBe('01970a9e-0007-7000-8000-000000000007');
    expect(mockLoadConversation).not.toHaveBeenCalled();
  });

  it('ensureWorkspaceTabConversationId n\u00e3o carrega o chatStore ao criar nova conversa', async () => {
    hoisted.setTabs([{ id: 'tab-new-id-only' }]);
    mockLoadConversation.mockClear();

    const id = await ensureWorkspaceTabConversationId(hoisted.tabs[0] as WorkspaceTab);

    expect(id).toBe('01970a9e-0099-7000-8000-000000000099');
    expect(mockCreateConversation).toHaveBeenCalledTimes(1);
    expect(mockUpdateTab).toHaveBeenCalledWith('tab-new-id-only', { conversation_id: '01970a9e-0099-7000-8000-000000000099' });
    expect(mockLoadConversation).not.toHaveBeenCalled();
  });

  it('ensureWorkspaceTabHasConversation n\u00e3o sincroniza o chatStore se a aba j\u00e1 n\u00e3o estiver ativa', async () => {
    hoisted.setTabs([{ id: 'tab-inactive', conversationId: '01970a9e-0007-7000-8000-000000000007' }, { id: 'other-tab', conversationId: '01970a9e-0011-7000-8000-000000000011' }]);
    hoisted.setActiveTab({ id: 'other-tab', conversationId: '01970a9e-0011-7000-8000-000000000011' });
    mockLoadConversation.mockClear();

    const id = await ensureWorkspaceTabHasConversation(hoisted.tabs[0] as WorkspaceTab);

    expect(id).toBe('01970a9e-0007-7000-8000-000000000007');
    expect(mockLoadConversation).not.toHaveBeenCalled();
  });
});
