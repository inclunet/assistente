import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const mockIsModalOpen = vi.fn();
const mockAddToast = vi.fn();

vi.mock('./workspaceStore', () => ({
  useWorkspaceStore: {
    getState: () => ({
      workspace: {
        tabs: [
          { id: 'tab-editor', type: 'editor' as const, title: 'x', position: 0 },
          { id: 'tab-chat', type: 'chat' as const, title: 'Chat', position: 1 },
        ],
      },
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

const mockEnsureWorkspaceTabHasConversation = vi.fn().mockResolvedValue("1");
vi.mock('../lib/workspaceConversation', () => ({
  ensureWorkspaceTabConversationId: (...args: unknown[]) =>
    mockEnsureWorkspaceTabHasConversation(...args),
}));

import {
  useWorkspaceChatModalStore,
  registerWorkspaceChatModalAdapter,
} from './workspaceChatModalStore';

function resetWorkspaceChatModalState() {
  useWorkspaceChatModalStore.setState({
    isOpen: false,
    boundTabId: null,
    boundConversationId: null,
    boundSurface: null,
    contextDisplay: '',
    sessionMeta: null,
    boundSend: null,
    focusNonce: 0,
    adapterError: null,
  });
}

describe('workspaceChatModalStore.requestOpen', () => {
  beforeEach(() => {
    resetWorkspaceChatModalState();
    mockIsModalOpen.mockReset();
    mockAddToast.mockReset();
    mockEnsureWorkspaceTabHasConversation.mockClear();
    registerWorkspaceChatModalAdapter('tab-editor', null);
  });

  afterEach(() => {
    registerWorkspaceChatModalAdapter('tab-editor', null);
  });

  it('não faz nada quando o tabId explícito não existe', async () => {
    await useWorkspaceChatModalStore.getState().requestOpen('tab-missing');
    expect(useWorkspaceChatModalStore.getState().isOpen).toBe(false);
    expect(mockAddToast).not.toHaveBeenCalled();
  });

  it('com chat modal já aberto, requestOpen só reforça o foco (bumpFocus) sem chamar prepare', async () => {
    useWorkspaceChatModalStore.getState().open('ctx', {}, 'tab-editor', '1', {
      conversationId: '1',
      sessionKey: 'modal:workspace-chat:tab-editor:1',
      surfaceId: 'modal:workspace-chat:tab-editor',
      surfaceType: 'modal',
      tabId: 'tab-editor',
    }, vi.fn());
    const nonceBefore = useWorkspaceChatModalStore.getState().focusNonce;
    const prepare = vi.fn();
    registerWorkspaceChatModalAdapter('tab-editor', { prepare, send: vi.fn() });

    await useWorkspaceChatModalStore.getState().requestOpen('tab-editor');

    expect(prepare).not.toHaveBeenCalled();
    expect(useWorkspaceChatModalStore.getState().focusNonce).toBe(nonceBefore + 1);
  });

  it('retorna cedo quando um modal genérico está aberto', async () => {
    mockIsModalOpen.mockReturnValue(true);
    const prepare = vi.fn().mockResolvedValue({ ok: true, contextDisplay: 'x', meta: null });
    registerWorkspaceChatModalAdapter('tab-editor', { prepare, send: vi.fn() });

    await useWorkspaceChatModalStore.getState().requestOpen('tab-editor');

    expect(prepare).not.toHaveBeenCalled();
    expect(mockAddToast).toHaveBeenCalledWith('workspace.chatModal.modalBlocked', 'info');
  });

  it('com aba chat ativa não foca o input quando um modal está aberto', async () => {
    mockIsModalOpen.mockReturnValue(true);
    const focusSpy = vi.spyOn(HTMLElement.prototype, 'focus').mockImplementation(() => {});

    await useWorkspaceChatModalStore.getState().requestOpen('tab-chat');

    expect(focusSpy).not.toHaveBeenCalled();
    focusSpy.mockRestore();
  });

  it('com aba chat ativa foca o textarea atual do chat page', async () => {
    mockIsModalOpen.mockReturnValue(false);
    document.body.innerHTML =
      '<div class="chat-page"><textarea class="chat-input__textarea"></textarea></div>';
    const textarea = document.querySelector('.chat-input__textarea') as HTMLTextAreaElement;
    const focusSpy = vi.spyOn(textarea, 'focus').mockImplementation(() => {});

    await useWorkspaceChatModalStore.getState().requestOpen('tab-chat');

    expect(focusSpy).toHaveBeenCalledTimes(1);
    focusSpy.mockRestore();
    document.body.innerHTML = '';
  });

  it('mostra toast quando o painel ativo não tem adaptador', async () => {
    await useWorkspaceChatModalStore.getState().requestOpen('tab-editor');

    expect(mockAddToast).toHaveBeenCalledWith('workspace.chatModal.panelNotSupported', 'info');
    expect(mockEnsureWorkspaceTabHasConversation).not.toHaveBeenCalled();
  });

  it('abre com boundTabId do tabId explícito quando prepare() tem sucesso', async () => {
    const meta = { kind: 'test' };
    const prepare = vi.fn().mockResolvedValue({ ok: true, contextDisplay: 'selection', meta });
    registerWorkspaceChatModalAdapter('tab-editor', { prepare, send: vi.fn() });

    await useWorkspaceChatModalStore.getState().requestOpen('tab-editor');

    expect(prepare).toHaveBeenCalledTimes(1);
    expect(mockEnsureWorkspaceTabHasConversation).toHaveBeenCalledTimes(1);
    const s = useWorkspaceChatModalStore.getState();
    expect(s.isOpen).toBe(true);
    expect(s.boundTabId).toBe('tab-editor');
    expect(s.boundConversationId).toBe("1");
    expect(s.boundSurface).toEqual({
      conversationId: '1',
      sessionKey: 'modal:workspace-chat:tab-editor:1',
      surfaceId: 'modal:workspace-chat:tab-editor',
      surfaceType: 'modal',
      tabId: 'tab-editor',
    });
    expect(s.contextDisplay).toBe('selection');
    expect(s.sessionMeta).toEqual(meta);
    expect(typeof s.boundSend).toBe('function');
  });

  it('abre usando tabId explícito sem consultar aba ativa', async () => {
    const prepare = vi.fn().mockResolvedValue({ ok: true, contextDisplay: 'selection', meta: null });
    registerWorkspaceChatModalAdapter('tab-editor', { prepare, send: vi.fn() });

    await useWorkspaceChatModalStore.getState().requestOpen('tab-editor');

    const s = useWorkspaceChatModalStore.getState();
    expect(s.isOpen).toBe(true);
    expect(s.boundTabId).toBe('tab-editor');
    expect(s.boundSurface?.tabId).toBe('tab-editor');
    expect(s.boundSurface?.surfaceType).toBe('modal');
  });

  it('não abre quando prepare() falha sem mensagem', async () => {
    const prepare = vi.fn().mockResolvedValue({ ok: false });
    registerWorkspaceChatModalAdapter('tab-editor', { prepare, send: vi.fn() });

    await useWorkspaceChatModalStore.getState().requestOpen('tab-editor');

    expect(useWorkspaceChatModalStore.getState().isOpen).toBe(false);
    expect(mockAddToast).not.toHaveBeenCalled();
    expect(mockEnsureWorkspaceTabHasConversation).not.toHaveBeenCalled();
  });

  it('mostra toast de erro quando prepare() lança', async () => {
    const prepare = vi.fn().mockRejectedValue(new Error('boom'));
    registerWorkspaceChatModalAdapter('tab-editor', { prepare, send: vi.fn() });

    await useWorkspaceChatModalStore.getState().requestOpen('tab-editor');

    expect(useWorkspaceChatModalStore.getState().isOpen).toBe(false);
    expect(mockAddToast).toHaveBeenCalledWith('workspace.chatModal.prepareFailed', 'error');
    expect(mockEnsureWorkspaceTabHasConversation).not.toHaveBeenCalled();
  });
});
