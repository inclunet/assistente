import { describe, expect, it, vi } from 'vitest';
import type { ChatTimelineEvent } from '../types/chatTimelineEvents';
import { ChatTimelineClient, type TimelineTransport } from './chatTimelineClient';
import type { TimelineSnapshot } from './timelineConversationState';

const position = { cursor: '', anchor: 'tail', anchorItemId: '', direction: '', limit: 20 };
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
function snapshot(windowId = 'a', overrides: Partial<TimelineSnapshot> = {}): TimelineSnapshot {
  return {
    conversationId: 'chat', scopeParentId: null, epoch: 'epoch', sequence: '10',
    windowId, windowRevision: '1', mode: 'historical', items: [], orderedItemIds: [],
    retainedItemIds: [], retentionBlocked: false, totalCount: 0, startIndex: 0,
    endIndex: 0, hasBefore: false, hasAfter: false, beforeCursor: '', afterCursor: '', ...overrides,
  };
}
function event(windowId = 'a', overrides: Partial<ChatTimelineEvent> = {}): ChatTimelineEvent {
  return {
    conversationId: 'chat', surfaceId: `${windowId}#1`, windowId, epoch: 'epoch', snapshotRevision: '1', syncRequired: false,
    updates: [{ conversationId: 'chat', epoch: 'epoch', sequence: '11', operations: [],
      windowPatches: [{ windowId, previousRevision: '1', revision: '2' }] }], ...overrides,
  };
}
function contentSnapshot(protectedContent: boolean): TimelineSnapshot {
    const value = snapshot();
    value.items = [{ itemId: 'item', conversationId: 'chat', scopeParentId: null, kind: 'assistant',
      turnId: 'turn', replyToItemId: 'question', messageIds: ['message'], representativeMessageId: 'message',
      position: 'opaque', revision: '10', state: protectedContent ? 'streaming' : 'completed',
      durability: protectedContent ? 'volatile' : 'persisted', segments: [],
      metadata: { origin: 'chat', role: 'assistant', pinned: false, model: '', promptTokens: 0, completionTokens: 0, totalTokens: 0 } }];
    value.orderedItemIds = ['item'];
    value.totalCount = value.endIndex = 1;
    return value;
}
function fixture() {
  let emit!: (event: ChatTimelineEvent) => void;
  const unsubscribe = vi.fn();
  const port = {
    listen: vi.fn((receive: typeof emit) => { emit = receive; return unsubscribe; }),
    open: vi.fn<TimelineTransport['open']>(async (_id, surface) => snapshot(surface.split('#')[0])),
    refresh: vi.fn<TimelineTransport['refresh']>(async id => snapshot(id, { windowRevision: '3', sequence: '12' })),
    navigate: vi.fn<TimelineTransport['navigate']>(async id => snapshot(id, { windowRevision: '3', sequence: '12' })),
    retain: vi.fn<TimelineTransport['retain']>(async () => {}),
    close: vi.fn<TimelineTransport['close']>(async () => {}),
  };
  const generations = new Map<string, number>();
  const client = new ChatTimelineClient(port, surface => {
    const generation = (generations.get(surface) ?? 0) + 1;
    generations.set(surface, generation);
    return `${surface}#${generation}`;
  });
  const open = (surface = 'a') => client.open('chat', surface, null, position);
  return { client, port, open, emit: (value: ChatTimelineEvent) => emit(value), unsubscribe };
}

describe('ChatTimelineClient', () => {
  it('instala um listener antes de Open e reaplica evento antecipado', async () => {
    const f = fixture();
    const pending = deferred<TimelineSnapshot>();
    f.port.open.mockReturnValueOnce(pending.promise);
    const opening = f.open();
    f.emit(event());
    pending.resolve(snapshot());
    await opening;
    expect(f.client.getWindow('chat', 'a')?.revision).toBe('2');
    f.port.open.mockResolvedValueOnce(snapshot('b', { sequence: '11' }));
    await f.open('b');
    expect(f.port.listen).toHaveBeenCalledTimes(1);
  });

  it('fecha capability retornada depois de close e permite reabrir somente após cleanup', async () => {
    const f = fixture();
    const pending = deferred<TimelineSnapshot>();
    f.port.open.mockReturnValueOnce(pending.promise);
    const opening = f.open();
    await Promise.resolve();
    const closing = f.client.close('chat', 'a');
    await expect(f.open()).rejects.toThrow();
    pending.resolve(snapshot());
    await Promise.all([opening, closing]);
    expect(f.port.close).toHaveBeenCalledExactlyOnceWith('a');
    expect(f.client.getWindow('chat', 'a')).toBeUndefined();
    f.port.open.mockResolvedValueOnce(snapshot('new-capability'));
    await f.open();
    expect(f.client.getWindow('chat', 'a')?.windowId).toBe('new-capability');
  });

  it('dispose remove visual e listener imediatamente e fecha Open tardio', async () => {
    const f = fixture();
    const pending = deferred<TimelineSnapshot>();
    f.port.open.mockReturnValue(pending.promise);
    const opening = f.open();
    await Promise.resolve();
    const disposal = f.client.dispose();
    expect(f.unsubscribe).toHaveBeenCalledOnce();
    expect(f.client.getWindow('chat', 'a')).toBeUndefined();
    pending.resolve(snapshot());
    await Promise.all([opening, disposal]);
    expect(f.port.close).toHaveBeenCalledExactlyOnceWith('a');
    await expect(f.open()).rejects.toThrow('disposed');
  });

  it('Close cancela Navigate pendente antes de aguardar sua resposta', async () => {
    const f = fixture();
    await f.open();
    const pending = deferred<TimelineSnapshot>();
    f.port.navigate.mockReturnValue(pending.promise);
    f.port.close.mockImplementation(async () => { pending.reject(new Error('cancelled')); });
    const navigation = f.client.navigate('chat', 'a', 'historical', position);
    const assertion = expect(navigation).rejects.toThrow('cancelled');
    await Promise.resolve();
    await f.client.close('chat', 'a');
    await assertion;
    expect(f.client.getWindow('chat', 'a')).toBeUndefined();
  });

  it('não inicia recuperação enquanto Navigate está pendente e não faz fallback automático', async () => {
    const f = fixture();
    await f.open();
    const pending = deferred<TimelineSnapshot>();
    f.port.navigate.mockReturnValue(pending.promise);
    const navigation = f.client.navigate('chat', 'a', 'historical', position);
    await expect(f.client.refresh('chat')).rejects.toThrow('busy');
    pending.reject(new Error('epoch changed'));
    await expect(navigation).rejects.toThrow('epoch changed');
    expect(f.port.refresh).not.toHaveBeenCalled();
    expect(f.client.getWindow('chat', 'a')?.sequence).toBe('10');
  });

  it('retenção usa revisão aplicada por evento sem alterar membership local', async () => {
    const f = fixture();
    await f.open();
    f.emit(event());
    await f.client.setRetention('chat', 'a', ['item']);
    expect(f.port.retain).toHaveBeenCalledWith('a', 'epoch', '2', ['item']);
    expect(f.client.getWindow('chat', 'a')?.metadata.retainedItemIds).toEqual([]);
  });

  it('promove recuperação somente depois de todas as janelas aceitarem snapshots', async () => {
    const f = fixture();
    await f.open();
    await f.open('b');
    const pending = deferred<TimelineSnapshot>();
    f.port.refresh.mockImplementation(async id => id === 'a' ? snapshot('a', { sequence: '12', windowRevision: '3' }) : pending.promise);
    const recovery = f.client.refresh('chat');
    expect(f.client.refresh('chat')).toBe(recovery);
    await Promise.resolve();
    await Promise.resolve();
    expect(f.client.getWindow('chat', 'a')?.sequence).toBe('10');
    pending.resolve(snapshot('b', { sequence: '12', windowRevision: '3' }));
    await recovery;
    expect(f.client.getWindow('chat', 'a')?.sequence).toBe('12');
    expect(f.client.getWindow('chat', 'b')?.sequence).toBe('12');
  });

  it('falha parcial congela visual e próximo Refresh usa as bases efetivamente retornadas', async () => {
    const f = fixture();
    await f.open();
    await f.open('b');
    f.port.refresh.mockImplementationOnce(async () => snapshot('a', { sequence: '12', windowRevision: '3' }))
      .mockRejectedValueOnce(new Error('offline'));
    await expect(f.client.refresh('chat')).rejects.toThrow('offline');
    f.emit(event());
    expect(f.client.getWindow('chat', 'a')?.sequence).toBe('10');
    await f.client.refresh('chat');
    expect(f.port.refresh.mock.calls.slice(-2)).toEqual([['a', '3', undefined], ['b', '1', undefined]]);
    expect(f.client.getError('chat')).toBeUndefined();
  });

  it('fechamento durante recuperação retira a superfície esperada mesmo com erro tardio', async () => {
    const f = fixture();
    await f.open();
    await f.open('b');
    const pending = deferred<TimelineSnapshot>();
    f.port.refresh.mockImplementation(async id => id === 'b' ? pending.promise : snapshot('a', { sequence: '12' }));
    f.port.close.mockImplementation(async () => { pending.reject(new Error('closed')); });
    const recovery = f.client.refresh('chat');
    await Promise.resolve();
    await f.client.close('chat', 'b');
    await recovery;
    expect(f.client.getWindow('chat', 'a')?.sequence).toBe('12');
    expect(f.client.getWindow('chat', 'b')).toBeUndefined();
  });

  it('rejeita nova época mista sem trocar visual e conserva bases para retry', async () => {
    const f = fixture();
    await f.open();
    await f.open('b');
    f.port.refresh.mockImplementation(async id => snapshot(id, { epoch: id, windowRevision: '4' }));
    await expect(f.client.refresh('chat')).rejects.toThrow();
    expect(f.client.getWindow('chat', 'a')?.epoch).toBe('epoch');
    f.port.refresh.mockImplementation(async id => snapshot(id, { epoch: 'new', windowRevision: '5' }));
    await f.client.refresh('chat');
    expect(f.port.refresh.mock.calls.slice(-2)).toEqual([['a', '4', undefined], ['b', '4', undefined]]);
    expect(f.client.getWindow('chat', 'a')?.epoch).toBe('new');
  });

  it('snapshot rejeitado de Open libera capability e permite tentar novamente', async () => {
    const f = fixture();
    f.port.open.mockResolvedValueOnce(snapshot('a', { conversationId: 'other' }));
    await expect(f.open()).rejects.toThrow('rejected');
    expect(f.port.close).toHaveBeenCalledExactlyOnceWith('a');
    await f.open();
    expect(f.client.getWindow('chat', 'a')).toBeDefined();
  });

  it('erro de cleanup remoto é reportado sem reativar apresentação', async () => {
    const f = fixture();
    await f.open();
    f.port.close.mockRejectedValue(new Error('cleanup failed'));
    await expect(f.client.dispose()).rejects.toThrow('cleanup failed');
    expect(f.client.getWindow('chat', 'a')).toBeUndefined();
  });

  it('permite escolher posição explícita em Refresh após perder âncora histórica', async () => {
    const f = fixture();
    await f.open();
    f.port.refresh.mockRejectedValueOnce(new Error('anchor unavailable'));
    await expect(f.client.refresh('chat')).rejects.toThrow('anchor unavailable');
    const replacement = { ...position, anchor: 'tail' };
    await f.client.refresh('chat', new Map([['a', replacement]]));
    expect(f.port.refresh).toHaveBeenLastCalledWith('a', '1', replacement);
    expect(f.client.getWindow('chat', 'a')?.sequence).toBe('12');
    expect(f.port.close).not.toHaveBeenCalled();
  });

  it('expõe recuperação ao atingir orçamento de capabilities aposentadas', async () => {
    const f = fixture();
    await f.open('visible');
    for (let index = 0; index < 257; index++) {
      await f.open(`transient-${index}`);
      await f.client.close('chat', `transient-${index}`);
    }
    expect(f.client.getError('chat')).toBeInstanceOf(Error);
    expect(f.client.getWindow('chat', 'visible')).toBeDefined();
    await f.client.refresh('chat');
    expect(f.client.getError('chat')).toBeUndefined();
  });

  it('não promove snapshots quando reaplicação do buffer encontra lacuna', async () => {
    const f = fixture();
    await f.open();
    const pending = deferred<TimelineSnapshot>();
    f.port.refresh.mockReturnValue(pending.promise);
    const recovery = f.client.refresh('chat');
    f.emit(event('a', { snapshotRevision: '3', updates: [{ conversationId: 'chat', epoch: 'epoch',
      sequence: '14', operations: [], windowPatches: [] }] }));
    pending.resolve(snapshot('a', { sequence: '12', windowRevision: '3' }));
    await expect(recovery).rejects.toThrow();
    expect(f.client.getWindow('chat', 'a')?.sequence).toBe('10');
    expect(f.client.getError('chat')).toBeInstanceOf(Error);
  });

  it('não inicia RPC agendado quando a sessão já foi descartada', async () => {
    const f = fixture();
    const opening = f.open();
    await f.client.dispose();
    await opening;
    expect(f.port.open).not.toHaveBeenCalled();
    expect(f.port.close).not.toHaveBeenCalled();
  });

  it('preserva capability de Open recusado para retry explícito se cleanup remoto falhar', async () => {
    const f = fixture();
    f.port.open.mockResolvedValueOnce(snapshot('rejected', { conversationId: 'other' }));
    f.port.close.mockRejectedValueOnce(new Error('cleanup failed'));
    await expect(f.open()).rejects.toThrow('cleanup failed');
    await expect(f.open()).rejects.toThrow('unavailable');
    await f.client.close('chat', 'a');
    expect(f.port.close).toHaveBeenNthCalledWith(2, 'rejected');
    await f.open();
    expect(f.client.getWindow('chat', 'a')?.windowId).toBe('a');
  });

  it('aposenta Open tardio fechado enquanto outra superfície ainda abre', async () => {
    const f = fixture();
    const a = deferred<TimelineSnapshot>();
    const b = deferred<TimelineSnapshot>();
    f.port.open.mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise);
    const openingA = f.open();
    const openingB = f.open('b');
    await Promise.resolve();
    const closing = f.client.close('chat', 'a');
    a.resolve(snapshot());
    await Promise.all([openingA, closing]);
    for (let index = 0; index < 300; index++) f.emit(event());
    b.resolve(snapshot('b'));
    await openingB;
    expect(f.client.getError('chat')).toBeUndefined();
    expect(f.client.getWindow('chat', 'b')).toBeDefined();
  });

  it('aposenta capability conhecida no candidato mesmo quando Refresh fechado rejeita', async () => {
    const f = fixture();
    await f.open();
    await f.open('b');
    const a = deferred<TimelineSnapshot>();
    const b = deferred<TimelineSnapshot>();
    f.port.refresh.mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise);
    f.port.close.mockImplementation(async () => { a.reject(new Error('cancelled')); });
    const recovery = f.client.refresh('chat');
    await Promise.resolve();
    await f.client.close('chat', 'a');
    for (let index = 0; index < 300; index++) f.emit(event());
    b.resolve(snapshot('b', { sequence: '12' }));
    await recovery;
    expect(f.client.getError('chat')).toBeUndefined();
    expect(f.client.getWindow('chat', 'b')?.sequence).toBe('12');
  });

  it('permite liberar retenção após falha sem descongelar visual nem aceitar nova retenção', async () => {
    const f = fixture();
    await f.open();
    f.emit(event());
    f.port.refresh.mockRejectedValueOnce(new Error('lost anchor'));
    await expect(f.client.refresh('chat')).rejects.toThrow('lost anchor');
    await expect(f.client.setRetention('chat', 'a', ['other'])).rejects.toThrow();
    await f.client.setRetention('chat', 'a', []);
    expect(f.port.retain).toHaveBeenCalledExactlyOnceWith('a', 'epoch', '2', []);
    expect(f.client.getError('chat')).toBeInstanceOf(Error);
    await f.client.refresh('chat', new Map([['a', position]]));
    expect(f.client.getError('chat')).toBeUndefined();
  });

  it('liberação após recuperação parcial mantém época e revisão do mesmo snapshot', async () => {
    const f = fixture();
    await f.open();
    await f.open('b');
    f.emit(event());
    f.port.refresh.mockResolvedValueOnce(snapshot('a', { epoch: 'new', windowRevision: '1' }))
      .mockRejectedValueOnce(new Error('offline'));
    await expect(f.client.refresh('chat')).rejects.toThrow();
    f.port.retain.mockRejectedValueOnce(new Error('stale'));
    await expect(f.client.setRetention('chat', 'a', [])).rejects.toThrow('stale');
    expect(f.port.retain).toHaveBeenCalledExactlyOnceWith('a', 'new', '1', []);
    expect(f.client.getError('chat')).toBeInstanceOf(Error);
  });

  it('Close falho mantém capability para retry sem restaurar conteúdo', async () => {
    const f = fixture();
    await f.open();
    f.port.close.mockRejectedValueOnce(new Error('offline'));
    await expect(f.client.close('chat', 'a')).rejects.toThrow('offline');
    expect(f.client.getWindow('chat', 'a')).toBeUndefined();
    await f.client.close('chat', 'a');
    expect(f.port.close).toHaveBeenNthCalledWith(2, 'a');
    await f.open();
  });

  it('descarta coordenador vazio após última superfície fechar', async () => {
    const f = fixture();
    await f.open();
    await f.client.close('chat', 'a');
    await expect(f.client.refresh('chat')).rejects.toThrow('Unknown');
  });

  it('Navigate com snapshot incompatível solicita recovery sem trocar apresentação', async () => {
    const f = fixture();
    await f.open();
    f.port.navigate.mockResolvedValueOnce(snapshot('a', { epoch: 'other' }));
    await f.client.navigate('chat', 'a', 'historical', position);
    expect(f.client.getError('chat')).toBeInstanceOf(Error);
    expect(f.client.getWindow('chat', 'a')?.epoch).toBe('epoch');
  });

  it('Refresh ignora eventos de superfície fechada antes de criar o candidato', async () => {
    const f = fixture();
    await f.open();
    await f.open('b');
    await f.client.close('chat', 'a');
    const pending = deferred<TimelineSnapshot>();
    f.port.refresh.mockReturnValue(pending.promise);
    const recovery = f.client.refresh('chat');
    for (let index = 0; index < 300; index++) f.emit(event());
    pending.resolve(snapshot('b', { sequence: '12' }));
    await recovery;
    expect(f.client.getError('chat')).toBeUndefined();
    expect(f.client.getWindow('chat', 'b')?.sequence).toBe('12');
  });

  it.each([false, true])('fechar janela com erro não mantém erro local, mas preserva falha global=%s', async global => {
    const f = fixture();
    await f.open();
    await f.open('b');
    const pending = deferred<TimelineSnapshot>();
    f.port.refresh.mockResolvedValueOnce(snapshot('a', { sequence: '12', windowRevision: '3' }))
      .mockReturnValueOnce(pending.promise);
    const recovery = f.client.refresh('chat');
    await Promise.resolve();
    await Promise.resolve();
    f.emit(event('a', { snapshotRevision: '3', syncRequired: !global, updates: [], epoch: global ? 'other' : 'epoch' }));
    await f.client.close('chat', 'a');
    pending.resolve(snapshot('b', { sequence: '12', windowRevision: '3' }));
    if (global) {
      await expect(recovery).rejects.toThrow();
      expect(f.client.getWindow('chat', 'b')?.sequence).toBe('10');
    } else {
      await recovery;
      expect(f.client.getError('chat')).toBeUndefined();
      expect(f.client.getWindow('chat', 'b')?.sequence).toBe('12');
    }
  });

  it('promoção e nova abertura não reabilitam entregas de inscrição encerrada', async () => {
    const f = fixture();
    await f.open();
    await f.open('b');
    await f.client.close('chat', 'a');
    await f.client.refresh('chat');
    const pending = deferred<TimelineSnapshot>();
    f.port.open.mockReturnValueOnce(pending.promise);
    const opening = f.open('c');
    for (let index = 0; index < 300; index++) f.emit(event());
    pending.resolve(snapshot('c', { sequence: '12' }));
    await opening;
    expect(f.client.getError('chat')).toBeUndefined();
    expect(f.client.getWindow('chat', 'b')).toBeDefined();
    expect(f.client.getWindow('chat', 'c')).toBeDefined();
  });

  it('fechar superfície com erro local libera operações nas demais sem Refresh', async () => {
    const f = fixture();
    await f.open();
    await f.open('b');
    f.emit(event('a', { syncRequired: true, updates: [] }));
    expect(f.client.getError('chat')).toBeDefined();
    await f.client.close('chat', 'a');
    expect(f.client.getError('chat')).toBeUndefined();
    await f.client.navigate('chat', 'b', 'historical', position);
    f.port.open.mockResolvedValueOnce(snapshot('new-a', { sequence: '12' }));
    await f.open();
    expect(f.client.getWindow('chat', 'a')?.windowId).toBe('new-a');
  });

  it.each([false, true])('último Close descarta cache em erro apenas quando dispensável, protegido=%s', async protectedContent => {
    const f = fixture();
    const value = contentSnapshot(protectedContent);
    f.port.open.mockResolvedValueOnce(value);
    await f.open();
    f.port.refresh.mockRejectedValueOnce(new Error('offline'));
    await expect(f.client.refresh('chat')).rejects.toThrow('offline');
    await f.client.close('chat', 'a');
    if (protectedContent) {
      expect(f.client.getError('chat')).toBeDefined();
      await expect(f.open()).rejects.toThrow('unavailable');
      await expect(f.client.refresh('chat')).rejects.toThrow('active surface');
    } else {
      await f.open();
      expect(f.client.getWindow('chat', 'a')).toBeDefined();
    }
  });

  it('rejeita evento sem correlação e evento com capability divergente', async () => {
    const f = fixture();
    await f.open();
    f.emit(event('a', { surfaceId: '' }));
    f.emit(event('other', { surfaceId: 'a#1' }));
    expect(f.client.getWindow('chat', 'a')?.sequence).toBe('10');
  });

  it.each([false, true])('não descarta protegido de janela fechada durante Refresh, outra janela=%s', async anotherWindow => {
    const f = fixture();
    f.port.open.mockResolvedValueOnce(contentSnapshot(true));
    await f.open();
    if (anotherWindow) await f.open('b');
    const pending = deferred<TimelineSnapshot>();
    f.port.refresh.mockReturnValueOnce(pending.promise);
    const recovery = f.client.refresh('chat');
    await Promise.resolve();
    await f.client.close('chat', 'a');
    pending.resolve(snapshot());
    await expect(recovery).rejects.toThrow('ownership transfer');
    if (anotherWindow) {
      expect(f.client.getWindow('chat', 'b')?.sequence).toBe('10');
      await f.client.close('chat', 'b');
    }
    await expect(f.open()).rejects.toThrow('unavailable');
  });

  it('permite promoção quando snapshot confirma persistência do conteúdo antes protegido', async () => {
    const f = fixture();
    f.port.open.mockResolvedValueOnce(contentSnapshot(true));
    await f.open();
    const persisted = contentSnapshot(false);
    persisted.sequence = persisted.items[0].revision = '11';
    persisted.windowRevision = '2';
    f.port.refresh.mockResolvedValueOnce(persisted);
    await f.client.refresh('chat');
    expect(f.client.getItem('chat', 'a', 'item')?.durability).toBe('persisted');
    expect(f.client.getError('chat')).toBeUndefined();
  });

  it.each([false, true])('preserva prova de remoção até promover candidato, duas superfícies=%s', async twoSurfaces => {
    const f = fixture();
    f.port.open.mockResolvedValueOnce(contentSnapshot(true));
    await f.open();
    if (twoSurfaces) {
      f.port.open.mockResolvedValueOnce({ ...contentSnapshot(true), windowId: 'b' });
      await f.open('b');
    }
    const a = deferred<TimelineSnapshot>();
    const b = deferred<TimelineSnapshot>();
    f.port.refresh.mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise);
    const recovery = f.client.refresh('chat');
    a.resolve({ ...contentSnapshot(true), windowRevision: '3' });
    b.resolve({ ...contentSnapshot(true), windowId: 'b', windowRevision: '3' });
    await Promise.resolve();
    await Promise.resolve();
    for (const id of twoSurfaces ? ['a', 'b'] : ['a']) {
      f.emit(event(id, { snapshotRevision: '3', updates: [{ conversationId: 'chat', epoch: 'epoch', sequence: '11',
        operations: [{ kind: 'item_remove', itemId: 'item', revision: '11' }],
        windowPatches: [{ windowId: id, previousRevision: '3', revision: '4', splices: [{ index: 0, deleteCount: 1, insertIds: [] }] }] }] }));
    }
    await recovery;
    expect(f.client.getError('chat')).toBeUndefined();
    expect(f.client.getWindow('chat', 'a')?.orderedItemIds).toEqual([]);
    if (twoSurfaces) expect(f.client.getWindow('chat', 'b')?.orderedItemIds).toEqual([]);
  });
});
