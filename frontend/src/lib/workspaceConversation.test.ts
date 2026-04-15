import { describe, it, expect, vi, beforeEach } from 'vitest';

const mockCreateConversation = vi.fn();
const mockUpdateTab = vi.fn();
const mockLoadConversation = vi.fn();

const hoisted = vi.hoisted(() => {
  let tabs: Array<{ id: string; conversationId?: number }> = [];
  let activeConversationId: number | null = null;
  let activeTab: { id: string; conversationId?: number } | null = null;
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
    setActiveId(id: number | null) {
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

vi.mock('@wailsjs/go/main/App', () => ({
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
      loadConversation: (id: number) => mockLoadConversation(id),
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
    mockCreateConversation.mockResolvedValue({ id: 99 });
    mockUpdateTab.mockResolvedValue(undefined);
    mockLoadConversation.mockImplementation(async (id: number) => {
      hoisted.setActiveId(id);
    });
  });

  it('dedupe inflight: duas chamadas seguidas à mesma aba partilham uma criação', async () => {
    hoisted.setTabs([{ id: 'tab-dedupe', conversationId: 0 }]);
    const tab = hoisted.tabs[0]!;

    const p1 = ensureWorkspaceTabHasConversation(tab as any);
    const p2 = ensureWorkspaceTabHasConversation(tab as any);

    const [a, b] = await Promise.all([p1, p2]);
    expect(a).toBe(99);
    expect(b).toBe(99);
    expect(mockCreateConversation).toHaveBeenCalledTimes(1);
  });

  it('quando a aba já tem conversationId, sincroniza o chatStore se necessário', async () => {
    hoisted.setTabs([{ id: 'tab-sync', conversationId: 7 }]);
    mockLoadConversation.mockClear();

    const id = await ensureWorkspaceTabHasConversation(hoisted.tabs[0] as any);

    expect(id).toBe(7);
    expect(mockCreateConversation).not.toHaveBeenCalled();
    expect(mockUpdateTab).not.toHaveBeenCalled();
    expect(mockLoadConversation).toHaveBeenCalledWith(7);
    expect(hoisted.activeConversationId).toBe(7);
  });

  it('quando conversationId é 0, cria conversa, atualiza aba e carrega mensagens', async () => {
    hoisted.setTabs([{ id: 'tab-new', conversationId: 0 }]);
    hoisted.setActiveTab({ id: 'tab-new', conversationId: 0 });

    const id = await ensureWorkspaceTabHasConversation(hoisted.tabs[0] as any);

    expect(id).toBe(99);
    expect(mockCreateConversation).toHaveBeenCalledTimes(1);
    expect(mockUpdateTab).toHaveBeenCalledWith('tab-new', { conversation_id: 99 });
    expect(mockLoadConversation).toHaveBeenCalledWith(99);
    expect(hoisted.activeConversationId).toBe(99);
  });

  it('lança se a aba não existe no workspace', async () => {
    hoisted.setTabs([]);
    await expect(ensureWorkspaceTabHasConversation({ id: 'missing' } as any)).rejects.toThrow(
      /Aba não encontrada/,
    );
  });

  it('não chama loadConversation quando o chat já está na conversa da aba', async () => {
    hoisted.setTabs([{ id: 'tab-skip-load', conversationId: 7 }]);
    hoisted.setActiveId(7);
    mockLoadConversation.mockClear();

    const id = await ensureWorkspaceTabHasConversation(hoisted.tabs[0] as any);

    expect(id).toBe(7);
    expect(mockLoadConversation).not.toHaveBeenCalled();
  });

  it('ensureWorkspaceTabConversationId não sincroniza o chatStore quando só precisa do id', async () => {
    hoisted.setTabs([{ id: 'tab-id-only', conversationId: 7 }]);
    mockLoadConversation.mockClear();

    const id = await ensureWorkspaceTabConversationId(hoisted.tabs[0] as any);

    expect(id).toBe(7);
    expect(mockLoadConversation).not.toHaveBeenCalled();
  });

  it('ensureWorkspaceTabConversationId não carrega o chatStore ao criar nova conversa', async () => {
    hoisted.setTabs([{ id: 'tab-new-id-only', conversationId: 0 }]);
    mockLoadConversation.mockClear();

    const id = await ensureWorkspaceTabConversationId(hoisted.tabs[0] as any);

    expect(id).toBe(99);
    expect(mockCreateConversation).toHaveBeenCalledTimes(1);
    expect(mockUpdateTab).toHaveBeenCalledWith('tab-new-id-only', { conversation_id: 99 });
    expect(mockLoadConversation).not.toHaveBeenCalled();
  });

  it('ensureWorkspaceTabHasConversation não sincroniza o chatStore se a aba já não estiver ativa', async () => {
    hoisted.setTabs([{ id: 'tab-inactive', conversationId: 7 }, { id: 'other-tab', conversationId: 11 }]);
    hoisted.setActiveTab({ id: 'other-tab', conversationId: 11 });
    mockLoadConversation.mockClear();

    const id = await ensureWorkspaceTabHasConversation(hoisted.tabs[0] as any);

    expect(id).toBe(7);
    expect(mockLoadConversation).not.toHaveBeenCalled();
  });
});
