import { canonicalTestFixture, canonicalTestItem } from '../../services/chatTimelineTestFixture';
import type { MessageNode } from '../../lib/chatMessageTree';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, render } from '@testing-library/react';
import { MessageList } from './MessageList';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announceRequest: () => true }),
}));
// Isola a identidade do container da lista, sem simular focusout.
// Não comprova o comportamento do Markdown ou do cursor virtual do NVDA.
vi.mock('./MessageNode', () => ({
  MessageNode: ({ node, siblingIndex }: { node: MessageNode; siblingIndex: number }) => (
    <div tabIndex={-1} data-message-node="" data-level="0"
      data-sibling-index={siblingIndex} data-testid={node.message.id}>
      {node.message.content}
    </div>
  ),
}));

const clients: ReturnType<typeof canonicalTestFixture>['client'][] = [];
afterEach(async () => {
  cleanup();
  await Promise.all(clients.splice(0).map(client => client.dispose()));
});

async function responseView(hasTarget = true) {
  const response = canonicalTestItem('turn:user', 'parcial', {
    state: 'streaming', turnId: 'user', representativeMessageId: hasTarget ? 'assistant' : null,
    messageIds: hasTarget ? ['assistant'] : [],
  });
  const fixture = canonicalTestFixture([response]);
  clients.push(fixture.client);
  const presentation = fixture.presentation();
  await presentation.open();
  return { presentation, complete() {
    fixture.publish('surface', { ...response, state: 'completed', representativeMessageId: 'assistant',
      messageIds: ['assistant'], segments: [{ segmentId: 'text', kind: 'text', text: 'final' }] });
  } };
}

describe('AEP-0116 — identidade visual na conclusão', () => {
  it('mantém o elemento e foco quando o backend atribui o alvo persistido', async () => {
    const f = await responseView(false);
    const view = render(<MessageList threadedMessages={f.presentation.nodes()} />);
    const element = view.getByTestId('turn:user');
    act(() => element.focus());
    expect(document.activeElement).toBe(element);
    act(() => f.complete());
    view.rerender(<MessageList threadedMessages={f.presentation.nodes()} />);
    expect(view.getByTestId('turn:user')).toBe(element);
    expect(element).toHaveTextContent('final');
    expect(document.activeElement).toBe(element);
    expect(f.presentation.getMessage('turn:user')?.timelineItem?.representativeMessageId).toBe('assistant');
  });

  it('preserva o mesmo elemento e foco quando o alvo existe desde o início', async () => {
    const f = await responseView();
    const view = render(<MessageList threadedMessages={f.presentation.nodes()} />);
    const element = view.getByTestId('turn:user');
    act(() => element.focus());
    act(() => f.complete());
    view.rerender(<MessageList threadedMessages={f.presentation.nodes()} />);
    expect(view.getByTestId('turn:user')).toBe(element);
    expect(document.activeElement).toBe(element);
  });

  it('não desloca foco que já saiu intencionalmente para o compositor', async () => {
    const f = await responseView();
    const surface = () => <>
      <input aria-label="Compositor" />
      <MessageList threadedMessages={f.presentation.nodes()} />
    </>;
    const view = render(surface());
    const input = view.getByRole('textbox');
    act(() => input.focus());
    act(() => f.complete());
    view.rerender(surface());
    expect(document.activeElement).toBe(input);
  });
});
