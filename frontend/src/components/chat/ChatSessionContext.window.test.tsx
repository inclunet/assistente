import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { canonicalTestFixture, canonicalTestItem } from '../../services/chatTimelineTestFixture';
import { createChatSurfaceIdentity } from '../../services/chatSessionRegistry';
import { useChatStore } from '../../store/chatStore';
import { useAuthStore } from '../../store/authStore';
import { ChatSessionProvider, useChatSession, type ChatSessionContextValue } from './ChatSessionContext';
import { MessageList } from './MessageList';

let fixture: ReturnType<typeof canonicalTestFixture>;
vi.mock('../../services/chatTimelineWails', () => ({ getChatTimelineClient: () => fixture.client }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announceRequest: vi.fn() }) }));
vi.mock('./MessageNode', () => ({ MessageNode: ({ ariaPosition, ariaSetSize }: {
  ariaPosition: number; ariaSetSize: number;
}) => <div role="listitem" aria-posinset={ariaPosition} aria-setsize={ariaSetSize} /> }));

const observed = new Map<string, ChatSessionContextValue>();
function Probe({ name }: { name: string }) {
  const state = useChatSession();
  observed.set(name, state);
  return <section aria-label={name}>
    <MessageList threadedMessages={state.threadedMessages} messageWindow={state.messageWindow}
      isLoadingMessageWindow={state.isLoadingMessageWindow} onLoadOlder={state.loadOlderMessages} />
  </section>;
}

beforeEach(() => {
  observed.clear();
  fixture = canonicalTestFixture([canonicalTestItem('reply')]);
  useChatStore.setState({ sessionsByConversationId: {}, timelinesByConversationId: {}, surfaceSessionsByKey: {} });
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'user', sessionId: 'session', role: 'admin' } });
  const open = fixture.port.open;
  fixture.port.open = vi.fn(async (...args: Parameters<typeof open>) => {
    const initial = await open(...args);
    const window = { ...initial, totalCount: 100, startIndex: 40, endIndex: 40, hasBefore: true, hasAfter: true };
    fixture.windows.set(window.windowId, window);
    return window;
  });
});
afterEach(async () => {
  await act(async () => cleanup());
  await fixture.client.dispose();
});

it('usa a janela do backend para posição, fronteiras e carregamento isolado por superfície', async () => {
  render(<>{['first', 'second'].map(name => <ChatSessionProvider key={name}
    surface={createChatSurfaceIdentity({ surfaceId: name, conversationId: 'conversation' })}>
    <Probe name={name} />
  </ChatSessionProvider>)}</>);
  await waitFor(() => {
    expect(screen.getAllByRole('listitem')).toHaveLength(2);
    expect([...observed.values()].every(value => !value.isLoadingMessageWindow)).toBe(true);
  });
  for (const item of screen.getAllByRole('listitem')) {
    expect(item).toHaveAttribute('aria-posinset', '41');
    expect(item).toHaveAttribute('aria-setsize', '100');
  }
  const first = observed.get('first')!;
  const second = observed.get('second')!;
  expect(first.messageWindow).toBe(first.presentation!.window!.metadata);
  const priorSecond = second.messageWindow;
  let finish!: () => void;
  vi.mocked(fixture.port.navigate).mockImplementationOnce(id => new Promise(resolve => {
    finish = () => resolve({ ...fixture.windows.get(id)!, windowRevision: '2',
      startIndex: 0, endIndex: 0, hasBefore: false });
  }));
  let navigation!: Promise<void>;
  act(() => { navigation = first.loadOlderMessages(); });
  await waitFor(() => expect(finish).toBeTypeOf('function'));
  expect(observed.get('first')!.isLoadingMessageWindow).toBe(true);
  expect(observed.get('second')!.isLoadingMessageWindow).toBe(false);
  expect(screen.getByRole('button', { name: 'chat.loadingOlderMessages' })).toBeDisabled();
  await act(async () => { finish(); await navigation; });
  expect(observed.get('first')!.isLoadingMessageWindow).toBe(false);
  expect(screen.getAllByRole('button', { name: 'chat.loadOlderMessages' })).toHaveLength(1);
  expect(screen.getAllByRole('listitem')[0]).toHaveAttribute('aria-posinset', '1');
  expect(observed.get('second')!.messageWindow).toBe(priorSecond);

  vi.mocked(fixture.port.navigate).mockRejectedValueOnce(new Error('offline'));
  const currentWindow = observed.get('first')!.messageWindow;
  await act(async () => {
    await expect(observed.get('first')!.loadNewerMessages()).rejects.toThrow('offline');
  });
  expect(observed.get('first')!.isLoadingMessageWindow).toBe(false);
  expect(observed.get('first')!.messageWindow).toBe(currentWindow);
  for (const session of Object.values(useChatStore.getState().surfaceSessionsByKey)) {
    expect(session).not.toHaveProperty('messageWindow');
    expect(session).not.toHaveProperty('visibleThreadedMessages');
    expect(session).not.toHaveProperty('isLoadingMessageWindow');
    expect(session).not.toHaveProperty('hasOlderMessages');
  }
});
