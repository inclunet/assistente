import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  announceChatBackgroundResponseDone,
  announceForActiveChatConversation,
  getChatConversationLabel,
  isChatConversationActive,
  playChatReceiveSoundIfActive,
} from './chatArbitration';

const hoisted = vi.hoisted(() => ({
  workspaceState: {
    workspace: {
      id: 'workspace-1',
      name: 'Workspace',
      activeTabId: 'tab-1',
      tabs: [
        { id: 'tab-1', type: 'chat', conversationId: 'conversation-1', title: 'Aba ativa', position: 0 },
        { id: 'tab-2', type: 'chat', conversationId: 'conversation-2', title: 'Aba inativa', position: 1 },
      ],
    },
    getActiveTab: () => ({ id: 'tab-1', type: 'chat', conversationId: 'conversation-1', title: 'Aba ativa', position: 0 }),
  },
  announceWithOrigin: vi.fn(),
  playReceiveSound: vi.fn(),
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: {
    getState: () => hoisted.workspaceState,
  },
}));

vi.mock('./voiceAccessibility/announcerBroker', () => ({
  announceWithOrigin: (...args: unknown[]) => hoisted.announceWithOrigin(...args),
}));

vi.mock('./audioFeedback', () => ({
  playReceiveSound: (...args: unknown[]) => hoisted.playReceiveSound(...args),
}));

vi.mock('i18next', () => ({
  default: {
    t: (key: string, opts?: Record<string, unknown>) => {
      if (key === 'chat.announce.backgroundResponseDone') return `Terminou: ${opts?.title}`;
      if (key === 'chat.conversation') return opts?.defaultValue ?? 'Conversa';
      return key;
    },
  },
}));

describe('chatArbitration', () => {
  beforeEach(() => {
    hoisted.announceWithOrigin.mockClear();
    hoisted.playReceiveSound.mockClear();
    hoisted.workspaceState.workspace.activeTabId = 'tab-1';
    hoisted.workspaceState.getActiveTab = () => ({
      id: 'tab-1',
      type: 'chat',
      conversationId: 'conversation-1',
      title: 'Aba ativa',
      position: 0,
    });
  });

  it('identifica a conversa ativa pelo workspace', () => {
    expect(isChatConversationActive('conversation-1')).toBe(true);
    expect(isChatConversationActive('conversation-2')).toBe(false);
  });

  it('usa título da aba antes do fallback para label', () => {
    expect(getChatConversationLabel('conversation-2', 'Fallback')).toBe('Aba inativa');
    expect(getChatConversationLabel('conversation-3', 'Fallback')).toBe('Fallback');
  });

  it('encaminha anúncio com origem da conversa', () => {
    announceForActiveChatConversation('conversation-1', 'respondendo', 'polite');
    announceForActiveChatConversation('conversation-2', 'silencioso', 'polite');

    expect(hoisted.announceWithOrigin).toHaveBeenCalledTimes(2);
    expect(hoisted.announceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({
      message: 'respondendo',
      announcePriority: 'polite',
      origin: expect.objectContaining({ conversationId: 'conversation-1', tabId: 'tab-1' }),
      eventType: 'progress',
    }));
  });

  it('anuncia conclusão usando label resolvida e origem contextual', () => {
    announceChatBackgroundResponseDone('conversation-2', 'Fallback');
    announceChatBackgroundResponseDone('conversation-1', 'Ativa');

    expect(hoisted.announceWithOrigin).toHaveBeenCalledTimes(1);
    expect(hoisted.announceWithOrigin).toHaveBeenCalledWith(expect.objectContaining({
      message: 'Terminou: Aba inativa',
      announcePriority: 'polite',
      origin: expect.objectContaining({ conversationId: 'conversation-2', tabId: 'tab-2' }),
      eventType: 'completion',
    }));
  });

  it('toca som de recebimento somente para conversa ativa', () => {
    playChatReceiveSoundIfActive('conversation-2');
    playChatReceiveSoundIfActive('conversation-1');

    expect(hoisted.playReceiveSound).toHaveBeenCalledTimes(1);
  });
});
