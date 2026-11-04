import { apidto } from '@wailsjs/go/models';
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
    value.items = [{ itemId: 'item', executionId: 'execution', conversationId: 'chat', scopeParentId: null, kind: 'assistant',
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
    reserveItem: vi.fn<TimelineTransport['reserveItem']>(),
    readItem: vi.fn<TimelineTransport['readItem']>(),
    releaseItem: vi.fn<TimelineTransport['releaseItem']>(async () => {}),
    listen: vi.fn((receive: typeof emit) => { emit = receive; return unsubscribe; }),
    open: vi.fn<TimelineTransport['open']>(async (_id, surface) => snapshot(surface.split('#')[0])),
    refresh: vi.fn<TimelineTransport['refresh']>(async id => snapshot(id, { windowRevision: '3', sequence: '12' })),
    navigate: vi.fn<TimelineTransport['navigate']>(async id => snapshot(id, { windowRevision: '3', sequence: '12' })),
    retain: vi.fn<TimelineTransport['retain']>(async () => {}),
    readRecovery: vi.fn<TimelineTransport['readRecovery']>(),
    reserveRecovery: vi.fn<TimelineTransport['reserveRecovery']>(),
    releaseRecovery: vi.fn<TimelineTransport['releaseRecovery']>(async () => {}),
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


const recoveryReference = { projectionId: 'projection', incarnation: 'storage', turnId: 'turn', executionId: 'execution' };

function itemRecoveryFixture() {
  const f = fixture();
  const original = contentSnapshot(true);
  original.items[0].state = 'interrupted';
  original.items[0].executionId = recoveryReference.executionId;
  original.items[0].recoveryReference = { ...recoveryReference };
  original.items[0].segments = [
    { segmentId: 'opaque', kind: 'text', text: 'conteúdo preservado 🦜' },
    { segmentId: 'tool', kind: 'tool_invocations', invocations: [{ callId: 'call', name: 'tool', status: 'completed', hasDetails: true, resultAvailability: 'available' }] },
  ];
  f.port.open.mockResolvedValueOnce(original);
  f.port.reserveItem.mockImplementation(async (_window, identity) => new apidto.TimelineRecoveryItemReservation({ conversationId: 'chat', identity }));
  f.port.readItem.mockImplementation(async () => structuredClone(original.items[0]));
  return { ...f, original };
}

describe('Refresh com transferência de recuperação', () => {
  it('oito aquisições recusadas não impedem reservar o nono item', async () => {
    const f = itemRecoveryFixture();
    f.original.items = Array.from({ length: 9 }, (_, index) => ({ ...structuredClone(f.original.items[0]),
      itemId: `item-${index}`, turnId: `turn-${index}`, recoveryReference: { ...recoveryReference, turnId: `turn-${index}` } }));
    f.original.orderedItemIds = f.original.items.map(item => item.itemId);
    f.original.totalCount = f.original.endIndex = 9;
    await f.open();
    f.port.reserveItem.mockImplementation(async (_window, identity) => {
      if (identity.itemId !== 'item-8') throw new Error('unavailable');
      return new apidto.TimelineRecoveryItemReservation({ conversationId: 'chat', identity });
    });
    f.port.readItem.mockResolvedValue(f.original.items[8]);
    f.port.refresh.mockResolvedValue({ ...f.original, windowRevision: '3',
      items: f.original.items.slice(0, 8), orderedItemIds: f.original.orderedItemIds.slice(0, 8), totalCount: 8, endIndex: 8 });
    await f.client.refresh('chat');
    expect(f.port.reserveItem).toHaveBeenCalledTimes(9);
    expect(f.client.getRecoveredItems('chat').map(value => value.identity.itemId)).toEqual(['item-8']);
    expect(f.port.releaseItem).toHaveBeenCalledTimes(8);
    await f.client.refresh('chat');
    await f.client.dispose();
  });

  it('recalcula cleanup quando evento remove item necessário durante outra liberação', async () => {
    const f = itemRecoveryFixture();
    const other = { ...structuredClone(f.original.items[0]), itemId: 'other', turnId: 'other-turn',
      recoveryReference: { ...recoveryReference, turnId: 'other-turn' } };
    f.original.items.push(other); f.original.orderedItemIds.push('other');
    f.original.totalCount = f.original.endIndex = 2;
    await f.open();
    f.port.readItem.mockImplementation(async (_window, identity) => structuredClone(f.original.items.find(item => item.itemId === identity.itemId)!));
    f.port.refresh.mockResolvedValueOnce(snapshot('a', { sequence: '12', windowRevision: '3',
      items: [other], orderedItemIds: ['other'], totalCount: 1, endIndex: 1 }));
    const release = deferred<void>();
    f.port.releaseItem.mockReturnValueOnce(release.promise);
    const refreshing = f.client.refresh('chat');
    await vi.waitFor(() => expect(f.port.releaseItem).toHaveBeenCalledOnce());
    f.emit(event('a', { snapshotRevision: '3', updates: [{ conversationId: 'chat', epoch: 'epoch', sequence: '13',
      operations: [{ kind: 'item_remove', itemId: 'item', revision: '13' }],
      windowPatches: [{ windowId: 'a', previousRevision: '3', revision: '4' }] }] }));
    release.resolve(); await refreshing;
    expect(f.port.releaseItem.mock.calls.map(([, identity]) => identity.itemId)).toEqual(['other', 'item']);
    expect(f.client.getRecoveredItems('chat')).toEqual([]);
    f.port.refresh.mockResolvedValueOnce(snapshot('a', { sequence: '14', windowRevision: '5',
      items: [other], orderedItemIds: ['other'], totalCount: 1, endIndex: 1 }));
    await f.client.refresh('chat');
    await f.client.dispose();
  });

  it.each([false, true])('distribui reservas entre capabilities, primeira com buffers=%s', async buffers => {
    const f = itemRecoveryFixture();
    f.original.items = Array.from({ length: 9 }, (_, index) => ({ ...structuredClone(f.original.items[0]),
      itemId: `item-${index}`, turnId: `turn-${index}`, recoveryReference: { ...recoveryReference, turnId: `turn-${index}` } }));
    f.original.orderedItemIds = f.original.items.map(item => item.itemId);
    f.original.totalCount = f.original.endIndex = 9;
    f.port.readItem.mockImplementation(async (_window, identity) => structuredClone(f.original.items.find(item => item.itemId === identity.itemId)!));
    const reservations = new Map<string, Set<string>>();
    f.port.reserveItem.mockImplementation(async (window, identity) => {
      const held = reservations.get(window) ?? new Set<string>();
      if (held.size >= (buffers && window === 'a' ? 7 : 8)) throw new Error('reservation limit');
      held.add(identity.itemId); reservations.set(window, held);
      return new apidto.TimelineRecoveryItemReservation({ conversationId: 'chat', identity });
    });
    await f.open(); await f.open('b');
    await f.client.refresh('chat');
    expect(f.client.getRecoveredItems('chat')).toHaveLength(9);
    expect(reservations.get('a')?.size).toBe(buffers ? 7 : 8);
    expect(reservations.get('b')?.size).toBe(buffers ? 2 : 1);
    await f.client.dispose();
  });

  it.each([false, true])('não substitui referência reservada por outra época, segunda janela=%s', async second => {
    const f = itemRecoveryFixture();
    await f.open();
    const next = structuredClone(f.original);
    next.epoch = 'new'; next.windowRevision = '3'; next.sequence = next.items[0].revision = '12';
    f.port.refresh.mockResolvedValueOnce(next);
    await f.client.refresh('chat');
    const [old] = f.client.getRecoveredItems('chat');
    if (second) { f.port.open.mockResolvedValueOnce(snapshot('b', { epoch: 'new', sequence: '12' })); await f.open('b'); }
    f.port.readItem.mockResolvedValue(next.items[0]);
    f.port.refresh.mockImplementation(async window => second ? snapshot(window, { epoch: 'new', sequence: '13', windowRevision: '4' }) :
      { ...next, windowRevision: '4' });
    await f.client.refresh('chat');
    expect(f.port.reserveItem.mock.calls.filter(([window]) => window === 'a')).toHaveLength(1);
    expect(f.client.getRecoveredItems('chat')).toContainEqual(old);
    expect(f.client.getRecoveredItems('chat')).toHaveLength(second ? 2 : 1);
    expect(f.port.releaseItem).not.toHaveBeenCalled();
    await f.client.dispose();
  });

  it('descritor antigo não lê nem libera reserva da superfície reaberta', async () => {
    const f = itemRecoveryFixture();
    await f.open(); await f.client.refresh('chat');
    const [old] = f.client.getRecoveredItems('chat');
    await f.client.releaseRecoveredItem('chat', old);
    await f.client.close('chat', 'a');
    f.port.open.mockResolvedValueOnce({ ...f.original, windowId: 'new-window' });
    await f.open(); await f.client.refresh('chat');
    const [current] = f.client.getRecoveredItems('chat');
    f.port.readItem.mockClear(); f.port.releaseItem.mockClear();
    await expect(f.client.readRecoveredItem('chat', old)).rejects.toThrow('unavailable');
    await expect(f.client.releaseRecoveredItem('chat', old)).rejects.toThrow('unavailable');
    expect(f.port.readItem).not.toHaveBeenCalled(); expect(f.port.releaseItem).not.toHaveBeenCalled();
    expect(await f.client.readRecoveredItem('chat', current)).toEqual(f.original.items[0]);
    await f.client.dispose();
  });

  it('cleanup entre superfícies não sobrescreve RPC pendente de outra janela', async () => {
    const f = itemRecoveryFixture();
    await f.open(); await f.open('b');
    f.port.reserveItem.mockRejectedValue(new Error('offline'));
    f.port.releaseItem.mockRejectedValue(new Error('offline'));
    await expect(f.client.refresh('chat')).rejects.toThrow('cleanup failed');
    const release = deferred<void>();
    const reading = deferred<apidto.TimelineRecoveryPage>();
    f.port.releaseItem.mockReset().mockReturnValueOnce(release.promise);
    const cleanup = f.client.cleanupRecoveryItems('chat');
    const rejected = expect(cleanup).rejects.toThrow('busy');
    await vi.waitFor(() => expect(f.port.releaseItem).toHaveBeenCalledOnce());
    f.port.readRecovery.mockReturnValueOnce(reading.promise);
    f.port.reserveRecovery.mockResolvedValue(recoveryReceipt());
    const opening = f.client.openRecovery('chat', 'b', recoveryReference);
    await vi.waitFor(() => expect(f.port.readRecovery).toHaveBeenCalledOnce());
    release.resolve(); await rejected;
    await expect(f.client.refresh('chat')).rejects.toThrow('busy');
    reading.resolve(recoveryPage()); await opening;
    await f.client.cleanupRecoveryItems('chat');
    await f.client.dispose();
  });

  it('reserva antes de Refresh, promove sem reinserir e permite leitura/liberação explícita', async () => {
    const f = itemRecoveryFixture();
    await f.open();
    await f.client.refresh('chat');
    expect(f.port.reserveItem.mock.invocationCallOrder[0]).toBeLessThan(f.port.refresh.mock.invocationCallOrder[0]);
    expect(f.client.getWindow('chat', 'a')?.orderedItemIds).toEqual([]);
    expect(f.client.getItem('chat', 'a', 'item')).toBeUndefined();
    const [held] = f.client.getRecoveredItems('chat');
    expect(await f.client.readRecoveredItem('chat', held)).toEqual(f.original.items[0]);
    await expect(f.client.close('chat', 'a')).rejects.toThrow('explicit release');
    expect(f.client.getWindow('chat', 'a')).toBeDefined();
    await f.client.refresh('chat');
    expect(f.port.reserveItem).toHaveBeenCalledTimes(1);
    expect(f.client.getRecoveredItems('chat')).toEqual([held]);
    await f.client.releaseRecoveredItem('chat', held);
    expect(f.client.getRecoveredItems('chat')).toEqual([]);
    await f.client.close('chat', 'a');
  });

  it.each(['epoch', 'retry', 'persisted-retry'])('não confunde identidade do item com preservação: %s', async scenario => {
    const f = itemRecoveryFixture();
    await f.open();
    const next = structuredClone(f.original);
    next.sequence = next.items[0].revision = '12';
    next.windowRevision = '3';
    if (scenario === 'epoch') next.epoch = 'new';
    else {
      next.items[0].executionId = 'retry';
      next.items[0].recoveryReference!.executionId = 'retry';
      next.items[0].segments = [];
      if (scenario === 'persisted-retry') {
        delete next.items[0].recoveryReference;
        next.items[0].durability = 'persisted';
        next.items[0].state = 'completed';
      }
    }
    f.port.refresh.mockResolvedValueOnce(next);
    await f.client.refresh('chat');
    expect(f.client.getRecoveredItems('chat')).toHaveLength(1);
    expect(f.client.getItem('chat', 'a', 'item')).toEqual(next.items[0]);
    await f.client.dispose();
    expect(f.port.close).toHaveBeenCalledOnce();
  });

  it('reserva recusada não bloqueia snapshot persistido da mesma execução', async () => {
    const f = itemRecoveryFixture();
    await f.open();
    f.port.reserveItem.mockRejectedValueOnce(new Error('cache evicted'));
    const next = structuredClone(f.original);
    next.items[0].state = 'completed'; next.items[0].durability = 'persisted';
    delete next.items[0].recoveryReference;
    f.port.refresh.mockResolvedValueOnce(next);
    await f.client.refresh('chat');
    expect(f.client.getError('chat')).toBeUndefined();
    expect(f.client.getRecoveredItems('chat')).toEqual([]);
    expect(f.port.releaseItem).toHaveBeenCalledOnce();
  });

  it.each(['text', 'tool', 'metadata', 'identity', 'receipt'])('prova do DTO inteiro recusa alteração em %s', async field => {
    const f = itemRecoveryFixture();
    await f.open();
    const value = structuredClone(f.original.items[0]);
    if (field === 'text') value.segments[0].text += 'diferente';
    if (field === 'tool') value.segments[1].invocations![0].hasDetails = false;
    if (field === 'metadata') value.metadata.model = 'other';
    if (field === 'identity') value.recoveryReference!.executionId = 'other';
    if (field === 'receipt') f.port.reserveItem.mockResolvedValueOnce(new apidto.TimelineRecoveryItemReservation({ conversationId: 'other', identity: {} }));
    f.port.readItem.mockResolvedValueOnce(value);
    await expect(f.client.refresh('chat')).rejects.toThrow('ownership transfer');
    expect(f.client.getItem('chat', 'a', 'item')).toEqual(f.original.items[0]);
    expect(f.client.getRecoveredItems('chat')).toEqual([]);
    expect(f.port.releaseItem).toHaveBeenCalledOnce();
  });

  it('falha de cleanup conserva identidade e exige retry explícito antes de novo Refresh', async () => {
    const f = itemRecoveryFixture();
    await f.open();
    f.port.reserveItem.mockRejectedValueOnce(new Error('lost response'));
    f.port.releaseItem.mockRejectedValueOnce(new Error('offline'));
    await expect(f.client.refresh('chat')).rejects.toThrow('cleanup failed');
    await expect(f.client.refresh('chat')).rejects.toThrow('cleanup pending');
    await f.client.cleanupRecoveryItems('chat');
    expect(f.port.releaseItem.mock.calls[0]).toEqual(f.port.releaseItem.mock.calls[1]);
    await f.client.refresh('chat');
    expect(f.client.getRecoveredItems('chat')).toHaveLength(1);
  });

  it.each(['reserve', 'read', 'refresh'])('Close durante %s impede transferência para capability fechada', async stage => {
    const f = itemRecoveryFixture();
    await f.open();
    const entered = deferred<void>();
    const pending = deferred<void>();
    if (stage === 'reserve') f.port.reserveItem.mockImplementationOnce(async (_window, identity) => {
      entered.resolve(); await pending.promise; return new apidto.TimelineRecoveryItemReservation({ conversationId: 'chat', identity });
    });
    if (stage === 'read') f.port.readItem.mockImplementationOnce(async () => {
      entered.resolve(); await pending.promise; return f.original.items[0];
    });
    if (stage === 'refresh') f.port.refresh.mockImplementationOnce(async () => {
      entered.resolve(); await pending.promise; return snapshot();
    });
    const recovering = f.client.refresh('chat');
    const failure = expect(recovering).rejects.toThrow('ownership transfer');
    await entered.promise;
    await f.client.close('chat', 'a');
    pending.resolve();
    await failure;
    expect(f.client.getRecoveredItems('chat')).toEqual([]);
    expect(f.port.close).toHaveBeenCalledOnce();
  });

  it('falha de liberação mantém leitura e bloqueia Close; logout encerra capability', async () => {
    const f = itemRecoveryFixture();
    await f.open(); await f.client.refresh('chat');
    const [held] = f.client.getRecoveredItems('chat');
    f.port.releaseItem.mockRejectedValueOnce(new Error('offline'));
    await expect(f.client.releaseRecoveredItem('chat', held)).rejects.toThrow('offline');
    expect(await f.client.readRecoveredItem('chat', held)).toEqual(f.original.items[0]);
    await expect(f.client.close('chat', 'a')).rejects.toThrow('explicit release');
    await f.client.dispose();
    await expect(f.client.readRecoveredItem('chat', held)).rejects.toThrow('disposed');
  });

  it('leitura tardia não revive reserva liberada nem descritor de outra superfície', async () => {
    const f = itemRecoveryFixture();
    await f.open(); await f.open('b'); await f.client.refresh('chat');
    const [held] = f.client.getRecoveredItems('chat');
    await expect(f.client.readRecoveredItem('chat', { ...held, surfaceId: 'b' })).rejects.toThrow('unavailable');
    const pending = deferred<typeof f.original.items[0]>();
    f.port.readItem.mockReturnValueOnce(pending.promise);
    const reading = f.client.readRecoveredItem('chat', held);
    const rejected = expect(reading).rejects.toThrow('unavailable');
    await f.client.releaseRecoveredItem('chat', held);
    pending.resolve(f.original.items[0]);
    await rejected;
    expect(f.client.getRecoveredItems('chat')).toEqual([]);
  });
});
function recoveryPage(cursor = '') {
  return new apidto.TimelineRecoveryPage({ conversationId: 'chat', reference: recoveryReference, found: true, contentVersion: '3',
    chunks: [{ iteration: 0, generation: '1', part: 0, kind: 'text', byteOffset: 0, text: 'bytes' }], nextCursor: cursor });
}
function recoveryReceipt() {
  return new apidto.TimelineRecoveryReservation({ conversationId: 'chat', reference: recoveryReference, contentVersion: '3' });
}
function recoveryFixture() {
  const f = fixture();
  f.port.readRecovery.mockResolvedValue(recoveryPage());
  f.port.reserveRecovery.mockResolvedValue(recoveryReceipt());
  return f;
}

describe('recuperação explícita de buffers no cliente', () => {
  it('mantém a guarda de Refresh mesmo com reserva e leitura concluídas', async () => {
    const f = recoveryFixture();
    f.port.open.mockResolvedValueOnce(contentSnapshot(true));
    await f.open();
    f.emit(event('a', { syncRequired: true }));
    const reader = await f.client.openRecovery('chat', 'a', recoveryReference);
    expect((await reader.next())?.chunks[0].text).toBe('bytes');
    await expect(f.client.refresh('chat')).rejects.toThrow('ownership transfer');
    expect(f.client.getItem('chat', 'a', 'item')).toBeDefined();
    await f.client.closeRecovery('chat', 'a');
    expect(f.port.releaseRecovery).toHaveBeenCalledExactlyOnceWith('a', recoveryReference, '3');
    await f.client.close('chat', 'a');
  });

  it('permite um leitor por abertura e isola reservas de duas janelas', async () => {
    const f = recoveryFixture();
    await f.open('a');
    await f.open('b');
    const a = await f.client.openRecovery('chat', 'a', recoveryReference);
    await expect(f.client.openRecovery('chat', 'a', recoveryReference)).rejects.toThrow('unavailable');
    const b = await f.client.openRecovery('chat', 'b', recoveryReference);
    await f.client.close('chat', 'a');
    await expect(a.next()).rejects.toThrow('closed');
    expect((await b.next())?.chunks[0].text).toBe('bytes');
    expect(f.port.releaseRecovery).not.toHaveBeenCalled();
    await b.close();
    expect(f.port.releaseRecovery).toHaveBeenCalledExactlyOnceWith('b', recoveryReference, '3');
    await f.client.close('chat', 'b');
  });

  it('conserva cleanup ambíguo e bloqueia outro leitor até liberação explícita', async () => {
    const f = recoveryFixture();
    await f.open();
    f.port.reserveRecovery.mockRejectedValueOnce(new Error('resposta perdida'));
    f.port.releaseRecovery.mockRejectedValueOnce(new Error('release perdido'));
    await expect(f.client.openRecovery('chat', 'a', recoveryReference)).rejects.toThrow('cleanup failed');
    await expect(f.client.openRecovery('chat', 'a', recoveryReference)).rejects.toThrow('unavailable');
    expect(f.port.reserveRecovery).toHaveBeenCalledOnce();
    await f.client.closeRecovery('chat', 'a');
    const reader = await f.client.openRecovery('chat', 'a', recoveryReference);
    expect((await reader.next())?.contentVersion).toBe('3');
    await f.client.close('chat', 'a');
  });

  it.each(['primeira página', 'reserva', 'próxima página'])('Close invalida %s pendente e não ressuscita abertura', async phase => {
    const f = recoveryFixture();
    await f.open();
    const read = deferred<apidto.TimelineRecoveryPage>();
    const reserve = deferred<apidto.TimelineRecoveryReservation>();
    let reading: Promise<unknown>;
    if (phase === 'próxima página') {
      f.port.readRecovery.mockResolvedValueOnce(recoveryPage('next'));
      const reader = await f.client.openRecovery('chat', 'a', recoveryReference);
      await reader.next();
      f.port.readRecovery.mockReturnValueOnce(read.promise);
      reading = reader.next();
      await vi.waitFor(() => expect(f.port.readRecovery).toHaveBeenCalledTimes(2));
    } else {
      if (phase === 'primeira página') f.port.readRecovery.mockReturnValueOnce(read.promise);
      else f.port.reserveRecovery.mockReturnValueOnce(reserve.promise);
      reading = f.client.openRecovery('chat', 'a', recoveryReference);
      await vi.waitFor(() => expect(phase === 'reserva' ? f.port.reserveRecovery : f.port.readRecovery).toHaveBeenCalledOnce());
    }
    const rejected = expect(reading).rejects.toThrow('closed');
    const closing = f.client.close('chat', 'a');
    await vi.waitFor(() => expect(f.port.close).toHaveBeenCalledExactlyOnceWith('a'));
    read.resolve(recoveryPage());
    reserve.resolve(recoveryReceipt());
    await Promise.all([closing, rejected]);
    f.port.open.mockResolvedValueOnce(snapshot('new-window'));
    await f.open();
    expect(f.client.getWindow('chat', 'a')?.windowId).toBe('new-window');
    expect(f.port.releaseRecovery).not.toHaveBeenCalled();
    await f.client.close('chat', 'a');
  });

  it('handle de abertura antiga nunca fecha a nova capability', async () => {
    const f = recoveryFixture();
    await f.open();
    const old = await f.client.openRecovery('chat', 'a', recoveryReference);
    await f.client.close('chat', 'a');
    f.port.open.mockResolvedValueOnce(snapshot('new-window'));
    await f.open();
    await old.close();
    expect(f.port.close).toHaveBeenCalledExactlyOnceWith('a');
    await expect(old.next()).rejects.toThrow('closed');
    await f.client.close('chat', 'a');
  });
});

describe('Release concorrente com Close da capability', () => {
  it.each([true, false])('suprime rejeição tardia somente com Close confirmado: %s', async confirmed => {
    const f = recoveryFixture();
    await f.open();
    await f.client.openRecovery('chat', 'a', recoveryReference);
    const release = deferred<void>();
    f.port.releaseRecovery.mockReturnValueOnce(release.promise);
    const cleanup = f.client.closeRecovery('chat', 'a');
    const outcome = confirmed ? expect(cleanup).resolves.toBeUndefined() : expect(cleanup).rejects.toThrow('release tardio');
    await vi.waitFor(() => expect(f.port.releaseRecovery).toHaveBeenCalledOnce());
    if (!confirmed) f.port.close.mockRejectedValueOnce(new Error('close falhou'));
    const close = f.client.close('chat', 'a');
    if (!confirmed) await expect(close).rejects.toThrow('close falhou');
    else {
      await close;
      f.port.open.mockResolvedValueOnce(snapshot('new-window'));
      await f.open();
    }
    release.reject(new Error('release tardio'));
    await outcome;
    if (confirmed) {
      expect(f.client.getWindow('chat', 'a')?.windowId).toBe('new-window');
      const next = await f.client.openRecovery('chat', 'a', recoveryReference);
      expect((await next.next())?.chunks[0].text).toBe('bytes');
    }
    expect(f.port.releaseRecovery).toHaveBeenCalledOnce();
    await f.client.close('chat', 'a');
  });
});
