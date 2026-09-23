import { useEffect, useLayoutEffect, useRef, useState, type RefObject } from 'react';
import { useChatStore, type Message } from '../../store/chatStore';
import { useAuthStore } from '../../store/authStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { getModalSnapshotGeneration } from '../../lib/modalRegistry';

interface DraftSnapshot {
  readonly editSession: object;
  readonly open: boolean;
  readonly revision: number;
  readonly content: string;
  readonly originalContent: string;
  readonly originalMessage: Message;
}
interface EditSource {
  readonly root: HTMLElement;
  readonly conversationId: string;
  readonly sessionKey: string;
  readonly messageId: string;
  read(): DraftSnapshot;
  pathname(): string | undefined;
  baseCurrent(): boolean;
  accept(revision: number, contextCurrent: () => boolean): boolean;
  rebaseConfirmed(snapshot: DraftSnapshot, contextCurrent: () => boolean): void;
  subscribe(changed: () => void): () => void;
}
const sources = new Set<EditSource>();

export function captureChatMessageEditDraft(root: HTMLElement, conversationId: string, sessionKey: string, messageId: string) {
  const candidates = [...sources].filter(source => source.root.isConnected && root.contains(source.root) &&
    source.conversationId === conversationId && source.sessionKey === sessionKey && source.messageId === messageId && source.read().open);
  if (candidates.length !== 1) return undefined;
  const source = candidates[0];
  const snapshot = source.read();
  if (!source.baseCurrent() || !snapshot.content.trim()) return undefined;
  const owner = useAuthStore.getState().user;
  const workspace = useWorkspaceStore.getState().workspace;
  const pathname = source.pathname();
  const modalGeneration = getModalSnapshotGeneration();
  let contextInvalid = false;
  let disposed = false;
  const contextCurrent = () => {
    const auth = useAuthStore.getState();
    const ws = useWorkspaceStore.getState().workspace;
    const valid = !contextInvalid && sources.has(source) && source.root.isConnected &&
      auth.isAuthenticated && auth.user?.userId === owner?.userId && auth.user?.sessionId === owner?.sessionId &&
      ws?.id === workspace?.id && ws?.activeTabId === workspace?.activeTabId &&
      ws?.tabs.find(tab => tab.id === ws.activeTabId)?.conversationId === conversationId &&
      source.pathname() === pathname && getModalSnapshotGeneration() === modalGeneration;
    if (!valid) contextInvalid = true;
    return valid;
  };
  const unsubscribe = source.subscribe(() => { contextCurrent(); });
  const draftCurrent = () => sources.has(source) && source.root.isConnected && source.read() === snapshot;
  return {
    content: snapshot.content, originalContent: snapshot.originalContent, originalMessage: snapshot.originalMessage,
    isCurrent: () => !disposed && draftCurrent() && contextCurrent() && source.baseCurrent(),
    contains: (target: Node) => source.root.contains(target),
    subscribe: source.subscribe,
    accept: (callerCurrent: () => boolean) => !disposed && draftCurrent() && contextCurrent() && callerCurrent() && source.accept(snapshot.revision, () => contextCurrent() && callerCurrent()),
    rebaseConfirmed: (callerCurrent: () => boolean) => { if (!disposed && contextCurrent() && callerCurrent()) source.rebaseConfirmed(snapshot, () => contextCurrent() && callerCurrent()); },
    dispose() { if (!disposed) { disposed = true; unsubscribe(); } },
  };
}

/** Local editor state; no optimistic timeline update, no persisted command data. */
export function useChatMessageEditDraft(rootRef: RefObject<HTMLDivElement | null>, message: Message, conversationId: string | null, sessionKey: string, pathname?: string) {
  const [draft, setDraft] = useState<DraftSnapshot>({ editSession: {}, open: false, revision: 0, content: message.content, originalContent: message.content, originalMessage: message });
  const snapshot = useRef(draft);
  const listeners = useRef(new Set<() => void>());
  const livePath = useRef(pathname); livePath.current = pathname;
  const base = useRef<{ invalid: boolean; owner: ReturnType<typeof useAuthStore.getState>['user'] } | undefined>(undefined);
  const update = (next: Omit<DraftSnapshot, 'revision'>) => {
    snapshot.current = { ...next, revision: snapshot.current.revision + 1 };
    setDraft(snapshot.current);
    listeners.current.forEach(changed => changed());
  };
  const baseCurrent = () => {
    const captured = base.current;
    if (!captured || captured.invalid || !snapshot.current.open || !conversationId) return false;
    const auth = useAuthStore.getState();
    const latest = useChatStore.getState().getConversationMessages(conversationId).find(item => item.id === message.id);
    const valid = auth.isAuthenticated && auth.user?.userId === captured.owner?.userId && auth.user?.sessionId === captured.owner?.sessionId &&
      latest === snapshot.current.originalMessage && latest?.content === snapshot.current.originalContent;
    if (!valid) captured.invalid = true;
    return valid;
  };
  useEffect(() => {
    const root = rootRef.current;
    if (!root || !conversationId || !sessionKey) return;
    const source: EditSource = {
      root, conversationId, sessionKey, messageId: message.id,
      read: () => snapshot.current, pathname: () => livePath.current, baseCurrent,
      subscribe(changed) { listeners.current.add(changed); return () => { listeners.current.delete(changed); }; },
      rebaseConfirmed(saved, callerCurrent) {
        const next = snapshot.current;
        if (!next.open || next.editSession !== saved.editSession || !callerCurrent()) return;
        const latest = useChatStore.getState().getConversationMessages(conversationId).find(item => item.id === message.id);
        if (!latest || latest.content !== saved.content || latest.isStreaming || latest.internal) return;
        // Only an authoritative success can advance the base. Never replace
        // the user's newer text or adopt a different editing session.
        if (base.current) base.current.invalid = false;
        update({ ...next, originalContent: latest.content, originalMessage: latest });
      },
      accept(revision, callerCurrent) {
        if (!snapshot.current.open || snapshot.current.revision !== revision || !callerCurrent()) return false;
        update({ ...snapshot.current, open: false });
        const closed = snapshot.current;
        const stopWatchingFocus = source.subscribe(() => { callerCurrent(); });
        requestAnimationFrame(() => {
          stopWatchingFocus();
          if (sources.has(source) && snapshot.current === closed && callerCurrent() && root.isConnected) root.focus();
        });
        return true;
      },
    };
    const changed = () => { baseCurrent(); listeners.current.forEach(notify => notify()); };
    const off = [useChatStore.subscribe(changed), useAuthStore.subscribe(changed), useWorkspaceStore.subscribe(changed)];
    sources.add(source);
    return () => { sources.delete(source); off.forEach(dispose => dispose()); listeners.current.forEach(notify => notify()); listeners.current.clear(); };
  }, [conversationId, sessionKey, message.id, rootRef]);
  useLayoutEffect(() => { baseCurrent(); listeners.current.forEach(notify => notify()); }, [pathname]);
  return {
    isEditing: draft.open, editContent: draft.content,
    open() {
      base.current = { invalid: false, owner: useAuthStore.getState().user };
      update({ editSession: {}, open: true, content: message.content, originalContent: message.content, originalMessage: message });
    },
    change(content: string) { update({ ...snapshot.current, content }); },
    cancel() { update({ ...snapshot.current, open: false, content: message.content }); },
  };
}
