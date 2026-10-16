import { ReadFocusContext } from './commandContextProviders';
import { getModalRegistrySnapshot } from './modalRegistry';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useWorkspaceChatModalStore } from '../store/workspaceChatModalStore';

export const CHAT_NAVIGATION_COMMAND_IDS = ['chat.focus.input', 'chat.focus.messages', 'chat.message.read.open', 'chat.message.menu.open', 'chat.message.reasoning.toggle', 'chat.message.thread.expand', 'chat.message.thread.collapse'] as const;
export type ChatNavigationCommandID = typeof CHAT_NAVIGATION_COMMAND_IDS[number];
export const CHAT_NAVIGATION_COMMAND_EVENT = 'commands:chat-navigation';
export interface ChatNavigationRequest { readonly commandID: ChatNavigationCommandID; readonly instanceId: string }
export function isChatNavigationCommand(id: string): id is ChatNavigationCommandID {
  return (CHAT_NAVIGATION_COMMAND_IDS as readonly string[]).includes(id);
}
export function requestChatNavigationCommand(commandID: ChatNavigationCommandID, instanceId: string): boolean {
  if (!isChatNavigationCommand(commandID) || !instanceId) return false;
  const event = new CustomEvent<ChatNavigationRequest>(CHAT_NAVIGATION_COMMAND_EVENT, { detail: { commandID, instanceId }, cancelable: true });
  window.dispatchEvent(event);
  return event.defaultPrevented;
}
export interface ChatNavigationContext {
  readonly pathname: string;
  readonly ownerId: string;
  readonly sessionId: string;
  readonly workspaceId: string;
  readonly tabId: string;
  readonly conversationId: string;
  readonly chatSessionKey?: string;
  readonly modalId?: string;
  readonly messageId?: string;
  readonly message?: object;
}
export interface ChatNavigationSurface {
  readonly root: HTMLElement;
  readonly instanceId: string;
  readonly allowedCommands: readonly ChatNavigationCommandID[];
  readContext(): ChatNavigationContext;
  isCurrent(): boolean;
  subscribe(changed: () => void): () => void;
  canOpen(commandID: ChatNavigationCommandID, target?: EventTarget | null): boolean;
  open(commandID: ChatNavigationCommandID): boolean;
}
export interface ChatNavigationTarget {
  readonly commandId: ChatNavigationCommandID;
  readonly instanceId: string;
  isCurrent(): boolean;
  canOpen(id: string, target?: EventTarget | null): boolean;
  open(id: string): boolean;
  dispose(): void;
}
const records = new Set<{ source: ChatNavigationSurface; leases: Set<() => void> }>();
export function registerChatNavigationSurface(source: ChatNavigationSurface): () => void {
  const record = { source, leases: new Set<() => void>() };
  records.add(record);
  return () => { records.delete(record); record.leases.forEach(dispose => dispose()); };
}
export function getChatMessageNavigationInstanceId(root: HTMLElement, messageId: string): string | undefined {
  const matches = [...records].filter(({ source }) => root.contains(source.root) && source.root.isConnected && source.readContext().messageId === messageId);
  return matches.length === 1 ? matches[0].source.instanceId : undefined;
}
function visible(root: HTMLElement): boolean {
  if (!root.isConnected || root.closest('[hidden],[inert],[aria-hidden="true"]')) return false;
  for (let element: HTMLElement | null = root; element; element = element.parentElement) {
    const style = getComputedStyle(element);
    if (style.display === 'none' || style.visibility === 'hidden' || style.visibility === 'collapse') return false;
  }
  return true;
}
function contextValid(source: ChatNavigationSurface, context: ChatNavigationContext): boolean {
  const auth = useAuthStore.getState();
  const ws = useWorkspaceStore.getState().workspace;
  const modal = useWorkspaceChatModalStore.getState();
  const stack = getModalRegistrySnapshot();
  const tab = ws?.tabs.find(item => item.id === context.tabId);
  return !document.querySelector('.message-node[aria-modal="true"]') &&
    !!context.ownerId && !!context.sessionId && !!context.conversationId && auth.isAuthenticated &&
    auth.user?.userId === context.ownerId && auth.user.sessionId === context.sessionId &&
    ws?.id === context.workspaceId && ws.activeTabId === context.tabId &&
    !!tab && (context.modalId ? true : tab.conversationId === context.conversationId) &&
    (context.modalId ? stack.topID === context.modalId && source.root.closest('[data-modal-id]')?.getAttribute('data-modal-id') === context.modalId &&
      !stack.dialogCommandScope && modal.isOpen && modal.boundTabId === context.tabId && modal.boundConversationId === context.conversationId : !stack.topID) &&
    visible(source.root) && source.isCurrent();
}
function sameContext(a: ChatNavigationContext, b: ChatNavigationContext): boolean {
  return a.pathname === b.pathname && a.ownerId === b.ownerId && a.sessionId === b.sessionId &&
    a.workspaceId === b.workspaceId && a.tabId === b.tabId && a.conversationId === b.conversationId &&
    a.chatSessionKey === b.chatSessionKey && a.modalId === b.modalId && a.messageId === b.messageId && a.message === b.message;
}
export function captureChatNavigationTarget(readPathname: () => string, commandID: string, expectedInstanceId?: string): ChatNavigationTarget | undefined {
  let release: (() => void) | undefined;
  try {
    if (!isChatNavigationCommand(commandID) || !document.hasFocus() || ReadFocusContext().composition === 'active') return;
    const path = readPathname();
    const messageCommand = commandID.startsWith('chat.message.');
    const origin = document.activeElement;
    if (expectedInstanceId === undefined && origin?.closest('[role="menu"],[role="listbox"],[role="combobox"]:not([aria-expanded="false"])')) return;
    const candidates = [...records].filter(({ source }) => {
      const context = source.readContext();
      if (!source.allowedCommands.includes(commandID) || context.pathname !== path || !contextValid(source, context)) return false;
      if (messageCommand && (!context.message || !context.messageId || !source.root.matches('.message-node'))) return false;
      if (expectedInstanceId !== undefined) return source.instanceId === expectedInstanceId;
      if (messageCommand) return origin?.closest('.message-node') === source.root;
      return true;
    });
    if (candidates.length !== 1) return;
    const record = candidates[0];
    const source = record.source;
    const context = Object.freeze({ ...source.readContext() });
    const generation = getModalRegistrySnapshot().generation;
    if (!source.canOpen(commandID, expectedInstanceId ? source.root : origin)) return;
    let disposed = false;
    const cleanups: (() => void)[] = [];
    const dispose = () => {
      if (disposed) return;
      disposed = true;
      record.leases.delete(dispose);
      cleanups.splice(0).forEach(cleanup => cleanup());
    };
    release = dispose;
    const isCurrent = () => {
      if (disposed) return false;
      try {
        const valid = records.has(record) && [...records].filter(other => other.source.instanceId === source.instanceId).length === 1 &&
          readPathname() === path && sameContext(context, source.readContext()) && contextValid(source, context) &&
          getModalRegistrySnapshot().generation === generation && document.hasFocus() && ReadFocusContext().composition !== 'active';
        if (!valid) dispose();
        return valid;
      } catch { dispose(); return false; }
    };
    record.leases.add(dispose);
    const watch = (subscribe: (changed: () => void) => () => void) => {
      const off = subscribe(() => { isCurrent(); });
      if (disposed) off(); else cleanups.push(off);
    };
    watch(source.subscribe.bind(source)); watch(useAuthStore.subscribe); watch(useWorkspaceStore.subscribe); watch(useWorkspaceChatModalStore.subscribe);
    const invalidate = () => dispose();
    window.addEventListener('blur', invalidate);
    document.addEventListener('compositionstart', invalidate, true);
    cleanups.push(() => window.removeEventListener('blur', invalidate), () => document.removeEventListener('compositionstart', invalidate, true));
    const canOpen = (id: string, target?: EventTarget | null) => {
      try { return id === commandID && isCurrent() && source.canOpen(commandID, target ?? source.root); }
      catch { dispose(); return false; }
    };
    if (!isCurrent()) { dispose(); return; }
    return { commandId: commandID, instanceId: source.instanceId, isCurrent, canOpen, dispose, open(id) {
      if (!canOpen(id)) return false;
      try { const result = source.open(commandID); dispose(); return result === true; }
      catch { dispose(); return false; }
    } };
  } catch { release?.(); return undefined; }
}
