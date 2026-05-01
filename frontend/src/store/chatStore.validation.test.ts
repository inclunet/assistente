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
const mockGetConversationInfo = vi.fn().mockResolvedValue({});
vi.mock('@wailsjs/go/app/App', () => ({
  SendMessage: (...args: unknown[]) => mockSendMessage(...args),
  RetryMessage: (...args: unknown[]) => mockRetryMessage(...args),
  GetMessages: (...args: unknown[]) => mockGetMessages(...args),
  GetRecentMessages: (...args: unknown[]) => mockGetRecentMessages(...args),
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
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
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
    mockGetConversationInfo.mockReset();
    mockGetConversationInfo.mockResolvedValue({});
    mockHandleChatSpeak.mockClear();
    const mod = await import('./chatStore');
    useChatStore = mod.useChatStore;
    useChatStore.setState({
      sessionsByConversationId: {
        [defaultConversationId]: {
          conversation: { id: defaultConversationId, title: 'Conversa', threadedMessages: [] },
          isLoading: false,
          hasOlderMessages: false,
          isLoadingOlderMessages: false,
          streamingMessageId: null,
          streamingReasoning: null,
          isThinking: false,
          activeToolCalls: [],
          completedSegments: [],
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

    const threaded = useChatStore.getState().sessionsByConversationId[defaultConversationId]?.conversation?.threadedMessages ?? [];
    const ids = threaded.map((node) => String(node.message.id));
    expect(new Set(ids).size).toBe(ids.length);

    const syntheticIds = ids.filter((id) => id.startsWith('streaming-01926b90-7a5a-7c4e-8d3f-000000000001-'));
    expect(syntheticIds.length).toBe(1);
    const firstAssistantError = threaded.find((node) => String(node.message.id) === syntheticIds[0]);
    expect(firstAssistantError).toBeDefined();
    expect(firstAssistantError?.message.isStreaming).toBe(false);
    expect(ids).toContain('01926b90-7a5a-7c4e-8d3f-000000014546');
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
});
