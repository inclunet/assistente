import { describe, expect, it } from 'vitest';
import {
  createEmptyChatSession,
  getChatSurfaceSessionsForConversation,
  getChatSession,
  getConversationTimeline,
  normalizeChatSurfaceOrigin,
  patchChatConversation,
  patchChatSession,
  removeChatSession,
  type ActiveConversation,
  type ChatSessionRegistryState,
} from './chatSessionRegistry';

const conversation = (id: string): ActiveConversation => ({
  id,
  title: `Conversa ${id}`,
  threadedMessages: [],
});

describe('chatSessionRegistry', () => {
  it('retorna sessão vazia para conversa ausente', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {},
      timelinesByConversationId: {},
      surfaceSessionsByKey: {},
    };

    expect(getChatSession(state, 'missing')).toMatchObject({
      conversation: null,
      isLoading: false,
      activeToolCalls: [],
      completedSegments: [],
    });
  });

  it('normaliza origin recalculando sessionKey quando a conversa muda', () => {
    const origin = {
      sessionKey: 'tab-a:old-conversation',
      conversationId: 'old-conversation',
      surfaceId: 'tab-a',
      surfaceType: 'page' as const,
    };

    expect(normalizeChatSurfaceOrigin(origin, 'new-conversation')).toMatchObject({
      conversationId: 'new-conversation',
      sessionKey: 'tab-a:new-conversation',
    });
  });

  it('normaliza origin preservando sessionKey quando a conversa já corresponde', () => {
    const origin = {
      sessionKey: 'tab-a:conversation-1',
      conversationId: 'conversation-1',
      surfaceId: 'tab-a',
      surfaceType: 'page' as const,
    };

    expect(normalizeChatSurfaceOrigin(origin, 'conversation-1')).toMatchObject({
      conversationId: 'conversation-1',
      sessionKey: 'tab-a:conversation-1',
    });
  });

  it('aplica patch criando sessão quando necessário', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {},
      timelinesByConversationId: {},
      surfaceSessionsByKey: {},
    };

    const next = patchChatSession(state, 'conversation-1', { isLoading: true });

    expect(next.sessionsByConversationId['conversation-1']).toMatchObject({
      isLoading: true,
      conversation: null,
    });
    expect(next.surfaceSessionsByKey?.['conversation:conversation-1']).toMatchObject({
      isLoading: true,
      conversationId: 'conversation-1',
    });
    expect(state.sessionsByConversationId['conversation-1']).toBeUndefined();
  });

  it('aplica updater na conversa quando ela existe', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {
        'conversation-1': {
          ...createEmptyChatSession('conversation-1'),
          conversation: conversation('conversation-1'),
        },
      },
      timelinesByConversationId: {},
      surfaceSessionsByKey: {},
    };

    const next = patchChatConversation(state, 'conversation-1', (current) => ({
      ...current,
      title: 'Renomeada',
    }));

    expect(next.sessionsByConversationId?.['conversation-1'].conversation?.title).toBe('Renomeada');
    expect(next.timelinesByConversationId?.['conversation-1'].title).toBe('Renomeada');
  });

  it('remove sessão sem mutar o estado original', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {
        'conversation-1': {
          ...createEmptyChatSession('conversation-1'),
          conversation: conversation('conversation-1'),
        },
      },
      timelinesByConversationId: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {
        'conversation:conversation-1': createEmptyChatSession('conversation-1'),
      },
    };

    const next = removeChatSession(state, 'conversation-1');

    expect(next.sessionsByConversationId['conversation-1']).toBeUndefined();
    expect(next.timelinesByConversationId?.['conversation-1']).toBeUndefined();
    expect(next.surfaceSessionsByKey?.['conversation:conversation-1']).toBeUndefined();
    expect(state.sessionsByConversationId['conversation-1']).toBeDefined();
  });

  it('compõe timeline compartilhada com sessão visual por chave', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {},
      timelinesByConversationId: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {
        'tab-a:conversation-1': {
          ...createEmptyChatSession('conversation-1', 'tab-a:conversation-1'),
          expandedThreads: new Set(['message-1']),
        },
        'tab-b:conversation-1': {
          ...createEmptyChatSession('conversation-1', 'tab-b:conversation-1'),
          expandedThreads: new Set(['message-2']),
        },
      },
    };

    expect(getChatSession(state, 'conversation-1', 'tab-a:conversation-1').conversation?.title).toBe('Conversa conversation-1');
    expect(getChatSession(state, 'conversation-1', 'tab-a:conversation-1').expandedThreads.has('message-1')).toBe(true);
    expect(getChatSession(state, 'conversation-1', 'tab-b:conversation-1').expandedThreads.has('message-1')).toBe(false);
    expect(getChatSession(state, 'conversation-1', 'tab-b:conversation-1').expandedThreads.has('message-2')).toBe(true);
  });

  it('não herda estado visual de compatibilidade em sessionKey não padrão', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {
        'conversation-1': {
          ...createEmptyChatSession('conversation-1'),
          conversation: conversation('conversation-1'),
          expandedThreads: new Set(['message-from-default']),
          editingMessageId: 'editing-default',
        },
      },
      timelinesByConversationId: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {},
    };

    const defaultSession = getChatSession(state, 'conversation-1');
    const surfaceSession = getChatSession(state, 'conversation-1', 'tab-a:conversation-1');

    expect(defaultSession.expandedThreads.has('message-from-default')).toBe(true);
    expect(surfaceSession.expandedThreads.has('message-from-default')).toBe(false);
    expect(surfaceSession.editingMessageId).toBeNull();
    expect(surfaceSession.conversation?.title).toBe('Conversa conversation-1');
  });

  it('não grava estado visual de sessionKey não padrão em sessionsByConversationId', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {
        'conversation-1': {
          ...createEmptyChatSession('conversation-1'),
          conversation: conversation('conversation-1'),
        },
      },
      timelinesByConversationId: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {},
    };

    const next = {
      ...state,
      ...patchChatSession(state, 'conversation-1', {
        editingMessageId: 'message-from-tab',
        expandedThreads: new Set(['message-from-tab']),
      }, 'tab-a:conversation-1'),
    };

    expect(next.sessionsByConversationId['conversation-1'].editingMessageId).toBeNull();
    expect(next.sessionsByConversationId['conversation-1'].expandedThreads.has('message-from-tab')).toBe(false);
    expect(getChatSession(next, 'conversation-1', 'tab-a:conversation-1').editingMessageId).toBe('message-from-tab');
    expect(getConversationTimeline(next, 'conversation-1')?.title).toBe('Conversa conversation-1');
  });

  it('atualiza timeline uma vez sem sobrescrever sessões visuais interessadas', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {},
      timelinesByConversationId: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {
        'tab-a:conversation-1': {
          ...createEmptyChatSession('conversation-1', 'tab-a:conversation-1'),
          expandedThreads: new Set(['message-1']),
        },
        'tab-b:conversation-1': {
          ...createEmptyChatSession('conversation-1', 'tab-b:conversation-1'),
          expandedThreads: new Set(['message-2']),
        },
      },
    };

    const next = {
      ...state,
      ...patchChatConversation(state, 'conversation-1', (current) => ({
        ...current,
        title: 'Timeline atualizada',
      })),
    };

    expect(getConversationTimeline(next, 'conversation-1')?.title).toBe('Timeline atualizada');
    expect(getChatSurfaceSessionsForConversation(next, 'conversation-1')).toHaveLength(2);
    expect(getChatSession(next, 'conversation-1', 'tab-a:conversation-1').expandedThreads.has('message-1')).toBe(true);
    expect(getChatSession(next, 'conversation-1', 'tab-b:conversation-1').expandedThreads.has('message-2')).toBe(true);
    expect(getChatSession(next, 'conversation-1', 'tab-a:conversation-1').conversation?.title).toBe('Timeline atualizada');
    expect(getChatSession(next, 'conversation-1', 'tab-b:conversation-1').conversation?.title).toBe('Timeline atualizada');
  });

  it('não apaga timeline ao aplicar patch visual sem conversation', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {},
      timelinesByConversationId: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {},
    };

    const next = {
      ...state,
      ...patchChatSession(state, 'conversation-1', { isThinking: true }, 'tab-a:conversation-1'),
    };

    expect(getConversationTimeline(next, 'conversation-1')?.title).toBe('Conversa conversation-1');
    expect(getChatSession(next, 'conversation-1', 'tab-a:conversation-1').isThinking).toBe(true);
    expect(getChatSession(next, 'conversation-1', 'tab-a:conversation-1').conversation?.title).toBe('Conversa conversation-1');
  });

  it('reusa referência da timeline em patch visual', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {},
      timelinesByConversationId: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {},
    };

    const patch = patchChatSession(state, 'conversation-1', { isThinking: true }, 'tab-a:conversation-1');

    expect(patch.timelinesByConversationId).toBe(state.timelinesByConversationId);
    expect(getChatSession({ ...state, ...patch }, 'conversation-1', 'tab-a:conversation-1').isThinking).toBe(true);
  });
});
