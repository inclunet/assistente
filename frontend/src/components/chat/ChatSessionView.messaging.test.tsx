import { withFixtureProjection } from '../../test/chatWindowFixture';
import { nodeFixture } from '../../test/chatPresentationFixture';
vi.mock('./useChatTimelinePresentation', async () => import('./canonicalPresentationFixture'));
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ChatSessionView } from './ChatSessionView';
import { sendChatSurfaceMessage } from './ChatSurfaceController';
import { useChatStore } from '../../store/chatStore';
import { useAuthStore } from '../../store/authStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { createChatSurfaceIdentity, createEmptyChatSurfaceSession } from '../../services/chatSessionRegistry';
import { CHAT_MESSAGING_EVENT, captureChatMessagingTarget, executeChatMessaging, type ChatMessagingRequest } from '../../lib/commandChatMessaging';
import { MediaCategory, type MediaFile } from '../../services/mediaService';
import { Modal } from '../ui/Modal';
import { useWorkspaceChatModalStore } from '../../store/workspaceChatModalStore';
import { setFixtureCanonicalItem, publishFixtureMessage } from './canonicalPresentationFixture';
import { canonicalTestItem } from '../../services/chatTimelineTestFixture';

const id = '01926b90-7a5a-7c4e-8d3f-000000000001';
const send = vi.hoisted(() => vi.fn());
const retry = vi.hoisted(() => vi.fn());
const t = vi.hoisted(() => (key: string) => key);
const translation = vi.hoisted(() => ({ current: (key: string) => key }));
const continuationProfile = vi.hoisted(() => ({ enabled: true, surfaceEnabled: true }));
vi.mock('react-i18next', () => ({ initReactI18next: { type: '3rdParty', init() {} }, useTranslation: () => ({ t: translation.current }) }));
vi.mock('@wailsjs/go/wailsapi/Chat', () => ({ SendMessage: send, RetryMessage: retry }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: () => () => {} }));
vi.mock('@wailsjs/go/wailsapi/Profiles', () => ({ GetProfile: vi.fn(async () => ({ chat: { streaming_recovery_show_continue: continuationProfile.surfaceEnabled } })), GetActiveProfile: vi.fn(async () => ({ chat: { streaming_recovery_show_continue: continuationProfile.enabled } })), GetActiveProfileSlug: vi.fn(async () => 'default') }));
vi.mock('@wailsjs/go/wailsapi/Skills', () => ({ GetUserInvocableSkillsForProfile: vi.fn(async () => []) }));
vi.mock('../../services/audioFeedback', () => ({ playSendSound: vi.fn(), playReceiveSound: vi.fn(), playChatErrorSound: vi.fn() }));
vi.mock('../../hooks/useAnnouncer', () => ({ announce: vi.fn(), useAnnouncer: () => ({ announce: vi.fn(), announceRequest: vi.fn() }) }));
vi.mock('../../services/tts', () => ({ ttsService: { isEnabled: () => false, hasVoiceConfig: () => false, isEnabledForUser: () => false, shouldUseAriaLiveForUser: () => false, isAutoReadEnabled: () => false, stop: vi.fn(), on: vi.fn(), off: vi.fn(), getVoiceContext: () => ({}), shouldUseAriaLiveForAgent: () => false } }));
vi.mock('./ChatToolbar', () => ({ ChatToolbar: () => null }));
vi.mock('./useAgentSessionCommands', () => ({ useAgentSessionCommands: () => [] }));
vi.mock('./VoiceButton', () => ({ VoiceButton: () => null }));
vi.mock('./MessageList', async () => {
  const React = await import('react');
  return { MessageList: React.forwardRef<HTMLDivElement>((_, ref) => <div ref={ref}>
    <button data-message-node data-message-id="response">first message</button>
    <button data-message-node data-message-id="other">other message</button>
  </div>) };
});
vi.mock('../workspace/WorkspacePanelContext', () => ({ useWorkspacePanel: () => ({ tab: { id: 'tab', type: 'chat' }, isActive: true }) }));
const surface = createChatSurfaceIdentity({ conversationId: id, tabId: 'tab', surfaceType: 'page' });
const runs: Promise<unknown>[] = [];
let listener: (event: Event) => void;
let status: 'succeeded' | 'denied' | 'failed' | 'outcome_unknown' = 'succeeded';
const begin = vi.fn();
const take = vi.fn();
const commit = vi.fn();
const complete = vi.fn();
const getResult = vi.fn();
function mount(profileSlug?: string) {
  return render(<MemoryRouter><ChatSessionView surface={surface} profileSlug={profileSlug} onSend={(content, media, origin, command) => sendChatSurfaceMessage(id, content, media, undefined, origin, command)} /></MemoryRouter>);
}
beforeEach(() => {
  continuationProfile.enabled = true;
  setFixtureCanonicalItem('response');
  translation.current = t;
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  send.mockReset().mockResolvedValue(id); retry.mockReset().mockResolvedValue(id);
  status = 'succeeded';
  begin.mockReset().mockImplementation(async (commandId: string) => ({ ticket: 'ticket', invocationId: 'invocation', commandId }));
  take.mockReset().mockResolvedValue({ ticket: 'ticket', invocationId: 'invocation', commandId: 'chat.message.send', handoffId: 'handoff' });
  commit.mockReset().mockResolvedValue(undefined); complete.mockReset().mockResolvedValue(undefined);
  getResult.mockReset().mockImplementation(async () => ({ invocationId: 'invocation', status }));
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'owner', sessionId: 'session', role: 'admin' } });
  useWorkspaceStore.setState({ workspace: { id: 'workspace', activeTabId: 'tab', tabs: [{ id: 'tab', type: 'chat', conversationId: id }] } as NonNullable<ReturnType<typeof useWorkspaceStore.getState>['workspace']> });
  const conversation = withFixtureProjection({ id, title: 'chat' }, { nodes: [] });
  useChatStore.setState({
    loadingConversationIds: new Set(),
    conversationInfoById: { [id]: conversation },
    surfaceSessionsByKey: { ['conversation:' + id]: { ...createEmptyChatSurfaceSession(id) } },
  });
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
describe('ChatSessionView + ChatInput + pipeline reais', () => {
  it.each([true, false])('admite continuação conforme perfil da superfície: %s', async enabled => {
    continuationProfile.enabled = !enabled;
    continuationProfile.surfaceEnabled = enabled;
    const item = canonicalTestItem('response', 'parcial', { conversationId: id, turnId: id, representativeMessageId: id, state: 'interrupted' });
    item.metadata.terminalReason = 'cancelled';
    setFixtureCanonicalItem('response', item);
    useChatStore.setState(state => ({ conversationInfoById: { ...state.conversationInfoById, [id]: withFixtureProjection({ id, title: 'chat' }, { nodes: [nodeFixture({ message: { id: 'response', conversationId: id, role: 'assistant', content: 'parcial', turnId: id }, children: [], childCount: 0 })] }) } }));
    await act(async () => { mount('surface-profile'); });
    const target = captureChatMessagingTarget(() => '/', 'chat.message.retry', undefined, undefined, { sourceItemId: 'response', messageId: id, continue: true });
    expect(!!target).toBe(enabled);
  });

  it('recusa reenvio cuja fonte já saiu da apresentação', async () => {
    await act(async () => { mount(); });
    expect(captureChatMessagingTarget(() => '/', 'chat.message.retry', undefined, undefined, { sourceItemId: 'removed', messageId: id })).toBeUndefined();
    expect(begin).not.toHaveBeenCalled();
  });

  it.each(['volatile', 'profile-disabled', 'persisted'] as const)('prepara continuação pela publicação canônica: %s', async scenario => {
    continuationProfile.enabled = scenario !== 'profile-disabled';
    const item = canonicalTestItem('response', 'parcial', { conversationId: id, turnId: id, representativeMessageId: id, state: 'interrupted', durability: scenario === 'volatile' ? 'volatile' : 'persisted' });
    item.metadata.terminalReason = 'cancelled';
    setFixtureCanonicalItem('response', item);
    useChatStore.setState(state => ({ conversationInfoById: { ...state.conversationInfoById, [id]: withFixtureProjection({ id, title: 'chat' }, { nodes: [nodeFixture({ message: { id: 'response', conversationId: id, role: 'assistant', content: 'parcial', turnId: id }, children: [], childCount: 0 })] }) } }));
    await act(async () => { mount(); });
    const target = captureChatMessagingTarget(() => '/', 'chat.message.retry', undefined, undefined, { sourceItemId: 'response', messageId: id, continue: true });
    if (scenario !== 'persisted') {
      expect(target).toBeUndefined();
      expect(retry).not.toHaveBeenCalled();
      return;
    }
    expect(target).toBeDefined();
    take.mockResolvedValue({ ticket: 'ticket', invocationId: 'invocation', commandId: 'chat.message.retry', handoffId: 'handoff' });
    await act(async () => { await executeChatMessaging({ beginUICommand: begin, takeUICommand: take, commitBackendCommand: commit, completeUICommand: complete, cancelUICommand: vi.fn(), getUICommandResult: getResult }, target!); });
    expect(retry).toHaveBeenCalledExactlyOnceWith(id, id, expect.objectContaining({ allowAssistantPrefill: true }));
    expect(send).not.toHaveBeenCalled();
  });

  it('recusa a continuação capturada quando a publicação é substituída antes do envio', async () => {
    const item = canonicalTestItem('response', 'parcial', { conversationId: id, turnId: id, representativeMessageId: id, state: 'interrupted' });
    item.metadata.terminalReason = 'cancelled';
    setFixtureCanonicalItem('response', item);
    useChatStore.setState(state => ({ conversationInfoById: { ...state.conversationInfoById, [id]: withFixtureProjection({ id, title: 'chat' }, { nodes: [nodeFixture({ message: { id: 'response', conversationId: id, role: 'assistant', content: 'parcial', turnId: id }, children: [], childCount: 0 })] }) } }));
    await act(async () => { mount(); });
    const target = captureChatMessagingTarget(() => '/', 'chat.message.retry', undefined, undefined, { sourceItemId: 'response', messageId: id, continue: true });
    expect(target).toBeDefined();
    act(() => publishFixtureMessage(id, 'response', 'outra execução'));
    expect(target!.isCurrent()).toBe(false);
    await act(async () => { await executeChatMessaging({ beginUICommand: begin, takeUICommand: take, commitBackendCommand: commit, completeUICommand: complete, cancelUICommand: vi.fn(), getUICommandResult: getResult }, target!); });
    expect(retry).not.toHaveBeenCalled();
    expect(begin).not.toHaveBeenCalled();
  });

  it.each(['begin', 'take'] as const)('recusa continuar se a seleção mudar durante %s', async phase => {
    const item = canonicalTestItem('response', 'parcial', { conversationId: id, turnId: id, representativeMessageId: id, state: 'interrupted' });
    item.metadata.terminalReason = 'cancelled';
    setFixtureCanonicalItem('response', item);
    useChatStore.setState(state => ({ conversationInfoById: { ...state.conversationInfoById, [id]: withFixtureProjection({ id, title: 'chat' }, { nodes: [nodeFixture({ message: { id: 'response', conversationId: id, role: 'assistant', content: 'parcial', turnId: id }, children: [], childCount: 0 })] }) } }));
    await act(async () => { mount(); });
    act(() => screen.getByRole('button', { name: 'first message' }).focus());
    const target = captureChatMessagingTarget(() => '/', 'chat.message.retry', undefined, undefined, { sourceItemId: 'response', messageId: id, continue: true });
    expect(target).toBeDefined();
    let release!: (value: unknown) => void;
    const pipelineRevision = useChatStore.getState().getMessagingPipelineRevision(id);
    const pending = new Promise(resolve => { release = resolve; });
    (phase === 'begin' ? begin : take).mockReturnValueOnce(pending);
    let running!: Promise<unknown>;
    await act(async () => {
      running = executeChatMessaging({ beginUICommand: begin, takeUICommand: take, commitBackendCommand: commit, completeUICommand: complete, cancelUICommand: vi.fn(), getUICommandResult: getResult }, target!);
      await Promise.resolve();
    });
    await waitFor(() => expect(phase === 'begin' ? begin : take).toHaveBeenCalledOnce());
    act(() => screen.getByRole('button', { name: 'other message' }).focus());
    expect(target!.isCurrent()).toBe(false);
    await act(async () => {
      release({ ticket: 'ticket', invocationId: 'invocation', commandId: 'chat.message.retry', handoffId: 'handoff' });
      await running;
    });
    expect(retry).not.toHaveBeenCalled();
    expect(send).not.toHaveBeenCalled();
    expect(useChatStore.getState().getMessagingPipelineRevision(id)).toBe(pipelineRevision);
  });

  it.each(['button', 'keyboard'])('preserva rascunho e explica conversa indisponível via %s', async source => {
    begin.mockRejectedValue('chat_conversation_unavailable');
    mount(); const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'rascunho preservado' } });
    if (source === 'button') fireEvent.click(screen.getByRole('button', { name: 'chat.send' }));
    else fireEvent.keyDown(input, { key: 'Enter' });
    await act(async () => { await Promise.all(runs); });
    expect(screen.getByText('chat.conversationUnavailable')).toBeInTheDocument();
    expect(input).toHaveValue('rascunho preservado');
    expect(send).not.toHaveBeenCalled(); expect(retry).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: 'chat.retryAriaLabel' })).not.toBeInTheDocument();
  });


  it('não oferece retry de uma falha anterior após detectar conversa indisponível', async () => {
    begin.mockRejectedValue('chat_conversation_unavailable'); mount();
    act(() => useChatStore.setState(state => ({ surfaceSessionsByKey: { ...state.surfaceSessionsByKey,
      [surface.sessionKey]: { ...state.surfaceSessionsByKey[surface.sessionKey], sendFailureMessage: 'falha anterior',
        sendFailureRetryable: true, sendFailureRetryContent: 'texto anterior', sendFailureRetryMediaFiles: [] },
    } })));
    const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'rascunho atual' } });
    fireEvent.click(screen.getByRole('button', { name: 'chat.send' }));
    await act(async () => { await Promise.all(runs); });
    expect(screen.getByText('chat.conversationUnavailable')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'chat.retryAriaLabel' })).not.toBeInTheDocument();
    expect(input).toHaveValue('rascunho atual');
    expect(useChatStore.getState().surfaceSessionsByKey[surface.sessionKey].sendFailureRetryContent).toBe('texto anterior');
  });

  it('não leva o aviso de conversa indisponível para outra conversa na mesma aba', async () => {
    begin.mockRejectedValueOnce('chat_conversation_unavailable');
    const view = mount(); const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'rascunho da origem' } });
    fireEvent.click(screen.getByRole('button', { name: 'chat.send' }));
    await act(async () => { await Promise.all(runs); });
    expect(screen.getByText('chat.conversationUnavailable')).toBeInTheDocument();
    expect(input).toHaveValue('rascunho da origem');
    const nextID = '01926b90-7a5a-7c4e-8d3f-000000000002';
    const nextSurface = createChatSurfaceIdentity({ conversationId: nextID, tabId: 'tab', surfaceType: 'page' });
    const nextConversation = withFixtureProjection({ id: nextID, title: 'outra conversa' }, { nodes: [] });
    act(() => {
      useChatStore.setState((state) => ({
        conversationInfoById: { ...state.conversationInfoById, [nextID]: nextConversation },
        surfaceSessionsByKey: {
          ...state.surfaceSessionsByKey,
          ['conversation:' + nextID]: { ...createEmptyChatSurfaceSession(nextID) },
        },
      }));
      useWorkspaceStore.setState({ workspace: { id: 'workspace', activeTabId: 'tab', tabs: [{ id: 'tab', type: 'chat', conversationId: nextID }] } as NonNullable<ReturnType<typeof useWorkspaceStore.getState>['workspace']> });
    });
    view.rerender(<MemoryRouter><ChatSessionView surface={nextSurface} onSend={(content, media, origin, command) => sendChatSurfaceMessage(nextID, content, media, undefined, origin, command)} /></MemoryRouter>);
    expect(screen.queryByText('chat.conversationUnavailable')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'chat.retryAriaLabel' })).not.toBeInTheDocument();
    expect(screen.getByRole('combobox')).toHaveValue('');
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'mensagem da nova conversa' } });
    fireEvent.click(screen.getByRole('button', { name: 'chat.send' }));
    await act(async () => { await Promise.all(runs); });
    expect(send).toHaveBeenCalledOnce();
    expect(send.mock.calls[0][0]).toBe(nextID);
  });
  it('trocar a tradução durante envio não abandona o recibo nem mantém rascunho já enviado', async () => {
    let release!: () => void;
    send.mockImplementation(() => new Promise(resolve => { release = () => resolve(id); }));
    const view = mount(); const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'enviado uma vez' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    await waitFor(() => expect(send).toHaveBeenCalledOnce());
    translation.current = key => key;
    view.rerender(<MemoryRouter><ChatSessionView surface={surface} onSend={(content, media, origin, command) => sendChatSurfaceMessage(id, content, media, undefined, origin, command)} /></MemoryRouter>);
    await act(async () => { release(); await Promise.all(runs); });
    expect(input).toHaveValue('');
    expect(send).toHaveBeenCalledOnce();
  });
  it('Enter envia uma vez com recibo fora de origin e limpa após resultado confirmado', async () => {
    mount();
    const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'rascunho' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(input).toHaveValue('rascunho');
    await waitFor(() => expect(send).toHaveBeenCalledOnce());
    await act(async () => { await Promise.all(runs); });
    expect(send.mock.calls[0][1]).toBe('rascunho');
    expect(send.mock.calls[0][3].command).toEqual({ ticket: 'ticket', handoffId: 'handoff' });
    expect(useChatStore.getState().surfaceSessionsByKey[surface.sessionKey].surfaceOrigin).not.toHaveProperty('command');
    expect(input).toHaveValue('');
    expect(commit).not.toHaveBeenCalled();
  });
  it('rascunho alterado durante Take invalida captura e não chega a Wails', async () => {
    let release!: () => void;
    take.mockImplementation(async () => { await new Promise<void>(resolve => { release = resolve; }); return { ticket: 'ticket', invocationId: 'invocation', commandId: 'chat.message.send', handoffId: 'handoff' }; });
    mount(); const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'primeiro' } }); fireEvent.keyDown(input, { key: 'Enter' });
    await waitFor(() => expect(take).toHaveBeenCalledOnce());
    fireEvent.change(input, { target: { value: 'segundo' } });
    await act(async () => { release(); await Promise.all(runs); });
    expect(send).not.toHaveBeenCalled(); expect(input).toHaveValue('segundo');
    expect(complete).toHaveBeenCalledWith('ticket', 'handoff', 'cancelled');
  });
  it('denied preserva draft e falha de transporte não habilita retry como mensagem nova', async () => {
    status = 'denied'; mount(); const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'preservar' } }); fireEvent.keyDown(input, { key: 'Enter' });
    await act(async () => { await Promise.all(runs); });
    expect(input).toHaveValue('preservar');
    expect(useChatStore.getState().surfaceSessionsByKey[surface.sessionKey].sendFailureRetryable).toBe(false);
    expect(useChatStore.getState().surfaceSessionsByKey[surface.sessionKey].isLoading).toBe(false);
  });
  it('outcome_unknown após rejeição Wails apresenta erro, preserva draft e não oferece replay', async () => {
    send.mockRejectedValue(new Error('transport lost'));
    status = 'outcome_unknown';
    mount(); const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'incerto' } }); fireEvent.keyDown(input, { key: 'Enter' });
    await act(async () => { await Promise.all(runs); });
    const session = useChatStore.getState().surfaceSessionsByKey[surface.sessionKey];
    expect(getResult).toHaveBeenCalledOnce();
    expect(send).toHaveBeenCalledOnce();
    expect(retry).not.toHaveBeenCalled();
    expect(input).toHaveValue('incerto');
    expect(session.sendFailureMessage).toContain('transport lost');
    expect(session.sendFailureRetryable).toBe(false);
    expect(session.sendFailureRetryContent).toBeNull();
    expect(session.isLoading).toBe(true);
    expect(screen.queryByRole('button', { name: 'chat.retryAriaLabel' })).not.toBeInTheDocument();
  });
  it('resultado indisponível após rejeição Wails também preserva draft sem replay', async () => {
    send.mockRejectedValue(new Error('transport lost'));
    getResult.mockRejectedValue(new Error('result unavailable'));
    mount(); const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'incerto sem consulta' } }); fireEvent.keyDown(input, { key: 'Enter' });
    await act(async () => { await Promise.all(runs); });
    const session = useChatStore.getState().surfaceSessionsByKey[surface.sessionKey];
    expect(getResult).toHaveBeenCalledOnce();
    expect(send).toHaveBeenCalledOnce();
    expect(retry).not.toHaveBeenCalled();
    expect(input).toHaveValue('incerto sem consulta');
    expect(session.sendFailureMessage).toContain('transport lost');
    expect(session.sendFailureRetryable).toBe(false);
    expect(session.isLoading).toBe(true);
    expect(screen.queryByRole('button', { name: 'chat.retryAriaLabel' })).not.toBeInTheDocument();
  });
  it('captura texto/revisão da mesma store antes de React rerender', async () => {
    mount();
    let target: ReturnType<typeof captureChatMessagingTarget>;
    act(() => {
      useChatStore.getState().setConversationDraftMessage(id, 'atual síncrono', surface.sessionKey);
      target = captureChatMessagingTarget(() => '/', 'chat.message.send');
    });
    expect(target).toBeDefined();
    await act(async () => {
      await executeChatMessaging({ beginUICommand: begin, takeUICommand: take, completeUICommand: complete, commitBackendCommand: commit, cancelUICommand: vi.fn(), getUICommandResult: getResult }, target!);
    });
    expect(send.mock.calls[0][1]).toBe('atual síncrono');
    expect(screen.getByRole('combobox')).toHaveValue('');
  });
  it('captura anexos atuais mesmo quando o callback ainda tem props antigas', async () => {
    const onSend = vi.fn(async () => {});
    render(<MemoryRouter><ChatSessionView surface={surface} onSend={onSend} /></MemoryRouter>);
    const media: MediaFile = { id: 'current', file: new File(['payload'], 'current.txt'), category: MediaCategory.DOCUMENT, mimeType: 'text/plain', extension: 'txt', fileName: 'current.txt', fileSize: 7, fileSizeFormatted: '7 B', icon: '' };
    let target: ReturnType<typeof captureChatMessagingTarget>;
    act(() => {
      useChatStore.getState().setConversationDraftMessage(id, 'atual', surface.sessionKey);
      useChatStore.getState().setConversationDraftMediaFiles(id, [media], surface.sessionKey);
      target = captureChatMessagingTarget(() => '/', 'chat.message.send', undefined, undefined, { content: 'atual', media: [] });
    });
    expect(target).toBeDefined();
    await act(async () => {
      await executeChatMessaging({ beginUICommand: begin, takeUICommand: take, completeUICommand: complete, commitBackendCommand: commit, cancelUICommand: vi.fn(), getUICommandResult: getResult }, target!);
    });
    expect(onSend).toHaveBeenCalledOnce();
    expect(onSend.mock.calls[0]).toEqual(expect.arrayContaining([[expect.objectContaining({ id: 'current' })]]));
  });
  it('reconcilia resposta Wails perdida com sucesso autoritativo sem reenviar', async () => {
    send.mockRejectedValue(new Error('transport lost'));
    mount(); const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'confirmado' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    await act(async () => { await Promise.all(runs); });
    expect(getResult).toHaveBeenCalledOnce();
    expect(send).toHaveBeenCalledOnce();
    expect(input).toHaveValue('');
  });
  it('cancel recusado não limpa pipeline nem loading', async () => {
    mount();
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'ativo' } });
    fireEvent.keyDown(screen.getByRole('combobox'), { key: 'Enter' });
    await act(async () => { await Promise.all(runs); });
    const revision = useChatStore.getState().getMessagingPipelineRevision(id);
    const finish = vi.spyOn(useChatStore.getState(), 'finishCommandCancellation');
    status = 'failed';
    take.mockResolvedValue({ ticket: 'ticket', invocationId: 'invocation', commandId: 'chat.response.cancel', handoffId: 'handoff' });
    fireEvent.click(screen.getByRole('button', { name: 'chat.cancelGenerationLabel' }));
    await act(async () => { await Promise.all(runs); });
    expect(commit).toHaveBeenCalledOnce();
    expect(finish).not.toHaveBeenCalled();
    expect(useChatStore.getState().getMessagingPipelineRevision(id)).toBe(revision);
    expect(useChatStore.getState().surfaceSessionsByKey[surface.sessionKey].isLoading).toBe(true);
  });
  it('falha antes de iniciar pipeline não limpa turno anterior', async () => {
    mount();
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'rascunho' } });
    const target = captureChatMessagingTarget(() => '/', 'chat.message.send');
    expect(target).toBeDefined();
    const finish = vi.spyOn(useChatStore.getState(), 'finishCommandCancellation');
    begin.mockRejectedValue(new Error('admission refused'));
    await act(async () => {
      await executeChatMessaging({ beginUICommand: begin, takeUICommand: take, completeUICommand: complete, commitBackendCommand: commit, cancelUICommand: vi.fn(), getUICommandResult: getResult }, target!);
    });
    expect(finish).not.toHaveBeenCalled();
    expect(send).not.toHaveBeenCalled();
    expect(screen.getByRole('combobox')).toHaveValue('rascunho');
  });
  it('modal com binding antigo recusa lease quando conversa da aba muda', async () => {
    useWorkspaceChatModalStore.setState({ isOpen: true, boundTabId: 'tab', boundConversationId: id });
    render(<MemoryRouter><Modal isOpen title="chat" onClose={() => {}}><ChatSessionView surface={surface} onSend={(content, media, origin, command) => sendChatSurfaceMessage(id, content, media, undefined, origin, command)} /></Modal></MemoryRouter>);
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'original' } });
    const target = captureChatMessagingTarget(() => '/', 'chat.message.send');
    expect(target).toBeDefined();
    act(() => useWorkspaceStore.setState(state => ({ workspace: { ...state.workspace!, tabs: state.workspace!.tabs.map(tab => ({ ...tab, conversationId: '01926b90-7a5a-7c4e-8d3f-000000000002' })) } })));
    expect(useWorkspaceChatModalStore.getState().boundConversationId).toBe(id);
    expect(target!.isCurrent()).toBe(false);
    expect(captureChatMessagingTarget(() => '/', 'chat.message.send')).toBeUndefined();
    target!.dispose();
  });
  it('retry de falha pré-submissão usa send e preserva novo rascunho', async () => {
    mount();
    act(() => {
      useChatStore.getState().setConversationDraftMessage(id, 'novo rascunho', surface.sessionKey);
      useChatStore.setState(state => ({ surfaceSessionsByKey: { ...state.surfaceSessionsByKey, [surface.sessionKey]: { ...state.surfaceSessionsByKey[surface.sessionKey], sendFailureMessage: 'falha anterior', sendFailureRetryable: true, sendFailureRetryContent: 'falha anterior', sendFailureRetryMediaFiles: [] } } }));
    });
    fireEvent.click(await screen.findByRole('button', { name: 'chat.retryAriaLabel' }));
    await act(async () => { await Promise.all(runs); });
    expect(begin).toHaveBeenCalledExactlyOnceWith('chat.message.send');
    expect(send.mock.calls[0][1]).toBe('falha anterior');
    expect(retry).not.toHaveBeenCalled();
    expect(screen.getByRole('combobox')).toHaveValue('novo rascunho');
  });
  it('cancel durante serialização não envia depois de liberar leitura e preserva draft', async () => {
    let reader: FileReader | undefined;
    vi.spyOn(FileReader.prototype, 'readAsArrayBuffer').mockImplementation(function (this: FileReader) { reader = this; });
    mount();
    const media: MediaFile = { id: 'media', file: new File(['payload'], 'note.txt', { type: 'text/plain' }), category: MediaCategory.DOCUMENT, mimeType: 'text/plain', extension: 'txt', fileName: 'note.txt', fileSize: 7, fileSizeFormatted: '7 B', icon: '' };
    act(() => { useChatStore.getState().setConversationDraftMessage(id, 'com mídia', surface.sessionKey); useChatStore.getState().setConversationDraftMediaFiles(id, [media], surface.sessionKey); });
    fireEvent.keyDown(screen.getByRole('combobox'), { key: 'Enter' });
    await waitFor(() => expect(reader).toBeDefined());
    expect(send).not.toHaveBeenCalled();
    take.mockResolvedValue({ ticket: 'ticket', invocationId: 'invocation', commandId: 'chat.response.cancel', handoffId: 'handoff' });
    fireEvent.click(screen.getByRole('button', { name: 'chat.cancelGenerationLabel' }));
    await waitFor(() => expect(commit).toHaveBeenCalledOnce());
    await act(async () => { await runs[1]; });
    await act(async () => {
      Object.defineProperty(reader!, 'result', { value: new ArrayBuffer(7) });
      reader!.onload?.call(reader!, new ProgressEvent('load') as ProgressEvent<FileReader>);
      await Promise.all(runs);
    });
    expect(send).not.toHaveBeenCalled();
    expect(screen.getByRole('combobox')).toHaveValue('com mídia');
    expect(useChatStore.getState().surfaceSessionsByKey[surface.sessionKey].isLoading).toBe(false);
  });
});
