import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createElement, useEffect } from 'react';
import { act, render, renderHook } from '@testing-library/react';
import { flushSync } from 'react-dom';
import { Editor } from '@tiptap/core';
import { Modal, useModalId } from '../components/ui/Modal';
import { buildRichTextExtensions } from '../components/editor/buildRichTextExtensions';
import { useEditorInsert } from '../pages/useEditorInsert';
import { prepareChatEditorTransfer } from './commandChatEditorTransfer';
import { registerEditorTransferSurface } from './commandEditorTransfer';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore, type WorkspaceData } from '../store/workspaceStore';
import { useEditorStore } from '../store/editorStore';
import { useChatStore, type Message } from '../store/chatStore';
import { useWorkspaceChatModalStore } from '../store/workspaceChatModalStore';
import { getModalRegistrySnapshot, registerChatPresentationModalScope, registerOpenModal, unregisterOpenModal } from './modalRegistry';

const transport = vi.hoisted(() => ({ prepare: vi.fn(), open: vi.fn(), validate: vi.fn() }));
vi.mock('./commandChatEditorWails', () => ({ prepareChatEditorCommand: transport.prepare, openChatEditorCommand: transport.open, validateChatEditorCommand: transport.validate }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: () => () => {} }));
vi.mock('../services/tts', () => ({ ttsService: { stop: vi.fn() } }));
vi.mock('../services/messageAudio', () => ({ messageAudioService: { stopCurrentAudio: vi.fn() } }));

const message = { id: 'message-a', conversationId: 'conversation-a', role: 'user', content: '**original**', internal: false } as Message;
const cleanups: Array<() => void> = [];
const apply = vi.fn();
const sourceCurrent = () => useWorkspaceStore.getState().workspace?.activeTabId === 'chat';
const draft = () => prepareChatEditorTransfer(message, { target: 'new_document', format: 'markdown', content: message.content, title: 'From chat' }, sourceCurrent)!;
function activate() {
  const ws = useWorkspaceStore.getState().workspace!;
  useWorkspaceStore.setState({ workspace: { ...ws, activeTabId: 'editor', tabs: ws.tabs.some(tab => tab.id === 'editor') ? ws.tabs : [...ws.tabs, { id: 'editor', type: 'editor', position: 1, title: 'Editor' }] } });
  if (!useEditorStore.getState().documents.editor) useEditorStore.getState().createDocument({ id: 'editor', markdown: '' });
}
function mountEditor() {
  cleanups.push(registerEditorTransferSurface({ documentId: 'editor', capture: () => {
    let used = false;
    return { documentId: 'editor', isCurrent: () => !used, apply: value => { used = true; apply(value); }, dispose: () => { used = true; } };
  } }));
}
function PresentationScope() {
  const id = useModalId();
  useEffect(() => id ? registerChatPresentationModalScope(id, ['chat.pinned.open', 'chat.tokens.open']) : undefined, [id]);
  return createElement('button', null, 'Message');
}
function OriginModal({ scoped }: { scoped: boolean }) {
  const isOpen = useWorkspaceChatModalStore(state => state.isOpen);
  return createElement(Modal, { isOpen, title: 'Chat', onClose: useWorkspaceChatModalStore.getState().close,
    children: scoped ? createElement(PresentationScope) : createElement('button', null, 'Message') });
}
function mountOriginModal(scoped: boolean) {
  useWorkspaceChatModalStore.setState({ isOpen: true, boundTabId: 'chat', boundConversationId: message.conversationId });
  const mounted = render(createElement(OriginModal, { scoped }));
  cleanups.push(mounted.unmount);
  const snapshot = getModalRegistrySnapshot();
  expect(snapshot.ids).toHaveLength(1);
  expect(!!snapshot.chatPresentationCommandIds).toBe(scoped);
  return snapshot;
}
function mountInactiveRealEditor() {
  const ws = useWorkspaceStore.getState().workspace!;
  useWorkspaceStore.setState({ workspace: { ...ws, tabs: [...ws.tabs, { id: 'editor', type: 'editor', title: 'Existing', position: 1 }] } });
  useEditorStore.getState().createDocument({ id: 'editor', markdown: 'original destination', mode: 'rich' });
  const doc = useEditorStore.getState().documents.editor;
  const root = document.createElement('div'); document.body.append(root);
  const editor = new Editor({ element: root, extensions: buildRichTextExtensions({ placeholder: '', imageFallbackLabel: 'image', imageLabelPrefix: 'image' }),
    content: { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'original destination' }] }] } });
  editor.commands.setTextSelection({ from: 1, to: 9 });
  const rootRef = { current: root }; const richEditorRef = { current: editor };
  const changed = vi.fn();
  const hook = renderHook(() => {
    const active = useWorkspaceStore(state => state.workspace?.activeTabId === 'editor');
    useEditorInsert({ commandSurface: { root: rootRef, active, asking: false }, activeTab: doc, currentDocumentId: 'editor', sessionLoaded: true, editorReadyNonce: 1,
      editorRef: { current: null }, monacoRef: { current: null }, richEditorRef, flushActiveRichMarkdownNow: changed });
    return active;
  });
  cleanups.push(() => { hook.unmount(); editor.destroy(); root.remove(); });
  expect(hook.result.current).toBe(false);
  return { editor, hook, changed };
}
beforeEach(() => {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'owner', sessionId: 'session', role: 'admin' } });
  useWorkspaceStore.setState({ workspace: { id: 'workspace', name: 'Workspace', activeTabId: 'chat', tabs: [{ id: 'chat', type: 'chat', title: 'Chat', position: 0, conversationId: message.conversationId }] } as WorkspaceData });
  useEditorStore.setState({ ownerUserId: 'owner', documents: {} });
  useChatStore.setState({ getConversationMessages: () => [message] });
  useWorkspaceChatModalStore.setState({ isOpen: false, boundTabId: null, boundConversationId: null });
  transport.prepare.mockReset().mockResolvedValue({ tabId: 'editor', draftId: 'draft' });
  transport.open.mockReset().mockImplementation(async () => { activate(); return { tabId: 'editor', draftId: 'draft' }; });
  transport.validate.mockReset().mockResolvedValue(undefined);
  apply.mockReset();
});
afterEach(() => { cleanups.splice(0).forEach(cleanup => cleanup()); vi.restoreAllMocks(); });

describe('chat → editor: transição explícita e confirmação da aplicação', () => {
  it.each([false, true])('fecha modal real após Open, scope=%s, preservando destino capturado inativo', async scoped => {
    const destination = mountInactiveRealEditor();
    const initial = mountOriginModal(scoped);
    const target = prepareChatEditorTransfer(message, { target: 'document', targetDocumentId: 'editor', content: 'replacement', format: 'plain' }, sourceCurrent)!;
    expect(target).toBeDefined(); cleanups.push(target.dispose);
    await target.prepareAdmission!('ticket');
    transport.open.mockImplementationOnce(async () => {
      flushSync(activate);
      expect(destination.hook.result.current).toBe(true);
      expect(useWorkspaceChatModalStore.getState().isOpen).toBe(true);
      expect(getModalRegistrySnapshot().generationNumber).toBe(initial.generationNumber);
      expect(destination.editor.getText()).toBe('original destination');
      return { tabId: 'editor' };
    });
    transport.validate.mockImplementationOnce(async () => {
      expect(useWorkspaceChatModalStore.getState().isOpen).toBe(false);
      const closed = getModalRegistrySnapshot();
      expect(closed.ids).toEqual([]);
      expect(closed.generationNumber).toBe(initial.generationNumber + (scoped ? 2 : 1));
      expect(destination.editor.getText()).toBe('original destination');
    });
    await act(async () => { await target.execute({ ticket: 'ticket', handoffId: 'handoff' }); });
    expect(destination.editor.getText()).toBe('replacement destination');
    expect(destination.changed).toHaveBeenCalledTimes(1);
    expect(transport.validate).toHaveBeenCalledOnce();
  });

  it('modal adicional aberto e fechado durante Open invalida sem fechar origem ou aplicar', async () => {
    const destination = mountInactiveRealEditor(); mountOriginModal(true);
    const target = prepareChatEditorTransfer(message, { target: 'document', targetDocumentId: 'editor', content: 'replacement', format: 'plain' }, sourceCurrent)!;
    cleanups.push(target.dispose); await target.prepareAdmission!('ticket');
    transport.open.mockImplementationOnce(async () => {
      const overlay = document.createElement('div'); overlay.className = 'modal-overlay'; overlay.dataset.modalId = 'temporary'; document.body.append(overlay);
      registerOpenModal('temporary'); unregisterOpenModal('temporary'); overlay.remove();
      flushSync(activate); return { tabId: 'editor' };
    });
    await act(async () => { await expect(target.execute({ ticket: 'ticket', handoffId: 'handoff' })).rejects.toThrow(); });
    expect(useWorkspaceChatModalStore.getState().isOpen).toBe(true);
    expect(destination.editor.getText()).toBe('original destination');
    expect(transport.validate).not.toHaveBeenCalled();
  });
  it('preparar não cria aba; espera montagem e confirma a aplicação uma vez', async () => {
    const target = draft(); cleanups.push(target.dispose);
    await target.prepareAdmission!('ticket');
    expect(transport.prepare).toHaveBeenCalledExactlyOnceWith('ticket', message.id, message.content, '');
    expect(useWorkspaceStore.getState().workspace?.tabs).toHaveLength(1);
    const executed = target.execute({ ticket: 'ticket', handoffId: 'handoff' });
    await vi.waitFor(() => expect(transport.open).toHaveBeenCalledOnce());
    expect(apply).not.toHaveBeenCalled();
    mountEditor(); await executed;
    expect(transport.validate).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff');
    expect(apply).toHaveBeenCalledExactlyOnceWith({ content: message.content, format: 'markdown', title: 'From chat' });
    expect(useEditorStore.getState()).not.toHaveProperty('requestInsert');
    expect(useEditorStore.getState()).not.toHaveProperty('pendingInsert');
  });

  it('não substitui o documento existente nem recaptura conteúdo alterado', async () => {
    const ws = useWorkspaceStore.getState().workspace!;
    useWorkspaceStore.setState({ workspace: { ...ws, tabs: [...ws.tabs, { id: 'editor', type: 'editor', title: 'Existing', position: 1 }] } });
    useEditorStore.getState().createDocument({ id: 'editor', markdown: 'existing' });
    const target = prepareChatEditorTransfer(message, { target: 'document', targetDocumentId: 'editor', content: 'snippet', format: 'plain' }, sourceCurrent)!;
    cleanups.push(target.dispose);
    useEditorStore.getState().setDocMarkdown('editor', 'changed concurrently');
    await expect(target.prepareAdmission!('ticket')).rejects.toThrow();
    expect(transport.prepare).not.toHaveBeenCalled(); expect(apply).not.toHaveBeenCalled();
    expect(useEditorStore.getState().documents.editor.markdown).toBe('changed concurrently');
  });

  it('permite o descarte da superfície fonte durante a transição autorizada', async () => {
    const target = draft(); cleanups.push(target.dispose);
    await target.prepareAdmission!('ticket');
    transport.open.mockImplementationOnce(async () => { activate(); target.dispose(); return { tabId: 'editor' }; });
    mountEditor(); await target.execute({ ticket: 'ticket', handoffId: 'handoff' });
    expect(apply).toHaveBeenCalledOnce();
  });

  it.each(['owner', 'tab', 'workspace', 'message'] as const)('%s ABA durante Open invalida permanentemente a operação', async kind => {
    const target = draft(); cleanups.push(target.dispose); await target.prepareAdmission!('ticket');
    transport.open.mockImplementationOnce(async () => {
      const ws = useWorkspaceStore.getState().workspace!; const user = useAuthStore.getState().user;
      if (kind === 'owner') { useAuthStore.setState({ user: { ...user!, sessionId: 'other' } }); useAuthStore.setState({ user }); }
      if (kind === 'tab') { useWorkspaceStore.setState({ workspace: { ...ws, activeTabId: 'other' } }); useWorkspaceStore.setState({ workspace: ws }); }
      if (kind === 'workspace') { useWorkspaceStore.setState({ workspace: { ...ws, id: 'other' } }); useWorkspaceStore.setState({ workspace: ws }); }
      if (kind === 'message') { useChatStore.setState({ getConversationMessages: () => [Object.assign(Object.create(Object.getPrototypeOf(message)), message, { content: 'changed' }) as Message] }); useChatStore.setState({ getConversationMessages: () => [message] }); }
      activate(); return { tabId: 'editor' };
    });
    mountEditor(); await expect(target.execute({ ticket: 'ticket', handoffId: 'handoff' })).rejects.toThrow();
    expect(apply).not.toHaveBeenCalled(); expect(transport.open).toHaveBeenCalledOnce();
  });

  it('resposta Open divergente ou perdida não aplica nem tenta abrir novamente', async () => {
    const target = draft(); cleanups.push(target.dispose); await target.prepareAdmission!('ticket');
    transport.open.mockImplementationOnce(async () => { activate(); throw new Error('lost'); });
    mountEditor(); await expect(target.execute({ ticket: 'ticket', handoffId: 'handoff' })).rejects.toThrow('lost');
    expect(apply).not.toHaveBeenCalled(); expect(transport.open).toHaveBeenCalledOnce();
  });

  it('revalidação backend negada antes da aplicação não modifica o destino', async () => {
    const target = draft(); cleanups.push(target.dispose); await target.prepareAdmission!('ticket');
    transport.validate.mockRejectedValue(new Error('epoch changed'));
    mountEditor(); await expect(target.execute({ ticket: 'ticket', handoffId: 'handoff' })).rejects.toThrow();
    expect(apply).not.toHaveBeenCalled();
  });

  it('não captura documento read-only nem modal de outra operação', () => {
    activate(); useWorkspaceStore.setState({ workspace: { ...useWorkspaceStore.getState().workspace!, activeTabId: 'chat' } });
    useEditorStore.setState({ documents: { editor: { ...useEditorStore.getState().documents.editor, readOnly: true } } });
    expect(prepareChatEditorTransfer(message, { target: 'document', targetDocumentId: 'editor', content: 'x', format: 'plain' }, sourceCurrent)).toBeUndefined();
    const overlay = document.createElement('div'); overlay.className = 'modal-overlay'; overlay.dataset.modalId = 'other'; document.body.appendChild(overlay);
    registerOpenModal('other'); cleanups.push(() => { unregisterOpenModal('other'); overlay.remove(); });
    expect(draft()).toBeUndefined();
  });
});
