import { act, cleanup, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { chat } from '../../../wailsjs/go/models';
import { useChatStore, type Message, type MessageNode } from '../../store/chatStore';
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
  onRender();
  const { liveContent } = useChatMessageLiveState(message);
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
});
