import { describe, expect, it } from 'vitest';
import {
  createEmptyChatSession,
  getChatSession,
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
    const state: ChatSessionRegistryState = { sessionsByConversationId: {} };

    expect(getChatSession(state, 'missing')).toMatchObject({
      conversation: null,
      isLoading: false,
      activeToolCalls: [],
      completedSegments: [],
    });
  });

  it('aplica patch criando sessão quando necessário', () => {
    const state: ChatSessionRegistryState = { sessionsByConversationId: {} };

    const next = patchChatSession(state, 'conversation-1', { isLoading: true });

    expect(next.sessionsByConversationId['conversation-1']).toMatchObject({
      isLoading: true,
      conversation: null,
    });
    expect(state.sessionsByConversationId['conversation-1']).toBeUndefined();
  });

  it('aplica updater na conversa quando ela existe', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {
        'conversation-1': {
          ...createEmptyChatSession(),
          conversation: conversation('conversation-1'),
        },
      },
    };

    const next = patchChatConversation(state, 'conversation-1', (current) => ({
      ...current,
      title: 'Renomeada',
    }));

    expect(next.sessionsByConversationId?.['conversation-1'].conversation?.title).toBe('Renomeada');
  });

  it('remove sessão sem mutar o estado original', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {
        'conversation-1': {
          ...createEmptyChatSession(),
          conversation: conversation('conversation-1'),
        },
      },
    };

    const next = removeChatSession(state, 'conversation-1');

    expect(next.sessionsByConversationId['conversation-1']).toBeUndefined();
    expect(state.sessionsByConversationId['conversation-1']).toBeDefined();
  });
});
