import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
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
  it('renderiza estado vazio', () => {
    render(<MessageList threadedMessages={[]} />);

    expect(screen.getByText('chat.emptyTitle')).toBeInTheDocument();
  });

  it('renderiza mensagens e loading', () => {
    const node = main.MessageNode.createFrom({
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
      childCount: 0,
      level: 0,
      children: [],
    });
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
    const node = main.MessageNode.createFrom({
      message: new main.EnrichedMessage({
        id: 'message-42',
        conversationId: 'conversation-1',
        role: 'user',
        content: 'Mensagem',
        createdAt: new Date().toISOString(),
        timestamp: Date.now(),
        isStreaming: false,
        internal: false,
      }),
      childCount: 0,
      level: 0,
      children: [],
    });
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
});
