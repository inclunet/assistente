import type { ChatTimelineEvent } from '../types/chatTimelineEvents';
import { TimelineCacheBudget } from './timelineCacheBudget';
import { TimelineConversationState, type TimelineSnapshot, type TimelineStateResult } from './timelineConversationState';

export interface TimelinePosition {
  cursor: string;
  anchor: string;
  anchorItemId: string;
  direction: string;
  limit: number;
}
export interface TimelineTransport {
  listen(receive: (event: ChatTimelineEvent) => void): () => void;
  open(conversationId: string, surfaceId: string, scope: string | null, position: TimelinePosition): Promise<TimelineSnapshot>;
  refresh(windowId: string, revision: string, position?: TimelinePosition): Promise<TimelineSnapshot>;
  navigate(windowId: string, epoch: string, revision: string, mode: string, position: TimelinePosition): Promise<TimelineSnapshot>;
  retain(windowId: string, epoch: string, revision: string, itemIds: string[]): Promise<void>;
  close(windowId: string): Promise<void>;
}
interface Slot {
  scope: string | null;
  windowId?: string;
  baseline?: string;
  pending?: Promise<unknown>;
  closed: boolean;
  closing?: Promise<void>;
}
interface Conversation {
  state: TimelineConversationState;
  slots: Map<string, Slot>;
  candidate?: TimelineConversationState;
  recovery?: Promise<void>;
  error?: unknown;
}

/** Transporte por sessão. Conteúdo, ordem e revisão aplicada pertencem ao coordenador. */
export class ChatTimelineClient {
  private readonly budget = new TimelineCacheBudget();
  private readonly conversations = new Map<string, Conversation>();
  private readonly listeners = new Set<() => void>();
  private readonly unsubscribe: () => void;
  private disposed = false;
  private disposal?: Promise<void>;

  constructor(private readonly transport: TimelineTransport) {
    this.unsubscribe = transport.listen(event => {
      const conversation = this.conversations.get(event.conversationId);
      if (this.disposed || !conversation || (conversation.error && !conversation.candidate)) return;
      this.observe(conversation, (conversation.candidate ?? conversation.state).receive(event));
      this.notify();
    });
  }

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  }
  getWindow(conversationId: string, surfaceId: string) {
    return this.conversations.get(conversationId)?.state.getWindow(surfaceId);
  }
  getItem(conversationId: string, surfaceId: string, itemId: string) {
    return this.conversations.get(conversationId)?.state.getItem(surfaceId, itemId);
  }
  getError(conversationId: string): unknown { return this.conversations.get(conversationId)?.error; }

  async open(conversationId: string, surfaceId: string, scope: string | null, position: TimelinePosition): Promise<void> {
    this.assertActive();
    let conversation = this.conversations.get(conversationId);
    if (!conversation) {
      conversation = { state: new TimelineConversationState(conversationId, this.budget), slots: new Map() };
      this.conversations.set(conversationId, conversation);
    }
    if (conversation.recovery || conversation.error || conversation.slots.has(surfaceId)) throw new Error('Timeline surface unavailable');
    const owner = conversation;
    const token = owner.state.beginSnapshot(surfaceId, scope);
    const slot: Slot = { scope, closed: false };
    owner.slots.set(surfaceId, slot);
    return this.run(slot, async () => {
      try {
        const snapshot = await this.transport.open(conversationId, surfaceId, scope, position);
        slot.windowId = snapshot.windowId;
        slot.baseline = snapshot.windowRevision;
        if (slot.closed || this.disposed) { await this.closeRemote(slot); return; }
        const result = owner.state.acceptSnapshot(token, snapshot);
        if (!owner.state.getWindow(surfaceId)) throw new Error('Timeline snapshot rejected');
        this.observe(owner, result);
      } catch (error) {
        this.observe(owner, owner.state.close(surfaceId));
        slot.closed = true;
        try { await this.closeRemote(slot); }
        finally { if (owner.slots.get(surfaceId) === slot) owner.slots.delete(surfaceId); }
        throw error;
      } finally { this.notify(); }
    });
  }

  async navigate(conversationId: string, surfaceId: string, mode: string, position: TimelinePosition): Promise<void> {
    const { conversation, slot, window } = this.ready(conversationId, surfaceId);
    const token = conversation.state.beginSnapshot(surfaceId, slot.scope);
    return this.run(slot, async () => {
      try {
        const snapshot = await this.transport.navigate(window.windowId, window.epoch, window.revision, mode, position);
        slot.baseline = snapshot.windowRevision;
        if (slot.closed || this.disposed) return;
        this.observe(conversation, conversation.state.acceptSnapshot(token, snapshot));
      } catch (error) {
        if (!slot.closed && !this.disposed) this.observe(conversation, conversation.state.failSnapshot(token));
        throw error;
      } finally { this.notify(); }
    });
  }

  async setRetention(conversationId: string, surfaceId: string, itemIds: string[]): Promise<void> {
    const { slot, window } = this.ready(conversationId, surfaceId);
    return this.run(slot, () => this.transport.retain(window.windowId, window.epoch, window.revision, itemIds));
  }

  /** Refresh é a única recuperação de época. Falha parcial conserva a apresentação congelada. */
  refresh(conversationId: string, positions: ReadonlyMap<string, TimelinePosition> = new Map()): Promise<void> {
    this.assertActive();
    const conversation = this.conversations.get(conversationId);
    if (!conversation) return Promise.reject(new Error('Unknown timeline conversation'));
    if (conversation.recovery) return conversation.recovery;
    if ([...conversation.slots.values()].some(slot => slot.pending)) return Promise.reject(new Error('Timeline surface busy'));
    const candidate = new TimelineConversationState(conversationId, this.budget);
    conversation.candidate = candidate;
    const requests = [...conversation.slots].filter(([, slot]) => !slot.closed).map(([surface, slot]) => ({
      surface, slot, token: candidate.beginSnapshot(surface, slot.scope),
    }));
    conversation.error = undefined;
    const recovery = Promise.resolve().then(async () => {
      const outcomes = await Promise.allSettled(requests.map(async ({ surface, slot, token }) => {
        if (slot.closed || this.disposed) return;
        const snapshot = await this.transport.refresh(slot.windowId!, slot.baseline!, positions.get(surface));
        slot.baseline = snapshot.windowRevision;
        if (slot.closed || this.disposed) return;
        if (snapshot.windowId !== slot.windowId) throw new Error('Timeline capability mismatch');
        const result = candidate.acceptSnapshot(token, snapshot);
        if (result.kind !== 'applied') throw new Error('Timeline recovery snapshot rejected');
      }));
      if (this.disposed) return;
      const failed = outcomes.find((outcome, index) => outcome.status === 'rejected' && !requests[index].slot.closed);
      if (failed?.status === 'rejected' || conversation.error || candidate.frozenReason) {
        throw failed?.status === 'rejected' ? failed.reason : conversation.error ?? new Error('Timeline recovery rejected');
      }
      const previous = conversation.state;
      conversation.state = candidate;
      previous.dispose();
    }).catch(error => { if (!this.disposed) conversation.error = error; throw error; }).finally(() => {
      if (conversation.state !== candidate) candidate.dispose();
      conversation.candidate = undefined;
      conversation.recovery = undefined;
      this.notify();
    });
    conversation.recovery = recovery;
    return recovery;
  }

  async close(conversationId: string, surfaceId: string): Promise<void> {
    const conversation = this.conversations.get(conversationId);
    const slot = conversation?.slots.get(surfaceId);
    if (!conversation || !slot) return;
    slot.closed = true;
    const closed = conversation.state.close(surfaceId);
    this.observe(conversation, conversation.candidate ? conversation.candidate.close(surfaceId) : closed);
    this.notify();
    // Close precisa cancelar a consulta remota antes de aguardar a Promise dela.
    const closing = this.closeRemote(slot);
    const outcomes = await Promise.allSettled([closing, slot.pending]);
    if (conversation.slots.get(surfaceId) === slot) conversation.slots.delete(surfaceId);
    if (slot.closing) await slot.closing;
    if (outcomes[0].status === 'rejected') throw outcomes[0].reason;
  }

  dispose(): Promise<void> {
    if (this.disposal) return this.disposal;
    this.disposed = true;
    this.unsubscribe();
    this.listeners.clear();
    const closing = [...this.conversations].flatMap(([id, conversation]) =>
      [...conversation.slots.keys()].map(surface => this.close(id, surface)));
    for (const conversation of this.conversations.values()) {
      conversation.state.dispose();
      conversation.candidate?.dispose();
    }
    this.conversations.clear();
    this.disposal = Promise.allSettled(closing).then(results => {
      const failure = results.find(result => result.status === 'rejected');
      if (failure?.status === 'rejected') throw failure.reason;
    });
    return this.disposal;
  }

  private closeRemote(slot: Slot): Promise<void> {
    if (!slot.windowId) return Promise.resolve();
    return slot.closing ??= this.transport.close(slot.windowId);
  }
  private ready(conversationId: string, surfaceId: string) {
    this.assertActive();
    const conversation = this.conversations.get(conversationId);
    const slot = conversation?.slots.get(surfaceId);
    const window = conversation?.state.getWindow(surfaceId);
    if (!conversation || !slot || !window || slot.closed || slot.pending || conversation.recovery || conversation.error) {
      throw new Error('Timeline surface unavailable');
    }
    return { conversation, slot, window };
  }
  private run(slot: Slot, action: () => Promise<void>): Promise<void> {
    const pending = Promise.resolve().then(() => {
      if (slot.closed || this.disposed) return;
      return action();
    }).finally(() => { if (slot.pending === pending) slot.pending = undefined; });
    slot.pending = pending;
    return pending;
  }
  private observe(conversation: Conversation, result: TimelineStateResult) {
    if (result.kind === 'recover') conversation.error = new Error(`Timeline recovery required: ${result.reason}`);
  }
  private assertActive() { if (this.disposed) throw new Error('Timeline session disposed'); }
  private notify() { if (!this.disposed) for (const listener of this.listeners) listener(); }
}
