import React from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, render } from '@testing-library/react';
import { MessageList } from './MessageList';
import { chat } from '../../../wailsjs/go/models';

const renderMessageNode = vi.hoisted(() => vi.fn());

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announceRequest: vi.fn(() => true) }),
}));

vi.mock('./MessageNode', () => ({
  MessageNode: React.memo((props: { node: chat.MessageNode; onReachStart?: () => void; onReachEnd?: () => void }) => {
    renderMessageNode(props);
    return <div data-message-node={String(props.node.message.id)} />;
  }),
}));

const createNode = (id = 'message-1') => chat.MessageNode.createFrom({
  message: new chat.EnrichedMessage({
    id,
    conversationId: 'conversation-1',
    role: 'assistant',
    content: 'resposta',
    createdAt: new Date().toISOString(),
    timestamp: Date.now(),
    isStreaming: false,
    internal: false,
  }),
  childCount: 0,
  level: 0,
  children: [],
});

describe('MessageList performance', () => {
  let restoreDimensions: () => void;
  beforeEach(() => {
    vi.useFakeTimers();
    renderMessageNode.mockClear();
    const proto = window.HTMLElement.prototype;
    const original = Object.getOwnPropertyDescriptor(proto, 'offsetHeight');
    Object.defineProperty(proto, 'offsetHeight', {
      configurable: true,
      get(this: HTMLElement) {
        if (this.classList.contains('message-list')) return 700;
        if (this.hasAttribute('data-index')) return 140;
        return 0;
      },
    });
    restoreDimensions = () => {
      if (original) Object.defineProperty(proto, 'offsetHeight', original);
      else delete (proto as unknown as Record<string, unknown>).offsetHeight;
    };
  });
  afterEach(() => {
    cleanup();
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
    restoreDimensions();
  });

  it('não re-renderiza nós estáveis quando só o estado do loading muda', () => {
    const node = createNode();
    const onReachEnd = vi.fn();
    const { rerender } = render(
      <MessageList
        threadedMessages={[node]}
        isLoading={false}
        onReachEnd={onReachEnd}
      />,
    );
    expect(renderMessageNode).toHaveBeenCalled();
    renderMessageNode.mockClear();

    for (let index = 0; index < 10; index += 1) {
      rerender(
        <MessageList
          threadedMessages={[node]}
          isLoading={index % 2 === 0}
          onReachEnd={onReachEnd}
        />,
      );
    }

    expect(renderMessageNode).toHaveBeenCalledTimes(0);
  });

  it.each([100, 500, 1000])('mantém a janela virtualizada estável com %i nós', (size) => {
    const nodes = Array.from({ length: size }, (_, index) => createNode(`message-${index}`));
    const onReachEnd = vi.fn();
    const { rerender } = render(
      <MessageList
        threadedMessages={nodes}
        isLoading={false}
        onReachEnd={onReachEnd}
      />,
    );
    act(() => vi.advanceTimersByTime(0));
    expect(renderMessageNode.mock.calls.length).toBeGreaterThan(0);
    expect(renderMessageNode.mock.calls.length).toBeLessThan(size);
    renderMessageNode.mockClear();

    for (let i = 0; i < 10; i++) {
      rerender(<MessageList threadedMessages={[...nodes]} isLoading={i % 2 === 0} onReachEnd={onReachEnd} />);
      act(() => vi.advanceTimersByTime(0));
    }

    expect(renderMessageNode).toHaveBeenCalledTimes(0);
  });

  it('atualiza somente o nó cujo conteúdo mudou', () => {
    const first = createNode('first');
    const second = createNode('second');
    const { rerender } = render(<MessageList threadedMessages={[first, second]} />);
    expect(renderMessageNode).toHaveBeenCalledTimes(2);
    renderMessageNode.mockClear();
    const changed = chat.MessageNode.createFrom({ ...second, message: { ...second.message, content: 'novo conteúdo' } });
    rerender(<MessageList threadedMessages={[first, changed]} />);
    expect(renderMessageNode).toHaveBeenCalledTimes(1);
    expect(renderMessageNode.mock.calls[0][0].node).toBe(changed);
  });

  it('callbacks de fronteira usam a paginação e ações atuais', () => {
    const nodes = [createNode()];
    const oldStart = vi.fn();
    const nextStart = vi.fn();
    const nextEnd = vi.fn();
    const onLoadOlder = vi.fn();
    const onLoadNewer = vi.fn();
    const { rerender } = render(<MessageList threadedMessages={nodes} onReachStart={oldStart} />);
    rerender(<MessageList threadedMessages={nodes} onReachStart={nextStart} onReachEnd={nextEnd} />);
    let props = renderMessageNode.mock.lastCall![0];
    act(() => { props.onReachStart(); props.onReachEnd(); });
    expect(oldStart).not.toHaveBeenCalled();
    expect(nextStart).toHaveBeenCalledTimes(1);
    expect(nextEnd).toHaveBeenCalledTimes(1);

    rerender(<MessageList threadedMessages={nodes} hasOlderMessages hasNewerMessages onLoadOlder={onLoadOlder} onLoadNewer={onLoadNewer} />);
    props = renderMessageNode.mock.lastCall![0];
    act(() => { props.onReachStart(); props.onReachEnd(); });
    expect(onLoadOlder).toHaveBeenCalledWith('navigation');
    expect(onLoadNewer).toHaveBeenCalledWith('navigation');

    onLoadOlder.mockClear();
    onLoadNewer.mockClear();
    rerender(<MessageList threadedMessages={nodes} hasOlderMessages hasNewerMessages isLoadingMessageWindow onLoadOlder={onLoadOlder} onLoadNewer={onLoadNewer} />);
    props = renderMessageNode.mock.lastCall![0];
    act(() => { props.onReachStart(); props.onReachEnd(); });
    expect(onLoadOlder).not.toHaveBeenCalled();
    expect(onLoadNewer).not.toHaveBeenCalled();
  });
});
