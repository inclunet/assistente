import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render } from '@testing-library/react';
import { MessageList } from './MessageList';
import { chat } from '../../../wailsjs/go/models';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announceRequest: () => true }) }));
// Controles reais do DOM isolam a identidade da linha e do descendente focado.
// Não simulamos focusout para induzir uma restauração que o navegador não faria.
vi.mock('./MessageNode', () => ({
  MessageNode: ({ node, siblingIndex }: { node: chat.MessageNode; siblingIndex: number }) => (
    <div tabIndex={-1} data-message-node="" data-level="0" data-sibling-index={siblingIndex} data-testid={node.message.id}>
      <span>{node.message.content}</span>
      <textarea aria-label={`edit-${node.message.id}`} defaultValue="rascunho local" />
      <div data-message-node="" data-level="1"><button type="button">{`tool-${node.message.id}`}</button></div>
    </div>
  ),
}));

const nodes = (count: number, prefix = 'item') => Array.from({ length: count }, (_, i) => chat.MessageNode.createFrom({
  message: { id: `${prefix}-${i}`, conversationId: 'conversation', role: 'user', content: `texto ${i}`,
    timestamp: i, createdAt: '2026-01-01T00:00:00Z' }, children: [], childCount: 0, level: 0,
}));

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

describe('identidade e retenção visual da lista', () => {
  it.each(['append', 'prepend', 'prepend-new-focus'])('preserva posição com alturas variadas em 40→41→40 (%s)', direction => {
    vi.useFakeTimers();
    const originalRect = HTMLElement.prototype.getBoundingClientRect;
    const rect = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function(this: HTMLElement) {
      if (!this.hasAttribute('data-timeline-key')) return originalRect.call(this);
      const container = this.closest('.message-list') as HTMLElement;
      const index = Number(this.dataset.index);
      const naturalTop = Array.from({ length: index }, (_, i) => i % 2 ? 220 : 80).reduce((a, b) => a + b, 0);
      const top = (this.style.position === 'absolute' ? Number.parseFloat(this.style.top) : naturalTop) - container.scrollTop;
      return { top, bottom: top + 140, height: 140, left: 0, right: 500, width: 500, x: 0, y: top, toJSON() {} };
    });
    try {
      const initial = nodes(40);
      const loadOlder = vi.fn(() => new Promise<void>(() => {}));
      const content = (items: chat.MessageNode[]) => <MessageList threadedMessages={items} hasOlderMessages onLoadOlder={loadOlder} />;
      const view = render(content(initial));
      const prepending = direction.startsWith('prepend');
      const target = prepending ? 'item-1' : 'item-39';
      let row = view.getByTestId(target).parentElement!;
      const container = view.container.querySelector('.message-list') as HTMLElement;
      Object.defineProperty(container, 'scrollHeight', { configurable: true, get: () =>
        Number.parseFloat(view.getByRole('list').style.height) || 6000 });
      let offset = prepending ? 75 : 120;
      container.scrollTop = row.getBoundingClientRect().top - offset;
      act(() => view.getByTestId(target).focus());
      act(() => vi.advanceTimersByTime(600));
      if (prepending) {
        fireEvent.scroll(container);
        expect(loadOlder).not.toHaveBeenCalled();
      }
      expect(row.getBoundingClientRect().top).toBe(offset);
      const next = content(prepending ? [...nodes(1, 'extra'), ...initial] : [...initial, ...nodes(1, 'extra')]);
      if (direction === 'prepend-new-focus') {
        const nextFocus = view.getByTestId('item-2');
        row = nextFocus.parentElement!;
        offset = row.getBoundingClientRect().top;
        act(() => { nextFocus.focus(); view.rerender(next); });
      } else view.rerender(next);
      expect(row.getBoundingClientRect().top).toBe(offset);
      view.rerender(content(initial));
      expect(row.getBoundingClientRect().top).toBe(offset);
    } finally { rect.mockRestore(); vi.useRealTimers(); }
  });
  it.each(['row', 'editor', 'thread-control'])('preserva o mesmo DOM ao cruzar 40→41→40, foco=%s', target => {
    const initial = nodes(40);
    const view = render(<MessageList threadedMessages={initial} />);
    const row = view.getByTestId('item-39');
    const focused = target === 'row' ? row : target === 'editor'
      ? view.getByRole('textbox', { name: 'edit-item-39' }) : view.getByRole('button', { name: 'tool-item-39' });
    act(() => focused.focus());
    const editor = view.getByRole('textbox', { name: 'edit-item-39' }) as HTMLTextAreaElement;
    fireEvent.change(editor, { target: { value: 'edição ainda não salva' } });
    editor.setSelectionRange(3, 7);
    view.rerender(<MessageList threadedMessages={[...initial, ...nodes(1, 'extra')]} />);
    expect(view.getByTestId('item-39')).toBe(row);
    expect(document.activeElement).toBe(focused);
    expect(editor.value).toBe('edição ainda não salva');
    expect(editor.selectionStart).toBe(3);
    expect(view.container.querySelectorAll('[data-level="0"]').length).toBeLessThan(30);
    view.rerender(<MessageList threadedMessages={initial} />);
    expect(view.getByTestId('item-39')).toBe(row);
    expect(document.activeElement).toBe(focused);
    expect(editor.selectionEnd).toBe(7);
  });

  it('prepend e conclusão preservam a identidade focada fora da faixa visível', () => {
    const initial = nodes(40);
    initial[39].message.role = 'assistant'; initial[39].message.turnId = 'question'; initial[39].message.isStreaming = true;
    const view = render(<MessageList threadedMessages={initial} />);
    const row = view.getByTestId('item-39');
    act(() => row.focus());
    const finished = chat.MessageNode.createFrom({ ...initial[39], message: { ...initial[39].message, content: 'final', isStreaming: false } });
    view.rerender(<MessageList threadedMessages={[...nodes(100, 'older'), ...initial.slice(0, 39), finished]} />);
    expect(view.getByTestId('item-39')).toBe(row);
    expect(row).toHaveAttribute('data-sibling-index', '139');
    expect(row).toHaveTextContent('final');
    expect(document.activeElement).toBe(row);
  });

  it('saída para o compositor libera a retenção sem recuperar foco nem acumular linhas', () => {
    const initial = nodes(40);
    const surface = (items: chat.MessageNode[]) => <><input aria-label="compositor" /><MessageList threadedMessages={items} /></>;
    const view = render(surface(initial));
    act(() => view.getByTestId('item-39').focus());
    view.rerender(surface([...initial, ...nodes(100, 'extra')]));
    expect(view.getByTestId('item-39')).toBeInTheDocument();
    const input = view.getByRole('textbox', { name: 'compositor' });
    act(() => input.focus());
    expect(document.activeElement).toBe(input);
    expect(view.queryByTestId('item-39')).not.toBeInTheDocument();
    expect(view.container.querySelectorAll('[data-level="0"]').length).toBeLessThan(30);
  });

  it('remoção do item não transfere foco para quem ocupou seu índice', () => {
    const initial = nodes(3);
    const view = render(<MessageList threadedMessages={initial} />);
    act(() => view.getByTestId('item-1').focus());
    view.rerender(<MessageList threadedMessages={[initial[0], initial[2]]} />);
    expect(document.activeElement).toBe(document.body);
    expect(view.getByTestId('item-2')).not.toHaveFocus();
  });
});
