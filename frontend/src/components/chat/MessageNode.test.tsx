import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MessageNode } from './MessageNode';
import { main } from '../../../wailsjs/go/models';

const chatMessageSpy = vi.fn();

vi.mock('./ChatMessage', () => ({
  ChatMessage: (props: { hasThreadIndicator?: boolean }) => {
    chatMessageSpy(props);
    return <div data-testid="chat-message" />;
  },
}));

type ChatStoreState = {
  toggleThreadExpanded: () => void;
  editingMessageId: string | null;
  setEditingMessageId: (id: string | null) => void;
  readingMessageId: string | null;
  setReadingMessageId: (id: string | null) => void;
  streamingMessageId: string | null;
  streamingReasoning: string;
  isThinking: boolean;
  toggleReasoningExpanded: () => void;
  activeToolCalls: Array<unknown>;
  completedSegments: Array<unknown>;
  expandedThreads: Set<string>;
  expandedReasonings: Set<string>;
};

vi.mock('../../store/chatStore', () => ({
  useChatStore: (selector: (state: ChatStoreState) => unknown) => selector({
    toggleThreadExpanded: vi.fn(),
    editingMessageId: null,
    setEditingMessageId: vi.fn(),
    readingMessageId: null,
    setReadingMessageId: vi.fn(),
    streamingMessageId: null,
    streamingReasoning: '',
    isThinking: false,
    toggleReasoningExpanded: vi.fn(),
    activeToolCalls: [],
    completedSegments: [],
    expandedThreads: new Set<string>(),
    expandedReasonings: new Set<string>(),
  }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  announce: vi.fn(),
}));

vi.mock('../../hooks/useVirtualModal', () => ({
  useVirtualModal: () => {},
}));

vi.mock('../../services/messageAudio', () => ({
  messageAudioService: { stopCurrentAudio: vi.fn() },
}));

vi.mock('@wailsjs/go/app/App', () => ({
  UpdateMessage: vi.fn(),
}));

vi.mock('../../utils/errorHandler', () => ({
  handleError: vi.fn(),
  ErrorSeverity: { RECOVERABLE: 'recoverable' },
}));

describe('MessageNode', () => {
  it('renderiza container e passa indicador de thread', () => {
    render(
      <MessageNode
        node={main.MessageNode.createFrom({
          message: new main.EnrichedMessage({
            id: '1',
            conversationId: 1,
            role: 'user',
            content: 'Oi',
            createdAt: new Date().toISOString(),
            timestamp: Date.now(),
            isStreaming: false,
            internal: false,
          }),
          childCount: 1,
          level: 0,
          children: [],
        })}
      />
    );

    expect(screen.getByRole('listitem')).toHaveAttribute('data-message-id', '1');
    expect(screen.getByTestId('chat-message')).toBeInTheDocument();
    expect(chatMessageSpy).toHaveBeenCalledWith(expect.objectContaining({ hasThreadIndicator: true }));
  });
});
