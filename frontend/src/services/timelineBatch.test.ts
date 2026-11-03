import { describe, expect, it } from 'vitest';
import type { TimelineItem, TimelineUpdate } from '../types/chatTimelineEvents';
import { prepareTimelineBatch, type TimelineBatchWindow, type TimelineCacheEntry } from './timelineBatch';

function item(id = 'answer', rev = '10'): TimelineItem {
  return {
    itemId: id, conversationId: 'chat', scopeParentId: null, kind: 'assistant',
    turnId: 'turn', replyToItemId: 'question', messageIds: ['message'],
    representativeMessageId: 'message', position: 'opaque', revision: rev,
    state: 'streaming', durability: 'persisted',
    segments: [
      { segmentId: 'text', kind: 'text', text: 'Olá' },
      { segmentId: 'reason', kind: 'reasoning', text: 'Pensando' },
    ],
    metadata: { origin: 'chat', role: 'assistant', pinned: false, model: '',
      promptTokens: 0, completionTokens: 0, totalTokens: 0 },
  };
}

function window(windowId = 'window'): TimelineBatchWindow {
  return {
    conversationId: 'chat', scopeParentId: null, epoch: 'epoch', windowId,
    revision: '1', sequence: '10', orderedItemIds: ['question', 'answer'],
    metadata: { retainedItemIds: [], retentionBlocked: false, totalCount: 2,
      startIndex: 0, endIndex: 2, hasBefore: false, hasAfter: false, beforeCursor: '', afterCursor: '' },
  };
}

function batch(overrides: Partial<TimelineUpdate> = {}): TimelineUpdate {
  return { conversationId: 'chat', epoch: 'epoch', sequence: '11', operations: [], windowPatches: [], ...overrides };
}

function cache(...items: TimelineItem[]): Map<string, TimelineCacheEntry> {
  return new Map(items.map(value => [value.itemId, { revision: value.revision, item: value }]));
}

const append = (segmentId = 'text') => ({
  kind: 'segment_append', itemId: 'answer', revision: '11', segmentId, text: '!',
});

describe('prepareTimelineBatch', () => {
  it('aplica dois segmentos da mesma revisão e preserva identidade da lista e itens intocados', () => {
    const view = window();
    const original = item();
    const entries = cache(original, item('question'));
    const result = prepareTimelineBatch(view, batch({ operations: [append(), append('reason')] }), id => entries.get(id));
    expect(result.kind).toBe('applied');
    if (result.kind !== 'applied') throw new Error('expected applied');
    expect(result.window.orderedItemIds).toBe(view.orderedItemIds);
    expect(result.window.metadata).toBe(view.metadata);
    expect(result.writes.size).toBe(1);
    expect(result.writes.get('answer')?.item?.segments.map(segment => segment.text)).toEqual(['Olá!', 'Pensando!']);
    expect(original.segments[0].text).toBe('Olá');
    expect(view.sequence).toBe('10');
  });

  it('descarta toda a transação quando uma operação posterior é inválida', () => {
    const original = item();
    const entries = cache(original);
    expect(prepareTimelineBatch(window(), batch({ operations: [append(), append('inexistente')] }), id => entries.get(id)))
      .toEqual({ kind: 'recover', reason: 'content' });
    expect(entries.get('answer')?.item).toBe(original);
    expect(original.segments[0].text).toBe('Olá');
  });

  it.each(['9', '10'])('ignora sequência duplicada %s antes de aplicar conteúdo', sequence => {
    expect(prepareTimelineBatch(window(), batch({ sequence, operations: [append()] }), () => undefined)).toEqual({ kind: 'duplicate' });
  });

  it.each(['12', '-1', '01', '1.5', '18446744073709551616'])('recupera lacuna ou sequência inválida %s', sequence => {
    expect(prepareTimelineBatch(window(), batch({ sequence }), () => undefined)).toEqual({ kind: 'recover', reason: 'gap' });
  });

  it('compara uint64 acima da precisão de Number sem arredondamento', () => {
    const view = { ...window(), sequence: '9007199254740992' };
    const result = prepareTimelineBatch(view, batch({ sequence: '9007199254740993' }), () => undefined);
    expect(result.kind).toBe('applied');
  });

  it.each([{ epoch: 'other' }, { conversationId: 'other' }])('recusa identidade externa %j', identity => {
    expect(prepareTimelineBatch(window(), batch(identity), () => undefined)).toEqual({ kind: 'recover', reason: 'epoch' });
  });

  it('avança lote filtrado vazio sem copiar lista ou conteúdo', () => {
    const view = window();
    const result = prepareTimelineBatch(view, batch(), () => undefined);
    if (result.kind !== 'applied') throw new Error('expected applied');
    expect(result.window.sequence).toBe('11');
    expect(result.window.orderedItemIds).toBe(view.orderedItemIds);
    expect(result.writes.size).toBe(0);
  });

  it.each([false, true])('duas janelas aplicam partes diferentes da mesma sequência (ordem invertida: %s)', reverse => {
    const entries = cache(item(), item('question'));
    const views = [window('a'), window('b')];
    const updates = [
      batch({ operations: [append()] }),
      batch({ operations: [{ ...append(), itemId: 'question' }] }),
    ];
    for (const index of reverse ? [1, 0] : [0, 1]) {
      const result = prepareTimelineBatch(views[index], updates[index], id => entries.get(id));
      if (result.kind !== 'applied') throw new Error('expected applied');
      for (const [id, value] of result.writes) entries.set(id, value);
      views[index] = result.window;
    }
    expect(views.map(view => view.sequence)).toEqual(['11', '11']);
    expect(entries.get('question')?.item?.segments[0].text).toBe('Olá!');
    expect(entries.get('answer')?.item?.segments[0].text).toBe('Olá!');
  });

  it('não reaplica delta já materializado pela outra janela', () => {
    const newer = item('answer', '11');
    newer.segments[0].text = 'Olá!';
    const entries = cache(newer);
    const result = prepareTimelineBatch(window(), batch({ operations: [append()] }), id => entries.get(id));
    if (result.kind !== 'applied') throw new Error('expected applied');
    expect(result.writes.size).toBe(0);
  });

  it.each([['a', 'b'], ['b', 'a']])('duas entregas do mesmo item não duplicam texto: %s, %s', (first, second) => {
    const entries = cache(item());
    for (const id of [first, second]) {
      const result = prepareTimelineBatch(window(id), batch({ operations: [append()] }), key => entries.get(key));
      if (result.kind !== 'applied') throw new Error('expected applied');
      for (const [key, entry] of result.writes) entries.set(key, entry);
      expect(result.window.sequence).toBe('11');
    }
    expect(entries.get('answer')?.item?.segments[0].text).toBe('Olá!');
  });

  it('hidratação completa não impede operações de outro item no mesmo lote', () => {
    const entries = cache(item(), item('question'));
    const full = item('answer', '11');
    full.segments[0].text = 'Completo';
    const result = prepareTimelineBatch(window(), batch({
      windowPatches: [{ windowId: 'window', previousRevision: '1', revision: '2', items: [full] }],
      operations: [append(), { ...append(), itemId: 'question' }],
    }), key => entries.get(key));
    if (result.kind !== 'applied') throw new Error('expected applied');
    expect(result.writes.get('answer')?.item?.segments[0].text).toBe('Completo');
    expect(result.writes.get('question')?.item?.segments[0].text).toBe('Olá!');
  });

  it('upsert fora da janela atualiza somente conteúdo já conhecido', () => {
    const entries = cache(item('known'));
    const result = prepareTimelineBatch(window(), batch({ operations: ['known', 'unknown'].map(id => ({
      kind: 'item_upsert', itemId: id, revision: '11', item: item(id, '11'),
    })) }), key => entries.get(key));
    if (result.kind !== 'applied') throw new Error('expected applied');
    expect([...result.writes.keys()]).toEqual(['known']);
    expect(result.window.orderedItemIds).toEqual(['question', 'answer']);
  });

  it('usa splices na ordem recebida e hidrata sem promover revisão de item', () => {
    const entries = cache(item(), item('question'));
    const metadata = { ...window().metadata, totalCount: 3, endIndex: 3 };
    const result = prepareTimelineBatch(window(), batch({ windowPatches: [{
      windowId: 'window', previousRevision: '1', revision: '2',
      splices: [{ index: 1, deleteCount: 1, insertIds: ['middle'] }, { index: 2, deleteCount: 0, insertIds: ['answer'] }],
      items: [item('middle', '5')], metadata,
    }] }), id => entries.get(id));
    if (result.kind !== 'applied') throw new Error('expected applied');
    expect(result.window.orderedItemIds).toEqual(['question', 'middle', 'answer']);
    expect(result.writes.get('middle')?.revision).toBe('5');
    expect(result.window.metadata).toEqual(metadata);
    expect(result.writes.has('answer')).toBe(false);
  });

  it.each([
    { previousRevision: '0' }, { syncRequired: true }, { revision: '1' },
    { splices: [{ index: 3, deleteCount: 0, insertIds: [] }] },
    { splices: [{ index: 0, deleteCount: 3, insertIds: [] }] },
    { splices: [{ index: 0, deleteCount: 0, insertIds: ['answer'] }] },
  ])('recusa patch inválido sem alterar janela %j', invalid => {
    const view = window();
    expect(prepareTimelineBatch(view, batch({ windowPatches: [{
      windowId: 'window', previousRevision: '1', revision: '2', ...invalid,
    }] }), () => undefined)).toEqual({ kind: 'recover', reason: 'patch' });
    expect(view.revision).toBe('1');
  });

  it('não ressuscita tombstone com hidratação atrasada de outra janela', () => {
    const entries = cache(item('question'));
    entries.set('answer', { revision: '12', item: null });
    const view = { ...window(), orderedItemIds: ['question'] };
    expect(prepareTimelineBatch(view, batch({ windowPatches: [{
      windowId: 'window', previousRevision: '1', revision: '2',
      splices: [{ index: 1, deleteCount: 0, insertIds: ['answer'] }], items: [item()],
    }] }), id => entries.get(id))).toEqual({ kind: 'recover', reason: 'content' });
    expect(entries.get('answer')?.item).toBeNull();
  });

  it('remoção é atômica com a saída da janela; ausência em página não remove item', () => {
    const entries = cache(item(), item('question'));
    const patch = { windowId: 'window', previousRevision: '1', revision: '2',
      splices: [{ index: 1, deleteCount: 1, insertIds: [] }] };
    const result = prepareTimelineBatch(window(), batch({ windowPatches: [patch],
      operations: [{ kind: 'item_remove', itemId: 'answer', revision: '11' }],
    }), id => entries.get(id));
    if (result.kind !== 'applied') throw new Error('expected applied');
    expect(result.writes.get('answer')).toEqual({ revision: '11', item: null });
    const page = prepareTimelineBatch(window(), batch({ windowPatches: [patch] }), id => entries.get(id));
    if (page.kind !== 'applied') throw new Error('expected applied');
    expect(page.writes.size).toBe(0);
  });

  it('substitui segmento e conclusão preservando itemId', () => {
    const entries = cache(item());
    const terminal = { ...item('answer', '11'), state: 'completed' };
    const result = prepareTimelineBatch(window(), batch({ operations: [{
      kind: 'item_upsert', itemId: 'answer', revision: '11', item: terminal,
    }] }), id => entries.get(id));
    if (result.kind !== 'applied') throw new Error('expected applied');
    expect(result.writes.get('answer')?.item?.state).toBe('completed');
    expect(result.window.orderedItemIds).toEqual(['question', 'answer']);
    const replaced = prepareTimelineBatch(window(), batch({ operations: [{
      kind: 'segment_replace', itemId: 'answer', revision: '11', segmentId: 'text',
      segment: { segmentId: 'text', kind: 'text', text: 'Texto final' },
    }] }), id => entries.get(id));
    if (replaced.kind !== 'applied') throw new Error('expected applied');
    expect(replaced.writes.get('answer')?.item?.segments[0].text).toBe('Texto final');
  });
});
