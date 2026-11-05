import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MessageList, type MessageListProps } from './MessageList';
import { useMessageInteraction } from './MessageInteractionContext';
import { ToolCallsSection } from './ToolCallsSection';
import { ContextMenu } from '../menu';
import { useContextMenu } from '../../hooks/useContextMenu';
import { chat } from '../../../wailsjs/go/models';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../../hooks/useAnnouncer', () => ({ announce: vi.fn(), useAnnouncer: () => ({ announceRequest: () => true }) }));
vi.mock('../../lib/messageMenuItems', () => ({ getMessageMenuItems: () => [{ id: 'copy', label: 'Copiar' }] }));
vi.mock('./MessageNode', () => ({
  MessageNode: function Row({ node, onContextMenu }: { node: chat.MessageNode; onContextMenu: MessageListProps['onContextMenu'] }) {
    const retain = useMessageInteraction();
    return <div data-testid={node.message.id} tabIndex={-1}>
      <button onClick={event => { event.stopPropagation(); onContextMenu?.(event, node.message, retain); }}>Menu</button>
      <ToolCallsSection tabNavigationEnabled activeToolCalls={[{ callId: node.message.id, name: 'echo', status: 'done', summary: node.message.content }]} />
    </div>;
  },
}));

const nodes = (count: number, prefix = 'item') => Array.from({ length: count }, (_, i) => chat.MessageNode.createFrom({
  message: { id: `${prefix}-${i}`, conversationId: 'conversation', role: 'user', content: `texto ${i}`,
    timestamp: i, createdAt: '2026-01-01T00:00:00Z' }, children: [], childCount: 0, level: 0,
}));

function Surface({ items }: { items: chat.MessageNode[] }) {
  const menu = useContextMenu({});
  return <><input aria-label="compositor" /><MessageList threadedMessages={items}
    onContextMenu={(event, message, retain) => menu.showMenu(event, message, true, retain)} />
    <ContextMenu visible={menu.menuVisible} items={menu.menuItems} onClose={menu.hideMenu} restoreFocusOnClose={false} />
  </>;
}

let restoreDimensions: () => void;
beforeEach(() => {
  const original = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetHeight');
  Object.defineProperty(HTMLElement.prototype, 'offsetHeight', { configurable: true, get(this: HTMLElement) {
    return this.classList.contains('message-list') ? 700 : this.hasAttribute('data-index') ? 140 : 0;
  } });
  restoreDimensions = () => {
    if (original) Object.defineProperty(HTMLElement.prototype, 'offsetHeight', original);
    else delete (HTMLElement.prototype as unknown as Record<string, unknown>).offsetHeight;
  };
});
afterEach(() => { cleanup(); restoreDimensions(); });

describe('portais pertencentes à mensagem', () => {
  it.each(['menu', 'details'])('retém a origem de %s até restaurar foco, então libera ao sair', async kind => {
    const initial = nodes(40);
    const view = render(<Surface items={initial} />);
    const row = screen.getByTestId('item-39');
    const trigger = within(row).getByRole('button', { name: kind === 'menu' ? 'Menu' : 'chat.technicalDetails' });
    act(() => trigger.focus());
    fireEvent.click(trigger);
    const overlay = await screen.findByRole(kind === 'menu' ? 'menu' : 'dialog');
    await waitFor(() => expect(overlay.contains(document.activeElement)).toBe(true));
    view.rerender(<Surface items={[...nodes(100, 'older'), ...initial]} />);
    expect(screen.getByTestId('item-39')).toBe(row);
    expect(overlay).toBeInTheDocument();
    expect(view.container.querySelectorAll('[data-timeline-key]').length).toBeLessThan(30);
    fireEvent.keyDown(document.activeElement!, { key: 'Escape' });
    await waitFor(() => expect(trigger).toHaveFocus());
    act(() => screen.getByRole('textbox', { name: 'compositor' }).focus());
    await waitFor(() => expect(screen.queryByTestId('item-39')).not.toBeInTheDocument());
  });
});
