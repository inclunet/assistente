import { recoveryItemIdentity, type RecoveryItemIdentity } from './timelineRecoveryItem';
import type { Message, MessageNode, TurnSegment } from '../lib/chatMessageTree';
import type { TimelineItem } from '../types/chatTimelineEvents';
import { ChatTimelineClient, type TimelinePosition } from './chatTimelineClient';

export interface ChatMessageSource {
  getMessage(id: string): Message | undefined;
  subscribe(changed: () => void): () => void;
  getCopyTarget?(id: string): { windowId: string; identity: RecoveryItemIdentity } | undefined;
}

const endPosition: TimelinePosition = { cursor: '', anchor: 'end', anchorItemId: '', direction: 'before', limit: 80 };

/** Uma superfície mantém somente sua navegação. Conteúdo e ordem vêm do cliente canônico. */
export class ChatTimelinePresentation implements ChatMessageSource {
  private readonly owners = new Map<string, string>();
  private readonly sources = new Map<string, string | null>();
  private readonly nodeWindows = new Map<string, MessageNode[]>();
  private readonly messages = new WeakMap<TimelineItem, Message>();
  private readonly listeners = new Set<() => void>();
  private unsubscribe: () => void = () => {};
  private pending: Promise<void> = Promise.resolve();
  private revision = 0;
  private busy = 0;
  private failure?: Error;
  private closing = false;

  constructor(readonly client: ChatTimelineClient, readonly conversationId: string, readonly surfaceId: string) {

  }

  subscribe = (changed: () => void): (() => void) => {
    this.listeners.add(changed);
    return () => { this.listeners.delete(changed); };
  };
  getRevision = () => this.revision;
  get loading() { return this.busy > 0; }
  get error() { return this.failure ?? this.client.getError(this.conversationId); }
  get window() { return this.client.getWindow(this.conversationId, this.surfaceId); }

  getCopyTarget(id: string): { windowId: string; identity: RecoveryItemIdentity } | undefined {
    if (this.closing) return;
    for (const surface of this.sources.keys()) {
      const window = this.client.getWindow(this.conversationId, surface);
      const item = this.client.getItem(this.conversationId, surface, id);
      if (!window || !item || item.representativeMessageId || !window.orderedItemIds.includes(id)) continue;
      const identity = recoveryItemIdentity(window.epoch, item);
      if (identity) return { windowId: window.windowId, identity };
    }
  }

  private notify() { this.revision++; this.listeners.forEach(changed => changed()); }
  private run(action: () => Promise<void>): Promise<void> {
    this.busy++; this.notify();
    const operation = this.pending.then(async () => {
      this.failure = undefined;
      try { if (this.closing) throw new Error('Timeline surface closed'); await action(); }
      catch (error) { this.failure = error instanceof Error ? error : new Error(String(error)); throw error; }
      finally { this.busy--; this.notify(); }
    });
    this.pending = operation.catch(() => undefined);
    return operation;
  }

  open(): Promise<void> {
    return this.run(async () => {
      this.unsubscribe = this.client.subscribe(() => this.notify());
      this.sources.set(this.surfaceId, null);
      await this.client.open(this.conversationId, this.surfaceId, null, endPosition);
      if (!this.closing) await this.client.navigate(this.conversationId, this.surfaceId, 'followTail', endPosition);
    });
  }

  navigate(direction: 'older' | 'newer' | 'start' | 'end', scope: string | null = null): Promise<void> {
    return this.run(async () => {
      const surface = [...this.sources].find(([, parent]) => parent === scope)?.[0];
      const window = surface && this.client.getWindow(this.conversationId, surface);
      if (!window || !surface || this.closing) throw new Error('Timeline surface unavailable');
      const position = direction === 'end' ? endPosition : direction === 'start'
        ? { ...endPosition, anchor: 'start', direction: 'after' }
        : { ...endPosition, anchor: '', cursor: direction === 'older' ? window.metadata.beforeCursor : window.metadata.afterCursor,
          direction: direction === 'older' ? 'before' : 'after' };
      await this.client.setRetention(this.conversationId, surface, []);
      await this.client.navigate(this.conversationId, surface, direction === 'end' ? 'followTail' : 'historical', position);
    });
  }

  refresh(): Promise<void> { return this.recover(false); }
  recoverAtEnd(): Promise<void> { return this.recover(true); }
  private recover(atEnd: boolean): Promise<void> {
    return this.run(async () => {
      await this.client.cleanupRecoveryItems(this.conversationId);
      if (!this.window) {
        if (this.client.hasSurface(this.conversationId, this.surfaceId)) await this.client.close(this.conversationId, this.surfaceId);
        this.sources.set(this.surfaceId, null);
        await this.client.open(this.conversationId, this.surfaceId, null, endPosition);
        if (!this.closing) await this.client.navigate(this.conversationId, this.surfaceId, 'followTail', endPosition);
        return;
      }
      if (atEnd && [...this.sources.keys()].some(surface => this.client.getWindow(this.conversationId, surface)?.metadata.retainedItemIds.length)) {
        throw new Error('Finish retained interactions before changing recovery position');
      }
      await this.client.refresh(this.conversationId, atEnd ? new Map([...this.sources.keys()].map(surface => [surface, endPosition])) : undefined);
      if (atEnd && !this.closing) await this.client.navigate(this.conversationId, this.surfaceId, 'followTail', endPosition);
    });
  }
  get recoveredItems() {
    return this.client.getRecoveredItems(this.conversationId).filter(item => this.sources.has(item.surfaceId));
  }
  threadWindow(parent: string) {
    const surface = [...this.sources].find(([, scope]) => scope === parent)?.[0];
    return surface ? this.client.getWindow(this.conversationId, surface) : undefined;
  }

  openThread(parentId: string): Promise<void> {
    return this.run(async () => {
      const surface = `${this.surfaceId}:thread:${parentId}`;
      if (this.sources.has(surface)) return;
      const owner = [...this.sources.keys()].find(key => this.client.getWindow(this.conversationId, key)?.orderedItemIds.some(id =>
        this.client.getItem(this.conversationId, key, id)?.representativeMessageId === parentId));
      if (!owner) throw new Error('Thread parent unavailable');
      this.sources.set(surface, parentId);
      this.owners.set(surface, owner);
      try { await this.client.open(this.conversationId, surface, parentId, endPosition); }
      catch (error) {
        if (!this.client.hasSurface(this.conversationId, surface)) { this.sources.delete(surface); this.owners.delete(surface); }
        throw error;
      }
    });
  }

  closeThread(parentId: string): Promise<void> {
    return this.run(async () => {
      const closeSurface = async (surface: string): Promise<void> => {
        for (const [child, owner] of this.owners) if (owner === surface) await closeSurface(child);
        await this.client.close(this.conversationId, surface);
        this.sources.delete(surface);
        this.owners.delete(surface);
        this.nodeWindows.delete(surface);
      };
      const surface = [...this.sources].find(([, scope]) => scope === parentId)?.[0];
      if (surface) await closeSurface(surface);
    });
  }

  retain(itemIds: string[]): Promise<void> {
    return this.run(async () => {
      const retained = new Set(itemIds);
      const ancestors = (nodes: MessageNode[], parents: string[]) => {
        for (const node of nodes) {
          const itemId = node.message.timelineItem!.itemId;
          const path = [...parents, itemId];
          if (retained.has(itemId)) for (const id of parents) retained.add(id);
          ancestors(node.children ?? [], path);
        }
      };
      ancestors(this.nodes(), []);
      for (const surface of this.sources.keys()) {
        const window = this.client.getWindow(this.conversationId, surface);
        if (window) await this.client.setRetention(this.conversationId, surface, [...retained].filter(id => window.orderedItemIds.includes(id)));
      }
    });
  }

  /** Uma recusa conserva a inscrição e o conteúdo recuperável para decisão explícita. */
  async close(): Promise<void> {
    this.closing = true;
    try {
      for (const surface of [...this.sources.keys()].reverse()) {
        await this.client.close(this.conversationId, surface);
        this.sources.delete(surface);
        this.owners.delete(surface);
        this.nodeWindows.delete(surface);
      }
      this.unsubscribe();
      this.listeners.clear();
    } catch (error) {
      this.failure = error instanceof Error ? error : new Error(String(error));
      this.notify();
      throw error;
    }
  }

  getMessageByRepresentative(id: string): Message | undefined {
    if (this.error) return undefined;
    for (const surface of this.sources.keys()) {
      for (const itemId of this.client.getWindow(this.conversationId, surface)?.orderedItemIds ?? []) {
        const item = this.client.getItem(this.conversationId, surface, itemId);
        if (item?.representativeMessageId === id) return this.message(item);
      }
    }
    return undefined;
  }

  getMessage = (id: string): Message | undefined => {
    if (this.error) return undefined;
    for (const surface of this.sources.keys()) {
      const item = this.client.getItem(this.conversationId, surface, id);
      if (item) return this.message(item);
    }
    return undefined;
  };

  nodes(scope: string | null = null): MessageNode[] {
    const surface = [...this.sources].find(([, parent]) => parent === scope)?.[0];
    const window = surface && this.client.getWindow(this.conversationId, surface);
    if (!surface || !window) return emptyNodes;
    const previous = this.nodeWindows.get(surface);
    const byId = new Map(previous?.map(node => [node.itemId, node]));
    const nodes = window.orderedItemIds.map((id) => {
      const item = this.client.getItem(this.conversationId, surface, id);
      if (!item) throw new Error('Canonical window item unavailable');
      const parent = item.representativeMessageId;
      const message = this.message(item);
      const children = parent ? this.nodes(parent) : emptyNodes;
      const existing = byId.get(id);
      if (existing?.message === message && existing.children === children) return existing;
      return { itemId: id, message, children,
        childCount: item.metadata.childCount } satisfies MessageNode;
    });
    if (previous?.length === nodes.length && nodes.every((node, index) => node === previous[index])) return previous;
    this.nodeWindows.set(surface, nodes);
    return nodes;
  }

  private message(item: TimelineItem): Message {
    const existing = this.messages.get(item);
    if (existing) return existing;
    const segments: TurnSegment[] = item.segments.filter(segment => segment.kind !== 'reasoning').map(segment => ({
      type: segment.kind === 'tool_invocations' ? 'tool_calls' : 'text', content: segment.text,
      toolInvocations: segment.invocations,
    }));
    const message = {
      ...item.metadata, timestamp: Date.parse(item.metadata.createdAt), id: item.itemId, conversationId: this.conversationId, parentId: item.scopeParentId ?? undefined,
      turnId: item.turnId ?? undefined, content: item.segments.filter(segment => segment.conclusion).map(segment => segment.text ?? '').join(''),
      reasoning: item.segments.filter(segment => segment.kind === 'reasoning').map(segment => segment.text ?? '').join('\n'),
      isStreaming: item.state === 'preparing' || item.state === 'streaming', internal: false,
      turnSegments: segments,
    timelineItem: item } satisfies Message;
    this.messages.set(item, message);
    return message;
  }
}

const emptyNodes: MessageNode[] = [];
