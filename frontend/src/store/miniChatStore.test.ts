import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const mockGetActiveTab = vi.fn();
const mockIsModalOpen = vi.fn();
const mockAddToast = vi.fn();

vi.mock('./workspaceStore', () => ({
  useWorkspaceStore: {
    getState: () => ({
      getActiveTab: () => mockGetActiveTab(),
    }),
  },
}));

vi.mock('../components/ui/Modal', () => ({
  isModalOpen: () => mockIsModalOpen(),
}));

vi.mock('./uiStore', () => ({
  useUIStore: {
    getState: () => ({
      addToast: mockAddToast,
    }),
  },
}));

vi.mock('i18next', () => ({
  default: {
    t: (key: string) => key,
  },
}));

const mockEnsureWorkspaceTabHasConversation = vi.fn().mockResolvedValue(1);
vi.mock('../lib/workspaceConversation', () => ({
  ensureWorkspaceTabHasConversation: (...args: unknown[]) => mockEnsureWorkspaceTabHasConversation(...args),
}));

import { useMiniChatStore, registerMiniChatAdapter } from './miniChatStore';

const editorTab = { id: 'tab-editor', type: 'editor' as const, title: 'x', position: 0 };

function resetMiniChatState() {
  useMiniChatStore.setState({
    isOpen: false,
    boundTabId: null,
    contextDisplay: '',
    sessionMeta: null,
    focusNonce: 0,
    adapterError: null,
  });
}

describe('miniChatStore.requestOpen', () => {
  beforeEach(() => {
    resetMiniChatState();
    mockGetActiveTab.mockReset();
    mockIsModalOpen.mockReset();
    mockAddToast.mockReset();
    mockEnsureWorkspaceTabHasConversation.mockClear();
    registerMiniChatAdapter('tab-editor', null);
  });

  afterEach(() => {
    registerMiniChatAdapter('tab-editor', null);
  });

  it('não faz nada quando não há aba ativa', async () => {
    mockGetActiveTab.mockReturnValue(undefined);
    await useMiniChatStore.getState().requestOpen();
    expect(useMiniChatStore.getState().isOpen).toBe(false);
    expect(mockAddToast).not.toHaveBeenCalled();
  });

  it('não reabre quando o mini-chat já está aberto', async () => {
    mockGetActiveTab.mockReturnValue(editorTab);
    useMiniChatStore.getState().open('ctx', {}, 'tab-editor');
    const prepare = vi.fn();
    registerMiniChatAdapter('tab-editor', { prepare, send: vi.fn() });

    await useMiniChatStore.getState().requestOpen();

    expect(prepare).not.toHaveBeenCalled();
  });

  it('retorna cedo quando um modal genérico está aberto', async () => {
    mockGetActiveTab.mockReturnValue(editorTab);
    mockIsModalOpen.mockReturnValue(true);
    const prepare = vi.fn().mockResolvedValue({ ok: true, contextDisplay: 'x', meta: null });
    registerMiniChatAdapter('tab-editor', { prepare, send: vi.fn() });

    await useMiniChatStore.getState().requestOpen();

    expect(prepare).not.toHaveBeenCalled();
  });

  it('com aba chat ativa não foca o input quando um modal está aberto', async () => {
    const chatTab = { id: 'tab-chat', type: 'chat' as const, title: 'Chat', position: 0 };
    mockGetActiveTab.mockReturnValue(chatTab);
    mockIsModalOpen.mockReturnValue(true);
    const focusSpy = vi.spyOn(HTMLElement.prototype, 'focus').mockImplementation(() => {});

    await useMiniChatStore.getState().requestOpen();

    expect(focusSpy).not.toHaveBeenCalled();
    focusSpy.mockRestore();
  });

  it('mostra toast quando o painel ativo não tem adaptador', async () => {
    mockGetActiveTab.mockReturnValue(editorTab);

    await useMiniChatStore.getState().requestOpen();

    expect(mockAddToast).toHaveBeenCalledWith('workspace.miniChat.panelNotSupported', 'info');
    expect(mockEnsureWorkspaceTabHasConversation).not.toHaveBeenCalled();
  });

  it('abre com boundTabId da aba ativa quando prepare() tem sucesso', async () => {
    mockGetActiveTab.mockReturnValue(editorTab);
    const meta = { kind: 'test' };
    const prepare = vi
      .fn()
      .mockResolvedValue({ ok: true, contextDisplay: 'selection', meta });
    registerMiniChatAdapter('tab-editor', { prepare, send: vi.fn() });

    await useMiniChatStore.getState().requestOpen();

    expect(prepare).toHaveBeenCalledTimes(1);
    expect(mockEnsureWorkspaceTabHasConversation).toHaveBeenCalledTimes(1);
    const s = useMiniChatStore.getState();
    expect(s.isOpen).toBe(true);
    expect(s.boundTabId).toBe('tab-editor');
    expect(s.contextDisplay).toBe('selection');
    expect(s.sessionMeta).toEqual(meta);
  });

  it('não abre quando prepare() falha sem mensagem', async () => {
    mockGetActiveTab.mockReturnValue(editorTab);
    const prepare = vi.fn().mockResolvedValue({ ok: false });
    registerMiniChatAdapter('tab-editor', { prepare, send: vi.fn() });

    await useMiniChatStore.getState().requestOpen();

    expect(useMiniChatStore.getState().isOpen).toBe(false);
    expect(mockAddToast).not.toHaveBeenCalled();
    expect(mockEnsureWorkspaceTabHasConversation).not.toHaveBeenCalled();
  });
});
