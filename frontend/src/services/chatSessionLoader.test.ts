import { beforeEach, describe, expect, it, vi } from 'vitest';
import { chat } from '../../wailsjs/go/models';
import {
  loadConversationSnapshot,
  loadMessageChildrenNodes,
  loadOlderConversationMessages,
  loadNewerConversationMessages,
  reloadConversationSnapshot,
} from './chatSessionLoader';

const mockGetConversationInfo = vi.fn();
const mockGetConversationMessageWindow = vi.fn();
const mockGetMessageChildren = vi.fn();

vi.mock('@wailsjs/go/wailsapi/Conversations', () => ({
  GetConversationInfo: (...args: unknown[]) => mockGetConversationInfo(...args),
  GetConversationMessageWindow: (...args: unknown[]) => mockGetConversationMessageWindow(...args),
  GetMessageChildren: (...args: unknown[]) => mockGetMessageChildren(...args),
}));

vi.mock('i18next', () => ({
  default: {
    t: (key: string) => (key === 'chat.conversation' ? 'Conversation fallback' : key),
  },
}));

const node = (id: string) => new chat.MessageNode({
  message: new chat.EnrichedMessage({ id, role: 'user', content: id }),
  children: [],
  level: 0,
  childCount: 0,
});

const windowResult = (ids: string[], overrides: Partial<chat.MessageWindow> = {}) => chat.MessageWindow.createFrom({
  scope: 'conversation',
  conversationId: 'conversation-1',
  nodes: ids.map(node),
  totalCount: ids.length,
  startIndex: 0,
  endIndex: ids.length - 1,
  hasBefore: false,
  hasAfter: false,
  ...overrides,
});

describe('chatSessionLoader', () => {
  beforeEach(() => {
    mockGetConversationInfo.mockReset();
    mockGetConversationMessageWindow.mockReset();
    mockGetMessageChildren.mockReset();
  });

  it('carrega snapshot recente e marca janela anterior', async () => {
    mockGetConversationInfo.mockResolvedValue({ title: 'Conversa', channel: 'telegram', contact_id: 'contact-1' });
    mockGetConversationMessageWindow.mockResolvedValue(windowResult(['visible-1', 'visible-2'], {
      totalCount: 3,
      startIndex: 1,
      endIndex: 2,
      hasBefore: true,
    }));

    const snapshot = await loadConversationSnapshot('conversation-1', 2);

    expect(mockGetConversationMessageWindow).toHaveBeenCalledWith({
      scope: 'conversation',
      conversationId: 'conversation-1',
      anchor: 'end',
      anchorMessageId: undefined,
      direction: 'before',
      limit: 2,
    });
    expect(snapshot).toMatchObject({
      title: 'Conversa',
      channel: 'telegram',
      contactId: 'contact-1',
      hasOlderMessages: true,
    });
    expect(snapshot.threadedMessages.map((item) => item.message.id)).toEqual(['visible-1', 'visible-2']);
    expect(snapshot.threadedMessages[0].originalIndex).toBe(1);
    expect(snapshot.messageWindow.totalCount).toBe(3);
  });

  it('usa i18n quando snapshot não tem título', async () => {
    mockGetConversationInfo.mockResolvedValue({ title: '' });
    mockGetConversationMessageWindow.mockResolvedValue(windowResult([]));

    const snapshot = await loadConversationSnapshot('conversation-1', 2);

    expect(snapshot.title).toBe('Conversation fallback');
  });

  it('carrega mensagens anteriores antes do cursor', async () => {
    mockGetConversationMessageWindow.mockResolvedValue(windowResult(['older'], {
      totalCount: 3,
      startIndex: 0,
      endIndex: 0,
      hasBefore: false,
      hasAfter: true,
    }));

    const result = await loadOlderConversationMessages('conversation-1', 'cursor', 1);

    expect(mockGetConversationMessageWindow).toHaveBeenCalledWith({
      scope: 'conversation',
      conversationId: 'conversation-1',
      anchor: undefined,
      anchorMessageId: 'cursor',
      direction: 'before',
      limit: 1,
    });
    expect(result.hasOlderMessages).toBe(false);
    expect(result.hasNewerMessages).toBe(true);
    expect(result.nodes.map((item) => item.message.id)).toEqual(['older']);
  });

  it('carrega mensagens posteriores depois do cursor', async () => {
    mockGetConversationMessageWindow.mockResolvedValue(windowResult(['newer'], {
      totalCount: 3,
      startIndex: 2,
      endIndex: 2,
      hasBefore: true,
      hasAfter: false,
    }));

    const result = await loadNewerConversationMessages('conversation-1', 'cursor', 1);

    expect(mockGetConversationMessageWindow).toHaveBeenCalledWith({
      scope: 'conversation',
      conversationId: 'conversation-1',
      anchor: undefined,
      anchorMessageId: 'cursor',
      direction: 'after',
      limit: 1,
    });
    expect(result.hasOlderMessages).toBe(true);
    expect(result.hasNewerMessages).toBe(false);
    expect(result.nodes[0].originalIndex).toBe(2);
  });

  it('não inventa índices absolutos quando a janela vem expandida por turno', async () => {
    const rawWindowNode = node('raw-window');
    rawWindowNode.originalIndex = 4;
    mockGetConversationMessageWindow.mockResolvedValue(chat.MessageWindow.createFrom({
      scope: 'conversation',
      conversationId: 'conversation-1',
      nodes: [node('expanded-before'), rawWindowNode],
      totalCount: 10,
      startIndex: 4,
      endIndex: 4,
      hasBefore: true,
      hasAfter: true,
    }));

    const result = await loadNewerConversationMessages('conversation-1', 'cursor', 1);

    expect(result.nodes[0].originalIndex).toBeUndefined();
    expect(result.nodes[1].originalIndex).toBe(4);
  });

  it('recarrega snapshot sem metadados da conversa', async () => {
    mockGetConversationMessageWindow.mockResolvedValue(windowResult(['message-1']));

    const result = await reloadConversationSnapshot('conversation-1', 2);

    expect(result.hasOlderMessages).toBe(false);
    expect(result.threadedMessages.map((item) => item.message.id)).toEqual(['message-1']);
  });

  it('carrega filhos de mensagem', async () => {
    mockGetMessageChildren.mockResolvedValue([node('child-1'), node('child-2')]);

    const children = await loadMessageChildrenNodes('message-1');

    expect(mockGetMessageChildren).toHaveBeenCalledWith('message-1');
    expect(children.map((item) => item.message.id)).toEqual(['child-1', 'child-2']);
    expect(children.map((item) => item.originalIndex)).toEqual([0, 1]);
  });
});
