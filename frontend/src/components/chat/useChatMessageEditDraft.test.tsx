import { useRef } from 'react';
import { act, cleanup, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { chat } from '../../../wailsjs/go/models';
import { useAuthStore } from '../../store/authStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useChatStore } from '../../store/chatStore';
import { createEmptyChatSession } from '../../services/chatSessionRegistry';
import { registerOpenModal, unregisterOpenModal } from '../../lib/modalRegistry';
import { captureChatMessageEditDraft, useChatMessageEditDraft } from './useChatMessageEditDraft';

vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: () => () => {} }));

const conversationId = '01926b90-7a5a-7c4e-8d3f-000000000001';
const messageId = '01926b90-7a5a-7c4e-8d3f-000000000011';
const sessionKey = `page:tab:tab:${conversationId}`;
let message: chat.EnrichedMessage;
let api: ReturnType<typeof useChatMessageEditDraft>;
const frames: FrameRequestCallback[] = [];
const modalClosers: Array<() => void> = [];
const captures: Array<NonNullable<ReturnType<typeof captureChatMessageEditDraft>>> = [];

function Harness({ pathname = '/' }: { pathname?: string }) {
  const root = useRef<HTMLDivElement>(null);
  api = useChatMessageEditDraft(root, message, conversationId, sessionKey, pathname);
  return <div ref={root} tabIndex={-1} data-testid="edit-root">
    <output>{api.isEditing ? 'editing' : 'closed'}</output>
    <textarea aria-label="Draft" value={api.editContent} onChange={event => api.change(event.target.value)} />
  </div>;
}
function replaceStoredMessage(next: chat.EnrichedMessage) {
  const conversation = { id: conversationId, title: 'Conversa', threadedMessages: [new chat.MessageNode({ message: next, children: [], level: 0, childCount: 0 })] };
  // MessageNode's constructor materializes DTOs; retain its exact stored identity.
  const stored = conversation.threadedMessages[0].message;
  useChatStore.setState({ timelinesByConversationId: { [conversationId]: conversation },
    sessionsByConversationId: { [conversationId]: { ...createEmptyChatSession(conversationId), conversation } } });
  return stored;
}
function capture() {
  const target = captureChatMessageEditDraft(screen.getByTestId('edit-root'), conversationId, sessionKey, messageId);
  if (target) captures.push(target);
  return target;
}
function open() {
  const view = render(<Harness />);
  act(() => api.open());
  act(() => api.change('Rascunho novo'));
  expect(capture()).toBeDefined();
  return view;
}
function flushFrames() { act(() => { frames.splice(0).forEach(callback => callback(0)); }); }
function openModal() {
  const overlay = document.createElement('div'); overlay.className = 'modal-overlay'; document.body.appendChild(overlay);
  registerOpenModal('draft-test-overlay');
  const close = () => { unregisterOpenModal('draft-test-overlay'); overlay.remove(); };
  modalClosers.push(close);
  return close;
}

beforeEach(() => {
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'owner', sessionId: 'session', role: 'admin' } });
  useWorkspaceStore.setState({ workspace: { id: 'workspace', name: 'Workspace', activeTabId: 'tab',
    tabs: [{ id: 'tab', type: 'chat', title: 'Chat', position: 0, conversationId }] } });
  message = replaceStoredMessage(new chat.EnrichedMessage({ id: messageId, conversationId, role: 'user', content: 'Original' }));
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => { frames.push(callback); return frames.length; });
});
afterEach(() => {
  cleanup(); captures.splice(0).forEach(target => target.dispose());
  modalClosers.splice(0).forEach(close => close()); frames.length = 0; vi.restoreAllMocks();
});

describe('useChatMessageEditDraft with real stores', () => {
  it('captures synchronous content before rerender and retains the original from opening', () => {
    open();
    act(() => {
      api.change('Ainda no mesmo evento');
      expect(capture()).toMatchObject({ content: 'Ainda no mesmo evento', originalContent: 'Original', originalMessage: message });
    });
  });

  it('draft ABA permanently invalidates the old capture but allows a new save capture', () => {
    open(); const old = capture()!;
    act(() => { api.change('Outro'); api.change('Rascunho novo'); });
    expect(old.isCurrent()).toBe(false); expect(old.accept(() => true)).toBe(false);
    expect(capture()?.isCurrent()).toBe(true); expect(api.isEditing).toBe(true);
  });

  it('cancel and reopen do not let an old result close the new editor', () => {
    open(); const old = capture()!;
    act(() => { api.cancel(); api.open(); api.change('Rascunho novo'); });
    expect(old.accept(() => true)).toBe(false); expect(old.isCurrent()).toBe(false);
    expect(capture()?.isCurrent()).toBe(true); expect(api.isEditing).toBe(true); expect(frames).toHaveLength(0);
  });

  it('confirmed own event may update message identity before result acceptance', () => {
    open(); const target = capture()!;
    act(() => { replaceStoredMessage(new chat.EnrichedMessage({ ...message, content: target.content })); });
    expect(target.isCurrent()).toBe(false);
    act(() => { expect(target.accept(() => true)).toBe(true); });
    expect(api.isEditing).toBe(false);
    flushFrames(); expect(screen.getByTestId('edit-root')).toHaveFocus();
  });

  it('own event plus a newer draft never closes or focuses the newer edit', () => {
    open(); const target = capture()!;
    act(() => { replaceStoredMessage(new chat.EnrichedMessage({ ...message, content: target.content })); api.change('Não salvo ainda'); });
    expect(target.accept(() => true)).toBe(false); expect(api.editContent).toBe('Não salvo ainda');
    expect(api.isEditing).toBe(true); expect(frames).toHaveLength(0);
  });

  it('external message update and content ABA cannot replace the original CAS base', () => {
    open();
    act(() => { replaceStoredMessage(new chat.EnrichedMessage({ ...message, content: 'Concorrente' })); });
    act(() => { replaceStoredMessage(new chat.EnrichedMessage({ ...message, content: 'Original' })); });
    expect(capture()).toBeUndefined(); expect(api.isEditing).toBe(true); expect(api.editContent).toBe('Rascunho novo');
  });

  it('confirmed own save rebases only the original, keeping a newer draft available for a second save', () => {
    open(); const saved = capture()!;
    act(() => {
      replaceStoredMessage(new chat.EnrichedMessage({ ...message, content: saved.content }));
      api.change('Segunda alteração');
    });
    expect(saved.accept(() => true)).toBe(false);
    act(() => saved.rebaseConfirmed(() => true));
    expect(capture()).toMatchObject({ originalContent: 'Rascunho novo', content: 'Segunda alteração' });
    expect(capture()?.isCurrent()).toBe(true); expect(api.isEditing).toBe(true); expect(frames).toHaveLength(0);
  });

  it('confirmation cannot rebase a different persisted value', () => {
    open(); const saved = capture()!;
    act(() => { replaceStoredMessage(new chat.EnrichedMessage({ ...message, content: 'Escrita concorrente' })); api.change('Não perder'); });
    act(() => saved.rebaseConfirmed(() => true));
    expect(capture()).toBeUndefined(); expect(api.editContent).toBe('Não perder'); expect(api.isEditing).toBe(true);
  });

  it('old confirmation cannot rebase a cancelled and reopened edit session', () => {
    open(); const saved = capture()!;
    act(() => { api.cancel(); api.open(); api.change('Outra sessão de edição'); });
    act(() => { replaceStoredMessage(new chat.EnrichedMessage({ ...message, content: saved.content })); });
    act(() => saved.rebaseConfirmed(() => true));
    expect(capture()).toBeUndefined(); expect(api.editContent).toBe('Outra sessão de edição'); expect(api.isEditing).toBe(true);
  });

  it.each(['owner', 'session', 'workspace', 'tab', 'conversation', 'authenticated'] as const)(
    '%s ABA after own message event cannot accept the old result', field => {
      open(); const target = capture()!;
      act(() => { replaceStoredMessage(new chat.EnrichedMessage({ ...message, content: target.content })); });
      const auth = useAuthStore.getState(); const workspace = useWorkspaceStore.getState().workspace!;
      act(() => {
        if (field === 'owner') useAuthStore.setState({ user: { ...auth.user!, userId: 'other' } });
        if (field === 'session') useAuthStore.setState({ user: { ...auth.user!, sessionId: 'other' } });
        if (field === 'authenticated') useAuthStore.setState({ isAuthenticated: false });
        if (field === 'workspace') useWorkspaceStore.setState({ workspace: { ...workspace, id: 'other' } });
        if (field === 'tab') useWorkspaceStore.setState({ workspace: { ...workspace, activeTabId: 'other' } });
        if (field === 'conversation') useWorkspaceStore.setState({ workspace: { ...workspace, tabs: workspace.tabs.map(tab => ({ ...tab, conversationId: 'other' })) } });
      });
      act(() => { useAuthStore.setState({ user: auth.user, isAuthenticated: true }); useWorkspaceStore.setState({ workspace }); });
      expect(target.accept(() => true)).toBe(false); expect(api.isEditing).toBe(true); expect(frames).toHaveLength(0);
    },
  );

  it('route ABA after own event cannot accept an old result', () => {
    const view = open(); const target = capture()!;
    act(() => { replaceStoredMessage(new chat.EnrichedMessage({ ...message, content: target.content })); });
    view.rerender(<Harness pathname="/settings" />); view.rerender(<Harness pathname="/" />);
    expect(target.accept(() => true)).toBe(false); expect(api.isEditing).toBe(true);
  });

  it('overlay opened and closed during a pending save invalidates that capture', () => {
    open(); const target = capture()!;
    const close = openModal(); close();
    expect(target.isCurrent()).toBe(false); expect(target.accept(() => true)).toBe(false);
    expect(api.isEditing).toBe(true); expect(api.editContent).toBe('Rascunho novo');
    expect(capture()?.isCurrent()).toBe(true);
  });

  it.each(['modal', 'tab'] as const)('%s roundtrip without a pending save preserves draft and permits a fresh capture', kind => {
    render(<Harness />); act(() => { api.open(); api.change('Não perder rascunho'); });
    if (kind === 'modal') { const close = openModal(); close(); }
    else {
      const workspace = useWorkspaceStore.getState().workspace!;
      act(() => useWorkspaceStore.setState({ workspace: { ...workspace, activeTabId: 'other' } }));
      act(() => useWorkspaceStore.setState({ workspace }));
    }
    expect(api.isEditing).toBe(true); expect(api.editContent).toBe('Não perder rascunho');
    expect(capture()).toMatchObject({ originalContent: 'Original', content: 'Não perder rascunho' });
    expect(capture()?.isCurrent()).toBe(true);
  });

  it('tab ABA invalidates the pending lease, but a new save may use the preserved draft', () => {
    open(); const old = capture()!; const workspace = useWorkspaceStore.getState().workspace!;
    act(() => useWorkspaceStore.setState({ workspace: { ...workspace, activeTabId: 'other' } }));
    act(() => useWorkspaceStore.setState({ workspace }));
    expect(old.isCurrent()).toBe(false); expect(old.accept(() => true)).toBe(false);
    expect(capture()?.isCurrent()).toBe(true); expect(api.editContent).toBe('Rascunho novo');
  });

  it('unmount invalidates capture and prevents result focus', () => {
    const view = open(); const target = capture()!; view.unmount();
    expect(target.isCurrent()).toBe(false); expect(target.accept(() => true)).toBe(false); expect(frames).toHaveLength(0);
  });

  it('caller guard is checked again at deferred focus after successful acceptance', () => {
    open(); const target = capture()!; let current = true;
    act(() => { expect(target.accept(() => current)).toBe(true); });
    current = false; flushFrames(); expect(screen.getByTestId('edit-root')).not.toHaveFocus();
  });

  it('reopening between acceptance and animation frame cannot steal focus', () => {
    open(); const target = capture()!;
    act(() => { expect(target.accept(() => true)).toBe(true); api.open(); api.change('Nova edição'); });
    screen.getByRole('textbox').focus(); flushFrames();
    expect(screen.getByRole('textbox')).toHaveFocus(); expect(api.isEditing).toBe(true);
  });
});
