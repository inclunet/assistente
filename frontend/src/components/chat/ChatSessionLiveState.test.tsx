import { act, cleanup, render, screen } from '@testing-library/react';
import { useEffect, useSyncExternalStore } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useChatStore } from '../../store/chatStore';
import { canonicalTestFixture, canonicalTestItem } from '../../services/chatTimelineTestFixture';
import type { ChatTimelinePresentation } from '../../services/chatTimelinePresentation';

function LiveProbe({ presentation, id, onRender }: {
  presentation: ChatTimelinePresentation; id: string; onRender: () => void;
}) {
  const message = useSyncExternalStore(presentation.subscribe, () => presentation.getMessage(id));
  useEffect(() => { onRender(); });
  return <span>{message?.content}</span>;
}

describe('estado live granular da apresentação canônica', () => {
  let fixture: ReturnType<typeof canonicalTestFixture>;
  beforeEach(() => {
    useChatStore.setState({ sessionsByConversationId: {}, timelinesByConversationId: {}, surfaceSessionsByKey: {} });
    fixture = canonicalTestFixture();
  });
  afterEach(async () => { cleanup(); await fixture.client.dispose(); });

  it('rerenderiza somente a mensagem e conversa afetadas com seletor estável', async () => {
    const one = canonicalTestItem('assistant-1', 'inicial 1', { conversationId: 'conversation-1', state: 'streaming' });
    const two = canonicalTestItem('assistant-2', 'inicial 2', { conversationId: 'conversation-2', state: 'streaming' });
    fixture.setItems([one, two]);
    const first = fixture.presentation('first', 'conversation-1');
    const second = fixture.presentation('second', 'conversation-2');
    await first.open(); await second.open();
    const renderOne = vi.fn(); const renderTwo = vi.fn();
    render(<><LiveProbe presentation={first} id={one.itemId} onRender={renderOne} />
      <LiveProbe presentation={second} id={two.itemId} onRender={renderTwo} /></>);
    const next = { ...one, segments: [{ segmentId: 'text', kind: 'text' as const, text: 'delta acumulado 👩🏽‍💻' }] };
    let delivered!: ReturnType<typeof fixture.publish>;
    act(() => { delivered = fixture.publish('first', next); });
    expect(screen.getByText('delta acumulado 👩🏽‍💻')).toBeInTheDocument();
    expect(renderOne).toHaveBeenCalledTimes(2);
    expect(renderTwo).toHaveBeenCalledTimes(1);
    act(() => { fixture.emit(delivered); });
    expect(renderOne).toHaveBeenCalledTimes(2);
    expect(renderTwo).toHaveBeenCalledTimes(1);
  });

  it('preserva identidades dos nós não alterados ao publicar frames live', async () => {
    const one = canonicalTestItem('assistant-1', 'inicial', { state: 'streaming' });
    const two = canonicalTestItem('assistant-2', 'intocada');
    fixture.setItems([one, two]);
    const presentation = fixture.presentation(); await presentation.open();
    const before = presentation.nodes();
    fixture.publish('surface', { ...one, segments: [{ segmentId: 'text', kind: 'text', text: 'novo frame' }] });
    const current = presentation.nodes();
    expect(current[0].itemId).toBe(before[0].itemId);
    expect(current[0].message.content).toBe('novo frame');
    expect(current[1]).toBe(before[1]);
    expect(current[1].message).toBe(before[1].message);
  });

  it('ignora terminal de janela fechada sem criar timeline nem estado Zustand', async () => {
    const item = canonicalTestItem('message', 'parcial', { state: 'streaming' });
    fixture.setItems([item]);
    const presentation = fixture.presentation(); await presentation.open();
    await presentation.close();
    const before = useChatStore.getState();
    fixture.publish('surface', { ...item, state: 'completed' });
    expect(presentation.getMessage(item.itemId)).toBeUndefined();
    expect(fixture.client.hasSurface('conversation', 'surface')).toBe(false);
    expect(useChatStore.getState()).toBe(before);
  });

  it('confirma a resposta visível sem exigir árvore nem sessão padrão no Zustand', async () => {
    const user = canonicalTestItem('question', 'pergunta', { kind: 'message' });
    const assistant = canonicalTestItem('response', '', { state: 'streaming', durability: 'live' });
    fixture.setItems([user, assistant]);
    const presentation = fixture.presentation(); await presentation.open();
    const previous = presentation.nodes();
    const before = useChatStore.getState();
    fixture.publish('surface', { ...assistant, state: 'completed', durability: 'persisted',
      segments: [{ segmentId: 'text', kind: 'text', text: 'resposta terminal' }] });
    const current = presentation.nodes();
    expect(current.map(node => node.itemId)).toEqual(['question', 'response']);
    expect(current[0]).toBe(previous[0]);
    expect(current[1].message.content).toBe('resposta terminal');
    expect(current[1].message.isStreaming).toBe(false);
    expect(useChatStore.getState()).toBe(before);
    expect(before.sessionsByConversationId).toEqual({});
    expect(before.timelinesByConversationId).toEqual({});
  });
});
