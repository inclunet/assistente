import { createRoot } from 'react-dom/client';
import { flushSync } from 'react-dom';
import { BrowserRouter } from 'react-router-dom';
import { MessageList } from '../../src/components/chat/MessageList';
import { ChatSessionProvider, useChatSession } from '../../src/components/chat/ChatSessionContext';
import { createChatSurfaceIdentity } from '../../src/services/chatSessionRegistry';
import { useAuthStore } from '../../src/store/authStore';
import { useChatStore } from '../../src/store/chatStore';
import { ChatTimelineClient } from '../../src/services/chatTimelineClient';
import { ChatTimelinePresentation } from '../../src/services/chatTimelinePresentation';
import { TimelineConversationState } from '../../src/services/timelineConversationState';
import type { TimelineSnapshot } from '../../src/services/timelineConversationState';
import type { TimelineItem } from '../../src/types/chatTimelineEvents';
import '../../src/lib/i18n';
import '../../src/theme.css';

// Harness isolado: componentes/estado reais, transporte determinístico sem rede,
// banco do usuário, geração, ACP ou modificações no bundle de produção.
const root = createRoot(document.getElementById('root')!);
let serial = 0;
let count = 100;
let textBytes = 0;
const windows = new Map<string, { conversationId: string; revision: number }>();
function snapshot(windowId: string, mode: string): TimelineSnapshot {
  const window = windows.get(windowId)!;
  const items: TimelineItem[] = Array.from({ length: Math.min(count, 240) }, (_, index) => {
    const id = `${window.conversationId}:${index}`;
    return {
      itemId: `message:${id}`, conversationId: window.conversationId, scopeParentId: null,
      kind: 'message', turnId: null, replyToItemId: '', messageIds: [id], representativeMessageId: id,
      revision: '1', state: 'completed', durability: 'persisted',
      segments: [{ segmentId: `text:${id}`, kind: 'text', text: `${id} ${textBytes ? 'x'.repeat(textBytes) : 'conteúdo da conversa '.repeat(200)}` }],
      metadata: { role: 'assistant', createdAt: '2026-01-01T00:00:00Z', source: '', childCount: 0,
        conclusionSegmentIds: [`text:${id}`], cacheReadTokens: 0, cacheWriteTokens: 0, cacheMissTokens: 0,
        origin: 'chat', pinned: false, model: '', promptTokens: 0, completionTokens: 0, totalTokens: 0 },
    };
  });
  return { conversationId: window.conversationId, scopeParentId: null, windowId,
    windowRevision: String(++window.revision), epoch: window.conversationId, sequence: '1', mode,
    items, orderedItemIds: items.map(item => item.itemId), retainedItemIds: [], retentionBlocked: false,
    totalCount: count, startIndex: count - items.length, endIndex: count - 1, hasBefore: count > items.length,
    hasAfter: false, beforeCursor: '', afterCursor: '' };
}
const rpc = {
  Open: async ({ conversationId }: { conversationId: string }) => {
    const id = `window:${++serial}`;
    windows.set(id, { conversationId, revision: 0 });
    return snapshot(id, 'historical');
  },
  Navigate: async (id: string, { mode }: { mode: string }) => snapshot(id, mode),
  Close: async (id: string) => { windows.delete(id); },
};
// Mantém o adaptador Wails, normalizador, eventos e lifecycle reais. Somente as
// três chamadas RPC usadas pela fixture recebem respostas locais.
Object.assign(window, { go: { wailsapi: { ChatTimeline: rpc } } });
function login() {
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'heap-user', sessionId: `session:${++serial}`, role: 'admin' } });
}
login();
function TimelineView() {
  const { threadedMessages } = useChatSession();
  return <div data-timeline-items={threadedMessages.length} style={{ display: 'contents' }}>
    <MessageList threadedMessages={threadedMessages} />
  </div>;
}
// Timer funciona também na WebView oculta, sem depender de requestAnimationFrame.
const settle = () => new Promise(resolve => setTimeout(resolve, 100));
async function waitFor(condition: () => boolean, message: string) {
  const deadline = Date.now() + 10_000;
  while (!condition()) {
    if (Date.now() > deadline) throw new Error(message);
    await settle();
  }
}
const api = {
  prototypes: {
    client: ChatTimelineClient.prototype,
    presentation: ChatTimelinePresentation.prototype,
    conversation: TimelineConversationState.prototype,
  },
  async cycle(messages: number, payloadBytes = 0) {
    count = messages;
    textBytes = payloadBytes;
    const surface = createChatSurfaceIdentity({ conversationId: `conversation:${++serial}`, surfaceId: `surface:${serial}` });
    flushSync(() => root.render(<BrowserRouter><div style={{ height: 700, display: 'flex' }}>
      <ChatSessionProvider surface={surface}><TimelineView /></ChatSessionProvider>
    </div></BrowserRouter>));
    await waitFor(() => !!document.querySelector(`[data-timeline-items="${Math.min(messages, 240)}"] [data-message-node]`), 'Lista não renderizou a janela completa');
    flushSync(() => root.render(null));
    await waitFor(() => windows.size === 0, 'Janela remota não encerrada');
    await settle();
    if (Object.keys(useChatStore.getState().surfaceSessionsByKey).length !== 0) throw new Error('Sessão visual não liberada');
  },
  async logout() {
    useAuthStore.setState({ isAuthenticated: false, user: null });
    await settle();
    login();
  },
};
Object.assign(window, { timelineHeap: api });
