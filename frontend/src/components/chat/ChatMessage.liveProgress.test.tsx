import { act, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { chat } from '../../../wailsjs/go/models';
import type { ToolCallStatus } from '../../types/chat';
import type { Message, TurnSegment } from '../../store/chatStore';
import { useChatStore } from '../../store/chatStore';
import {
  createChatSurfaceIdentity,
  createEmptyChatSurfaceSession,
  type ChatSurfaceIdentity,
} from '../../services/chatSessionRegistry';
import { ChatSessionProvider, useChatNodeSessionState } from './ChatSessionContext';
import { ChatMessage } from './ChatMessage';

// O renderer é um detalhe pesado da mensagem. Mantemos o DOM textual real de
// ChatMessage e do bloco de tools, sem substituir store, contexto ou estado live.
vi.mock('../ui/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => (
    <div data-testid="markdown">{content}</div>
  ),
}));

const conversationId = '01926b90-7a5a-7c4e-8d3f-000000000001';
const secondConversationId = '01926b90-7a5a-7c4e-8d3f-000000000002';
const surfaceId = 'embedded:live-progress-test';

const surfaceFor = (targetConversationId: string): ChatSurfaceIdentity => (
  createChatSurfaceIdentity({
    conversationId: targetConversationId,
    surfaceId,
    surfaceType: 'embedded',
  })
);

const invocation = (callId: string, name: string, status = 'succeeded') => ({
  invocationId: `inv-${callId}`,
  callId,
  name,
  status,
  hasDetails: false,
  resultAvailability: 'available',
});

const messageFor = (
  id: string,
  targetConversationId: string,
  overrides: Partial<ConstructorParameters<typeof chat.EnrichedMessage>[0]> = {},
): Message => new chat.EnrichedMessage({
  id,
  conversationId: targetConversationId,
  role: 'assistant',
  content: '',
  createdAt: '2026-09-17T00:00:00.000Z',
  timestamp: Date.parse('2026-09-17T00:00:00.000Z'),
  isStreaming: true,
  internal: false,
  ...overrides,
}) as Message;

const seedStore = ({
  targetConversationId,
  messageId,
  streamingMessageId = messageId,
  completedSegments = [],
  activeToolCalls = [],
  liveContent,
}: {
  targetConversationId: string;
  messageId: string;
  streamingMessageId?: string | null;
  completedSegments?: TurnSegment[];
  activeToolCalls?: ToolCallStatus[];
  liveContent?: string;
}) => {
  const surface = surfaceFor(targetConversationId);
  const emptySurface = createEmptyChatSurfaceSession(
    targetConversationId,
    surface.sessionKey,
  );

  useChatStore.setState({
    sessionsByConversationId: {},
    timelinesByConversationId: {
      [targetConversationId]: {
        id: targetConversationId,
        title: 'Conversa de teste',
        threadedMessages: [],
      },
    },
    surfaceSessionsByKey: {
      [surface.sessionKey]: {
        ...emptySurface,
        streamingMessageId,
        completedSegments,
        activeToolCalls,
      },
    },
    liveMessageContentByConversationId: liveContent === undefined
      ? {}
      : { [targetConversationId]: { [messageId]: liveContent } },
  });

  return surface;
};

const renderMessage = (surface: ChatSurfaceIdentity, message: Message) => render(
  <ChatSessionProvider surface={surface}>
    <SessionMessage key={`${surface.sessionKey}:${message.id}`} message={message} />
  </ChatSessionProvider>,
);

// Mantém a mesma fronteira de dados do MessageNode de produção: o contexto
// resolve o estado por sessão e só então os props transitórios chegam ao
// ChatMessage. O hook live continua responsável apenas pelo conteúdo textual.
function SessionMessage({ message }: { message: Message }) {
  const {
    streamingMessageId,
    streamingReasoning,
    isThinking,
    activeToolCalls,
    completedSegments,
    reasoningExpanded,
  } = useChatNodeSessionState(message.id);
  const isCurrentStreamingMessage = message.id === streamingMessageId;

  return (
    <ChatMessage
      message={message}
      streamingReasoning={isCurrentStreamingMessage ? (streamingReasoning || undefined) : undefined}
      isThinking={isCurrentStreamingMessage ? isThinking : false}
      isReasoningExpanded={reasoningExpanded}
      activeToolCalls={isCurrentStreamingMessage ? activeToolCalls : undefined}
      completedSegments={isCurrentStreamingMessage ? completedSegments : undefined}
    />
  );
}

describe('progresso live de ChatMessage', () => {
  beforeEach(() => {
    useChatStore.setState({
      sessionsByConversationId: {},
      timelinesByConversationId: {},
      surfaceSessionsByKey: {},
      liveMessageContentByConversationId: {},
    });
  });

  it('não deixa turnSegments persistidos esconderem segmentos concluídos e texto vivo', () => {
    const messageId = 'assistant-persisted-and-live';
    const message = messageFor(messageId, conversationId, {
      content: '',
      isStreaming: true,
      turnSegments: [
        { type: 'text', content: 'texto persistido antigo' },
        {
          type: 'tool_calls',
          toolInvocations: [invocation('persisted-tool', 'ferramenta-antiga')],
        },
      ],
    });
    const surface = seedStore({
      targetConversationId: conversationId,
      messageId,
      completedSegments: [{ type: 'text', content: 'segmento concluído atual' }],
      liveContent: 'texto vivo atual',
    });

    renderMessage(surface, message);

    expect(screen.getByText('segmento concluído atual')).toBeInTheDocument();
    expect(screen.getByText('texto vivo atual')).toBeInTheDocument();
    expect(screen.queryByText('texto persistido antigo')).not.toBeInTheDocument();
    expect(screen.queryByText('ferramenta-antiga')).not.toBeInTheDocument();
  });

  it('respeita completedSegments vazio definido e não reativa o histórico persistido', () => {
    const messageId = 'assistant-empty-live-segments';
    const message = messageFor(messageId, conversationId, {
      content: '',
      isStreaming: true,
      turnSegments: [{ type: 'text', content: 'histórico que não é o frame atual' }],
    });
    const surface = seedStore({
      targetConversationId: conversationId,
      messageId,
      completedSegments: [],
      liveContent: 'texto vivo sem segmentos concluídos',
    });

    renderMessage(surface, message);

    expect(screen.getByText('texto vivo sem segmentos concluídos')).toBeInTheDocument();
    expect(screen.queryByText('histórico que não é o frame atual')).not.toBeInTheDocument();
  });

  it('mantém texto live visível quando o snapshot persistido contém text_edit', () => {
    const messageId = 'assistant-text-edit-then-stream';
    const message = messageFor(messageId, conversationId, {
      content: '',
      isStreaming: true,
      turnSegments: [{
        type: 'tool_calls',
        toolInvocations: [invocation('persisted-text-edit', 'text_edit')],
      }],
    });
    const surface = seedStore({
      targetConversationId: conversationId,
      messageId,
      completedSegments: [],
      liveContent: 'novo texto depois do edit',
    });

    renderMessage(surface, message);

    expect(screen.getByText('novo texto depois do edit')).toBeInTheDocument();
  });

  it('mostra a ferramenta pendente durante execução sem emitir placeholder terminal', () => {
    const messageId = 'assistant-tool-only-running';
    const message = messageFor(messageId, conversationId, {
      content: '',
      isStreaming: true,
    });
    const surface = seedStore({
      targetConversationId: conversationId,
      messageId,
      completedSegments: [],
      activeToolCalls: [{
        name: 'buscar-documentos',
        callId: 'running-tool',
        args: '{}',
        status: 'running',
      }],
    });

    const { rerender } = renderMessage(surface, message);

    expect(screen.getByRole('listitem')).toHaveTextContent('chat.toolStatusRunning');
    expect(screen.queryByText('chat.toolOnlyTurnPlaceholder')).not.toBeInTheDocument();
    expect(screen.queryByText('chat.waitingForNextStep')).not.toBeInTheDocument();

    const finalMessage = messageFor(messageId, conversationId, {
      content: '',
      isStreaming: false,
      turnSegments: [{
        type: 'tool_calls',
        toolInvocations: [invocation('running-tool', 'buscar-documentos')],
      }],
    });
    act(() => {
      useChatStore.setState((state) => ({
        surfaceSessionsByKey: {
          ...state.surfaceSessionsByKey,
          [surface.sessionKey]: {
            ...state.surfaceSessionsByKey[surface.sessionKey],
            streamingMessageId: null,
            activeToolCalls: [],
            completedSegments: [],
          },
        },
        liveMessageContentByConversationId: {},
      }));
    });
    rerender(
      <ChatSessionProvider surface={surface}>
        <SessionMessage message={finalMessage} />
      </ChatSessionProvider>,
    );

    expect(screen.getByText('chat.toolOnlyTurnPlaceholder')).toBeInTheDocument();
    expect(screen.getByRole('listitem')).toHaveTextContent('chat.toolStatusSucceeded');
  });

  it('renderiza duas rodadas, texto corrente antes das tools correntes e espera sem tool running', () => {
    const messageId = 'assistant-two-rounds';
    const message = messageFor(messageId, conversationId, {
      content: '',
      isStreaming: true,
    });
    const completedSegments: TurnSegment[] = [
      { type: 'text', content: 'texto da primeira rodada' },
      {
        type: 'tool_calls',
        toolCalls: [{
          id: 'first-round-tool',
          type: 'function',
          function: { name: 'buscar-inicial', arguments: '{}' },
        }],
      },
    ];
    const surface = seedStore({
      targetConversationId: conversationId,
      messageId,
      completedSegments,
      liveContent: 'texto da segunda rodada',
      activeToolCalls: [{
        name: 'buscar-corrente',
        callId: 'current-round-tool',
        status: 'running',
      }],
    });

    renderMessage(surface, message);

    const currentText = screen.getByText('texto da segunda rodada');
    const currentTool = screen.getAllByRole('listitem')[1];
    expect(screen.getByText('texto da primeira rodada')).toBeInTheDocument();
    expect(currentText).toBeInTheDocument();
    expect(currentTool).toBeInTheDocument();
    expect(currentText.compareDocumentPosition(currentTool) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

  });

  it('mostra waitingForNextStep quando a rodada aguarda o próximo passo sem tool running', () => {
    const messageId = 'assistant-waiting-next-step';
    const message = messageFor(messageId, conversationId, {
      content: '',
      isStreaming: true,
    });
    const surface = seedStore({
      targetConversationId: conversationId,
      messageId,
      completedSegments: [
        { type: 'text', content: 'texto da rodada anterior' },
        {
          type: 'tool_calls',
          toolCalls: [{
            id: 'completed-tool',
            type: 'function',
            function: { name: 'ferramenta-concluída', arguments: '{}' },
          }],
        },
      ],
    });

    renderMessage(surface, message);

    expect(screen.getByText('texto da rodada anterior')).toBeInTheDocument();
    expect(screen.getByText('chat.waitingForNextStep')).toBeInTheDocument();
  });

  it('volta ao estado canônico depois do fim do turno e limpa o texto transitório', () => {
    const messageId = 'assistant-canonical-after-finish';
    const streamingMessage = messageFor(messageId, conversationId, {
      content: '',
      isStreaming: true,
    });
    const surface = seedStore({
      targetConversationId: conversationId,
      messageId,
      completedSegments: [{ type: 'text', content: 'intermediário ao vivo' }],
      liveContent: 'texto corrente',
    });
    const { rerender } = renderMessage(surface, streamingMessage);

    expect(screen.getByText('intermediário ao vivo')).toBeInTheDocument();
    expect(screen.getByText('texto corrente')).toBeInTheDocument();

    const finalMessage = messageFor(messageId, conversationId, {
      content: 'resposta canônica',
      isStreaming: false,
      turnSegments: [
        { type: 'text', content: 'intermediário ao vivo' },
        {
          type: 'tool_calls',
          toolInvocations: [invocation('final-tool', 'ferramenta-final')],
        },
        { type: 'text', content: 'resposta canônica' },
      ],
    });

    act(() => {
      useChatStore.setState((state) => ({
        surfaceSessionsByKey: {
          ...state.surfaceSessionsByKey,
          [surface.sessionKey]: {
            ...state.surfaceSessionsByKey[surface.sessionKey],
            streamingMessageId: null,
            completedSegments: [],
            activeToolCalls: [],
          },
        },
        liveMessageContentByConversationId: {},
      }));
    });
    rerender(
      <ChatSessionProvider surface={surface}>
        <SessionMessage message={finalMessage} />
      </ChatSessionProvider>,
    );

    expect(screen.getByText('intermediário ao vivo')).toBeInTheDocument();
    expect(screen.getByText('resposta canônica')).toBeInTheDocument();
    expect(screen.getByRole('listitem')).toHaveTextContent('chat.toolStatusSucceeded');
    expect(screen.queryByText('texto corrente')).not.toBeInTheDocument();
    expect(useChatStore.getState().liveMessageContentByConversationId).toEqual({});
    expect(useChatStore.getState().surfaceSessionsByKey[surface.sessionKey]).toMatchObject({
      streamingMessageId: null,
      completedSegments: [],
      activeToolCalls: [],
    });
  });

  it('troca de conversa usa a sessão live correta no store real', () => {
    const firstMessageId = 'assistant-conversation-one';
    const secondMessageId = 'assistant-conversation-two';
    const firstSurface = surfaceFor(conversationId);
    const secondSurface = surfaceFor(secondConversationId);
    const firstMessage = messageFor(firstMessageId, conversationId, { content: '', isStreaming: true });
    const secondMessage = messageFor(secondMessageId, secondConversationId, { content: '', isStreaming: true });
    const firstSession = createEmptyChatSurfaceSession(conversationId, firstSurface.sessionKey);
    const secondSession = createEmptyChatSurfaceSession(secondConversationId, secondSurface.sessionKey);

    useChatStore.setState({
      sessionsByConversationId: {},
      timelinesByConversationId: {},
      surfaceSessionsByKey: {
        [firstSurface.sessionKey]: {
          ...firstSession,
          streamingMessageId: firstMessageId,
          completedSegments: [{ type: 'text', content: 'live da conversa um' }],
        },
        [secondSurface.sessionKey]: {
          ...secondSession,
          streamingMessageId: secondMessageId,
          completedSegments: [{ type: 'text', content: 'live da conversa dois' }],
        },
      },
      liveMessageContentByConversationId: {
        [conversationId]: { [firstMessageId]: 'texto um' },
        [secondConversationId]: { [secondMessageId]: 'texto dois' },
      },
    });

    const { rerender } = renderMessage(firstSurface, firstMessage);
    expect(screen.getByText('live da conversa um')).toBeInTheDocument();
    expect(screen.getByText('texto um')).toBeInTheDocument();

    rerender(
      <ChatSessionProvider surface={secondSurface}>
        <SessionMessage key={`${secondSurface.sessionKey}:${secondMessage.id}`} message={secondMessage} />
      </ChatSessionProvider>,
    );

    expect(screen.getByText('live da conversa dois')).toBeInTheDocument();
    expect(screen.getByText('texto dois')).toBeInTheDocument();
    expect(screen.queryByText('live da conversa um')).not.toBeInTheDocument();
    expect(screen.queryByText('texto um')).not.toBeInTheDocument();
  });
});
