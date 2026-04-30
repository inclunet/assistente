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
  sessionsByConversationId: Record<string, unknown>;
  toggleThreadExpanded: () => void;
  toggleConversationThreadExpanded: () => void;
  editingMessageId: string | null;
  setEditingMessageId: (id: string | null) => void;
  setConversationEditingMessageId: (conversationId: string, id: string | null) => void;
  readingMessageId: string | null;
  setReadingMessageId: (id: string | null) => void;
  setConversationReadingMessageId: (conversationId: string, id: string | null) => void;
  streamingMessageId: string | null;
  streamingReasoning: string;
  isThinking: boolean;
  toggleReasoningExpanded: () => void;
  toggleConversationReasoningExpanded: () => void;
  activeToolCalls: Array<unknown>;
  completedSegments: Array<unknown>;
  expandedThreads: Set<string>;
  expandedReasonings: Set<string>;
};

vi.mock('../../store/chatStore', () => ({
  useChatStore: (selector: (state: ChatStoreState) => unknown) => selector({
    sessionsByConversationId: {},
    toggleThreadExpanded: vi.fn(),
    toggleConversationThreadExpanded: vi.fn(),
    editingMessageId: null,
    setEditingMessageId: vi.fn(),
    setConversationEditingMessageId: vi.fn(),
    readingMessageId: null,
    setReadingMessageId: vi.fn(),
    setConversationReadingMessageId: vi.fn(),
    streamingMessageId: null,
    streamingReasoning: '',
    isThinking: false,
    toggleReasoningExpanded: vi.fn(),
    toggleConversationReasoningExpanded: vi.fn(),
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
            conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
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
