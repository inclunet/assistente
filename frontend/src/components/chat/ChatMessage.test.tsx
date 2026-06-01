import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import { chat } from '../../../wailsjs/go/models';
import { ChatMessage } from './ChatMessage';

const subscribeSpy = vi.fn();
const conversationId = '01926b90-7a5a-7c4e-8d3f-000000000001';
const originalIntersectionObserver = globalThis.IntersectionObserver;
const buildAriaLabelMock = vi.hoisted(() => vi.fn((_args: unknown) => 'aria-label'));

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
  buildChatMessageAriaLabel: (args: unknown) => buildAriaLabelMock(args),
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
  ToolCallsSection: ({ toolCallsJson }: { toolCallsJson?: string }) => (
    <div data-testid="toolcalls" data-json={toolCallsJson ?? ''} />
  ),
}));

describe('ChatMessage', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    buildAriaLabelMock.mockClear();
    if (originalIntersectionObserver) {
      vi.stubGlobal('IntersectionObserver', originalIntersectionObserver);
    }
  });

  it('renderiza conteudo e botao de audio', () => {
    const onSpeak = vi.fn();
    const message = new chat.EnrichedMessage({
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
    expect(buildAriaLabelMock).toHaveBeenCalledWith(expect.objectContaining({
      timePrefix: 'chat.sent',
    }));
  });

  it('renderiza modo de edicao', () => {
    const message = new chat.EnrichedMessage({
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
    const message = new chat.EnrichedMessage({
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

  it('não renderiza markdown pesado no frame em que o streaming termina', () => {
    class MockIntersectionObserver {
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
    const streamingMessage = new chat.EnrichedMessage({
      id: '1',
      conversationId,
      role: 'assistant',
      content: largeContent,
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: true,
      internal: false,
    });
    const completedMessage = new chat.EnrichedMessage({
      ...streamingMessage,
      isStreaming: false,
    });

    const { rerender } = render(<ChatMessage message={streamingMessage} />);
    expect(screen.getByText(largeContent)).toBeInTheDocument();

    rerender(<ChatMessage message={completedMessage} />);

    expect(screen.getByText('chat.largeMessageDeferred')).toBeInTheDocument();
    expect(screen.queryByText(largeContent)).not.toBeInTheDocument();
  });

  it('mantém tool calls no aria-label de mensagem pesada deferida', () => {
    class MockIntersectionObserver {
      observe = vi.fn();
      disconnect = vi.fn();
      unobserve = vi.fn();
      takeRecords = vi.fn(() => []);
      root = null;
      rootMargin = '';
      thresholds = [];
    }
    vi.stubGlobal('IntersectionObserver', MockIntersectionObserver);

    const toolCalls = JSON.stringify([{
      id: 'tool-1',
      type: 'function',
      function: {
        name: 'search_documents',
        arguments: 'x'.repeat(8_100),
      },
    }]);
    const message = new chat.EnrichedMessage({
      id: 'tool-only',
      conversationId,
      role: 'assistant',
      content: '',
      toolCalls,
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
    });

    render(<ChatMessage message={message} />);

    expect(buildAriaLabelMock).toHaveBeenCalledWith(expect.objectContaining({
      toolCallsRaw: JSON.stringify([{ function: { name: 'search_documents' } }]),
    }));
    expect(screen.queryByTestId('toolcalls')).not.toBeInTheDocument();
  });

  // Issue #150: o backend envia segmentos canônicos (texto → tools → texto →
  // tools → resposta final) e o ChatMessage precisa renderizar tudo dentro de
  // UMA única entrada acessível, com a cadeia de raciocínio em ordem.
  it('renderiza segmentos canônicos do turno em ordem cronológica numa única entrada', () => {
    const message = new chat.EnrichedMessage({
      id: 'turn-final',
      conversationId,
      role: 'assistant',
      content: 'resposta final',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
      turnSegments: [
        { type: 'text', content: 'vou pesquisar' },
        {
          type: 'tool_calls',
          toolCalls: [{
            id: 'tool-1',
            type: 'function',
            function: { name: 'search', arguments: '{}' },
            result: 'resultado da busca',
          }],
        },
        { type: 'text', content: 'agora vou refinar' },
        {
          type: 'tool_calls',
          toolCalls: [{
            id: 'tool-2',
            type: 'function',
            function: { name: 'fetch', arguments: '{}' },
          }],
        },
        { type: 'text', content: 'resposta final' },
      ],
    });

    const { container } = render(<ChatMessage message={message} />);

    expect(container.querySelectorAll('.chat-message')).toHaveLength(1);
    expect(screen.getByText('vou pesquisar')).toBeInTheDocument();
    expect(screen.getByText('agora vou refinar')).toBeInTheDocument();
    expect(screen.getByText('resposta final')).toBeInTheDocument();
    expect(screen.getAllByTestId('toolcalls')).toHaveLength(2);
    expect(screen.getByRole('heading', { level: 3 })).toHaveTextContent('chat.assistant');
  });

  it('renderiza placeholder acessível para turno sem resposta do assistente', () => {
    const message = new chat.EnrichedMessage({
      id: 'tool-only',
      conversationId,
      role: 'assistant',
      content: '',
      source: 'tool_only_turn_placeholder',
      toolCalls: JSON.stringify([{ id: 'tool-1', type: 'function', function: { name: 'tool_result', arguments: '' }, result: 'ok' }]),
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
    });

    render(<ChatMessage message={message} />);

    expect(screen.getByText('chat.toolOnlyTurnPlaceholder')).toBeInTheDocument();
    expect(screen.getByTestId('toolcalls')).toBeInTheDocument();
    expect(screen.queryByText('tool_only_turn_placeholder')).not.toBeInTheDocument();
    expect(buildAriaLabelMock).toHaveBeenCalledWith(expect.objectContaining({
      displayContent: 'chat.toolOnlyTurnPlaceholder',
    }));
  });

  // Turnos tool-only persistidos chegam do backend com `turnSegments` contendo
  // apenas um segmento `tool_calls`. Sem injetar o placeholder no início, o
  // leitor de tela perderia o contexto e a entrada soaria como resposta cortada.
  it('injeta placeholder textual antes das tools quando o turno tool-only tem apenas turnSegments', () => {
    const message = new chat.EnrichedMessage({
      id: 'tool-only-segmented',
      conversationId,
      role: 'assistant',
      content: '',
      source: 'tool_only_turn_placeholder',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
      turnSegments: [
        {
          type: 'tool_calls',
          toolCalls: [{
            id: 'tool-1',
            type: 'function',
            function: { name: 'search', arguments: '{}' },
            result: 'ok',
          }],
        },
      ],
    });

    const { container } = render(<ChatMessage message={message} />);

    const placeholder = screen.getByText('chat.toolOnlyTurnPlaceholder');
    const tools = screen.getByTestId('toolcalls');
    expect(placeholder).toBeInTheDocument();
    expect(tools).toBeInTheDocument();
    expect(container.querySelectorAll('.chat-message')).toHaveLength(1);
    // Placeholder precisa vir ANTES das tools na ordem do DOM para que o NVDA
    // anuncie o contexto antes do bloco de ferramentas.
    expect(placeholder.compareDocumentPosition(tools) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});
