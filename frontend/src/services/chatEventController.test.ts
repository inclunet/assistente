import { messageFixture, nodeFixture } from '../test/chatPresentationFixture';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  startChatEventController,
  stopAllChatEventControllers,
  type ChatEventControllerAdapter,
  type ChatEventSession,
} from './chatEventController';
import { CHAT_TURN_EVENT_NAMES, resetChatEventHubForTests } from './chatEventHub';
import {
  type Message,
  type MessageNode,
} from '../lib/chatMessageTree';
import type { ConversationInfo, ChatSurfaceOrigin } from './chatSessionRegistry';
import { canonicalTestFixture, canonicalTestItem } from './chatTimelineTestFixture';
// Sentinela exclusiva do teste: o controller operacional não pode escrever histórico.
type ActiveConversation = ConversationInfo & { threadedMessages: MessageNode[] };
import type { TimelineItem, TimelineSegment } from '../types/chatTimelineEvents';

async function canonicalView(conversationId = 'conversation-1') {
  const item = canonicalTestItem('response', '', { conversationId, state: 'streaming', segments: [] });
  const fixture = canonicalTestFixture([item]);
  const presentation = fixture.presentation('visual', conversationId);
  await presentation.open();
  return { presentation, fixture, publish(segments: TimelineSegment[], overrides: Partial<TimelineItem> = {}) {
    fixture.publish('visual', { ...item, ...overrides, segments });
  }, tools() {
    return presentation.getMessage('response')!.turnSegments!.flatMap(segment => segment.toolInvocations ?? []);
  } };
}

const mockAnnounce = vi.fn();
vi.mock('../hooks/useAnnouncer', () => ({
  announce: (...args: unknown[]) => mockAnnounce(...args),
}));

const mockAnnounceWithOrigin = vi.fn();
vi.mock('./voiceAccessibility/announcerBroker', () => ({
  announceWithOrigin: (...args: unknown[]) => mockAnnounceWithOrigin(...args),
}));

const mockReloadConversationSnapshot = vi.fn().mockResolvedValue({
  threadedMessages: [],
  messageWindow: {
    scope: 'conversation',
    conversationId: 'conversation-1',
    totalCount: 0,
    startIndex: 0,
    endIndex: -1,
    hasBefore: false,
    hasAfter: false,
  },
  hasOlderMessages: false,
  hasNewerMessages: false,
});
vi.mock('@wailsjs/go/app/App', () => ({
}));
vi.mock('./chatSessionLoader', () => ({
  reloadConversationSnapshot: (...args: unknown[]) => mockReloadConversationSnapshot(...args),
}));

type EventCallback = (data: unknown) => void;
const eventListeners = new Map<string, EventCallback[]>();

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (event: string, cb: EventCallback) => {
    const list = eventListeners.get(event) || [];
    list.push(cb);
    eventListeners.set(event, list);
    return () => {
      const current = eventListeners.get(event) || [];
      eventListeners.set(event, current.filter((candidate) => candidate !== cb));
    };
  },
}));

const mockPlayChatReceiveSoundIfActive = vi.fn();
const mockPlayChatErrorSoundIfActive = vi.fn();
const mockAnnounceForActiveChatConversation = vi.fn();
const mockAnnounceChatBackgroundResponseDone = vi.fn();
const mockGetChatConversationVoiceOrigin = vi.fn((
  conversationId: string,
  _fallbackTitle?: string | null,
  surfaceOrigin?: ChatSurfaceOrigin | null,
) => {
  return {
    conversationId,
    surfaceId: surfaceOrigin?.surfaceId ?? `conversation:${conversationId}`,
    sessionKey: surfaceOrigin?.sessionKey ?? `conversation:${conversationId}`,
    surfaceType: surfaceOrigin?.surfaceType ?? 'page',
    tabId: surfaceOrigin?.tabId,
    title: `Conversa ${conversationId}`,
  };
});
vi.mock('./chatArbitration', () => ({
  playChatReceiveSoundIfActive: (...args: unknown[]) => mockPlayChatReceiveSoundIfActive(...args),
  playChatErrorSoundIfActive: (...args: unknown[]) => mockPlayChatErrorSoundIfActive(...args),
  announceForActiveChatConversation: (...args: unknown[]) => mockAnnounceForActiveChatConversation(...args),
  announceChatBackgroundResponseDone: (...args: unknown[]) => mockAnnounceChatBackgroundResponseDone(...args),
  getChatConversationVoiceOrigin: (conversationId: string, fallbackTitle?: string | null, origin?: ChatSurfaceOrigin | null) => (
    mockGetChatConversationVoiceOrigin(conversationId, fallbackTitle, origin)
  ),
}));

const mockHandleChatSpeak = vi.fn().mockResolvedValue(undefined);
vi.mock('./chatSpeak', () => ({
  handleChatSpeak: (...args: unknown[]) => Promise.resolve(mockHandleChatSpeak(...args)),
}));

vi.mock('i18next', () => ({
  default: {
    t: (key: string, opts?: Record<string, unknown>) => {
      if (key === 'chat.errorPrefix') return `Erro: ${opts?.message}`;
      if (key === 'chat.sendErrorPrefix') return `Falha ao enviar: ${opts?.message}`;
      if (key === 'chat.announce.externalMessage') return `${opts?.from} via ${opts?.channel}: ${opts?.message}`;
      return key;
    },
  },
}));

interface TestSession extends ChatEventSession {
  conversation: ActiveConversation | null;
  activeToolCalls: import('../types/chat').ToolCallStatus[];
  completedSegments: import('../lib/chatMessageTree').TurnSegment[];
  isLoading: boolean;
  streamingMessageId: string | null;
  streamingReasoning: string | null;
  isThinking: boolean;
}

const createMessage = (id: string, role: string, content = '', conversationId = 'conversation-1'): Message => (
  messageFixture({
    id,
    role,
    content,
    conversationId,
    isStreaming: false,
    internal: false,
    createdAt: '2026-04-30T00:00:00.000Z',
  }) as Message
);

const createNode = (message: Message): MessageNode => (
  nodeFixture({
    message,
    children: [],

    childCount: 0,
  }) as MessageNode
);

const createConversation = (id: string): ActiveConversation => ({
  id,
  title: `Conversa ${id}`,
  threadedMessages: [],
});

const createSession = (conversationId: string): TestSession => ({
  conversation: createConversation(conversationId),
  isLoading: false,
  streamingMessageId: null,
  sendFailureMessage: null,
  sendFailureAnnounced: false,
  sendFailureRetryable: false,
  sendFailureRetryContent: null,
  sendFailureRetryMediaFiles: [],
  streamingReasoning: null,
  isThinking: false,
  activeToolCalls: [],
  completedSegments: [],
});

function emitEvent(name: string, data: unknown) {
  // Mantém os cenários históricos focados no comportamento do controller,
  // convertendo suas fixtures acumuladas para o contrato delta atual.
  if (name === 'chat:stream' && data && typeof data === 'object' && 'content' in data) {
    const { content, ...event } = data as Record<string, unknown>;
    if (event.done && content) {
      const listeners = eventListeners.get(name) || [];
      for (const listener of listeners) {
        listener({ ...event, done: false, delta: content, reset: true, sequence: 0 });
      }
      data = event;
    } else {
      data = { ...event, delta: content, reset: true, sequence: 0 };
    }
  }
  const listeners = eventListeners.get(name) || [];
  for (const listener of listeners) listener(data);
}

function createAdapter(initialConversationIds: string[]) {
  const sessions: Record<string, TestSession> = Object.fromEntries(
    initialConversationIds.map((conversationId) => [conversationId, createSession(conversationId)]),
  );
  const updateMessage = vi.fn();
  const commitMessage = vi.fn();

  // Sondas de teste permitem provar que o controller não usa mutações visuais.
  // Não fazem parte da interface de produção.
  const adapter: ChatEventControllerAdapter & {
    patchConversation: (id: string, update: (conversation: ActiveConversation) => ActiveConversation) => void;
    updateMessage: typeof updateMessage;
    commitMessage: typeof commitMessage;
  } = {
    getSession: (conversationId) => sessions[conversationId] ?? createSession(conversationId),
    patchSession: (conversationId, patch) => {
      const current = sessions[conversationId] ?? createSession(conversationId);
      sessions[conversationId] = {
        ...current,
        ...patch,
      } as TestSession;
    },
    patchConversation: vi.fn(),
    updateMessage,
    commitMessage,
    setConversationLoading: (conversationId, isLoading) => {
      const current = sessions[conversationId] ?? createSession(conversationId);
      sessions[conversationId] = {
        ...current,
        isLoading,
      };
    },
  };

  return {
    adapter,
    sessions,
    updateMessage,
    commitMessage,
  };
}

describe('chatEventController', () => {
  it('aplica a identidade final local após substituir uma ponte MCP', async () => {
    const view = await canonicalView();
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    const identity = { conversationId: 'conversation-1', turnId: 'turn-1', assistantMessageId: 'assistant-1', callId: 'replaced' };
    emitEvent('chat:tool_start', { ...identity, name: 'search', origin: 'mcp_bridge', serverLabel: 'remote' });
    emitEvent('chat:tool_end', { ...identity, name: 'local_alias', origin: 'builtin', status: 'ok' });
    const tool = { invocationId: 'inv-replaced', callId: 'replaced', name: 'local_alias', origin: 'builtin', status: 'succeeded', hasDetails: false, resultAvailability: 'available' };
    view.publish([{ segmentId: 'tools', kind: 'tool_invocations', invocations: [tool] }]);
    expect(view.tools()).toEqual([tool]);
    emitEvent('chat:segment_done', { ...identity, hasMore: true, content: '' });
    expect(view.tools()).toEqual([tool]);
    expect(view.tools()[0].serverLabel).toBeUndefined();
    await view.presentation.close();
  });

  it('preserva texto antes das ferramentas e falhas entre duas rodadas', async () => {
    const view = await canonicalView();
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    const identity = { conversationId: 'conversation-1', turnId: 'turn-1', assistantMessageId: 'assistant-1' };

    emitEvent('chat:tool_start', { ...identity, name: 'search', callId: 'call-1' });
    const call = { invocationId: 'inv-call-1', callId: 'call-1', name: 'search', status: 'running', hasDetails: false, resultAvailability: 'pending' };
    view.publish([{ segmentId: 'tools-1', kind: 'tool_invocations', invocations: [call] }]);
    expect(view.tools()[0].status).toBe('running');
    emitEvent('chat:tool_end', { ...identity, name: 'search', callId: 'call-1', status: 'error', summary: 'Indisponível', errorCode: 'invocation_ledger_unavailable' });
    emitEvent('chat:segment_done', { ...identity, hasMore: true, content: 'Vou pesquisar.' });
    emitEvent('chat:tool_start', { ...identity, name: 'read', callId: 'call-2' });
    const firstSegments: TimelineSegment[] = [{ segmentId: 'text-1', kind: 'text', text: 'Vou pesquisar.' },
      { segmentId: 'tools-1', kind: 'tool_invocations', invocations: [{ ...call, status: 'failed', resultAvailability: 'unavailable' }] }];
    view.publish(firstSegments);
    expect(view.presentation.getMessage('response')!.turnSegments!.map(segment => segment.type)).toEqual(['text', 'tool_calls']);
    expect(view.tools()[0]).toMatchObject({ status: 'failed', resultAvailability: 'unavailable' });
    emitEvent('chat:tool_end', { ...identity, name: 'read', callId: 'call-2', status: 'ok' });
    emitEvent('chat:segment_done', { ...identity, hasMore: true, content: 'Vou consultar outra fonte.' });

    view.publish([...firstSegments, { segmentId: 'text-2', kind: 'text', text: 'Vou consultar outra fonte.' },
      { segmentId: 'tools-2', kind: 'tool_invocations', invocations: [{ ...call, invocationId: 'inv-call-2', callId: 'call-2', name: 'read', status: 'succeeded' }] }]);
    const segments = view.presentation.getMessage('response')!.turnSegments!;
    expect(segments.map(segment => segment.type)).toEqual(['text', 'tool_calls', 'text', 'tool_calls']);
    expect(segments.filter(segment => segment.type === 'text').map(segment => segment.content)).toEqual(['Vou pesquisar.', 'Vou consultar outra fonte.']);
    expect(view.tools().map(tool => tool.status)).toEqual(['failed', 'succeeded']);
    await view.presentation.close();
  });

  it('mantém cancelamento distinto de falha no estado ao vivo e no anúncio', async () => {
    const view = await canonicalView();
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    const identity = { conversationId: 'conversation-1', turnId: 'turn-cancel', assistantMessageId: 'assistant-cancel' };

    emitEvent('chat:tool_start', { ...identity, name: 'run_command', callId: 'call-cancel', origin: 'builtin' });
    emitEvent('chat:tool_end', {
      ...identity,
      name: 'run_command',
      callId: 'call-cancel',
      status: 'error',
      errorKind: 'cancelled',
      origin: 'builtin',
    });

    view.publish([{ segmentId: 'tools', kind: 'tool_invocations', invocations: [{ invocationId: 'inv-cancel', callId: 'call-cancel', name: 'run_command',
      status: 'cancelled', hasDetails: false, resultAvailability: 'unavailable' }] }]);
    expect(view.tools()[0].status).toBe('cancelled');
    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledWith(
      'conversation-1',
      'chat.toolRunCommand. chat.toolStatusCancelled',
      'polite',
      undefined,
    );
    expect(mockAnnounceWithOrigin).not.toHaveBeenCalled();

    emitEvent('chat:tool_failure', {
      ...identity,
      name: 'run_command',
      callId: 'call-cancel',
      errorKind: 'cancelled',
      willRetry: false,
      origin: 'builtin',
    });
    expect(mockAnnounceWithOrigin).not.toHaveBeenCalled();
    await view.presentation.close();
  });

  it('anuncia cancelamento recebido somente como tool_failure uma única vez', async () => {
    const view = await canonicalView();
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    const identity = { conversationId: 'conversation-1', turnId: 'turn-cancel', assistantMessageId: 'assistant-cancel' };
    emitEvent('chat:tool_start', { ...identity, name: 'run_command', callId: 'call-cancel-only', origin: 'builtin' });

    emitEvent('chat:tool_failure', {
      ...identity,
      name: 'run_command',
      callId: 'call-cancel-only',
      errorKind: 'cancelled',
      willRetry: false,
      origin: 'builtin',
    });
    emitEvent('chat:tool_failure', {
      ...identity,
      name: 'run_command',
      callId: 'call-cancel-only',
      errorKind: 'cancelled',
      willRetry: false,
      origin: 'builtin',
    });

    view.publish([{ segmentId: 'tools', kind: 'tool_invocations', invocations: [{ invocationId: 'inv-cancel-only', callId: 'call-cancel-only', name: 'run_command',
      status: 'cancelled', hasDetails: false, resultAvailability: 'unavailable' }] }]);
    expect(view.tools()[0].status).toBe('cancelled');
    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledTimes(1);
    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledWith(
      'conversation-1',
      'chat.toolRunCommand. chat.toolStatusCancelled',
      'polite',
      undefined,
    );
    await view.presentation.close();
  });

  it('não promove status terminal legado ou desconhecido a anúncio de sucesso', () => {
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    const identity = { conversationId: 'conversation-1', turnId: 'turn-status', assistantMessageId: 'assistant-status' };

    emitEvent('chat:tool_start', { ...identity, name: 'run_command', callId: 'call-legacy', origin: 'builtin' });
    emitEvent('chat:tool_end', { ...identity, name: 'run_command', callId: 'call-legacy', status: 'cancelled', origin: 'builtin' });
    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledWith(
      'conversation-1', 'chat.toolRunCommand. chat.toolStatusCancelled', 'polite', undefined,
    );

    emitEvent('chat:tool_start', { ...identity, name: 'run_command', callId: 'call-unknown', origin: 'builtin' });
    emitEvent('chat:tool_end', { ...identity, name: 'run_command', callId: 'call-unknown', status: 'mystery', origin: 'builtin' });
    vi.advanceTimersByTime(2000);
    expect(mockAnnounceForActiveChatConversation.mock.calls.flat().join(' ')).not.toContain('chat.toolStatusSucceeded');
    expect(mockAnnounceWithOrigin.mock.calls.flat().map(value => JSON.stringify(value)).join(' ')).not.toContain('chat.toolStatusSucceeded');
  });

  beforeEach(() => {
    vi.useFakeTimers();
    resetChatEventHubForTests();
    eventListeners.clear();
    mockAnnounce.mockClear();
    mockAnnounceWithOrigin.mockClear();
    mockReloadConversationSnapshot.mockReset();
    mockReloadConversationSnapshot.mockResolvedValue({
      threadedMessages: [],
      messageWindow: {
        scope: 'conversation',
        conversationId: 'conversation-1',
        totalCount: 0,
        startIndex: 0,
        endIndex: -1,
        hasBefore: false,
        hasAfter: false,
      },
      hasOlderMessages: false,
      hasNewerMessages: false,
    });
    mockPlayChatReceiveSoundIfActive.mockClear();
    mockPlayChatErrorSoundIfActive.mockClear();
    mockAnnounceForActiveChatConversation.mockClear();
    mockAnnounceChatBackgroundResponseDone.mockClear();
    mockHandleChatSpeak.mockClear();
  });

  afterEach(() => {
    stopAllChatEventControllers();
    resetChatEventHubForTests();
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
    eventListeners.clear();
  });

  it('encerra loading sem criar falha quando o envio é cancelado antes do backend', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);
    const handle = startChatEventController({
      conversationId: 'conversation-1',
      initialUserContent: 'mensagem',
      adapter,
    });

    expect(sessions['conversation-1'].isLoading).toBe(true);
    handle.handleSendCancellation();

    expect(sessions['conversation-1'].isLoading).toBe(false);
    expect(sessions['conversation-1'].sendFailureMessage).toBeNull();
  });

  it('mantém eventos isolados por conversationId', () => {
    const { adapter, sessions } = createAdapter(['conversation-1', 'conversation-2']);

    startChatEventController({ conversationId: 'conversation-1', initialUserContent: 'oi 1', adapter });
    startChatEventController({ conversationId: 'conversation-2', initialUserContent: 'oi 2', adapter });

    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'oi 1',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'resposta 1',
      done: false,
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'resposta 1',
      done: true,
      messageId: 'assistant-1',
    });

    // O transporte operacional não cria outra árvore visual em nenhuma sessão.
    expect(sessions['conversation-1'].conversation?.threadedMessages).toEqual([]);
    expect(adapter.patchConversation).not.toHaveBeenCalled();
    expect(sessions['conversation-2'].conversation?.threadedMessages).toEqual([]);
    expect(mockPlayChatReceiveSoundIfActive).toHaveBeenCalledWith('conversation-1', undefined);
    expect(mockPlayChatReceiveSoundIfActive).not.toHaveBeenCalledWith('conversation-2', expect.anything());
    emitEvent('chat:done', { conversationId: 'conversation-1', hadToolCalls: false });
    expect(sessions['conversation-1'].isLoading).toBe(false);
    expect(sessions['conversation-2'].isLoading).toBe(true);
  });

  it('anuncia deltas Unicode válidos sem reconstruir texto e ignora lacunas de sequência', () => {
    const { adapter, sessions, updateMessage } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      userMessageId: 'turn-1',
      userContent: 'pergunta',
    });

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      messageId: 'assistant-1',
      delta: 'Olá ',
      reset: true,
      sequence: 0,
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      messageId: 'assistant-1',
      baseContent: 'base corrompida',
      delta: ' reset inválido',
      reset: true,
      sequence: 2,
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      messageId: 'assistant-1',
      delta: '世界 👩🏽‍💻',
      sequence: 1,
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      messageId: 'assistant-1',
      delta: ' não pode entrar',
      sequence: 3,
    });
    vi.advanceTimersByTime(16);

    expect(sessions['conversation-1'].conversation?.threadedMessages).toEqual([]);
    expect(adapter.patchConversation).not.toHaveBeenCalled();
    expect(updateMessage).not.toHaveBeenCalled();
    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledTimes(1);
  });

  it('publica conteúdo canônico sem um segundo buffer RAF no controller', async () => {
    const view = await canonicalView();
    const rafSpy = vi.spyOn(globalThis, 'requestAnimationFrame');
    const { adapter, updateMessage } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });

    for (const [sequence, delta] of ['Olá ', '世界 ', '👩🏽‍💻'].entries()) {
      emitEvent('chat:stream', {
        conversationId: 'conversation-1',
        turnId: 'turn-1',
        messageId: 'assistant-1',
        delta,
        reset: sequence === 0,
        sequence,
      });
    }

    expect(rafSpy).not.toHaveBeenCalled();
    expect(updateMessage).not.toHaveBeenCalled();
    view.publish([{ segmentId: 'text', kind: 'text', conclusion: true, text: 'Olá 世界 👩🏽‍💻' }]);
    vi.advanceTimersByTime(16);
    expect(updateMessage).not.toHaveBeenCalled();
    expect(view.presentation.getMessage('response')?.content).toBe('Olá 世界 👩🏽‍💻');
    await view.presentation.close();
    rafSpy.mockRestore();
  });

  it('terminal e desmontagem não reescrevem o conteúdo canônico nem deixam RAF pendente', async () => {
    const view = await canonicalView();
    const cancelSpy = vi.spyOn(globalThis, 'cancelAnimationFrame');
    const first = createAdapter(['conversation-1']);
    const handle = startChatEventController({ conversationId: 'conversation-1', adapter: first.adapter });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      messageId: 'assistant-1',
      delta: 'terminal',
      reset: true,
      sequence: 0,
    });

    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      assistantMessageId: 'assistant-1',
      hadToolCalls: false,
    });

    view.publish([{ segmentId: 'text', kind: 'text', conclusion: true, text: 'terminal' }], { state: 'completed' });
    expect(view.presentation.getMessage('response')?.content).toBe('terminal');
    expect(view.presentation.getMessage('response')?.isStreaming).toBe(false);
    expect(cancelSpy).not.toHaveBeenCalled();
    expect(first.commitMessage).not.toHaveBeenCalled();
    const updatesAfterTerminal = first.updateMessage.mock.calls.length;
    vi.advanceTimersByTime(32);
    expect(first.updateMessage).toHaveBeenCalledTimes(updatesAfterTerminal);

    const second = createAdapter(['conversation-2']);
    const secondHandle = startChatEventController({ conversationId: 'conversation-2', adapter: second.adapter });
    emitEvent('chat:stream', {
      conversationId: 'conversation-2',
      turnId: 'turn-2',
      messageId: 'assistant-2',
      delta: 'desmontagem',
      reset: true,
      sequence: 0,
    });
    secondHandle.cleanup();
    expect(second.commitMessage).not.toHaveBeenCalled();
    const updatesAfterUnmount = second.updateMessage.mock.calls.length;
    vi.advanceTimersByTime(32);
    expect(second.updateMessage).toHaveBeenCalledTimes(updatesAfterUnmount);
    handle.cleanup();
    expect(view.presentation.getMessage('response')?.content).toBe('terminal');
    await view.presentation.close();
    cancelSpy.mockRestore();
  });

  it('mantém publicações independentes em conversas paralelas sem acumulados no controller', async () => {
    const first = await canonicalView('conversation-1');
    const second = await canonicalView('conversation-2');
    const { adapter, updateMessage } = createAdapter(['conversation-1', 'conversation-2']);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    startChatEventController({ conversationId: 'conversation-2', adapter });

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      messageId: 'assistant-1',
      delta: 'um',
      reset: true,
      sequence: 0,
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-2',
      turnId: 'turn-2',
      messageId: 'assistant-2',
      delta: 'dois',
      reset: true,
      sequence: 0,
    });
    vi.advanceTimersByTime(16);

    first.publish([{ segmentId: 'text', kind: 'text', conclusion: true, text: 'um' }]);
    expect(second.presentation.getMessage('response')?.content).toBe('');
    second.publish([{ segmentId: 'text', kind: 'text', conclusion: true, text: 'dois' }]);
    expect(updateMessage).not.toHaveBeenCalled();
    expect(first.presentation.getMessage('response')?.content).toBe('um');
    expect(second.presentation.getMessage('response')?.content).toBe('dois');
    await first.presentation.close(); await second.presentation.close();
  });

  it('apresenta a substituição canônica ao recuperar o mesmo turno sem herdar texto anterior', async () => {
    const view = await canonicalView();
    const { adapter, updateMessage } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      messageId: 'assistant-1',
      delta: 'tentativa parcial',
      reset: true,
      sequence: 0,
    });
    view.publish([{ segmentId: 'text', kind: 'text', conclusion: true, text: 'tentativa parcial' }]);
    expect(view.presentation.getMessage('response')?.content).toBe('tentativa parcial');
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      messageId: 'assistant-1',
      delta: 'resposta recuperada',
      reset: true,
      sequence: 0,
    });
    view.publish([{ segmentId: 'recovered', kind: 'text', conclusion: true, text: 'resposta recuperada' }], {
      metadata: { ...canonicalTestItem('response').metadata },
    });
    vi.advanceTimersByTime(16);

    expect(view.presentation.getMessage('response')?.content).toBe('resposta recuperada');
    expect(updateMessage).not.toHaveBeenCalled();
    await view.presentation.close();
  });

  it('reiniciar a mesma conversa cancela o controller anterior sem criar assistant local sem messageId', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', initialUserContent: 'primeira', adapter });
    startChatEventController({ conversationId: 'conversation-1', initialUserContent: 'segunda', adapter });

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'resposta',
      done: false,
    });

    expect(sessions['conversation-1'].conversation?.threadedMessages).toEqual([]);
    expect(adapter.patchConversation).not.toHaveBeenCalled();
  });

  it('processa messages_ready, stream e done sem alterar a ordem visual publicada pelo backend', async () => {
    const question = canonicalTestItem('user-1', 'pergunta', { conversationId: 'conversation-1', turnId: null });
    question.metadata.role = 'user';
    const response = canonicalTestItem('assistant-1', '', { conversationId: 'conversation-1', state: 'streaming' });
    const fixture = canonicalTestFixture([question, response]);
    const presentation = fixture.presentation('visual', 'conversation-1');
    await presentation.open();
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', initialUserContent: 'fallback', adapter });

    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'pergunta',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'parcial',
      done: false,
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'parcial',
      done: true,
      messageId: 'assistant-1',
    });
    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      hadToolCalls: false,
    });

    fixture.publish('visual', { ...response, state: 'completed', segments: [{ segmentId: 'text', kind: 'text', conclusion: true, text: 'parcial' }] });
    const messages = presentation.nodes();
    expect(messages.map(node => node.itemId)).toEqual(['user-1', 'assistant-1']);
    expect(messages[0].message.content).toBe('pergunta');
    expect(messages[1].message.content).toBe('parcial');
    expect(messages[1].message.isStreaming).toBe(false);
    expect(sessions['conversation-1'].isLoading).toBe(false);
    expect(mockAnnounceChatBackgroundResponseDone).toHaveBeenCalledWith('conversation-1', 'Conversa conversation-1', undefined);
    expect(sessions['conversation-1'].conversation?.threadedMessages).toEqual([]);
    expect(adapter.patchConversation).not.toHaveBeenCalled();
    await presentation.close();
  });

  it('em erro no chat:done sem assistantMessageId não cria mensagem assistant local', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);
    const surfaceOrigin = {
      conversationId: 'conversation-1',
      sessionKey: 'tab-1:conversation-1',
      surfaceId: 'tab-1',
      surfaceType: 'page' as const,
      tabId: 'tab-1',
    };

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'pergunta',
      turnId: 'user-1',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'parcial',
      done: false,
      turnId: 'user-1',
    });

    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      hadToolCalls: false,
      errorMessage: 'assistant_placeholder_error',
      turnId: 'user-1',
      surfaceOrigin,
    });

    const messages = sessions['conversation-1'].conversation?.threadedMessages ?? [];
    expect(messages).toHaveLength(0);
    expect(sessions['conversation-1'].sendFailureMessage).toBe('chat.errors.assistantPlaceholder');
    expect(sessions['conversation-1'].sendFailureAnnounced).toBe(true);
    expect(sessions['conversation-1'].sendFailureRetryable).toBe(false);
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith({
      message: 'chat.errors.assistantPlaceholder',
      origin: expect.objectContaining({
        conversationId: 'conversation-1',
        sessionKey: 'tab-1:conversation-1',
        surfaceId: 'tab-1',
        tabId: 'tab-1',
      }),
      eventType: 'error',
      announcePriority: 'assertive',
    });
    expect(mockPlayChatErrorSoundIfActive).toHaveBeenCalledWith('conversation-1', surfaceOrigin);
  });

  it('em erro no chat:done preserva o alvo canônico e anuncia a falha sem concatená-la ao parcial', async () => {
    const view = await canonicalView();
    const { adapter } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'pergunta',
      turnId: 'user-1',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'parcial',
      done: false,
      turnId: 'user-1',
      messageId: 'assistant-db-1',
    });

    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      hadToolCalls: false,
      errorMessage: 'boom',
      turnId: 'user-1',
      assistantMessageId: 'assistant-db-1',
    });

    view.publish([{ segmentId: 'text', kind: 'text', conclusion: true, text: 'parcial' }], {
      state: 'failed', representativeMessageId: 'assistant-db-1',
      metadata: { ...canonicalTestItem('response').metadata, terminalReason: 'error' },
    });
    const message = view.presentation.getMessage('response')!;
    expect(message.timelineItem?.representativeMessageId).toBe('assistant-db-1');
    expect(message.content).toBe('parcial');
    expect(message.timelineItem?.metadata.terminalReason).toBe('error');
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'boom', eventType: 'error' }));
    await view.presentation.close();
  });

  it('anuncia limite de saída e preserva parcial canônico sem som de erro', async () => {
    const view = await canonicalView();
    const { adapter } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'pergunta',
      turnId: 'user-1',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'resposta parcial',
      done: false,
      turnId: 'user-1',
      messageId: 'assistant-limit',
    });
    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      reason: 'output_limit',
      turnId: 'user-1',
      assistantMessageId: 'assistant-limit',
    });

    view.publish([{ segmentId: 'text', kind: 'text', conclusion: true, text: 'resposta parcial' }], {
      state: 'interrupted', metadata: { ...canonicalTestItem('response').metadata, terminalReason: 'output_limit' },
    });
    expect(view.presentation.getMessage('response')?.content).toBe('resposta parcial');
    expect(view.presentation.getMessage('response')?.timelineItem?.metadata.terminalReason).toBe('output_limit');
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({
      message: 'chat.outputLimitReached',
      eventType: 'system',
      announcePriority: 'assertive',
    }));
    expect(mockPlayChatErrorSoundIfActive).not.toHaveBeenCalled();
    await view.presentation.close();
  });

  it('anuncia limite de saída sem prefixo de erro quando não houve texto', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });
    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'pergunta',
      turnId: 'user-1',
    });
    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      reason: 'output_limit',
      turnId: 'user-1',
      assistantMessageId: 'assistant-limit-empty',
    });

    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'chat.outputLimitReached', eventType: 'system' }));
    expect(sessions['conversation-1'].conversation?.threadedMessages).toEqual([]);
    expect(adapter.patchConversation).not.toHaveBeenCalled();
    expect(mockPlayChatErrorSoundIfActive).not.toHaveBeenCalled();
  });

  it('anuncia limite depois das tools concluídas quando o terminal não traz patch nem texto', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    const identity = { conversationId: 'conversation-1', turnId: 'turn-1', assistantMessageId: 'assistant-1' };
    emitEvent('chat:tool_start', { ...identity, name: 'search', callId: 'call-1', origin: 'builtin' });
    emitEvent('chat:tool_end', { ...identity, name: 'search', callId: 'call-1', status: 'ok' });
    emitEvent('chat:segment_done', { ...identity, hasMore: true, content: '' });
    emitEvent('chat:done', { ...identity, reason: 'output_limit' });
    vi.runOnlyPendingTimers();
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'chat.outputLimitReached', eventType: 'system' }));
    expect(mockPlayChatErrorSoundIfActive).not.toHaveBeenCalled();
    expect(sessions['conversation-1'].isLoading).toBe(false);
  });

  it('preserva o aviso de limite quando o patch persistido está vazio', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'pergunta',
      turnId: 'user-1',
    });

    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      reason: 'output_limit',
      turnId: 'user-1',
      assistantMessageId: 'assistant-limit-patch',
      turnPatch: {
        message: {
          id: 'assistant-limit-patch',
          conversationId: 'conversation-1',
          turnId: 'user-1',
          content: '',
          createdAt: '2026-09-19T10:00:00Z',
          timestamp: 1,
        },
      },
    });

    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'chat.outputLimitReached', eventType: 'system' }));
    expect(mockPlayChatErrorSoundIfActive).not.toHaveBeenCalled();
    expect(sessions['conversation-1'].isLoading).toBe(false);
  });

  it('em erro no chat:stream usa messageId persistido para interrupção', () => {
    const { adapter } = createAdapter(['conversation-1']);
    const surfaceOrigin = {
      conversationId: 'conversation-1',
      sessionKey: 'tab-1:conversation-1',
      surfaceId: 'tab-1',
      surfaceType: 'page' as const,
      tabId: 'tab-1',
    };

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'pergunta',
      turnId: 'user-1',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'parcial',
      done: false,
      turnId: 'user-1',
      messageId: 'assistant-db-2',
    });
    mockPlayChatReceiveSoundIfActive.mockClear();
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      error: 'boom stream',
      done: true,
      turnId: 'user-1',
      messageId: 'assistant-db-2',
      surfaceOrigin,
    });

    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith({
      message: 'boom stream',
      origin: expect.objectContaining({
        conversationId: 'conversation-1',
        sessionKey: 'tab-1:conversation-1',
        surfaceId: 'tab-1',
        tabId: 'tab-1',
      }),
      eventType: 'error',
      announcePriority: 'assertive',
    });
    expect(mockPlayChatErrorSoundIfActive).toHaveBeenCalledWith('conversation-1', surfaceOrigin);
    expect(mockPlayChatReceiveSoundIfActive).not.toHaveBeenCalled();
  });

  it('anuncia erro traduzido quando chat:stream falha sem parcial sem criar fallback visual', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'pergunta',
      turnId: 'user-1',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      error: 'internal_error',
      turnId: 'user-1',
      messageId: 'assistant-db-2',
    });

    expect(sessions['conversation-1'].conversation?.threadedMessages).toEqual([]);
    expect(adapter.patchConversation).not.toHaveBeenCalled();
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'chat.errors.internalError', eventType: 'error' }));
    expect(sessions['conversation-1'].isLoading).toBe(false);
    expect(mockPlayChatErrorSoundIfActive).toHaveBeenCalledWith('conversation-1', undefined);
  });

  it('anuncia erro de chat:stream sem messageId terminal e preserva a classificação de resposta admitida', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:tool_start', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      assistantMessageId: 'assistant-db-existing',
      name: 'buscar',
      callId: 'call-1',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      error: 'boom sem id terminal',
      turnId: 'user-1',
    });

    expect(sessions['conversation-1'].conversation?.threadedMessages).toEqual([]);
    expect(adapter.patchConversation).not.toHaveBeenCalled();
    expect(sessions['conversation-1']).toMatchObject({ sendFailureMessage: 'boom sem id terminal', sendFailureRetryable: false, sendFailureAnnounced: true });
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'boom sem id terminal', eventType: 'error' }));
  });

  it('anuncia erro de chat:done sem assistantMessageId terminal e preserva a classificação de resposta admitida', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:thinking', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      assistantMessageId: 'assistant-db-existing',
      started: true,
    });
    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      hadToolCalls: false,
      errorMessage: 'done sem id terminal',
    });

    expect(sessions['conversation-1'].conversation?.threadedMessages).toEqual([]);
    expect(adapter.patchConversation).not.toHaveBeenCalled();
    expect(sessions['conversation-1']).toMatchObject({ sendFailureMessage: 'done sem id terminal', sendFailureRetryable: false, sendFailureAnnounced: true });
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'done sem id terminal', eventType: 'error' }));
  });

  it('anuncia progresso interno pendente antes de uma ferramenta longa terminar', () => {
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:tool_start', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      name: 'buscar',
      callId: 'call-1',
      origin: 'builtin',
    });

    expect(mockAnnounceForActiveChatConversation).not.toHaveBeenCalled();
    vi.advanceTimersByTime(249);
    expect(mockAnnounceForActiveChatConversation).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);

    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledWith(
      'conversation-1',
      'chat.toolGeneric. chat.toolStatusRunning',
      'polite',
      undefined,
    );
    emitEvent('chat:tool_end', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      name: 'buscar',
      callId: 'call-1',
      status: 'ok',
      origin: 'builtin',
    });
    vi.advanceTimersByTime(250);
    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledTimes(2);
    expect(mockAnnounceForActiveChatConversation).toHaveBeenLastCalledWith(
      'conversation-1',
      'chat.toolGenericDone. chat.toolStatusSucceeded',
      'polite',
      undefined,
    );
  });

  it('agrupa ferramentas internas rápidas no primeiro tool_end sem duplicar a rajada', () => {
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });

    for (const [callId, name] of [['call-1', 'buscar'], ['call-2', 'ler arquivo']]) {
      emitEvent('chat:tool_start', {
        conversationId: 'conversation-1',
        turnId: 'turn-1',
        name,
        callId,
        origin: 'builtin',
      });
    }
    emitEvent('chat:tool_end', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      name: 'buscar',
      callId: 'call-1',
      status: 'ok',
      origin: 'builtin',
    });
    emitEvent('chat:tool_end', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      name: 'ler arquivo',
      callId: 'call-2',
      status: 'ok',
      origin: 'builtin',
    });
    vi.advanceTimersByTime(250);

    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledTimes(1);
    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledWith(
      'conversation-1',
      'chat.toolGenericDone. chat.toolStatusSucceeded; chat.toolGenericDone. chat.toolStatusSucceeded',
      'polite',
      undefined,
    );
  });

  it('cancela progresso pendente ao desmontar e preserva a origem da superfície', () => {
    const { adapter } = createAdapter(['conversation-1']);
    const surfaceOrigin = {
      tabId: 'tab-1',
      surfaceId: 'tab-1',
      sessionKey: 'tab-1:conversation-1',
      conversationId: 'conversation-1',
      surfaceType: 'page' as const,
    };
    const handle = startChatEventController({ conversationId: 'conversation-1', origin: surfaceOrigin, adapter });

    emitEvent('chat:tool_start', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      name: 'buscar',
      callId: 'call-1',
      origin: 'builtin',
      surfaceOrigin,
    });
    handle.cleanup();
    vi.advanceTimersByTime(250);

    expect(mockAnnounceForActiveChatConversation).not.toHaveBeenCalled();
  });

  it('isola o anúncio de progresso entre conversas mesmo quando uma termina', () => {
    const { adapter } = createAdapter(['conversation-1', 'conversation-2']);
    const first = startChatEventController({ conversationId: 'conversation-1', adapter });
    startChatEventController({ conversationId: 'conversation-2', adapter });

    emitEvent('chat:tool_start', {
      conversationId: 'conversation-1',
      name: 'primeira',
      callId: 'call-1',
      origin: 'builtin',
    });
    emitEvent('chat:tool_start', {
      conversationId: 'conversation-2',
      name: 'segunda',
      callId: 'call-2',
      origin: 'builtin',
    });
    first.cleanup();
    vi.advanceTimersByTime(250);

    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledTimes(1);
    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledWith(
      'conversation-2',
      'chat.toolGeneric. chat.toolStatusRunning',
      'polite',
      undefined,
    );
  });

  it('anuncia retry/falha uma vez e não repete progresso do mesmo callId', () => {
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:tool_start', {
      conversationId: 'conversation-1',
      name: 'buscar',
      callId: 'call-1',
      origin: 'builtin',
    });
    emitEvent('chat:tool_failure', {
      conversationId: 'conversation-1',
      name: 'buscar',
      callId: 'call-1',
      origin: 'builtin',
      willRetry: true,
    });
    emitEvent('chat:tool_start', {
      conversationId: 'conversation-1',
      name: 'buscar',
      callId: 'call-1',
      origin: 'builtin',
    });
    vi.advanceTimersByTime(250);

    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledTimes(2);
    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledWith(
      'conversation-1',
      'chat.toolGeneric. chat.toolStatusRetrying',
      'polite',
      undefined,
    );

    emitEvent('chat:tool_failure', {
      conversationId: 'conversation-1',
      name: 'buscar',
      callId: 'call-1',
      origin: 'builtin',
      willRetry: false,
    });
    expect(mockAnnounceWithOrigin).toHaveBeenCalledTimes(1);
    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledTimes(2);
    expect(mockAnnounceForActiveChatConversation).toHaveBeenLastCalledWith(
      'conversation-1',
      'chat.toolGeneric. chat.toolStatusRunning',
      'polite',
      undefined,
    );
  });

  it('usa a origem da ferramenta do agente nos anúncios de progresso', () => {
    const { adapter } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:tool_start', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      assistantMessageId: 'assistant-db-acp',
      name: 'execute',
      callId: 'call-acp',
      summary: 'npm test',
      origin: 'acp_agent',
    });

    vi.runOnlyPendingTimers();
    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledWith('conversation-1', 'chat.toolGeneric. chat.toolStatusRunning', 'polite', undefined);

    emitEvent('chat:tool_end', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      assistantMessageId: 'assistant-db-acp',
      name: 'execute',
      callId: 'call-acp',
      status: 'ok',
      origin: 'acp_agent',
    });


    emitEvent('chat:segment_done', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      assistantMessageId: 'assistant-db-acp',
      content: 'primeiro bloco',
      hasMore: true,
    });

    expect(mockAnnounceForActiveChatConversation.mock.calls.some(call => String(call[1]).includes('execute'))).toBe(false);
  });

  it('continua anunciando a execução quando os metadados da conversa chegam depois', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);
    sessions['conversation-1'].conversation = null;

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:tool_start', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      assistantMessageId: 'assistant-db-delayed',
      name: 'buscar',
      callId: 'call-1',
    });

    sessions['conversation-1'].conversation = createConversation('conversation-1');

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      messageId: 'assistant-db-delayed',
      content: 'resposta após carregar',
      done: false,
    });
    vi.runOnlyPendingTimers();

    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledWith('conversation-1', 'chat.announce.assistantResponding', 'polite', undefined);
    expect(sessions['conversation-1'].isLoading).toBe(true);
    emitEvent('chat:done', { conversationId: 'conversation-1', turnId: 'user-1', reason: 'completed' });
    expect(sessions['conversation-1'].isLoading).toBe(false);
  });

  it('toca som de erro em chat:error e chat:tool_failure final, respeitando arbitragem', () => {
    const { adapter } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:tool_failure', {
      conversationId: 'conversation-1',
      name: 'minha_tool',
      callId: 'call-1',
      willRetry: true,
    });
    expect(mockPlayChatErrorSoundIfActive).not.toHaveBeenCalled();

    emitEvent('chat:tool_failure', {
      conversationId: 'conversation-1',
      name: 'minha_tool',
      callId: 'call-1',
      willRetry: false,
    });
    expect(mockPlayChatErrorSoundIfActive).toHaveBeenCalledWith('conversation-1', undefined);

    mockPlayChatErrorSoundIfActive.mockClear();
    emitEvent('chat:error', {
      conversationId: 'conversation-1',
      error: 'falha geral',
    });
    expect(mockPlayChatErrorSoundIfActive).toHaveBeenCalledWith('conversation-1', undefined);
  });

  it('traduz a falha de preparação MCP no evento de erro', () => {
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    emitEvent('chat:error', {
      conversationId: 'conversation-1',
      error: 'mcp_preparation_error',
    });
    expect(mockAnnounce).toHaveBeenCalledWith('chat.errors.mcpPreparation');
  });

  it('limpa falha operacional anterior quando uma nova tentativa termina com sucesso', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);
    const identity = { conversationId: 'conversation-1', turnId: 'turn-1', assistantMessageId: 'assistant-1' };
    startChatEventController({ conversationId: 'conversation-1', adapter });
    emitEvent('chat:stream', { ...identity, messageId: 'assistant-1', reset: true, sequence: 0, delta: 'Tentativa antiga' });
    emitEvent('chat:tool_start', { ...identity, name: 'search', callId: 'old-call', origin: 'builtin' });
    emitEvent('chat:tool_end', { ...identity, name: 'search', callId: 'old-call', status: 'ok' });
    emitEvent('chat:done', { ...identity, reason: 'error', errorMessage: 'persistence_error' });
    vi.runOnlyPendingTimers();
    startChatEventController({ conversationId: 'conversation-1', adapter });
    emitEvent('chat:stream', { ...identity, messageId: 'assistant-1', reset: true, sequence: 0, baseContent: '', delta: 'Resposta nova' });
    emitEvent('chat:stream', { ...identity, messageId: 'assistant-1', done: true });
    emitEvent('chat:done', { ...identity, reason: 'completed' });
    vi.runOnlyPendingTimers();
    expect(sessions['conversation-1'].sendFailureMessage).toBeNull();
    expect(sessions['conversation-1'].sendFailureRetryable).toBe(false);
    expect(sessions['conversation-1'].isLoading).toBe(false);
    expect(mockAnnounceChatBackgroundResponseDone).toHaveBeenCalledOnce();
    expect(adapter.patchConversation).not.toHaveBeenCalled();
  });

  it('anuncia falha de salvamento sem pedir nova geração', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1', userMessageId: 'user-1',
      userContent: 'pergunta', turnId: 'user-1',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1', turnId: 'user-1',
      messageId: 'assistant-db-3', content: 'resposta completa recebida', done: false,
    });
    // Terminal arrives before the animation frame has rendered the last delta.
    emitEvent('chat:done', {
      conversationId: 'conversation-1', turnId: 'user-1',
      assistantMessageId: 'assistant-db-3', reason: 'error',
      errorMessage: 'persistence_error',
    });
    vi.runOnlyPendingTimers();
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'chat.errors.persistenceError', eventType: 'error' }));
    expect(sessions['conversation-1'].isLoading).toBe(false);
    expect(mockReloadConversationSnapshot).not.toHaveBeenCalled();
    expect(sessions['conversation-1'].sendFailureRetryable).toBe(false);
  });

  it('anuncia falha sem parcial sem fabricar conteúdo de resposta', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'pergunta',
      turnId: 'user-1',
    });
    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      hadToolCalls: false,
      errorMessage: 'internal_error',
      turnId: 'user-1',
      assistantMessageId: 'assistant-db-3',
    });

    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'chat.errors.internalError', eventType: 'error' }));
    expect(sessions['conversation-1'].conversation?.threadedMessages).toEqual([]);
    expect(adapter.patchConversation).not.toHaveBeenCalled();
    expect(sessions['conversation-1'].isLoading).toBe(false);
  });

  it('propaga surfaceOrigin dos eventos para anúncio, som e fala', async () => {
    const { adapter } = createAdapter(['conversation-1']);
    const surfaceOrigin = {
      conversationId: 'conversation-1',
      sessionKey: 'tab-1:conversation-1',
      surfaceId: 'tab-1',
      surfaceType: 'page' as const,
      tabId: 'tab-1',
    };

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'resposta',
      done: false,
      messageId: 'assistant-1',
      surfaceOrigin,
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'resposta',
      done: true,
      messageId: 'assistant-1',
      surfaceOrigin,
    });
    emitEvent('chat:speak', {
      conversationId: 'conversation-1',
      role: 'assistant',
      text: 'fala com origem',
      strategy: 'webspeech',
      surfaceOrigin,
    });
    await Promise.resolve();
    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      hadToolCalls: false,
      surfaceOrigin,
    });

    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledWith(
      'conversation-1',
      'chat.announce.assistantResponding',
      'polite',
      surfaceOrigin,
    );
    expect(mockPlayChatReceiveSoundIfActive).toHaveBeenCalledWith('conversation-1', surfaceOrigin);
    expect(mockGetChatConversationVoiceOrigin).toHaveBeenCalledWith('conversation-1', undefined, surfaceOrigin);
    expect(mockHandleChatSpeak.mock.calls[0][0]).toMatchObject({
      accessibilityOrigin: expect.objectContaining({
        conversationId: 'conversation-1',
        sessionKey: 'tab-1:conversation-1',
        surfaceId: 'tab-1',
        tabId: 'tab-1',
      }),
    });
    expect(mockAnnounceChatBackgroundResponseDone).toHaveBeenCalledWith(
      'conversation-1',
      'Conversa conversation-1',
      surfaceOrigin,
    );
  });

  it('mantém chat:speak ativo até o done e ignora eventos depois do cleanup', async () => {
    const { adapter } = createAdapter(['conversation-1']);

    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:speak', {
      conversationId: 'conversation-1',
      role: 'assistant',
      text: 'antes do done',
      strategy: 'webspeech',
    });
    await Promise.resolve();

    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      hadToolCalls: false,
    });
    emitEvent('chat:speak', {
      conversationId: 'conversation-1',
      role: 'assistant',
      text: 'depois do done',
      strategy: 'webspeech',
    });
    await Promise.resolve();

    expect(mockHandleChatSpeak).toHaveBeenCalledTimes(1);
    expect(mockHandleChatSpeak.mock.calls[0][0]).toMatchObject({
      text: 'antes do done',
      accessibilityOrigin: {
        conversationId: 'conversation-1',
        surfaceId: 'conversation:conversation-1',
        title: 'Conversa conversation-1',
      },
    });
  });

  it('ignora update de streaming sem messageId ao limpar controller', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    const handle = startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      content: 'conteúdo atrasado',
      done: false,
    });
    handle.cleanup();
    vi.runOnlyPendingTimers();

    expect(sessions['conversation-1'].conversation?.threadedMessages).toEqual([]);
    expect(adapter.patchConversation).not.toHaveBeenCalled();
  });

  it('entrada externa anuncia origem e usa a sessão por conversationId', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);

    startChatEventController({
      conversationId: 'conversation-1',
      external: { channel: 'telegram', from: 'Maria', text: 'fallback externo' },
      adapter,
    });

    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-ext',
      userContent: '**olá** externo',
    });

    expect(sessions['conversation-1'].isLoading).toBe(true);
    expect(sessions['conversation-1'].conversation?.threadedMessages).toEqual([]);
    expect(adapter.patchConversation).not.toHaveBeenCalled();
    expect(mockAnnounce).toHaveBeenCalledWith('Maria via telegram: olá externo');
  });

  it('anuncia a ferramenta do agente sem expor seu nome técnico quando o fim não repete a origem', () => {
    const { adapter } = createAdapter(['conversation-1']);

    startChatEventController({
      conversationId: 'conversation-1',
      external: { channel: 'telegram', from: 'Maria', text: 'fallback externo' },
      adapter,
    });

    emitEvent('chat:tool_start', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      assistantMessageId: 'assistant-db-acp',
      name: 'execute',
      callId: 'call-acp',
      origin: 'acp_agent',
    });
    emitEvent('chat:tool_end', {
      conversationId: 'conversation-1',
      turnId: 'user-1',
      assistantMessageId: 'assistant-db-acp',
      name: 'execute',
      callId: 'call-acp',
      status: 'ok',
    });

    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledWith(
      'conversation-1',
      'chat.toolGenericDone. chat.toolStatusSucceeded',
      'polite',
      undefined,
    );
  });

  it('preserva o provedor MCP no anúncio amigável de falha mesmo quando a falha omite a origem', () => {
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({
      conversationId: 'conversation-1',
      external: { channel: 'telegram', from: 'Maria', text: 'fallback externo' },
      adapter,
    });

    emitEvent('chat:tool_start', {
      conversationId: 'conversation-1',
      name: 'crm_internal_lookup_v2',
      callId: 'call-mcp',
      origin: 'mcp_native',
      serverLabel: 'CRM Exemplo',
    });
    emitEvent('chat:tool_failure', {
      conversationId: 'conversation-1',
      name: 'crm_internal_lookup_v2',
      callId: 'call-mcp',
      willRetry: false,
    });

    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({
      message: 'chat.toolMcpProvider. chat.toolStatusFailed',
    }));
  });

  // O writer/SQLite prova a decisão de abortar antes do efeito e a persistência.
  // Aqui o terminal operacional anuncia a falha sem fabricar segmentos visuais.
  it.each([
    [false, 'persisted'], [true, 'persisted'], [false, 'volatile'], [true, 'volatile'],
  ] as const)('anuncia falha do ledger preservando a fonte canônica: parcial=%s durabilidade=%s', async (partial, durability) => {
    const view = await canonicalView();
    const { adapter, sessions } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    const identity = { conversationId: 'conversation-1', turnId: 'turn', assistantMessageId: 'assistant' };
    emitEvent('chat:messages_ready', { ...identity, userMessageId: 'question', userContent: 'pergunta' });
    emitEvent('chat:tool_start', { ...identity, name: 'echo', callId: 'blocked' });
    emitEvent('chat:tool_end', { ...identity, callId: 'blocked', status: 'error', errorKind: 'unavailable',
      errorCode: 'invocation_ledger_unavailable', summary: 'diagnostic' });
    const segments: TimelineSegment[] = partial ? [{ segmentId: 'text', kind: 'text', conclusion: true, text: 'parcial recebido' }] : [];
    view.publish(segments, { state: 'failed', durability,
      metadata: { ...canonicalTestItem('response').metadata, terminalReason: 'persistence_error' } });
    const canonical = view.presentation.getMessage('response');
    emitEvent('chat:done', { ...identity, reason: 'error', errorMessage: 'persistence_error' });
    expect(view.presentation.getMessage('response')).toBe(canonical);
    expect(canonical?.content).toBe(partial ? 'parcial recebido' : '');
    expect(canonical?.timelineItem?.metadata.terminalReason).toBe('persistence_error');
    expect(view.tools()).toEqual([]);
    expect(adapter.patchConversation).not.toHaveBeenCalled();
    expect(sessions['conversation-1'].isLoading).toBe(false);
    expect(sessions['conversation-1'].sendFailureRetryable).toBe(false);
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'chat.errors.persistenceError', eventType: 'error' }));
    await view.presentation.close();
  });

  it('ignora patches visuais operacionais sem recarregar nem modificar outra fonte', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);
    const transientAssistant = createMessage('backend-assistant', 'assistant', 'temporária');
    transientAssistant.turnId = 'turn-1';
    const duplicateAssistant = createMessage('assistant-iteration-2', 'assistant', 'outra iteração');
    duplicateAssistant.turnId = 'turn-1';
    const legacyTool = createMessage('legacy-tool', 'tool', 'resultado legado');
    legacyTool.turnId = 'turn-1';
    sessions['conversation-1'].conversation!.threadedMessages = [
      createNode(transientAssistant),
      createNode(duplicateAssistant),
      createNode(legacyTool),
    ];
    const originalMessages = sessions['conversation-1'].conversation!.threadedMessages;

    startChatEventController({ conversationId: 'conversation-1', adapter });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      messageId: 'backend-assistant',
      delta: 'rascunho em streaming',
      reset: true,
      sequence: 0,
    });

    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      assistantMessageId: 'backend-assistant',
      hadToolCalls: true,
      turnPatch: {
        message: {
          id: 'backend-assistant',
          conversationId: 'conversation-1',
          turnId: 'turn-1',
          content: 'resposta final',
          createdAt: '2026-09-08T20:00:00Z',
          timestamp: 1,
          turnSegments: [
            { type: 'text', content: 'vou consultar' },
            {
              type: 'tool_calls',
              toolInvocations: [{
                invocationId: 'inv-1',
                callId: 'call-1',
                name: 'update_plan',
                status: 'succeeded',
                hasDetails: true,
                resultAvailability: 'available',
              }],
            },
            { type: 'text', content: 'resposta final' },
          ],
        },
      },
    });

    expect(mockReloadConversationSnapshot).not.toHaveBeenCalled();
    expect(sessions['conversation-1'].conversation?.threadedMessages.map((node) => node.message.id)).toEqual([
      'backend-assistant',
      'assistant-iteration-2',
      'legacy-tool',
    ]);
    expect(sessions['conversation-1'].conversation?.threadedMessages).toBe(originalMessages);
    expect(adapter.patchConversation).not.toHaveBeenCalled();
    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledTimes(1);
  });

  it('mantém um listener global por evento após vários turnos', () => {
    const { adapter } = createAdapter(['conversation-1']);

    for (let index = 0; index < 5; index += 1) {
      startChatEventController({ conversationId: 'conversation-1', adapter });
      emitEvent('chat:done', {
        conversationId: 'conversation-1',
        turnId: `turn-${index}`,
        hadToolCalls: false,
      });
    }

    expect(eventListeners.size).toBe(CHAT_TURN_EVENT_NAMES.length);
    for (const name of CHAT_TURN_EVENT_NAMES) {
      expect(eventListeners.get(name)).toHaveLength(1);
    }
  });

  it('ignora evento atrasado de outro turnId na mesma conversa', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      turnId: 'turn-atual',
      userMessageId: 'turn-atual',
      userContent: 'pergunta atual',
    });

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      turnId: 'turn-antigo',
      messageId: 'assistant-antigo',
      content: 'evento atrasado',
      done: false,
    });

    emitEvent('chat:done', { conversationId: 'conversation-1', turnId: 'turn-antigo', reason: 'completed' });
    expect(sessions['conversation-1'].isLoading).toBe(true);
    expect(mockPlayChatReceiveSoundIfActive).not.toHaveBeenCalled();
    emitEvent('chat:done', { conversationId: 'conversation-1', turnId: 'turn-atual', reason: 'completed' });
    expect(sessions['conversation-1'].isLoading).toBe(false);
  });

  it('anuncia conclusão genérica quando o turno com ferramentas termina sem texto', () => {
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      hadToolCalls: true,
    });

    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledWith(
      'conversation-1',
      'chat.progressLabel',
      'polite',
      undefined,
    );
  });

  it('não anuncia conclusão quando o turno termina cancelado', () => {
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      turnId: 'turn-cancelled',
      assistantMessageId: 'assistant-cancelled',
      hadToolCalls: true,
      reason: 'cancelled',
    });

    expect(mockAnnounceForActiveChatConversation).not.toHaveBeenCalledWith(
      'conversation-1',
      'chat.progressLabel',
      'polite',
      undefined,
    );
    expect(mockAnnounceChatBackgroundResponseDone).not.toHaveBeenCalled();
  });

  it('encerra cancelamento sem aplicar patch visual do evento operacional', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      turnId: 'turn-cancelled',
      messageId: 'assistant-cancelled',
      content: 'conteúdo transitório',
      done: false,
    });

    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      turnId: 'turn-cancelled',
      assistantMessageId: 'assistant-cancelled',
      reason: 'cancelled',
      turnPatch: {
        message: {
          id: 'assistant-cancelled',
          conversationId: 'conversation-1',
          turnId: 'turn-cancelled',
          content: 'parcial persistido',
          createdAt: '2026-09-19T10:00:00Z',
          timestamp: 1,
        },
      },
    });

    expect(adapter.patchConversation).not.toHaveBeenCalled();
    expect(sessions['conversation-1'].isLoading).toBe(false);
    expect(mockPlayChatReceiveSoundIfActive).not.toHaveBeenCalled();
    expect(mockAnnounceChatBackgroundResponseDone).not.toHaveBeenCalled();
  });

  it('cobre superfície inativa com o anúncio de resposta em segundo plano', () => {
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      hadToolCalls: true,
    });

    // O aviso tool-only usa eventType 'progress' e o broker o silencia fora da
    // aba ativa; quem cobre a aba inativa é o anúncio de segundo plano.
    expect(mockAnnounceChatBackgroundResponseDone).toHaveBeenCalledWith(
      'conversation-1',
      'Conversa conversation-1',
      undefined,
    );
  });

  it('não anuncia conclusão genérica quando o turno com ferramentas produziu texto', () => {
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      messageId: 'assistant-1',
      content: 'resposta com texto',
      done: false,
    });
    mockAnnounceForActiveChatConversation.mockClear();

    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      hadToolCalls: true,
    });

    expect(mockAnnounceForActiveChatConversation).not.toHaveBeenCalledWith(
      'conversation-1',
      'chat.progressLabel',
      'polite',
      undefined,
    );
  });

  it('anuncia processamento, falha e cancelamento de mídia de forma acessível', () => {
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:media_processing', {
      conversationId: 'conversation-1',
      status: 'started',
    });
    emitEvent('chat:media_processing', {
      conversationId: 'conversation-1',
      status: 'failed',
      error: 'indisponível',
    });
    emitEvent('chat:media_processing', {
      conversationId: 'conversation-1',
      status: 'cancelled',
    });

    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledWith(
      'conversation-1',
      'chat.mediaProcessing.started',
      'polite',
      undefined,
    );
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({
      message: 'chat.mediaProcessing.failed',
      eventType: 'error',
      announcePriority: 'assertive',
    }));
    expect(mockAnnounceForActiveChatConversation).toHaveBeenCalledWith(
      'conversation-1',
      'chat.mediaProcessing.cancelled',
      'polite',
      undefined,
    );
  });

  it('considera baseContent como texto na continuação explícita', () => {
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      messageId: 'assistant-1',
      baseContent: 'resposta parcial',
      delta: ' ',
      reset: true,
      sequence: 0,
      done: false,
    });
    mockAnnounceForActiveChatConversation.mockClear();

    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      assistantMessageId: 'assistant-1',
      hadToolCalls: true,
    });

    expect(mockAnnounceForActiveChatConversation).not.toHaveBeenCalledWith(
      'conversation-1',
      'chat.progressLabel',
      'polite',
      undefined,
    );
  });

  it('traduz streaming_interrupted', () => {
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      error: 'streaming_interrupted',
      turnId: 't1',
      messageId: 'a1',
    });
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'chat.errors.streamingInterrupted', eventType: 'error' }));
  });

  it('traduz streaming_interrupted em chat:done sem sobrescrever a conclusão canônica com turnPatch', async () => {
    const view = await canonicalView();
    const { adapter } = createAdapter(['conversation-1']);
    view.publish([{ segmentId: 'text', kind: 'text', conclusion: true, text: 'resposta parcial' }], { state: 'interrupted' });
    startChatEventController({ conversationId: 'conversation-1', adapter });
    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-1',
      userContent: 'pergunta',
      turnId: 't1',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      delta: 'texto transmitido incompleto',
      reset: true,
      sequence: 0,
      turnId: 't1',
      messageId: 'a1',
    });
    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      errorMessage: 'streaming_interrupted',
      turnId: 't1',
      assistantMessageId: 'a1',
      hadToolCalls: false,
      turnPatch: {
        message: {
          id: 'a1',
          conversationId: 'conversation-1',
          turnId: 't1',
          content: 'resposta parcial',
          createdAt: '2026-09-11T18:00:00Z',
          timestamp: 1,
        },
      },
    });

    expect(view.presentation.getMessage('response')?.content).toBe('resposta parcial');
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'chat.errors.streamingInterrupted' }));
    await view.presentation.close();
  });

  it('traduz streaming_idle_timeout', () => {
    const { adapter } = createAdapter(['conversation-2']);
    startChatEventController({ conversationId: 'conversation-2', adapter });
    emitEvent('chat:stream', {
      conversationId: 'conversation-2',
      error: 'streaming_idle_timeout',
      turnId: 't1',
      messageId: 'a1',
    });
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'chat.errors.streamingIdleTimeout', eventType: 'error' }));
  });

  it('traduz streaming_retries_exhausted', () => {
    const { adapter } = createAdapter(['conversation-2']);
    startChatEventController({ conversationId: 'conversation-2', adapter });
    emitEvent('chat:stream', {
      conversationId: 'conversation-2',
      error: 'streaming_retries_exhausted',
      turnId: 't1',
      messageId: 'a1',
    });
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'chat.errors.streamingRetriesExhausted', eventType: 'error' }));
  });

  it('traduz streaming_prompt_cache_hint_rejected', () => {
    const { adapter } = createAdapter(['conversation-2']);
    startChatEventController({ conversationId: 'conversation-2', adapter });
    emitEvent('chat:stream', {
      conversationId: 'conversation-2',
      error: 'streaming_prompt_cache_hint_rejected',
      turnId: 't1',
      messageId: 'a1',
    });
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'chat.errors.streamingPromptCacheHintRejected', eventType: 'error' }));
  });

  it.each([
    ['chatgpt_response_incomplete', 'incomplete'], ['chatgpt_stream_interrupted', 'interrupted'],
    ['chatgpt_plan_limit', 'planLimit'], ['chatgpt_model_unavailable', 'modelUnavailable'],
    ['chatgpt_reauthorization_required', 'reauthorize'], ['chatgpt_permission_required', 'permission'],
    ['chatgpt_temporarily_unavailable', 'temporary'], ['chatgpt_rate_limit', 'rateLimit'], ['chatgpt_request_cancelled', 'cancelled'], ['chatgpt_request_failed', 'failed'],
  ])('traduz falha ChatGPT %s', (error, key) => {
    const { adapter } = createAdapter(['conversation-2']);
    startChatEventController({ conversationId: 'conversation-2', adapter });
    emitEvent('chat:stream', { conversationId: 'conversation-2', error, turnId: 't1', messageId: 'a1' });
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: `chatgpt.errors.${key}`, eventType: 'error' }));
  });

  it('traduz streaming_token_rate_limit', () => {
    const { adapter } = createAdapter(['conversation-2']);
    startChatEventController({ conversationId: 'conversation-2', adapter });
    emitEvent('chat:stream', {
      conversationId: 'conversation-2',
      error: 'streaming_token_rate_limit',
      turnId: 't1',
      messageId: 'a1',
    });
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'chat.errors.streamingTokenRateLimit', eventType: 'error' }));
  });

  it('preserva bytes do parcial canônico e anuncia erro terminal de chat:stream separadamente', async () => {
    const view = await canonicalView();
    const { adapter } = createAdapter(['conversation-1']);
    view.publish([{ segmentId: 'text', kind: 'text', conclusion: true, text: 'resposta parcial  \n' }]);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      delta: 'resposta parcial  \n',
      reset: true,
      sequence: 0,
      turnId: 't1',
      messageId: 'a1',
    });
    vi.runOnlyPendingTimers();
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      error: 'streaming_idle_timeout',
      sequence: 1,
      turnId: 't1',
      messageId: 'a1',
    });

    expect(view.presentation.getMessage('response')?.content).toBe('resposta parcial  \n');
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'chat.errors.streamingIdleTimeout' }));
    await view.presentation.close();
  });

  it('preserva parcial canônico e anuncia erro terminal de chat:done separadamente', async () => {
    const view = await canonicalView();
    const { adapter } = createAdapter(['conversation-1']);
    view.publish([{ segmentId: 'text', kind: 'text', conclusion: true, text: 'resposta parcial' }]);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      delta: 'resposta parcial',
      reset: true,
      sequence: 0,
      turnId: 't1',
      messageId: 'a1',
    });
    vi.runOnlyPendingTimers();
    emitEvent('chat:done', {
      conversationId: 'conversation-1',
      errorMessage: 'streaming_interrupted',
      turnId: 't1',
      assistantMessageId: 'a1',
      hadToolCalls: false,
    });

    expect(view.presentation.getMessage('response')?.content).toBe('resposta parcial');
    expect(mockAnnounceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({ message: 'chat.errors.streamingInterrupted' }));
    await view.presentation.close();
  });

  it('limpa reasoning descartado quando o backend substitui os segmentos da tentativa', async () => {
    const view = await canonicalView();
    const { adapter } = createAdapter(['conversation-1']);
    startChatEventController({ conversationId: 'conversation-1', adapter });
    emitEvent('chat:thinking', {
      conversationId: 'conversation-1',
      assistantMessageId: 'a1',
      turnId: 't1',
      started: true,
      content: 'raciocínio descartado',
    });
    view.publish([{ segmentId: 'reasoning', kind: 'reasoning', text: 'raciocínio já salvo' }]);
    expect(view.presentation.getMessage('response')?.reasoning).toBe('raciocínio já salvo');
    emitEvent('chat:thinking', {
      conversationId: 'conversation-1',
      assistantMessageId: 'a1',
      turnId: 't1',
      done: true,
      content: '',
    });

    view.publish([], { metadata: { ...canonicalTestItem('response').metadata, thinking: false } });
    expect(view.presentation.getMessage('response')?.timelineItem?.metadata.thinking).toBe(false);
    expect(view.presentation.getMessage('response')?.reasoning).toBe('');
    await view.presentation.close();
  });

  it('não vincula o controller por thinking atrasado antes de messages_ready', () => {
    const { adapter, sessions } = createAdapter(['conversation-1']);
    sessions['conversation-1'].conversation!.threadedMessages = [
      createNode(createMessage('assistant-antigo', 'assistant', 'resposta persistida')),
    ];
    startChatEventController({ conversationId: 'conversation-1', adapter });

    emitEvent('chat:thinking', {
      conversationId: 'conversation-1',
      assistantMessageId: 'assistant-antigo',
      turnId: 'turn-antigo',
      started: true,
      content: 'atrasado',
    });
    emitEvent('chat:messages_ready', {
      conversationId: 'conversation-1',
      userMessageId: 'user-novo',
      userContent: 'pergunta nova',
      turnId: 'turn-novo',
    });
    emitEvent('chat:stream', {
      conversationId: 'conversation-1',
      messageId: 'assistant-novo',
      turnId: 'turn-novo',
      delta: 'resposta nova',
      reset: true,
      sequence: 0,
    });
    vi.runOnlyPendingTimers();

    const messages = sessions['conversation-1'].conversation?.threadedMessages ?? [];
    expect(messages.map((node) => node.message.id)).toEqual(['assistant-antigo']);
    expect(messages[0].message.content).toBe('resposta persistida');
    expect(messages[0].message.isStreaming).toBe(false);
    expect(messages[0].message.reasoning).toBeUndefined();
    emitEvent('chat:done', { conversationId: 'conversation-1', turnId: 'turn-antigo' });
    expect(sessions['conversation-1'].isLoading).toBe(true);
    emitEvent('chat:done', { conversationId: 'conversation-1', turnId: 'turn-novo' });
    expect(sessions['conversation-1'].isLoading).toBe(false);
  });
});
