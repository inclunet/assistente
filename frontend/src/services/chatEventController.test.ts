import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { main } from '../../wailsjs/go/models';
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

const mockAnnounce = vi.fn();
vi.mock('../hooks/useAnnouncer', () => ({
  announce: (...args: unknown[]) => mockAnnounce(...args),
}));

const mockGetMessages = vi.fn().mockResolvedValue([]);
vi.mock('@wailsjs/go/app/App', () => ({
  GetMessages: (...args: unknown[]) => mockGetMessages(...args),
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
const mockAnnounceForActiveChatConversation = vi.fn();
const mockAnnounceChatBackgroundResponseDone = vi.fn();
const mockGetChatConversationVoiceOrigin = vi.fn((conversationId: string) => ({
  conversationId,
  surfaceId: `conversation:${conversationId}`,
  sessionKey: `conversation:${conversationId}`,
  surfaceType: 'page',
  title: `Conversa ${conversationId}`,
}));
vi.mock('./chatArbitration', () => ({
  playChatReceiveSoundIfActive: (...args: unknown[]) => mockPlayChatReceiveSoundIfActive(...args),
  announceForActiveChatConversation: (...args: unknown[]) => mockAnnounceForActiveChatConversation(...args),
  announceChatBackgroundResponseDone: (...args: unknown[]) => mockAnnounceChatBackgroundResponseDone(...args),
  getChatConversationVoiceOrigin: (...args: unknown[]) => mockGetChatConversationVoiceOrigin(...args),
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
  streamingReasoning: string | null;
  isThinking: boolean;
}

const createMessage = (id: string, role: string, content = '', conversationId = 'conversation-1'): Message => (
  new main.EnrichedMessage({
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
  new main.MessageNode({
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
    mockGetMessages.mockReset();
    mockGetMessages.mockResolvedValue([]);
    mockPlayChatReceiveSoundIfActive.mockClear();
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
    expect(mockPlayChatReceiveSoundIfActive).toHaveBeenCalledWith('conversation-1');
  });

  it('reiniciar a mesma conversa cancela o controller anterior', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', initialUserContent: 'primeira', adapter });
    startChatEventController({ conversationId: 'conversation-1', initialUserContent: 'segunda', adapter });

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'resposta',
      done: false,
    });

    expect(sessions['conversation-1'].conversation?.threadedMessages).toHaveLength(1);
    expect(sessions['conversation-1'].conversation?.threadedMessages[0].message.id).toContain('streaming-conversation-1');
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
    expect(mockAnnounceChatBackgroundResponseDone).toHaveBeenCalledWith('conversation-1', 'Conversa conversation-1');
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

  it('descarta update de streaming pendente ao limpar controller', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    const handle = startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'conteúdo atrasado',
      done: false,
    });
    handle.cleanup();
    vi.runOnlyPendingTimers();

    const assistantMessage = sessions['conversation-1'].conversation?.threadedMessages[0]?.message;
    expect(assistantMessage?.content).toBe('');
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

  it('recarrega mensagens do backend ao finalizar resposta com tool calls', async () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);
    mockGetMessages.mockResolvedValue([
      createNode(createMessage('backend-user', 'user', 'pergunta')),
      createNode(createMessage('backend-assistant', 'assistant', 'resposta com ferramenta')),
    ]);

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

    expect(mockGetMessages).toHaveBeenCalledWith('conversation-1', null);
    expect(sessions['conversation-1'].conversation?.threadedMessages.map((node) => node.message.id)).toEqual([
      'backend-user',
      'backend-assistant',
    ]);
  });
});
