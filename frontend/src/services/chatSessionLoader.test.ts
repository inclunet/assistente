import { beforeEach, describe, expect, it, vi } from 'vitest';
import { main } from '../../wailsjs/go/models';
import {
  loadConversationSnapshot,
  loadMessageChildrenNodes,
  loadOlderConversationMessages,
  reloadConversationSnapshot,
} from './chatSessionLoader';

const mockGetConversationInfo = vi.fn();
const mockGetRecentMessages = vi.fn();
const mockGetMessagesBefore = vi.fn();
const mockGetMessageChildren = vi.fn();

vi.mock('@wailsjs/go/app/App', () => ({
  GetConversationInfo: (...args: unknown[]) => mockGetConversationInfo(...args),
  GetRecentMessages: (...args: unknown[]) => mockGetRecentMessages(...args),
  GetMessagesBefore: (...args: unknown[]) => mockGetMessagesBefore(...args),
  GetMessageChildren: (...args: unknown[]) => mockGetMessageChildren(...args),
}));

vi.mock('i18next', () => ({
  default: {
    t: (key: string) => (key === 'chat.conversation' ? 'Conversation fallback' : key),
  },
}));

const node = (id: string) => new main.MessageNode({
  message: new main.EnrichedMessage({ id, role: 'user', content: id }),
  children: [],
  level: 0,
  childCount: 0,
});

describe('chatSessionLoader', () => {
  beforeEach(() => {
    mockGetConversationInfo.mockReset();
    mockGetRecentMessages.mockReset();
    mockGetMessagesBefore.mockReset();
    mockGetMessageChildren.mockReset();
  });

  it('carrega snapshot recente e marca janela anterior', async () => {
    mockGetConversationInfo.mockResolvedValue({ title: 'Conversa', channel: 'telegram', contact_id: 'contact-1' });
    mockGetRecentMessages.mockResolvedValue([node('older'), node('visible-1'), node('visible-2')]);

    const snapshot = await loadConversationSnapshot('conversation-1', 2);

    expect(mockGetRecentMessages).toHaveBeenCalledWith('conversation-1', 3);
    expect(snapshot).toMatchObject({
      title: 'Conversa',
      channel: 'telegram',
      contactId: 'contact-1',
      hasOlderMessages: true,
    });
    expect(snapshot.threadedMessages.map((item) => item.message.id)).toEqual(['visible-1', 'visible-2']);
    expect(snapshot.threadedMessages[0].originalIndex).toBe(0);
  });

  it('usa i18n quando snapshot não tem título', async () => {
    mockGetConversationInfo.mockResolvedValue({ title: '' });
    mockGetRecentMessages.mockResolvedValue([]);

    const snapshot = await loadConversationSnapshot('conversation-1', 2);

    expect(snapshot.title).toBe('Conversation fallback');
  });

  it('carrega mensagens anteriores antes do cursor', async () => {
    mockGetMessagesBefore.mockResolvedValue([node('older'), node('visible')]);

    const result = await loadOlderConversationMessages('conversation-1', 'cursor', 1);

    expect(mockGetMessagesBefore).toHaveBeenCalledWith('conversation-1', 'cursor', 2);
    expect(result.hasOlderMessages).toBe(true);
    expect(result.nodes.map((item) => item.message.id)).toEqual(['visible']);
  });

  it('recarrega snapshot sem metadados da conversa', async () => {
    mockGetRecentMessages.mockResolvedValue([node('message-1')]);

    const result = await reloadConversationSnapshot('conversation-1', 2);

    expect(result.hasOlderMessages).toBe(false);
    expect(result.threadedMessages.map((item) => item.message.id)).toEqual(['message-1']);
  });

  it('carrega filhos de mensagem', async () => {
    mockGetMessageChildren.mockResolvedValue([node('child-1')]);

    const children = await loadMessageChildrenNodes('message-1');

    expect(mockGetMessageChildren).toHaveBeenCalledWith('message-1');
    expect(children.map((item) => item.message.id)).toEqual(['child-1']);
  });
});
