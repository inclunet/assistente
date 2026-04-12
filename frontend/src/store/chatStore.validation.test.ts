import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// ── Mocks ──────────────────────────────────────────────────────────────

const mockAnnounce = vi.fn();
vi.mock('../hooks/useAnnouncer', () => ({
  announce: (...args: unknown[]) => mockAnnounce(...args),
}));

const mockSendMessage = vi.fn().mockResolvedValue(undefined);
vi.mock('@wailsjs/go/main/App', () => ({
  SendMessage: (...args: unknown[]) => mockSendMessage(...args),
  GetMessages: vi.fn().mockResolvedValue([]),
  GetConversationInfo: vi.fn().mockResolvedValue({}),
  EnsureConversation: vi.fn().mockResolvedValue(1),
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
  handleChatSpeak: (...args: unknown[]) => mockHandleChatSpeak(...args),
}));

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

describe('chatStore validation', () => {
  let useChatStore: typeof import('./chatStore').useChatStore;

  beforeEach(async () => {
    vi.resetModules();
    eventListeners.clear();
    mockAnnounce.mockClear();
    mockSendMessage.mockClear();
    mockHandleChatSpeak.mockClear();
    const mod = await import('./chatStore');
    useChatStore = mod.useChatStore;
    useChatStore.setState({ activeConversationId: 1 });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('rejects message exceeding max content size', async () => {
    const bigContent = 'x'.repeat(512 * 1024 + 1);

    await useChatStore.getState().sendMessage(bigContent);

    expect(mockAnnounce).toHaveBeenCalledTimes(1);
    expect(mockAnnounce.mock.calls[0][0]).toContain('grande');
    expect(mockSendMessage).not.toHaveBeenCalled();
  });

  it('accepts message at exact max content size', async () => {
    const exactContent = 'x'.repeat(512 * 1024);

    await useChatStore.getState().sendMessage(exactContent);

    expect(mockSendMessage).toHaveBeenCalled();
  });

  it('rejects media exceeding max size', async () => {
    const fakeFile = new File([new ArrayBuffer(15 * 1024 * 1024)], 'big.bin', { type: 'application/octet-stream' });

    await useChatStore.getState().sendMessage('hello', [{
      id: 'test-1',
      file: fakeFile,
      category: 'document' as any,
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
      emitEvent('chat:error', { conversationId: 1, error: 'Provedor LLM não disponível' });
      return Promise.reject(new Error('backend error'));
    });

    await useChatStore.getState().sendMessage('hello');

    expect(mockAnnounce).toHaveBeenCalledWith('Provedor LLM não disponível');
    expect(useChatStore.getState().isLoading).toBe(false);
  });

  it('rejects when no active conversation', async () => {
    useChatStore.setState({ activeConversationId: null });

    await useChatStore.getState().sendMessage('hello');

    expect(mockSendMessage).not.toHaveBeenCalled();
  });

  it('chat:speak event invokes handleChatSpeak for matching conversation', async () => {
    mockSendMessage.mockImplementation(() => {
      // Simula backend: emite chat:messages_ready, depois chat:speak do user
      emitEvent('chat:messages_ready', { conversationId: 1, userMessageId: 10, userContent: 'oi' });
      emitEvent('chat:speak', { conversationId: 1, role: 'user', text: 'oi', strategy: 'announce', origin: 'user_message' });
      // Simula chat:speak do assistant (antes do done)
      emitEvent('chat:speak', { conversationId: 1, role: 'assistant', text: 'Resposta', strategy: 'webspeech', origin: 'assistant_message' });
      emitEvent('chat:done', { conversationId: 1, assistantMessageId: 11, hadToolCalls: false });
      return Promise.resolve();
    });

    await useChatStore.getState().sendMessage('oi');

    expect(mockHandleChatSpeak).toHaveBeenCalledTimes(2);
    expect(mockHandleChatSpeak.mock.calls[0][0]).toMatchObject({
      conversationId: 1,
      role: 'user',
      strategy: 'announce',
    });
    expect(mockHandleChatSpeak.mock.calls[1][0]).toMatchObject({
      conversationId: 1,
      role: 'assistant',
      strategy: 'webspeech',
    });
  });

  it('chat:speak event is ignored for different conversation', async () => {
    mockSendMessage.mockImplementation(() => {
      emitEvent('chat:messages_ready', { conversationId: 1, userMessageId: 10, userContent: 'oi' });
      // Evento de outra conversa
      emitEvent('chat:speak', { conversationId: 999, role: 'assistant', text: 'Outro', strategy: 'announce' });
      emitEvent('chat:done', { conversationId: 1, assistantMessageId: 11, hadToolCalls: false });
      return Promise.resolve();
    });

    await useChatStore.getState().sendMessage('oi');

    expect(mockHandleChatSpeak).not.toHaveBeenCalled();
  });
});
