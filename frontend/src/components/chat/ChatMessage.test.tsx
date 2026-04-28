import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { main } from '../../../wailsjs/go/models';
import { ChatMessage } from './ChatMessage';

const subscribeSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('../../store/chatStore', () => {
  const useChatStore = () => ({});
  useChatStore.subscribe = (cb: (state: unknown) => void) => {
    subscribeSpy(cb);
    return () => {};
  };
  useChatStore.getState = () => ({
    streamingMessageId: null,
    completedSegments: [],
    activeToolCalls: [],
    tabs: [],
  });
  return { useChatStore };
});

vi.mock('../../lib/dateUtils', () => ({
  formatRelativeTime: () => 'agora',
}));

vi.mock('../../lib/chatUtils', () => ({
  isAgentMessage: () => false,
}));

vi.mock('../../lib/chatMessageAriaLabel', () => ({
  buildChatMessageAriaLabel: () => 'aria-label',
}));

vi.mock('../ui/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => <div>{content}</div>,
}));

vi.mock('./ThreadIndicator', () => ({
  ThreadIndicator: () => <div data-testid="thread" />,
}));

vi.mock('./ReasoningSection', () => ({
  ReasoningSection: () => <div data-testid="reasoning" />,
}));

vi.mock('./ToolCallsSection', () => ({
  ToolCallsSection: () => <div data-testid="toolcalls" />,
}));

describe('ChatMessage', () => {
  it('renderiza conteudo e botao de audio', () => {
    const onSpeak = vi.fn();
    const message = new main.EnrichedMessage({
      id: '1',
      conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
      role: 'user',
      content: 'Ola',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
    });

    render(
      <ChatMessage
        message={message}
        onSpeak={onSpeak}
      />
    );

    expect(screen.getByText('Ola')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'chat.playAudio' })).toBeInTheDocument();
  });

  it('renderiza modo de edicao', () => {
    const message = new main.EnrichedMessage({
      id: '1',
      conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
      role: 'user',
      content: 'Ola',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
    });
    render(
      <ChatMessage
        message={message}
        isEditing
        editContent=""
      />
    );

    const textarea = screen.getByRole('textbox', { name: 'chat.editMessage' });
    expect(textarea).toBeInTheDocument();
    const saveButton = screen.getByRole('button', { name: 'common.save' });
    expect(saveButton).toBeDisabled();
  });
});
