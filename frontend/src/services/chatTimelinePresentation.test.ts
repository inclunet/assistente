import { describe, expect, it, vi } from 'vitest';
import type { TimelineItem } from '../types/chatTimelineEvents';
import { ChatTimelineClient, type TimelineTransport } from './chatTimelineClient';
import { ChatTimelinePresentation } from './chatTimelinePresentation';
import type { TimelineSnapshot } from './timelineConversationState';

function item(id: string, scope: string | null = null): TimelineItem {
  return { itemId: id, scopeParentId: scope, turnId: null,
    representativeMessageId: id, revision: '1', state: 'completed', durability: 'persisted',
    segments: [{ segmentId: 'text', kind: 'text', conclusion: true, text: id }],
    metadata: { createdAt: '2026-01-01T00:00:00Z', source: '', childCount: 0,
      cacheReadTokens: 0, cacheWriteTokens: 0, cacheMissTokens: 0,
      role: 'user', pinned: false, model: '', promptTokens: 0, completionTokens: 0, totalTokens: 0 } };
}

function fixture() {
  const scopes = new Map<string, string | null>();
  const modes = new Map<string, 'historical' | 'followTail'>();
  let nextItems = [item('second'), item('first')];
  const snapshot = (surface: string, scope: string | null): TimelineSnapshot => ({
    conversationId: 'conversation', scopeParentId: scope, windowId: surface, epoch: 'epoch', windowRevision: '1', sequence: '1', mode: modes.get(surface) ?? 'historical',
    orderedItemIds: nextItems.map(value => value.itemId), items: nextItems, retainedItemIds: [], retentionBlocked: false,
    totalCount: nextItems.length, startIndex: 0, endIndex: nextItems.length, hasBefore: false, hasAfter: false, beforeCursor: '', afterCursor: '',
  });
  const transport: TimelineTransport = {
    listen: () => () => {}, open: vi.fn(async (_conversation, surface, scope) => { scopes.set(surface, scope); return snapshot(surface, scope); }),
    navigate: vi.fn(async (window, _epoch, _revision, mode) => { modes.set(window, mode); return snapshot(window, scopes.get(window) ?? null); }), refresh: vi.fn(async window => snapshot(window, null)), retain: vi.fn(async (windowId, epoch, windowRevision) => ({ windowId, epoch, windowRevision })), close: vi.fn(async () => {}),
    readRecovery: vi.fn(), reserveRecovery: vi.fn(), releaseRecovery: vi.fn(), reserveItem: vi.fn(), readItem: vi.fn(), releaseItem: vi.fn(),
  };
  const client = new ChatTimelineClient(transport, surface => surface);
  const presentation = new ChatTimelinePresentation(client, 'conversation', 'surface');
  return { presentation, transport, modes, setItems(items: TimelineItem[]) { nextItems = items; } };
}

describe('canonical presentation', () => {
  it('keeps backend order and preserves message references while only navigation changes', async () => {
    const f = fixture();
    await f.presentation.open();
    const first = f.presentation.nodes();
    expect(first.map(node => node.itemId)).toEqual(['second', 'first']);
    expect(f.presentation.nodes()[0].message).toBe(first[0].message);
    expect(f.presentation.getMessage('second')).toBe(first[0].message);
    await f.presentation.close();
    expect(f.transport.close).toHaveBeenCalledWith('surface');
  });

  it('opens a thread in its own scope and closes both capabilities', async () => {
    const f = fixture();
    await f.presentation.open();
    f.setItems([item('child', 'second')]);
    await f.presentation.openThread('second');
    expect(f.transport.open).toHaveBeenLastCalledWith('conversation', 'surface:thread:second', 'second', expect.any(Object));
    expect(f.presentation.nodes()[0].children?.[0].itemId).toBe('child');
    await f.presentation.close();
    expect(f.transport.close).toHaveBeenCalledTimes(2);
  });

  it('uses the visual identity even when a response has no persisted action target', async () => {
    const f = fixture();
    f.setItems([{ ...item('turn:question'), state: 'preparing', representativeMessageId: null,
      metadata: { ...item('turn:question').metadata, role: 'assistant' } }]);
    await f.presentation.open();
    const message = f.presentation.nodes()[0].message;
    expect(message.id).toBe('turn:question');
    expect(message.isStreaming).toBe(true);
    expect(message.timelineItem?.representativeMessageId).toBeNull();
    await f.presentation.close();
  });
});

it('retains a thread item and its ancestor in their respective windows', async () => {
 const f = fixture(); await f.presentation.open();
 f.setItems([item('child', 'second')]); await f.presentation.openThread('second');
 await f.presentation.retain(['child']);
 expect(f.transport.retain).toHaveBeenCalledWith('surface', 'epoch', '1', ['second']);
 expect(f.transport.retain).toHaveBeenCalledWith('surface:thread:second', 'epoch', '1', ['child']);
 await f.presentation.close();
});

it('retries thread Open after its failed capability was cleaned up', async () => {
 const f = fixture(); await f.presentation.open();
 vi.mocked(f.transport.open).mockRejectedValueOnce(new Error('offline'));
 await expect(f.presentation.openThread('second')).rejects.toThrow('offline');
 f.setItems([item('child', 'second')]); await f.presentation.openThread('second');
 expect(f.presentation.nodes()[0].children?.[0].itemId).toBe('child');
 await f.presentation.close();
});

it('closes descendant capabilities even after their parent item leaves the page', async () => {
 const f = fixture(); await f.presentation.open();
 f.setItems([item('child', 'second')]); await f.presentation.openThread('second');
 f.setItems([item('grandchild', 'child')]); await f.presentation.openThread('child');
 f.setItems([item('other', 'second')]); await f.presentation.navigate('end', 'second');
 await f.presentation.closeThread('second');
 expect(f.transport.close).toHaveBeenCalledWith('surface:thread:child');
 expect(f.transport.close).toHaveBeenCalledWith('surface:thread:second');
 await f.presentation.close();
});

it('assembles the whole conclusion using only the references supplied by the backend', async () => {
 const f = fixture(); const response = item('answer');
 response.segments = [{ segmentId: 'intermediate', kind: 'text', text: 'not conclusion' }, { segmentId: 'first', kind: 'text', text: 'hello ' }, { segmentId: 'last', kind: 'text', text: 'world' }];
 response.segments[1].conclusion = response.segments[2].conclusion = true;
 f.setItems([response]); await f.presentation.open();
 expect(f.presentation.getMessage('answer')?.content).toBe('hello world');
 await f.presentation.close();
});

it('recovers a failed initial Open without remounting the surface', async () => {
 const f = fixture(); vi.mocked(f.transport.open).mockRejectedValueOnce(new Error('offline'));
 await expect(f.presentation.open()).rejects.toThrow('offline');
 await f.presentation.refresh();
 expect(f.presentation.nodes().map(node => node.itemId)).toEqual(['second', 'first']);
 expect(f.transport.open).toHaveBeenCalledTimes(2);
 expect(f.transport.navigate).toHaveBeenLastCalledWith('surface', 'epoch', '1', 'followTail', expect.any(Object));
 await f.presentation.close();
});

it('normal Refresh preserves the backend position and mode instead of choosing a new historical window', async () => {
 const f = fixture(); await f.presentation.open();
 await f.presentation.refresh();
 expect(f.transport.refresh).toHaveBeenCalledWith('surface', '1', undefined);
 await f.presentation.close();
});

it('accepts an empty navigation snapshot as the authoritative window', async () => {
 const f = fixture(); await f.presentation.open();
 f.setItems([]);
 await f.presentation.navigate('newer');
 expect(f.presentation.nodes()).toEqual([]);
 expect(f.presentation.window?.metadata).toMatchObject({ totalCount: 0, hasBefore: false, hasAfter: false });
 await f.presentation.close();
});

it.each(['start', 'end'] as const)('navigates to %s using the backend anchor and replaces membership', async direction => {
 const f = fixture(); await f.presentation.open();
 f.modes.set('surface', direction === 'end' ? 'historical' : 'followTail');
 await f.presentation.refresh();
 expect(f.presentation.window?.mode).toBe(direction === 'end' ? 'historical' : 'followTail');
 expect(f.presentation.window?.metadata).toMatchObject({ hasBefore: false, hasAfter: false });
 f.setItems([item('destination')]);
 await f.presentation.navigate(direction);
 expect(f.transport.navigate).toHaveBeenLastCalledWith('surface', 'epoch', '1', direction === 'end' ? 'followTail' : 'historical',
   expect.objectContaining({ anchor: direction, cursor: '', direction: direction === 'end' ? 'before' : 'after' }));
 expect(f.presentation.nodes().map(node => node.itemId)).toEqual(['destination']);
 expect(f.presentation.window?.mode).toBe(direction === 'end' ? 'followTail' : 'historical');
 await f.presentation.close();
});

it('keeps navigation loading local to the requesting presentation', async () => {
 const f = fixture(); await f.presentation.open();
 const other = new ChatTimelinePresentation(f.presentation.client, 'conversation', 'other');
 await other.open();
 let release!: () => void;
 const gate = new Promise<void>(resolve => { release = resolve; });
 const navigate = vi.mocked(f.transport.navigate).getMockImplementation()!;
 vi.mocked(f.transport.navigate).mockImplementationOnce(async (...args) => {
   await gate;
   return navigate(...args);
 });
 const pending = f.presentation.navigate('older');
 expect(f.presentation.loading).toBe(true);
 expect(other.loading).toBe(false);
 release(); await pending;
 expect(f.presentation.loading).toBe(false);
 await f.presentation.close(); await other.close();
});

it.each(['preparing', 'completed'])('renders an empty conclusion received as a JSON array (%s)', async state => {
 const f = fixture(); const response = item('answer');
 response.state = state; response.segments = state === 'preparing' ? [] : [{ segmentId: 'tool', kind: 'tool_invocations', invocations: [] }];
 f.setItems([JSON.parse(JSON.stringify(response))]); await f.presentation.open();
 expect(f.presentation.getMessage('answer')?.content).toBe('');
 await f.presentation.close();
});

it('exposes the exact published copy capability without reserving recovery bytes', async () => {
  const f = fixture();
  const value = { ...item('turn:question'), turnId: 'question', state: 'failed', durability: 'volatile', representativeMessageId: null,
    recoveryReference: { projectionId: 'projection', incarnation: 'incarnation', turnId: 'question', executionId: 'execution' } };
  f.setItems([value]);
  await f.presentation.open();
  expect(f.presentation.getCopyTarget(value.itemId)).toEqual({ windowId: 'surface', identity: { itemId: value.itemId, epoch: 'epoch', revision: value.revision, reference: value.recoveryReference } });
  expect(f.transport.reserveItem).not.toHaveBeenCalled();
  expect(f.presentation.getCopyTarget('outside')).toBeUndefined();
  const closing = f.presentation.close();
  expect(f.presentation.getCopyTarget(value.itemId)).toBeUndefined();
  await closing;
});

it.each([
  ['2026-01-01T00:00:00.123456789Z', Date.UTC(2026, 0, 1, 0, 0, 0, 123)],
  ['2026-01-01T00:00:00.123456789-03:00', Date.UTC(2026, 0, 1, 3, 0, 0, 123)],
])('converte a data %s em milissegundos somente para exibição', async (createdAt, expected) => {
  const f = fixture();
  const original = item('date');
  f.setItems([{ ...original, metadata: { ...original.metadata, createdAt } }]);
  await f.presentation.open();
  expect(f.presentation.nodes()[0].message.timestamp).toBe(expected);
  expect(f.presentation.nodes()[0].message.createdAt).toBe(createdAt);
  await f.presentation.close();
});
