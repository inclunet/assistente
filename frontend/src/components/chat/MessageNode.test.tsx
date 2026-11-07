import { withFixtureProjection } from '../../test/chatWindowFixture';
import type { MessageNode as ChatMessageNode } from '../../lib/chatMessageTree';
import { messageFixture, nodeFixture } from '../../test/chatPresentationFixture';
import { fixtureMessages } from './canonicalPresentationFixture';
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { act, createEvent, fireEvent, render as renderUI, screen, cleanup, waitFor } from '@testing-library/react';
import { MessageNode } from './MessageNode';
import { MessageList } from './MessageList';
import React, { type ReactElement, type ReactNode } from 'react';
import { useAuthStore } from '../../store/authStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useChatStore } from '../../store/chatStore';
import { ChatSessionProvider } from './ChatSessionContext';
import { WorkspacePanelProvider } from '../workspace/WorkspacePanelContext';
import { createChatSurfaceIdentity, createEmptyChatSurfaceSession } from '../../services/chatSessionRegistry';
import { CHAT_NAVIGATION_COMMAND_EVENT, captureChatNavigationTarget, type ChatNavigationRequest } from '../../lib/commandChatNavigation';

const chatMessageSpy = vi.fn();

vi.mock('./ChatMessage', () => ({
  ChatMessage: (props: { hasThreadIndicator?: boolean; isReading?: boolean }) => {
    chatMessageSpy(props);
    return (
      <div data-testid="chat-message">
        <button type="button" data-testid="inner-control">Controle interno</button>
      </div>
    );
  },
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  announce: vi.fn(),
  useAnnouncer: () => ({ announceRequest: () => true }),
}));

vi.mock('../../services/messageAudio', () => ({
  messageAudioService: { stopCurrentAudio: vi.fn(), isCurrentlyPlaying: () => false },
}));

vi.mock('../../services/tts', () => ({ ttsService: { isSpeaking: () => false, stop: vi.fn() } }));

vi.mock('@wailsjs/go/app/App', () => ({
  UpdateMessage: vi.fn(),
}));

vi.mock('../../utils/errorHandler', () => ({
  handleError: vi.fn(),
  ErrorSeverity: { RECOVERABLE: 'recoverable' },
}));


const conversationId = '01926b90-7a5a-7c4e-8d3f-000000000001';
const tab = { id: 'tab', type: 'chat' as const, conversationId, title: 'Chat', position: 0 };
const surface = createChatSurfaceIdentity({ conversationId, tabId: tab.id, surfaceType: 'page' });
function dispatchNavigation(event: Event) {
  const { commandID, instanceId } = (event as CustomEvent<ChatNavigationRequest>).detail;
  const lease = captureChatNavigationTarget(() => '/chat', commandID, instanceId);
  try { if (lease?.canOpen(commandID)) { event.preventDefault(); lease.open(commandID); } }
  finally { lease?.dispose(); }
}
function render(ui: ReactElement) {
  const nodes: ChatMessageNode[] = [];
  const prepare = (child: ReactNode): ReactNode => {
    if (!React.isValidElement(child)) return child;
    const element = child as ReactElement<{ node?: ChatMessageNode; commandPathname?: string; children?: ReactNode }>;
    if (element.type === MessageNode && element.props.node) {
      nodes.push(element.props.node);
      return React.cloneElement(element, { commandPathname: '/chat' });
    }
    return React.cloneElement(element, {}, React.Children.map(element.props.children, prepare));
  };
  const content = prepare(ui);
  const conversation = withFixtureProjection({ id: conversationId, title: 'Chat' }, { nodes: nodes });
  useChatStore.setState({
    conversationInfoById: { [conversationId]: conversation },
    surfaceSessionsByKey: {
      ['conversation:' + conversationId]: { ...createEmptyChatSurfaceSession(conversationId) },
    },
  });
  return renderUI(<WorkspacePanelProvider value={{ tab, isActive: true }}><ChatSessionProvider surface={surface}>{content}</ChatSessionProvider></WorkspacePanelProvider>);
}
vi.mock('./useChatTimelinePresentation', () => ({
  useChatTimelinePresentation: () => ({
    presentation: {
      getMessage: (id: string) => fixtureMessages(conversationId).find(message => message.id === id),
      subscribe: useChatStore.subscribe, threadWindow: () => undefined,
      get window() { return undefined; },
    },
    nodes: fixtureMessages(conversationId).map(message => ({ message, children: [],  childCount: 0 })),
    revision: 0,
  }),
}));

beforeEach(() => {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'owner', sessionId: 'session', role: 'admin' } });
  useWorkspaceStore.setState({ workspace: { id: 'ws', name: 'Workspace', activeTabId: 'tab', tabs: [tab] } });
  window.addEventListener(CHAT_NAVIGATION_COMMAND_EVENT, dispatchNavigation);
});
afterEach(() => { cleanup(); window.removeEventListener(CHAT_NAVIGATION_COMMAND_EVENT, dispatchNavigation); vi.restoreAllMocks(); });

describe('MessageNode', () => {
  it('navega por mensagens reais e preserva leitura ao alternar o modo da lista', async () => {
    vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockImplementation(function (this: HTMLElement) {
      return this.classList.contains('message-list') ? 700 : this.hasAttribute('data-index') ? 140 : 0;
    });
    const nodes = Array.from({ length: 41 }, (_, index) => nodeFixture({
      message: { id: `row-${index}`, conversationId, role: 'assistant', turnId: `turn-${index}`,
        content: `mensagem ${index}`, timestamp: index, isStreaming: false, internal: false },
      children: [], childCount: 0,
    }));
    const conversation = withFixtureProjection({ id: conversationId, title: 'Chat' }, { nodes: nodes });
    useChatStore.setState({
      conversationInfoById: { [conversationId]: conversation },
      surfaceSessionsByKey: {
        ['conversation:' + conversationId]: { ...createEmptyChatSurfaceSession(conversationId) },
      },
    });
    const content = (count: number) => <WorkspacePanelProvider value={{ tab, isActive: true }}>
      <ChatSessionProvider surface={surface}><MessageList threadedMessages={nodes.slice(0, count)} commandPathname="/chat" /></ChatSessionProvider>
    </WorkspacePanelProvider>;
    const view = renderUI(content(3));
    const row = (index: number) => view.container.querySelector(`[data-message-id="row-${index}"]`) as HTMLElement;
    act(() => row(0).focus());
    fireEvent.keyDown(row(0), { key: 'ArrowDown' });
    expect(row(1)).toHaveFocus();
    fireEvent.keyDown(row(1), { key: 'ArrowDown' });
    expect(row(2)).toHaveFocus();
    fireEvent.keyDown(row(2), { key: 'ArrowUp' });
    expect(row(1)).toHaveFocus();
    const original = row(1);
    fireEvent.keyDown(original, { key: 'Enter' });
    expect(original).toHaveClass('message-node--reading');
    view.rerender(content(41));
    expect(row(1)).toBe(original);
    expect(original).toHaveClass('message-node--reading');
    view.rerender(content(40));
    expect(row(1)).toBe(original);
    expect(original).toHaveClass('message-node--reading');
    await waitFor(() => expect(row(39).closest('[inert]')).not.toBeNull());
    expect(original.closest('[inert]')).toBeNull();
    fireEvent.keyDown(original, { key: 'Escape' });
    fireEvent.keyDown(original, { key: 'ArrowDown' });
    expect(row(2)).toHaveFocus();
    fireEvent.keyDown(row(2), { key: 'ArrowUp' });
    expect(original).toHaveFocus();
  });

  const keyboardNode = () => nodeFixture({
    message: { id: '01926b90-7a5a-7c4e-8d3f-000000000002', conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
      role: 'user', content: 'Mensagem', internal: false, isStreaming: false },
    childCount: 0,  children: [],
  });

  it.each([' ', 'Delete', 'F2', 'c'])('ação %s ignora repeat, IME, 229 e evento já consumido', async key => {
    const onSpeak = vi.fn(); const onDelete = vi.fn(); const onEdit = vi.fn(); const onCopy = vi.fn();
    const node = keyboardNode();
    render(<MessageNode node={node} onSpeak={onSpeak} onDelete={onDelete} onEdit={onEdit} onCopy={onCopy} />);
    const item = screen.getByRole('listitem');
    const options = { key, ctrlKey: key === 'c' };
    for (const extra of [{ repeat: true }, { isComposing: true }, { keyCode: 229 }]) {
      fireEvent.keyDown(item, { ...options, ...extra });
    }
    const consumed = createEvent.keyDown(item, options); consumed.preventDefault(); fireEvent(item, consumed);
    for (const callback of [onSpeak, onDelete, onEdit, onCopy]) expect(callback).not.toHaveBeenCalled();
    await act(async () => { fireEvent.keyDown(item, options); });
    const callback = key === ' ' ? onSpeak : key === 'Delete' ? onDelete : key === 'F2' ? onEdit : onCopy;
    if (key === 'c') expect(callback).toHaveBeenCalledExactlyOnceWith(node.message, false);
    else expect(callback).toHaveBeenCalledExactlyOnceWith(node.message);
  });

  it('preserva repetição das setas de navegação', () => {
    const onFocusSiblingIndex = vi.fn();
    render(<MessageNode node={keyboardNode()} siblingIndex={0} siblingCount={2} onFocusSiblingIndex={onFocusSiblingIndex} />);
    const item = screen.getByRole('listitem');
    fireEvent.keyDown(item, { key: 'ArrowDown' });
    fireEvent.keyDown(item, { key: 'ArrowDown', repeat: true });
    expect(onFocusSiblingIndex).toHaveBeenCalledTimes(2);
    expect(onFocusSiblingIndex).toHaveBeenLastCalledWith(1);
  });

  it('não executa ações de mensagem sobre textarea ou controles em leitura', () => {
    const onSpeak = vi.fn(); const onDelete = vi.fn(); const onEdit = vi.fn(); const onCopy = vi.fn();
    render(<MessageNode node={keyboardNode()} onSpeak={onSpeak} onDelete={onDelete} onEdit={onEdit} onCopy={onCopy} />);
    const item = screen.getByRole('listitem');
    const textarea = document.createElement('textarea'); item.appendChild(textarea);
    for (const key of [' ', 'Delete', 'F2', 'c']) {
      expect(fireEvent.keyDown(textarea, { key, ctrlKey: key === 'c' })).toBe(true);
    }
    textarea.remove();
    fireEvent.keyDown(item, { key: 'Enter' });
    for (const key of [' ', 'Delete', 'F2', 'c']) {
      expect(fireEvent.keyDown(screen.getByTestId('inner-control'), { key, ctrlKey: key === 'c' })).toBe(true);
    }
    for (const callback of [onSpeak, onDelete, onEdit, onCopy]) expect(callback).not.toHaveBeenCalled();
  });

  it('renderiza container e passa indicador de thread', () => {
    render(
      <MessageNode
        node={nodeFixture({
          message: messageFixture({
            id: '1',
            conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
            role: 'user',
            content: 'Oi',
            createdAt: new Date().toISOString(),
            timestamp: Date.now(),
            isStreaming: false,
            internal: false,
          }),
          childCount: 1,

          children: [],
        })}
      />
    );

    expect(screen.getByRole('listitem')).toHaveAttribute('data-message-id', '1');
    expect(screen.getByTestId('chat-message')).toBeInTheDocument();
    expect(chatMessageSpy).toHaveBeenCalledWith(expect.objectContaining({ hasThreadIndicator: true }));
  });

  it('deixa controles internos processarem Enter durante a leitura isolada', () => {
    const onOuterKeyDown = vi.fn();
    render(
      <div onKeyDown={onOuterKeyDown}>
        <MessageNode
          node={nodeFixture({
            message: messageFixture({
              id: 'reading-message',
              conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
              role: 'assistant',
              content: 'Mensagem com controle',
              createdAt: new Date().toISOString(),
              timestamp: Date.now(),
              isStreaming: false,
              internal: false,
            }),
            childCount: 0,

            children: [],
          })}
        />
      </div>,
    );

    const item = screen.getByRole('listitem');
    expect(fireEvent.keyDown(item, { key: 'Enter' })).toBe(false);
    expect(chatMessageSpy).toHaveBeenLastCalledWith(expect.objectContaining({ isReading: true }));
    onOuterKeyDown.mockClear();

    // O Enter do botão não pode ser preventDefault pelo atalho ancestral.
    expect(fireEvent.keyDown(screen.getByTestId('inner-control'), { key: 'Enter' })).toBe(true);
    fireEvent.keyDown(screen.getByTestId('inner-control'), { key: 'ArrowDown' });
    expect(onOuterKeyDown).not.toHaveBeenCalled();
  });
});
