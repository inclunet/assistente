import type { ChatTimelineEvent } from '../types/chatTimelineEvents';
import { TimelineCacheBudget } from './timelineCacheBudget';
import { revision } from './timelineBatch';
import { TimelineConversationState, type TimelineSnapshot } from './timelineConversationState';

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
  registrationId: string;
  scope: string | null;
  windowId?: string;
  baseline?: string;
  epoch?: string;
  pending?: Promise<unknown>;
  closed: boolean;
  closing?: Promise<void>;
}
interface Conversation {
  state: TimelineConversationState;
  slots: Map<string, Slot>;
  candidate?: TimelineConversationState;
  recovery?: Promise<void>;
  recoveryError?: Error;
}

/** Transporte por sessão. Conteúdo, ordem e revisão aplicada pertencem ao coordenador. */
export class ChatTimelineClient {
  private readonly budget = new TimelineCacheBudget();
  private readonly conversations = new Map<string, Conversation>();
  private readonly listeners = new Set<() => void>();
  private readonly unsubscribe: () => void;
  private disposed = false;
  private disposal?: Promise<void>;

  constructor(private readonly transport: TimelineTransport, private readonly registrationId = (_surface: string): string => crypto.randomUUID()) {
    this.unsubscribe = transport.listen(event => {
      const conversation = this.conversations.get(event.conversationId);
      if (this.disposed || !conversation || (conversation.recoveryError && !conversation.candidate)) return;
      const slot = [...conversation.slots.values()].find(value => !value.closed && value.registrationId === event.surfaceId);
      if (!slot || (slot.windowId && slot.windowId !== event.windowId)) return;
      (conversation.candidate ?? conversation.state).receive(event);
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
  getError(conversationId: string): Error | undefined {
    const conversation = this.conversations.get(conversationId);
    if (!conversation) return;
    const reason = (conversation.candidate ?? conversation.state).recoveryReason;
    return conversation.recoveryError ?? (reason ? new Error(`Timeline recovery required: ${reason}`) : undefined);
  }

  async open(conversationId: string, surfaceId: string, scope: string | null, position: TimelinePosition): Promise<void> {
    this.assertActive();
    this.collectIdle();
    let conversation = this.conversations.get(conversationId);
    if (!conversation) {
      conversation = { state: new TimelineConversationState(conversationId, this.budget), slots: new Map() };
      this.conversations.set(conversationId, conversation);
    }
    if (conversation.recovery || this.getError(conversationId) || conversation.slots.has(surfaceId)) throw new Error('Timeline surface unavailable');
    const owner = conversation;
    const token = owner.state.beginSnapshot(surfaceId, scope);
    const slot: Slot = { scope, closed: false, registrationId: this.registrationId(surfaceId) };
    owner.slots.set(surfaceId, slot);
    return this.run(slot, async () => {
      try {
        const snapshot = await this.transport.open(conversationId, slot.registrationId, scope, position);
        slot.windowId = snapshot.windowId;
        slot.baseline = snapshot.windowRevision;
        slot.epoch = snapshot.epoch;
        if (slot.closed || this.disposed) {
          owner.state.acceptSnapshot(token, snapshot);
          await this.closeRemote(slot);
          return;
        }
        owner.state.acceptSnapshot(token, snapshot);
        if (!owner.state.getWindow(surfaceId)) throw new Error('Timeline snapshot rejected');
      } catch (error) {
        if (slot.closed || this.disposed) throw error;
        owner.state.close(surfaceId);
        slot.closed = true;
        await this.closeRemote(slot);
        if (owner.slots.get(surfaceId) === slot) owner.slots.delete(surfaceId);
        this.collectIdle();
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
        slot.epoch = snapshot.epoch;
        if (slot.closed || this.disposed) return;
        conversation.state.acceptSnapshot(token, snapshot);
      } catch (error) {
        if (!slot.closed && !this.disposed) conversation.state.failSnapshot(token);
        throw error;
      } finally { this.notify(); }
    });
  }

  async setRetention(conversationId: string, surfaceId: string, itemIds: string[]): Promise<void> {
    const { slot, window } = this.ready(conversationId, surfaceId, itemIds.length === 0);
    // Liberação não recupera o visual. Usa somente checkpoints comprovados;
    // revisão desconhecida continua sujeita ao CAS do backend, sem retry implícito.
    const useSnapshot = this.getError(conversationId) && slot.epoch && slot.baseline &&
      (slot.epoch !== window.epoch || revision(slot.baseline) > revision(window.revision));
    const epoch = useSnapshot ? slot.epoch! : window.epoch;
    const currentRevision = useSnapshot ? slot.baseline! : window.revision;
    return this.run(slot, () => this.transport.retain(window.windowId, epoch, currentRevision, itemIds));
  }

  /** Refresh é a única recuperação de época. Falha parcial conserva a apresentação congelada. */
  refresh(conversationId: string, positions: ReadonlyMap<string, TimelinePosition> = new Map()): Promise<void> {
    this.assertActive();
    const conversation = this.conversations.get(conversationId);
    if (!conversation) return Promise.reject(new Error('Unknown timeline conversation'));
    if (conversation.recovery) return conversation.recovery;
    if (![...conversation.slots.values()].some(slot => !slot.closed)) return Promise.reject(new Error('Timeline recovery requires an active surface'));
    if ([...conversation.slots.values()].some(slot => slot.pending)) return Promise.reject(new Error('Timeline surface busy'));
    const candidate = new TimelineConversationState(conversationId, this.budget);
    conversation.candidate = candidate;
    const requests = [...conversation.slots].filter(([, slot]) => !slot.closed).map(([surface, slot]) => ({
      surface, slot, token: candidate.beginSnapshot(surface, slot.scope),
    }));
    conversation.recoveryError = undefined;
    const recovery = Promise.resolve().then(async () => {
      const outcomes = await Promise.allSettled(requests.map(async ({ surface, slot, token }) => {
        if (slot.closed || this.disposed) return;
        const snapshot = await this.transport.refresh(slot.windowId!, slot.baseline!, positions.get(surface));
        slot.baseline = snapshot.windowRevision;
        slot.epoch = snapshot.epoch;
        if (slot.closed || this.disposed) return;
        if (snapshot.windowId !== slot.windowId) throw new Error('Timeline capability mismatch');
        const result = candidate.acceptSnapshot(token, snapshot);
        if (result.kind !== 'applied') throw new Error('Timeline recovery snapshot rejected');
      }));
      if (this.disposed) return;
      const failed = outcomes.find((outcome, index) => outcome.status === 'rejected' && !requests[index].slot.closed);
      if (failed?.status === 'rejected' || candidate.recoveryReason) {
        throw failed?.status === 'rejected' ? failed.reason : new Error(`Timeline recovery rejected: ${candidate.recoveryReason}`);
      }
      if (conversation.state.wouldDiscardProtectedItems(candidate)) throw new Error('Timeline protected content requires ownership transfer');
      conversation.recoveryError = undefined;
      const previous = conversation.state;
      conversation.state = candidate;
      previous.dispose();
    }).catch(error => {
      if (!this.disposed) conversation.recoveryError = error instanceof Error ? error : Object.assign(new Error('Timeline recovery failed'), { cause: error });
      throw error;
    }).finally(() => {
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
    conversation.state.close(surfaceId);
    conversation.candidate?.close(surfaceId, slot.windowId);
    this.notify();
    // Close precisa cancelar a consulta remota antes de aguardar a Promise dela.
    const closing = this.closeRemote(slot);
    const outcomes = await Promise.allSettled([closing, slot.pending]);
    if (outcomes[0].status === 'rejected') throw outcomes[0].reason;
    // Open pode ter descoberto a capability enquanto aguardávamos seu retorno.
    if (slot.windowId && !slot.closing) throw outcomes[1].status === 'rejected' ? outcomes[1].reason : new Error('Timeline cleanup incomplete');
    if (slot.closing) await slot.closing;
    if (conversation.slots.get(surfaceId) === slot) conversation.slots.delete(surfaceId);
    this.collectIdle();
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
    return slot.closing ??= this.transport.close(slot.windowId).catch(error => {
      slot.closing = undefined;
      throw error;
    });
  }
  private collectIdle() {
    for (const [id, conversation] of this.conversations) {
      if (conversation.slots.size || conversation.recovery || conversation.candidate) continue;
      if (conversation.state.cachedItems && (!this.getError(id) || conversation.state.hasProtectedItems)) continue;
      conversation.state.dispose();
      this.conversations.delete(id);
    }
  }
  private ready(conversationId: string, surfaceId: string, allowRelease = false) {
    this.assertActive();
    const conversation = this.conversations.get(conversationId);
    const slot = conversation?.slots.get(surfaceId);
    const window = conversation?.state.getWindow(surfaceId);
    if (!conversation || !slot || !window || slot.closed || slot.pending || conversation.recovery || (this.getError(conversationId) && !allowRelease)) {
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
  private assertActive() { if (this.disposed) throw new Error('Timeline session disposed'); }
  private notify() { if (!this.disposed) for (const listener of this.listeners) listener(); }
}
