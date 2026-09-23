import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ChatSessionView } from './ChatSessionView';
import { sendChatSurfaceMessage } from './ChatSurfaceController';
import { useChatStore } from '../../store/chatStore';
import { useAuthStore } from '../../store/authStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { createChatSurfaceIdentity, createEmptyChatSession } from '../../services/chatSessionRegistry';
import { CHAT_MESSAGING_EVENT, captureChatMessagingTarget, executeChatMessaging, type ChatMessagingRequest } from '../../lib/commandChatMessaging';
import { chat } from '../../../wailsjs/go/models';
import { ttsService } from '../../services/tts';
import { messageAudioService } from '../../services/messageAudio';
import { registerEditorTransferSurface } from '../../lib/commandEditorTransfer';
import { useEditorStore } from '../../store/editorStore';
import { Modal } from '../ui/Modal';
import { useWorkspaceChatModalStore } from '../../store/workspaceChatModalStore';
import { CHAT_NAVIGATION_COMMAND_EVENT, captureChatNavigationTarget, type ChatNavigationRequest } from '../../lib/commandChatNavigation';
function navigationListener(event: Event) {
  const { commandID, instanceId } = (event as CustomEvent<ChatNavigationRequest>).detail;
  const target = captureChatNavigationTarget(() => '/', commandID, instanceId);
  if (!target) return;
  try { if (target.open(commandID)) event.preventDefault(); } finally { target.dispose(); }
}

const id = '01926b90-7a5a-7c4e-8d3f-000000000001';
const send = vi.hoisted(() => vi.fn());
const prepareMessage = vi.hoisted(() => vi.fn(async () => {}));
const commitMessage = vi.hoisted(() => vi.fn(async () => {}));
const editorTransport = vi.hoisted(() => ({ prepare: vi.fn(), open: vi.fn(), validate: vi.fn() }));
vi.mock('../../lib/commandChatEditorWails', () => ({ prepareChatEditorCommand: editorTransport.prepare, openChatEditorCommand: editorTransport.open, validateChatEditorCommand: editorTransport.validate }));
vi.mock('../../lib/commandChatMessageWails', () => ({ prepareChatMessageCommand: prepareMessage, commitChatMessageCommand: commitMessage }));
const retry = vi.hoisted(() => vi.fn());
const t = vi.hoisted(() => (key: string) => key);
const announceMessage = vi.hoisted(() => vi.fn());
const configure = vi.hoisted(() => vi.fn(async () => false));
const deepLink = vi.hoisted(() => vi.fn(async () => {}));
vi.mock('../../store/confirmStore', () => ({ requestConfirm: configure }));
vi.mock('../../lib/deepLinks', () => ({ executeDeepLink: deepLink }));
vi.mock('react-i18next', () => ({ initReactI18next: { type: '3rdParty', init() {} }, useTranslation: () => ({ t, i18n: { language: 'pt-BR' } }) }));
vi.mock('@wailsjs/go/wailsapi/Chat', () => ({ SendMessage: send, RetryMessage: retry }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: () => () => {} }));
vi.mock('@wailsjs/go/wailsapi/Profiles', () => ({ GetActiveProfile: vi.fn(async () => ({})), GetActiveProfileSlug: vi.fn(async () => 'default') }));
vi.mock('@wailsjs/go/wailsapi/Skills', () => ({ GetUserInvocableSkillsForProfile: vi.fn(async () => []) }));
vi.mock('../../services/audioFeedback', () => ({ playSendSound: vi.fn(), playReceiveSound: vi.fn(), playChatErrorSound: vi.fn() }));
vi.mock('../../hooks/useAnnouncer', () => ({ announce: announceMessage, useAnnouncer: () => ({ announce: announceMessage, announceRequest: vi.fn() }) }));
vi.mock('../../services/tts', () => ({ ttsService: { isEnabled: () => false, hasVoiceConfig: vi.fn(() => true), getVolume: () => 1, isSpeaking: () => false, isEnabledForUser: () => false, shouldUseAriaLiveForUser: () => false, isAutoReadEnabled: () => false, stop: vi.fn(), on: vi.fn(), off: vi.fn(), getVoiceContext: vi.fn(() => null), shouldUseAriaLiveForAgent: () => false } }));
vi.mock('./ChatToolbar', () => ({ ChatToolbar: () => null }));
vi.mock('./useAgentSessionCommands', () => ({ useAgentSessionCommands: () => [] }));
vi.mock('./VoiceButton', () => ({ VoiceButton: () => null }));
vi.mock('../workspace/WorkspacePanelContext', () => ({
  useWorkspacePanel: () => ({ tab: { id: 'tab', type: 'chat' }, isActive: true }),
  useOptionalWorkspacePanel: () => ({ tab: { id: 'tab', type: 'chat' }, isActive: true }),
}));
const surface = createChatSurfaceIdentity({ conversationId: id, tabId: 'tab', surfaceType: 'page' });
const runs: Promise<unknown>[] = [];
let listener: (event: Event) => void;
let status: 'succeeded' | 'denied' | 'failed' = 'succeeded';
const begin = vi.fn();
const take = vi.fn();
const commit = vi.fn();
const complete = vi.fn();
const getResult = vi.fn();
function mount() {
  return render(<MemoryRouter><ChatSessionView surface={surface} onSend={(content, media, origin, command) => sendChatSurfaceMessage(id, content, media, undefined, origin, command)} /></MemoryRouter>);
}
beforeEach(() => {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  send.mockReset().mockResolvedValue(id); retry.mockReset().mockResolvedValue(id);
  status = 'succeeded';
  vi.mocked(ttsService.hasVoiceConfig).mockReturnValue(true);
  vi.mocked(ttsService.getVoiceContext).mockReturnValue(undefined);
  configure.mockReset().mockResolvedValue(false); deepLink.mockClear();
  begin.mockReset().mockImplementation(async (commandId: string) => ({ ticket: 'ticket', invocationId: 'invocation', commandId }));
  take.mockReset().mockResolvedValue({ ticket: 'ticket', invocationId: 'invocation', commandId: 'chat.message.send', handoffId: 'handoff' });
  commit.mockReset().mockResolvedValue(undefined); complete.mockReset().mockImplementation(async (_ticket, _handoff, completed) => { status = completed; });
  getResult.mockReset().mockImplementation(async () => ({ invocationId: 'invocation', status }));
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'owner', sessionId: 'session', role: 'admin' } });
  useWorkspaceChatModalStore.setState({ isOpen: false, boundConversationId: null, boundTabId: null });
  useWorkspaceStore.setState({ workspace: { id: 'workspace', activeTabId: 'tab', tabs: [{ id: 'tab', type: 'chat', conversationId: id }] } as NonNullable<ReturnType<typeof useWorkspaceStore.getState>['workspace']> });
  prepareMessage.mockClear(); commitMessage.mockClear();
  vi.spyOn(HTMLElement.prototype, 'scrollIntoView').mockImplementation(() => {});
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn(async () => {}) } });
  const conversation = { id, title: 'chat', threadedMessages: [firstID, secondID].map((messageID, index) => new chat.MessageNode({
    message: new chat.EnrichedMessage({ id: messageID, conversationId: id, role: 'user', content: index ? 'segunda' : '**primeira**' }), children: [], hasChildren: false, childCount: 0, level: 0,
  })) };
  useChatStore.setState({ timelinesByConversationId: { [id]: conversation }, sessionsByConversationId: { [id]: { ...createEmptyChatSession(id), conversation } }, surfaceSessionsByKey: {}, loadingConversationIds: new Set() });
  listener = event => {
    const request = (event as CustomEvent<ChatMessagingRequest>).detail;
    const target = request.target ?? captureChatMessagingTarget(() => '/', request.commandId, request.instanceId);
    if (!target) return;
    event.preventDefault();
    runs.push(executeChatMessaging({ beginUICommand: begin, takeUICommand: take, commitBackendCommand: commit, completeUICommand: complete,
      cancelUICommand: vi.fn(async () => {}), getUICommandResult: getResult,
    }, target));
  };
  window.addEventListener(CHAT_MESSAGING_EVENT, listener);
  window.addEventListener(CHAT_NAVIGATION_COMMAND_EVENT, navigationListener);
});
afterEach(async () => {
  cleanup();
  useChatStore.getState().cancelConversationTurn(id);
  await Promise.allSettled(runs.splice(0));
  window.removeEventListener(CHAT_MESSAGING_EVENT, listener);
  window.removeEventListener(CHAT_NAVIGATION_COMMAND_EVENT, navigationListener);
  vi.restoreAllMocks();
});

const firstID = '01926b90-7a5a-7c4e-8d3f-000000000011';
const secondID = '01926b90-7a5a-7c4e-8d3f-000000000012';
function node(messageID = firstID) { return document.querySelector<HTMLElement>('[data-message-id="' + messageID + '"]')!; }
function port() { return { beginUICommand: begin, takeUICommand: take, completeUICommand: complete, commitBackendCommand: commit, cancelUICommand: vi.fn(async () => {}), getUICommandResult: getResult }; }
function capture(commandID: string) {
  act(() => node().focus());
  take.mockResolvedValue({ ticket: 'ticket', invocationId: 'invocation', commandId: commandID, handoffId: 'handoff' });
  return captureChatMessagingTarget(() => '/', commandID);
}
describe('ChatSessionView message actions — superfície/lista/registry reais', () => {
  it('captura foco messages a partir do ChatInput fechado, mas não atravessa autocomplete aberto', () => {
    mount(); const input = screen.getByRole('combobox'); act(() => input.focus());
    expect(input).toHaveAttribute('aria-expanded', 'false');
    const target = captureChatNavigationTarget(() => '/', 'chat.focus.messages');
    expect(target).toBeDefined();
    act(() => { expect(target!.open('chat.focus.messages')).toBe(true); });
    expect(screen.getByRole('list')).toHaveFocus();
    act(() => input.focus()); input.setAttribute('aria-expanded', 'true');
    expect(captureChatNavigationTarget(() => '/', 'chat.focus.messages')).toBeUndefined();
    expect(input).toHaveFocus(); expect(begin).not.toHaveBeenCalled();
  });
  it('foco messages alcança lista e não última mensagem; input reutiliza campo existente', () => {
    mount();
    const messages = captureChatNavigationTarget(() => '/', 'chat.focus.messages');
    expect(messages).toBeDefined();
    act(() => { expect(messages!.open('chat.focus.messages')).toBe(true); });
    expect(screen.getByRole('list')).toHaveFocus(); expect(node(secondID)).not.toHaveFocus();
    const input = captureChatNavigationTarget(() => '/', 'chat.focus.input');
    act(() => { expect(input!.open('chat.focus.input')).toBe(true); });
    expect(screen.getByRole('combobox')).toHaveFocus(); expect(begin).not.toHaveBeenCalled();
  });
  it('foco messages funciona com região vazia', () => {
    useChatStore.setState(state => ({ timelinesByConversationId: { [id]: { ...state.timelinesByConversationId[id], threadedMessages: [] } } }));
    mount();
    const target = captureChatNavigationTarget(() => '/', 'chat.focus.messages');
    act(() => { expect(target!.open('chat.focus.messages')).toBe(true); });
    expect(screen.getByRole('region', { name: 'chat.messageListLabel' })).toHaveFocus();
    expect(begin).not.toHaveBeenCalled();
  });
  it('input desabilitado não admite comando de foco', () => {
    mount();
    (screen.getByRole('combobox') as HTMLTextAreaElement).disabled = true;
    expect(screen.getByRole('combobox')).toBeDisabled();
    expect(captureChatNavigationTarget(() => '/', 'chat.focus.input')).toBeUndefined();
    expect(begin).not.toHaveBeenCalled();
  });
  it('menu real abre leitura exata, mantém documento após timers e Escape retorna ao nó original', async () => {
    mount(); act(() => node().focus()); fireEvent.keyUp(node(), { key: 'ContextMenu' });
    await userEvent.click(await screen.findByRole('menuitem', { name: /Ativar modo de leitura/ }));
    await waitFor(() => expect(node()).toHaveAttribute('aria-modal', 'true'));
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 80)); });
    expect(document.activeElement?.getAttribute('role')).toBe('document');
    expect(node().contains(document.activeElement)).toBe(true); expect(node(secondID)).not.toHaveAttribute('aria-modal');
    fireEvent.keyDown(document.activeElement!, { key: 'Escape' });
    await waitFor(() => expect(node()).toHaveFocus());
    expect(node()).not.toHaveAttribute('aria-modal'); expect(begin).not.toHaveBeenCalled();
  });
  it('menu com mensagem substituída não abre leitura da nova versão', async () => {
    mount(); act(() => node().focus()); fireEvent.keyUp(node(), { key: 'ContextMenu' });
    await screen.findByRole('menuitem', { name: /Ativar modo de leitura/ });
    act(() => useChatStore.getState().updateConversationMessage(id, firstID, 'alterada'));
    await userEvent.click(screen.getByRole('menuitem', { name: /Ativar modo de leitura/ }));
    expect(node()).not.toHaveAttribute('aria-modal'); expect(begin).not.toHaveBeenCalled();
  });
  it('menu real alterna raciocínio somente do nó capturado', async () => {
    useChatStore.setState(state => ({ timelinesByConversationId: { [id]: { ...state.timelinesByConversationId[id],
      threadedMessages: state.timelinesByConversationId[id].threadedMessages.map(item => new chat.MessageNode({ ...item,
        message: new chat.EnrichedMessage({ ...item.message, role: 'assistant', reasoning: 'raciocínio ' + item.message.id }) })) } } }));
    mount(); act(() => node().focus()); fireEvent.keyUp(node(), { key: 'ContextMenu' });
    await userEvent.click(await screen.findByRole('menuitem', { name: /Ver raciocínio do modelo/ }));
    expect(useChatStore.getState().isConversationReasoningExpanded(id, firstID, surface.sessionKey)).toBe(true);
    expect(useChatStore.getState().isConversationReasoningExpanded(id, secondID, surface.sessionKey)).toBe(false);
    expect(begin).not.toHaveBeenCalled();
  });
  it('comandos expand/collapse aplicam o estado à thread real sem escolher outro nó', () => {
    const childID = '01926b90-7a5a-7c4e-8d3f-000000000013';
    useChatStore.setState(state => ({ timelinesByConversationId: { [id]: { ...state.timelinesByConversationId[id],
      threadedMessages: state.timelinesByConversationId[id].threadedMessages.map((item, index) => index ? item : new chat.MessageNode({ ...item,
        hasChildren: true, childCount: 1, children: [new chat.MessageNode({
          message: new chat.EnrichedMessage({ id: childID, conversationId: id, parentId: firstID, role: 'user', content: 'filha' }),
          children: [], childCount: 0, hasChildren: false, level: 1,
        })] })) } } }));
    mount(); act(() => node().focus());
    const expand = captureChatNavigationTarget(() => '/', 'chat.message.thread.expand');
    act(() => { expect(expand!.open('chat.message.thread.expand')).toBe(true); });
    expect(node()).toHaveAttribute('aria-expanded', 'true'); expect(node(childID)).toBeInTheDocument();
    act(() => node().focus());
    const collapse = captureChatNavigationTarget(() => '/', 'chat.message.thread.collapse');
    act(() => { expect(collapse!.open('chat.message.thread.collapse')).toBe(true); });
    expect(node()).toHaveAttribute('aria-expanded', 'false'); expect(node(childID)).not.toBeInTheDocument();
    expect(useChatStore.getState().isConversationThreadExpanded(id, secondID, surface.sessionKey)).toBe(false);
    expect(begin).not.toHaveBeenCalled();
  });
  it('chat em modal sobre aba editor admite focos e leitura sem exigir conversationId na aba', async () => {
    const modalSurface = createChatSurfaceIdentity({ conversationId: id, tabId: 'tab', surfaceType: 'modal' });
    useWorkspaceStore.setState({ workspace: { id: 'workspace', name: 'workspace', activeTabId: 'tab', tabs: [{ id: 'tab', type: 'editor', title: 'Editor', position: 0 }] } });
    useWorkspaceChatModalStore.setState({ isOpen: true, boundTabId: 'tab', boundConversationId: id, boundSurface: modalSurface });
    const view = render(<MemoryRouter><Modal isOpen title="Chat modal" onClose={() => {}}>
      <ChatSessionView variant="embedded" surface={modalSurface} onSend={vi.fn()} />
    </Modal></MemoryRouter>);
    const messages = captureChatNavigationTarget(() => '/', 'chat.focus.messages');
    expect(messages).toBeDefined(); act(() => { expect(messages!.open('chat.focus.messages')).toBe(true); });
    expect(screen.getByRole('list')).toHaveFocus();
    const input = captureChatNavigationTarget(() => '/', 'chat.focus.input');
    act(() => { expect(input!.open('chat.focus.input')).toBe(true); });
    expect(screen.getByRole('combobox')).toHaveFocus();
    act(() => node().focus()); fireEvent.keyUp(node(), { key: 'ContextMenu' });
    await userEvent.click(await screen.findByRole('menuitem', { name: /Ativar modo de leitura/ }));
    await waitFor(() => expect(node()).toHaveAttribute('aria-modal', 'true'));
    fireEvent.keyDown(document.activeElement!, { key: 'Escape' });
    await waitFor(() => expect(node()).toHaveFocus());
    view.rerender(<MemoryRouter><Modal isOpen title="Chat modal" onClose={() => {}}>
      <ChatSessionView variant="embedded" surface={modalSurface} onSend={vi.fn()} />
    </Modal><Modal isOpen title="Decision" onClose={() => {}}><button>Decidir</button></Modal></MemoryRouter>);
    expect(captureChatNavigationTarget(() => '/', 'chat.focus.input')).toBeUndefined();
    expect(begin).not.toHaveBeenCalled();
  });
  it('envio ao editor usa mensagem focada, transição autorizada e ACK real', async () => {
    useEditorStore.setState({ ownerUserId: 'owner', documents: {} });
    editorTransport.prepare.mockResolvedValue({ tabId: 'editor' });
    editorTransport.open.mockImplementation(async () => {
      const workspace = useWorkspaceStore.getState().workspace!;
      useWorkspaceStore.setState({ workspace: { ...workspace, activeTabId: 'editor', tabs: [...workspace.tabs, { id: 'editor', title: 'Editor', position: 1, type: 'editor' }] } });
      useEditorStore.getState().createDocument({ id: 'editor', markdown: '' });
      return { tabId: 'editor' };
    });
    editorTransport.validate.mockResolvedValue(undefined);
    const applied = vi.fn();
    const off = registerEditorTransferSurface({ documentId: 'editor', capture: () => ({ documentId: 'editor', isCurrent: () => true, apply: applied, dispose() {} }) });
    try {
      mount(); const target = capture('chat.message.send_to_editor'); expect(target).toBeDefined();
      await act(async () => { expect(await executeChatMessaging(port(), target!)).toBe('succeeded'); });
      expect(editorTransport.prepare).toHaveBeenCalledExactlyOnceWith('ticket', firstID, '**primeira**', '');
      expect(applied).toHaveBeenCalledWith(expect.objectContaining({ content: '**primeira**', format: 'markdown' }));
      expect(editorTransport.validate).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff');
      expect(complete).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff', 'succeeded');
      expect(prepareMessage).not.toHaveBeenCalled(); expect(commitMessage).not.toHaveBeenCalled();
    } finally { off(); }
  });
  it('envio ao editor não escolhe última mensagem nem executa no campo de texto', () => {
    mount();
    expect(captureChatMessagingTarget(() => '/', 'chat.message.send_to_editor')).toBeUndefined();
    act(() => node().focus());
    expect(captureChatMessagingTarget(() => '/', 'chat.message.send_to_editor', undefined, screen.getByRole('combobox'))).toBeUndefined();
  });
  it.each(['chat.message.copy', 'chat.message.copy_markdown'])('%s prepara ID focado e copia uma vez', async commandID => {
    mount(); const target = capture(commandID); expect(target).toBeDefined();
    await act(async () => { await executeChatMessaging(port(), target!); });
    expect(prepareMessage).toHaveBeenCalledExactlyOnceWith('ticket', firstID);
    expect(navigator.clipboard.writeText).toHaveBeenCalledExactlyOnceWith(commandID.endsWith('markdown') ? '**primeira**' : 'primeira');
    expect(complete).toHaveBeenCalledWith('ticket', 'handoff', 'succeeded');
  });
  it('copy rejeitado falha sem declarar sucesso', async () => {
    mount(); const target = capture('chat.message.copy');
    vi.mocked(navigator.clipboard.writeText).mockRejectedValue(new Error('clipboard denied'));
    await act(async () => { expect(await executeChatMessaging(port(), target!)).toBe('failed'); });
    expect(complete).not.toHaveBeenCalledWith('ticket', 'handoff', 'succeeded');
  });
  it('sem seleção não escolhe latest; input explícito não captura', () => {
    mount();
    expect(captureChatMessagingTarget(() => '/', 'chat.message.copy')).toBeUndefined();
    act(() => node().focus());
    expect(captureChatMessagingTarget(() => '/', 'chat.message.copy', undefined, screen.getByRole('combobox'))).toBeUndefined();
  });
  it('seleção A-B-A durante Prepare invalida sem efeito', async () => {
    mount(); const target = capture('chat.message.copy');
    prepareMessage.mockImplementationOnce(async () => { act(() => { node(secondID).focus(); node().focus(); }); });
    await act(async () => { await executeChatMessaging(port(), target!); });
    expect(navigator.clipboard.writeText).not.toHaveBeenCalled(); expect(take).not.toHaveBeenCalled();
  });
  it('troca conteúdo durante Take recusa mesmo ID', async () => {
    mount(); const target = capture('chat.message.copy');
    take.mockImplementationOnce(async () => {
      useChatStore.setState(state => ({ timelinesByConversationId: { ...state.timelinesByConversationId, [id]: { ...state.timelinesByConversationId[id], threadedMessages: [] } } }));
      return { ticket: 'ticket', invocationId: 'invocation', commandId: 'chat.message.copy', handoffId: 'handoff' };
    });
    await act(async () => { await executeChatMessaging(port(), target!); });
    expect(navigator.clipboard.writeText).not.toHaveBeenCalled();
  });
  it('edit.open abre edição selecionada sem ledger nem Wails', async () => {
    mount(); const target = capture('chat.message.edit.open');
    await act(async () => { await executeChatMessaging(port(), target!); });
    expect(node().querySelector('textarea')).toHaveValue('**primeira**');
    expect(begin).not.toHaveBeenCalled(); expect(prepareMessage).not.toHaveBeenCalled(); expect(complete).not.toHaveBeenCalled();
  });
  it.each(['chat.message.pin.toggle', 'chat.message.delete'])('%s commita sem endpoint legado e refresh só confirmado', async commandID => {
    mount(); const target = capture(commandID);
    const refresh = vi.spyOn(useChatStore.getState(), 'loadConversationSession').mockResolvedValue(undefined);
    await act(async () => { await executeChatMessaging(port(), target!); });
    expect(commitMessage).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff');
    expect(getResult).toHaveBeenCalledOnce();
    expect(refresh).toHaveBeenCalledWith(id, { refreshSurfaceWindows: true });
  });
  it('speak usa a mensagem focada e não a última', async () => {
    mount(); const target = capture('chat.message.speak');
    await act(async () => { await executeChatMessaging(port(), target!); });
    expect(prepareMessage).toHaveBeenCalledExactlyOnceWith('ticket', firstID);
    expect(complete).toHaveBeenCalledWith('ticket', 'handoff', 'succeeded');
    expect(announceMessage).toHaveBeenCalledWith(expect.stringContaining('primeira'));
  });
  it('F2 usa comando local e Ctrl+C no editor preserva edição nativa', async () => {
    mount(); act(() => node().focus());
    fireEvent.keyDown(node(), { key: 'F2' });
    await act(async () => { await Promise.all(runs); });
    const editor = node().querySelector('textarea')!;
    expect(editor).toHaveValue('**primeira**');
    fireEvent.keyDown(editor, { key: 'c', ctrlKey: true });
    expect(navigator.clipboard.writeText).not.toHaveBeenCalled();
    expect(begin).not.toHaveBeenCalled();
  });
  it('fala confirma início sem esperar fim; guard sobrevive dispose e listeners terminam no fim', async () => {
    mount(); const target = capture('chat.message.speak');
    vi.mocked(ttsService.getVoiceContext).mockReturnValue({ providerId: 'openai', voiceId: 'v', model: 'tts', rate: 1 });
    let finish!: (value: boolean) => void;
    let start!: () => void;
    let playbackCurrent!: () => boolean;
    vi.spyOn(messageAudioService, 'speakMessage').mockImplementation(async (_id, _volume, _provider, guard, onStarted) => {
      playbackCurrent = guard!; start = onStarted!;
      return await new Promise<boolean>(resolve => { finish = resolve; });
    });
    const unsubscribe = vi.fn();
    const original = useAuthStore.subscribe;
    const subscription = vi.spyOn(useAuthStore, 'subscribe').mockImplementation(listener => {
      const off = original(listener); return () => { unsubscribe(); off(); };
    });
    let completed = false;
    const running = executeChatMessaging(port(), target!).then(result => { completed = true; return result; });
    await act(async () => { for (let i = 0; i < 20 && !start; i++) await Promise.resolve(); });
    expect(start).toBeTypeOf('function'); expect(completed).toBe(false);
    await act(async () => { start(); expect(await running).toBe('succeeded'); });
    expect(playbackCurrent()).toBe(true);
    expect(unsubscribe).not.toHaveBeenCalled();
    await act(async () => { finish(true); });
    expect(unsubscribe).toHaveBeenCalledOnce(); subscription.mockRestore();
  });
  it('contexto muda antes de iniciar fala: não confirma início nem espera áudio atrasado', async () => {
    mount(); const target = capture('chat.message.speak');
    vi.mocked(ttsService.getVoiceContext).mockReturnValue({ providerId: 'openai', voiceId: 'v', model: 'tts', rate: 1 });
    let finish!: (value: boolean) => void;
    let guard!: () => boolean;
    vi.spyOn(messageAudioService, 'speakMessage').mockImplementation(async (_id, _volume, _provider, isCurrent) => {
      guard = isCurrent!; return await new Promise<boolean>(resolve => { finish = resolve; });
    });
    const running = executeChatMessaging(port(), target!);
    await act(async () => { for (let i = 0; i < 20 && !guard; i++) await Promise.resolve(); });
    act(() => useAuthStore.setState({ user: { userId: 'other', sessionId: 'other', role: 'admin' } }));
    await act(async () => { expect(await running).toBe('cancelled'); });
    expect(guard()).toBe(false);
    expect(complete).not.toHaveBeenCalledWith('ticket', 'handoff', 'succeeded');
    await act(async () => { finish(false); });
  });
  it('início de áudio pendente expira e resposta atrasada perde autorização', async () => {
    mount(); const target = capture('chat.message.speak');
    vi.mocked(ttsService.getVoiceContext).mockReturnValue({ providerId: 'openai', voiceId: 'v', model: 'tts', rate: 1 });
    let finish!: (value: boolean) => void;
    let guard!: () => boolean;
    vi.spyOn(messageAudioService, 'speakMessage').mockImplementation(async (_id, _volume, _provider, isCurrent) => {
      guard = isCurrent!; return await new Promise<boolean>(resolve => { finish = resolve; });
    });
    vi.useFakeTimers();
    try {
      const running = executeChatMessaging(port(), target!);
      await act(async () => { for (let i = 0; i < 20 && !guard; i++) await Promise.resolve(); });
      expect(guard).toBeTypeOf('function');
      await act(async () => { await vi.advanceTimersByTimeAsync(20_000); expect(await running).toBe('failed'); });
      expect(guard()).toBe(false);
      expect(complete).not.toHaveBeenCalledWith('ticket', 'handoff', 'succeeded');
      await act(async () => { finish(false); });
    } finally { vi.useRealTimers(); }
  });
  it('sem voz configurada oferece configuração e cancelamento não navega', async () => {
    vi.mocked(ttsService.hasVoiceConfig).mockReturnValue(false);
    mount(); act(() => node().focus()); fireEvent.keyDown(node(), { key: ' ' });
    await act(async () => {});
    expect(configure).toHaveBeenCalledOnce(); expect(deepLink).not.toHaveBeenCalled();
    expect(captureChatMessagingTarget(() => '/', 'chat.message.speak')).toBeUndefined();
    expect(begin).not.toHaveBeenCalled();
  });
  it('configurar voz abre perfil capturado sem enviar mensagem', async () => {
    vi.mocked(ttsService.hasVoiceConfig).mockReturnValue(false); configure.mockResolvedValue(true);
    mount(); act(() => node().focus()); fireEvent.keyDown(node(), { key: ' ' });
    await act(async () => {});
    expect(deepLink).toHaveBeenCalledWith(expect.objectContaining({ resource: 'profiles', resourceId: 'default', tab: 'voice' }), expect.objectContaining({ caller: expect.objectContaining({ conversationId: id, tabId: 'tab' }) }));
    expect(begin).not.toHaveBeenCalled();
  });
  it('troca owner durante configuração recusa navegação após confirmação', async () => {
    vi.mocked(ttsService.hasVoiceConfig).mockReturnValue(false);
    configure.mockImplementationOnce(async () => { useAuthStore.setState({ user: { userId: 'other', sessionId: 'new', role: 'admin' } }); return true; });
    mount(); act(() => node().focus()); fireEvent.keyDown(node(), { key: ' ' });
    await act(async () => {});
    expect(configure).toHaveBeenCalledOnce(); expect(deepLink).not.toHaveBeenCalled();
  });
});
