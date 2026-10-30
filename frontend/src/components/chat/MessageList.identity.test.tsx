import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render } from '@testing-library/react';
import { MessageList } from './MessageList';
import { chat } from '../../../wailsjs/go/models';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announceRequest: () => true }),
}));
// Isola a identidade do container da lista, sem simular focusout.
// Não comprova o comportamento do Markdown ou do cursor virtual do NVDA.
vi.mock('./MessageNode', () => ({
  MessageNode: ({ node, siblingIndex }: { node: chat.MessageNode; siblingIndex: number }) => (
    <div tabIndex={-1} data-message-node="" data-level="0"
      data-sibling-index={siblingIndex} data-testid={node.message.id}>
      {node.message.content}
    </div>
  ),
}));

afterEach(cleanup);

const response = (turnId: string | undefined, streaming: boolean) => chat.MessageNode.createFrom({
  message: { id: 'assistant', conversationId: 'conversation', role: 'assistant',
    content: streaming ? 'parcial' : 'final', turnId, isStreaming: streaming,
    timestamp: 100, createdAt: '2026-01-01T00:00:00Z' },
  children: [], childCount: 0, level: 0,
});

describe('AEP-0116 — identidade visual na conclusão', () => {
  it.fails('mantém foco ao receber o turno tardiamente', () => {
    const view = render(<MessageList threadedMessages={[response(undefined, true)]} />);
    const element = view.getByTestId('assistant');
    element.focus();
    expect(document.activeElement).toBe(element);
    view.rerender(<MessageList threadedMessages={[response('user', false)]} />);
    expect(document.activeElement).toBe(view.getByTestId('assistant'));
  });

  it('preserva o mesmo elemento e foco quando o turno existe desde o início', () => {
    const view = render(<MessageList threadedMessages={[response('user', true)]} />);
    const element = view.getByTestId('assistant');
    element.focus();
    view.rerender(<MessageList threadedMessages={[response('user', false)]} />);
    expect(view.getByTestId('assistant')).toBe(element);
    expect(document.activeElement).toBe(element);
  });

  it('não desloca foco que já saiu intencionalmente para o compositor', () => {
    const surface = (streaming: boolean) => <>
      <input aria-label="Compositor" />
      <MessageList threadedMessages={[response('user', streaming)]} />
    </>;
    const view = render(surface(true));
    const input = view.getByRole('textbox');
    input.focus();
    view.rerender(surface(false));
    expect(document.activeElement).toBe(input);
  });
});
