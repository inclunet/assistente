import { afterEach, describe, expect, it, vi } from 'vitest';
import { createStore } from 'zustand/vanilla';
import { apidto } from '@wailsjs/go/models';

const mocks = vi.hoisted(() => ({
  refresh: vi.fn(),
  reserveItem: vi.fn(), readItem: vi.fn(), releaseItem: vi.fn().mockResolvedValue(undefined),
  read: vi.fn(), reserve: vi.fn(), release: vi.fn().mockResolvedValue(undefined),
  open: vi.fn(), close: vi.fn().mockResolvedValue(undefined), eventsOn: vi.fn(), unsubscribe: vi.fn(),
}));
const auth = createStore<{ isAuthenticated: boolean; user: { userId: string; sessionId: string } | null }>(() => ({
  isAuthenticated: true, user: { userId: 'user', sessionId: 'session' },
}));
vi.mock('../store/authStore', () => ({ useAuthStore: auth }));
vi.mock('@wailsjs/go/wailsapi/ChatTimeline', () => ({
  ReserveRecoveryItem: mocks.reserveItem, ReadRecoveryItem: mocks.readItem, ReleaseRecoveryItem: mocks.releaseItem,
  ReadRecoveryPage: mocks.read, ReserveRecovery: mocks.reserve, ReleaseRecovery: mocks.release,
  Open: mocks.open, Close: mocks.close, Refresh: mocks.refresh, Navigate: vi.fn(), SetRetention: vi.fn(),
}));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: mocks.eventsOn }));

afterEach(() => {
  auth.setState({ isAuthenticated: false, user: null });
  vi.clearAllMocks();
});

describe('chatTimelineWails', () => {
  it('integra reserva, leitura, Refresh e liberação com DTOs Wails', async () => {
    auth.setState({ isAuthenticated: true, user: { userId: 'user', sessionId: 'item-recovery' } });
    mocks.eventsOn.mockReturnValue(mocks.unsubscribe);
    const reference = { projectionId: 'p', incarnation: 'i', turnId: 'turn', executionId: 'execution' };
    const item = new apidto.CanonicalItem({ recoveryReference: reference, executionId: 'execution', itemId: 'item',
      conversationId: 'chat', scopeParentId: null, turnId: 'turn', kind: 'response', replyToItemId: 'question',
      representativeMessageId: null, messageIds: [], position: 'opaque', revision: '1', state: 'interrupted',
      durability: 'volatile', segments: [{ segmentId: 'text', kind: 'text', text: 'preservar' }],
      metadata: { origin: 'runtime', role: 'assistant', model: '', pinned: false, promptTokens: 0, completionTokens: 0, totalTokens: 0 } });
    const base = { conversationId: 'chat', scopeParentId: null, epoch: 'epoch', windowId: 'window',
      windowRevision: '1', sequence: '1', mode: 'historical', items: [item], orderedItemIds: ['item'],
      retainedItemIds: [], totalCount: 1, startIndex: 0, endIndex: 1, hasBefore: false, hasAfter: false,
      retentionBlocked: false, beforeCursor: '', afterCursor: '' };
    mocks.open.mockResolvedValue(new apidto.SubscribedTimelineSnapshot(base));
    mocks.refresh.mockResolvedValue(new apidto.SubscribedTimelineSnapshot({ ...base, windowRevision: '2', sequence: '2', items: [], orderedItemIds: [] }));
    mocks.reserveItem.mockImplementation(async (_window: string, identity: apidto.TimelineRecoveryItemRequest) =>
      new apidto.TimelineRecoveryItemReservation({ conversationId: 'chat', identity }));
    mocks.readItem.mockResolvedValue(item);
    const { getChatTimelineClient } = await import('./chatTimelineWails');
    const client = getChatTimelineClient();
    await client.open('chat', 'surface', null, { cursor: '', anchor: 'tail', anchorItemId: '', direction: '', limit: 20 });
    await client.refresh('chat');
    const [held] = client.getRecoveredItems('chat');
    expect((await client.readRecoveredItem('chat', held)).segments[0].text).toBe('preservar');
    await client.releaseRecoveredItem('chat', held);
    for (const rpc of [mocks.reserveItem, mocks.readItem, mocks.releaseItem]) {
      expect(rpc.mock.calls[0][0]).toBe('window');
      expect(rpc.mock.calls[0][1]).toBeInstanceOf(apidto.TimelineRecoveryItemRequest);
      expect(rpc.mock.calls[0][1]).toMatchObject({ reference, itemId: 'item', epoch: 'epoch', revision: '1' });
    }
    await client.close('chat', 'surface');
  });
  it('normaliza nullables de DTO sem fabricar identidade ou conteúdo', async () => {
    const { normalizeTimelineSnapshot } = await import('./chatTimelineWails');
    const value = normalizeTimelineSnapshot(new apidto.SubscribedTimelineSnapshot({
      windowId: 'window', items: [{ itemId: 'item' }],
    }));
    expect(value.scopeParentId).toBeNull();
    expect(value.items[0]).toMatchObject({ itemId: 'item', scopeParentId: null, turnId: null,
      representativeMessageId: null, segments: [], messageIds: [] });
    expect(value.orderedItemIds).toEqual([]);
    expect(mocks.eventsOn).not.toHaveBeenCalled();
  });

  it('reutiliza cliente da sessão e descarta Open tardio depois de trocar sessão', async () => {
    auth.setState({ isAuthenticated: true, user: { userId: 'user', sessionId: 'first' } });
    mocks.eventsOn.mockReturnValue(mocks.unsubscribe);
    let resolve!: (value: apidto.SubscribedTimelineSnapshot) => void;
    mocks.open.mockReturnValue(new Promise<apidto.SubscribedTimelineSnapshot>(yes => { resolve = yes; }));
    const { getChatTimelineClient } = await import('./chatTimelineWails');
    const client = getChatTimelineClient();
    expect(getChatTimelineClient()).toBe(client);
    const opening = client.open('chat', 'surface', null, { cursor: '', anchor: 'tail', anchorItemId: '', direction: '', limit: 20 });
    await Promise.resolve();
    expect(mocks.open.mock.calls[0][0]).toBeInstanceOf(apidto.OpenTimelineRequest);
    expect(mocks.open.mock.calls[0][0]).toMatchObject({ conversationId: 'chat', surfaceId: expect.any(String),
      scopeParentId: null, cursor: '', anchor: 'tail', anchorItemId: '', direction: '', limit: 20 });
    auth.setState({ user: { userId: 'user', sessionId: 'second' } });
    expect(mocks.unsubscribe).toHaveBeenCalledOnce();
    expect(getChatTimelineClient()).not.toBe(client);
    resolve(new apidto.SubscribedTimelineSnapshot({ windowId: 'late' }));
    await opening;
    expect(mocks.close).toHaveBeenCalledExactlyOnceWith('late');
    expect(client.getWindow('chat', 'surface')).toBeUndefined();
  });

  it('getter valida identidade antes do listener de limpeza quando outro assinante chama primeiro', async () => {
    auth.setState({ isAuthenticated: true, user: { userId: 'user', sessionId: 'first' } });
    mocks.eventsOn.mockReturnValue(mocks.unsubscribe);
    const { getChatTimelineClient } = await import('./chatTimelineWails');
    let fromEarlierSubscriber: ReturnType<typeof getChatTimelineClient> | undefined;
    const unsubscribe = auth.subscribe(() => { fromEarlierSubscriber = getChatTimelineClient(); });
    try {
      const old = getChatTimelineClient();
      auth.setState({ user: { userId: 'user', sessionId: 'second' } });
      expect(fromEarlierSubscriber).toBeDefined();
      expect(fromEarlierSubscriber).not.toBe(old);
      expect(getChatTimelineClient()).toBe(fromEarlierSubscriber);
      expect(mocks.unsubscribe).toHaveBeenCalledOnce();
      await expect(old.open('chat', 'surface', null, { cursor: '', anchor: 'end', anchorItemId: '', direction: '', limit: 20 }))
        .rejects.toThrow('disposed');
    } finally { unsubscribe(); }
  });

  it('encaminha payload do listener Wails ao coordenador', async () => {
    auth.setState({ isAuthenticated: true, user: { userId: 'user', sessionId: 'session' } });
    mocks.eventsOn.mockReturnValue(mocks.unsubscribe);
    mocks.open.mockResolvedValue(new apidto.SubscribedTimelineSnapshot({ conversationId: 'chat',
      windowId: 'window', windowRevision: '1', epoch: 'epoch', sequence: '10', mode: 'historical',
      items: [], orderedItemIds: [], retainedItemIds: [], retentionBlocked: false, totalCount: 0,
      startIndex: 0, endIndex: 0, hasBefore: false, hasAfter: false, beforeCursor: '', afterCursor: '' }));
    const { getChatTimelineClient } = await import('./chatTimelineWails');
    const client = getChatTimelineClient();
    await client.open('chat', 'surface', null, { cursor: '', anchor: 'end', anchorItemId: '', direction: '', limit: 20 });
    expect(mocks.eventsOn.mock.calls[0][0]).toBe('chat:timeline');
    mocks.eventsOn.mock.calls[0][1]({ conversationId: 'chat', surfaceId: mocks.open.mock.calls[0][0].surfaceId, windowId: 'window', epoch: 'epoch',
      snapshotRevision: '1', syncRequired: false, updates: [{ conversationId: 'chat', epoch: 'epoch',
        sequence: '11', operations: [], windowPatches: [{ windowId: 'window', previousRevision: '1', revision: '2' }] }] });
    expect(client.getWindow('chat', 'surface')?.revision).toBe('2');
  });
});

it('conecta leitor aos DTOs Wails e invalida reserva na troca de sessão', async () => {
  auth.setState({ isAuthenticated: true, user: { userId: 'user', sessionId: 'reader' } });
  mocks.eventsOn.mockReturnValue(mocks.unsubscribe);
  mocks.open.mockResolvedValue(new apidto.SubscribedTimelineSnapshot({ conversationId: 'chat',
    windowId: 'window', epoch: 'epoch', sequence: '1', windowRevision: '1', mode: 'historical',
    totalCount: 0, startIndex: 0, endIndex: 0, hasBefore: false, hasAfter: false, retentionBlocked: false }));
  const reference = { projectionId: 'p', incarnation: 'i', turnId: 't', executionId: 'e' };
  mocks.read.mockResolvedValue(new apidto.TimelineRecoveryPage({
    conversationId: 'chat', reference, contentVersion: '2', found: true, chunks: [], nextCursor: '',
  }));
  mocks.reserve.mockResolvedValue(new apidto.TimelineRecoveryReservation({ conversationId: 'chat', reference, contentVersion: '2' }));
  const { getChatTimelineClient } = await import('./chatTimelineWails');
  const client = getChatTimelineClient();
  await client.open('chat', 'surface', null, { cursor: '', anchor: 'tail', anchorItemId: '', direction: '', limit: 20 });
  const reader = await client.openRecovery('chat', 'surface', reference);
  expect(mocks.read.mock.calls[0][1]).toBeInstanceOf(apidto.TimelineRecoveryPageRequest);
  expect(mocks.read.mock.calls[0][1]).toMatchObject({ reference, cursor: '' });
  expect(mocks.reserve.mock.calls[0][1]).toBeInstanceOf(apidto.TimelineRecoveryReservationRequest);
  expect(mocks.reserve.mock.calls[0][1]).toMatchObject({ reference, contentVersion: '2' });
  await reader.close();
  expect(mocks.release.mock.calls[0][1]).toBeInstanceOf(apidto.TimelineRecoveryReservationRequest);
  expect(mocks.release.mock.calls[0][1]).toMatchObject({ reference, contentVersion: '2' });
  const held = await client.openRecovery('chat', 'surface', reference);
  auth.setState({ user: { userId: 'user', sessionId: 'new' } });
  await expect(held.next()).rejects.toThrow('closed');
  await vi.waitFor(() => expect(mocks.close).toHaveBeenCalledExactlyOnceWith('window'));
  expect(mocks.release).toHaveBeenCalledOnce();
});
