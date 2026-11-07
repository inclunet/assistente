import { afterEach, describe, expect, it, vi } from 'vitest';
import { createStore } from 'zustand/vanilla';
import { useStore } from 'zustand';
import { act, renderHook, waitFor } from '@testing-library/react';
import { apidto } from '@wailsjs/go/models';
import { canonicalTestItem, wireFixtureItem } from './chatTimelineTestFixture';

const mocks = vi.hoisted(() => ({
  navigate: vi.fn(),
  refresh: vi.fn(),
  reserveItem: vi.fn(), readItem: vi.fn(), releaseItem: vi.fn().mockResolvedValue(undefined),
  read: vi.fn(), reserve: vi.fn(), release: vi.fn().mockResolvedValue(undefined),
  open: vi.fn(), close: vi.fn().mockResolvedValue(undefined), eventsOn: vi.fn(), unsubscribe: vi.fn(),
}));
const auth = createStore<{ isAuthenticated: boolean; user: { userId: string; sessionId: string } | null }>(() => ({
  isAuthenticated: true, user: { userId: 'user', sessionId: 'session' },
}));
vi.mock('../store/authStore', () => ({ useAuthStore: Object.assign(
  <T,>(selector: (state: ReturnType<typeof auth.getState>) => T) => useStore(auth, selector), auth,
) }));
vi.mock('@wailsjs/go/wailsapi/ChatTimeline', () => ({
  ReserveRecoveryItem: mocks.reserveItem, ReadRecoveryItem: mocks.readItem, ReleaseRecoveryItem: mocks.releaseItem,
  ReadRecoveryPage: mocks.read, ReserveRecovery: mocks.reserve, ReleaseRecovery: mocks.release,
  Open: mocks.open, Close: mocks.close, Refresh: mocks.refresh, Navigate: mocks.navigate, SetRetention: vi.fn(),
}));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: mocks.eventsOn }));

afterEach(() => {
  auth.setState({ isAuthenticated: false, user: null });
  vi.clearAllMocks();
});

describe('chatTimelineWails', () => {
  it('limpa a conversa excluída com o hook ainda montado e preserva outra conversa', async () => {
    auth.setState({ isAuthenticated: true, user: { userId: 'user', sessionId: 'deletion' } });
    mocks.eventsOn.mockReturnValue(mocks.unsubscribe);
    mocks.open.mockImplementation(async (request: apidto.OpenTimelineRequest) => new apidto.SubscribedTimelineSnapshot({
      conversationId: request.conversationId, scopeParentId: null, epoch: 'epoch', windowId: request.conversationId,
      windowRevision: '1', sequence: '1', mode: 'followTail',
      items: [canonicalTestItem('item', 'conteúdo recebido')], orderedItemIds: ['item'], retainedItemIds: [],
      totalCount: 1, startIndex: 0, endIndex: 1, hasBefore: false, hasAfter: false, retentionBlocked: false,
    }));
    mocks.navigate.mockImplementation(async (windowId: string) => new apidto.SubscribedTimelineSnapshot({
      conversationId: windowId, scopeParentId: null, epoch: 'epoch', windowId,
      windowRevision: '2', sequence: '2', mode: 'followTail',
      items: [canonicalTestItem('item', 'conteúdo recebido')], orderedItemIds: ['item'], retainedItemIds: [],
      totalCount: 1, startIndex: 0, endIndex: 1, hasBefore: false, hasAfter: false, retentionBlocked: false,
    }));
    const { useChatTimelinePresentation } = await import('../components/chat/useChatTimelinePresentation');
    const { result, unmount } = renderHook(() => ({
      deleted: useChatTimelinePresentation('deleted', 'surface'),
      other: useChatTimelinePresentation('other', 'surface'),
    }));
    await waitFor(() => {
      expect(result.current.deleted.presentation?.window?.revision).toBe('2');
      expect(result.current.other.presentation?.window?.revision).toBe('2');
      expect(result.current.deleted.presentation?.error).toBeUndefined();
    });
    const otherWindow = result.current.other.presentation?.window;
    expect(result.current.deleted.nodes).toHaveLength(1);
    const otherNodes = result.current.other.nodes;
    const previousRevision = result.current.deleted.revision;
    const deletedListener = mocks.eventsOn.mock.calls.find(([name]) => name === 'conversation:deleted')![1];
    act(() => deletedListener({ conversation_id: 'deleted' }));
    expect(result.current.deleted.presentation?.window).toBeUndefined();
    expect(result.current.deleted.nodes).toEqual([]);
    expect(result.current.deleted.revision).toBeGreaterThan(previousRevision);
    expect(result.current.other.presentation?.window).toBe(otherWindow);
    expect(result.current.other.nodes).toBe(otherNodes);
    await waitFor(() => expect(mocks.close).toHaveBeenCalledExactlyOnceWith('deleted'));
    unmount();
    await waitFor(() => expect(mocks.close).toHaveBeenCalledWith('other'));
  });

  it('atualiza as janelas montadas após limpeza e preserva outra conversa', async () => {
    auth.setState({ isAuthenticated: true, user: { userId: 'user', sessionId: 'cleared' } });
    mocks.eventsOn.mockReturnValue(mocks.unsubscribe);
    mocks.open.mockImplementation(async (request: apidto.OpenTimelineRequest) => new apidto.SubscribedTimelineSnapshot({
      conversationId: request.conversationId, scopeParentId: null, epoch: 'epoch', windowId: request.conversationId,
      windowRevision: '1', sequence: '1', mode: 'followTail',
      items: [canonicalTestItem('item', 'conteúdo recebido')], orderedItemIds: ['item'], retainedItemIds: [],
      totalCount: 1, startIndex: 0, endIndex: 1, hasBefore: false, hasAfter: false, retentionBlocked: false,
    }));
    mocks.navigate.mockImplementation(async (windowId: string) => new apidto.SubscribedTimelineSnapshot({
      conversationId: windowId, scopeParentId: null, epoch: 'epoch', windowId,
      windowRevision: '2', sequence: '2', mode: 'followTail',
      items: [canonicalTestItem('item', 'conteúdo recebido')], orderedItemIds: ['item'], retainedItemIds: [],
      totalCount: 1, startIndex: 0, endIndex: 1, hasBefore: false, hasAfter: false, retentionBlocked: false,
    }));
    let cleared = false;
    mocks.refresh.mockImplementation(async (windowId: string) => {
      cleared = true;
      return new apidto.SubscribedTimelineSnapshot({ conversationId: windowId, scopeParentId: null, epoch: 'epoch', windowId,
        windowRevision: '3', sequence: '3', mode: 'historical', items: [], orderedItemIds: [], retainedItemIds: [],
        totalCount: 0, startIndex: 0, endIndex: -1, hasBefore: false, hasAfter: false, retentionBlocked: false });
    });
    const initialNavigate = mocks.navigate.getMockImplementation()!;
    mocks.navigate.mockImplementation(async (windowId: string) => cleared && windowId === 'deleted'
      ? new apidto.SubscribedTimelineSnapshot({ conversationId: windowId, scopeParentId: null, epoch: 'epoch', windowId,
        windowRevision: '4', sequence: '4', mode: 'followTail', items: [], orderedItemIds: [], retainedItemIds: [],
        totalCount: 0, startIndex: 0, endIndex: -1, hasBefore: false, hasAfter: false, retentionBlocked: false })
      : initialNavigate(windowId));
    const { useChatTimelinePresentation } = await import('../components/chat/useChatTimelinePresentation');
    const { result, unmount } = renderHook(() => ({
      deleted: useChatTimelinePresentation('deleted', 'surface'),
      other: useChatTimelinePresentation('other', 'surface'),
    }));
    await waitFor(() => {
      expect(result.current.deleted.presentation?.window?.revision).toBe('2');
      expect(result.current.other.presentation?.window?.revision).toBe('2');
      expect(result.current.deleted.presentation?.error).toBeUndefined();
    });
    const otherWindow = result.current.other.presentation?.window;
    expect(result.current.deleted.nodes).toHaveLength(1);
    const otherNodes = result.current.other.nodes;
    const previousRevision = result.current.deleted.revision;
    const deletedListener = mocks.eventsOn.mock.calls.find(([name]) => name === 'conversation:cleared')![1];
    act(() => deletedListener({ conversation_id: 'deleted' }));
    await waitFor(() => expect(result.current.deleted.presentation?.window?.revision).toBe('4'));
    expect(result.current.deleted.presentation?.window?.mode).toBe('followTail');
    expect(result.current.deleted.nodes).toEqual([]);
    expect(result.current.deleted.revision).toBeGreaterThan(previousRevision);
    expect(result.current.other.presentation?.window).toBe(otherWindow);
    expect(result.current.other.nodes).toBe(otherNodes);
    expect(mocks.refresh).toHaveBeenCalledTimes(1);
    unmount();
    await waitFor(() => expect(mocks.close).toHaveBeenCalledWith('other'));
  });

  it('descarta Open pendente da conversa excluída e fecha sua capability tardia', async () => {
    auth.setState({ isAuthenticated: true, user: { userId: 'user', sessionId: 'pending-deletion' } });
    mocks.eventsOn.mockReturnValue(mocks.unsubscribe);
    let resolve!: (snapshot: apidto.SubscribedTimelineSnapshot) => void;
    mocks.open.mockReturnValue(new Promise<apidto.SubscribedTimelineSnapshot>(yes => { resolve = yes; }));
    const { getChatTimelineClient } = await import('./chatTimelineWails');
    const client = getChatTimelineClient();
    const opening = client.open('chat', 'surface', null, { cursor: '', anchor: 'tail', anchorItemId: '', direction: '', limit: 20 });
    await Promise.resolve();
    mocks.eventsOn.mock.calls.find(([name]) => name === 'conversation:deleted')![1]({ conversation_id: 'chat' });
    expect(client.hasSurface('chat', 'surface')).toBe(false);
    resolve(new apidto.SubscribedTimelineSnapshot({ windowId: 'late' }));
    await opening;
    expect(mocks.close).toHaveBeenCalledExactlyOnceWith('late');
    expect(client.getWindow('chat', 'surface')).toBeUndefined();
  });

  it('integra reserva, leitura, Refresh e liberação com DTOs Wails', async () => {
    auth.setState({ isAuthenticated: true, user: { userId: 'user', sessionId: 'item-recovery' } });
    mocks.eventsOn.mockReturnValue(mocks.unsubscribe);
    const reference = { projectionId: 'p', incarnation: 'i', turnId: 'turn', executionId: 'execution' };
    const item = new apidto.CanonicalItem({ recoveryReference: reference, executionId: 'execution', itemId: 'item',
      scopeParentId: null, turnId: 'turn', representativeMessageId: null, revision: '1', state: 'interrupted',
      durability: 'volatile', segments: [{ segmentId: 'text', kind: 'text', text: 'preservar' }],
      metadata: { createdAt: '2026-01-01T00:00:00Z', role: 'assistant', model: '', pinned: false, promptTokens: 0, completionTokens: 0, totalTokens: 0 } });
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
      representativeMessageId: null, segments: [], });
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
    expect(mocks.unsubscribe).toHaveBeenCalledTimes(3);
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
      expect(mocks.unsubscribe).toHaveBeenCalledTimes(3);
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

it('normaliza defaults igualmente em snapshot, operações, patches e recuperação', async () => {
  const { normalizeTimelineItem, normalizeTimelineSnapshot, normalizeTimelineEvent } = await import('./chatTimelineWails');
  const { sameRecoveryItem } = await import('./timelineRecoveryItem');
  const wire = { itemId: 'turn:q', revision: '1', representativeMessageId: null, segments: [],
    metadata: { createdAt: '2026-01-01T00:00:00Z', role: 'assistant' } };
  const recovered = normalizeTimelineItem(wire);
  const snapshot = normalizeTimelineSnapshot(new apidto.SubscribedTimelineSnapshot({ items: [wire] }));
  const event = normalizeTimelineEvent({ surfaceId: 's', windowId: 'w', conversationId: 'conversation', epoch: 'e', snapshotRevision: '1', syncRequired: false,
    updates: [{ conversationId: 'conversation', epoch: 'e', sequence: '1',
      operations: [{ kind: 'item_upsert', itemId: wire.itemId, revision: '1', item: wire }],
      windowPatches: [{ windowId: 'w', previousRevision: '0', revision: '1', items: [wire] }],
    }] });
  for (const value of [snapshot.items[0], event.updates[0].operations[0].item!, event.updates[0].windowPatches[0].items![0]]) {
    expect(sameRecoveryItem(value, recovered)).toBe(true);
    expect(value.representativeMessageId).toBeNull();
    expect(value.state).toBe('');
    expect(value.durability).toBe('persisted');
    expect(value.segments.filter(segment => segment.conclusion)).toEqual([]);
    expect(value.metadata).toMatchObject({ model: '', pinned: false, totalTokens: 0, childCount: 0 });
  }
  const populated = normalizeTimelineItem({ ...wire, metadata: { ...wire.metadata, pinned: true, childCount: 2, model: 'model', cacheReadTokens: 7, promptTokens: 8, completionTokens: 9, totalTokens: 17 } });
  expect(populated.metadata).toMatchObject({ pinned: true, childCount: 2, model: 'model', cacheReadTokens: 7, promptTokens: 8, completionTokens: 9, totalTokens: 17 });
  expect(wire.metadata).not.toHaveProperty('totalTokens');
});

it('aceita recuperação no teto wire mesmo quando os defaults locais excedem esse teto', async () => {
  const { normalizeTimelineItem } = await import('./timelineWire');
  const { sameRecoveryItem } = await import('./timelineRecoveryItem');
  const { metadata, ...identity } = wireFixtureItem(canonicalTestItem('turn:limit', ''));
  const wire = { ...identity, scopeParentId: undefined, durability: undefined,
    metadata: { createdAt: metadata.createdAt, role: metadata.role } };
  const limit = 512 * 1024;
  const remaining = limit - new TextEncoder().encode(JSON.stringify(wire)).length;
  wire.segments[0].text = 'x'.repeat(remaining);
  expect(new TextEncoder().encode(JSON.stringify(wire)).length).toBe(limit);
  const normalized = normalizeTimelineItem(wire);
  expect(new TextEncoder().encode(JSON.stringify(normalized)).length).toBeGreaterThan(limit);
  expect(sameRecoveryItem(normalized, normalizeTimelineItem(JSON.parse(JSON.stringify(wire))))).toBe(true);
  const changed = { ...normalized, metadata: { ...normalized.metadata, role: 'user' } };
  expect(sameRecoveryItem(normalized, changed)).toBe(false);
  wire.segments = [{ ...wire.segments[0], text: wire.segments[0].text + 'x' }];
  expect(() => sameRecoveryItem(normalized, normalizeTimelineItem(wire))).toThrow('Timeline recovery item exceeds limit');
});
