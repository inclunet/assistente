import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { chat } from '../../wailsjs/go/models';
import {
  startChatEventController,
  stopAllChatEventControllers,
  type ChatEventControllerAdapter,
  type ChatEventSession,
} from './chatEventController';
import {
  updateMessageContentInTree,
  updateMessageReasoningInTree,
  type ChatTreeConversation,
  type Message,
  type MessageNode,
} from '../lib/chatMessageTree';
import type { ChatSurfaceOrigin } from './chatSessionRegistry';

const mockAnnounce = vi.fn();
vi.mock('../hooks/useAnnouncer', () => ({
  announce: (...args: unknown[]) => mockAnnounce(...args),
}));

const mockReloadConversationSnapshot = vi.fn().mockResolvedValue({
  threadedMessages: [],
  messageWindow: {
    scope: 'conversation',
    conversationId: 'conversation-1',
    totalCount: 0,
    startIndex: 0,
    endIndex: -1,
    hasBefore: false,
    hasAfter: false,
  },
  hasOlderMessages: false,
  hasNewerMessages: false,
});
vi.mock('@wailsjs/go/app/App', () => ({
}));
vi.mock('./chatSessionLoader', () => ({
  reloadConversationSnapshot: (...args: unknown[]) => mockReloadConversationSnapshot(...args),
}));
vi.mock('./messageWindowLimits', () => ({
  INITIAL_MESSAGE_WINDOW_SIZE: 80,
}));

type EventCallback = (data: unknown) => void;
const eventListeners = new Map<string, EventCallback[]>();

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (event: string, cb: EventCallback) => {
    const list = eventListeners.get(event) || [];
    list.push(cb);
    eventListeners.set(event, list);
    return () => {
      const current = eventListeners.get(event) || [];
      eventListeners.set(event, current.filter((candidate) => candidate !== cb));
    };
  },
}));

const mockPlayChatReceiveSoundIfActive = vi.fn();
const mockPlayChatErrorSoundIfActive = vi.fn();
const mockAnnounceForActiveChatConversation = vi.fn();
const mockAnnounceChatBackgroundResponseDone = vi.fn();
const mockGetChatConversationVoiceOrigin = vi.fn((
  conversationId: string,
  _fallbackTitle?: string | null,
  surfaceOrigin?: ChatSurfaceOrigin | null,
) => {
  return {
    conversationId,
    surfaceId: surfaceOrigin?.surfaceId ?? `conversation:${conversationId}`,
    sessionKey: surfaceOrigin?.sessionKey ?? `conversation:${conversationId}`,
    surfaceType: surfaceOrigin?.surfaceType ?? 'page',
    tabId: surfaceOrigin?.tabId,
    title: `Conversa ${conversationId}`,
  };
});
vi.mock('./chatArbitration', () => ({
  playChatReceiveSoundIfActive: (...args: unknown[]) => mockPlayChatReceiveSoundIfActive(...args),
  playChatErrorSoundIfActive: (...args: unknown[]) => mockPlayChatErrorSoundIfActive(...args),
  announceForActiveChatConversation: (...args: unknown[]) => mockAnnounceForActiveChatConversation(...args),
  announceChatBackgroundResponseDone: (...args: unknown[]) => mockAnnounceChatBackgroundResponseDone(...args),
  getChatConversationVoiceOrigin: (conversationId: string, fallbackTitle?: string | null, origin?: ChatSurfaceOrigin | null) => (
    mockGetChatConversationVoiceOrigin(conversationId, fallbackTitle, origin)
  ),
}));

const mockHandleChatSpeak = vi.fn().mockResolvedValue(undefined);
vi.mock('./chatSpeak', () => ({
  handleChatSpeak: (...args: unknown[]) => Promise.resolve(mockHandleChatSpeak(...args)),
}));

vi.mock('i18next', () => ({
  default: {
    t: (key: string, opts?: Record<string, unknown>) => {
      if (key === 'chat.errorPrefix') return `Erro: ${opts?.message}`;
      if (key === 'chat.sendErrorPrefix') return `Falha ao enviar: ${opts?.message}`;
      if (key === 'chat.announce.externalMessage') return `${opts?.from} via ${opts?.channel}: ${opts?.message}`;
      return key;
    },
  },
}));

interface TestSession extends ChatEventSession {
  isLoading: boolean;
  streamingMessageId: string | null;
  lastInterruptedMessageId: string | null;
  streamingReasoning: string | null;
  isThinking: boolean;
}

const createMessage = (id: string, role: string, content = '', conversationId = 'conversation-1'): Message => (
  new chat.EnrichedMessage({
    id,
    role,
    content,
    conversationId,
    isStreaming: false,
    internal: false,
    createdAt: '2026-04-30T00:00:00.000Z',
  }) as Message
);

const createNode = (message: Message): MessageNode => (
  new chat.MessageNode({
    message,
    children: [],
    level: 0,
    childCount: 0,
  }) as MessageNode
);

const createConversation = (id: string): ChatTreeConversation => ({
  id,
  title: `Conversa ${id}`,
  threadedMessages: [],
});

const createSession = (conversationId: string): TestSession => ({
  conversation: createConversation(conversationId),
  isLoading: false,
  streamingMessageId: null,
  sendFailureMessage: null,
  sendFailureRetryable: false,
  lastInterruptedMessageId: null,
  streamingReasoning: null,
  isThinking: false,
  activeToolCalls: [],
  completedSegments: [],
});

function emitEvent(name: string, data: unknown) {
  const listeners = eventListeners.get(name) || [];
  for (const listener of listeners) listener(data);
}

function createAdapter(initialConversationIds: string[]) {
  const sessions: Record<string, TestSession> = Object.fromEntries(
    initialConversationIds.map((conversationId) => [conversationId, createSession(conversationId)]),
  );

  const adapter: ChatEventControllerAdapter = {
    getSession: (conversationId) => sessions[conversationId] ?? createSession(conversationId),
    patchSession: (conversationId, patch) => {
      const current = sessions[conversationId] ?? createSession(conversationId);
      sessions[conversationId] = {
        ...current,
        ...patch,
      } as TestSession;
    },
    patchConversation: (conversationId, updater) => {
      const current = sessions[conversationId] ?? createSession(conversationId);
      if (!current.conversation) return;
      sessions[conversationId] = {
        ...current,
        conversation: updater(current.conversation),
      };
    },
    updateMessage: (conversationId, messageId, content) => {
      const current = sessions[conversationId] ?? createSession(conversationId);
      if (!current.conversation) return;
      sessions[conversationId] = {
        ...current,
        conversation: {
          ...current.conversation,
          threadedMessages: updateMessageContentInTree(current.conversation.threadedMessages, messageId, content),
        },
      };
    },
    updateReasoning: (conversationId, messageId, reasoning) => {
      const current = sessions[conversationId] ?? createSession(conversationId);
      if (!current.conversation) return;
      sessions[conversationId] = {
        ...current,
        conversation: {
          ...current.conversation,
          threadedMessages: updateMessageReasoningInTree(current.conversation.threadedMessages, messageId, reasoning),
        },
      };
    },
    setConversationLoading: (conversationId, isLoading) => {
      const current = sessions[conversationId] ?? createSession(conversationId);
      sessions[conversationId] = {
        ...current,
        isLoading,
      };
    },
  };

  return {
    adapter,
    sessions,
  };
}

describe('chatEventController', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    eventListeners.clear();
    mockAnnounce.mockClear();
    mockReloadConversationSnapshot.mockReset();
    mockReloadConversationSnapshot.mockResolvedValue({
      threadedMessages: [],
      messageWindow: {
        scope: 'conversation',
        conversationId: 'conversation-1',
        totalCount: 0,
        startIndex: 0,
        endIndex: -1,
        hasBefore: false,
        hasAfter: false,
      },
      hasOlderMessages: false,
      hasNewerMessages: false,
    });
    mockPlayChatReceiveSoundIfActive.mockClear();
    mockPlayChatErrorSoundIfActive.mockClear();
    mockAnnounceForActiveChatConversation.mockClear();
    mockAnnounceChatBackgroundResponseDone.mockClear();
    mockHandleChatSpeak.mockClear();
  });

  afterEach(() => {
    stopAllChatEventControllers();
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
    eventListeners.clear();
  });

  it('mantém eventos isolados por conversationId', () => {
    const { adapter, sessions } = createAdapter(['conversation-1', 'conversation-2']);

    startChatEventController({ conversationId: 'conversation-1', initialUserContent: 'oi 1', adapter });
    startChatEventController({ conversationId: 'conversation-2', initialUserContent: 'oi 2', adapter });

    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'oi 1',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'resposta 1',
      done: false,
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'resposta 1',
      done: true,
      messageId: 'assistant-1',
    });

    expect(sessions['conversation-1'].conversation?.threadedMessages.map((node) => node.message.id)).toEqual([
      'user-1',
      'assistant-1',
    ]);
    expect(sessions['conversation-2'].conversation?.threadedMessages).toEqual([]);
    expect(mockPlayChatReceiveSoundIfActive).toHaveBeenCalledWith('conversation-1', undefined);
  });

  it('reiniciar a mesma conversa cancela o controller anterior sem criar assistant local sem messageId', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', initialUserContent: 'primeira', adapter });
    startChatEventController({ conversationId: 'conversation-1', initialUserContent: 'segunda', adapter });

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'resposta',
      done: false,
    });

    expect(sessions['conversation-1'].conversation?.threadedMessages).toEqual([]);
  });

  it('processa messages_ready, stream e done atualizando a sessão correta', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', initialUserContent: 'fallback', adapter });

    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'pergunta',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'parcial',
      done: false,
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'parcial',
      done: true,
      messageId: 'assistant-1',
    });
    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      hadToolCalls: false,
    });

    const messages = sessions['conversation-1'].conversation?.threadedMessages ?? [];
    expect(messages[0].message.content).toBe('pergunta');
    expect(messages[1].message.content).toBe('parcial');
    expect(messages[1].message.isStreaming).toBe(false);
    expect(sessions['conversation-1'].isLoading).toBe(false);
    expect(mockAnnounceChatBackgroundResponseDone).toHaveBeenCalledWith('conversation-1', 'Conversa conversation-1', undefined);
  });

  it('em erro no chat:done sem assistantMessageId não cria mensagem assistant local', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'pergunta',
      turnId: 'user-1',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'parcial',
      done: false,
      turnId: 'user-1',
    });

    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      hadToolCalls: false,
      errorMessage: 'boom',
      turnId: 'user-1',
    });

    const messages = sessions['conversation-1'].conversation?.threadedMessages ?? [];
    expect(messages).toHaveLength(1);
    expect(messages[0].message.id).toBe('user-1');
    expect(sessions['conversation-1'].lastInterruptedMessageId).toBeNull();
    expect(sessions['conversation-1'].sendFailureMessage).toBe('boom');
    expect(sessions['conversation-1'].sendFailureRetryable).toBe(false);
    expect(mockAnnounce).toHaveBeenCalledWith('boom', 'assertive');
    expect(mockPlayChatErrorSoundIfActive).toHaveBeenCalledWith('conversation-1', undefined);
  });

  it('em erro no chat:done usa assistantMessageId persistido quando disponível', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'pergunta',
      turnId: 'user-1',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'parcial',
      done: false,
      turnId: 'user-1',
      messageId: 'assistant-db-1',
    });

    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      hadToolCalls: false,
      errorMessage: 'boom',
      turnId: 'user-1',
      assistantMessageId: 'assistant-db-1',
    });

    const messages = sessions['conversation-1'].conversation?.threadedMessages ?? [];
    expect(messages[1].message.id).toBe('assistant-db-1');
    expect(messages[1].message.content).toBe('parcial');
    expect(sessions['conversation-1'].lastInterruptedMessageId).toBe('assistant-db-1');
  });

  it('em erro no chat:stream usa messageId persistido para interrupção', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'pergunta',
      turnId: 'user-1',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'parcial',
      done: false,
      turnId: 'user-1',
      messageId: 'assistant-db-2',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      error: 'boom stream',
      turnId: 'user-1',
      messageId: 'assistant-db-2',
    });

    expect(sessions['conversation-1'].lastInterruptedMessageId).toBe('assistant-db-2');
  });

  it('preenche fallback visual quando chat:stream falha sem parcial', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'pergunta',
      turnId: 'user-1',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      error: 'boom stream',
      turnId: 'user-1',
      messageId: 'assistant-db-2',
    });

    const messages = sessions['conversation-1'].conversation?.threadedMessages ?? [];
    expect(messages[1].message.content).toBe('Erro: boom stream');
    expect(sessions['conversation-1'].lastInterruptedMessageId).toBe('assistant-db-2');
    expect(mockPlayChatErrorSoundIfActive).toHaveBeenCalledWith('conversation-1', undefined);
  });

  it('preenche erro de chat:stream no assistant já criado mesmo sem messageId terminal', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:tool_start', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      assistantMessageId: 'assistant-db-existing',
      name: 'buscar',
      callId: 'call-1',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      error: 'boom sem id terminal',
      turnId: 'user-1',
    });

    const messages = sessions['conversation-1'].conversation?.threadedMessages ?? [];
    expect(messages[0].message.id).toBe('assistant-db-existing');
    expect(messages[0].message.content).toBe('Erro: boom sem id terminal');
    expect(sessions['conversation-1'].lastInterruptedMessageId).toBe('assistant-db-existing');
  });

  it('preenche erro de chat:done no assistant já criado mesmo sem assistantMessageId terminal', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:thinking', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      assistantMessageId: 'assistant-db-existing',
      started: true,
    });
    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      hadToolCalls: false,
      errorMessage: 'done sem id terminal',
    });

    const messages = sessions['conversation-1'].conversation?.threadedMessages ?? [];
    expect(messages[0].message.id).toBe('assistant-db-existing');
    expect(messages[0].message.content).toBe('Erro: done sem id terminal');
    expect(sessions['conversation-1'].lastInterruptedMessageId).toBe('assistant-db-existing');
  });

  it('mostra tool calls antes do primeiro chunk usando assistantMessageId persistido', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:tool_start', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      assistantMessageId: 'assistant-db-tool',
      name: 'buscar',
      callId: 'call-1',
      args: '{}',
    });

    const messages = sessions['conversation-1'].conversation?.threadedMessages ?? [];
    expect(messages.map((node) => node.message.id)).toEqual(['assistant-db-tool']);
    expect(sessions['conversation-1'].streamingMessageId).toBe('assistant-db-tool');
    expect(sessions['conversation-1'].activeToolCalls[0]).toMatchObject({
      name: 'buscar',
      callId: 'call-1',
      status: 'running',
    });
  });

  it('não bloqueia criação posterior quando a conversa ainda não está carregada', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);
    sessions['conversation-1'].conversation = null;

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:tool_start', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      assistantMessageId: 'assistant-db-delayed',
      name: 'buscar',
      callId: 'call-1',
    });

    sessions['conversation-1'].conversation = createConversation('conversation-1');

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      messageId: 'assistant-db-delayed',
      content: 'resposta após carregar',
      done: false,
    });
    vi.runOnlyPendingTimers();

    const messages = sessions['conversation-1'].conversation?.threadedMessages ?? [];
    expect(messages.map((node) => node.message.id)).toEqual(['assistant-db-delayed']);
    expect(messages[0].message.content).toBe('resposta após carregar');
  });

  it('marca assistant existente como streaming ao reutilizar messageId persistido', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);
    sessions['conversation-1'].conversation = {
      ...createConversation('conversation-1'),
      threadedMessages: [createNode(createMessage('assistant-db-existing', 'assistant', ''))],
    };

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      messageId: 'assistant-db-existing',
      content: 'resposta em andamento',
      done: false,
    });
    vi.runOnlyPendingTimers();

    const messages = sessions['conversation-1'].conversation?.threadedMessages ?? [];
    expect(messages[0].message.id).toBe('assistant-db-existing');
    expect(messages[0].message.isStreaming).toBe(true);
    expect(messages[0].message.content).toBe('resposta em andamento');
  });

  it('preserva reasoning final antes do primeiro chunk usando assistantMessageId persistido', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:thinking', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      assistantMessageId: 'assistant-db-thinking',
      content: 'raciocínio final',
      done: true,
    });

    const messages = sessions['conversation-1'].conversation?.threadedMessages ?? [];
    expect(messages.map((node) => node.message.id)).toEqual(['assistant-db-thinking']);
    expect(messages[0].message.reasoning).toBe('raciocínio final');
  });

  it('toca som de erro em chat:error e chat:tool_failure final, respeitando arbitragem', () => {
    const { adapter } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:tool_failure', {
      conversationId: 'conversation-1',
      name: 'minha_tool',
      callId: 'call-1',
      willRetry: true,
    });
    expect(mockPlayChatErrorSoundIfActive).not.toHaveBeenCalled();

    emitEvent('chat:tool_failure', {
      conversationId: 'conversation-1',
      name: 'minha_tool',
      callId: 'call-1',
      willRetry: false,
    });
    expect(mockPlayChatErrorSoundIfActive).toHaveBeenCalledWith('conversation-1', undefined);

    mockPlayChatErrorSoundIfActive.mockClear();
    emitEvent('chat:error', {
      conversationId: 'conversation-1',
      error: 'falha geral',
    });
    expect(mockPlayChatErrorSoundIfActive).toHaveBeenCalledWith('conversation-1', undefined);
  });

  it('preenche fallback visual quando chat:done falha sem parcial', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'pergunta',
      turnId: 'user-1',
    });
    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      hadToolCalls: false,
      errorMessage: 'boom done',
      turnId: 'user-1',
      assistantMessageId: 'assistant-db-3',
    });

    const messages = sessions['conversation-1'].conversation?.threadedMessages ?? [];
    expect(messages[1].message.content).toBe('Erro: boom done');
    expect(sessions['conversation-1'].lastInterruptedMessageId).toBe('assistant-db-3');
  });

  it('propaga surfaceOrigin dos eventos para anúncio, som e fala', async () => {
    const { adapter } = createAdapter(['conversation-1']);
    const surfaceOrigin = {
      conversationId: 'conversation-1',
      sessionKey: 'tab-1:conversation-1',
      surfaceId: 'tab-1',
      surfaceType: 'page' as const,
      tabId: 'tab-1',
    };

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'resposta',
      done: false,
      messageId: 'assistant-1',
      surfaceOrigin,
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'resposta',
      done: true,
      messageId: 'assistant-1',
      surfaceOrigin,
    });
    emitEvent('chat:speak', {
      conversationId: 'conversation-1',
      role: 'assistant',
      text: 'fala com origem',
      strategy: 'webspeech',
      surfaceOrigin,
    });
    await Promise.resolve();
    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      hadToolCalls: false,
      surfaceOrigin,
    });

    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledWith(
      'conversation-1',
      'chat.announce.assistantResponding',
      'polite',
      surfaceOrigin,
    );
    expect(mockPlayChatReceiveSoundIfActive).toHaveBeenCalledWith('conversation-1', surfaceOrigin);
    expect(mockGetChatConversationVoiceOrigin).toHaveBeenCalledWith('conversation-1', undefined, surfaceOrigin);
    expect(mockHandleChatSpeak.mock.calls[0][0]).toMatchObject({
      accessibilityOrigin: expect.objectContaining({
        conversationId: 'conversation-1',
        sessionKey: 'tab-1:conversation-1',
        surfaceId: 'tab-1',
        tabId: 'tab-1',
      }),
    });
    expect(mockAnnounceChatBackgroundResponseDone).toHaveBeenCalledWith(
      'conversation-1',
      'Conversa conversation-1',
      surfaceOrigin,
    );
  });

  it('mantém chat:speak ativo até o done e ignora eventos depois do cleanup', async () => {
    const { adapter } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:speak', {
      conversationId: 'conversation-1',
      role: 'assistant',
      text: 'antes do done',
      strategy: 'webspeech',
    });
    await Promise.resolve();

    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      hadToolCalls: false,
    });
    emitEvent('chat:speak', {
      conversationId: 'conversation-1',
      role: 'assistant',
      text: 'depois do done',
      strategy: 'webspeech',
    });
    await Promise.resolve();

    expect(mockHandleChatSpeak).toHaveBeenCalledTimes(1);
    expect(mockHandleChatSpeak.mock.calls[0][0]).toMatchObject({
      text: 'antes do done',
      accessibilityOrigin: {
        conversationId: 'conversation-1',
        surfaceId: 'conversation:conversation-1',
        title: 'Conversa conversation-1',
      },
    });
  });

  it('ignora update de streaming sem messageId ao limpar controller', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    const handle = startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'conteúdo atrasado',
      done: false,
    });
    handle.cleanup();
    vi.runOnlyPendingTimers();

    expect(sessions['conversation-1'].conversation?.threadedMessages).toEqual([]);
  });

  it('entrada externa anuncia origem e usa a sessão por conversationId', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({
      conversationId: 'conversation-1',
      external: { channel: 'telegram', from: 'Maria', text: 'fallback externo' },
      adapter,
    });

    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-ext',
      userContent: '**olá** externo',
    });

    expect(sessions['conversation-1'].conversation?.threadedMessages[0].message).toMatchObject({
      id: 'user-ext',
      content: '**olá** externo',
      source: 'telegram',
    });
    expect(mockAnnounce).toHaveBeenCalledWith('Maria via telegram: olá externo');
  });

  it('recarrega janela canônica ao finalizar resposta com tool calls', async () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);
    const backendNodes = [
      createNode(createMessage('backend-user', 'user', 'pergunta')),
      createNode(createMessage('backend-assistant', 'assistant', 'resposta com ferramenta')),
    ];
    mockReloadConversationSnapshot.mockResolvedValue({
      threadedMessages: backendNodes,
      messageWindow: {
        scope: 'conversation',
        conversationId: 'conversation-1',
        totalCount: 2,
        startIndex: 0,
        endIndex: 1,
        hasBefore: false,
        hasAfter: false,
      },
      hasOlderMessages: false,
      hasNewerMessages: false,
    });

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'resposta temporária',
      done: false,
    });
    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      hadToolCalls: true,
    });
    await Promise.resolve();

    expect(mockReloadConversationSnapshot).toHaveBeenCalledWith('conversation-1', 80);
    expect(sessions['conversation-1'].conversation?.threadedMessages.map((node) => node.message.id)).toEqual([
      'backend-user',
      'backend-assistant',
    ]);
  });
});
