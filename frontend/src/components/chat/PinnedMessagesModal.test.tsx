import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { chat, database } from '../../../wailsjs/go/models';
import { PinnedMessagesModal } from './PinnedMessagesModal';

const { getPinnedMessages, toggleMessagePin, announce, eventHandlers } = vi.hoisted(() => ({
  getPinnedMessages: vi.fn(),
  toggleMessagePin: vi.fn(),
  announce: vi.fn(),
  eventHandlers: new Map<string, (data: unknown) => void>(),
}));

vi.mock('@wailsjs/go/wailsapi/Conversations', () => ({
  GetPinnedMessages: (...args: unknown[]) => getPinnedMessages(...args),
  ToggleMessagePin: (...args: unknown[]) => toggleMessagePin(...args),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (name: string, handler: (data: unknown) => void) => {
    eventHandlers.set(name, handler);
    return () => eventHandlers.delete(name);
  },
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce }),
}));

describe('PinnedMessagesModal', () => {
  beforeEach(() => {
    eventHandlers.clear();
    getPinnedMessages.mockReset();
    toggleMessagePin.mockReset();
    announce.mockReset();
  });

  it('lista mensagens fixadas e permite desafixar', async () => {
    getPinnedMessages.mockResolvedValue([
      database.ChatMessage.createFrom({
        id: 'message-1',
        conversationId: 'conversation-1',
        role: 'assistant',
        content: 'Resposta importante',
        pinned: true,
      }),
    ]);
    toggleMessagePin.mockResolvedValue(chat.EnrichedMessage.createFrom({
      id: 'message-1',
      conversationId: 'conversation-1',
      role: 'assistant',
      content: 'Resposta importante',
      pinned: false,
    }));
    const user = userEvent.setup();

    render(
      <PinnedMessagesModal
        conversationId="conversation-1"
        isOpen
        onClose={vi.fn()}
      />,
    );

    expect(await screen.findByText('Resposta importante')).toBeInTheDocument();
    const unpin = screen.getByRole('button', { name: 'chat.unpinMessage' });
    await user.click(unpin);

    expect(toggleMessagePin).toHaveBeenCalledWith('message-1');
    expect(announce).toHaveBeenCalledWith('chat.announce.messageUnpinned');
  });

  it('recarrega somente para evento da conversa exibida', async () => {
    getPinnedMessages.mockResolvedValue([]);
    render(
      <PinnedMessagesModal
        conversationId="conversation-1"
        isOpen
        onClose={vi.fn()}
      />,
    );
    await waitFor(() => expect(getPinnedMessages).toHaveBeenCalledTimes(1));

    eventHandlers.get('message:pin_changed')?.({ conversationId: 'other' });
    expect(getPinnedMessages).toHaveBeenCalledTimes(1);
    eventHandlers.get('message:pin_changed')?.({ conversationId: 'conversation-1' });
    await waitFor(() => expect(getPinnedMessages).toHaveBeenCalledTimes(2));
  });
});
