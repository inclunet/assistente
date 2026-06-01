import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { MessageList } from './MessageList';
import { chat } from '../../../wailsjs/go/models';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const hoisted = vi.hoisted(() => ({
  messageNodeMock: vi.fn(),
}));
vi.mock('./MessageNode', () => ({
  MessageNode: (props: unknown) => {
    hoisted.messageNodeMock(props);
    return <div data-testid="message-node" />;
  },
}));

describe('MessageList', () => {
  const createNode = (id = '1') => chat.MessageNode.createFrom({
    message: new chat.EnrichedMessage({
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

  it('renderiza mensagens e loading', () => {
    const node = createNode();
    render(
      <MessageList
        threadedMessages={[node]}
        isLoading
      />
    );

    expect(screen.getByTestId('message-node')).toBeInTheDocument();
    expect(screen.getByRole('status')).toBeInTheDocument();
  });

  it('repassa posição absoluta e tamanho total para itens renderizados', () => {
    hoisted.messageNodeMock.mockClear();
    const node = createNode('message-42');
    (node as typeof node & { originalIndex?: number }).originalIndex = 41;

    render(
      <MessageList
        threadedMessages={[node]}
        messageWindow={{
          scope: 'conversation',
          conversationId: 'conversation-1',
          totalCount: 100,
          startIndex: 41,
          endIndex: 41,
          hasBefore: true,
          hasAfter: true,
        }}
      />
    );

    expect(hoisted.messageNodeMock).toHaveBeenCalledWith(expect.objectContaining({
      ariaPosition: 42,
      ariaSetSize: 100,
    }));
  });

  it('usa contagem visual local quando algum item não tem índice absoluto', () => {
    hoisted.messageNodeMock.mockClear();
    const firstNode = createNode('message-1');
    const expandedTurnNode = createNode('message-expanded');
    const thirdNode = createNode('message-3');
    (firstNode as typeof firstNode & { originalIndex?: number }).originalIndex = 40;
    (thirdNode as typeof thirdNode & { originalIndex?: number }).originalIndex = 41;

    render(
      <MessageList
        threadedMessages={[firstNode, expandedTurnNode, thirdNode]}
        messageWindow={{
          scope: 'conversation',
          conversationId: 'conversation-1',
          totalCount: 100,
          startIndex: 40,
          endIndex: 41,
          hasBefore: true,
          hasAfter: true,
        }}
      />
    );

    const calls = hoisted.messageNodeMock.mock.calls.map(([props]) => props);
    expect(calls).toEqual(expect.arrayContaining([
      expect.objectContaining({ ariaPosition: 1, ariaSetSize: 3 }),
      expect.objectContaining({ ariaPosition: 2, ariaSetSize: 3 }),
      expect.objectContaining({ ariaPosition: 3, ariaSetSize: 3 }),
    ]));
  });

  it('usa posições canônicas quando o backend retorna item de turno consolidado', () => {
    hoisted.messageNodeMock.mockClear();
    const userNode = createNode('user-1');
    const turnNode = createNode('assistant-final');
    turnNode.message.role = 'assistant';
    turnNode.message.turnId = 'user-1';
    turnNode.message.toolCalls = JSON.stringify([{ id: 'tool-1', result: 'ok' }]);
    (userNode as typeof userNode & { originalIndex?: number }).originalIndex = 0;
    (turnNode as typeof turnNode & { originalIndex?: number }).originalIndex = 1;

    render(
      <MessageList
        threadedMessages={[userNode, turnNode]}
        messageWindow={{
          scope: 'conversation',
          conversationId: 'conversation-1',
          totalCount: 2,
          startIndex: 0,
          endIndex: 1,
          hasBefore: false,
          hasAfter: false,
        }}
      />
    );

    const calls = hoisted.messageNodeMock.mock.calls.map(([props]) => props);
    expect(calls).toEqual(expect.arrayContaining([
      expect.objectContaining({ ariaPosition: 1, ariaSetSize: 2 }),
      expect.objectContaining({ ariaPosition: 2, ariaSetSize: 2 }),
    ]));
  });

  it('usa o último assistant como representante ao consolidar turno local', () => {
    hoisted.messageNodeMock.mockClear();
    const firstAssistant = createNode('assistant-1');
    firstAssistant.message.role = 'assistant';
    firstAssistant.message.turnId = 'turn-1';
    firstAssistant.message.content = 'intermediário';
    const secondAssistant = createNode('assistant-2');
    secondAssistant.message.role = 'assistant';
    secondAssistant.message.turnId = 'turn-1';
    secondAssistant.message.content = '';
    secondAssistant.message.toolCalls = JSON.stringify([{ id: 'tool-1' }]);
    const thirdAssistant = createNode('assistant-3');
    thirdAssistant.message.role = 'assistant';
    thirdAssistant.message.turnId = 'turn-1';
    thirdAssistant.message.content = '';
    thirdAssistant.message.toolCalls = JSON.stringify([{ id: 'tool-2' }]);

    render(<MessageList threadedMessages={[firstAssistant, secondAssistant, thirdAssistant]} />);

    const props = hoisted.messageNodeMock.mock.calls[0][0] as { node: chat.MessageNode };
    expect(hoisted.messageNodeMock).toHaveBeenCalledTimes(1);
    expect(props.node.message.id).toBe('assistant-3');
    expect(props.node.message.content).toBe('intermediário');
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
      { type: 'tool_calls', toolCalls: [{ id: 'tool-1', type: 'function', function: { name: 'search', arguments: '{}' } }] },
      { type: 'text', content: 'resposta final' },
    ];

    render(<MessageList threadedMessages={[userNode, turnNode]} />);

    expect(hoisted.messageNodeMock).toHaveBeenCalledTimes(2);
    const turnProps = hoisted.messageNodeMock.mock.calls[1][0] as {
      node: chat.MessageNode & { message: chat.EnrichedMessage & { turnSegments?: unknown[] } };
    };
    expect(turnProps.node.message.id).toBe('assistant-final');
    expect(turnProps.node.message.turnSegments).toEqual([
      { type: 'text', content: 'intermediário' },
      { type: 'tool_calls', toolCalls: [{ id: 'tool-1', type: 'function', function: { name: 'search', arguments: '{}' } }] },
      { type: 'text', content: 'resposta final' },
    ]);
  });

  it('preserva segmentos canônicos ao consolidar nó de backend com transitório do mesmo turno', () => {
    hoisted.messageNodeMock.mockClear();
    const canonicalNode = createNode('assistant-canonical');
    canonicalNode.message.role = 'assistant';
    canonicalNode.message.turnId = 'turn-1';
    canonicalNode.message.content = 'resposta canônica';
    (canonicalNode.message as typeof canonicalNode.message & { _turnSegments?: unknown })._turnSegments = [
      { type: 'text', content: 'segmento canônico' },
      { type: 'tool_calls', toolCalls: [{ id: 'tool-1', type: 'function', function: { name: 'search', arguments: '{}' } }] },
    ];
    const streamingNode = createNode('streaming-turn-1');
    streamingNode.message.role = 'assistant';
    streamingNode.message.turnId = 'turn-1';
    streamingNode.message.content = 'continuação transitória';
    streamingNode.message.isStreaming = true;

    render(<MessageList threadedMessages={[canonicalNode, streamingNode]} />);

    const props = hoisted.messageNodeMock.mock.calls[0][0] as {
      node: chat.MessageNode & { message: chat.EnrichedMessage & { _turnSegments?: unknown[] } };
    };
    expect(hoisted.messageNodeMock).toHaveBeenCalledTimes(1);
    expect(props.node.message.id).toBe('streaming-turn-1');
    expect(props.node.message._turnSegments).toEqual([
      { type: 'text', content: 'segmento canônico' },
      { type: 'tool_calls', toolCalls: [{ id: 'tool-1', type: 'function', function: { name: 'search', arguments: '{}' } }] },
      { type: 'text', content: 'continuação transitória' },
    ]);
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
        hasOlderMessages
        hasNewerMessages
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
  });

  it('não pagina mensagens posteriores a partir de nó de streaming', () => {
    hoisted.messageNodeMock.mockClear();
    const onLoadNewer = vi.fn();
    const onReachEnd = vi.fn();
    const streamingNode = createNode('streaming-conversation-1-1');
    streamingNode.message.isStreaming = true;

    render(
      <MessageList
        threadedMessages={[streamingNode]}
        hasNewerMessages
        onLoadNewer={onLoadNewer}
        onReachEnd={onReachEnd}
      />
    );

    const props = hoisted.messageNodeMock.mock.calls[0][0] as { onReachEnd: () => void };
    props.onReachEnd();

    expect(onLoadNewer).not.toHaveBeenCalled();
    expect(onReachEnd).toHaveBeenCalledTimes(1);
  });
});
