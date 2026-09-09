import { act, cleanup, render, screen } from '@testing-library/react';
import { useEffect } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { chat } from '../../../wailsjs/go/models';
import { useChatStore, type Message, type MessageNode } from '../../store/chatStore';
import { createEmptyChatSurfaceSession } from '../../services/chatSessionRegistry';
import { useChatMessageLiveState } from './ChatSessionContext';

const createMessage = (id: string, conversationId: string, content: string): Message => (
  new chat.EnrichedMessage({
    id,
    conversationId,
    role: 'assistant',
    content,
    isStreaming: true,
    internal: false,
    createdAt: '2026-09-09T00:00:00.000Z',
  }) as Message
);

const createNode = (message: Message): MessageNode => (
  new chat.MessageNode({ message, children: [], childCount: 0, level: 0 }) as MessageNode
);

function LiveProbe({ message, onRender }: { message: Message; onRender: () => void }) {
  const { liveContent } = useChatMessageLiveState(message);
  useEffect(() => {
    onRender();
  });
  return <span>{liveContent ?? message.content}</span>;
}

describe('estado live granular do chat', () => {
  beforeEach(() => {
    useChatStore.setState({
      sessionsByConversationId: {},
      timelinesByConversationId: {},
      surfaceSessionsByKey: {},
      liveMessageContentByConversationId: {},
    });
  });

  afterEach(() => {
    cleanup();
    useChatStore.setState({ liveMessageContentByConversationId: {} });
  });

  it('rerenderiza somente a mensagem e conversa afetadas com seletor estável', () => {
    const renderOne = vi.fn();
    const renderTwo = vi.fn();
    const messageOne = createMessage('assistant-1', 'conversation-1', 'inicial 1');
    const messageTwo = createMessage('assistant-2', 'conversation-2', 'inicial 2');

    render(
      <>
        <LiveProbe message={messageOne} onRender={renderOne} />
        <LiveProbe message={messageTwo} onRender={renderTwo} />
      </>,
    );

    act(() => {
      useChatStore.getState().setConversationLiveMessageContent('conversation-1', 'assistant-1', 'delta acumulado 👩🏽‍💻');
    });
    expect(screen.getByText('delta acumulado 👩🏽‍💻')).toBeInTheDocument();
    expect(renderOne).toHaveBeenCalledTimes(2);
    expect(renderTwo).toHaveBeenCalledTimes(1);

    act(() => {
      useChatStore.getState().setConversationLiveMessageContent('conversation-1', 'assistant-1', 'delta acumulado 👩🏽‍💻');
    });
    expect(renderOne).toHaveBeenCalledTimes(2);
    expect(renderTwo).toHaveBeenCalledTimes(1);
  });

  it('não altera identidades da timeline ao publicar frames live', () => {
    const message = createMessage('assistant-1', 'conversation-1', 'inicial');
    const untouched = createNode(createMessage('assistant-2', 'conversation-1', 'intocada'));
    const nodes = [createNode(message), untouched];
    const timeline = {
      id: 'conversation-1',
      title: 'Conversa',
      threadedMessages: nodes,
    };
    useChatStore.setState({
      timelinesByConversationId: { 'conversation-1': timeline },
    });

    act(() => {
      useChatStore.getState().setConversationLiveMessageContent('conversation-1', 'assistant-1', 'novo frame');
    });

    const currentTimeline = useChatStore.getState().timelinesByConversationId['conversation-1'];
    expect(currentTimeline).toBe(timeline);
    expect(currentTimeline.threadedMessages).toBe(nodes);
    expect(currentTimeline.threadedMessages[1]).toBe(untouched);
  });

  it('preserva o snapshot Zustand quando o commit não tem timeline nem estado live', () => {
    const before = useChatStore.getState();

    act(() => {
      useChatStore.getState().commitConversationLiveMessage('conversation-missing', 'message-missing', '');
    });

    expect(useChatStore.getState()).toBe(before);
  });

  it('consolida no terminal o nó transitório visível que ainda não entrou na timeline', () => {
    const conversationId = 'conversation-1';
    const sessionKey = 'tab-1:conversation-1';
    const userNode = createNode(createMessage('user-1', conversationId, 'pergunta'));
    userNode.message.role = 'user';
    userNode.message.isStreaming = false;
    const assistantNode = createNode(createMessage('assistant-1', conversationId, ''));
    useChatStore.setState({
      timelinesByConversationId: {
        [conversationId]: {
          id: conversationId,
          title: 'Conversa',
          threadedMessages: [userNode],
        },
      },
      surfaceSessionsByKey: {
        [sessionKey]: {
          ...createEmptyChatSurfaceSession(conversationId, sessionKey),
          visibleThreadedMessages: [userNode, assistantNode],
          streamingMessageId: 'assistant-1',
        },
      },
      liveMessageContentByConversationId: {
        [conversationId]: { 'assistant-1': 'resposta terminal' },
      },
    });

    act(() => {
      useChatStore.getState().commitConversationLiveMessage(
        conversationId,
        'assistant-1',
        'resposta terminal',
      );
    });

    const state = useChatStore.getState();
    expect(state.surfaceSessionsByKey[sessionKey].visibleThreadedMessages?.[1].message.content)
      .toBe('resposta terminal');
    expect(state.liveMessageContentByConversationId[conversationId]).toBeUndefined();
  });
});
