import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MediaCategory } from '../services/mediaService';

// ── Mocks ──────────────────────────────────────────────────────────────

const mockAnnounce = vi.fn();
vi.mock('../hooks/useAnnouncer', () => ({
  announce: (...args: unknown[]) => mockAnnounce(...args),
}));

const mockSendMessage = vi.fn().mockResolvedValue(undefined);
const mockRetryMessage = vi.fn().mockResolvedValue(undefined);
const mockGetMessages = vi.fn().mockResolvedValue([]);
const mockGetRecentMessages = vi.fn().mockResolvedValue([]);
const mockGetMessagesBefore = vi.fn().mockResolvedValue([]);
const mockGetConversationMessageWindow = vi.fn().mockResolvedValue({
  scope: 'conversation',
  conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
  nodes: [],
  totalCount: 0,
  startIndex: 0,
  endIndex: -1,
  hasBefore: false,
  hasAfter: false,
});
const mockGetConversationInfo = vi.fn().mockResolvedValue({});
vi.mock('@wailsjs/go/app/App', () => ({
  SendMessage: (...args: unknown[]) => mockSendMessage(...args),
  RetryMessage: (...args: unknown[]) => mockRetryMessage(...args),
  GetMessages: (...args: unknown[]) => mockGetMessages(...args),
  GetRecentMessages: (...args: unknown[]) => mockGetRecentMessages(...args),
  GetMessagesBefore: (...args: unknown[]) => mockGetMessagesBefore(...args),
  GetConversationMessageWindow: (...args: unknown[]) => mockGetConversationMessageWindow(...args),
  GetConversationInfo: (...args: unknown[]) => mockGetConversationInfo(...args),
  EnsureConversation: vi.fn().mockResolvedValue("01926b90-7a5a-7c4e-8d3f-000000000001"),
  AssignConversationToChannel: vi.fn(),
  UnassignConversationFromChannel: vi.fn(),
  GetMessageChildren: vi.fn().mockResolvedValue([]),
}));

type EventCallback = (...args: unknown[]) => void;
const eventListeners: Map<string, EventCallback[]> = new Map();

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (event: string, cb: EventCallback) => {
    const list = eventListeners.get(event) || [];
    list.push(cb);
    eventListeners.set(event, list);
    return () => {
      const l = eventListeners.get(event);
      if (l) eventListeners.set(event, l.filter((fn) => fn !== cb));
    };
  },
}));

vi.mock('../services/audioFeedback', () => ({
  playSendSound: vi.fn(),
  playReceiveSound: vi.fn(),
}));

vi.mock('../services/tts', () => ({
  ttsService: {
    isEnabledForUser: () => false,
    shouldUseAriaLiveForUser: () => false,
    isAutoReadEnabled: () => false,
    stop: vi.fn(),
    getVolume: () => 1,
    getVoiceContext: () => ({}),
    speakAsRole: vi.fn().mockResolvedValue(undefined),
    shouldUseAriaLiveForAgent: () => false,
  },
}));

vi.mock('../services/messageAudio', () => ({
  messageAudioService: {
    stopCurrentAudio: vi.fn(),
    speakMessage: vi.fn().mockResolvedValue(false),
  },
}));

const mockHandleChatSpeak = vi.fn();
vi.mock('../services/chatSpeak', () => ({
  handleChatSpeak: (...args: unknown[]) => Promise.resolve(mockHandleChatSpeak(...args)),
}));

const defaultConversationId = "01926b90-7a5a-7c4e-8d3f-000000000001";

function createMessageNode(id: string, conversationId = defaultConversationId) {
  return {
    message: {
      id,
      conversationId,
      role: 'user',
      content: id,
      createdAt: new Date().toISOString(),
    },
    children: [],
    childCount: 0,
  };
}

vi.mock('i18next', () => ({
  default: {
    t: (key: string, opts?: Record<string, unknown>) => {
      if (typeof opts === 'object' && opts.defaultValue) return opts.defaultValue;
      return key;
    },
  },
}));

function emitEvent(name: string, data: unknown) {
  const cbs = eventListeners.get(name) || [];
  for (const cb of cbs) cb(data);
}

function deferred<T>() {
  let resolve!: (value?: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = (value?: T) => res(value as T);
    reject = rej;
  });
  return { promise, resolve, reject };
}

async function flushMicrotasks() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}

describe('chatStore validation', () => {
  type MessageNode = import('./chatStore').MessageNode;
  let useChatStore: typeof import('./chatStore').useChatStore;

  beforeEach(async () => {
    vi.resetModules();
    eventListeners.clear();
    mockAnnounce.mockClear();
    mockSendMessage.mockClear();
    mockRetryMessage.mockClear();
    mockGetMessages.mockReset();
    mockGetMessages.mockResolvedValue([]);
    mockGetRecentMessages.mockReset();
    mockGetRecentMessages.mockResolvedValue([]);
    mockGetMessagesBefore.mockReset();
    mockGetMessagesBefore.mockResolvedValue([]);
    mockGetConversationMessageWindow.mockReset();
    mockGetConversationMessageWindow.mockResolvedValue({
      scope: 'conversation',
      conversationId: defaultConversationId,
      nodes: [],
      totalCount: 0,
      startIndex: 0,
      endIndex: -1,
      hasBefore: false,
      hasAfter: false,
    });
    mockGetConversationInfo.mockReset();
    mockGetConversationInfo.mockResolvedValue({});
    mockHandleChatSpeak.mockClear();
    const mod = await import('./chatStore');
    useChatStore = mod.useChatStore;
    useChatStore.setState({
      sessionsByConversationId: {
        [defaultConversationId]: {
          sessionKey: `conversation:${defaultConversationId}`,
          conversationId: defaultConversationId,
          conversation: { id: defaultConversationId, title: 'Conversa', threadedMessages: [] },
          isLoading: false,
          hasOlderMessages: false,
          isLoadingOlderMessages: false,
          isLoadingMessageWindow: false,
          streamingMessageId: null,
          streamingReasoning: null,
          isThinking: false,
          activeToolCalls: [],
          completedSegments: [],
          draftMessage: '',
          draftMediaFiles: [],
          scrollTop: 0,
          scrollAnchorMessageId: null,
          expandedThreads: new Set<string>(),
          expandedReasonings: new Set<string>(),
          editingMessageId: null,
          readingMessageId: null,
          skipFocusRestore: false,
        },
      },
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('rejects message exceeding max content size', async () => {
    const bigContent = 'x'.repeat(512 * 1024 + 1);

    await useChatStore.getState().sendMessageToConversation(defaultConversationId, bigContent);

    expect(mockAnnounce).toHaveBeenCalledTimes(1);
    expect(mockAnnounce.mock.calls[0][0]).toContain('grande');
    expect(mockSendMessage).not.toHaveBeenCalled();
  });

  it('accepts message at exact max content size', async () => {
    const exactContent = 'x'.repeat(512 * 1024);

    await useChatStore.getState().sendMessageToConversation(defaultConversationId, exactContent);

    expect(mockSendMessage).toHaveBeenCalled();
  });

  it('retry de mensagem existente usa RetryMessage sem criar novo SendMessage', async () => {
    await useChatStore.getState().retryMessageToConversation("1", "42");

    expect(mockRetryMessage).toHaveBeenCalledWith("1", "42", expect.any(Object));
    expect(mockSendMessage).not.toHaveBeenCalled();
  });

  it('rejects media exceeding max size', async () => {
    const fakeFile = new File([new ArrayBuffer(15 * 1024 * 1024)], 'big.bin', { type: 'application/octet-stream' });

    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'hello', [{
      id: 'test-1',
      file: fakeFile,
      category: MediaCategory.DOCUMENT,
      mimeType: 'application/octet-stream',
      extension: '.bin',
      fileName: 'big.bin',
      fileSize: fakeFile.size,
      fileSizeFormatted: '15 MB',
      icon: '📄',
      preview: '',
    }]);

    expect(mockAnnounce).toHaveBeenCalledTimes(1);
    expect(mockAnnounce.mock.calls[0][0]).toContain('mídia');
    expect(mockSendMessage).not.toHaveBeenCalled();
  });

  it('handles chat:error event from backend', async () => {
    // Simulate real Wails behavior: backend emits chat:error, then returns error
    mockSendMessage.mockImplementation(() => {
      emitEvent('chat:error', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", error: 'Provedor LLM não disponível' });
      return Promise.reject(new Error('backend error'));
    });

    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'hello');

    expect(mockAnnounce).toHaveBeenCalledWith('Provedor LLM não disponível');
    expect(useChatStore.getState().sessionsByConversationId[defaultConversationId]?.isLoading).toBe(false);
  });

  it('sendMessageToConversation envia usando o conversationId explícito', async () => {
    await useChatStore.getState().sendMessageToConversation("01926b90-7a5a-7c4e-8d3f-000000000007", 'hello');

    expect(mockSendMessage).toHaveBeenCalledWith("01926b90-7a5a-7c4e-8d3f-000000000007", 'hello', '', expect.any(Object));
  });

  it('carrega janela anterior apenas para a superfície solicitada', async () => {
    const currentNode = createMessageNode('message-10') as unknown as MessageNode;
    const olderNode = createMessageNode('message-09') as unknown as MessageNode;
    mockGetConversationMessageWindow.mockResolvedValue({
      scope: 'conversation',
      conversationId: defaultConversationId,
      nodes: [olderNode],
      totalCount: 10,
      startIndex: 8,
      endIndex: 8,
      hasBefore: true,
      hasAfter: true,
    });

    const baseSurface = {
      conversationId: defaultConversationId,
      queuedTurnCount: 0,
      isLoading: false,
      hasOlderMessages: true,
      isLoadingOlderMessages: false,
      isLoadingMessageWindow: false,
      streamingMessageId: null,
      streamingReasoning: null,
      isThinking: false,
      activeToolCalls: [],
      completedSegments: [],
      draftMessage: '',
      draftMediaFiles: [],
      scrollTop: 0,
      scrollAnchorMessageId: null,
      expandedThreads: new Set<string>(),
      expandedReasonings: new Set<string>(),
      editingMessageId: null,
      readingMessageId: null,
      skipFocusRestore: false,
      visibleThreadedMessages: [currentNode],
      messageWindow: {
        scope: 'conversation' as const,
        conversationId: defaultConversationId,
        totalCount: 10,
        startIndex: 9,
        endIndex: 9,
        hasBefore: true,
        hasAfter: false,
      },
    };

    const firstNode = createMessageNode('first-message') as unknown as MessageNode;
    firstNode.originalIndex = 1;

    useChatStore.setState({
      timelinesByConversationId: {
        [defaultConversationId]: {
          id: defaultConversationId,
          title: 'Conversa',
          threadedMessages: [currentNode],
        },
      },
      surfaceSessionsByKey: {
        'surface-a': { ...baseSurface, sessionKey: 'surface-a' },
        'surface-b': { ...baseSurface, sessionKey: 'surface-b' },
      },
    });

    await useChatStore.getState().loadOlderMessagesForConversation(defaultConversationId, 'surface-a');

    expect(mockGetConversationMessageWindow).toHaveBeenCalledWith(expect.objectContaining({
      anchorMessageId: 'message-10',
      direction: 'before',
    }));
    const state = useChatStore.getState();
    expect(state.surfaceSessionsByKey['surface-a'].visibleThreadedMessages?.map((node) => node.message.id))
      .toEqual(['message-09', 'message-10']);
    expect(state.surfaceSessionsByKey['surface-b'].visibleThreadedMessages?.map((node) => node.message.id))
      .toEqual(['message-10']);
  });

  it('repassa parâmetros estruturados de surface no envio', async () => {
    await useChatStore.getState().sendMessageToConversation("01926b90-7a5a-7c4e-8d3f-000000000007", 'hello', undefined, {
      tabType: 'editor',
      activeFilePath: '/tmp/readme.md',
      surfaceStateJson: '{"filePath":"/tmp/readme.md"}',
      surfaceContextJson: '{"selectedText":"hello"}',
    });

    expect(mockSendMessage).toHaveBeenCalledWith("01926b90-7a5a-7c4e-8d3f-000000000007", 'hello', '', expect.objectContaining({
      tabType: 'editor',
      activeFilePath: '/tmp/readme.md',
      surfaceStateJson: '{"filePath":"/tmp/readme.md"}',
      surfaceContextJson: '{"selectedText":"hello"}',
    }));
  });

  it('registra origem de superfície no controller de envio', async () => {
    const origin = {
      sessionKey: 'tab-chat:01926b90-7a5a-7c4e-8d3f-000000000001',
      conversationId: defaultConversationId,
      tabId: 'tab-chat',
      surfaceId: 'tab-chat',
      surfaceType: 'page' as const,
    };

    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'hello', undefined, undefined, { origin });

    expect(useChatStore.getState().surfaceSessionsByKey[origin.sessionKey]?.surfaceOrigin).toEqual(origin);
  });

  it('propaga eventos sem origem para superfícies existentes da conversa', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    useChatStore.setState({
      timelinesByConversationId: {
        [defaultConversationId]: { id: defaultConversationId, title: 'Conversa', threadedMessages: [] },
      },
      surfaceSessionsByKey: {
        [`tab-a:${defaultConversationId}`]: createEmptyChatSession(defaultConversationId, `tab-a:${defaultConversationId}`),
        [`tab-b:${defaultConversationId}`]: createEmptyChatSession(defaultConversationId, `tab-b:${defaultConversationId}`),
      },
    });

    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'hello');
    emitEvent('chat:tool_start', {
      conversationId: defaultConversationId,
      name: 'search_web',
      callId: 'tool-1',
      args: '{}',
    });
    await flushMicrotasks();

    expect(useChatStore.getState().surfaceSessionsByKey[`tab-a:${defaultConversationId}`]?.activeToolCalls[0]).toMatchObject({
      callId: 'tool-1',
      status: 'running',
    });
    expect(useChatStore.getState().surfaceSessionsByKey[`tab-b:${defaultConversationId}`]?.activeToolCalls[0]).toMatchObject({
      callId: 'tool-1',
      status: 'running',
    });
  });

  it('remove timeline e superfícies órfãs ao deletar conversa', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const orphanConversationId = '01926b90-7a5a-7c4e-8d3f-000000000099';
    const orphanSessionKey = `tab-chat:${orphanConversationId}`;
    useChatStore.setState({
      sessionsByConversationId: {},
      timelinesByConversationId: {
        [orphanConversationId]: { id: orphanConversationId, title: 'Órfã', threadedMessages: [] },
      },
      surfaceSessionsByKey: {
        [orphanSessionKey]: createEmptyChatSession(orphanConversationId, orphanSessionKey),
      },
    });

    useChatStore.getState().handleConversationDeleted(orphanConversationId);

    expect(useChatStore.getState().timelinesByConversationId[orphanConversationId]).toBeUndefined();
    expect(useChatStore.getState().surfaceSessionsByKey[orphanSessionKey]).toBeUndefined();
  });

  it('envia sem recarregar quando timeline já está carregada sem sessão legada', async () => {
    useChatStore.setState({
      sessionsByConversationId: {},
      timelinesByConversationId: {
        [defaultConversationId]: { id: defaultConversationId, title: 'Conversa', threadedMessages: [] },
      },
      surfaceSessionsByKey: {},
    });

    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'hello');

    expect(mockGetConversationInfo).not.toHaveBeenCalled();
    expect(mockGetMessages).not.toHaveBeenCalled();
    expect(mockGetRecentMessages).not.toHaveBeenCalled();
    expect(mockSendMessage).toHaveBeenCalledWith(defaultConversationId, 'hello', '', expect.any(Object));
  });

  it('faz retry sem recarregar quando timeline já está carregada sem sessão legada', async () => {
    useChatStore.setState({
      sessionsByConversationId: {},
      timelinesByConversationId: {
        [defaultConversationId]: { id: defaultConversationId, title: 'Conversa', threadedMessages: [] },
      },
      surfaceSessionsByKey: {},
    });

    await useChatStore.getState().retryMessageToConversation(defaultConversationId, 'message-1');

    expect(mockGetConversationInfo).not.toHaveBeenCalled();
    expect(mockGetMessages).not.toHaveBeenCalled();
    expect(mockGetRecentMessages).not.toHaveBeenCalled();
    expect(mockRetryMessage).toHaveBeenCalledWith(defaultConversationId, 'message-1', expect.any(Object));
  });

  it('reset do banco invalida turnos pendentes', async () => {
    const firstSend = deferred<void>();
    mockSendMessage.mockImplementationOnce(() => firstSend.promise);

    const first = useChatStore.getState().sendMessageToConversation(defaultConversationId, 'primeira');
    const second = useChatStore.getState().sendMessageToConversation(defaultConversationId, 'segunda');

    await flushMicrotasks();
    expect(mockSendMessage).toHaveBeenCalledTimes(1);

    useChatStore.getState().handleDatabaseReset();
    firstSend.resolve();

    await first;
    await second;

    expect(mockSendMessage).toHaveBeenCalledTimes(1);
    expect(useChatStore.getState().sessionsByConversationId).toEqual({});
    expect(useChatStore.getState().timelinesByConversationId).toEqual({});
    expect(useChatStore.getState().surfaceSessionsByKey).toEqual({});
  });

  it('propaga fim de loading para superfícies materializadas da conversa', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const surfaceSessionKey = `tab-chat:${defaultConversationId}`;
    useChatStore.setState({
      loadingConversationIds: new Set([defaultConversationId]),
      surfaceSessionsByKey: {
        [surfaceSessionKey]: {
          ...createEmptyChatSession(defaultConversationId, surfaceSessionKey),
          isLoading: true,
        },
      },
    });

    useChatStore.getState().cancelConversationTurn(defaultConversationId);

    expect(useChatStore.getState().loadingConversationIds.has(defaultConversationId)).toBe(false);
    expect(useChatStore.getState().surfaceSessionsByKey[surfaceSessionKey]?.isLoading).toBe(false);
  });

  it('mantém loading visual restrito à superfície de origem', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const send = deferred<void>();
    const originSessionKey = `tab-a:${defaultConversationId}`;
    const otherSessionKey = `tab-b:${defaultConversationId}`;
    mockSendMessage.mockImplementationOnce(() => send.promise);
    useChatStore.setState({
      surfaceSessionsByKey: {
        [originSessionKey]: createEmptyChatSession(defaultConversationId, originSessionKey),
        [otherSessionKey]: createEmptyChatSession(defaultConversationId, otherSessionKey),
      },
    });

    const pending = useChatStore.getState().sendMessageToConversation(
      defaultConversationId,
      'mensagem',
      undefined,
      undefined,
      {
        origin: {
          surfaceId: 'tab-a',
          surfaceType: 'page',
          sessionKey: originSessionKey,
          conversationId: defaultConversationId,
        },
      },
    );

    await flushMicrotasks();

    const state = useChatStore.getState();
    expect(state.surfaceSessionsByKey[originSessionKey]?.isLoading).toBe(true);
    expect(state.surfaceSessionsByKey[otherSessionKey]?.isLoading).toBe(false);

    emitEvent('chat:done', { conversationId: defaultConversationId });
    send.resolve();
    await pending;
  });

  it('mantém rascunho e anexos isolados por superfície', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `tab-a:${defaultConversationId}`;
    const otherSessionKey = `tab-b:${defaultConversationId}`;
    const mediaFile = {
      id: 'media-1',
    } as unknown as import('../services/mediaService').MediaFile;
    useChatStore.setState({
      surfaceSessionsByKey: {
        [originSessionKey]: createEmptyChatSession(defaultConversationId, originSessionKey),
        [otherSessionKey]: createEmptyChatSession(defaultConversationId, otherSessionKey),
      },
    });
    const sessionsBefore = useChatStore.getState().sessionsByConversationId;

    useChatStore.getState().setConversationDraftMessage(defaultConversationId, 'rascunho A', originSessionKey);
    useChatStore.getState().setConversationDraftMediaFiles(defaultConversationId, [mediaFile], originSessionKey);

    const state = useChatStore.getState();
    expect(state.sessionsByConversationId).toBe(sessionsBefore);
    expect(state.surfaceSessionsByKey[originSessionKey]?.draftMessage).toBe('rascunho A');
    expect(state.surfaceSessionsByKey[originSessionKey]?.draftMediaFiles).toEqual([mediaFile]);
    expect(state.surfaceSessionsByKey[otherSessionKey]?.draftMessage).toBe('');
    expect(state.surfaceSessionsByKey[otherSessionKey]?.draftMediaFiles).toEqual([]);
  });

  it('mantém scroll e âncora visual isolados por superfície', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `tab-a:${defaultConversationId}`;
    const otherSessionKey = `tab-b:${defaultConversationId}`;
    useChatStore.setState({
      surfaceSessionsByKey: {
        [originSessionKey]: createEmptyChatSession(defaultConversationId, originSessionKey),
        [otherSessionKey]: createEmptyChatSession(defaultConversationId, otherSessionKey),
      },
    });
    const sessionsBefore = useChatStore.getState().sessionsByConversationId;

    useChatStore.getState().setConversationScrollState(
      defaultConversationId,
      { scrollTop: 320, scrollAnchorMessageId: 'message-7' },
      originSessionKey,
    );

    const state = useChatStore.getState();
    expect(state.sessionsByConversationId).toBe(sessionsBefore);
    expect(state.surfaceSessionsByKey[originSessionKey]?.scrollTop).toBe(320);
    expect(state.surfaceSessionsByKey[originSessionKey]?.scrollAnchorMessageId).toBe('message-7');
    expect(state.surfaceSessionsByKey[otherSessionKey]?.scrollTop).toBe(0);
    expect(state.surfaceSessionsByKey[otherSessionKey]?.scrollAnchorMessageId).toBeNull();
  });

  it('não recria estado ao persistir scroll visual idêntico', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `tab-a:${defaultConversationId}`;
    useChatStore.setState({
      surfaceSessionsByKey: {
        [originSessionKey]: {
          ...createEmptyChatSession(defaultConversationId, originSessionKey),
          scrollTop: 320,
          scrollAnchorMessageId: 'message-7',
        },
      },
    });
    const stateBefore = useChatStore.getState();

    useChatStore.getState().setConversationScrollState(
      defaultConversationId,
      { scrollTop: 320, scrollAnchorMessageId: 'message-7' },
      originSessionKey,
    );

    const stateAfter = useChatStore.getState();
    expect(stateAfter).toBe(stateBefore);
    expect(stateAfter.surfaceSessionsByKey).toBe(stateBefore.surfaceSessionsByKey);
  });

  it('não recria estado ao limpar rascunho já vazio', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `tab-a:${defaultConversationId}`;
    useChatStore.setState({
      surfaceSessionsByKey: {
        [originSessionKey]: createEmptyChatSession(defaultConversationId, originSessionKey),
      },
    });
    const stateBefore = useChatStore.getState();

    useChatStore.getState().clearConversationDraft(defaultConversationId, originSessionKey);

    const stateAfter = useChatStore.getState();
    expect(stateAfter).toBe(stateBefore);
    expect(stateAfter.surfaceSessionsByKey).toBe(stateBefore.surfaceSessionsByKey);
  });

  it('completa origem de superfície já materializada sem recriar compatibilidade', () => {
    const originSessionKey = `tab-a:${defaultConversationId}`;
    const origin = {
      sessionKey: originSessionKey,
      conversationId: defaultConversationId,
      tabId: 'tab-a',
      surfaceId: 'tab-a',
      surfaceType: 'embedded' as const,
    };
    useChatStore.getState().setConversationDraftMessage(defaultConversationId, 'rascunho A', originSessionKey);
    const sessionsBefore = useChatStore.getState().sessionsByConversationId;

    useChatStore.getState().ensureConversationSurfaceSession(defaultConversationId, originSessionKey, origin);

    const state = useChatStore.getState();
    expect(state.sessionsByConversationId).toBe(sessionsBefore);
    expect(state.surfaceSessionsByKey[originSessionKey]?.draftMessage).toBe('rascunho A');
    expect(state.surfaceSessionsByKey[originSessionKey]?.surfaceOrigin).toEqual(origin);
  });

  it('mantém flags de paginação independentes nas superfícies ao carregar conversa', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const surfaceSessionKey = `tab-a:${defaultConversationId}`;
    mockGetConversationInfo.mockResolvedValueOnce({ title: 'Conversa carregada' });
    mockGetConversationMessageWindow.mockResolvedValueOnce({
      scope: 'conversation',
      conversationId: defaultConversationId,
      nodes: [],
      totalCount: 0,
      startIndex: 0,
      endIndex: -1,
      hasBefore: false,
      hasAfter: false,
    });
    useChatStore.setState({
      surfaceSessionsByKey: {
        [surfaceSessionKey]: {
          ...createEmptyChatSession(defaultConversationId, surfaceSessionKey),
          hasOlderMessages: true,
          isLoadingOlderMessages: true,
        },
      },
    });

    await useChatStore.getState().loadConversationSession(defaultConversationId);

    const surfaceSession = useChatStore.getState().surfaceSessionsByKey[surfaceSessionKey];
    expect(surfaceSession?.hasOlderMessages).toBe(false);
    expect(surfaceSession?.isLoadingOlderMessages).toBe(false);
    expect(surfaceSession?.messageWindow?.totalCount).toBe(0);
  });

  it('preserva cache carregado e atualiza superfícies que estavam no fim ao recarregar conversa', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const scrolledSessionKey = `tab-history:${defaultConversationId}`;
    const tailSessionKey = `tab-tail:${defaultConversationId}`;
    const olderNode = createMessageNode('older-visible') as unknown as MessageNode;
    olderNode.originalIndex = 0;
    olderNode.message.createdAt = new Date(Date.now() - 2_000).toISOString();
    const oldTailNode = createMessageNode('old-tail') as unknown as MessageNode;
    oldTailNode.originalIndex = 1;
    oldTailNode.message.createdAt = new Date(Date.now() - 1_000).toISOString();
    const freshTailNode = createMessageNode('fresh-tail') as unknown as MessageNode;
    freshTailNode.originalIndex = 2;
    freshTailNode.message.createdAt = new Date().toISOString();
    mockGetConversationInfo.mockResolvedValueOnce({ title: 'Conversa carregada' });
    mockGetConversationMessageWindow.mockResolvedValueOnce({
      scope: 'conversation',
      conversationId: defaultConversationId,
      nodes: [freshTailNode],
      totalCount: 3,
      startIndex: 2,
      endIndex: 2,
      hasBefore: true,
      hasAfter: false,
    });
    useChatStore.setState({
      timelinesByConversationId: {
        [defaultConversationId]: {
          id: defaultConversationId,
          title: 'Conversa',
          threadedMessages: [olderNode, oldTailNode],
        },
      },
      surfaceSessionsByKey: {
        [scrolledSessionKey]: {
          ...createEmptyChatSession(defaultConversationId, scrolledSessionKey),
          visibleThreadedMessages: [olderNode],
          messageWindow: {
            scope: 'conversation',
            conversationId: defaultConversationId,
            totalCount: 3,
            startIndex: 0,
            endIndex: 0,
            hasBefore: false,
            hasAfter: true,
          },
        },
        [tailSessionKey]: {
          ...createEmptyChatSession(defaultConversationId, tailSessionKey),
          visibleThreadedMessages: [oldTailNode],
          messageWindow: {
            scope: 'conversation',
            conversationId: defaultConversationId,
            totalCount: 2,
            startIndex: 1,
            endIndex: 1,
            hasBefore: true,
            hasAfter: false,
          },
        },
      },
    });

    await useChatStore.getState().loadConversationSession(defaultConversationId);

    const state = useChatStore.getState();
    expect(state.timelinesByConversationId[defaultConversationId]?.threadedMessages.map((node) => node.message.id))
      .toEqual(['older-visible', 'old-tail', 'fresh-tail']);
    expect(state.surfaceSessionsByKey[scrolledSessionKey]?.visibleThreadedMessages?.map((node) => node.message.id))
      .toEqual(['older-visible']);
    expect(state.surfaceSessionsByKey[tailSessionKey]?.visibleThreadedMessages?.map((node) => node.message.id))
      .toEqual(['fresh-tail']);
  });

  it('carrega mensagens antigas usando a sessão de superfície chamadora', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `tab-a:${defaultConversationId}`;
    const otherSessionKey = `tab-b:${defaultConversationId}`;
    const olderNode = createMessageNode('older-message') as unknown as MessageNode;
    olderNode.message.createdAt = new Date(Date.now() - 1_000).toISOString();
    mockGetConversationMessageWindow.mockResolvedValueOnce({
      scope: 'conversation',
      conversationId: defaultConversationId,
      nodes: [olderNode],
      totalCount: 2,
      startIndex: 0,
      endIndex: 0,
      hasBefore: false,
      hasAfter: true,
    });
    const firstNode = createMessageNode('first-message') as unknown as MessageNode;
    firstNode.originalIndex = 1;

    useChatStore.setState({
      timelinesByConversationId: {
        [defaultConversationId]: {
          id: defaultConversationId,
          title: 'Conversa',
          threadedMessages: [firstNode],
        },
      },
      surfaceSessionsByKey: {
        [originSessionKey]: {
          ...createEmptyChatSession(defaultConversationId, originSessionKey),
          hasOlderMessages: true,
        },
        [otherSessionKey]: {
          ...createEmptyChatSession(defaultConversationId, otherSessionKey),
          hasOlderMessages: true,
        },
      },
    });

    await useChatStore.getState().loadOlderMessagesForConversation(defaultConversationId, originSessionKey);

    expect(mockGetConversationMessageWindow).toHaveBeenCalledWith({
      scope: 'conversation',
      conversationId: defaultConversationId,
      anchor: undefined,
      anchorMessageId: 'first-message',
      direction: 'before',
      limit: expect.any(Number),
    });
    const state = useChatStore.getState();
    expect(state.timelinesByConversationId[defaultConversationId]?.threadedMessages[0]?.message.id).toBe('older-message');
    expect(state.surfaceSessionsByKey[originSessionKey]?.hasOlderMessages).toBe(false);
    expect(state.surfaceSessionsByKey[otherSessionKey]?.hasOlderMessages).toBe(true);
    expect(state.surfaceSessionsByKey[originSessionKey]?.isLoadingOlderMessages).toBe(false);
    expect(state.surfaceSessionsByKey[otherSessionKey]?.isLoadingOlderMessages).toBe(false);
  });

  it('carrega mensagens posteriores preservando metadata da janela da superfície', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `tab-a:${defaultConversationId}`;
    const currentNode = createMessageNode('current-message') as unknown as MessageNode;
    currentNode.originalIndex = 1;
    const newerNode = createMessageNode('newer-message') as unknown as MessageNode;
    mockGetConversationMessageWindow.mockResolvedValueOnce({
      scope: 'conversation',
      conversationId: defaultConversationId,
      nodes: [newerNode],
      totalCount: 3,
      startIndex: 2,
      endIndex: 2,
      hasBefore: true,
      hasAfter: false,
    });

    useChatStore.setState({
      timelinesByConversationId: {
        [defaultConversationId]: {
          id: defaultConversationId,
          title: 'Conversa',
          threadedMessages: [currentNode],
        },
      },
      surfaceSessionsByKey: {
        [originSessionKey]: {
          ...createEmptyChatSession(defaultConversationId, originSessionKey),
          visibleThreadedMessages: [currentNode],
          messageWindow: {
            scope: 'conversation',
            conversationId: defaultConversationId,
            totalCount: 3,
            startIndex: 1,
            endIndex: 1,
            hasBefore: true,
            hasAfter: true,
          },
        },
      },
    });

    await useChatStore.getState().loadNewerMessagesForConversation(defaultConversationId, originSessionKey);

    expect(mockGetConversationMessageWindow).toHaveBeenCalledWith({
      scope: 'conversation',
      conversationId: defaultConversationId,
      anchor: undefined,
      anchorMessageId: 'current-message',
      direction: 'after',
      limit: expect.any(Number),
    });
    const surfaceSession = useChatStore.getState().surfaceSessionsByKey[originSessionKey];
    expect(surfaceSession.visibleThreadedMessages?.map((node) => node.message.id))
      .toEqual(['current-message', 'newer-message']);
    expect(surfaceSession.messageWindow).toMatchObject({
      startIndex: 1,
      endIndex: 2,
      totalCount: 3,
      hasBefore: true,
      hasAfter: false,
    });
  });

  it('não usa mensagem sintética de streaming como âncora de paginação posterior', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `tab-a:${defaultConversationId}`;
    const currentNode = createMessageNode('current-message') as unknown as MessageNode;
    currentNode.originalIndex = 1;
    const streamingNode = createMessageNode('streaming-conversation-1-1') as unknown as MessageNode;
    streamingNode.message.role = 'assistant';
    streamingNode.message.turnId = 'current-message';
    streamingNode.message.isStreaming = true;
    const newerNode = createMessageNode('newer-message') as unknown as MessageNode;
    newerNode.message.role = 'assistant';
    newerNode.message.turnId = 'current-message';
    newerNode.originalIndex = 2;
    mockGetConversationMessageWindow.mockResolvedValueOnce({
      scope: 'conversation',
      conversationId: defaultConversationId,
      nodes: [newerNode],
      totalCount: 4,
      startIndex: 2,
      endIndex: 2,
      hasBefore: true,
      hasAfter: false,
    });

    useChatStore.setState({
      timelinesByConversationId: {
        [defaultConversationId]: {
          id: defaultConversationId,
          title: 'Conversa',
          threadedMessages: [currentNode, streamingNode],
        },
      },
      surfaceSessionsByKey: {
        [originSessionKey]: {
          ...createEmptyChatSession(defaultConversationId, originSessionKey),
          visibleThreadedMessages: [currentNode, streamingNode],
          messageWindow: {
            scope: 'conversation',
            conversationId: defaultConversationId,
            totalCount: 4,
            startIndex: 1,
            endIndex: 1,
            hasBefore: true,
            hasAfter: true,
          },
        },
      },
    });

    await useChatStore.getState().loadNewerMessagesForConversation(defaultConversationId, originSessionKey);

    expect(mockGetConversationMessageWindow).toHaveBeenCalledWith(expect.objectContaining({
      anchorMessageId: 'current-message',
      direction: 'after',
    }));
    const surfaceSession = useChatStore.getState().surfaceSessionsByKey[originSessionKey];
    expect(surfaceSession.visibleThreadedMessages?.map((node) => node.message.id))
      .toEqual(['current-message', 'newer-message']);
    expect(surfaceSession.visibleThreadedMessages?.some((node) => node.message.id.startsWith('streaming-')))
      .toBe(false);
  });

  it('limpa hasAfter quando não há âncora persistida para paginação posterior', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `tab-a:${defaultConversationId}`;
    const streamingNode = createMessageNode('streaming-conversation-1-1') as unknown as MessageNode;
    streamingNode.message.role = 'assistant';
    streamingNode.message.isStreaming = true;

    useChatStore.setState({
      timelinesByConversationId: {
        [defaultConversationId]: {
          id: defaultConversationId,
          title: 'Conversa',
          threadedMessages: [streamingNode],
        },
      },
      surfaceSessionsByKey: {
        [originSessionKey]: {
          ...createEmptyChatSession(defaultConversationId, originSessionKey),
          visibleThreadedMessages: [streamingNode],
          messageWindow: {
            scope: 'conversation',
            conversationId: defaultConversationId,
            totalCount: 1,
            startIndex: 0,
            endIndex: 0,
            hasBefore: false,
            hasAfter: true,
          },
        },
      },
    });

    await useChatStore.getState().loadNewerMessagesForConversation(defaultConversationId, originSessionKey);

    expect(mockGetConversationMessageWindow).not.toHaveBeenCalled();
    expect(useChatStore.getState().surfaceSessionsByKey[originSessionKey]?.messageWindow?.hasAfter).toBe(false);
  });

  it('carrega mensagens posteriores sem acionar estado de mensagens anteriores', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `tab-a:${defaultConversationId}`;
    const currentNode = createMessageNode('current-message') as unknown as MessageNode;
    currentNode.originalIndex = 1;
    let resolveWindow: (value: unknown) => void = () => {};
    mockGetConversationMessageWindow.mockImplementationOnce(() => new Promise((resolve) => {
      resolveWindow = resolve;
    }));

    useChatStore.setState({
      timelinesByConversationId: {
        [defaultConversationId]: {
          id: defaultConversationId,
          title: 'Conversa',
          threadedMessages: [currentNode],
        },
      },
      surfaceSessionsByKey: {
        [originSessionKey]: {
          ...createEmptyChatSession(defaultConversationId, originSessionKey),
          visibleThreadedMessages: [currentNode],
          messageWindow: {
            scope: 'conversation',
            conversationId: defaultConversationId,
            totalCount: 3,
            startIndex: 1,
            endIndex: 1,
            hasBefore: true,
            hasAfter: true,
          },
        },
      },
    });

    const pendingLoad = useChatStore.getState().loadNewerMessagesForConversation(defaultConversationId, originSessionKey);
    await Promise.resolve();

    const loadingSession = useChatStore.getState().surfaceSessionsByKey[originSessionKey];
    expect(loadingSession?.isLoadingMessageWindow).toBe(true);
    expect(loadingSession?.isLoadingOlderMessages).toBe(false);

    resolveWindow({
      scope: 'conversation',
      conversationId: defaultConversationId,
      nodes: [],
      totalCount: 3,
      startIndex: 0,
      endIndex: -1,
      hasBefore: true,
      hasAfter: false,
    });
    await pendingLoad;

    const settledSession = useChatStore.getState().surfaceSessionsByKey[originSessionKey];
    expect(settledSession?.isLoadingMessageWindow).toBe(false);
    expect(settledSession?.isLoadingOlderMessages).toBe(false);
  });

  it('ignora página posterior vazia sem corromper metadata de janela visível', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `tab-a:${defaultConversationId}`;
    const currentNode = createMessageNode('current-message') as unknown as MessageNode;
    currentNode.originalIndex = 2;
    mockGetConversationMessageWindow.mockResolvedValueOnce({
      scope: 'conversation',
      conversationId: defaultConversationId,
      nodes: [],
      totalCount: 3,
      startIndex: 0,
      endIndex: -1,
      hasBefore: true,
      hasAfter: false,
    });

    useChatStore.setState({
      timelinesByConversationId: {
        [defaultConversationId]: {
          id: defaultConversationId,
          title: 'Conversa',
          threadedMessages: [currentNode],
        },
      },
      surfaceSessionsByKey: {
        [originSessionKey]: {
          ...createEmptyChatSession(defaultConversationId, originSessionKey),
          visibleThreadedMessages: [currentNode],
          messageWindow: {
            scope: 'conversation',
            conversationId: defaultConversationId,
            totalCount: 3,
            startIndex: 2,
            endIndex: 2,
            hasBefore: true,
            hasAfter: true,
          },
        },
      },
    });

    await useChatStore.getState().loadNewerMessagesForConversation(defaultConversationId, originSessionKey);

    const surfaceSession = useChatStore.getState().surfaceSessionsByKey[originSessionKey];
    expect(surfaceSession.visibleThreadedMessages?.map((node) => node.message.id)).toEqual(['current-message']);
    expect(surfaceSession.messageWindow).toMatchObject({
      startIndex: 2,
      endIndex: 2,
      totalCount: 3,
      hasBefore: true,
      hasAfter: false,
    });
  });

  it('não corta turnos expandidos ao aparar janela renderizada', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `tab-a:${defaultConversationId}`;
    const nodes = Array.from({ length: 240 }, (_, index) => {
      const node = createMessageNode(`message-${index}`) as unknown as MessageNode;
      node.originalIndex = index;
      return node;
    });
    const turnUser = nodes[239];
    const turnAssistant = createMessageNode('message-240') as unknown as MessageNode;
    const turnTool = createMessageNode('message-241') as unknown as MessageNode;
    turnUser.message.turnId = 'turn-boundary';
    turnAssistant.message.turnId = 'turn-boundary';
    turnAssistant.message.role = 'assistant';
    turnTool.message.turnId = 'turn-boundary';
    turnTool.message.role = 'tool';
    turnAssistant.originalIndex = 240;
    mockGetConversationMessageWindow.mockResolvedValueOnce({
      scope: 'conversation',
      conversationId: defaultConversationId,
      nodes: [turnAssistant, turnTool],
      totalCount: 241,
      startIndex: 240,
      endIndex: 240,
      hasBefore: true,
      hasAfter: false,
    });

    useChatStore.setState({
      timelinesByConversationId: {
        [defaultConversationId]: {
          id: defaultConversationId,
          title: 'Conversa',
          threadedMessages: nodes,
        },
      },
      surfaceSessionsByKey: {
        [originSessionKey]: {
          ...createEmptyChatSession(defaultConversationId, originSessionKey),
          visibleThreadedMessages: nodes,
          messageWindow: {
            scope: 'conversation',
            conversationId: defaultConversationId,
            totalCount: 241,
            startIndex: 0,
            endIndex: 239,
            hasBefore: false,
            hasAfter: true,
          },
        },
      },
    });

    await useChatStore.getState().loadNewerMessagesForConversation(defaultConversationId, originSessionKey);

    const surfaceSession = useChatStore.getState().surfaceSessionsByKey[originSessionKey];
    const ids = surfaceSession.visibleThreadedMessages?.map((node) => node.message.id) ?? [];
    expect(ids).toContain('message-239');
    expect(ids).toContain('message-240');
    expect(ids).toContain('message-241');
  });

  it('substitui a janela visível ao saltar para o início da conversa', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `tab-a:${defaultConversationId}`;
    const middleNode = createMessageNode('middle-message') as unknown as MessageNode;
    middleNode.originalIndex = 5;
    const startNode = createMessageNode('start-message') as unknown as MessageNode;
    mockGetConversationMessageWindow.mockResolvedValueOnce({
      scope: 'conversation',
      conversationId: defaultConversationId,
      nodes: [startNode],
      totalCount: 10,
      startIndex: 0,
      endIndex: 0,
      hasBefore: false,
      hasAfter: true,
    });

    useChatStore.setState({
      timelinesByConversationId: {
        [defaultConversationId]: {
          id: defaultConversationId,
          title: 'Conversa',
          threadedMessages: [middleNode],
        },
      },
      surfaceSessionsByKey: {
        [originSessionKey]: {
          ...createEmptyChatSession(defaultConversationId, originSessionKey),
          visibleThreadedMessages: [middleNode],
          messageWindow: {
            scope: 'conversation',
            conversationId: defaultConversationId,
            totalCount: 10,
            startIndex: 5,
            endIndex: 5,
            hasBefore: true,
            hasAfter: true,
          },
        },
      },
    });

    await useChatStore.getState().loadBoundaryMessagesForConversation(defaultConversationId, originSessionKey, 'start');

    expect(mockGetConversationMessageWindow).toHaveBeenCalledWith({
      scope: 'conversation',
      conversationId: defaultConversationId,
      anchor: 'start',
      anchorMessageId: undefined,
      direction: 'after',
      limit: expect.any(Number),
    });
    const surfaceSession = useChatStore.getState().surfaceSessionsByKey[originSessionKey];
    expect(surfaceSession.visibleThreadedMessages?.map((node) => node.message.id)).toEqual(['start-message']);
    expect(surfaceSession.messageWindow).toMatchObject({
      startIndex: 0,
      endIndex: 0,
      hasBefore: false,
      hasAfter: true,
    });
  });

  it('não recarrega limite quando a superfície já está no início ou fim', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `tab-a:${defaultConversationId}`;
    const node = createMessageNode('message-1') as unknown as MessageNode;
    useChatStore.setState({
      timelinesByConversationId: {
        [defaultConversationId]: {
          id: defaultConversationId,
          title: 'Conversa',
          threadedMessages: [node],
        },
      },
      surfaceSessionsByKey: {
        [originSessionKey]: {
          ...createEmptyChatSession(defaultConversationId, originSessionKey),
          visibleThreadedMessages: [node],
          messageWindow: {
            scope: 'conversation',
            conversationId: defaultConversationId,
            totalCount: 1,
            startIndex: 0,
            endIndex: 0,
            hasBefore: false,
            hasAfter: false,
          },
        },
      },
    });

    await useChatStore.getState().loadBoundaryMessagesForConversation(defaultConversationId, originSessionKey, 'start');
    await useChatStore.getState().loadBoundaryMessagesForConversation(defaultConversationId, originSessionKey, 'end');

    expect(mockGetConversationMessageWindow).not.toHaveBeenCalled();
  });

  it('limpa mensagens usando timeline mesmo sem sessão legada', async () => {
    useChatStore.setState({
      sessionsByConversationId: {},
      timelinesByConversationId: {
        [defaultConversationId]: {
          id: defaultConversationId,
          title: 'Conversa',
          threadedMessages: [
            {
              message: {
                id: 'message-1',
                conversationId: defaultConversationId,
                role: 'user',
                content: 'oi',
                createdAt: new Date().toISOString(),
              },
              children: [],
              childCount: 0,
            } as unknown as MessageNode,
          ],
        },
      },
      surfaceSessionsByKey: {},
    });

    useChatStore.getState().handleConversationCleared(defaultConversationId);

    expect(useChatStore.getState().timelinesByConversationId[defaultConversationId]?.threadedMessages).toEqual([]);
    expect(useChatStore.getState().timelinesByConversationId[defaultConversationId]?.title).toBe('chat.conversationCleared');
    expect(mockAnnounce).toHaveBeenCalledWith('chat.announce.conversationMessagesRemoved');
  });

  it('serializa envios concorrentes da mesma conversa', async () => {
    const firstSend = deferred<void>();
    mockSendMessage
      .mockImplementationOnce(() => firstSend.promise)
      .mockResolvedValueOnce(undefined);

    const first = useChatStore.getState().sendMessageToConversation(defaultConversationId, 'primeira');
    const second = useChatStore.getState().sendMessageToConversation(defaultConversationId, 'segunda');

    await flushMicrotasks();
    expect(mockSendMessage).toHaveBeenCalledTimes(1);
    expect(mockSendMessage.mock.calls[0][1]).toBe('primeira');
    expect(useChatStore.getState().sessionsByConversationId[defaultConversationId]?.queuedTurnCount).toBe(1);

    firstSend.resolve();
    await first;
    await second;

    expect(mockSendMessage).toHaveBeenCalledTimes(2);
    expect(mockSendMessage.mock.calls[1][1]).toBe('segunda');
    expect(useChatStore.getState().sessionsByConversationId[defaultConversationId]?.queuedTurnCount).toBe(0);
  });

  it('serializa retry atrás de envio ativo da mesma conversa', async () => {
    const firstSend = deferred<void>();
    mockSendMessage.mockImplementationOnce(() => firstSend.promise);
    mockRetryMessage.mockResolvedValueOnce(undefined);

    const first = useChatStore.getState().sendMessageToConversation(defaultConversationId, 'primeira');
    const retry = useChatStore.getState().retryMessageToConversation(defaultConversationId, 'message-1');

    await flushMicrotasks();
    expect(mockSendMessage).toHaveBeenCalledTimes(1);
    expect(mockRetryMessage).not.toHaveBeenCalled();
    expect(useChatStore.getState().sessionsByConversationId[defaultConversationId]?.queuedTurnCount).toBe(1);

    firstSend.resolve();
    await first;
    await retry;

    expect(mockRetryMessage).toHaveBeenCalledWith(defaultConversationId, 'message-1', expect.any(Object));
    expect(useChatStore.getState().sessionsByConversationId[defaultConversationId]?.queuedTurnCount).toBe(0);
  });

  it('mantem envios de conversas diferentes em paralelo', async () => {
    const firstSend = deferred<void>();
    const otherConversationId = '01926b90-7a5a-7c4e-8d3f-000000000007';
    useChatStore.setState({
      sessionsByConversationId: {
        ...useChatStore.getState().sessionsByConversationId,
        [otherConversationId]: {
          ...useChatStore.getState().sessionsByConversationId[defaultConversationId],
          conversation: { id: otherConversationId, title: 'Outra', threadedMessages: [] },
        },
      },
    });
    mockSendMessage
      .mockImplementationOnce(() => firstSend.promise)
      .mockResolvedValueOnce(undefined);

    const first = useChatStore.getState().sendMessageToConversation(defaultConversationId, 'primeira');
    const second = useChatStore.getState().sendMessageToConversation(otherConversationId, 'segunda');

    await flushMicrotasks();
    expect(mockSendMessage).toHaveBeenCalledTimes(2);

    firstSend.resolve();
    await Promise.all([first, second]);
  });

  it('chat:speak event invokes handleChatSpeak for matching conversation', async () => {
    mockSendMessage.mockImplementation(() => {
      // Simula backend: emite chat:messages_ready, depois chat:speak do user
      emitEvent('chat:messages_ready', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", userMessageId: "01926b90-7a5a-7c4e-8d3f-000000000010", userContent: 'oi' });
      emitEvent('chat:speak', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", role: 'user', text: 'oi', strategy: 'announce', origin: 'user_message' });
      // Simula chat:speak do assistant (antes do done)
      emitEvent('chat:speak', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", role: 'assistant', text: 'Resposta', strategy: 'webspeech', origin: 'assistant_message' });
      emitEvent('chat:done', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", assistantMessageId: "01926b90-7a5a-7c4e-8d3f-000000000011", hadToolCalls: false });
      return Promise.resolve();
    });

    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'oi');

    expect(mockHandleChatSpeak).toHaveBeenCalledTimes(2);
    expect(mockHandleChatSpeak.mock.calls[0][0]).toMatchObject({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      role: 'user',
      strategy: 'announce',
    });
    expect(mockHandleChatSpeak.mock.calls[1][0]).toMatchObject({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      role: 'assistant',
      strategy: 'webspeech',
    });
  });

  it('chat:speak event is ignored for different conversation', async () => {
    mockSendMessage.mockImplementation(() => {
      emitEvent('chat:messages_ready', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", userMessageId: "01926b90-7a5a-7c4e-8d3f-000000000010", userContent: 'oi' });
      // Evento de outra conversa
      emitEvent('chat:speak', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000999", role: 'assistant', text: 'Outro', strategy: 'announce' });
      emitEvent('chat:done', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", assistantMessageId: "01926b90-7a5a-7c4e-8d3f-000000000011", hadToolCalls: false });
      return Promise.resolve();
    });

    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'oi');

    expect(mockHandleChatSpeak).not.toHaveBeenCalled();
  });

  it('usa placeholder único por envio e finaliza streaming após erro no stream', async () => {
    useChatStore.setState({
      sessionsByConversationId: {
        "01926b90-7a5a-7c4e-8d3f-000000000001": {
          ...useChatStore.getState().sessionsByConversationId["01926b90-7a5a-7c4e-8d3f-000000000001"],
          conversation: { id: "01926b90-7a5a-7c4e-8d3f-000000000001", title: 'Conversa', threadedMessages: [] },
        },
      },
      timelinesByConversationId: {
        "01926b90-7a5a-7c4e-8d3f-000000000001": {
          id: "01926b90-7a5a-7c4e-8d3f-000000000001",
          title: 'Conversa',
          threadedMessages: [],
        },
      },
      surfaceSessionsByKey: {},
    });

    mockSendMessage
      .mockImplementationOnce(() => {
        emitEvent('chat:messages_ready', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", userMessageId: "01926b90-7a5a-7c4e-8d3f-000000014535", userContent: 'falha 1' });
        emitEvent('chat:stream', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", content: 'parcial', done: false });
        emitEvent('chat:stream', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", error: '401 Unauthorized' });
        return Promise.resolve();
      })
      .mockImplementationOnce(() => {
        emitEvent('chat:messages_ready', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", userMessageId: "01926b90-7a5a-7c4e-8d3f-000000014545", userContent: 'ok 2' });
        emitEvent('chat:stream', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", content: 'resposta final', done: false });
        emitEvent('chat:stream', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", messageId: "01926b90-7a5a-7c4e-8d3f-000000014546", content: 'resposta final', done: true });
        emitEvent('chat:done', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", assistantMessageId: "01926b90-7a5a-7c4e-8d3f-000000014546", hadToolCalls: false });
        return Promise.resolve();
      });

    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'falha 1');
    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'ok 2');
    await flushMicrotasks();

    const threaded = useChatStore.getState().getConversationThreadedMessages(defaultConversationId) ?? [];
    const ids = threaded.map((node) => String(node.message.id));
    expect(new Set(ids).size).toBe(ids.length);
    expect(mockSendMessage).toHaveBeenCalledTimes(2);

    const syntheticIds = ids.filter((id) => id.startsWith('streaming-01926b90-7a5a-7c4e-8d3f-000000000001-'));
    expect(syntheticIds.length).toBeLessThanOrEqual(1);
    const firstAssistantError = syntheticIds.length > 0
      ? threaded.find((node) => String(node.message.id) === syntheticIds[0])
      : undefined;
    expect(firstAssistantError?.message.isStreaming ?? false).toBe(false);
    const finalAssistant = threaded.find((node) => String(node.message.id) === '01926b90-7a5a-7c4e-8d3f-000000014546');
    expect(finalAssistant?.message.role).toBe('assistant');
    expect(finalAssistant?.message.isStreaming).toBe(false);
  });

  it('não duplica mensagem real quando outro caminho já inseriu o mesmo assistant id', async () => {
    const conversation = {
      id: "01926b90-7a5a-7c4e-8d3f-000000000001",
      title: 'Conversa',
      threadedMessages: [
        {
          message: {
            id: '01926b90-7a5a-7c4e-8d3f-000000014731',
            conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
            role: 'assistant',
            content: 'mensagem já sincronizada',
            createdAt: new Date().toISOString(),
            isStreaming: false,
          },
          children: [],
          childCount: 0,
        } as unknown as MessageNode,
      ],
    };
    useChatStore.setState({
      sessionsByConversationId: {
        "01926b90-7a5a-7c4e-8d3f-000000000001": {
          ...useChatStore.getState().sessionsByConversationId["01926b90-7a5a-7c4e-8d3f-000000000001"],
          conversation,
        },
      },
    });

    mockSendMessage.mockImplementationOnce(() => {
      emitEvent('chat:messages_ready', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", userMessageId: "01926b90-7a5a-7c4e-8d3f-000000014730", userContent: 'oi' });
      emitEvent('chat:stream', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", content: 'resposta parcial', done: false });
      emitEvent('chat:stream', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", messageId: "01926b90-7a5a-7c4e-8d3f-000000014731", content: 'resposta final', done: true });
      emitEvent('chat:done', { conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001", assistantMessageId: "01926b90-7a5a-7c4e-8d3f-000000014731", hadToolCalls: false });
      return Promise.resolve();
    });

    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'oi');

    const threaded = useChatStore.getState().sessionsByConversationId[defaultConversationId]?.conversation?.threadedMessages ?? [];
    const assistant14731 = threaded.filter((node) => String(node.message.id) === '01926b90-7a5a-7c4e-8d3f-000000014731');
    expect(assistant14731).toHaveLength(1);
  });

  it('mantém resposta em streaming na janela visual da superfície de origem', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `page:tab-a:${defaultConversationId}`;
    const initialNode = createMessageNode('initial-message') as unknown as MessageNode;
    initialNode.originalIndex = 0;
    useChatStore.setState({
      sessionsByConversationId: {
        [defaultConversationId]: {
          ...useChatStore.getState().sessionsByConversationId[defaultConversationId],
          conversation: {
            id: defaultConversationId,
            title: 'Conversa',
            threadedMessages: [initialNode],
          },
          visibleThreadedMessages: [initialNode],
          messageWindow: {
            scope: 'conversation',
            conversationId: defaultConversationId,
            totalCount: 5,
            startIndex: 0,
            endIndex: 0,
            hasBefore: false,
            hasAfter: true,
          },
        },
      },
      surfaceSessionsByKey: {
        [originSessionKey]: {
          ...createEmptyChatSession(defaultConversationId, originSessionKey),
          visibleThreadedMessages: [initialNode],
          messageWindow: {
            scope: 'conversation',
            conversationId: defaultConversationId,
            totalCount: 5,
            startIndex: 0,
            endIndex: 0,
            hasBefore: false,
            hasAfter: true,
          },
        },
      },
    });
    mockSendMessage.mockImplementationOnce(() => {
      emitEvent('chat:messages_ready', {
        conversationId: defaultConversationId,
        userMessageId: 'surface-user-message',
        userContent: 'oi',
      });
      emitEvent('chat:stream', {
        conversationId: defaultConversationId,
        content: 'resposta parcial',
        done: false,
      });
      return Promise.resolve();
    });

    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'oi', undefined, undefined, {
      origin: {
        conversationId: defaultConversationId,
        sessionKey: originSessionKey,
        surfaceId: 'page:tab-a',
        surfaceType: 'page',
        tabId: 'tab-a',
      },
    });
    await flushMicrotasks();

    const surfaceMessages = useChatStore.getState().surfaceSessionsByKey[originSessionKey]
      ?.visibleThreadedMessages
      ?.map((node) => node.message.role);
    expect(surfaceMessages).toEqual(['user', 'user', 'assistant']);
  });

  it('não anexa refresh completo à superfície que está navegando histórico antigo', async () => {
    const { createEmptyChatSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `page:tab-a:${defaultConversationId}`;
    const initialNode = createMessageNode('initial-message') as unknown as MessageNode;
    initialNode.originalIndex = 0;
    const refreshedNodes = [
      initialNode,
      ...Array.from({ length: 6 }, (_, index) => createMessageNode(`offscreen-${index}`) as unknown as MessageNode),
    ];
    useChatStore.setState({
      sessionsByConversationId: {
        [defaultConversationId]: {
          ...useChatStore.getState().sessionsByConversationId[defaultConversationId],
          conversation: {
            id: defaultConversationId,
            title: 'Conversa',
            threadedMessages: [initialNode],
          },
          visibleThreadedMessages: [initialNode],
          messageWindow: {
            scope: 'conversation',
            conversationId: defaultConversationId,
            totalCount: 8,
            startIndex: 0,
            endIndex: 0,
            hasBefore: false,
            hasAfter: true,
          },
        },
      },
      surfaceSessionsByKey: {
        [originSessionKey]: {
          ...createEmptyChatSession(defaultConversationId, originSessionKey),
          visibleThreadedMessages: [initialNode],
          messageWindow: {
            scope: 'conversation',
            conversationId: defaultConversationId,
            totalCount: 8,
            startIndex: 0,
            endIndex: 0,
            hasBefore: false,
            hasAfter: true,
          },
        },
      },
    });
    mockGetConversationInfo.mockResolvedValueOnce({ title: 'Conversa' });
    mockGetConversationMessageWindow.mockResolvedValueOnce({
      scope: 'conversation',
      conversationId: defaultConversationId,
      nodes: refreshedNodes,
      totalCount: refreshedNodes.length,
      startIndex: 0,
      endIndex: refreshedNodes.length - 1,
      hasBefore: false,
      hasAfter: false,
    });
    mockSendMessage.mockImplementationOnce(() => {
      emitEvent('chat:done', {
        conversationId: defaultConversationId,
        hadToolCalls: true,
      });
      return Promise.resolve();
    });

    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'oi', undefined, undefined, {
      origin: {
        conversationId: defaultConversationId,
        sessionKey: originSessionKey,
        surfaceId: 'page:tab-a',
        surfaceType: 'page',
        tabId: 'tab-a',
      },
    });
    await flushMicrotasks();

    const state = useChatStore.getState();
    expect(state.surfaceSessionsByKey[originSessionKey]?.visibleThreadedMessages?.map((node) => node.message.id)).toEqual([
      'initial-message',
    ]);
    expect(state.timelinesByConversationId[defaultConversationId]?.threadedMessages).toHaveLength(refreshedNodes.length);
  });
});
