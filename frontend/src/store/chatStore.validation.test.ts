import { withFixtureProjection } from '../test/chatWindowFixture';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MediaCategory } from '../services/mediaService';

// ── Mocks ──────────────────────────────────────────────────────────────

const mockAnnounce = vi.fn();
vi.mock('../hooks/useAnnouncer', () => ({
  announce: (...args: unknown[]) => mockAnnounce(...args),
}));

const mockSendMessage = vi.fn().mockResolvedValue(undefined);
const mockRetryMessage = vi.fn().mockResolvedValue(undefined);
const mockCancelStreaming = vi.fn().mockResolvedValue(undefined);
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
vi.mock('@wailsjs/go/wailsapi/Chat', () => ({
  SendMessage: (...args: unknown[]) => mockSendMessage(...args),
  RetryMessage: (...args: unknown[]) => mockRetryMessage(...args),
}));

vi.mock('@wailsjs/go/wailsapi/LLMModels', () => ({
  CancelStreamingForConversation: (...args: unknown[]) => mockCancelStreaming(...args),
  CancelStreamingExecution: (...args: unknown[]) => mockCancelStreaming(...args),
}));

vi.mock('@wailsjs/go/wailsapi/Messaging', () => ({
  AssignConversationToChannel: vi.fn(),
  UnassignConversationFromChannel: vi.fn(),
}));

vi.mock('@wailsjs/go/wailsapi/Conversations', () => ({
  GetMessages: (...args: unknown[]) => mockGetMessages(...args),
  GetRecentMessages: (...args: unknown[]) => mockGetRecentMessages(...args),
  GetMessagesBefore: (...args: unknown[]) => mockGetMessagesBefore(...args),
  GetConversationMessageWindow: (...args: unknown[]) => mockGetConversationMessageWindow(...args),
  GetConversationInfo: (...args: unknown[]) => mockGetConversationInfo(...args),
  EnsureConversation: vi.fn().mockResolvedValue("01926b90-7a5a-7c4e-8d3f-000000000001"),
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
      if (key === 'chat.validation.messageTooLarge') {
        return `Mensagem muito grande (${opts?.sizeBytes} bytes). Máximo permitido: ${opts?.maxKiB} KiB.`;
      }
      return key;
    },
  },
}));

function emitEvent(name: string, data: unknown) {
  if (name === 'chat:stream' && data && typeof data === 'object' && 'content' in data) {
    const { content, ...event } = data as Record<string, unknown>;
    if (event.done && content) {
      const callbacks = eventListeners.get(name) || [];
      for (const callback of callbacks) {
        callback({ ...event, done: false, delta: content, reset: true, sequence: 0 });
      }
      data = event;
    } else {
      data = { ...event, delta: content, reset: true, sequence: 0 };
    }
  }
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
    mockCancelStreaming.mockReset().mockResolvedValue(undefined);
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
      conversationInfoById: {
        [defaultConversationId]: withFixtureProjection(
          { id: defaultConversationId, title: 'Conversa' },
          { nodes: [] }
        ),
      },
      surfaceSessionsByKey: {
        ['conversation:' + defaultConversationId]: {
          sessionKey: `conversation:${defaultConversationId}`,
          conversationId: defaultConversationId,
          isLoading: false,
          sendFailureMessage: null,
          sendFailureAnnounced: false,
          sendFailureRetryable: false,
          sendFailureRetryContent: null,
          sendFailureRetryMediaFiles: [],
          draftMessage: '',
          draftMediaFiles: [],
          scrollTop: 0,
          scrollAnchorMessageId: null,
          expandedThreads: new Set<string>(),
          expandedReasonings: new Set<string>(),
          editingMessageId: null,
          skipFocusRestore: false,
        },
      },
    });
  }, 30_000);

  afterEach(() => {
    useChatStore.getState().handleDatabaseReset();
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  it('compartilha a carga de metadados entre consumidores concorrentes', async () => {
    let finish!: (value: unknown) => void;
    mockGetConversationInfo.mockReturnValueOnce(new Promise(resolve => { finish = resolve; }));
    const first = useChatStore.getState().loadConversationSession(defaultConversationId);
    const second = useChatStore.getState().loadConversationSession(defaultConversationId);
    expect(second).toBe(first);
    await Promise.resolve();
    expect(mockGetConversationInfo).toHaveBeenCalledTimes(1);
    finish({ title: 'compartilhada' });
    await Promise.all([first, second]);
    expect(useChatStore.getState().conversationInfoById[defaultConversationId]?.title).toBe('compartilhada');
  });

  it.each(['delete', 'reset'])('recusa metadados tardios após %s e permite nova carga', async action => {
    let finish!: (value: unknown) => void;
    mockGetConversationInfo.mockReturnValueOnce(new Promise(resolve => { finish = resolve; }));
    const loading = useChatStore.getState().loadConversationSession(defaultConversationId);
    await Promise.resolve();
    const rejected = expect(loading).rejects.toThrow('chat-messaging-stale');
    if (action === 'delete') useChatStore.getState().handleConversationDeleted(defaultConversationId);
    else useChatStore.getState().handleDatabaseReset();
    mockGetConversationInfo.mockResolvedValueOnce({ title: 'nova' });
    await useChatStore.getState().loadConversationSession(defaultConversationId);
    finish({ title: 'antiga' });
    await rejected;
    expect(useChatStore.getState().conversationInfoById[defaultConversationId]?.title).toBe('nova');
  });

  it('envio auditado reutiliza SendMessage e serializa somente a correlação', async () => {
    const isCurrent = vi.fn(() => true);
    const handoff = { ticket: 'ticket-send', handoffId: 'handoff-send' };
    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'conteúdo', undefined, undefined, {
      command: { handoff, isCurrent },
    });
    expect(mockSendMessage).toHaveBeenCalledExactlyOnceWith(defaultConversationId, 'conteúdo', '', expect.objectContaining({ command: handoff }));
    expect(mockRetryMessage).not.toHaveBeenCalled();
    expect(mockSendMessage.mock.calls[0][3].command).not.toHaveProperty('isCurrent');
    expect(JSON.parse(JSON.stringify(mockSendMessage.mock.calls[0][3])).command).toEqual(handoff);
    expect(isCurrent).toHaveBeenCalled();
  });

  it('retry auditado conserva o ID persistido e não cria outro envio', async () => {
    const handoff = { ticket: 'ticket-retry', handoffId: 'handoff-retry' };
    await useChatStore.getState().retryMessageToConversation(defaultConversationId, 'persisted-message', undefined, {
      command: { handoff, isCurrent: () => true },
    });
    expect(mockRetryMessage).toHaveBeenCalledExactlyOnceWith(defaultConversationId, 'persisted-message', expect.objectContaining({ command: handoff }));
    expect(mockSendMessage).not.toHaveBeenCalled();
  });

  it('guarda obsoleta recusa antes de enviar ou iniciar o turno', async () => {
    await expect(useChatStore.getState().sendMessageToConversation(defaultConversationId, 'rascunho', undefined, undefined, {
      command: { handoff: { ticket: 'ticket', handoffId: 'handoff' }, isCurrent: () => false },
    })).rejects.toThrow('chat-messaging-stale');
    expect(mockSendMessage).not.toHaveBeenCalled();
    expect(useChatStore.getState().getMessagingPipelineRevision(defaultConversationId)).toBe(0);
  });

  it('validação do envio auditado anuncia o limite sem chamar Wails', async () => {
    await expect(useChatStore.getState().sendMessageToConversation(defaultConversationId, 'x'.repeat(512 * 1024 + 1), undefined, undefined, {
      command: { handoff: { ticket: 'ticket', handoffId: 'handoff' }, isCurrent: () => true },
    })).rejects.toThrow('chat-messaging-stale');
    expect(mockSendMessage).not.toHaveBeenCalled();
    expect(mockAnnounce).toHaveBeenCalledWith('Mensagem muito grande (524289 bytes). Máximo permitido: 512 KiB.', 'assertive');
  });

  it('revalida o alvo depois da serialização assíncrona dos anexos', async () => {
    const serializer = await import('../services/mediaSerialization');
    const gate = deferred<string>();
    const serialize = vi.spyOn(serializer, 'serializeMediaForConversation').mockReturnValue(gate.promise);
    let current = true;
    const file = new File(['anexo'], 'a.txt', { type: 'text/plain' });
    const send = useChatStore.getState().sendMessageToConversation(defaultConversationId, 'rascunho', [{
      id: 'media', file, category: MediaCategory.DOCUMENT, mimeType: 'text/plain', extension: '.txt',
      fileName: 'a.txt', fileSize: file.size, fileSizeFormatted: '5 B', icon: '', preview: '',
    }], undefined, { command: { handoff: { ticket: 'ticket', handoffId: 'handoff' }, isCurrent: () => current } });
    const rejected = expect(send).rejects.toThrow('chat-messaging-stale');
    await vi.waitFor(() => expect(serialize).toHaveBeenCalledOnce());
    current = false;
    gate.resolve('[]');
    await rejected;
    expect(mockSendMessage).not.toHaveBeenCalled();
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId].isLoading).toBe(false);
  });

  it('erro de transporte auditado não oferece retry como nova mensagem', async () => {
    mockSendMessage.mockRejectedValueOnce(new Error('resposta perdida após possível persistência'));
    await expect(useChatStore.getState().sendMessageToConversation(defaultConversationId, 'não duplicar', undefined, undefined, {
      command: { handoff: { ticket: 'ticket', handoffId: 'handoff' }, isCurrent: () => true },
    })).rejects.toThrow('resposta perdida');
    const session = useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId];
    expect(session.sendFailureMessage).toBe('chat.sendErrorPrefix');
    expect(session.sendFailureAnnounced).toBe(false);
    expect(session.sendFailureRetryable).toBe(false);
    expect(session.sendFailureRetryContent).toBeNull();
    expect(session.isLoading).toBe(true);
    expect(mockSendMessage).toHaveBeenCalledOnce();
    expect(mockRetryMessage).not.toHaveBeenCalled();
  });

  it('limpeza tardia de cancelamento não encerra um pipeline posterior', async () => {
    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'primeira');
    const previous = useChatStore.getState().getMessagingPipelineRevision(defaultConversationId);
    emitEvent('chat:done', { conversationId: defaultConversationId });
    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'segunda');
    const current = useChatStore.getState().getMessagingPipelineRevision(defaultConversationId);
    expect(current).toBeGreaterThan(previous);
    useChatStore.getState().finishCommandCancellation(defaultConversationId, `conversation:${defaultConversationId}`, previous);
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId].isLoading).toBe(true);
    useChatStore.getState().finishCommandCancellation(defaultConversationId, `conversation:${defaultConversationId}`, current);
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId].isLoading).toBe(false);
  });

  it('revisão do rascunho detecta alteração ABA', () => {
    const key = `conversation:${defaultConversationId}`;
    const state = useChatStore.getState();
    state.setConversationDraftMessage(defaultConversationId, 'A', key);
    const initial = state.getDraftRevision(key);
    state.setConversationDraftMessage(defaultConversationId, 'B', key);
    state.setConversationDraftMessage(defaultConversationId, 'A', key);
    expect(state.getDraftRevision(key)).toBeGreaterThan(initial);
  });

  it('cancelamento confirmado encerra o loading da execução admitida', async () => {
    const userId = '01926b90-7a5a-7c4e-8d3f-000000000010';
    const assistantId = '01926b90-7a5a-7c4e-8d3f-000000000011';
    mockSendMessage.mockImplementationOnce(async () => {
      emitEvent('chat:messages_ready', { conversationId: defaultConversationId, userMessageId: userId, userContent: 'pergunta', turnId: userId });
      emitEvent('chat:stream', { conversationId: defaultConversationId, messageId: assistantId, turnId: userId, content: 'resposta parcial', done: false });
    });
    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'pergunta');
    const revision = useChatStore.getState().getMessagingPipelineRevision(defaultConversationId);
    useChatStore.getState().finishCommandCancellation(defaultConversationId, `conversation:${defaultConversationId}`, revision);
    const session = useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId];
    expect(session.isLoading).toBe(false);
    // Conteúdo/turno/parcial são cobertos no runtime por
    // TestExecutionWriterToolFirstTerminalUsesExplicitExecutionMode e no renderer
    // por ChatSessionLiveState; este store somente encerra a operação admitida.
    expect(useChatStore.getState().getMessagingPipelineRevision(defaultConversationId)).toBeGreaterThan(revision);
  });

  it('rejects message exceeding max content size', async () => {
    const bigContent = 'x'.repeat(512 * 1024 + 1);
    const focusedInput = document.createElement('textarea');
    document.body.appendChild(focusedInput);
    focusedInput.focus();

    const accepted = await useChatStore.getState().sendMessageToConversation(defaultConversationId, bigContent);

    expect(accepted).toBe(false);
    expect(mockAnnounce).toHaveBeenCalledTimes(1);
    expect(mockAnnounce).toHaveBeenCalledWith(
      'Mensagem muito grande (524289 bytes). Máximo permitido: 512 KiB.',
      'assertive',
    );
    expect(mockSendMessage).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(focusedInput);
    focusedInput.remove();
  });

  it('accepts message at exact max content size', async () => {
    const exactContent = 'x'.repeat(512 * 1024);

    const accepted = await useChatStore.getState().sendMessageToConversation(defaultConversationId, exactContent);

    expect(accepted).toBe(true);
    expect(mockSendMessage).toHaveBeenCalled();
  });

  it('aceita emoji exatamente no limite UTF-8 e rejeita um byte adicional', async () => {
    const exactContent = '😀'.repeat((512 * 1024) / 4);

    await useChatStore.getState().sendMessageToConversation(defaultConversationId, exactContent);
    expect(mockSendMessage).toHaveBeenCalledTimes(1);

    mockSendMessage.mockClear();
    await useChatStore.getState().sendMessageToConversation(defaultConversationId, `${exactContent}a`);

    expect(mockSendMessage).not.toHaveBeenCalled();
    expect(mockAnnounce).toHaveBeenLastCalledWith(
      'Mensagem muito grande (524289 bytes). Máximo permitido: 512 KiB.',
      'assertive',
    );
  });

  it('aceita conteúdo vazio', async () => {
    await useChatStore.getState().sendMessageToConversation(defaultConversationId, '');

    expect(mockSendMessage).toHaveBeenCalledWith(defaultConversationId, '', '', expect.any(Object));
  });

  it('conta somente o texto quando há anexo serializado em base64', async () => {
    const file = new File(['conteúdo binário'], 'anexo.txt', { type: 'text/plain' });
    const content = '😀'.repeat((512 * 1024) / 4);

    await useChatStore.getState().sendMessageToConversation(defaultConversationId, content, [{
      id: 'anexo-1',
      file,
      category: MediaCategory.DOCUMENT,
      mimeType: 'text/plain',
      extension: '.txt',
      fileName: 'anexo.txt',
      fileSize: file.size,
      fileSizeFormatted: '16 B',
      icon: '📄',
      preview: '',
    }]);

    expect(mockSendMessage).toHaveBeenCalledWith(
      defaultConversationId,
      content,
      expect.stringContaining('"data"'),
      expect.any(Object),
    );
  });

  it('rejeita texto excedente antes de preparar anexos no Worker', async () => {
    const file = new File(['anexo'], 'anexo.txt', { type: 'text/plain' });
    const arrayBuffer = vi.fn().mockResolvedValue(new ArrayBuffer(5));
    Object.defineProperty(file, 'arrayBuffer', { value: arrayBuffer });

    await useChatStore.getState().sendMessageToConversation(
      defaultConversationId,
      '😀'.repeat((512 * 1024) / 4 + 1),
      [{
        id: 'anexo-bloqueado',
        file,
        category: MediaCategory.DOCUMENT,
        mimeType: 'text/plain',
        extension: '.txt',
        fileName: 'anexo.txt',
        fileSize: file.size,
        fileSizeFormatted: '5 B',
        icon: '📄',
        preview: '',
      }],
    );

    expect(arrayBuffer).not.toHaveBeenCalled();
    expect(mockSendMessage).not.toHaveBeenCalled();
  });

  it('retry de mensagem existente usa RetryMessage sem criar novo SendMessage', async () => {
    await useChatStore.getState().retryMessageToConversation("1", "42");

    expect(mockRetryMessage).toHaveBeenCalledWith("1", "42", expect.any(Object));
    expect(mockSendMessage).not.toHaveBeenCalled();
  });

  it('rejects media exceeding max size', async () => {
    const fakeFile = new File([new ArrayBuffer(15 * 1024 * 1024)], 'big.bin', { type: 'application/octet-stream' });

    const accepted = await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'hello', [{
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

    expect(accepted).toBe(false);
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
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId]?.isLoading).toBe(false);
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
      surfaceContextJson: '{"surfaceType":"editor","surfaceId":"tab-1","snapshotVersion":"editor:tab-1:1","selection":{"kind":"text","text":"hello"}}',
    });

    expect(mockSendMessage).toHaveBeenCalledWith("01926b90-7a5a-7c4e-8d3f-000000000007", 'hello', '', expect.objectContaining({
      tabType: 'editor',
      activeFilePath: '/tmp/readme.md',
      surfaceStateJson: '{"filePath":"/tmp/readme.md"}',
      surfaceContextJson: '{"surfaceType":"editor","surfaceId":"tab-1","snapshotVersion":"editor:tab-1:1","selection":{"kind":"text","text":"hello"}}',
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

    expect(useChatStore.getState().surfaceSessionsByKey[origin.sessionKey]?.surfaceOrigin).toEqual({
      ...origin,
      executionId: expect.any(String),
    });
  });

  it('propaga eventos sem origem para superfícies existentes da conversa', async () => {
    const { createEmptyChatSurfaceSession } = await import('../services/chatSessionRegistry');
    useChatStore.setState({
      conversationInfoById: {
        [defaultConversationId]: withFixtureProjection({ id: defaultConversationId, title: 'Conversa' }, { nodes: [] }),
      },
      surfaceSessionsByKey: {
        [`tab-a:${defaultConversationId}`]: createEmptyChatSurfaceSession(defaultConversationId, `tab-a:${defaultConversationId}`),
        [`tab-b:${defaultConversationId}`]: createEmptyChatSurfaceSession(defaultConversationId, `tab-b:${defaultConversationId}`),
      },
    });

    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'hello');
    emitEvent('chat:done', {
      conversationId: defaultConversationId,
      reason: 'completed',
    });
    await flushMicrotasks();

    expect(useChatStore.getState().surfaceSessionsByKey[`tab-a:${defaultConversationId}`]?.isLoading).toBe(false);
    expect(useChatStore.getState().surfaceSessionsByKey[`tab-b:${defaultConversationId}`]?.isLoading).toBe(false);
  });

  it('remove timeline e superfícies órfãs ao deletar conversa', async () => {
    const { createEmptyChatSurfaceSession } = await import('../services/chatSessionRegistry');
    const orphanConversationId = '01926b90-7a5a-7c4e-8d3f-000000000099';
    const orphanSessionKey = `tab-chat:${orphanConversationId}`;
    useChatStore.setState({
      conversationInfoById: {
        [orphanConversationId]: withFixtureProjection({ id: orphanConversationId, title: 'Órfã' }, { nodes: [] }),
        [defaultConversationId]: withFixtureProjection({ id: defaultConversationId, title: 'Preservada' }, { nodes: [] }),
      },
      surfaceSessionsByKey: {
        [orphanSessionKey]: createEmptyChatSurfaceSession(orphanConversationId, orphanSessionKey),
      },
    });

    const metadataBeforeDeletion = useChatStore.getState().conversationInfoById[defaultConversationId];
    useChatStore.getState().handleConversationDeleted(orphanConversationId);

    expect(useChatStore.getState().conversationInfoById[orphanConversationId]).toBeUndefined();
    expect(useChatStore.getState().surfaceSessionsByKey[orphanSessionKey]).toBeUndefined();
    expect(useChatStore.getState().conversationInfoById[defaultConversationId]).toBe(metadataBeforeDeletion);
  });

  it('envia sem recarregar quando timeline já está carregada sem sessão legada', async () => {
    useChatStore.setState({
      conversationInfoById: {
        [defaultConversationId]: withFixtureProjection({ id: defaultConversationId, title: 'Conversa' }, { nodes: [] }),
      },
      surfaceSessionsByKey: {},
    });

    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'hello');

    expect(mockGetConversationInfo).not.toHaveBeenCalled();
    expect(mockGetMessages).not.toHaveBeenCalled();
    expect(mockGetRecentMessages).not.toHaveBeenCalled();
    expect(mockSendMessage).toHaveBeenCalledWith(defaultConversationId, 'hello', '', expect.any(Object));
  });

  it.each([false, true])('encaminha modo de continuação ao backend antes de qualquer delta (%s)', async (allowAssistantPrefill) => {
    const assistantId = '01926b90-7a5a-7c4e-8d3f-000000000011';
    const prior = createMessageNode(assistantId);
    const assistant = {
      ...prior,
      message: {
        ...prior.message, role: 'assistant', content: 'Parcial', turnId: 'turn-1',
        turnSegments: [
          { type: 'tool_calls', toolInvocations: [{ callId: 'old-call', name: 'search', status: 'ok', hasDetails: true, resultAvailability: 'available' }] },
          { type: 'text', content: 'Parcial' },
        ],
      },
    } as unknown as MessageNode;
    useChatStore.setState({ surfaceSessionsByKey: {},
      conversationInfoById: { [defaultConversationId]: withFixtureProjection({ id: defaultConversationId, title: 'Conversa' }, { nodes: [assistant] }) },
    });
    await useChatStore.getState().retryMessageToConversation(defaultConversationId, 'user-1', { allowAssistantPrefill });
    const identity = { conversationId: defaultConversationId, turnId: 'turn-1', assistantMessageId: assistantId };
    emitEvent('chat:tool_start', { ...identity, name: 'read', callId: 'new-call', origin: 'builtin' });
    emitEvent('chat:tool_end', { ...identity, name: 'read', callId: 'new-call', status: 'ok' });
    emitEvent('chat:done', { ...identity, reason: 'cancelled' });
    // Seleção de ferramentas e conteúdo é provada no runtime por
    // TestExecutionWriterToolFirstTerminalUsesExplicitExecutionMode.
    expect(mockSendMessage).not.toHaveBeenCalled();
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId].isLoading).toBe(false);
    expect(mockRetryMessage).toHaveBeenCalledWith(defaultConversationId, 'user-1', expect.objectContaining({ allowAssistantPrefill }));
  });

  it('faz retry sem recarregar quando timeline já está carregada sem sessão legada', async () => {
    useChatStore.setState({
      conversationInfoById: {
        [defaultConversationId]: withFixtureProjection({ id: defaultConversationId, title: 'Conversa' }, { nodes: [] }),
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
    expect(useChatStore.getState().surfaceSessionsByKey).toEqual({});
    expect(useChatStore.getState().conversationInfoById).toEqual({});
    expect(useChatStore.getState().surfaceSessionsByKey).toEqual({});
  });

  it('propaga fim de loading para superfícies materializadas da conversa', async () => {
    const { createEmptyChatSurfaceSession } = await import('../services/chatSessionRegistry');
    const surfaceSessionKey = `tab-chat:${defaultConversationId}`;
    useChatStore.setState({
      loadingConversationIds: new Set([defaultConversationId]),
      surfaceSessionsByKey: {
        [surfaceSessionKey]: {
          ...createEmptyChatSurfaceSession(defaultConversationId, surfaceSessionKey),
          isLoading: true,
        },
      },
    });

    useChatStore.getState().cancelConversationTurn(defaultConversationId);

    expect(useChatStore.getState().loadingConversationIds.has(defaultConversationId)).toBe(false);
    expect(useChatStore.getState().surfaceSessionsByKey[surfaceSessionKey]?.isLoading).toBe(false);
  });

  it('mantém loading visual restrito à superfície de origem', async () => {
    const { createEmptyChatSurfaceSession } = await import('../services/chatSessionRegistry');
    const send = deferred<void>();
    const originSessionKey = `tab-a:${defaultConversationId}`;
    const otherSessionKey = `tab-b:${defaultConversationId}`;
    mockSendMessage.mockImplementationOnce(() => send.promise);
    useChatStore.setState({
      surfaceSessionsByKey: {
        [originSessionKey]: createEmptyChatSurfaceSession(defaultConversationId, originSessionKey),
        [otherSessionKey]: createEmptyChatSurfaceSession(defaultConversationId, otherSessionKey),
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
    const { createEmptyChatSurfaceSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `tab-a:${defaultConversationId}`;
    const otherSessionKey = `tab-b:${defaultConversationId}`;
    const mediaFile = {
      id: 'media-1',
    } as unknown as import('../services/mediaService').MediaFile;
    useChatStore.setState({
      surfaceSessionsByKey: {
        [originSessionKey]: createEmptyChatSurfaceSession(defaultConversationId, originSessionKey),
        [otherSessionKey]: createEmptyChatSurfaceSession(defaultConversationId, otherSessionKey),
      },
    });
    const sessionsBefore = useChatStore.getState().surfaceSessionsByKey;
    const metadataBefore = useChatStore.getState().conversationInfoById;

    useChatStore.getState().setConversationDraftMessage(defaultConversationId, 'rascunho A', originSessionKey);
    useChatStore.getState().setConversationDraftMediaFiles(defaultConversationId, [mediaFile], originSessionKey);

    const state = useChatStore.getState();
    expect(state.conversationInfoById).toBe(metadataBefore);
    expect(state.surfaceSessionsByKey['conversation:' + defaultConversationId]).toBe(sessionsBefore['conversation:' + defaultConversationId]);
    expect(state.surfaceSessionsByKey[originSessionKey]?.draftMessage).toBe('rascunho A');
    expect(state.surfaceSessionsByKey[originSessionKey]?.draftMediaFiles).toEqual([mediaFile]);
    expect(state.surfaceSessionsByKey[otherSessionKey]?.draftMessage).toBe('');
    expect(state.surfaceSessionsByKey[otherSessionKey]?.draftMediaFiles).toEqual([]);
  });

  it('mantém scroll e âncora visual isolados por superfície', async () => {
    const { createEmptyChatSurfaceSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `tab-a:${defaultConversationId}`;
    const otherSessionKey = `tab-b:${defaultConversationId}`;
    useChatStore.setState({
      surfaceSessionsByKey: {
        [originSessionKey]: createEmptyChatSurfaceSession(defaultConversationId, originSessionKey),
        [otherSessionKey]: createEmptyChatSurfaceSession(defaultConversationId, otherSessionKey),
      },
    });
    const sessionsBefore = useChatStore.getState().surfaceSessionsByKey;
    const metadataBefore = useChatStore.getState().conversationInfoById;

    useChatStore.getState().setConversationScrollState(
      defaultConversationId,
      { scrollTop: 320, scrollAnchorMessageId: 'message-7' },
      originSessionKey,
    );

    const state = useChatStore.getState();
    expect(state.conversationInfoById).toBe(metadataBefore);
    expect(state.surfaceSessionsByKey['conversation:' + defaultConversationId]).toBe(sessionsBefore['conversation:' + defaultConversationId]);
    expect(state.surfaceSessionsByKey[originSessionKey]?.scrollTop).toBe(320);
    expect(state.surfaceSessionsByKey[originSessionKey]?.scrollAnchorMessageId).toBe('message-7');
    expect(state.surfaceSessionsByKey[otherSessionKey]?.scrollTop).toBe(0);
    expect(state.surfaceSessionsByKey[otherSessionKey]?.scrollAnchorMessageId).toBeNull();
  });

  it('não recria estado ao persistir scroll visual idêntico', async () => {
    const { createEmptyChatSurfaceSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `tab-a:${defaultConversationId}`;
    useChatStore.setState({
      surfaceSessionsByKey: {
        [originSessionKey]: {
          ...createEmptyChatSurfaceSession(defaultConversationId, originSessionKey),
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
    const { createEmptyChatSurfaceSession } = await import('../services/chatSessionRegistry');
    const originSessionKey = `tab-a:${defaultConversationId}`;
    useChatStore.setState({
      surfaceSessionsByKey: {
        [originSessionKey]: createEmptyChatSurfaceSession(defaultConversationId, originSessionKey),
      },
    });
    const stateBefore = useChatStore.getState();

    useChatStore.getState().clearConversationDraft(defaultConversationId, originSessionKey);

    const stateAfter = useChatStore.getState();
    expect(stateAfter).toBe(stateBefore);
    expect(stateAfter.surfaceSessionsByKey).toBe(stateBefore.surfaceSessionsByKey);
  });

  it('completa origem de superfície já materializada sem criar sessão padrão', () => {
    const originSessionKey = `tab-a:${defaultConversationId}`;
    const origin = {
      sessionKey: originSessionKey,
      conversationId: defaultConversationId,
      tabId: 'tab-a',
      surfaceId: 'tab-a',
      surfaceType: 'embedded' as const,
    };
    useChatStore.getState().setConversationDraftMessage(defaultConversationId, 'rascunho A', originSessionKey);
    const sessionsBefore = useChatStore.getState().surfaceSessionsByKey;
    const metadataBefore = useChatStore.getState().conversationInfoById;

    useChatStore.getState().ensureConversationSurfaceSession(defaultConversationId, originSessionKey, origin);

    const state = useChatStore.getState();
    expect(state.conversationInfoById).toBe(metadataBefore);
    expect(state.surfaceSessionsByKey['conversation:' + defaultConversationId]).toBe(sessionsBefore['conversation:' + defaultConversationId]);
    expect(state.surfaceSessionsByKey[originSessionKey]?.draftMessage).toBe('rascunho A');
    expect(state.surfaceSessionsByKey[originSessionKey]?.surfaceOrigin).toEqual(origin);
  });

  // Saltos às bordas pertencem à apresentação: hasAfter=false não dispensa
  // Navigate(end), que também retoma followTail. Prova em chatTimelinePresentation.

  it('limpa mensagens usando timeline mesmo sem sessão legada', async () => {
    useChatStore.setState({
      conversationInfoById: {
        [defaultConversationId]: withFixtureProjection({ id: defaultConversationId, title: 'Conversa' }, { nodes: [
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
          ] }),
      },
      surfaceSessionsByKey: {},
    });

    useChatStore.getState().handleConversationCleared(defaultConversationId);

    expect(useChatStore.getState().conversationInfoById[defaultConversationId]).not.toHaveProperty('threadedMessages');
    expect(useChatStore.getState().conversationInfoById[defaultConversationId]?.title).toBe('chat.conversationCleared');
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
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId]?.queuedTurnCount).toBe(1);

    firstSend.resolve();
    await first;
    emitEvent('chat:done', { conversationId: defaultConversationId });
    await flushMicrotasks();
    await second;

    expect(mockSendMessage).toHaveBeenCalledTimes(2);
    expect(mockSendMessage.mock.calls[1][1]).toBe('segunda');
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId]?.queuedTurnCount).toBe(0);
  });

  it.each(['envio', 'retry'] as const)(
    'tratamento de comando converte cancelamento de %s enfileirado em stale',
    async (operation) => {
      const firstSend = deferred<void>();
      mockSendMessage.mockImplementationOnce(() => firstSend.promise);
      const first = useChatStore.getState().sendMessageToConversation(defaultConversationId, 'primeira');
      await flushMicrotasks();

      const command = {
        handoff: { ticket: `ticket-${operation}`, handoffId: `handoff-${operation}` },
        isCurrent: () => true,
      };
      const queued = operation === 'envio'
        ? useChatStore.getState().sendMessageToConversation(defaultConversationId, 'comando', undefined, undefined, { command })
        : useChatStore.getState().retryMessageToConversation(defaultConversationId, 'persisted-message', undefined, { command });
      await flushMicrotasks();

      expect(useChatStore.getState().cancelNextQueuedTurn(defaultConversationId)).toBe(true);
      await expect(queued).rejects.toThrow('chat-messaging-stale');

      firstSend.resolve();
      emitEvent('chat:done', { conversationId: defaultConversationId });
      await first;
      expect(mockSendMessage).toHaveBeenCalledTimes(1);
      expect(mockRetryMessage).not.toHaveBeenCalled();
    },
  );

  it('serializa retry atrás de envio ativo da mesma conversa', async () => {
    const firstSend = deferred<void>();
    mockSendMessage.mockImplementationOnce(() => firstSend.promise);
    mockRetryMessage.mockResolvedValueOnce(undefined);

    const first = useChatStore.getState().sendMessageToConversation(defaultConversationId, 'primeira');
    const retry = useChatStore.getState().retryMessageToConversation(defaultConversationId, 'message-1');

    await flushMicrotasks();
    expect(mockSendMessage).toHaveBeenCalledTimes(1);
    expect(mockRetryMessage).not.toHaveBeenCalled();
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId]?.queuedTurnCount).toBe(1);

    firstSend.resolve();
    await first;
    emitEvent('chat:done', { conversationId: defaultConversationId });
    await flushMicrotasks();
    await retry;

    expect(mockRetryMessage).toHaveBeenCalledWith(defaultConversationId, 'message-1', expect.any(Object));
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId]?.queuedTurnCount).toBe(0);
  });

  it('conta e cancela envio de comando enquanto aguarda admissão', async () => {
    const firstSend = deferred<void>();
    mockSendMessage.mockImplementationOnce(() => firstSend.promise);

    const first = useChatStore.getState().sendMessageToConversation(defaultConversationId, 'primeira');
    await flushMicrotasks();
    expect(mockSendMessage).toHaveBeenCalledTimes(1);

    const admission = useChatStore.getState().waitForMessagingAdmission(defaultConversationId);
    await flushMicrotasks();
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId]?.queuedTurnCount).toBe(1);
    expect(useChatStore.getState().cancelNextQueuedTurn(defaultConversationId)).toBe(true);
    await expect(admission).rejects.toThrow('chat-messaging-stale');
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId]?.queuedTurnCount).toBe(0);
    expect(mockSendMessage).toHaveBeenCalledTimes(1);

    firstSend.resolve();
    await first;
    emitEvent('chat:done', { conversationId: defaultConversationId });
  });

  it('mantém contagem e cancelamento após a barreira começar antes da vez do executor', async () => {
    const firstAdmission = useChatStore.getState().registerMessagingAdmission(defaultConversationId);
    expect(firstAdmission.activate()).toBe(true);
    const secondAdmission = useChatStore.getState().registerMessagingAdmission(defaultConversationId);
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId]?.queuedTurnCount).toBe(1);

    firstAdmission.complete();
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId]?.queuedTurnCount).toBe(1);
    expect(useChatStore.getState().cancelNextQueuedTurn(defaultConversationId)).toBe(true);
    await expect(secondAdmission.cancelled).rejects.toBeDefined();
    expect(secondAdmission.activate()).toBe(false);
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId]?.queuedTurnCount).toBe(0);
  });

  it('limpeza de admissão antiga não subtrai a contagem de uma nova entrada', async () => {
    const firstSend = deferred<void>();
    const secondSend = deferred<void>();
    const thirdSend = deferred<void>();
    mockSendMessage
      .mockImplementationOnce(() => firstSend.promise)
      .mockImplementationOnce(() => secondSend.promise)
      .mockImplementationOnce(() => thirdSend.promise);
    const first = useChatStore.getState().sendMessageToConversation(defaultConversationId, 'primeira');
    await flushMicrotasks();

    const oldAdmission = useChatStore.getState().waitForMessagingAdmission(defaultConversationId);
    await flushMicrotasks();
    useChatStore.getState().cancelConversationTurn(defaultConversationId);
    const second = useChatStore.getState().sendMessageToConversation(defaultConversationId, 'segunda');
    const third = useChatStore.getState().sendMessageToConversation(defaultConversationId, 'terceira');
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId]?.queuedTurnCount).toBe(2);

    firstSend.resolve();
    emitEvent('chat:done', { conversationId: defaultConversationId });
    await first;
    await expect(oldAdmission).rejects.toThrow('chat-messaging-stale');
    await flushMicrotasks();
    expect(mockSendMessage).toHaveBeenCalledTimes(2);
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId]?.queuedTurnCount).toBe(1);

    secondSend.resolve();
    emitEvent('chat:done', { conversationId: defaultConversationId });
    await second;
    await flushMicrotasks();
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId]?.queuedTurnCount).toBe(1);
    thirdSend.resolve();
    emitEvent('chat:done', { conversationId: defaultConversationId });
    await third;
    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId]?.queuedTurnCount).toBe(0);
  });

  it('resposta tardia do cancelamento não limpa o controller do próximo envio', async () => {
    const { getChatEventControllerCleanup } = await import('../services/chatEventController');
    const cancellation = deferred<void>();
    mockCancelStreaming.mockImplementationOnce(() => cancellation.promise);
    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'primeira');
    const cancel = useChatStore.getState().cancelStreaming(defaultConversationId);
    emitEvent('chat:done', { conversationId: defaultConversationId });
    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'segunda');
    const nextController = getChatEventControllerCleanup(defaultConversationId);
    expect(nextController).toBeTypeOf('function');
    cancellation.resolve();
    await cancel;

    expect(useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId]?.isLoading).toBe(true);
    emitEvent('chat:stream', {
      conversationId: defaultConversationId,
      messageId: 'second-assistant',
      content: 'segundo envio ativo',
      done: false,
    });
    expect(getChatEventControllerCleanup(defaultConversationId)).toBe(nextController);
    emitEvent('chat:done', { conversationId: defaultConversationId });
  });

  it('cancela a identidade do controller ativo mesmo quando solicitado por outra superfície', async () => {
    const origin = {
      conversationId: defaultConversationId,
      sessionKey: `page:a:${defaultConversationId}`,
      surfaceId: 'page:a',
      surfaceType: 'page' as const,
    };
    await useChatStore.getState().sendMessageToConversation(defaultConversationId, 'primeira', undefined, undefined, { origin });
    const executionId = mockSendMessage.mock.calls[0][3].surfaceExecutionId;
    expect(executionId).toEqual(expect.any(String));
    await useChatStore.getState().cancelStreaming(defaultConversationId);
    expect(mockCancelStreaming).toHaveBeenCalledWith(defaultConversationId, executionId);
  });

  it('mantem envios de conversas diferentes em paralelo', async () => {
    const firstSend = deferred<void>();
    const otherConversationId = '01926b90-7a5a-7c4e-8d3f-000000000007';
    useChatStore.setState({
      conversationInfoById: {
        ...useChatStore.getState().conversationInfoById,
        [otherConversationId]: withFixtureProjection(
          { id: otherConversationId, title: 'Outra' },
          { nodes: [] }
        ),
      },
      surfaceSessionsByKey: {
        ...useChatStore.getState().surfaceSessionsByKey,
        ['conversation:' + otherConversationId]: {
          ...useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId],
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

  it('libera a fila após erro no stream e conclui o próximo envio sem criar mensagens locais', async () => {
    useChatStore.setState({
      conversationInfoById: {
        '01926b90-7a5a-7c4e-8d3f-000000000001': withFixtureProjection(
          { id: '01926b90-7a5a-7c4e-8d3f-000000000001', title: 'Conversa' },
          { nodes: [] }
        ),
      },
      surfaceSessionsByKey: {
        ['conversation:' + '01926b90-7a5a-7c4e-8d3f-000000000001']: {
          ...useChatStore.getState().surfaceSessionsByKey[
            'conversation:' + '01926b90-7a5a-7c4e-8d3f-000000000001'
          ],
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
    await flushMicrotasks();

    expect(mockSendMessage).toHaveBeenCalledTimes(2);
    expect(mockSendMessage.mock.calls.map(call => call[1])).toEqual(['falha 1', 'ok 2']);
    const session = useChatStore.getState().surfaceSessionsByKey['conversation:' + defaultConversationId];
    expect(session.isLoading).toBe(false);
    expect(session.queuedTurnCount).toBe(0);
    expect(session.sendFailureMessage).toBeNull();
    expect(session).not.toHaveProperty('conversation');
    expect(useChatStore.getState().conversationInfoById[defaultConversationId]).not.toHaveProperty('threadedMessages');
  });

});
