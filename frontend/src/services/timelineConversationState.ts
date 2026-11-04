import type { ChatTimelineEvent, TimelineItem, TimelineWindowMetadata } from '../types/chatTimelineEvents';
import { prepareTimelineBatch, revision, validItem, type TimelineBatchWindow, type TimelineCacheEntry } from './timelineBatch';
import { TimelineCacheBudget } from './timelineCacheBudget';

/** Wire normalizado pelo adaptador RPC; nullable não usa os tipos de evento gerados. */
export interface TimelineSnapshot extends TimelineWindowMetadata {
  conversationId: string;
  scopeParentId: string | null;
  windowId: string;
  windowRevision: string;
  epoch: string;
  sequence: string;
  mode: string;
  items: TimelineItem[];
  orderedItemIds: string[];
}

export interface TimelineSnapshotRequest {
  readonly surfaceId: string;
  readonly scopeParentId: string | null;
}

export interface TimelineSurfaceWindow extends TimelineBatchWindow {
  snapshotRevision: string;
  mode: string;
}

type Reason = 'snapshot' | 'epoch' | 'gap' | 'patch' | 'content' | 'overflow' | 'retention';
export type TimelineStateResult = { kind: 'applied' | 'buffered' | 'ignored' } | { kind: 'recover'; reason: Reason };
interface Surface {
  scopeParentId: string | null;
  window?: TimelineSurfaceWindow;
  request?: { token: TimelineSnapshotRequest; minimum: bigint };
  recovery?: Reason;
}
interface RecordEntry {
  entry: TimelineCacheEntry;
  // Somente apresentação de uma janela atrasada, nunca entrada do reducer.
  previousItem?: TimelineItem;
}
interface Buffered {
  event: ChatTimelineEvent;
  bytes: number;
  batches: number;
}

const encoder = new TextEncoder();
const bytesOf = (value: unknown) => encoder.encode(JSON.stringify(value)).byteLength;
const protectedItem = (item: TimelineItem | null | undefined) => item &&
  (item.durability === 'volatile' || item.state === 'preparing' || item.state === 'streaming');

/**
 * Dono síncrono de uma conversa/época. O adaptador registra o listener antes de
 * beginSnapshot/Open. RPCs, autorização e notificações de UI ficam fora daqui.
 * Uma instância congelada conserva a apresentação até uma substituta receber
 * snapshots válidos; dispose é obrigatório no logout e após essa substituição.
 */
export class TimelineConversationState {
  private readonly surfaces = new Map<string, Surface>();
  private readonly records = new Map<string, RecordEntry>();
  private readonly openingRequests = new WeakSet<TimelineSnapshotRequest>();
  private readonly retiredWindows = new Set<string>();
  private retiredBytes = 0;
  private referenced = new Set<string>();
  private buffered: Buffered[] = [];
  private bufferBytes = 0;
  private bufferBatches = 0;
  private floor = 0n;
  private highest = 0n;
  private nextCollection?: bigint;
  private epoch?: string;
  private stopped?: Reason;
  private disposed = false;
  private replacementReferences = new Set<string>();

  constructor(readonly conversationId: string, private readonly budget: TimelineCacheBudget, replacementOf?: TimelineConversationState) {
    if (replacementOf) {
      for (const [id, record] of replacementOf.records) {
        if (protectedItem(record.entry.item)) this.replacementReferences.add(id);
      }
    }
  }

  finishReplacement(): void {
    this.replacementReferences.clear();
    this.collect();
  }

  get frozenReason(): Reason | undefined { return this.stopped; }
  get recoveryReason(): Reason | undefined {
    return this.stopped ?? [...this.surfaces.values()].find(surface => surface.recovery)?.recovery;
  }
  get cachedItems(): number { return this.records.size; }
  get hasProtectedItems(): boolean { return [...this.records.values()].some(record => protectedItem(record.entry.item)); }
  protectedItems(): { epoch: string; item: TimelineItem }[] {
    if (!this.epoch) return [];
    return [...this.records.values()].flatMap(record => protectedItem(record.entry.item)
      ? [{ epoch: this.epoch!, item: record.entry.item! }] : []);
  }
  protectedItemsLostBy(replacement: TimelineConversationState): { epoch: string; item: TimelineItem }[] {
    const lost: { epoch: string; item: TimelineItem }[] = [];
    for (const [id, record] of this.records) {
      if (!protectedItem(record.entry.item)) continue;
      const next = replacement.records.get(id);
      const before = record.entry.item?.recoveryReference;
      const after = next?.entry.item?.recoveryReference;
      const expectedExecution = before?.executionId ?? record.entry.item?.executionId;
      const otherExecution = next?.entry.item && ((!expectedExecution || next.entry.item.executionId !== expectedExecution) ||
        (before && after && (before.projectionId !== after.projectionId || before.incarnation !== after.incarnation ||
          before.turnId !== after.turnId || before.executionId !== after.executionId)));
      if (!next || replacement.epoch !== this.epoch || otherExecution || revision(next.entry.revision) < revision(record.entry.revision)) {
        lost.push({ epoch: this.epoch!, item: record.entry.item! });
      }
    }
    return lost;
  }
  get bufferedBatches(): number { return this.bufferBatches; }

  getWindow(surfaceId: string): TimelineSurfaceWindow | undefined {
    return this.surfaces.get(surfaceId)?.window;
  }

  getItem(surfaceId: string, itemId: string): TimelineItem | undefined {
    const window = this.getWindow(surfaceId);
    if (!window?.orderedItemIds.includes(itemId)) return undefined;
    const record = this.records.get(itemId);
    if (record?.entry.item) return record.entry.item;
    return record && revision(window.sequence) < revision(record.entry.revision) ? record.previousItem : undefined;
  }

  beginSnapshot(surfaceId: string, scopeParentId: string | null): TimelineSnapshotRequest {
    if (this.disposed || this.stopped) throw new Error('Timeline state requires replacement');
    let surface = this.surfaces.get(surfaceId);
    if (surface && surface.scopeParentId !== scopeParentId) throw new Error('Close surface before changing scope');
    if (!surface) {
      surface = { scopeParentId };
      this.surfaces.set(surfaceId, surface);
    }
    const token = Object.freeze({ surfaceId, scopeParentId });
    if (!surface.window) this.openingRequests.add(token);
    surface.request = { token, minimum: surface.window ? revision(surface.window.sequence) : this.floor };
    return token;
  }

  failSnapshot(token: TimelineSnapshotRequest): TimelineStateResult {
    const surface = this.surfaces.get(token.surfaceId);
    if (surface?.request?.token !== token) return { kind: 'ignored' };
    surface.request = undefined;
    surface.recovery = 'snapshot';
    this.pruneBuffer();
    this.collect();
    return { kind: 'recover', reason: 'snapshot' };
  }

  acceptSnapshot(token: TimelineSnapshotRequest, snapshot: TimelineSnapshot): TimelineStateResult {
    const surface = this.surfaces.get(token.surfaceId);
    if (this.disposed || this.stopped) return { kind: 'ignored' };
    if (surface?.request?.token !== token) {
      this.retireDiscardedOpen(token, snapshot);
      return this.stopped ? { kind: 'recover', reason: this.stopped } : { kind: 'ignored' };
    }
    if (snapshot.conversationId !== this.conversationId || snapshot.scopeParentId !== surface.scopeParentId ||
        !snapshot.windowId || !snapshot.epoch || this.retiredWindows.has(snapshot.windowId) ||
        (surface.window && snapshot.windowId !== surface.window.windowId) ||
        [...this.surfaces.values()].some(other => other !== surface && other.window?.windowId === snapshot.windowId)) {
      return this.rejectSnapshot(token, snapshot);
    }
    if (this.epoch && this.epoch !== snapshot.epoch) return this.freeze('epoch');
    const writes = new Map<string, TimelineCacheEntry>();
    let sequence: bigint;
    try {
      sequence = revision(snapshot.sequence);
      revision(snapshot.windowRevision);
      if (sequence < surface.request.minimum || sequence < this.floor ||
          new Set(snapshot.orderedItemIds).size !== snapshot.orderedItemIds.length ||
          new Set(snapshot.items.map(item => item.itemId)).size !== snapshot.items.length ||
          snapshot.items.length !== snapshot.orderedItemIds.length) throw new Error('snapshot');
      for (const item of snapshot.items) {
        validItem(item, snapshot, sequence);
        if (!snapshot.orderedItemIds.includes(item.itemId) || item.scopeParentId !== snapshot.scopeParentId) throw new Error('scope');
        const previous = this.records.get(item.itemId)?.entry;
        if (!previous || revision(previous.revision) < revision(item.revision)) {
          writes.set(item.itemId, { revision: item.revision, item: structuredClone(item) });
        }
      }
      for (const id of snapshot.orderedItemIds) {
        const entry = writes.get(id) ?? this.records.get(id)?.entry;
        if (!id || !entry?.item || entry.item.scopeParentId !== snapshot.scopeParentId) throw new Error('membership');
      }
    } catch {
      return this.rejectSnapshot(token, snapshot);
    }
    this.epoch = snapshot.epoch;
    this.write(writes);
    surface.window = {
      conversationId: this.conversationId, scopeParentId: snapshot.scopeParentId,
      windowId: snapshot.windowId, epoch: snapshot.epoch, revision: snapshot.windowRevision,
      snapshotRevision: snapshot.windowRevision, sequence: snapshot.sequence,
      mode: snapshot.mode, orderedItemIds: [...snapshot.orderedItemIds],
      metadata: {
        retainedItemIds: [...snapshot.retainedItemIds], retentionBlocked: snapshot.retentionBlocked,
        totalCount: snapshot.totalCount, startIndex: snapshot.startIndex, endIndex: snapshot.endIndex,
        hasBefore: snapshot.hasBefore, hasAfter: snapshot.hasAfter,
        beforeCursor: snapshot.beforeCursor, afterCursor: snapshot.afterCursor,
      },
    };
    surface.request = undefined;
    surface.recovery = undefined;
    if (sequence > this.highest) this.highest = sequence;
    const pending = this.buffered.filter(value => value.event.windowId === snapshot.windowId);
    this.removeBuffered(value => value.event.windowId === snapshot.windowId);
    let recovery: TimelineStateResult | undefined;
    for (const value of pending) {
      const result = this.receive(value.event);
      if (result.kind === 'recover') {
        recovery = result;
        break;
      }
    }
    this.pruneBuffer();
    this.collect();
    return this.stopped ? { kind: 'recover', reason: this.stopped } : recovery ?? { kind: 'applied' };
  }

  receive(event: ChatTimelineEvent): TimelineStateResult {
    if (this.disposed || this.stopped || event.conversationId !== this.conversationId ||
        this.retiredWindows.has(event.windowId)) return { kind: 'ignored' };
    const surface = [...this.surfaces.values()].find(value => value.window?.windowId === event.windowId);
    if (!surface) {
      return [...this.surfaces.values()].some(value => value.request && !value.window)
        ? this.buffer(event) : { kind: 'ignored' };
    }
    const window = surface.window!;
    try {
      const baseline = revision(event.snapshotRevision);
      if (baseline < revision(window.snapshotRevision)) return { kind: 'ignored' };
      if (surface.request) return this.buffer(event);
      if (event.epoch !== this.epoch) return this.freeze('epoch');
      if (surface.recovery) return { kind: 'recover', reason: surface.recovery };
      if (baseline !== revision(window.snapshotRevision)) throw new Error('baseline');
    } catch {
      surface.recovery = 'snapshot';
      return { kind: 'recover', reason: 'snapshot' };
    }
    if (event.syncRequired) {
      surface.recovery = 'snapshot';
      return { kind: 'recover', reason: 'snapshot' };
    }
    let collect = false;
    for (const update of event.updates) {
      const result = prepareTimelineBatch(surface.window!, update, id => this.records.get(id)?.entry);
      if (result.kind === 'recover') {
        surface.recovery = result.reason;
        if (collect) this.collect();
        return result;
      }
      if (result.kind === 'duplicate') continue;
      collect = this.write(result.writes) || collect;
      surface.window = { ...surface.window!, ...result.window };
      const sequence = revision(result.window.sequence);
      if (sequence > this.highest) this.highest = sequence;
      collect ||= update.windowPatches.length > 0 || [...result.writes.values()].some(entry => !entry.item);
    }
    if (collect || (this.nextCollection !== undefined && this.safeFloor() >= this.nextCollection)) this.collect();
    return this.stopped ? { kind: 'recover', reason: this.stopped } : { kind: 'applied' };
  }

  close(surfaceId: string, knownWindowId?: string): TimelineStateResult {
    if (this.disposed) return { kind: 'ignored' };
    const window = this.getWindow(surfaceId);
    if (window) this.retireWindow(window.windowId);
    if (knownWindowId && knownWindowId !== window?.windowId &&
        ![...this.surfaces.entries()].some(([id, surface]) => id !== surfaceId && surface.window?.windowId === knownWindowId)) {
      this.retireWindow(knownWindowId);
    }
    this.surfaces.delete(surfaceId);
    this.pruneBuffer();
    this.collect();
    return this.stopped ? { kind: 'recover', reason: this.stopped } : { kind: 'applied' };
  }

  dispose(): void {
    for (const record of this.records.values()) this.budget.pin(record);
    this.records.clear();
    this.referenced.clear();
    this.surfaces.clear();
    this.retiredWindows.clear();
    this.retiredBytes = 0;
    this.replacementReferences.clear();
    this.buffered = [];
    this.bufferBytes = this.bufferBatches = 0;
    this.disposed = true;
  }

  private write(writes: ReadonlyMap<string, TimelineCacheEntry>): boolean {
    let collect = false;
    for (const [id, entry] of writes) {
      collect ||= !this.referenced.has(id);
      const record = this.records.get(id) ?? { entry };
      this.budget.pin(record);
      if (!entry.item && record.entry.item) record.previousItem = record.entry.item;
      if (entry.item) record.previousItem = undefined;
      record.entry = entry;
      this.records.set(id, record);
    }
    return collect;
  }

  private buffer(event: ChatTimelineEvent): TimelineStateResult {
    const bytes = bytesOf(event);
    const batches = Math.max(1, event.updates.length);
    if (this.bufferBatches + batches >= 256 || this.bufferBytes + bytes >= 2 * 1024 * 1024) return this.freeze('overflow');
    this.buffered.push({ event: structuredClone(event), bytes, batches });
    this.bufferBatches += batches;
    this.bufferBytes += bytes;
    return { kind: 'buffered' };
  }

  private retireWindow(windowId: string): void {
    this.removeBuffered(value => value.event.windowId === windowId);
    if (this.retiredWindows.has(windowId)) return;
    const bytes = encoder.encode(windowId).byteLength;
    if (this.retiredWindows.size >= 256 || this.retiredBytes + bytes > 2 * 1024 * 1024) {
      this.freeze('retention');
      return;
    }
    this.retiredWindows.add(windowId);
    this.retiredBytes += bytes;
  }

  private retireDiscardedOpen(token: TimelineSnapshotRequest, snapshot: TimelineSnapshot): void {
    // Refresh/Navigate continuam apontando para a capability viva. Só o Open
    // emitido por esta instância pode descobrir uma capability a descartar.
    if (this.openingRequests.has(token) && snapshot.conversationId === this.conversationId &&
        snapshot.scopeParentId === token.scopeParentId && snapshot.windowId &&
        ![...this.surfaces.values()].some(value => value.window?.windowId === snapshot.windowId)) {
      this.retireWindow(snapshot.windowId);
    }
  }

  private rejectSnapshot(token: TimelineSnapshotRequest, snapshot: TimelineSnapshot): TimelineStateResult {
    this.retireDiscardedOpen(token, snapshot);
    return this.stopped ? { kind: 'recover', reason: this.stopped } : this.failSnapshot(token);
  }

  private removeBuffered(remove: (value: Buffered) => boolean): void {
    this.buffered = this.buffered.filter(value => {
      if (!remove(value)) return true;
      this.bufferBytes -= value.bytes;
      this.bufferBatches -= value.batches;
      return false;
    });
  }

  private pruneBuffer(): void {
    const pendingOpen = [...this.surfaces.values()].some(surface => surface.request && !surface.window);
    const pendingWindows = new Set([...this.surfaces.values()].filter(surface => surface.request).map(surface => surface.window?.windowId));
    const knownWindows = new Set([...this.surfaces.values()].map(surface => surface.window?.windowId));
    this.removeBuffered(value => !pendingWindows.has(value.event.windowId) &&
      (knownWindows.has(value.event.windowId) || !pendingOpen));
  }

  private freeze(reason: Reason): TimelineStateResult {
    this.stopped = reason;
    for (const surface of this.surfaces.values()) {
      surface.request = undefined;
      surface.recovery = reason;
    }
    this.buffered = [];
    this.bufferBytes = this.bufferBatches = 0;
    return { kind: 'recover', reason };
  }

  private collect(): void {
    const minimum = this.safeFloor();
    const referenced = new Set(this.replacementReferences);
    for (const surface of this.surfaces.values()) {
      if (surface.window) {
        for (const id of surface.window.orderedItemIds) referenced.add(id);
        for (const id of surface.window.metadata.retainedItemIds) referenced.add(id);
      }
    }
    if (minimum > this.floor) this.floor = minimum;
    this.referenced = referenced;
    let deferredItems = 0;
    let deferredBytes = 0;
    this.nextCollection = undefined;
    // Remova todas as novas referências da LRU antes de liberar qualquer outro
    // registro: uma liberação pode expulsar um item visitado depois neste índice.
    for (const [id, record] of this.records) {
      if (referenced.has(id) || protectedItem(record.entry.item)) this.budget.pin(record);
    }
    for (const [id, record] of this.records) {
      if (referenced.has(id) || protectedItem(record.entry.item)) {
        continue;
      }
      record.previousItem = undefined;
      if (revision(record.entry.revision) > this.floor) {
        this.budget.pin(record);
        deferredItems++;
        deferredBytes += bytesOf(record.entry);
        const sequence = revision(record.entry.revision);
        if (this.nextCollection === undefined || sequence < this.nextCollection) this.nextCollection = sequence;
      } else if (!record.entry.item) {
        this.budget.pin(record);
        this.records.delete(id);
      } else {
        this.budget.release(record, bytesOf(record.entry.item), () => this.records.delete(id));
      }
    }
    if (deferredItems > this.budget.maxItems || deferredBytes > this.budget.maxBytes) this.freeze('retention');
  }

  private safeFloor(): bigint {
    let minimum = this.highest;
    for (const surface of this.surfaces.values()) {
      if (surface.window && revision(surface.window.sequence) < minimum) minimum = revision(surface.window.sequence);
      if (surface.request && surface.request.minimum < minimum) minimum = surface.request.minimum;
    }
    return minimum;
  }
}
