import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MessageList } from './MessageList';
import { main } from '../../../wailsjs/go/models';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('./MessageNode', () => ({
  MessageNode: () => <div data-testid="message-node" />,
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
});
