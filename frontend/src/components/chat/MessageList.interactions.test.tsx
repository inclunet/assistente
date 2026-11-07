import { invocationFixture } from '../../test/chatPresentationFixture';
import { nodeFixture } from '../../test/chatPresentationFixture';
import type { Message, MessageNode } from '../../lib/chatMessageTree';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MessageList, type MessageListProps } from './MessageList';
import { useMessageInteraction, useActiveMessageInteraction } from './MessageInteractionContext';
import { createContext, useContext, useEffect, useRef, useState } from 'react';
import { useChatStore } from '../../store/chatStore';
import { ToolCallsSection } from './ToolCallsSection';
import { ContextMenu } from '../menu';
import { useContextMenu } from '../../hooks/useContextMenu';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../../hooks/useAnnouncer', () => ({ announce: vi.fn(), useAnnouncer: () => ({ announceRequest: () => true }) }));
vi.mock('../../lib/messageMenuItems', () => ({ getMessageMenuItems: (message: Message, options: { onEdit?: (message: Message) => void }) => [
  { id: 'copy', label: 'Copiar' }, { id: 'edit', label: 'Editar', action: () => options.onEdit?.(message) },
] }));
const Editing = createContext<string | null>(null);
vi.mock('./MessageNode', () => ({
  MessageNode: function Row({ node, onContextMenu }: { node: MessageNode; onContextMenu: MessageListProps['onContextMenu'] }) {
    const retain = useMessageInteraction();
    const editing = useContext(Editing) === node.message.id;
    useActiveMessageInteraction(editing);
    const editor = useRef<HTMLTextAreaElement>(null);
    useEffect(() => {
      if (editing) {
        const frame = requestAnimationFrame(() => editor.current?.focus());
        return () => cancelAnimationFrame(frame);
      }
    }, [editing]);
    return <div data-testid={node.message.id} tabIndex={-1}>
      {editing && <textarea ref={editor} aria-label="edição" defaultValue={node.message.content} />}
      <button onClick={event => { event.stopPropagation(); onContextMenu?.(event, node.message, retain); }}>Menu</button>
      <ToolCallsSection tabNavigationEnabled toolInvocations={[invocationFixture({ callId: node.message.id, name: 'echo', status: 'done', outputPreview: node.message.content })]} />
    </div>;
  },
}));

const nodes = (count: number, prefix = 'item') => Array.from({ length: count }, (_, i) => nodeFixture({
  message: { id: `${prefix}-${i}`, conversationId: 'conversation', role: 'user', content: `texto ${i}`,
    timestamp: i, createdAt: '2026-01-01T00:00:00Z' }, children: [], childCount: 0,
}));

function Surface({ items }: { items: MessageNode[] }) {
  const [editing, setEditing] = useState<string | null>(null);
  const menu = useContextMenu({ sessionKey: 'test', onEdit: message => setEditing(message.id) });
  return <><input aria-label="compositor" /><Editing.Provider value={editing}><MessageList threadedMessages={items}
    onContextMenu={(event, message, retain) => menu.showMenu(event, message, true, retain)} />
    </Editing.Provider>
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
  it('transfere menu para edição quando o timer termina antes do autofocus', async () => {
    const initial = nodes(40);
    const view = render(<Surface items={initial} />);
    const row = screen.getByTestId('item-39');
    const trigger = within(row).getByRole('button', { name: 'Menu' });
    act(() => trigger.focus());
    fireEvent.click(trigger);
    await screen.findByRole('menu');
    view.rerender(<Surface items={[...nodes(100, 'older'), ...initial]} />);
    const frames: FrameRequestCallback[] = [];
    const frame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => { frames.push(callback); return frames.length; });
    const skip = vi.spyOn(useChatStore.getState(), 'consumeSkipFocusRestore').mockReturnValue(true);
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
    try {
      fireEvent.click(screen.getByRole('menuitem', { name: 'Editar' }));
      act(() => vi.advanceTimersByTime(11));
      const editor = screen.getByRole('textbox', { name: 'edição' });
      expect(editor).not.toHaveFocus();
      expect(screen.getByTestId('item-39')).toBe(row);
      act(() => frames.forEach(callback => callback(16)));
      expect(editor).toHaveFocus();
      expect(editor).toHaveValue('texto 39');
    } finally { vi.useRealTimers(); frame.mockRestore(); skip.mockRestore(); }
  });
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
