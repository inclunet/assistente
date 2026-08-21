import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { chat } from '../../../wailsjs/go/models';
import { ChatMessage } from './ChatMessage';

const subscribeSpy = vi.fn();
const conversationId = '01926b90-7a5a-7c4e-8d3f-000000000001';
const originalIntersectionObserver = globalThis.IntersectionObserver;
const buildAriaLabelMock = vi.hoisted(() => vi.fn((_args: unknown) => 'aria-label'));
const announceRequestMock = vi.hoisted(() => vi.fn(() => true));
const markdownRendererSpy = vi.hoisted(() => vi.fn());

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

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announceRequest: announceRequestMock,
  }),
}));

vi.mock('../ui/MarkdownRenderer', () => ({
  MarkdownRenderer: (props: { content: string; tabNavigation?: string }) => {
    markdownRendererSpy(props);
    return <div>{props.content}</div>;
  },
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
    vi.useRealTimers();
    vi.unstubAllGlobals();
    buildAriaLabelMock.mockClear();
    announceRequestMock.mockClear();
    markdownRendererSpy.mockClear();
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

  it('habilita a navegação interna do renderer somente no modo de leitura', () => {
    const message = new chat.EnrichedMessage({
      id: 'reading-navigation',
      conversationId,
      role: 'assistant',
      content: '[Link](https://example.com)',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
    });

    const onSpeak = vi.fn();
    const { rerender } = render(<ChatMessage message={message} onSpeak={onSpeak} />);
    expect(markdownRendererSpy).toHaveBeenLastCalledWith(expect.objectContaining({
      tabNavigation: 'disabled',
    }));

    rerender(<ChatMessage message={message} onSpeak={onSpeak} isReading />);
    expect(markdownRendererSpy).toHaveBeenLastCalledWith(expect.objectContaining({
      tabNavigation: 'enabled',
    }));
    expect(screen.getByRole('button', { name: 'chat.playAudio' })).toHaveAttribute('tabindex', '0');
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

  it('não anuncia conteúdo do assistente em nenhuma fase do streaming', () => {
    const streamingMessage = new chat.EnrichedMessage({
      id: 'assistant-no-announce-1',
      conversationId,
      role: 'assistant',
      content: 'Resposta parcial com conteúdo suficiente para anunciar.',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: true,
      internal: false,
    });
    const completedMessage = new chat.EnrichedMessage({
      ...streamingMessage,
      isStreaming: false,
      content: 'Resposta parcial com conteúdo suficiente para anunciar. Final.',
    });

    const { rerender } = render(<ChatMessage message={streamingMessage} />);
    rerender(<ChatMessage message={completedMessage} />);

    // AEP-0041 §4.3: a fala do conteúdo vem só do chat:speak do backend.
    expect(announceRequestMock).not.toHaveBeenCalled();
  });

  it('não anuncia conclusão de turno agêntico sem texto', () => {
    const streamingMessage = new chat.EnrichedMessage({
      id: 'assistant-tool-only-1',
      conversationId,
      role: 'assistant',
      content: '',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: true,
      internal: false,
    });
    const completedToolOnlyMessage = new chat.EnrichedMessage({
      ...streamingMessage,
      isStreaming: false,
      turnSegments: [
        {
          type: 'tool_calls',
          toolCalls: [{ id: 'tool-1', type: 'function', function: { name: 'search', arguments: '{}' } }],
        },
      ],
    });

    const { rerender } = render(
      <ChatMessage
        message={streamingMessage}
        completedSegments={[
          {
            type: 'tool_calls',
            toolCalls: [{ id: 'tool-1', type: 'function', function: { name: 'search', arguments: '{}' } }],
          },
        ]}
      />
    );
    rerender(<ChatMessage message={completedToolOnlyMessage} />);

    // O aviso de turno tool-only é responsabilidade do chatEventController.
    expect(announceRequestMock).not.toHaveBeenCalled();
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

  // Issue #163 (Parte B): a conclusão canônica do turno é `message.content`
  // consolidado pelo backend (`consolidateTimelineTurn` => finalContent = última
  // resposta textual do assistente). O aria-label deve refletir essa conclusão
  // MESMO quando ela NÃO é o último `TurnSegment` de texto da cadeia (ex.: a
  // cadeia termina num texto intermediário ou em tool_calls), nunca um trecho
  // intermediário.
  it('usa message.content como conclusão quando ele não é o último segmento de texto', () => {
    const message = new chat.EnrichedMessage({
      id: 'turn-final',
      conversationId,
      role: 'assistant',
      // Conclusão canônica consolidada do backend.
      content: 'resposta final consolidada',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
      // A cadeia termina num texto INTERMEDIÁRIO seguido de tool_calls — sem um
      // segmento de texto final que coincida com a conclusão.
      turnSegments: [
        { type: 'text', content: 'vou pesquisar' },
        {
          type: 'tool_calls',
          toolCalls: [{ id: 'tool-1', type: 'function', function: { name: 'search', arguments: '{}' }, result: 'r' }],
        },
        { type: 'text', content: 'agora vou refinar' },
        {
          type: 'tool_calls',
          toolCalls: [{ id: 'tool-2', type: 'function', function: { name: 'fetch', arguments: '{}' } }],
        },
      ],
    });

    render(<ChatMessage message={message} />);

    expect(buildAriaLabelMock).toHaveBeenLastCalledWith(expect.objectContaining({
      displayContent: 'resposta final consolidada',
    }));
    const lastCalls = buildAriaLabelMock.mock.calls;
    const lastArgs = lastCalls[lastCalls.length - 1]?.[0] as { displayContent: string };
    expect(lastArgs.displayContent).not.toContain('vou pesquisar');
    expect(lastArgs.displayContent).not.toContain('agora vou refinar');
  });

  // Issue #163 (Parte B): quando a conclusão é, de fato, o último segmento de
  // texto e coincide com `message.content`, o aria-label traz essa resposta.
  it('usa a conclusão como último segmento quando coincide com message.content', () => {
    const message = new chat.EnrichedMessage({
      id: 'turn-final-seg',
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
          toolCalls: [{ id: 'tool-1', type: 'function', function: { name: 'search', arguments: '{}' }, result: 'r' }],
        },
        { type: 'text', content: 'resposta final' },
      ],
    });

    render(<ChatMessage message={message} />);

    expect(buildAriaLabelMock).toHaveBeenLastCalledWith(expect.objectContaining({
      displayContent: 'resposta final',
    }));
    const lastCalls = buildAriaLabelMock.mock.calls;
    const lastArgs = lastCalls[lastCalls.length - 1]?.[0] as { displayContent: string };
    expect(lastArgs.displayContent).not.toContain('vou pesquisar');
  });

  it('substitui (não concatena) o aria-label pelo texto ao vivo mais recente durante o streaming, mesmo antes de virar TurnSegment', () => {
    // Cenário real do streaming agêntico: a iteração em curso aparece em
    // `content` (live) e só vira um TurnSegment concluído depois. O anúncio
    // precisa refletir esse trecho mais recente, sem ficar preso no último
    // segmento já fechado.
    const first = new chat.EnrichedMessage({
      id: 'turn-streaming',
      conversationId,
      role: 'assistant',
      content: 'parte um',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: true,
      internal: false,
    });

    const { rerender } = render(
      <ChatMessage
        message={first}
        completedSegments={[{ type: 'text', content: 'parte um' }]}
      />
    );

    expect(buildAriaLabelMock).toHaveBeenLastCalledWith(expect.objectContaining({
      displayContent: 'parte um',
    }));

    // O novo trecho ("parte dois") chega como conteúdo ao vivo; os segmentos
    // concluídos ainda terminam no texto anterior ("parte um") + tool_calls.
    const second = new chat.EnrichedMessage({
      id: 'turn-streaming',
      conversationId,
      role: 'assistant',
      content: 'parte dois',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: true,
      internal: false,
    });

    rerender(
      <ChatMessage
        message={second}
        completedSegments={[
          { type: 'text', content: 'parte um' },
          { type: 'tool_calls', toolCalls: [{ id: 't', type: 'function', function: { name: 'search', arguments: '{}' } }] },
        ]}
      />
    );

    expect(buildAriaLabelMock).toHaveBeenLastCalledWith(expect.objectContaining({
      displayContent: 'parte dois',
    }));
    const calls = buildAriaLabelMock.mock.calls;
    const lastArgs = calls[calls.length - 1]?.[0] as { displayContent: string };
    // Substituição: o anúncio reflete só o trecho mais recente, sem acumular o anterior.
    expect(lastArgs.displayContent).not.toContain('parte um');
  });

  it('mantém o placeholder tool-only no aria-label quando o turno só tem tool_calls', () => {
    const message = new chat.EnrichedMessage({
      id: 'tool-only-aria',
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
          toolCalls: [{ id: 'tool-1', type: 'function', function: { name: 'search', arguments: '{}' }, result: 'ok' }],
        },
      ],
    });

    render(<ChatMessage message={message} />);

    expect(buildAriaLabelMock).toHaveBeenLastCalledWith(expect.objectContaining({
      displayContent: 'chat.toolOnlyTurnPlaceholder',
    }));
  });

  // Issue #163 (Parte B): fallback — quando `message.content` está ausente/vazio,
  // a conclusão é o último `TurnSegment` de texto NÃO-vazio, ignorando segmentos
  // finais em branco.
  it('faz fallback para o último segmento textual não-vazio quando content está vazio', () => {
    const message = new chat.EnrichedMessage({
      id: 'turn-empty-content',
      conversationId,
      role: 'assistant',
      content: '',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
      turnSegments: [
        { type: 'text', content: 'primeiro' },
        {
          type: 'tool_calls',
          toolCalls: [{ id: 'tool-1', type: 'function', function: { name: 'search', arguments: '{}' } }],
        },
        { type: 'text', content: 'segundo' },
        { type: 'text', content: '   ' },
      ],
    });

    render(<ChatMessage message={message} />);

    expect(buildAriaLabelMock).toHaveBeenLastCalledWith(expect.objectContaining({
      displayContent: 'segundo',
    }));
  });

  // Issue #163 (Parte B): quando o último segmento textual é vazio/whitespace mas
  // a conclusão canônica está em `message.content`, o aria-label usa o content —
  // não o último segmento (que está em branco) nem um intermediário.
  it('usa message.content quando o último segmento textual está em branco', () => {
    const message = new chat.EnrichedMessage({
      id: 'turn-blank-final',
      conversationId,
      role: 'assistant',
      content: 'conclusão canônica',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
      turnSegments: [
        { type: 'text', content: 'intermediário' },
        {
          type: 'tool_calls',
          toolCalls: [{ id: 'tool-1', type: 'function', function: { name: 'search', arguments: '{}' } }],
        },
        { type: 'text', content: '   ' },
      ],
    });

    render(<ChatMessage message={message} />);

    expect(buildAriaLabelMock).toHaveBeenLastCalledWith(expect.objectContaining({
      displayContent: 'conclusão canônica',
    }));
    const lastCalls = buildAriaLabelMock.mock.calls;
    const lastArgs = lastCalls[lastCalls.length - 1]?.[0] as { displayContent: string };
    expect(lastArgs.displayContent).not.toContain('intermediário');
  });

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

  // Issue #163 (Parte A): a cadeia inteira do turno (segmentos de texto E tool
  // calls) precisa estar dentro de `.chat-message__content` — o alvo de foco do
  // modo de leitura (role="document") —, garantindo que o leitor de tela leia
  // TUDO quando a cadeia está expandida.
  it('mantém toda a cadeia (segmentos + tool calls) dentro de chat-message__content no modo de leitura', () => {
    const message = new chat.EnrichedMessage({
      id: 'turn-reading',
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
          toolCalls: [{ id: 'tool-1', type: 'function', function: { name: 'search', arguments: '{}' }, result: 'r' }],
        },
        { type: 'text', content: 'agora vou refinar' },
        {
          type: 'tool_calls',
          toolCalls: [{ id: 'tool-2', type: 'function', function: { name: 'fetch', arguments: '{}' } }],
        },
        { type: 'text', content: 'resposta final' },
      ],
    });

    const { container } = render(<ChatMessage message={message} isReading />);

    const content = container.querySelector('.chat-message__content');
    expect(content).not.toBeNull();
    expect(content).toHaveTextContent('vou pesquisar');
    expect(content).toHaveTextContent('agora vou refinar');
    expect(content).toHaveTextContent('resposta final');
    expect(content?.querySelectorAll('[data-testid="toolcalls"]')).toHaveLength(2);
  });

  // Issue #163 (Parte A): a cadeia oferece uma affordance acessível de
  // contrair/expandir (botão com nome e `aria-expanded`). Inicia expandida
  // (preserva o visual) e recolher remove a cadeia do DOM por economia.
  it('expõe um controle de contrair/expandir a cadeia com aria-expanded', () => {
    const message = new chat.EnrichedMessage({
      id: 'turn-toggle',
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
          toolCalls: [{ id: 'tool-1', type: 'function', function: { name: 'search', arguments: '{}' }, result: 'r' }],
        },
        { type: 'text', content: 'resposta final' },
      ],
    });

    render(<ChatMessage message={message} />);

    const toggle = screen.getByRole('button', { name: 'chat.collapseChain' });
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
    // Fora do modo de leitura o toggle não é alcançável por Tab (convenção do
    // componente: interativos in-message usam tabIndex=-1 para não criar tabstops
    // ao longo da lista de mensagens).
    expect(toggle).toHaveAttribute('tabindex', '-1');
    expect(screen.getByText('vou pesquisar')).toBeInTheDocument();
    expect(screen.getAllByTestId('toolcalls')).toHaveLength(1);

    fireEvent.click(toggle);

    const collapsed = screen.getByRole('button', { name: 'chat.expandChain' });
    expect(collapsed).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByText('vou pesquisar')).not.toBeInTheDocument();
    expect(screen.queryByTestId('toolcalls')).not.toBeInTheDocument();

    fireEvent.click(collapsed);

    expect(screen.getByRole('button', { name: 'chat.collapseChain' })).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByText('vou pesquisar')).toBeInTheDocument();
    expect(screen.getAllByTestId('toolcalls')).toHaveLength(1);
  });

  // Issue #163 (PR #168, comentário de legle): o toggle só pode receber foco no
  // modo de leitura; do contrário ele (e os demais interativos da mensagem) não
  // deve ser um tabstop ao navegar pela lista de mensagens.
  it('torna o toggle da cadeia focável apenas no modo de leitura', () => {
    const message = new chat.EnrichedMessage({
      id: 'turn-toggle-tabindex',
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
          toolCalls: [{ id: 'tool-1', type: 'function', function: { name: 'search', arguments: '{}' }, result: 'r' }],
        },
        { type: 'text', content: 'resposta final' },
      ],
    });

    const { rerender } = render(<ChatMessage message={message} />);
    expect(screen.getByRole('button', { name: 'chat.collapseChain' })).toHaveAttribute('tabindex', '-1');

    rerender(<ChatMessage message={message} isReading />);
    expect(screen.getByRole('button', { name: 'chat.collapseChain' })).toHaveAttribute('tabindex', '0');
  });

  // Issue #163 (PR #168, comentário Copilot 3337447363): se o usuário recolheu a
  // cadeia, ao entrar no modo de leitura ela precisa ser forçada de volta ao DOM
  // para que o role="document" do useVirtualModal exponha todos os segmentos +
  // tool calls. O `aria-expanded` permanece consistente com o conteúdo visível.
  it('força a cadeia recolhida de volta ao DOM ao entrar no modo de leitura', () => {
    const message = new chat.EnrichedMessage({
      id: 'turn-reading-collapsed',
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
          toolCalls: [{ id: 'tool-1', type: 'function', function: { name: 'search', arguments: '{}' }, result: 'r' }],
        },
        { type: 'text', content: 'agora vou refinar' },
        {
          type: 'tool_calls',
          toolCalls: [{ id: 'tool-2', type: 'function', function: { name: 'fetch', arguments: '{}' } }],
        },
        { type: 'text', content: 'resposta final' },
      ],
    });

    const { rerender } = render(<ChatMessage message={message} />);

    // Usuário recolhe a cadeia fora do modo de leitura.
    fireEvent.click(screen.getByRole('button', { name: 'chat.collapseChain' }));
    expect(screen.getByRole('button', { name: 'chat.expandChain' })).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByText('vou pesquisar')).not.toBeInTheDocument();
    expect(screen.queryByTestId('toolcalls')).not.toBeInTheDocument();

    // Ao entrar no modo de leitura, a cadeia inteira volta ao DOM.
    rerender(<ChatMessage message={message} isReading />);

    const toggle = screen.getByRole('button', { name: 'chat.collapseChain' });
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByText('vou pesquisar')).toBeInTheDocument();
    expect(screen.getByText('agora vou refinar')).toBeInTheDocument();
    expect(screen.getByText('resposta final')).toBeInTheDocument();
    expect(screen.getAllByTestId('toolcalls')).toHaveLength(2);
  });

  // Issue #163 (PR #168, comentário Copilot 3337834792): ao SAIR do modo de
  // leitura, o estado da cadeia deve voltar ao que era antes — se o usuário a
  // tinha recolhido, ela volta a recolher (não fica expandida permanentemente).
  it('restaura o estado recolhido da cadeia ao sair do modo de leitura', () => {
    const message = new chat.EnrichedMessage({
      id: 'turn-restore-collapsed',
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
          toolCalls: [{ id: 'tool-1', type: 'function', function: { name: 'search', arguments: '{}' }, result: 'r' }],
        },
        { type: 'text', content: 'resposta final' },
      ],
    });

    const { rerender } = render(<ChatMessage message={message} />);

    // Usuário recolhe a cadeia.
    fireEvent.click(screen.getByRole('button', { name: 'chat.collapseChain' }));
    expect(screen.getByRole('button', { name: 'chat.expandChain' })).toHaveAttribute('aria-expanded', 'false');

    // Entra no modo de leitura — a cadeia é forçada a expandir.
    rerender(<ChatMessage message={message} isReading />);
    expect(screen.getByRole('button', { name: 'chat.collapseChain' })).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByText('vou pesquisar')).toBeInTheDocument();

    // Sai do modo de leitura — o estado recolhido anterior é restaurado.
    rerender(<ChatMessage message={message} />);
    expect(screen.getByRole('button', { name: 'chat.expandChain' })).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByText('vou pesquisar')).not.toBeInTheDocument();
    expect(screen.queryByTestId('toolcalls')).not.toBeInTheDocument();
  });

  // Issue #163 (PR #168, comentário baixa-confiança ChatMessage.tsx:536): com a
  // cadeia recolhida (fora de streaming) a mensagem NÃO pode ficar vazia — exibe
  // a CONCLUSÃO do turno (mesma fonte de verdade do aria-label), sem as tool
  // calls nem os segmentos intermediários.
  it('exibe a conclusão (sem tool calls/intermediários) quando a cadeia está recolhida', () => {
    const message = new chat.EnrichedMessage({
      id: 'turn-collapsed-preview',
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
          toolCalls: [{ id: 'tool-1', type: 'function', function: { name: 'search', arguments: '{}' }, result: 'r' }],
        },
        { type: 'text', content: 'agora vou refinar' },
        { type: 'text', content: 'resposta final' },
      ],
    });

    render(<ChatMessage message={message} />);

    fireEvent.click(screen.getByRole('button', { name: 'chat.collapseChain' }));

    // Conclusão visível; mensagem não fica vazia.
    expect(screen.getByText('resposta final')).toBeInTheDocument();
    // Segmentos intermediários e tool calls ocultos no estado recolhido.
    expect(screen.queryByText('vou pesquisar')).not.toBeInTheDocument();
    expect(screen.queryByText('agora vou refinar')).not.toBeInTheDocument();
    expect(screen.queryByTestId('toolcalls')).not.toBeInTheDocument();
  });
});
