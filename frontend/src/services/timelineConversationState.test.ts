import { describe, expect, it } from 'vitest';
import type { ChatTimelineEvent, TimelineItem, TimelineUpdate } from '../types/chatTimelineEvents';
import { TimelineCacheBudget } from './timelineCacheBudget';
import { TimelineConversationState, type TimelineSnapshot } from './timelineConversationState';

function item(id = 'answer', sequence = '10', conversationId = 'chat'): TimelineItem {
  return {
    itemId: id, conversationId, scopeParentId: null, kind: 'assistant', turnId: 'turn',
    replyToItemId: 'question', messageIds: ['message'], representativeMessageId: 'message',
    position: 'opaque', revision: sequence, state: 'completed', durability: 'persisted',
    segments: [{ segmentId: 'text', kind: 'text', text: 'Olá' }],
    metadata: { origin: 'chat', role: 'assistant', pinned: false, model: '',
      promptTokens: 0, completionTokens: 0, totalTokens: 0 },
  };
}

function snapshot(windowId = 'a', items = [item()], overrides: Partial<TimelineSnapshot> = {}): TimelineSnapshot {
  return {
    conversationId: 'chat', scopeParentId: null, epoch: 'epoch', sequence: '10',
    windowId, windowRevision: '1', mode: 'historical', items,
    orderedItemIds: items.map(value => value.itemId), retainedItemIds: [], retentionBlocked: false,
    totalCount: items.length, startIndex: 0, endIndex: items.length, hasBefore: false,
    hasAfter: false, beforeCursor: '', afterCursor: '', ...overrides,
  };
}

function event(windowId = 'a', overrides: Partial<TimelineUpdate> = {}, envelope: Partial<ChatTimelineEvent> = {}): ChatTimelineEvent {
  return {
    conversationId: 'chat', epoch: 'epoch', windowId, snapshotRevision: '1', syncRequired: false,
    updates: [{ conversationId: 'chat', epoch: 'epoch', sequence: '11', windowPatches: [],
      operations: [{ kind: 'segment_append', itemId: 'answer', revision: '11', segmentId: 'text', text: '!' }],
      ...overrides }], ...envelope,
  };
}

function open(state: TimelineConversationState, surfaceId = 'a', value = snapshot(surfaceId)) {
  const token = state.beginSnapshot(surfaceId, value.scopeParentId);
  expect(state.acceptSnapshot(token, value)).toEqual({ kind: 'applied' });
}

function remove(windowId = 'a'): ChatTimelineEvent {
  return event(windowId, {
    operations: [{ kind: 'item_remove', itemId: 'answer', revision: '11' }],
    windowPatches: [{ windowId, previousRevision: '1', revision: '2',
      splices: [{ index: 0, deleteCount: 1, insertIds: [] }] }],
  });
}

describe('TimelineConversationState', () => {
  it('recebe evento antes de Open resolver e reaplica somente depois do snapshot', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    const token = state.beginSnapshot('surface', null);
    expect(state.receive(event())).toEqual({ kind: 'buffered' });
    expect(state.getWindow('surface')).toBeUndefined();
    expect(state.acceptSnapshot(token, snapshot())).toEqual({ kind: 'applied' });
    expect(state.getItem('surface', 'answer')?.segments[0].text).toBe('Olá!');
    expect(state.getWindow('surface')?.sequence).toBe('11');
    expect(state.bufferedBatches).toBe(0);
  });

  it('consulta substituída não altera cache nem janela', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    const old = state.beginSnapshot('a', null);
    const current = state.beginSnapshot('a', null);
    expect(state.acceptSnapshot(old, snapshot('old'))).toEqual({ kind: 'ignored' });
    expect(state.cachedItems).toBe(0);
    expect(state.receive(event('old'))).toEqual({ kind: 'ignored' });
    expect(state.acceptSnapshot(current, snapshot('current'))).toEqual({ kind: 'applied' });
    expect(state.getWindow('a')?.windowId).toBe('current');
  });

  it('fechar/reabrir descarta resposta e eventos da capability anterior', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    const old = state.beginSnapshot('a', null);
    state.close('a');
    const current = state.beginSnapshot('a', null);
    expect(state.receive(event('old'))).toEqual({ kind: 'buffered' });
    expect(state.acceptSnapshot(old, snapshot('old'))).toEqual({ kind: 'ignored' });
    expect(state.acceptSnapshot(current, snapshot('new'))).toEqual({ kind: 'applied' });
    expect(state.getItem('a', 'answer')?.segments[0].text).toBe('Olá');
    expect(state.bufferedBatches).toBe(0);
    expect(state.receive(event('old'))).toEqual({ kind: 'ignored' });
  });

  it('snapshot atrasado não rebaixa conteúdo compartilhado atualizado pela outra janela', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    open(state);
    const pending = state.beginSnapshot('b', null);
    state.receive(event());
    expect(state.acceptSnapshot(pending, snapshot('b'))).toEqual({ kind: 'applied' });
    expect(state.getItem('a', 'answer')).toBe(state.getItem('b', 'answer'));
    expect(state.getItem('b', 'answer')?.segments[0].text).toBe('Olá!');
    expect(state.receive(event('b'))).toEqual({ kind: 'applied' });
    expect(state.getItem('b', 'answer')?.segments[0].text).toBe('Olá!');
  });

  it('fechar A remove seus lotes e ignora atrasados sem afetar Open pendente de B', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    open(state);
    state.beginSnapshot('a', null);
    const pending = state.beginSnapshot('b', null);
    state.receive(event('a'));
    state.receive(event('b'));
    expect(state.bufferedBatches).toBe(2);
    state.close('a');
    expect(state.bufferedBatches).toBe(1);
    for (let index = 0; index < 300; index++) expect(state.receive(event('a'))).toEqual({ kind: 'ignored' });
    expect(state.frozenReason).toBeUndefined();
    expect(state.acceptSnapshot(pending, snapshot('b'))).toEqual({ kind: 'applied' });
    expect(state.getItem('b', 'answer')?.segments[0].text).toBe('Olá!');
  });

  it('resposta tardia de Open aposenta a capability descoberta sem bloquear outro Open', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    const closed = state.beginSnapshot('a', null);
    const pending = state.beginSnapshot('b', null);
    state.close('a');
    state.receive(event('a'));
    expect(state.acceptSnapshot(closed, snapshot('a'))).toEqual({ kind: 'ignored' });
    expect(state.bufferedBatches).toBe(0);
    expect(state.receive(event('a'))).toEqual({ kind: 'ignored' });
    expect(state.acceptSnapshot(pending, snapshot('b'))).toEqual({ kind: 'applied' });
  });

  it('Refresh obsoleto não aposenta a capability ativa nem aceita token fabricado', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    open(state);
    const old = state.beginSnapshot('a', null);
    const current = state.beginSnapshot('a', null);
    expect(state.acceptSnapshot(old, snapshot('a'))).toEqual({ kind: 'ignored' });
    expect(state.acceptSnapshot({ surfaceId: 'a', scopeParentId: null }, snapshot('a'))).toEqual({ kind: 'ignored' });
    expect(state.acceptSnapshot(current, snapshot('a', [item()], { windowRevision: '2' }))).toEqual({ kind: 'applied' });
    expect(state.receive(event('a', {}, { snapshotRevision: '2' }))).toEqual({ kind: 'applied' });
    expect(state.getItem('a', 'answer')?.segments[0].text).toBe('Olá!');
  });

  it('Open recusado por tombstone descarta seus eventos sem afetar outra abertura', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    open(state);
    const rejected = state.beginSnapshot('b', null);
    const pending = state.beginSnapshot('c', null);
    state.receive(remove('a'));
    state.receive(event('b'));
    expect(state.acceptSnapshot(rejected, snapshot('b'))).toEqual({ kind: 'recover', reason: 'snapshot' });
    expect(state.bufferedBatches).toBe(0);
    state.close('b');
    for (let index = 0; index < 300; index++) expect(state.receive(event('b'))).toEqual({ kind: 'ignored' });
    expect(state.frozenReason).toBeUndefined();
    expect(state.acceptSnapshot(pending, snapshot('c', [], { sequence: '11' }))).toEqual({ kind: 'applied' });
  });

  it('snapshot inválido de Refresh preserva a capability para nova recuperação', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    open(state);
    const rejected = state.beginSnapshot('a', null);
    expect(state.acceptSnapshot(rejected, snapshot('a', [], { orderedItemIds: ['answer'] })))
      .toEqual({ kind: 'recover', reason: 'snapshot' });
    const recovery = state.beginSnapshot('a', null);
    expect(state.acceptSnapshot(recovery, snapshot('a', [item()], { windowRevision: '2' })))
      .toEqual({ kind: 'applied' });
    expect(state.receive(event('a', {}, { snapshotRevision: '2' }))).toEqual({ kind: 'applied' });
  });

  it('falha de Refresh libera seus lotes, preservando eventos de Open ainda pendente', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    open(state);
    const failed = state.beginSnapshot('a', null);
    const pending = state.beginSnapshot('b', null);
    for (let index = 0; index < 254; index++) state.receive(event('a'));
    state.receive(event('b'));
    expect(state.bufferedBatches).toBe(255);
    expect(state.failSnapshot(failed)).toEqual({ kind: 'recover', reason: 'snapshot' });
    expect(state.bufferedBatches).toBe(1);
    expect(state.receive(event('b', { sequence: '12', operations: [{
      kind: 'segment_append', itemId: 'answer', revision: '12', segmentId: 'text', text: '?',
    }] }))).toEqual({ kind: 'buffered' });
    expect(state.acceptSnapshot(pending, snapshot('b'))).toEqual({ kind: 'applied' });
    expect(state.getItem('b', 'answer')?.segments[0].text).toBe('Olá!?');
    expect(state.frozenReason).toBeUndefined();
  });

  it.each(['count', 'bytes'])('metadados de capabilities aposentadas têm limite por %s', limit => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    open(state, 'visible');
    const original = state.getItem('visible', 'answer');
    if (limit === 'count') {
      for (let index = 0; index < 256; index++) {
        open(state, 'temporary', snapshot(`window-${index}`));
        expect(state.close('temporary')).toEqual({ kind: 'applied' });
      }
    }
    open(state, 'temporary', snapshot(limit === 'count' ? 'overflow' : 'x'.repeat(2 * 1024 * 1024 + 1)));
    expect(state.close('temporary')).toEqual({ kind: 'recover', reason: 'retention' });
    expect(state.getItem('visible', 'answer')).toBe(original);
    expect(state.receive(event('visible'))).toEqual({ kind: 'ignored' });
  });

  it('preserva a janela durante navegação e ignora eventos da base substituída', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    open(state);
    const previous = state.getWindow('a');
    const pending = state.beginSnapshot('a', null);
    state.receive(event());
    state.receive(event('a', { sequence: '12', operations: [
      { kind: 'segment_append', itemId: 'answer', revision: '12', segmentId: 'text', text: '?' },
    ] }, { snapshotRevision: '2' }));
    expect(state.getWindow('a')).toBe(previous);
    const full = item('answer', '11');
    full.segments[0].text = 'Atual';
    expect(state.acceptSnapshot(pending, snapshot('a', [full], { sequence: '11', windowRevision: '2' })))
      .toEqual({ kind: 'applied' });
    expect(state.getItem('a', 'answer')?.segments[0].text).toBe('Atual?');
  });

  it('lacuna preserva texto e exige recuperação antes de novos deltas', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    open(state);
    const original = state.getItem('a', 'answer');
    expect(state.receive(event('a', { sequence: '12' }))).toEqual({ kind: 'recover', reason: 'gap' });
    expect(state.receive(event())).toEqual({ kind: 'recover', reason: 'gap' });
    expect(state.getItem('a', 'answer')).toBe(original);
    const token = state.beginSnapshot('a', null);
    expect(state.acceptSnapshot(token, snapshot('a', [item('answer', '12')], { sequence: '12', windowRevision: '2' })))
      .toEqual({ kind: 'applied' });
  });

  it('remoção conserva corpo só para apresentação atrasada, sem ressuscitar snapshot antigo', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    open(state);
    open(state, 'b');
    const oldBody = state.getItem('b', 'answer');
    const pending = state.beginSnapshot('c', null);
    expect(state.receive(remove())).toEqual({ kind: 'applied' });
    expect(state.getItem('a', 'answer')).toBeUndefined();
    expect(state.getItem('b', 'answer')).toBe(oldBody);
    expect(state.acceptSnapshot(pending, snapshot('c'))).toEqual({ kind: 'recover', reason: 'snapshot' });
    expect(state.receive(remove('b'))).toEqual({ kind: 'applied' });
    expect(state.getItem('b', 'answer')).toBeUndefined();
    state.close('c');
    expect(state.cachedItems).toBe(0);
  });

  it('piso de GC recusa resposta antiga mesmo com token atual após fechar janela', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    open(state);
    state.receive(remove());
    state.close('a');
    expect(state.cachedItems).toBe(0);
    const pending = state.beginSnapshot('b', null);
    expect(state.acceptSnapshot(pending, snapshot('b'))).toEqual({ kind: 'recover', reason: 'snapshot' });
    expect(state.cachedItems).toBe(0);
  });

  it('consulta pendente impede GC prematuro e é descartada após fechamento', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    open(state);
    const pending = state.beginSnapshot('b', null);
    state.receive(remove());
    expect(state.cachedItems).toBe(1);
    state.close('b');
    expect(state.cachedItems).toBe(0);
    expect(state.acceptSnapshot(pending, snapshot('b'))).toEqual({ kind: 'ignored' });
  });

  it('lote vazio da janela atrasada libera tombstone sem outra mudança estrutural', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    open(state);
    open(state, 'b', snapshot('b', []));
    state.receive(remove());
    expect(state.cachedItems).toBe(1);
    expect(state.receive(event('b', { operations: [] }))).toEqual({ kind: 'applied' });
    expect(state.cachedItems).toBe(0);
    expect(state.getWindow('b')?.sequence).toBe('11');
  });

  it('fechar a última superfície libera tombstone e sua apresentação retida', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    open(state);
    open(state, 'b');
    const pending = state.beginSnapshot('b', null);
    state.receive(remove());
    expect(state.getItem('b', 'answer')).toBeDefined();
    state.close('a');
    state.close('b');
    expect(state.cachedItems).toBe(0);
    expect(state.getItem('b', 'answer')).toBeUndefined();
    expect(state.acceptSnapshot(pending, snapshot('b'))).toEqual({ kind: 'ignored' });
  });

  it('troca de época congela estado anterior e substituto não compartilha revisões', () => {
    const budget = new TimelineCacheBudget();
    const previous = new TimelineConversationState('chat', budget);
    open(previous);
    const old = previous.getItem('a', 'answer');
    expect(previous.receive(event('a', {}, { epoch: 'new' }))).toEqual({ kind: 'recover', reason: 'epoch' });
    expect(previous.receive(event())).toEqual({ kind: 'ignored' });
    expect(previous.getItem('a', 'answer')).toBe(old);
    const next = new TimelineConversationState('chat', budget);
    open(next, 'a', snapshot('a', [item('answer', '1')], { epoch: 'new', sequence: '1', windowRevision: '2' }));
    expect(next.getItem('a', 'answer')?.revision).toBe('1');
    expect(next.receive(event())).toEqual({ kind: 'ignored' });
    expect(next.frozenReason).toBeUndefined();
    previous.dispose();
    expect(next.getItem('a', 'answer')?.revision).toBe('1');
  });

  it.each(['count', 'bytes'])('overflow de %s invalida solicitações e preserva visual', limit => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    open(state);
    const original = state.getItem('a', 'answer');
    const pending = state.beginSnapshot('a', null);
    if (limit === 'count') {
      for (let index = 0; index < 255; index++) expect(state.receive(event()).kind).toBe('buffered');
    }
    const overflow = limit === 'count' ? event() : event('a', { operations: [{
      kind: 'segment_append', itemId: 'answer', revision: '11', segmentId: 'text', text: 'x'.repeat(2 * 1024 * 1024),
    }] });
    expect(state.receive(overflow)).toEqual({ kind: 'recover', reason: 'overflow' });
    expect(state.bufferedBatches).toBe(0);
    expect(state.getItem('a', 'answer')).toBe(original);
    expect(state.acceptSnapshot(pending, snapshot('a'))).toEqual({ kind: 'ignored' });
  });

  it('LRU compartilha cota entre conversas e não expulsa janela em uso', () => {
    const budget = new TimelineCacheBudget(1);
    const first = new TimelineConversationState('chat', budget);
    const second = new TimelineConversationState('other', budget);
    open(first);
    open(second, 'b', snapshot('b', [item('other-item', '10', 'other')], { conversationId: 'other' }));
    expect(budget.usage.items).toBe(0);
    first.close('a');
    expect(budget.usage.items).toBe(1);
    expect(second.getItem('b', 'other-item')).toBeDefined();
    second.close('b');
    expect(first.cachedItems).toBe(0);
    expect(second.cachedItems).toBe(1);
    expect(budget.usage.items).toBe(1);
  });

  it('limite de bytes expulsa conteúdo dispensável e mantém ativo/volatile', () => {
    const budget = new TimelineCacheBudget(240, 1);
    const state = new TimelineConversationState('chat', budget);
    open(state, 'a', snapshot('a', [item(), { ...item('active'), state: 'streaming' }, { ...item('volatile'), durability: 'volatile' }]));
    state.close('a');
    expect(state.cachedItems).toBe(2);
    expect(budget.usage).toEqual({ items: 0, bytes: 0 });
    state.dispose();
    expect(state.cachedItems).toBe(0);
  });

  it('pins da nova página precedem a liberação da página anterior na LRU', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget(1));
    open(state, 'a', snapshot('a', [item('first'), item('second')]));
    const first = state.beginSnapshot('a', null);
    state.acceptSnapshot(first, snapshot('a', [item('first')], { windowRevision: '2' }));
    const second = state.beginSnapshot('a', null);
    expect(state.acceptSnapshot(second, snapshot('a', [item('second')], { windowRevision: '3' })))
      .toEqual({ kind: 'applied' });
    expect(state.getItem('a', 'second')).toBeDefined();
    expect(state.cachedItems).toBe(2);
  });

  it('tombstones impedidos de GC geram recuperação limitada sem apagar corpo visível', () => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget(0));
    open(state);
    const pending = state.beginSnapshot('b', null);
    expect(state.receive(remove())).toEqual({ kind: 'recover', reason: 'retention' });
    expect(state.cachedItems).toBe(1);
    expect(state.acceptSnapshot(pending, snapshot('b'))).toEqual({ kind: 'ignored' });
    expect(state.receive(event())).toEqual({ kind: 'ignored' });
  });

  it('logout invalida respostas e libera somente os registros desta instância', () => {
    const budget = new TimelineCacheBudget();
    const state = new TimelineConversationState('chat', budget);
    open(state);
    const pending = state.beginSnapshot('a', null);
    state.close('a');
    expect(budget.usage.items).toBe(1);
    state.dispose();
    expect(state.acceptSnapshot(pending, snapshot())).toEqual({ kind: 'ignored' });
    expect(state.receive(event())).toEqual({ kind: 'ignored' });
    expect(state.cachedItems).toBe(0);
    expect(budget.usage.items).toBe(0);
  });

  it.each([
    { conversationId: 'other' }, { scopeParentId: 'thread' }, { items: [] },
    { sequence: '18446744073709551616' }, { orderedItemIds: ['answer', 'answer'] },
  ])('snapshot inválido não altera o conteúdo aceito: %j', invalid => {
    const state = new TimelineConversationState('chat', new TimelineCacheBudget());
    open(state);
    const original = state.getItem('a', 'answer');
    const pending = state.beginSnapshot('a', null);
    expect(state.acceptSnapshot(pending, snapshot('a', [item()], invalid))).toEqual({ kind: 'recover', reason: 'snapshot' });
    expect(state.getItem('a', 'answer')).toBe(original);
  });

  it('falha no replay após snapshot mantém referências fora da LRU', () => {
    const budget = new TimelineCacheBudget(1);
    const state = new TimelineConversationState('chat', budget);
    open(state);
    state.close('a');
    const pending = state.beginSnapshot('b', null);
    state.receive(event('b', { sequence: '12' }));
    expect(state.acceptSnapshot(pending, snapshot('b'))).toEqual({ kind: 'recover', reason: 'gap' });
    expect(budget.usage.items).toBe(0);
    const other = new TimelineConversationState('other', budget);
    open(other, 'c', snapshot('c', [item('second', '10', 'other')], { conversationId: 'other' }));
    other.close('c');
    expect(state.getItem('b', 'answer')).toBeDefined();
  });
});
