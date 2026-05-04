import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ChatPanel } from './ChatPanel';
import type { ChatSurfaceIdentity, ChatSurfaceOrigin } from '../../services/chatSessionRegistry';

const chatSessionViewMock = vi.fn();

vi.mock('./ChatSessionView', () => ({
  ChatSessionView: (props: {
    variant?: 'page' | 'embedded';
    surface: ChatSurfaceIdentity;
    onSend: (content: string, mediaFiles: undefined, origin: ChatSurfaceOrigin) => Promise<void>;
  }) => {
    chatSessionViewMock(props);
    return (
      <button
        type="button"
        onClick={() => props.onSend('oi', undefined, {
          sessionKey: props.surface.sessionKey,
          conversationId: props.surface.conversationId ?? 'conversation-a',
          surfaceId: props.surface.surfaceId,
          surfaceType: props.surface.surfaceType,
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
        surface={{
          conversationId: 'conversation-a',
          sessionKey: 'surface-a:conversation-a',
          surfaceId: 'surface-a',
          surfaceType: 'modal',
          tabId: 'tab-chat',
        }}
        onSend={vi.fn()}
        showShortcutsHelp={false}
      />,
    );

    expect(chatSessionViewMock).toHaveBeenCalledWith(expect.objectContaining({
      showShortcutsHelp: false,
      surface: {
        conversationId: 'conversation-a',
        sessionKey: 'surface-a:conversation-a',
        surfaceId: 'surface-a',
        surfaceType: 'modal',
        tabId: 'tab-chat',
      },
      variant: 'embedded',
    }));
  });

  it('adapta envio da view para contexto de superfície', async () => {
    const user = userEvent.setup();
    const onSend = vi.fn().mockResolvedValue(undefined);

    render(
      <ChatPanel
        surface={{
          conversationId: 'conversation-a',
          sessionKey: 'surface-a:conversation-a',
          surfaceId: 'surface-a',
          surfaceType: 'embedded',
          tabId: 'tab-chat',
        }}
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
        surface={{
          conversationId: null,
          sessionKey: 'surface-a:conversation-a',
          surfaceId: 'surface-a',
          surfaceType: 'embedded',
          tabId: 'tab-chat',
        }}
        onSend={onSend}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'send' }));

    expect(onSend).toHaveBeenCalledWith('oi', undefined, expect.objectContaining({
      conversationId: 'conversation-a',
    }));
  });
});
