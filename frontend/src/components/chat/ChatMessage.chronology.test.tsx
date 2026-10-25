import { act, fireEvent, render, screen, within } from '@testing-library/react';
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

vi.mock('../ui/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => (
    <div data-testid="markdown">{content}</div>
  ),
}));

const conversationId = '01926b90-7a5a-7c4e-8d3f-000000000101';
const surfaceId = 'embedded:chronology-test';
const finalContent = 'resposta final canônica';

const surfaceFor = (targetConversationId: string): ChatSurfaceIdentity => (
  createChatSurfaceIdentity({
    conversationId: targetConversationId,
    surfaceId,
    surfaceType: 'embedded',
  })
);

const invocation = (callId: string, name: string) => ({
  invocationId: `inv-${callId}`,
  callId,
  name,
  status: 'succeeded',
  hasDetails: false,
  resultAvailability: 'available',
});

const messageFor = (
  id: string,
  overrides: Partial<ConstructorParameters<typeof chat.EnrichedMessage>[0]> = {},
): Message => new chat.EnrichedMessage({
  id,
  conversationId,
  role: 'assistant',
  content: '',
  createdAt: '2026-09-17T00:00:00.000Z',
  timestamp: Date.parse('2026-09-17T00:00:00.000Z'),
  isStreaming: false,
  internal: false,
  ...overrides,
}) as Message;

const chronologicalSegments = (includeConclusion = true): TurnSegment[] => [
  { type: 'text', content: 'texto intermediário um' },
  {
    type: 'tool_calls',
    toolInvocations: [invocation('one', 'buscar-primeira-fonte')],
  },
  { type: 'text', content: 'texto intermediário dois' },
  {
    type: 'tool_calls',
    toolInvocations: [invocation('two', 'refinar-segunda-fonte')],
  },
  ...(includeConclusion ? [{ type: 'text', content: finalContent } as TurnSegment] : []),
];

const seedStore = ({
  targetConversationId,
  messageId,
  streamingMessageId = null,
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
        title: 'Conversa cronológica',
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

function SessionMessage({ message, isReading = false }: { message: Message; isReading?: boolean }) {
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
      isReading={isReading}
    />
  );
}

const renderMessage = (
  surface: ChatSurfaceIdentity,
  message: Message,
  isReading = false,
) => render(
  <ChatSessionProvider surface={surface}>
    <SessionMessage message={message} isReading={isReading} />
  </ChatSessionProvider>,
);

const messageContent = () => document.querySelector('.chat-message__content') as HTMLElement;

const assertChronologicalOrder = (content: HTMLElement) => {
  const chain = content.querySelector('.chat-message__segments-log') as HTMLElement;
  const firstText = within(chain).getByText('texto intermediário um');
  const firstTools = chain.querySelectorAll('.tool-calls-section')[0] as HTMLElement;
  const secondText = within(chain).getByText('texto intermediário dois');
  const secondTools = chain.querySelectorAll('.tool-calls-section')[1] as HTMLElement;
  const conclusion = within(content).getByText(finalContent);

  expect(firstText.compareDocumentPosition(firstTools) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  expect(firstTools.compareDocumentPosition(secondText) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  expect(secondText.compareDocumentPosition(secondTools) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  expect(chain.compareDocumentPosition(conclusion) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  expect(chain).not.toContainElement(conclusion);
};

describe('cronologia de ChatMessage', () => {
  beforeEach(() => {
    useChatStore.setState({
      sessionsByConversationId: {},
      timelinesByConversationId: {},
      surfaceSessionsByKey: {},
      liveMessageContentByConversationId: {},
    });
  });

  it('exibe apenas o sufixo ao continuar e mantém a conclusão integral após finalizar', () => {
    const message = messageFor('assistant-live-continuation', { isStreaming: true });
    const prefix: TurnSegment[] = [
      { type: 'text', content: 'Parcial' },
      { type: 'tool_calls', toolInvocations: [invocation('previous', 'buscar-fonte')] },
    ];
    const surface = seedStore({
      targetConversationId: conversationId,
      messageId: message.id,
      streamingMessageId: message.id,
      completedSegments: prefix,
      liveContent: ' novo',
    });
    const { rerender } = renderMessage(surface, message);
    expect(within(messageContent()).getAllByTestId('markdown').map(node => node.textContent)).toEqual(['Parcial', ' novo']);

    const completed = messageFor(message.id, { content: 'Parcial novo' });
    completed._turnSegments = [...prefix, { type: 'text', content: ' novo' }];
    completed._resumableContent = 'Parcial novo';
    act(() => {
      seedStore({ targetConversationId: conversationId, messageId: message.id });
    });
    rerender(
      <ChatSessionProvider surface={surface}>
        <SessionMessage message={completed} />
      </ChatSessionProvider>,
    );
    expect(within(messageContent()).getAllByTestId('markdown').map(node => node.textContent)).toEqual(['Parcial', ' novo']);
    fireEvent.click(screen.getByRole('button', { name: 'chat.collapseChain' }));
    expect(within(messageContent()).getAllByTestId('markdown').map(node => node.textContent)).toEqual(['Parcial novo']);
    fireEvent.click(screen.getByRole('button', { name: 'chat.expandChain' }));
    expect(within(messageContent()).getAllByTestId('markdown').map(node => node.textContent)).toEqual(['Parcial', ' novo']);
  });

  it('exibe a cadeia recebida e o aviso uma vez após falha de gravação, expandida ou recolhida', () => {
    const warning = 'Erro: não foi possível salvar';
    const scalar = `${finalContent}\n\n${warning}`;
    const message = messageFor('assistant-unsaved', { content: scalar });
    message._turnSegments = [
      { type: 'text', content: finalContent },
      { type: 'tool_calls', toolCalls: [{
        id: 'last-call', type: 'function', function: { name: 'search', arguments: '{}' },
        origin: 'builtin', status: 'done',
      }] },
      { type: 'text', content: warning, diagnostic: true },
    ];
    message._resumableContent = finalContent;
    const surface = seedStore({ targetConversationId: conversationId, messageId: message.id });
    renderMessage(surface, message);
    const expanded = messageContent();
    expect(within(expanded).getAllByTestId('markdown').map(node => node.textContent)).toEqual([finalContent, warning]);
    const tools = expanded.querySelector('.tool-calls-section')!;
    expect(within(expanded).getByText(finalContent).compareDocumentPosition(tools) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(tools.compareDocumentPosition(within(expanded).getByText(warning)) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'chat.collapseChain' }));
    expect(within(messageContent()).getAllByTestId('markdown').map(node => node.textContent)).toEqual([scalar]);
    fireEvent.click(screen.getByRole('button', { name: 'chat.expandChain' }));
    expect(within(messageContent()).getAllByTestId('markdown').map(node => node.textContent)).toEqual([finalContent, warning]);
  });

  it.each([
    { texts: ['Original'], scalar: 'Editado', represented: false },
    { texts: ['Original'], scalar: 'Editado', represented: false, success: true },
    { texts: ['notfoo'], scalar: 'foo', represented: false },
    { texts: ['Antes', 'Depois'], scalar: 'AntesDepois', represented: true },
    { texts: ['Histórico', 'Parcial', ' continuado'], scalar: 'Parcial continuado', represented: true },
  ])('mantém conclusão recebida visível sem duplicar fragmentos ($scalar, sucesso: $success)', ({ texts, scalar, represented, success }) => {
    const warning = 'Erro: não foi possível salvar';
    const message = messageFor('assistant-received-conclusion', { content: success ? scalar : `${scalar}\n\n${warning}` });
    message._resumableContent = scalar;
    message._turnSegments = texts.flatMap((text, index): TurnSegment[] => [
      { type: 'text', content: text },
      { type: 'tool_calls', toolInvocations: [invocation(`call-${index}`, 'buscar-fonte')] },
    ]);
    if (!success) message._turnSegments.push({ type: 'text', content: warning, diagnostic: true });
    const surface = seedStore({ targetConversationId: conversationId, messageId: message.id });
    renderMessage(surface, message);
    const expanded = [...texts, ...(!success ? [warning] : []), ...(!represented ? [scalar] : [])];
    expect(within(messageContent()).getAllByTestId('markdown').map(node => node.textContent)).toEqual(expanded);
    fireEvent.click(screen.getByRole('button', { name: 'chat.collapseChain' }));
    expect(within(messageContent()).getAllByTestId('markdown').map(node => node.textContent)).toEqual([message.content]);
    fireEvent.click(screen.getByRole('button', { name: 'chat.expandChain' }));
    expect(within(messageContent()).getAllByTestId('markdown').map(node => node.textContent)).toEqual(expanded);
  });

  it('mantém base divergente separada na conclusão recebida sem duplicar scalar', () => {
    const message = messageFor('assistant-divergent-base', { content: 'Editado novo' });
    message._resumableContent = 'Editado novo';
    message._turnSegments = [
      { type: 'text', content: 'Original' },
      { type: 'text', content: 'Editado novo' },
    ];
    const surface = seedStore({ targetConversationId: conversationId, messageId: message.id });
    renderMessage(surface, message);
    expect(within(messageContent()).getAllByTestId('markdown').map(node => node.textContent)).toEqual(['Original', 'Editado novo']);
    fireEvent.click(screen.getByRole('button', { name: 'chat.collapseChain' }));
    expect(within(messageContent()).getAllByTestId('markdown').map(node => node.textContent)).toEqual(['Editado novo']);
    fireEvent.click(screen.getByRole('button', { name: 'chat.expandChain' }));
    expect(within(messageContent()).getAllByText('Editado novo')).toHaveLength(1);
  });

  it('mostra texto editado sem descartar tools da cronologia canônica', () => {
    const editedContent = 'Resposta corrigida pelo usuário';
    const message = messageFor('assistant-edited-canonical', {
      content: editedContent,
      turnSegments: [
        { type: 'text', content: 'Texto original da execução' },
        { type: 'tool_calls', toolInvocations: [invocation('canonical-call', 'buscar-fonte')] },
      ],
    });
    const canonicalSegments = message.turnSegments;
    expect(message._turnSegments).toBeUndefined();
    const surface = seedStore({ targetConversationId: conversationId, messageId: message.id });
    renderMessage(surface, message);
    const expanded = messageContent();
    expect(within(expanded).getAllByText(editedContent)).toHaveLength(1);
    expect(expanded.querySelector('.tool-calls-section')).not.toBeNull();
    expect(expanded.querySelector('.tool-calls-section')!.compareDocumentPosition(within(expanded).getByText(editedContent)) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'chat.collapseChain' }));
    expect(within(messageContent()).getAllByTestId('markdown').map(item => item.textContent)).toEqual([editedContent]);
    fireEvent.click(screen.getByRole('button', { name: 'chat.expandChain' }));
    expect(messageContent().querySelector('.tool-calls-section')).not.toBeNull();
    expect(within(messageContent()).getAllByText(editedContent)).toHaveLength(1);
    expect(message.turnSegments).toBe(canonicalSegments);
    expect(message.turnSegments?.[1].toolInvocations?.[0].callId).toBe('canonical-call');
  });

  it('mostra placeholder de turno só com tools na cadeia recebida expandida e recolhida', () => {
    const message = messageFor('assistant-tools-only', { content: '' });
    message._turnSegments = [{ type: 'tool_calls', toolCalls: [{
      id: 'only-call', type: 'function', function: { name: 'search', arguments: '{}' },
      origin: 'builtin', status: 'done',
    }] }];
    const surface = seedStore({ targetConversationId: conversationId, messageId: message.id });
    renderMessage(surface, message);
    const placeholder = 'chat.toolOnlyTurnPlaceholder';
    expect(within(messageContent()).getByText(placeholder)).toBeVisible();
    expect(messageContent().querySelector('.tool-calls-section')).not.toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'chat.collapseChain' }));
    expect(within(messageContent()).getByText(placeholder)).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'chat.expandChain' }));
    expect(within(messageContent()).getAllByText(placeholder)).toHaveLength(1);
  });

  it('mantém aviso de limite visível após tools na cadeia recebida expandida', () => {
    const warning = 'chat.outputLimitReached';
    const message = messageFor('assistant-limit', { content: warning });
    message._turnSegments = [
      { type: 'tool_calls', toolCalls: [{
        id: 'limit-call', type: 'function', function: { name: 'search', arguments: '{}' },
        origin: 'builtin', status: 'done',
      }] },
      { type: 'text', content: warning, diagnostic: true },
    ];
    const surface = seedStore({ targetConversationId: conversationId, messageId: message.id });
    renderMessage(surface, message);
    const expanded = messageContent();
    expect(within(expanded).getAllByTestId('markdown').map(item => item.textContent)).toEqual([warning]);
    expect(expanded.querySelector('.tool-calls-section')!.compareDocumentPosition(within(expanded).getByText(warning)) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'chat.collapseChain' }));
    expect(within(messageContent()).getAllByTestId('markdown').map(item => item.textContent)).toEqual([warning]);
  });

  it('mantém texto → tools → texto → tools e coloca a conclusão uma vez após a cadeia', () => {
    const message = messageFor('assistant-chronological', {
      content: finalContent,
      turnSegments: chronologicalSegments(),
    });
    const surface = seedStore({
      targetConversationId: conversationId,
      messageId: message.id,
    });

    renderMessage(surface, message);

    const content = messageContent();
    const toggle = screen.getByRole('button', { name: 'chat.collapseChain' });
    const region = document.getElementById(toggle.getAttribute('aria-controls') || '');
    expect(region).not.toBeNull();
    expect(screen.getAllByText(finalContent, { exact: true })).toHaveLength(1);
    expect(region).not.toContainElement(screen.getByText(finalContent, { exact: true }));
    assertChronologicalOrder(content);
  });

  it('preserva a conclusão ao recolher/reexpandir sem duplicar e mantém o toggle acessível', () => {
    const message = messageFor('assistant-collapse', {
      content: finalContent,
      turnSegments: chronologicalSegments(),
    });
    const surface = seedStore({
      targetConversationId: conversationId,
      messageId: message.id,
    });

    const { rerender } = renderMessage(surface, message);
    const toggle = screen.getByRole('button', { name: 'chat.collapseChain' });
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
    expect(toggle).toHaveAttribute('tabindex', '-1');
    assertChronologicalOrder(messageContent());

    fireEvent.click(toggle);

    const collapsed = screen.getByRole('button', { name: 'chat.expandChain' });
    const collapsedRegion = document.getElementById(collapsed.getAttribute('aria-controls') || '');
    expect(collapsed).toHaveAttribute('aria-expanded', 'false');
    expect(screen.getAllByText(finalContent, { exact: true })).toHaveLength(1);
    expect(collapsedRegion).not.toContainElement(screen.getByText(finalContent, { exact: true }));
    expect(collapsedRegion).not.toHaveTextContent('texto intermediário um');
    expect(collapsedRegion).not.toHaveTextContent('buscar-primeira-fonte');

    fireEvent.click(collapsed);

    expect(screen.getByRole('button', { name: 'chat.collapseChain' })).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getAllByText(finalContent, { exact: true })).toHaveLength(1);
    expect(screen.getAllByTestId('markdown')).toHaveLength(3);
    assertChronologicalOrder(messageContent());

    rerender(
      <ChatSessionProvider surface={surface}>
        <SessionMessage message={message} isReading />
      </ChatSessionProvider>,
    );
    const readingToggle = screen.getByRole('button', { name: 'chat.collapseChain' });
    expect(readingToggle).toHaveAttribute('tabindex', '0');
    expect(messageContent().querySelectorAll('[aria-live]')).toHaveLength(0);
    expect(messageContent()).toContainElement(screen.getByText(finalContent, { exact: true }));
  });

  it('coloca o placeholder de turno somente com tools depois das tools', () => {
    const placeholder = 'chat.toolOnlyTurnPlaceholder';
    const message = messageFor('assistant-tool-only', {
      turnSegments: [{
        type: 'tool_calls',
        toolInvocations: [invocation('only', 'executar-apenas-tool')],
      }],
    });
    const surface = seedStore({
      targetConversationId: conversationId,
      messageId: message.id,
    });

    renderMessage(surface, message);

    const content = messageContent();
    const region = content.querySelector('.chat-message__segments-log') as HTMLElement;
    const tools = region.querySelector('.tool-calls-section') as HTMLElement;
    const placeholderElement = within(content).getByText(placeholder, { exact: true });
    expect(region).not.toContainElement(placeholderElement);
    expect(tools.compareDocumentPosition(placeholderElement) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.getAllByText(placeholder, { exact: true })).toHaveLength(1);
  });

  it('mantém a ordem ao passar de streaming para patch canônico e depois recarregar', () => {
    const messageId = 'assistant-stream-to-history';
    const streamingMessage = messageFor(messageId, {
      content: '',
      isStreaming: true,
    });
    const surface = seedStore({
      targetConversationId: conversationId,
      messageId,
      streamingMessageId: messageId,
      completedSegments: chronologicalSegments(false),
      liveContent: finalContent,
    });
    const { rerender } = renderMessage(surface, streamingMessage);

    expect(screen.getByText(finalContent, { exact: true })).toBeInTheDocument();
    expect(screen.getByText('texto intermediário um', { exact: true })).toBeInTheDocument();
    expect(screen.getAllByTestId('markdown')).toHaveLength(3);

    const canonicalMessage = messageFor(messageId, {
      content: finalContent,
      isStreaming: false,
      turnSegments: chronologicalSegments(),
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
        <SessionMessage message={canonicalMessage} />
      </ChatSessionProvider>,
    );
    assertChronologicalOrder(messageContent());

    // Um reload entrega um novo objeto com o mesmo patch canônico; a cadeia
    // deve continuar idêntica, sem reaproveitar estado transitório ou duplicar
    // a conclusão.
    const reloadedMessage = messageFor(messageId, {
      content: finalContent,
      isStreaming: false,
      turnSegments: chronologicalSegments(),
    });
    rerender(
      <ChatSessionProvider surface={surface}>
        <SessionMessage message={reloadedMessage} />
      </ChatSessionProvider>,
    );
    expect(screen.getAllByText(finalContent, { exact: true })).toHaveLength(1);
    expect(messageContent().querySelectorAll('.tool-calls-section')).toHaveLength(2);
    assertChronologicalOrder(messageContent());
  });
});
