import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import { main } from '../../../wailsjs/go/models';
import { ChatMessage } from './ChatMessage';

const subscribeSpy = vi.fn();
const conversationId = '01926b90-7a5a-7c4e-8d3f-000000000001';
const originalIntersectionObserver = globalThis.IntersectionObserver;

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
    sessionsByConversationId: {
      [conversationId]: {
        streamingMessageId: null,
        completedSegments: [],
        activeToolCalls: [],
        conversation: {
          id: conversationId,
          title: 'Conversa',
          threadedMessages: [],
        },
      },
    },
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
  afterEach(() => {
    vi.unstubAllGlobals();
    if (originalIntersectionObserver) {
      vi.stubGlobal('IntersectionObserver', originalIntersectionObserver);
    }
  });

  it('renderiza conteudo e botao de audio', () => {
    const onSpeak = vi.fn();
    const message = new main.EnrichedMessage({
      id: '1',
      conversationId,
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
      conversationId,
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

  it('adia renderizacao de markdown grande ate entrar na area visivel', async () => {
    let observerCallback: IntersectionObserverCallback | null = null;
    class MockIntersectionObserver {
      constructor(callback: IntersectionObserverCallback) {
        observerCallback = callback;
      }
      observe = vi.fn();
      disconnect = vi.fn();
      unobserve = vi.fn();
      takeRecords = vi.fn(() => []);
      root = null;
      rootMargin = '';
      thresholds = [];
    }
    vi.stubGlobal('IntersectionObserver', MockIntersectionObserver);

    const largeContent = `inicio-${'a'.repeat(8_100)}-fim`;
    const message = new main.EnrichedMessage({
      id: '1',
      conversationId,
      role: 'assistant',
      content: largeContent,
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
    });

    const { container } = render(<ChatMessage message={message} />);

    expect(screen.getByText('chat.largeMessageDeferred')).toBeInTheDocument();
    expect(screen.queryByText(largeContent)).not.toBeInTheDocument();

    await act(async () => {
      observerCallback?.([
        { isIntersecting: true } as IntersectionObserverEntry,
      ], {} as IntersectionObserver);
    });

    expect(container).toHaveTextContent(largeContent);
  });
});
