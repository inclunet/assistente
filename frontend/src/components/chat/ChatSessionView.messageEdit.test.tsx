import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
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


const id = '01926b90-7a5a-7c4e-8d3f-000000000001';
const send = vi.hoisted(() => vi.fn());
const prepareMessage = vi.hoisted(() => vi.fn(async () => {}));
const prepareEdit = vi.hoisted(() => vi.fn(async (_ticket: string, _messageId: string, _original: string, _content: string) => {}));
const events = vi.hoisted(() => new Map<string, Set<(event: unknown) => void>>());
const commitMessage = vi.hoisted(() => vi.fn(async () => {}));
vi.mock('../../lib/commandChatMessageWails', () => ({ prepareChatMessageCommand: prepareMessage, prepareChatMessageEditCommand: prepareEdit, commitChatMessageCommand: commitMessage }));
const retry = vi.hoisted(() => vi.fn());
const t = vi.hoisted(() => (key: string) => key);
const announceMessage = vi.hoisted(() => vi.fn());
const configure = vi.hoisted(() => vi.fn(async () => false));
const deepLink = vi.hoisted(() => vi.fn(async () => {}));
vi.mock('../../store/confirmStore', () => ({ requestConfirm: configure }));
vi.mock('../../lib/deepLinks', () => ({ executeDeepLink: deepLink }));
vi.mock('react-i18next', () => ({ initReactI18next: { type: '3rdParty', init() {} }, useTranslation: () => ({ t }) }));
vi.mock('@wailsjs/go/wailsapi/Chat', () => ({ SendMessage: send, RetryMessage: retry }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, callback: (event: unknown) => void) => {
  const callbacks = events.get(name) ?? new Set(); events.set(name, callbacks); callbacks.add(callback);
  return () => callbacks.delete(callback);
} }));
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
let status: 'succeeded' | 'denied' | 'failed' | 'outcome_unknown' = 'succeeded';
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
  take.mockReset().mockResolvedValue({ ticket: 'ticket', invocationId: 'invocation', commandId: 'chat.message.edit.save', handoffId: 'handoff' });
  commit.mockReset().mockResolvedValue(undefined); complete.mockReset().mockImplementation(async (_ticket, _handoff, completed) => { status = completed; });
  getResult.mockReset().mockImplementation(async () => ({ invocationId: 'invocation', status }));
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'owner', sessionId: 'session', role: 'admin' } });
  useWorkspaceStore.setState({ workspace: { id: 'workspace', activeTabId: 'tab', tabs: [{ id: 'tab', type: 'chat', conversationId: id }] } as NonNullable<ReturnType<typeof useWorkspaceStore.getState>['workspace']> });
  prepareMessage.mockClear(); prepareEdit.mockReset().mockResolvedValue(undefined); commitMessage.mockReset().mockResolvedValue(undefined);
  announceMessage.mockClear();
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
});
afterEach(async () => {
  cleanup();
  useChatStore.getState().cancelConversationTurn(id);
  await Promise.allSettled(runs.splice(0));
  window.removeEventListener(CHAT_MESSAGING_EVENT, listener);
  vi.restoreAllMocks();
});

const firstID = '01926b90-7a5a-7c4e-8d3f-000000000011';
const secondID = '01926b90-7a5a-7c4e-8d3f-000000000012';
function node(messageID = firstID) { return document.querySelector<HTMLElement>('[data-message-id="' + messageID + '"]')!; }

const saveID = 'chat.message.edit.save';
function emitUpdate(content: string, conversationId = id) {
  events.get('message:updated')?.forEach(callback => callback({ conversationId, messageId: firstID, content }));
}
async function openEditor(content = 'editado') {
  act(() => node().focus());
  fireEvent.keyDown(node(), { key: 'F2' });
  await act(async () => { await runs[runs.length - 1]; });
  const editor = node().querySelector<HTMLTextAreaElement>('textarea')!;
  expect(editor).not.toBeNull();
  fireEvent.change(editor, { target: { value: content } });
  return editor;
}
function clickSave() { fireEvent.click(screen.getByRole('button', { name: 'common.save' })); }
describe('salvar edição: MessageNode, fonte draft, store e executor reais', () => {
  it('evento scoped atualiza lista antesGetResult e sucesso fecha apenas draft capturado', async () => {
    mount(); await openEditor('  novo texto  ');
    commitMessage.mockImplementationOnce(async () => {
      expect(node().querySelector('textarea')).toHaveValue('  novo texto  ');
      expect(useChatStore.getState().getConversationMessages(id)[0].content).toBe('**primeira**');
      emitUpdate('  novo texto  ');
    });
    clickSave(); await act(async () => { await Promise.all(runs); });
    expect(prepareEdit).toHaveBeenCalledExactlyOnceWith('ticket', firstID, '**primeira**', '  novo texto  ');
    expect(commitMessage).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff');
    expect(node().querySelector('textarea')).toBeNull();
    expect(node()).toHaveTextContent('novo texto');
    expect(complete).not.toHaveBeenCalledWith('ticket', 'handoff', 'succeeded');
  });
  it('não aceita evento cross-conversa ou alias legado', async () => {
    mount(); await openEditor();
    act(() => {
      emitUpdate('externo', '01926b90-7a5a-7c4e-8d3f-000000000099');
      events.get('message:updated')?.forEach(callback => callback({ message_id: firstID, content: 'legado' }));
    });
    expect(useChatStore.getState().getConversationMessages(id)[0].content).toBe('**primeira**');
    expect(captureChatMessagingTarget(() => '/', saveID)).toBeDefined();
  });
  it('base original atualizada durante edição recusa salvar mesmo depois ABA', async () => {
    mount(); await openEditor();
    act(() => { emitUpdate('concorrente'); emitUpdate('**primeira**'); });
    expect(captureChatMessagingTarget(() => '/', saveID)).toBeUndefined();
    clickSave(); await act(async () => { await Promise.all(runs); });
    expect(begin).not.toHaveBeenCalled(); expect(commitMessage).not.toHaveBeenCalled();
    expect(node().querySelector('textarea')).toHaveValue('editado');
    expect(announceMessage).toHaveBeenCalledWith('chat.editSaveError');
  });
  it('draft A-B-A durante prepare cancela antes Take', async () => {
    mount(); const editor = await openEditor('A');
    prepareEdit.mockImplementationOnce(async () => { fireEvent.change(editor, { target: { value: 'B' } }); fireEvent.change(editor, { target: { value: 'A' } }); });
    clickSave(); await act(async () => { await Promise.all(runs); });
    expect(take).not.toHaveBeenCalled(); expect(commitMessage).not.toHaveBeenCalled();
    expect(editor).toHaveValue('A');
  });
  it('texto novo após commit não é apagado ao chegar succeeded', async () => {
    mount(); const editor = await openEditor('enviado');
    commitMessage.mockImplementationOnce(async () => { emitUpdate('enviado'); fireEvent.change(editor, { target: { value: 'novo rascunho' } }); });
    clickSave(); await act(async () => { await Promise.all(runs); });
    expect(node().querySelector('textarea')).toHaveValue('novo rascunho');
    expect(useChatStore.getState().getConversationMessages(id)[0].content).toBe('enviado');
    commitMessage.mockImplementationOnce(async () => { emitUpdate('novo rascunho'); });
    clickSave(); await act(async () => { await Promise.all(runs); });
    expect(prepareEdit).toHaveBeenLastCalledWith('ticket', firstID, 'enviado', 'novo rascunho');
    expect(commitMessage).toHaveBeenCalledTimes(2);
    expect(node().querySelector('textarea')).toBeNull();
  });
  it.each(['workspace', 'owner', 'selection'])('sucesso após %s ABA não fecha/foca edição obsoleta', async kind => {
    mount(); await openEditor();
    getResult.mockImplementationOnce(async () => {
      if (kind === 'workspace') {
        const ws = useWorkspaceStore.getState().workspace!;
        useWorkspaceStore.setState({ workspace: { ...ws, activeTabId: 'other' } });
        useWorkspaceStore.setState({ workspace: ws });
      } else if (kind === 'owner') {
        const user = useAuthStore.getState().user;
        useAuthStore.setState({ user: { userId: 'other', sessionId: 'other', role: 'admin' } });
        useAuthStore.setState({ user });
      } else { node(secondID).focus(); node().focus(); }
      return { invocationId: 'invocation', status: 'succeeded' };
    });
    clickSave(); await act(async () => { await Promise.all(runs); });
    expect(node().querySelector('textarea')).toHaveValue('editado');
  });
  it.each(['succeeded', 'outcome_unknown'] as const)('commit transportlost reconcilia %s sem repetir', async result => {
    mount(); await openEditor();
    commitMessage.mockImplementationOnce(async () => { if (result === 'succeeded') emitUpdate('editado'); throw new Error('lost response'); });
    status = result;
    clickSave(); await act(async () => { await Promise.all(runs); });
    expect(commitMessage).toHaveBeenCalledOnce(); expect(getResult).toHaveBeenCalledOnce();
    if (result === 'succeeded') expect(node().querySelector('textarea')).toBeNull();
    else expect(node().querySelector('textarea')).toHaveValue('editado');
  });
  it('cancelar e reabrir enquanto GetResult aguarda mantém nova edição', async () => {
    mount(); await openEditor('primeiro draft');
    let resolve!: (result: { invocationId: string; status: string }) => void;
    getResult.mockImplementationOnce(() => new Promise(done => { resolve = done; }));
    clickSave(); await waitFor(() => expect(getResult).toHaveBeenCalledOnce());
    fireEvent.click(screen.getByRole('button', { name: 'common.cancel' }));
    await openEditor('reaberto');
    await act(async () => { resolve({ invocationId: 'invocation', status: 'succeeded' }); await Promise.all(runs); });
    expect(node().querySelector('textarea')).toHaveValue('reaberto');
  });
  it('CtrlEnter repetido/composição/consumido não envia; normal prepara snapshot exato', async () => {
    mount(); const editor = await openEditor();
    fireEvent.keyDown(editor, { key: 'Enter', ctrlKey: true, repeat: true });
    fireEvent.keyDown(editor, { key: 'Enter', ctrlKey: true, isComposing: true });
    fireEvent.keyDown(editor, { key: 'Enter', ctrlKey: true, keyCode: 229 });
    const consumed = new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true, cancelable: true }); consumed.preventDefault(); fireEvent(editor, consumed);
    expect(begin).not.toHaveBeenCalled();
    fireEvent.keyDown(editor, { key: 'Enter', ctrlKey: true });
    await act(async () => { await Promise.all(runs); });
    expect(begin).toHaveBeenCalledExactlyOnceWith(saveID); expect(commitMessage).toHaveBeenCalledOnce();
  });
  it('fora da edição e textarea de outra superfície recusam', async () => {
    mount(); act(() => node().focus());
    expect(captureChatMessagingTarget(() => '/', saveID)).toBeUndefined();
    const editor = await openEditor();
    const accepted = captureChatMessagingTarget(() => '/', saveID, undefined, editor);
    expect(accepted).toBeDefined(); accepted?.dispose();
    expect(captureChatMessagingTarget(() => '/', saveID, undefined, screen.getByRole('combobox'))).toBeUndefined();
  });
});
