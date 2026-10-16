import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const fixture = vi.hoisted(() => {
  const workspaceState = {
    workspace: {
      id: 'workspace-1',
      tabs: [
        { id: 'tab-editor', type: 'editor' as const, title: 'x', position: 0 },
        { id: 'tab-chat', type: 'chat' as const, title: 'Chat', position: 1 },
      ],
      activeTabId: 'tab-editor',
    },
  };
  return {
    workspaceState,
    workspaceSubscribers: new Set<(state: typeof workspaceState) => void>(),
    modalGeneration: 0,
  };
});

const { workspaceState, workspaceSubscribers } = fixture;
const authState = {
  isAuthenticated: true,
  user: { userId: 'user-1', sessionId: 'session-1', role: 'user' },
};
const mockIsModalOpen = vi.fn();
const mockGetModalRegistrySnapshot = vi.fn(() => ({ generation: `modal-${fixture.modalGeneration}` }));
const mockAddToast = vi.fn();
const mockUpdateTab = vi.fn().mockResolvedValue(undefined);

vi.mock('./workspaceStore', () => ({
  useWorkspaceStore: {
    getState: () => ({
      ...workspaceState,
      updateTab: mockUpdateTab,
    }),
    subscribe: (listener: (state: typeof workspaceState) => void) => {
      workspaceSubscribers.add(listener);
      return () => {
        workspaceSubscribers.delete(listener);
      };
    },
  },
}));

vi.mock('./authStore', () => ({
  useAuthStore: {
    getState: () => authState,
    subscribe: (listener: (state: typeof authState) => void) => {
      void listener;
      return () => undefined;
    },
  },
}));

vi.mock('../lib/modalRegistry', () => ({
  isModalOpen: () => mockIsModalOpen(),
  getModalRegistrySnapshot: () => mockGetModalRegistrySnapshot(),
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

const mockEnsureWorkspaceTabConversationId = vi.fn().mockResolvedValue("1");
vi.mock('../lib/workspaceConversation', () => ({
  ensureWorkspaceTabConversationId: (...args: unknown[]) =>
    mockEnsureWorkspaceTabConversationId(...args),
}));

import {
  useWorkspaceChatModalStore,
  prepareWorkspaceChatOpen,
  registerWorkspaceChatCommandDispatcher,
  registerWorkspaceChatModalAdapter,
  type WorkspaceChatModalPrepareResult,
} from './workspaceChatModalStore';

const TEST_CONVERSATION_ID = '01900000-0000-7000-8000-000000000001';

async function dispatchPreparedWorkspaceChatOpen(tabId: string) {
  const prepared = await prepareWorkspaceChatOpen(tabId);
  if (!prepared) return;
  try {
    const tab = workspaceState.workspace.tabs.find((candidate) => candidate.id === tabId);
    prepared.present(tab?.type === 'chat' ? '' : TEST_CONVERSATION_ID);
  } finally {
    prepared.dispose();
  }
}

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

function changeActiveTab(tabId: string) {
  workspaceState.workspace.activeTabId = tabId;
  workspaceSubscribers.forEach((listener) => listener(workspaceState));
}

function notifyWorkspaceUpdate() {
  workspaceSubscribers.forEach((listener) => listener(workspaceState));
}

describe('workspaceChatModalStore.requestOpen', () => {
  let unregisterDispatcher: (() => void) | undefined;

  beforeEach(() => {
    resetWorkspaceChatModalState();
    mockIsModalOpen.mockReset();
    mockAddToast.mockReset();
    mockGetModalRegistrySnapshot.mockClear();
    fixture.modalGeneration = 0;
    authState.isAuthenticated = true;
    authState.user = { userId: 'user-1', sessionId: 'session-1', role: 'user' };
    workspaceState.workspace.id = 'workspace-1';
    workspaceState.workspace.activeTabId = 'tab-editor';
    workspaceSubscribers.clear();
    mockEnsureWorkspaceTabConversationId.mockClear();
    registerWorkspaceChatModalAdapter('tab-editor', null);
    document.body.innerHTML = '<div class="workspace-layout"></div>';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    unregisterDispatcher = registerWorkspaceChatCommandDispatcher(dispatchPreparedWorkspaceChatOpen);
  });

  afterEach(() => {
    unregisterDispatcher?.();
    unregisterDispatcher = undefined;
    registerWorkspaceChatModalAdapter('tab-editor', null);
    vi.restoreAllMocks();
    document.body.innerHTML = '';
    expect(workspaceSubscribers.size).toBe(0);
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
    expect(useWorkspaceChatModalStore.getState().isOpen).toBe(false);
    expect(mockAddToast).not.toHaveBeenCalled();
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
    changeActiveTab('tab-chat');
    document.body.innerHTML =
      '<div class="workspace-layout"><button data-tab-id="tab-chat">tab button</button><div class="ws-content__panel" data-tab-id="tab-chat" data-active="true"><div class="chat-page"><textarea class="chat-input__textarea"></textarea></div></div></div>';
    const textarea = document.querySelector('.chat-input__textarea') as HTMLTextAreaElement;
    const focusSpy = vi.spyOn(textarea, 'focus').mockImplementation(() => {});

    await useWorkspaceChatModalStore.getState().requestOpen('tab-chat');

    expect(focusSpy).toHaveBeenCalledTimes(1);
    focusSpy.mockRestore();
    document.body.innerHTML = '';
  });

  it('mostra toast quando o painel ativo não tem adaptador', async () => {
    await useWorkspaceChatModalStore.getState().requestOpen('tab-editor');

    expect(mockAddToast).not.toHaveBeenCalled();
    expect(mockEnsureWorkspaceTabConversationId).not.toHaveBeenCalled();
  });

  it('abre com boundTabId do tabId explícito quando prepare() tem sucesso', async () => {
    const meta = { kind: 'test' };
    const prepare = vi.fn().mockResolvedValue({ ok: true, contextDisplay: 'selection', meta });
    registerWorkspaceChatModalAdapter('tab-editor', { prepare, send: vi.fn() });

    await useWorkspaceChatModalStore.getState().requestOpen('tab-editor');

    expect(prepare).toHaveBeenCalledTimes(1);
    expect(mockEnsureWorkspaceTabConversationId).not.toHaveBeenCalled();
    const s = useWorkspaceChatModalStore.getState();
    expect(s.isOpen).toBe(true);
    expect(s.boundTabId).toBe('tab-editor');
    expect(s.boundConversationId).toBe(TEST_CONVERSATION_ID);
    expect(s.boundSurface).toEqual({
      conversationId: TEST_CONVERSATION_ID,
      sessionKey: `modal:workspace-chat:tab-editor:${TEST_CONVERSATION_ID}`,
      surfaceId: 'modal:workspace-chat:tab-editor',
      surfaceType: 'modal',
      tabId: 'tab-editor',
    });
    expect(s.contextDisplay).toBe('selection');
    expect(s.sessionMeta).toEqual(meta);
    expect(typeof s.boundSend).toBe('function');
    expect(workspaceSubscribers.size).toBe(0);
  });

  it('abre usando tabId explícito quando ele é a aba ativa', async () => {
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
    expect(mockEnsureWorkspaceTabConversationId).not.toHaveBeenCalled();
  });

  it('mostra toast de erro quando prepare() lança', async () => {
    const prepare = vi.fn().mockRejectedValue(new Error('boom'));
    registerWorkspaceChatModalAdapter('tab-editor', { prepare, send: vi.fn() });

    await useWorkspaceChatModalStore.getState().requestOpen('tab-editor');

    expect(useWorkspaceChatModalStore.getState().isOpen).toBe(false);
    expect(mockAddToast).toHaveBeenCalledWith('workspace.chatModal.prepareFailed', 'error');
    expect(mockEnsureWorkspaceTabConversationId).not.toHaveBeenCalled();
  });

  it.each([
    ['erro', 'rejeitar'] as const,
    ['resultado recusado', 'recusar'] as const,
  ])('não exibe feedback de prepare após perder o contexto (%s)', async (_label, mode) => {
    let resolvePrepare!: (value: WorkspaceChatModalPrepareResult) => void;
    let rejectPrepare!: (reason?: unknown) => void;
    const prepare = vi.fn(() => new Promise<WorkspaceChatModalPrepareResult>((resolve, reject) => {
      resolvePrepare = resolve;
      rejectPrepare = reject;
    }));
    registerWorkspaceChatModalAdapter('tab-editor', { prepare, send: vi.fn() });

    const pending = useWorkspaceChatModalStore.getState().requestOpen('tab-editor');
    await vi.waitFor(() => expect(prepare).toHaveBeenCalledTimes(1));
    changeActiveTab('tab-chat');
    if (mode === 'rejeitar') rejectPrepare(new Error('stale failure'));
    else resolvePrepare({ ok: false, message: 'stale feedback' });
    await pending;

    expect(mockAddToast).not.toHaveBeenCalled();
    expect(useWorkspaceChatModalStore.getState().isOpen).toBe(false);
  });

  it('descarta abertura quando a aba ativa muda e volta durante prepare (ABA)', async () => {
    let resolvePrepare!: (result: WorkspaceChatModalPrepareResult) => void;
    const prepare = vi.fn(() => new Promise<WorkspaceChatModalPrepareResult>((resolve) => {
      resolvePrepare = resolve;
    }));
    registerWorkspaceChatModalAdapter('tab-editor', { prepare, send: vi.fn() });

    const pending = useWorkspaceChatModalStore.getState().requestOpen('tab-editor');
    await vi.waitFor(() => expect(prepare).toHaveBeenCalledTimes(1));
    changeActiveTab('tab-chat');
    changeActiveTab('tab-editor');
    resolvePrepare({ ok: true, contextDisplay: 'stale', meta: null });
    await pending;

    expect(mockEnsureWorkspaceTabConversationId).not.toHaveBeenCalled();
    expect(useWorkspaceChatModalStore.getState().isOpen).toBe(false);
  });

  it.each(['workspace', 'tab', 'adapter'] as const)('não reabre após remoção/troca e retorno do mesmo %s', async (kind) => {
    let resolvePrepare!: (result: WorkspaceChatModalPrepareResult) => void;
    const adapter = {
      prepare: vi.fn(() => new Promise<WorkspaceChatModalPrepareResult>((resolve) => { resolvePrepare = resolve; })),
      send: vi.fn(),
    };
    registerWorkspaceChatModalAdapter('tab-editor', adapter);
    const pending = useWorkspaceChatModalStore.getState().requestOpen('tab-editor');
    if (kind === 'workspace') {
      workspaceState.workspace.id = 'workspace-other';
      notifyWorkspaceUpdate();
      workspaceState.workspace.id = 'workspace-1';
      notifyWorkspaceUpdate();
    } else if (kind === 'tab') {
      const tabs = workspaceState.workspace.tabs;
      workspaceState.workspace.tabs = tabs.filter((tab) => tab.id !== 'tab-editor');
      notifyWorkspaceUpdate();
      workspaceState.workspace.tabs = tabs;
      notifyWorkspaceUpdate();
    } else {
      registerWorkspaceChatModalAdapter('tab-editor', null);
      registerWorkspaceChatModalAdapter('tab-editor', adapter);
    }
    resolvePrepare({ ok: true, contextDisplay: 'stale', meta: null });
    await pending;
    expect(mockEnsureWorkspaceTabConversationId).not.toHaveBeenCalled();
    expect(useWorkspaceChatModalStore.getState().isOpen).toBe(false);
  });

  it.each([
    ['substituído', { prepare: vi.fn(), send: vi.fn() }],
    ['removido', null],
  ])('descarta abertura quando o adapter é %s durante prepare', async (_label, nextAdapter) => {
    let resolvePrepare!: (result: WorkspaceChatModalPrepareResult) => void;
    const prepare = vi.fn(() => new Promise<WorkspaceChatModalPrepareResult>((resolve) => {
      resolvePrepare = resolve;
    }));
    registerWorkspaceChatModalAdapter('tab-editor', { prepare, send: vi.fn() });

    const pending = useWorkspaceChatModalStore.getState().requestOpen('tab-editor');
    await vi.waitFor(() => expect(prepare).toHaveBeenCalledTimes(1));
    registerWorkspaceChatModalAdapter('tab-editor', nextAdapter);
    resolvePrepare({ ok: true, contextDisplay: 'stale', meta: null });
    await pending;

    expect(mockEnsureWorkspaceTabConversationId).not.toHaveBeenCalled();
    expect(useWorkspaceChatModalStore.getState().isOpen).toBe(false);
  });

  it('descarta abertura quando a stack de modais muda e volta durante prepare (ABA)', async () => {
    let resolvePrepare!: (result: WorkspaceChatModalPrepareResult) => void;
    const prepare = vi.fn(() => new Promise<WorkspaceChatModalPrepareResult>((resolve) => {
      resolvePrepare = resolve;
    }));
    registerWorkspaceChatModalAdapter('tab-editor', { prepare, send: vi.fn() });

    const pending = useWorkspaceChatModalStore.getState().requestOpen('tab-editor');
    await vi.waitFor(() => expect(prepare).toHaveBeenCalledTimes(1));
    fixture.modalGeneration += 1;
    fixture.modalGeneration += 1;
    resolvePrepare({ ok: true, contextDisplay: 'stale', meta: null });
    await pending;

    expect(mockEnsureWorkspaceTabConversationId).not.toHaveBeenCalled();
    expect(useWorkspaceChatModalStore.getState().isOpen).toBe(false);
  });

  it('não abre nem exibe toast após close durante prepare do adapter', async () => {
    let resolvePrepare!: (result: WorkspaceChatModalPrepareResult) => void;
    const prepare = vi.fn().mockResolvedValue({ ok: true, contextDisplay: 'ctx', meta: null });
    prepare.mockImplementationOnce(
      () => new Promise<WorkspaceChatModalPrepareResult>((resolve) => { resolvePrepare = resolve; }),
    );
    registerWorkspaceChatModalAdapter('tab-editor', { prepare, send: vi.fn() });

    const pending = useWorkspaceChatModalStore.getState().requestOpen('tab-editor');
    await vi.waitFor(() => expect(prepare).toHaveBeenCalledTimes(1));
    useWorkspaceChatModalStore.getState().close();
    resolvePrepare({ ok: true, contextDisplay: 'ctx', meta: null });
    await pending;

    expect(useWorkspaceChatModalStore.getState().isOpen).toBe(false);
    expect(mockAddToast).not.toHaveBeenCalled();
  });

  it('uma nova solicitação invalida a abertura anterior', async () => {
    let resolveFirst!: (result: WorkspaceChatModalPrepareResult) => void;
    const firstPrepare = vi.fn(() => new Promise<WorkspaceChatModalPrepareResult>((resolve) => {
      resolveFirst = resolve;
    }));
    registerWorkspaceChatModalAdapter('tab-editor', { prepare: firstPrepare, send: vi.fn() });
    const first = useWorkspaceChatModalStore.getState().requestOpen('tab-editor');
    await vi.waitFor(() => expect(firstPrepare).toHaveBeenCalledTimes(1));

    const secondPrepare = vi.fn().mockResolvedValue({ ok: true, contextDisplay: 'new', meta: null });
    registerWorkspaceChatModalAdapter('tab-editor', { prepare: secondPrepare, send: vi.fn() });
    const second = useWorkspaceChatModalStore.getState().requestOpen('tab-editor');
    resolveFirst({ ok: true, contextDisplay: 'old', meta: null });
    await Promise.all([first, second]);

    expect(secondPrepare).toHaveBeenCalledTimes(1);
    expect(useWorkspaceChatModalStore.getState().contextDisplay).toBe('new');
    expect(useWorkspaceChatModalStore.getState().isOpen).toBe(true);
  });

  it('listener antigo não invalida nova solicitação após troca de workspace e atualização comum', async () => {
    let resolveFirst!: (result: WorkspaceChatModalPrepareResult) => void;
    let prepareCalls = 0;
    const prepare = vi.fn(() => {
      prepareCalls += 1;
      if (prepareCalls === 1) {
        return new Promise<WorkspaceChatModalPrepareResult>((resolve) => { resolveFirst = resolve; });
      }
      return Promise.resolve({ ok: true as const, contextDisplay: 'new', meta: null });
    });
    registerWorkspaceChatModalAdapter('tab-editor', { prepare, send: vi.fn() });

    const first = useWorkspaceChatModalStore.getState().requestOpen('tab-editor');
    await vi.waitFor(() => expect(prepare).toHaveBeenCalledTimes(1));
    workspaceState.workspace.id = 'workspace-2';
    notifyWorkspaceUpdate();

    const second = useWorkspaceChatModalStore.getState().requestOpen('tab-editor');
    notifyWorkspaceUpdate();
    resolveFirst({ ok: true, contextDisplay: 'old', meta: null });
    await Promise.all([first, second]);

    expect(mockEnsureWorkspaceTabConversationId).not.toHaveBeenCalled();
    expect(useWorkspaceChatModalStore.getState().contextDisplay).toBe('new');
    expect(useWorkspaceChatModalStore.getState().isOpen).toBe(true);
  });

  it('open direto invalida uma solicitação pendente', async () => {
    let resolvePrepare!: (result: WorkspaceChatModalPrepareResult) => void;
    const prepare = vi.fn(() => new Promise<WorkspaceChatModalPrepareResult>((resolve) => {
      resolvePrepare = resolve;
    }));
    registerWorkspaceChatModalAdapter('tab-editor', { prepare, send: vi.fn() });

    const pending = useWorkspaceChatModalStore.getState().requestOpen('tab-editor');
    await vi.waitFor(() => expect(prepare).toHaveBeenCalledTimes(1));
    useWorkspaceChatModalStore.getState().open(
      'direct',
      null,
      'tab-editor',
      'existing',
      {
        conversationId: 'existing',
        sessionKey: 'modal:workspace-chat:tab-editor:existing',
        surfaceId: 'modal:workspace-chat:tab-editor',
        surfaceType: 'modal',
        tabId: 'tab-editor',
      },
      vi.fn(),
    );
    resolvePrepare({ ok: true, contextDisplay: 'old', meta: null });
    await pending;

    expect(useWorkspaceChatModalStore.getState().contextDisplay).toBe('direct');
    expect(useWorkspaceChatModalStore.getState().boundConversationId).toBe('existing');
  });
});

describe('workspaceChatModalStore.setBoundConversation', () => {
  beforeEach(() => {
    resetWorkspaceChatModalState();
    mockUpdateTab.mockClear();
    mockUpdateTab.mockResolvedValue(undefined);
    mockAddToast.mockReset();
  });

  const openModalBoundTo = (conversationId: string, tabId = 'tab-editor') => {
    useWorkspaceChatModalStore.getState().open('ctx', {}, tabId, conversationId, {
      conversationId,
      sessionKey: `modal:workspace-chat:${tabId}:${conversationId}`,
      surfaceId: `modal:workspace-chat:${tabId}`,
      surfaceType: 'modal',
      tabId,
    }, vi.fn());
  };

  it('é no-op quando o modal está fechado', () => {
    useWorkspaceChatModalStore.getState().setBoundConversation('99');
    const s = useWorkspaceChatModalStore.getState();
    expect(s.boundConversationId).toBeNull();
    expect(mockUpdateTab).not.toHaveBeenCalled();
  });

  it('é no-op quando a conversa é a mesma', () => {
    openModalBoundTo('1');
    useWorkspaceChatModalStore.getState().setBoundConversation('1');
    expect(mockUpdateTab).not.toHaveBeenCalled();
  });

  it('recria a superfície, atualiza boundConversationId e persiste o vínculo na aba', async () => {
    openModalBoundTo('1');

    useWorkspaceChatModalStore.getState().setBoundConversation('2');

    const s = useWorkspaceChatModalStore.getState();
    expect(s.boundConversationId).toBe('2');
    expect(s.boundSurface).toEqual({
      conversationId: '2',
      sessionKey: 'modal:workspace-chat:tab-editor:2',
      surfaceId: 'modal:workspace-chat:tab-editor',
      surfaceType: 'modal',
      tabId: 'tab-editor',
    });
    await vi.waitFor(() => {
      expect(mockUpdateTab).toHaveBeenCalledWith('tab-editor', { conversation_id: '2' });
    });
  });

  it('serializa a persistência em trocas rápidas (latest-wins, sem fora de ordem)', async () => {
    openModalBoundTo('1');

    // 1ª persistência lenta: enquanto pendente, fazem-se mais duas trocas.
    let resolveFirst!: () => void;
    mockUpdateTab.mockImplementationOnce(
      () => new Promise<void>((resolve) => { resolveFirst = resolve; }),
    );

    useWorkspaceChatModalStore.getState().setBoundConversation('2');
    await vi.waitFor(() => expect(mockUpdateTab).toHaveBeenCalledTimes(1));

    useWorkspaceChatModalStore.getState().setBoundConversation('3');
    useWorkspaceChatModalStore.getState().setBoundConversation('4');

    // Enquanto a 1ª não resolve, nada mais é persistido (escritas encadeadas).
    expect(mockUpdateTab).toHaveBeenCalledTimes(1);
    expect(mockUpdateTab).toHaveBeenLastCalledWith('tab-editor', { conversation_id: '2' });

    resolveFirst();

    // Ao liberar a fila, a troca intermediária ('3') é pulada: persiste só a última.
    await vi.waitFor(() => {
      expect(mockUpdateTab).toHaveBeenLastCalledWith('tab-editor', { conversation_id: '4' });
    });
    expect(mockUpdateTab).toHaveBeenCalledTimes(2);
    expect(useWorkspaceChatModalStore.getState().boundConversationId).toBe('4');
  });

  it('mostra toast de erro quando a persistência do vínculo falha', async () => {
    openModalBoundTo('1');
    mockUpdateTab.mockRejectedValueOnce(new Error('backend down'));

    useWorkspaceChatModalStore.getState().setBoundConversation('2');

    await vi.waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith('chat.switchError', 'error');
    });
    // A troca visual otimista permanece; o toast informa que o vínculo não persistiu.
    expect(useWorkspaceChatModalStore.getState().boundConversationId).toBe('2');
  });

  it('latest-wins é por aba: troca em outra aba não invalida persistência pendente', async () => {
    openModalBoundTo('1', 'tab-editor');

    // Persistência da aba A fica pendente na fila.
    let resolveFirst!: () => void;
    mockUpdateTab.mockImplementationOnce(
      () => new Promise<void>((resolve) => { resolveFirst = resolve; }),
    );
    useWorkspaceChatModalStore.getState().setBoundConversation('2');
    await vi.waitFor(() => expect(mockUpdateTab).toHaveBeenCalledTimes(1));

    // Modal é fechado e reaberto vinculado a outra aba; nova troca lá.
    useWorkspaceChatModalStore.getState().close();
    openModalBoundTo('10', 'tab-chat');
    useWorkspaceChatModalStore.getState().setBoundConversation('11');

    // Cadeias são independentes por aba: a persistência da aba B acontece sem
    // esperar a pendente da aba A, e não a invalida.
    await vi.waitFor(() => {
      expect(mockUpdateTab).toHaveBeenCalledWith('tab-chat', { conversation_id: '11' });
    });
    expect(mockUpdateTab).toHaveBeenCalledWith('tab-editor', { conversation_id: '2' });
    expect(mockUpdateTab).toHaveBeenCalledTimes(2);

    resolveFirst();
  });
});
