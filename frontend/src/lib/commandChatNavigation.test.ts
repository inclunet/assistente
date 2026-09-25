import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { CHAT_NAVIGATION_COMMAND_IDS, CHAT_NAVIGATION_COMMAND_EVENT, captureChatNavigationTarget, registerChatNavigationSurface, requestChatNavigationCommand, getChatMessageNavigationInstanceId, type ChatNavigationContext, type ChatNavigationCommandID } from './commandChatNavigation';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useWorkspaceChatModalStore } from '../store/workspaceChatModalStore';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';

const cleanups: (() => void)[] = [];
let count = 0;
const id: ChatNavigationCommandID = 'chat.message.read.open';
function surface(options: { parent?: HTMLElement; context?: Partial<ChatNavigationContext>; instanceId?: string; commands?: readonly ChatNavigationCommandID[] } = {}) {
  const root = document.createElement('div'); root.className = 'message-node'; root.tabIndex = -1;
  (options.parent ?? document.body).append(root);
  const context: ChatNavigationContext = { pathname: '/chat', ownerId: 'owner', sessionId: 'session', workspaceId: 'ws', tabId: 'tab', conversationId: 'conv',
    messageId: `message-${++count}`, message: { content: 'original' }, ...options.context };
  const mutable = { context };
  const instanceId = options.instanceId ?? `node-${count}`;
  const listeners = new Set<() => void>();
  const open = vi.fn(() => true);
  const unregister = registerChatNavigationSurface({ root, instanceId, allowedCommands: options.commands ?? CHAT_NAVIGATION_COMMAND_IDS,
    readContext: () => mutable.context, isCurrent: () => root.isConnected,
    subscribe(changed) { listeners.add(changed); return () => { listeners.delete(changed); }; },
    canOpen: (_id, target) => !(target instanceof Element && target.matches('input,textarea')), open });
  cleanups.push(unregister, () => root.remove());
  return { root, mutable, instanceId, open, unregister, listeners, changed: () => listeners.forEach(changed => changed()) };
}
function modal(modalId: string) {
  const root = document.createElement('div'); root.className = 'modal-overlay'; root.dataset.modalId = modalId;
  document.body.append(root); registerOpenModal(modalId);
  const close = () => { unregisterOpenModal(modalId); root.remove(); };
  cleanups.push(close); return { root, close };
}
beforeEach(() => {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'owner', sessionId: 'session', role: 'admin' } });
  useWorkspaceStore.setState({ workspace: { id: 'ws', name: 'Workspace', activeTabId: 'tab', tabs: [{ id: 'tab', type: 'chat', conversationId: 'conv', title: 'Chat', position: 0 }] } });
  useWorkspaceChatModalStore.setState({ isOpen: false, boundTabId: null, boundConversationId: null });
});
afterEach(() => { cleanups.splice(0).reverse().forEach(cleanup => cleanup()); vi.restoreAllMocks(); });
const capture = (commandID: string = id, instanceId?: string) => captureChatNavigationTarget(() => '/chat', commandID, instanceId);
describe('commandChatNavigation', () => {
  it.each(CHAT_NAVIGATION_COMMAND_IDS)('%s executa uma vez sem retarget após foco mover', commandID => {
    const source = surface(); source.root.focus();
    const lease = capture(commandID)!;
    const picker = document.createElement('input'); document.body.append(picker); picker.focus();
    expect(lease.commandId).toBe(commandID); expect(lease.canOpen(commandID)).toBe(true);
    expect(lease.open(commandID)).toBe(true); expect(lease.open(commandID)).toBe(false);
    expect(source.open).toHaveBeenCalledExactlyOnceWith(commandID); expect(source.listeners.size).toBe(0);
    picker.remove();
  });
  it('mensagem automática é nó mais próximo, não pai', () => {
    const parent = surface(); const child = surface({ parent: parent.root }); child.root.focus();
    const lease = capture()!; expect(lease.instanceId).toBe(child.instanceId); lease.open(id);
    expect(parent.open).not.toHaveBeenCalled(); expect(child.open).toHaveBeenCalledTimes(1);
  });
  it('fora de mensagem não escolhe latest; expectedInstance permite ingresso próprio', () => {
    const source = surface(); expect(capture()).toBeUndefined();
    const lease = capture(id, source.instanceId)!; expect(lease.open(id)).toBe(true);
  });
  it('helper de menu exige ID exato e único sob superfície', () => {
    const first = surface(); const second = surface();
    expect(getChatMessageNavigationInstanceId(first.root, first.mutable.context.messageId!)).toBe(first.instanceId);
    expect(getChatMessageNavigationInstanceId(first.root, second.mutable.context.messageId!)).toBeUndefined();
    surface({ parent: first.root, context: { messageId: first.mutable.context.messageId } });
    expect(getChatMessageNavigationInstanceId(first.root, first.mutable.context.messageId!)).toBeUndefined();
  });
  it('duas surfaces foco input sem owner único recusam; expectedInstance é exato', () => {
    const first = surface(); surface();
    expect(capture('chat.focus.input')).toBeUndefined();
    const lease = capture('chat.focus.input', first.instanceId)!; expect(lease.open('chat.focus.input')).toBe(true);
    expect(capture(id, 'missing')).toBeUndefined();
  });
  it('instanceId duplicado não escolhe arbitrariamente', () => {
    surface({ instanceId: 'duplicate' }); surface({ instanceId: 'duplicate' });
    expect(capture(id, 'duplicate')).toBeUndefined();
  });
  it('não troca comando da lease', () => {
    const source = surface(); source.root.focus(); const lease = capture()!;
    expect(lease.open('chat.message.menu.open')).toBe(false); expect(source.open).not.toHaveBeenCalled(); lease.dispose();
  });
  it.each(['owner', 'session', 'workspace', 'tab', 'conversation'] as const)('ABA %s invalidado por stores', kind => {
    const source = surface(); source.root.focus(); const lease = capture()!;
    if (kind === 'owner' || kind === 'session') {
      const user = useAuthStore.getState().user!;
      useAuthStore.setState({ user: { ...user, [kind === 'owner' ? 'userId' : 'sessionId']: 'other' } }); useAuthStore.setState({ user });
    } else {
      const workspace = useWorkspaceStore.getState().workspace!;
      useWorkspaceStore.setState({ workspace: kind === 'conversation' ? { ...workspace, tabs: workspace.tabs.map(tab => ({ ...tab, conversationId: 'other' })) } : { ...workspace, [kind === 'tab' ? 'activeTabId' : 'id']: 'other' } });
      useWorkspaceStore.setState({ workspace });
    }
    expect(lease.open(id)).toBe(false); expect(source.listeners.size).toBe(0);
  });
  it.each(['message', 'messageId', 'pathname', 'chatSessionKey'] as const)('ABA %s do source é sticky', key => {
    const source = surface(); source.root.focus(); const lease = capture()!;
    const original = source.mutable.context;
    source.mutable.context = { ...original, [key]: key === 'message' ? { content: 'different' } : 'other' }; source.changed();
    source.mutable.context = original; source.changed();
    expect(lease.isCurrent()).toBe(false);
  });
  it('unregister limpa leases e callbacks de modo idempotente', () => {
    const source = surface(); source.root.focus(); const lease = capture()!;
    source.unregister(); source.unregister(); lease.dispose();
    expect(lease.isCurrent()).toBe(false); expect(source.listeners.size).toBe(0);
  });
  it.each(['hidden', 'inert', 'aria-hidden'])('bloqueia ancestor %s', attribute => {
    const source = surface(); source.root.setAttribute(attribute, attribute === 'aria-hidden' ? 'true' : '');
    expect(capture(id, source.instanceId)).toBeUndefined();
  });
  it('read virtual modal impede comandos de foco mesmo sem modalRegistry', () => {
    const source = surface(); source.root.setAttribute('aria-modal', 'true');
    expect(capture('chat.focus.input', source.instanceId)).toBeUndefined();
  });
  it('editor de mensagem bloqueia captura automática', () => {
    const source = surface(); const input = document.createElement('textarea'); source.root.append(input); input.focus();
    expect(capture()).toBeUndefined();
  });
  it('menu bloqueia automático mas ingresso explícito fixa nó original', () => {
    const source = surface(); const menu = document.createElement('div'); menu.setAttribute('role', 'menu');
    const input = document.createElement('input'); menu.append(input); source.root.append(menu); input.focus();
    expect(capture()).toBeUndefined(); const lease = capture(id, source.instanceId)!; expect(lease.open(id)).toBe(true);
  });
  it.each(['false', 'true', null])('combobox aria-expanded=%s: fechado permite foco, demais bloqueiam', expanded => {
    const source = surface();
    const input = document.createElement('textarea'); input.setAttribute('role', 'combobox');
    if (expanded !== null) input.setAttribute('aria-expanded', expanded);
    source.root.append(input); input.focus();
    // Focus commands are owned by the chat surface, not the message textarea.
    const off = registerChatNavigationSurface({ root: source.root, instanceId: 'focus-surface', allowedCommands: ['chat.focus.messages'],
      readContext: () => source.mutable.context, isCurrent: () => true, subscribe: () => () => {}, canOpen: () => true, open: () => true });
    source.unregister();
    try {
      const lease = capture('chat.focus.messages');
      expect(!!lease).toBe(expanded === 'false'); lease?.dispose();
    } finally { off(); }
  });
  it('chat modal em aba editor sem conversationId é permitido somente com binding próprio', () => {
    const workspace = useWorkspaceStore.getState().workspace!;
    useWorkspaceStore.setState({ workspace: { ...workspace, tabs: [{ id: 'tab', type: 'editor', title: 'Doc', position: 0 }] } });
    useWorkspaceChatModalStore.setState({ isOpen: true, boundTabId: 'tab', boundConversationId: 'conv' });
    const own = modal('own'); const source = surface({ parent: own.root, context: { modalId: 'own' } }); source.root.focus();
    const lease = capture()!; expect(lease.open(id)).toBe(true);
    useWorkspaceChatModalStore.setState({ boundConversationId: 'other' }); expect(capture()).toBeUndefined();
  });
  it('topmodal arbitrário não autoriza source e ABA modal invalida', () => {
    const source = surface(); source.root.focus(); const lease = capture()!;
    const top = modal('other'); expect(capture()).toBeUndefined(); top.close(); source.root.focus();
    expect(lease.open(id)).toBe(false);
  });
  it('IME/blur invalidam lease, documento sem foco não captura', () => {
    const source = surface(); source.root.focus(); const lease = capture()!;
    document.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true })); document.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true }));
    expect(lease.isCurrent()).toBe(false); const second = capture()!; window.dispatchEvent(new Event('blur')); expect(second.isCurrent()).toBe(false);
    vi.mocked(document.hasFocus).mockReturnValue(false); expect(capture()).toBeUndefined();
  });
  it('evento cancelável não tem efeito sem dispatcher', () => {
    const source = surface(); expect(requestChatNavigationCommand(id, source.instanceId)).toBe(false); expect(source.open).not.toHaveBeenCalled();
    const listener = (event: Event) => { expect((event as CustomEvent).detail).toEqual({ commandID: id, instanceId: source.instanceId }); event.preventDefault(); };
    window.addEventListener(CHAT_NAVIGATION_COMMAND_EVENT, listener);
    try { expect(requestChatNavigationCommand(id, source.instanceId)).toBe(true); }
    finally { window.removeEventListener(CHAT_NAVIGATION_COMMAND_EVENT, listener); }
  });
});
