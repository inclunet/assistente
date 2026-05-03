import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ChatPanel } from './ChatPanel';
import type { ChatSurfaceOrigin } from '../../services/chatSessionRegistry';

const chatSessionViewMock = vi.fn();

vi.mock('./ChatSessionView', () => ({
  ChatSessionView: (props: {
    variant?: 'page' | 'embedded';
    surfaceType?: string;
    conversationId?: string | null;
    surfaceId?: string;
    sessionKey?: string;
    onSend: (content: string, mediaFiles: undefined, origin: ChatSurfaceOrigin) => Promise<void>;
  }) => {
    chatSessionViewMock(props);
    return (
      <button
        type="button"
        onClick={() => props.onSend('oi', undefined, {
          sessionKey: 'surface-a:conversation-a',
          conversationId: 'conversation-a',
          surfaceId: 'surface-a',
          surfaceType: props.surfaceType === 'modal' ? 'modal' : 'embedded',
        })}
      >
        send
      </button>
    );
  },
}));

describe('ChatPanel', () => {
  beforeEach(() => {
    chatSessionViewMock.mockClear();
  });

  it('expõe contrato declarativo de superfície para a view interna', () => {
    render(
      <ChatPanel
        conversationId="conversation-a"
        surfaceId="surface-a"
        sessionKey="surface-a:conversation-a"
        surfaceType="modal"
        onSend={vi.fn()}
        showShortcutsHelp={false}
      />,
    );

    expect(chatSessionViewMock).toHaveBeenCalledWith(expect.objectContaining({
      conversationId: 'conversation-a',
      sessionKey: 'surface-a:conversation-a',
      surfaceId: 'surface-a',
      showShortcutsHelp: false,
      surfaceType: 'modal',
      variant: 'embedded',
    }));
  });

  it('adapta envio da view para contexto de superfície', async () => {
    const user = userEvent.setup();
    const onSend = vi.fn().mockResolvedValue(undefined);

    render(
      <ChatPanel
        conversationId="conversation-a"
        surfaceId="surface-a"
        surfaceType="embedded"
        onSend={onSend}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'send' }));

    expect(onSend).toHaveBeenCalledWith('oi', undefined, {
      conversationId: 'conversation-a',
      origin: expect.objectContaining({
        conversationId: 'conversation-a',
        sessionKey: 'surface-a:conversation-a',
        surfaceId: 'surface-a',
        surfaceType: 'embedded',
      }),
    });
  });

  it('não expõe conversationId vazio no contexto de envio', async () => {
    const user = userEvent.setup();
    const onSend = vi.fn().mockResolvedValue(undefined);

    render(
      <ChatPanel
        conversationId=""
        surfaceId="surface-a"
        surfaceType="embedded"
        onSend={onSend}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'send' }));

    expect(onSend).toHaveBeenCalledWith('oi', undefined, expect.objectContaining({
      conversationId: 'conversation-a',
    }));
  });
});
