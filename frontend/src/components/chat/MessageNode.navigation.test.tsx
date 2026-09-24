import { act, cleanup, createEvent, fireEvent, render, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { MessageNode, type MessageNodeProps } from './MessageNode';
import { ChatSessionProvider } from './ChatSessionContext';
import { WorkspacePanelProvider } from '../workspace/WorkspacePanelContext';
import { useAuthStore } from '../../store/authStore';
import { useChatStore } from '../../store/chatStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useWorkspaceChatModalStore } from '../../store/workspaceChatModalStore';
import { createChatSurfaceIdentity, createEmptyChatSession, createEmptyChatSurfaceSession, patchChatConversation } from '../../services/chatSessionRegistry';
import { CHAT_NAVIGATION_COMMAND_EVENT, captureChatNavigationTarget, requestChatNavigationCommand, type ChatNavigationRequest, type ChatNavigationCommandID } from '../../lib/commandChatNavigation';
import { attachChildrenToMessage } from '../../lib/chatMessageTree';
import { chat } from '../../../wailsjs/go/models';

vi.mock('../../services/audioFeedback', () => ({ playBumpSound: vi.fn(), playMessageSound: vi.fn() }));
vi.mock('../../services/messageAudio', () => ({ messageAudioService: { isCurrentlyPlaying: () => false, stopCurrentAudio: vi.fn() } }));
vi.mock('../../services/tts', () => ({ ttsService: { isSpeaking: () => false, stop: vi.fn() } }));
const cid = '01926b90-7a5a-7c4e-8d3f-000000000001';
const mid = '01926b90-7a5a-7c4e-8d3f-000000000002';
const childId = '01926b90-7a5a-7c4e-8d3f-000000000003';
const tab = { id: 'tab', type: 'chat' as const, conversationId: cid, title: 'Chat', position: 0 };
const surface = createChatSurfaceIdentity({ conversationId: cid, tabId: tab.id, surfaceType: 'page' });
const executed = vi.fn();
function dispatch(event: Event) {
  const { commandID, instanceId } = (event as CustomEvent<ChatNavigationRequest>).detail;
  const lease = captureChatNavigationTarget(() => '/chat', commandID, instanceId);
  try { if (lease?.canOpen(commandID)) { event.preventDefault(); executed(commandID, lease.open(commandID)); } }
  finally { lease?.dispose(); }
}
function node(id = mid, internal = false): chat.MessageNode {
  return new chat.MessageNode({ message: new chat.EnrichedMessage({ id, conversationId: cid, role: 'assistant', content: '**Mensagem** [link](https://example.org)', reasoning: 'Raciocínio', internal, isStreaming: false }), children: [], childCount: 0, level: internal ? 1 : 0 });
}
function seed(nodes: chat.MessageNode[]) {
  const conversation = { id: cid, title: 'Chat', threadedMessages: nodes };
  useChatStore.setState({ timelinesByConversationId: { [cid]: conversation }, sessionsByConversationId: { [cid]: { ...createEmptyChatSession(cid), conversation } }, surfaceSessionsByKey: {} });
}
function replaceTree(nodes: chat.MessageNode[]) {
  useChatStore.setState({ timelinesByConversationId: { [cid]: { id: cid, title: 'Chat', threadedMessages: nodes } } });
}
function attachChildrenToConversationInStore(messageId: string, children: chat.MessageNode[]) {
  useChatStore.setState(state => patchChatConversation(state, cid, conversation => ({
    ...conversation,
    threadedMessages: attachChildrenToMessage(conversation.threadedMessages, messageId, children),
  })));
}
function Tree(props: Omit<MessageNodeProps, 'node'>) {
  const nodes = useChatStore(state => state.surfaceSessionsByKey[surface.sessionKey]?.visibleThreadedMessages
    ?? state.timelinesByConversationId[cid].threadedMessages);
  return <>{nodes.map(item => <MessageNode key={item.message.id} node={item} commandPathname="/chat" {...props} />)}</>;
}
function mount(props: Omit<MessageNodeProps, 'node'> = {}) {
  return render(<MemoryRouter initialEntries={['/chat']}><WorkspacePanelProvider value={{ tab, isActive: true }}><ChatSessionProvider surface={surface}><Tree {...props} /></ChatSessionProvider></WorkspacePanelProvider></MemoryRouter>);
}
function root(id = mid) { return document.querySelector<HTMLElement>(`.message-node[data-message-id="${id}"]`)!; }
function request(id: ChatNavigationCommandID, messageId = mid) {
  return requestChatNavigationCommand(id, root(messageId).dataset.chatNavigationInstance!);
}
function deferred() { let resolve!: () => void; const promise = new Promise<void>(done => { resolve = done; }); return { promise, resolve }; }
beforeEach(() => {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'owner', sessionId: 'session', role: 'admin' } });
  useWorkspaceStore.setState({ workspace: { id: 'ws', name: 'Workspace', activeTabId: 'tab', tabs: [tab] } });
  useWorkspaceChatModalStore.setState({ isOpen: false, boundTabId: null, boundConversationId: null });
  seed([node()]); executed.mockClear();
  window.addEventListener(CHAT_NAVIGATION_COMMAND_EVENT, dispatch);
});
afterEach(() => { cleanup(); window.removeEventListener(CHAT_NAVIGATION_COMMAND_EVENT, dispatch); vi.restoreAllMocks(); });

describe('MessageNode navigation — registry, Provider, store e leitura reais', () => {
  it('paleta abre leitura exata e Escape retorna ao root original, não picker', () => {
    mount(); root().focus();
    const lease = captureChatNavigationTarget(() => '/chat', 'chat.message.read.open')!;
    const picker = document.createElement('input'); document.body.append(picker); picker.focus();
    act(() => { expect(lease.open('chat.message.read.open')).toBe(true); });
    expect(root()).toHaveAttribute('aria-modal', 'true');
    expect(document.activeElement).toHaveAttribute('role', 'document');
    fireEvent.keyDown(document.activeElement!, { key: 'Escape' });
    expect(root()).not.toHaveAttribute('aria-modal'); expect(document.activeElement).toBe(root());
    picker.remove();
  });
  it('Enter passa pelo evento e sem dispatcher não abre leitura', () => {
    mount(); root().focus(); window.removeEventListener(CHAT_NAVIGATION_COMMAND_EVENT, dispatch);
    fireEvent.keyDown(root(), { key: 'Enter' }); expect(root()).not.toHaveAttribute('aria-modal');
    window.addEventListener(CHAT_NAVIGATION_COMMAND_EVENT, dispatch);
    fireEvent.keyDown(root(), { key: 'Enter' }); expect(root()).toHaveAttribute('aria-modal', 'true');
    expect(executed).toHaveBeenCalledExactlyOnceWith('chat.message.read.open', true);
  });
  it('R e botão reasoning usam toggle registrado sem abrir leitura', () => {
    mount(); root().focus();
    fireEvent.keyDown(root(), { key: 'r' });
    expect(useChatStore.getState().isConversationReasoningExpanded(cid, mid, surface.sessionKey)).toBe(true);
    const button = root().querySelector<HTMLButtonElement>('.reasoning-section__header')!;
    fireEvent.keyDown(button, { key: 'Enter' });
    expect(useChatStore.getState().isConversationReasoningExpanded(cid, mid, surface.sessionKey)).toBe(false);
    expect(root()).not.toHaveAttribute('aria-modal');
    expect(executed).toHaveBeenCalledTimes(2);
  });
  it('Enter de botão thread e link não é gesto da mensagem', () => {
    const parent = node(); parent.children = [node(childId, true)]; parent.childCount = 1; seed([parent]); mount();
    const button = root().querySelector<HTMLButtonElement>('.thread-indicator')!;
    expect(fireEvent.keyDown(button, { key: 'Enter' })).toBe(true);
    expect(executed).not.toHaveBeenCalled();
    fireEvent.click(button); expect(root()).toHaveAttribute('aria-expanded', 'true');
    const link = root().querySelector('a')!; expect(fireEvent.keyDown(link, { key: 'Enter' })).toBe(true);
    expect(root()).not.toHaveAttribute('aria-modal');
  });
  it.each(['Enter', 'r'])('%s descarta repeat, IME, 229, modifiers e defaultPrevented', key => {
    mount(); root().focus();
    for (const flags of [{ repeat: true }, { isComposing: true }, { keyCode: 229 }, { ctrlKey: true }, { altKey: true }, { metaKey: true }]) fireEvent.keyDown(root(), { key, ...flags });
    const consumed = createEvent.keyDown(root(), { key }); consumed.preventDefault(); fireEvent(root(), consumed);
    expect(executed).not.toHaveBeenCalled(); expect(root()).not.toHaveAttribute('aria-modal');
  });
  it('menu por teclado e contextmenu compartilham callback registrado', () => {
    const menu = vi.fn(); mount({ onContextMenu: menu }); root().focus();
    fireEvent.keyUp(root(), { key: 'F10', shiftKey: true });
    expect(menu).toHaveBeenCalledTimes(1); expect(menu.mock.calls[0][0].currentTarget).toBe(root());
    fireEvent.contextMenu(root().querySelector('.chat-message')!);
    expect(menu).toHaveBeenCalledTimes(2);
    expect(executed.mock.calls.map(call => call[0])).toEqual(['chat.message.menu.open', 'chat.message.menu.open']);
  });
  it('setas expandem, focam filho e recolhem; repeat de navegação permanece', async () => {
    const parent = node(); parent.children = [node(childId, true)]; parent.childCount = 1; seed([parent]); mount(); root().focus();
    await act(async () => { fireEvent.keyDown(root(), { key: 'ArrowRight' }); });
    expect(root()).toHaveAttribute('aria-expanded', 'true'); expect(document.activeElement).toBe(root(childId));
    fireEvent.keyDown(root(childId), { key: 'ArrowLeft', repeat: true }); expect(document.activeElement).toBe(root());
    fireEvent.keyDown(root(), { key: 'ArrowLeft' }); expect(root()).toHaveAttribute('aria-expanded', 'false');
    expect(executed.mock.calls.map(call => call[0])).toEqual(['chat.message.thread.expand', 'chat.message.thread.collapse']);
  });
  it('carrega filhos uma vez e foca só após render real', async () => {
    const parent = node(); parent.childCount = 1; seed([parent]);
    const wait = deferred();
    const children = [node(childId, true)];
    const load = vi.fn(async () => { await wait.promise; attachChildrenToConversationInStore(mid, children); return children; });
    mount({ onLoadChildren: load }); root().focus();
    fireEvent.keyDown(root(), { key: 'ArrowRight' }); fireEvent.keyDown(root(), { key: 'ArrowRight', repeat: true });
    expect(load).toHaveBeenCalledExactlyOnceWith(mid); expect(root(childId)).toBeNull();
    await act(async () => { wait.resolve(); await wait.promise; });
    await waitFor(() => expect(document.activeElement).toBe(root(childId)));
    expect(root().querySelector('.thread-indicator')).not.toBeDisabled();
  });
  it.each(['workspace', 'message', 'unmount'] as const)('load tardio após %s não foca outro contexto', async kind => {
    const parent = node(); parent.childCount = 1; seed([parent]); const wait = deferred();
    const load = vi.fn(async () => { await wait.promise; return [node(childId, true)]; });
    const view = mount({ onLoadChildren: load }); root().focus(); fireEvent.keyDown(root(), { key: 'ArrowRight' });
    const external = document.createElement('button'); document.body.append(external);
    if (kind === 'workspace') {
      const workspace = useWorkspaceStore.getState().workspace!;
      act(() => { useWorkspaceStore.setState({ workspace: { ...workspace, id: 'other' } }); useWorkspaceStore.setState({ workspace }); });
    } else if (kind === 'message') { act(() => { replaceTree([node()]); replaceTree([parent]); }); }
    else view.unmount();
    external.focus(); await act(async () => { wait.resolve(); await wait.promise; });
    expect(document.activeElement).toBe(external); external.remove();
  });
  it('troca mensagem same-ID invalida lease antigo e registra lease novo para a referência renderizada', () => {
    const replacement = node();
    mount(); root().focus(); const oldLease = captureChatNavigationTarget(() => '/chat', 'chat.message.reasoning.toggle')!;
    act(() => replaceTree([replacement]));
    expect(oldLease.isCurrent()).toBe(false);
    expect(oldLease.open('chat.message.reasoning.toggle')).toBe(false);
    const refreshedLease = captureChatNavigationTarget(() => '/chat', 'chat.message.reasoning.toggle');
    expect(refreshedLease).toBeDefined();
    expect(refreshedLease?.isCurrent()).toBe(true);
    expect(oldLease.isCurrent()).toBe(false);
    refreshedLease?.dispose();
    expect(useChatStore.getState().isConversationReasoningExpanded(cid, mid, surface.sessionKey)).toBe(false);
  });
  it('revoga lease em troca canônica same-ID sem trocar projeção visível; referência visível nova recebe lease novo', () => {
    const original = node();
    const replacement = node();
    seed([original]);
    useChatStore.setState(state => ({
      surfaceSessionsByKey: {
        ...state.surfaceSessionsByKey,
        [surface.sessionKey]: {
          ...createEmptyChatSurfaceSession(cid, surface.sessionKey),
          visibleThreadedMessages: [original],
        },
      },
    }));
    mount(); root().focus();
    const oldLease = captureChatNavigationTarget(() => '/chat', 'chat.message.reasoning.toggle')!;
    expect(oldLease.isCurrent()).toBe(true);

    act(() => replaceTree([replacement]));
    expect(useChatStore.getState().surfaceSessionsByKey[surface.sessionKey].visibleThreadedMessages?.[0].message).toBe(original.message);
    expect(oldLease.isCurrent()).toBe(false);
    expect(oldLease.open('chat.message.reasoning.toggle')).toBe(false);

    act(() => useChatStore.setState(state => ({
      surfaceSessionsByKey: {
        ...state.surfaceSessionsByKey,
        [surface.sessionKey]: {
          ...state.surfaceSessionsByKey[surface.sessionKey],
          visibleThreadedMessages: [replacement],
        },
      },
    })));
    const freshLease = captureChatNavigationTarget(() => '/chat', 'chat.message.reasoning.toggle');
    expect(freshLease).toBeDefined();
    expect(freshLease?.isCurrent()).toBe(true);
    expect(oldLease.isCurrent()).toBe(false);
    freshLease?.dispose();
  });
  it('leitura virtual não permite comando de reasoning quebrar isolamento', () => {
    mount(); root().focus(); fireEvent.keyDown(root(), { key: 'Enter' });
    expect(request('chat.message.reasoning.toggle')).toBe(false);
    expect(root()).toHaveAttribute('aria-modal', 'true');
  });
  it.each(['chat.message.menu.open', 'chat.message.read.open', 'chat.message.reasoning.toggle'] as const)('streaming preserva ingresso fresco %s', commandID => {
    const message = node(); message.message.isStreaming = true; seed([message]);
    const menu = vi.fn(); mount({ onContextMenu: menu }); root().focus();
    act(() => { expect(request(commandID)).toBe(true); });
    expect(executed).toHaveBeenCalledExactlyOnceWith(commandID, true);
    if (commandID === 'chat.message.read.open') expect(root()).toHaveAttribute('aria-modal', 'true');
    if (commandID === 'chat.message.menu.open') expect(menu).toHaveBeenCalledTimes(1);
    if (commandID === 'chat.message.reasoning.toggle') expect(useChatStore.getState().isConversationReasoningExpanded(cid, mid, surface.sessionKey)).toBe(true);
  });
  it('remoção/recriação da sessão UI invalida lease mesmo com timeline intacta', () => {
    mount(); root().focus(); const lease = captureChatNavigationTarget(() => '/chat', 'chat.message.reasoning.toggle')!;
    const sessions = useChatStore.getState().surfaceSessionsByKey;
    act(() => { useChatStore.setState({ surfaceSessionsByKey: {} }); useChatStore.setState({ surfaceSessionsByKey: sessions }); });
    expect(lease.open('chat.message.reasoning.toggle')).toBe(false);
    expect(useChatStore.getState().isConversationReasoningExpanded(cid, mid, surface.sessionKey)).toBe(false);
  });
  it('botão reasoning em streaming usa conteúdo transitório da sessão antes de persistir', () => {
    const message = node(); message.message.reasoning = ''; message.message.isStreaming = true; seed([message]); mount();
    act(() => useChatStore.setState(state => ({ surfaceSessionsByKey: { ...state.surfaceSessionsByKey,
      [surface.sessionKey]: { ...state.surfaceSessionsByKey[surface.sessionKey], streamingMessageId: mid, streamingReasoning: 'Raciocínio em progresso' } } })));
    const button = root().querySelector<HTMLButtonElement>('.reasoning-section__header')!;
    expect(button).not.toBeNull(); fireEvent.click(button);
    expect(useChatStore.getState().isConversationReasoningExpanded(cid, mid, surface.sessionKey)).toBe(true);
    expect(executed).toHaveBeenCalledExactlyOnceWith('chat.message.reasoning.toggle', true);
  });
});
