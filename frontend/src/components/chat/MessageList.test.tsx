import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { MessageList } from './MessageList';
import { main } from '../../../wailsjs/go/models';

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
  const createNode = (id = '1') => main.MessageNode.createFrom({
    message: new main.EnrichedMessage({
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
});
