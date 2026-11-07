import { windowFixture } from '../../test/chatWindowFixture';
import type { Message, MessageNode } from '../../lib/chatMessageTree';
import { messageFixture, nodeFixture, segmentFixture } from '../../test/chatPresentationFixture';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import React from 'react';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { MessageList } from './MessageList';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const announceRequestMock = vi.hoisted(() => vi.fn(() => true));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announceRequest: announceRequestMock,
  }),
}));

const hoisted = vi.hoisted(() => ({
  messageNodeMock: vi.fn(),
}));
vi.mock('./MessageNode', () => ({
  MessageNode: (props: { level?: number; siblingIndex?: number; [key: string]: unknown }) => {
    hoisted.messageNodeMock(props);
    return (
      <div
        data-testid="message-node"
        data-message-node=""
        data-level={props.level}
        data-sibling-index={props.siblingIndex}
        tabIndex={-1}
      />
    );
  },
}));

describe('MessageList', () => {
  const createNode = (id = '1') => nodeFixture({
    message: messageFixture({
      id,
      conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
      role: 'user',
      content: 'Oi',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
    }),
    childCount: 0,
    level: 0,
    children: [],
  });

  it('renderiza estado vazio', () => {
    render(<MessageList threadedMessages={[]} />);

    expect(screen.getByText('chat.emptyTitle')).toBeInTheDocument();
  });

  it('expõe região vazia por ref com foco programático sem adicionar parada de Tab', () => {
    const ref = React.createRef<HTMLDivElement>();
    render(<MessageList ref={ref} threadedMessages={[]} />);
    const region = screen.getByRole('region', { name: 'chat.messageListLabel' });
    expect(ref.current).toBe(region);
    expect(region).toHaveAttribute('tabindex', '-1');
    region.focus();
    expect(region).toHaveFocus();
  });

  it('renderiza mensagens e loading', () => {
    const node = createNode();
    render(
      <MessageList
        threadedMessages={[node]}
        isLoading
      />
    );

    expect(screen.getByTestId('message-node')).toBeInTheDocument();
    expect(screen.getByLabelText('chat.typing')).toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('reanuncia loading quando o broker rejeita progresso por origem inativa', () => {
    announceRequestMock
      .mockReturnValueOnce(false)
      .mockReturnValueOnce(true);
    const node = createNode();
    const origin = { conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001', surfaceId: 'chat-tab', surfaceType: 'chat' as const };
    const { rerender } = render(
      <MessageList
        threadedMessages={[node]}
        isLoading
        origin={origin}
      />
    );

    rerender(
      <MessageList
        threadedMessages={[node]}
        isLoading
        origin={{ ...origin }}
      />
    );

    expect(announceRequestMock).toHaveBeenCalledTimes(2);
  });

  it('reanuncia loading quando a origem muda', () => {
    const node = createNode();
    const origin = { conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001', surfaceId: 'chat-tab', surfaceType: 'chat' as const };
    const { rerender } = render(
      <MessageList
        threadedMessages={[node]}
        isLoading
        origin={origin}
      />
    );

    rerender(
      <MessageList
        threadedMessages={[node]}
        isLoading
        origin={{ ...origin, surfaceId: 'workspace-panel', surfaceType: 'page' as const }}
      />
    );

    expect(announceRequestMock).toHaveBeenCalledTimes(2);
  });

  it('anuncia mudança de texto enquanto loading continua ativo', () => {
    const node = createNode();
    const { rerender } = render(
      <MessageList
        threadedMessages={[node]}
        isLoading
        loadingText="Carregando resposta"
      />
    );

    rerender(
      <MessageList
        threadedMessages={[node]}
        isLoading
        loadingText="Ainda carregando"
      />
    );

    expect(announceRequestMock).toHaveBeenNthCalledWith(1, expect.objectContaining({
      message: 'Carregando resposta',
    }));
    expect(announceRequestMock).toHaveBeenNthCalledWith(2, expect.objectContaining({
      message: 'Ainda carregando',
    }));
  });

  it('repassa posição absoluta e tamanho total para itens renderizados', () => {
    hoisted.messageNodeMock.mockClear();
    const node = createNode('message-42');
    (node as typeof node & { originalIndex?: number }).originalIndex = 41;

    render(
      <MessageList
        threadedMessages={[node]}
        messageWindow={windowFixture({
          totalCount: 100,
          startIndex: 41,
          endIndex: 41,
          hasBefore: true,
          hasAfter: true,
        })}
      />
    );

    expect(hoisted.messageNodeMock).toHaveBeenCalledWith(expect.objectContaining({
      ariaPosition: 42,
      ariaSetSize: 100,
    }));
  });

  it('usa contagem da janela mesmo quando o DTO antigo não tem índice absoluto', () => {
    hoisted.messageNodeMock.mockClear();
    const firstNode = createNode('message-1');
    const expandedTurnNode = createNode('message-expanded');
    const thirdNode = createNode('message-3');
    (firstNode as typeof firstNode & { originalIndex?: number }).originalIndex = 40;
    (thirdNode as typeof thirdNode & { originalIndex?: number }).originalIndex = 41;

    render(
      <MessageList
        threadedMessages={[firstNode, expandedTurnNode, thirdNode]}
        messageWindow={windowFixture({
          totalCount: 100,
          startIndex: 40,
          endIndex: 41,
          hasBefore: true,
          hasAfter: true,
        })}
      />
    );

    const calls = hoisted.messageNodeMock.mock.calls.map(([props]) => props);
    expect(calls).toEqual(expect.arrayContaining([
      expect.objectContaining({ ariaPosition: 41, ariaSetSize: 100 }),
      expect.objectContaining({ ariaPosition: 42, ariaSetSize: 100 }),
      expect.objectContaining({ ariaPosition: 43, ariaSetSize: 100 }),
    ]));
  });

  it('usa posições canônicas quando o backend retorna item de turno consolidado', () => {
    hoisted.messageNodeMock.mockClear();
    const userNode = createNode('user-1');
    const turnNode = createNode('assistant-final');
    turnNode.message.role = 'assistant';
    turnNode.message.turnId = 'user-1';
    turnNode.message.turnSegments = [segmentFixture({
      type: 'tool_calls',
      toolInvocations: [{
        invocationId: 'inv-1',
        callId: 'tool-1',
        name: 'search',
        status: 'succeeded',
        hasDetails: true,
        resultAvailability: 'available',
      }],
    })];
    (userNode as typeof userNode & { originalIndex?: number }).originalIndex = 0;
    (turnNode as typeof turnNode & { originalIndex?: number }).originalIndex = 1;

    render(
      <MessageList
        threadedMessages={[userNode, turnNode]}
        messageWindow={windowFixture({
          totalCount: 2,
          startIndex: 0,
          endIndex: 1,
          hasBefore: false,
          hasAfter: false,
        })}
      />
    );

    const calls = hoisted.messageNodeMock.mock.calls.map(([props]) => props);
    expect(calls).toEqual(expect.arrayContaining([
      expect.objectContaining({ ariaPosition: 1, ariaSetSize: 2 }),
      expect.objectContaining({ ariaPosition: 2, ariaSetSize: 2 }),
    ]));
  });

  it('não mantém uma segunda consolidação persistida no frontend', () => {
    hoisted.messageNodeMock.mockClear();
    const firstAssistant = createNode('assistant-1');
    firstAssistant.message.role = 'assistant';
    firstAssistant.message.turnId = 'turn-1';
    firstAssistant.message.content = 'intermediário';
    const secondAssistant = createNode('assistant-2');
    secondAssistant.message.role = 'assistant';
    secondAssistant.message.turnId = 'turn-2';
    secondAssistant.message.content = '';
    const thirdAssistant = createNode('assistant-3');
    thirdAssistant.message.role = 'assistant';
    thirdAssistant.message.turnId = 'turn-3';
    thirdAssistant.message.content = '';

    render(<MessageList threadedMessages={[firstAssistant, secondAssistant, thirdAssistant]} />);

    const renderedIds = hoisted.messageNodeMock.mock.calls.map(
      ([props]) => (props as { node: MessageNode }).node.message.id,
    );
    expect(renderedIds).toEqual(['assistant-1', 'assistant-2', 'assistant-3']);
  });

  // Issue #150: quando o backend devolve um único nó canônico já consolidado
  // com `turnSegments`, o MessageList NÃO deve quebrá-lo em itens separados —
  // o turno inteiro é UMA entrada do histórico.
  it('mantém turno canônico do backend como uma única entrada com segments', () => {
    hoisted.messageNodeMock.mockClear();
    const userNode = createNode('user-1');
    const turnNode = createNode('assistant-final');
    turnNode.message.role = 'assistant';
    turnNode.message.turnId = 'turn-1';
    turnNode.message.content = 'resposta final';
    (turnNode.message as unknown as Record<string, unknown>).turnSegments = [
      { type: 'text', content: 'intermediário' },
      { type: 'tool_calls', toolInvocations: [{ invocationId: 'inv-1', callId: 'tool-1', name: 'search', status: 'succeeded', hasDetails: true, resultAvailability: 'available' }] },
      { type: 'text', content: 'resposta final' },
    ];

    render(<MessageList threadedMessages={[userNode, turnNode]} />);

    expect(hoisted.messageNodeMock).toHaveBeenCalledTimes(2);
    const turnProps = hoisted.messageNodeMock.mock.calls[1][0] as {
      node: MessageNode & { message: Message & { turnSegments?: unknown[] } };
    };
    expect(turnProps.node.message.id).toBe('assistant-final');
    expect(turnProps.node.message.turnSegments).toEqual([
      { type: 'text', content: 'intermediário' },
      { type: 'tool_calls', toolInvocations: [{ invocationId: 'inv-1', callId: 'tool-1', name: 'search', status: 'succeeded', hasDetails: true, resultAvailability: 'available' }] },
      { type: 'text', content: 'resposta final' },
    ]);
  });

  it('não mistura segmentos canônicos com outro nó transitório', () => {
    hoisted.messageNodeMock.mockClear();
    const canonicalNode = createNode('assistant-canonical');
    canonicalNode.message.role = 'assistant';
    canonicalNode.message.turnId = 'turn-1';
    canonicalNode.message.content = 'resposta canônica';
    (canonicalNode.message as typeof canonicalNode.message & { _turnSegments?: unknown })._turnSegments = [
      { type: 'text', content: 'segmento canônico' },
      { type: 'tool_calls', toolInvocations: [{ invocationId: 'inv-1', callId: 'tool-1', name: 'search', status: 'succeeded', hasDetails: true, resultAvailability: 'available' }] },
    ];
    const streamingNode = createNode('streaming-turn-1');
    streamingNode.message.role = 'assistant';
    streamingNode.message.turnId = 'turn-2';
    streamingNode.message.content = 'continuação transitória';
    streamingNode.message.isStreaming = true;

    render(<MessageList threadedMessages={[canonicalNode, streamingNode]} />);

    const canonicalProps = hoisted.messageNodeMock.mock.calls[0][0] as {
      node: MessageNode & { message: Message & { _turnSegments?: unknown[] } };
    };
    const streamingProps = hoisted.messageNodeMock.mock.calls[1][0] as {
      node: MessageNode & { message: Message & { _turnSegments?: unknown[] } };
    };
    expect(hoisted.messageNodeMock).toHaveBeenCalledTimes(2);
    expect(canonicalProps.node.message._turnSegments).toEqual([
      { type: 'text', content: 'segmento canônico' },
      { type: 'tool_calls', toolInvocations: [{ invocationId: 'inv-1', callId: 'tool-1', name: 'search', status: 'succeeded', hasDetails: true, resultAvailability: 'available' }] },
    ]);
    expect(streamingProps.node.message.content).toBe('continuação transitória');
  });

  it('dispara callbacks de salto por Ctrl+Home e Ctrl+End', () => {
    const onJumpToStart = vi.fn();
    const onJumpToEnd = vi.fn();

    render(
      <MessageList
        threadedMessages={[createNode()]}
        onJumpToStart={onJumpToStart}
        onJumpToEnd={onJumpToEnd}
      />
    );

    const list = screen.getByRole('list');
    fireEvent.keyDown(list, { key: 'Home', ctrlKey: true });
    fireEvent.keyDown(list, { key: 'End', ctrlKey: true });

    expect(onJumpToStart).toHaveBeenCalledTimes(1);
    expect(onJumpToEnd).toHaveBeenCalledTimes(1);
  });

  it('carrega janelas adjacentes por scroll do usuário', async () => {
    const onLoadOlder = vi.fn();
    const onLoadNewer = vi.fn();

    render(
      <MessageList
        threadedMessages={[createNode()]}
        messageWindow={windowFixture({ hasBefore: true, hasAfter: true })}
        onLoadOlder={onLoadOlder}
        onLoadNewer={onLoadNewer}
      />
    );

    await new Promise((resolve) => {
      window.setTimeout(resolve, 0);
    });

    const container = screen.getByLabelText('chat.messageListLabel');
    Object.defineProperty(container, 'scrollHeight', { configurable: true, value: 1000 });
    Object.defineProperty(container, 'clientHeight', { configurable: true, value: 200 });

    Object.defineProperty(container, 'scrollTop', { configurable: true, value: 0 });
    fireEvent.scroll(container);
    expect(onLoadOlder).toHaveBeenCalledTimes(1);

    Object.defineProperty(container, 'scrollTop', { configurable: true, value: 800 });
    fireEvent.scroll(container);
    expect(onLoadNewer).toHaveBeenCalledTimes(1);

    // O gatilho separa o carregamento automático da navegação: só o automático
    // pode cair no meio da leitura de uma resposta.
    expect(onLoadOlder).toHaveBeenCalledWith('scroll');
    expect(onLoadNewer).toHaveBeenCalledWith('scroll');
  });

  it('marca como navegação o carregamento pedido pelo botão de mensagens antigas', () => {
    const onLoadOlder = vi.fn();

    render(
      <MessageList
        threadedMessages={[createNode()]}
        messageWindow={windowFixture({ hasBefore: true })}
        onLoadOlder={onLoadOlder}
      />
    );

    fireEvent.click(screen.getByText('chat.loadOlderMessages'));

    expect(onLoadOlder).toHaveBeenCalledWith('navigation');
  });

  it('usa carregamento genérico para a navegação compartilhada e restaura o botão ao terminar', () => {
    const props = { threadedMessages: [createNode()], messageWindow: windowFixture({ hasBefore: true, hasAfter: true }), onLoadOlder: vi.fn() };
    const { rerender } = render(<MessageList {...props} isLoadingMessageWindow />);
    const loading = screen.getByRole('button', { name: 'common.loading' });
    expect(loading).toBeDisabled();
    expect(loading).toHaveAttribute('aria-busy', 'true');
    rerender(<MessageList {...props} />);
    expect(screen.getByRole('button', { name: 'chat.loadOlderMessages' })).toBeEnabled();
  });

  // Regressão de produto com foco real, sem fabricar focusout para o body.
  it('preserva o mesmo nó focado durante atualização de streaming e prepend', () => {
    const first = createNode('user-1');
    const streaming = createNode('streaming-1');
    streaming.message.role = 'assistant';
    streaming.message.isStreaming = true;

    const { container, rerender } = render(
      <MessageList threadedMessages={[first, streaming]} isLoading />
    );

    const list = container.querySelector('.message-list__list') as HTMLElement;
    const lastNode = list.querySelector(
      '[data-message-node][data-level="0"][data-sibling-index="1"]'
    ) as HTMLElement;

    // Entra na lista: foco no último irmão (mensagem em streaming).
    act(() => lastNode.focus());
    expect(document.activeElement).toBe(lastNode);

    // O índice muda, mas a identidade e o elemento permanecem.
    const updatedStreaming = createNode('streaming-1');
    updatedStreaming.message.role = 'assistant';
    updatedStreaming.message.isStreaming = true;
    updatedStreaming.message.content = 'parcial...';
    rerender(<MessageList threadedMessages={[createNode('older'), first, updatedStreaming]} isLoading />);

    const restored = list.querySelector(
      '[data-message-node][data-level="0"][data-sibling-index="2"]'
    ) as HTMLElement;
    expect(restored).toBe(lastNode);
    expect(document.activeElement).toBe(lastNode);
  });

  it('não rouba o foco quando ele saiu da lista intencionalmente para outro elemento', () => {
    const outside = document.createElement('button');
    document.body.appendChild(outside);

    const first = createNode('user-1');
    const streaming = createNode('streaming-1');
    streaming.message.role = 'assistant';
    streaming.message.isStreaming = true;

    const { container, rerender } = render(
      <MessageList threadedMessages={[first, streaming]} isLoading />
    );

    const list = container.querySelector('.message-list__list') as HTMLElement;
    const lastNode = list.querySelector(
      '[data-message-node][data-level="0"][data-sibling-index="1"]'
    ) as HTMLElement;

    fireEvent.focusIn(lastNode, { target: lastNode });
    // Saída intencional para um elemento concreto fora da lista (ex.: input).
    outside.focus();
    fireEvent.focusOut(lastNode, { relatedTarget: outside });

    const updatedStreaming = createNode('streaming-1');
    updatedStreaming.message.role = 'assistant';
    updatedStreaming.message.isStreaming = true;
    updatedStreaming.message.content = 'parcial...';
    rerender(<MessageList threadedMessages={[first, updatedStreaming]} isLoading />);

    expect(document.activeElement).toBe(outside);
    document.body.removeChild(outside);
  });

  it('pagina conforme hasNewer do backend mesmo quando o último item recebe streaming', () => {
    hoisted.messageNodeMock.mockClear();
    const onLoadNewer = vi.fn();
    const onReachEnd = vi.fn();
    const streamingNode = createNode('streaming-conversation-1-1');
    streamingNode.message.isStreaming = true;

    render(
      <MessageList
        threadedMessages={[streamingNode]}
        messageWindow={windowFixture({ hasAfter: true })}
        onLoadNewer={onLoadNewer}
        onReachEnd={onReachEnd}
      />
    );

    const props = hoisted.messageNodeMock.mock.calls[0][0] as { onReachEnd: () => void };
    props.onReachEnd();

    expect(onLoadNewer).toHaveBeenCalledTimes(1);
    expect(onReachEnd).not.toHaveBeenCalled();
  });
});

describe('MessageList (virtualização)', () => {
  const VIEWPORT_HEIGHT = 700;
  const ITEM_HEIGHT = 140;
  let restoreDims: (() => void) | null = null;

  const createNodes = (count: number) =>
    Array.from({ length: count }, (_, index) =>
      nodeFixture({
        message: messageFixture({
          id: `msg-${index}`,
          conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
          role: index % 2 === 0 ? 'user' : 'assistant',
          content: `Mensagem ${index}`,
          createdAt: new Date().toISOString(),
          timestamp: Date.now() + index,
          isStreaming: false,
          internal: false,
        }),
        childCount: 0,
        level: 0,
        children: [],
      })
    );

  beforeEach(() => {
    announceRequestMock.mockReset();
    announceRequestMock.mockReturnValue(true);
    vi.useFakeTimers();
    const proto = window.HTMLElement.prototype;
    const original = Object.getOwnPropertyDescriptor(proto, 'offsetHeight');
    Object.defineProperty(proto, 'offsetHeight', {
      configurable: true,
      get(this: HTMLElement) {
        if (this.classList?.contains('message-list')) return VIEWPORT_HEIGHT;
        if (this.hasAttribute?.('data-index')) return ITEM_HEIGHT;
        return 0;
      },
    });
    restoreDims = () => {
      if (original) {
        Object.defineProperty(proto, 'offsetHeight', original);
      } else {
        delete (proto as unknown as Record<string, unknown>).offsetHeight;
      }
    };
  });

  afterEach(() => {
    cleanup();
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
    restoreDims?.();
    restoreDims = null;
  });

  const renderedIndexes = (container: HTMLElement): number[] =>
    Array.from(container.querySelectorAll('[data-message-node]'))
      .map((el) => Number(el.getAttribute('data-sibling-index')))
      .filter((value) => !Number.isNaN(value));

  it('renderiza apenas um subconjunto das mensagens em conversas longas', () => {
    const total = 120;
    const { container } = render(<MessageList threadedMessages={createNodes(total)} />);

    const rendered = screen.getAllByTestId('message-node');
    expect(rendered.length).toBeGreaterThan(0);
    // Virtualizado: muito menos itens no DOM do que o total.
    expect(rendered.length).toBeLessThan(total / 2);

    const indexes = renderedIndexes(container);
    // O topo da lista está visível ao montar (auto-scroll mantém o fim, mas o
    // virtualizer ainda materializa apenas uma faixa contígua de itens).
    expect(Math.min(...indexes)).toBeGreaterThanOrEqual(0);
  });

  it('não virtualiza conversas curtas e fornece a mesma navegação entre linhas', () => {
    hoisted.messageNodeMock.mockClear();
    const total = 5;
    render(<MessageList threadedMessages={createNodes(total)} />);

    expect(screen.getAllByTestId('message-node')).toHaveLength(total);
    // Os wrappers são iguais nos dois modos; a lista resolve o irmão correto.
    const calls = hoisted.messageNodeMock.mock.calls.map(([props]) => props);
    expect(calls.every((props) => typeof props.onFocusSiblingIndex === 'function')).toBe(true);
  });

  it('injeta callback de foco virtualizado para os nós renderizados', () => {
    hoisted.messageNodeMock.mockClear();
    render(<MessageList threadedMessages={createNodes(120)} />);

    const calls = hoisted.messageNodeMock.mock.calls.map(([props]) => props);
    expect(calls.length).toBeGreaterThan(0);
    expect(calls.every((props) => typeof props.onFocusSiblingIndex === 'function')).toBe(true);
  });

  it('materializa itens fora da viewport ao rolar (janela acompanha o scroll)', () => {
    const total = 120;
    const { container } = render(<MessageList threadedMessages={createNodes(total)} />);

    const scrollContainer = screen.getByLabelText('chat.messageListLabel');
    Object.defineProperty(scrollContainer, 'scrollTop', {
      configurable: true,
      value: ITEM_HEIGHT * 60,
    });
    fireEvent.scroll(scrollContainer);

    const indexes = renderedIndexes(container);
    // Após rolar bastante para baixo, itens próximos ao índice 60 são montados
    // e o item 0 deixa de existir no DOM — confirmando o janelamento.
    expect(Math.max(...indexes)).toBeGreaterThanOrEqual(40);
    expect(indexes).not.toContain(0);
  });

  it('não quebra Ctrl+Home/Ctrl+End com lista virtualizada', () => {
    const onJumpToStart = vi.fn();
    const onJumpToEnd = vi.fn();
    render(
      <MessageList
        threadedMessages={createNodes(120)}
        onJumpToStart={onJumpToStart}
        onJumpToEnd={onJumpToEnd}
      />
    );

    const list = screen.getByRole('list');
    fireEvent.keyDown(list, { key: 'Home', ctrlKey: true });
    fireEvent.keyDown(list, { key: 'End', ctrlKey: true });

    expect(onJumpToStart).toHaveBeenCalledTimes(1);
    expect(onJumpToEnd).toHaveBeenCalledTimes(1);
  });

  it.each(['Home', 'End'])('salto recusado por Ctrl+%s mantém foco e consome a rejeição', async key => {
    const refuse = vi.fn(async () => { throw new Error('navigation refused'); });
    const { container } = render(<MessageList threadedMessages={createNodes(3)} onJumpToStart={refuse} onJumpToEnd={refuse} />);
    const middle = container.querySelector('[data-sibling-index="1"]') as HTMLElement;
    act(() => middle.focus());
    await act(async () => { fireEvent.keyDown(middle, { key, ctrlKey: true }); });
    await act(async () => { await vi.advanceTimersByTimeAsync(100); });
    expect(refuse).toHaveBeenCalledOnce();
    expect(middle).toHaveFocus();
  });

  it('Ctrl+End materializa e foca o último item mesmo com outra mensagem retida', async () => {
    const { container } = render(<MessageList threadedMessages={createNodes(120)} onJumpToEnd={async () => {}} />);
    const scrollContainer = container.querySelector('.message-list') as HTMLElement;
    Object.defineProperty(scrollContainer, 'scrollHeight', { configurable: true, value: ITEM_HEIGHT * 120 });
    Object.defineProperty(scrollContainer, 'clientHeight', { configurable: true, value: VIEWPORT_HEIGHT });
    const scroll = vi.fn((options?: ScrollToOptions | number, y?: number) => {
      scrollContainer.scrollTop = typeof options === 'object' ? options.top ?? 0 : y ?? 0;
      fireEvent.scroll(scrollContainer);
    });
    scrollContainer.scrollTo = scroll;
    const first = container.querySelector('[data-sibling-index="0"]') as HTMLElement;
    act(() => first.focus());
    expect(renderedIndexes(container)).not.toContain(119);
    await act(async () => { fireEvent.keyDown(first, { key: 'End', ctrlKey: true }); });
    await act(async () => { await vi.advanceTimersByTimeAsync(100); });
    expect(document.activeElement).toHaveAttribute('data-sibling-index', '119');
    expect(scroll).toHaveBeenCalled();
  });

  it('faz auto-scroll para o fim ao receber nova mensagem', () => {
    const scrollSpy = vi.spyOn(window.HTMLElement.prototype, 'scrollIntoView');
    const initial = createNodes(2);
    const { rerender } = render(<MessageList threadedMessages={initial} />);

    scrollSpy.mockClear();
    rerender(<MessageList threadedMessages={createNodes(3)} />);

    expect(scrollSpy).toHaveBeenCalled();
    scrollSpy.mockRestore();
  });

  it('virtualiza corretamente quando recebe um callback ref externo', () => {
    let externalNode: HTMLElement | null = null;
    const callbackRef = (node: HTMLDivElement | null) => {
      externalNode = node;
    };
    const total = 120;
    render(<MessageList ref={callbackRef} threadedMessages={createNodes(total)} />);

    // O callback ref externo recebe o elemento de scroll real.
    expect(externalNode).toBeInstanceOf(HTMLElement);
    expect((externalNode as HTMLElement | null)?.classList.contains('message-list')).toBe(true);

    // A virtualização continua funcionando (apenas um subconjunto é renderizado),
    // provando que getScrollElement usa o ref interno como fonte de verdade
    // mesmo com um callback ref externo (cujo `.current` não existe).
    const rendered = screen.getAllByTestId('message-node');
    expect(rendered.length).toBeGreaterThan(0);
    expect(rendered.length).toBeLessThan(total / 2);
  });

  it('encaminha o nó para um RefObject externo mantendo a virtualização', () => {
    const externalRef = React.createRef<HTMLDivElement>();
    const total = 120;
    render(<MessageList ref={externalRef} threadedMessages={createNodes(total)} />);

    expect(externalRef.current).toBeInstanceOf(HTMLElement);
    expect(externalRef.current?.classList.contains('message-list')).toBe(true);

    const rendered = screen.getAllByTestId('message-node');
    expect(rendered.length).toBeLessThan(total / 2);
  });
});
