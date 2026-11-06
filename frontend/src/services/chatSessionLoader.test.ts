import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  loadConversationSnapshot,
} from './chatSessionLoader';
import { canonicalTestFixture, canonicalTestItem } from './chatTimelineTestFixture';
import type { TimelineSnapshot } from './timelineConversationState';

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

describe('chatSessionLoader', () => {
  let fixture: ReturnType<typeof canonicalTestFixture>;
  const page = (ids: string[], overrides: Partial<TimelineSnapshot> = {}): TimelineSnapshot => ({
    conversationId: 'conversation-1', scopeParentId: null, windowId: 'surface', epoch: 'epoch', windowRevision: '2', sequence: '1',
    mode: 'historical', items: ids.map(id => canonicalTestItem(id, id, { conversationId: 'conversation-1' })), orderedItemIds: ids,
    retainedItemIds: [], retentionBlocked: false, totalCount: ids.length, startIndex: 0, endIndex: ids.length - 1,
    hasBefore: false, hasAfter: false, beforeCursor: 'opaque-before', afterCursor: 'opaque-after', ...overrides,
  });
  beforeEach(() => {
    fixture = canonicalTestFixture();
    mockGetConversationInfo.mockReset();
    mockGetConversationMessageWindow.mockReset();
    mockGetMessageChildren.mockReset();
  });
  afterEach(async () => { await fixture.client.dispose(); });

  it('carrega snapshot recente e marca janela anterior', async () => {
    mockGetConversationInfo.mockResolvedValue({ title: 'Conversa', channel: 'telegram', contact_id: 'contact-1' });
    const snapshot = page(['visible-1', 'visible-2'], {
      totalCount: 3,
      startIndex: 1,
      endIndex: 2,
      hasBefore: true,
    });
    vi.mocked(fixture.port.open).mockResolvedValue(snapshot);
    vi.mocked(fixture.port.navigate).mockResolvedValue(snapshot);

    const metadata = await loadConversationSnapshot('conversation-1');
    const presentation = fixture.presentation('surface', 'conversation-1');
    await presentation.open();

    expect(mockGetConversationMessageWindow).not.toHaveBeenCalled();
    expect(fixture.port.open).toHaveBeenCalledWith('conversation-1', 'surface', null,
      { cursor: '', anchor: 'end', anchorItemId: '', direction: 'before', limit: 80 });
    expect(metadata).toMatchObject({
      title: 'Conversa',
      channel: 'telegram',
      contactId: 'contact-1',
    });
    expect(presentation.window?.metadata.hasBefore).toBe(true);
    expect(presentation.nodes().map((item) => item.message.id)).toEqual(['visible-1', 'visible-2']);
    expect(presentation.nodes()[0].originalIndex).toBe(1);
    expect(presentation.window?.metadata.totalCount).toBe(3);
  });

  it('usa i18n quando snapshot não tem título', async () => {
    mockGetConversationInfo.mockResolvedValue({ title: '' });
    mockGetConversationMessageWindow.mockRejectedValue(new Error("API legada não deve ser chamada"));

    const snapshot = await loadConversationSnapshot('conversation-1');

    expect(snapshot.title).toBe('Conversation fallback');
  });

  it('carrega mensagens anteriores antes do cursor', async () => {
    const presentation = fixture.presentation('surface', 'conversation-1');
    await presentation.open();
    vi.mocked(fixture.port.navigate).mockResolvedValueOnce(page(['older'], {
      totalCount: 3,
      startIndex: 0,
      endIndex: 0,
      hasBefore: false,
      hasAfter: true,
    }));

    await presentation.navigate('older');

    expect(fixture.port.navigate).toHaveBeenLastCalledWith('surface', 'epoch', '1', 'historical',
      { cursor: 'before', anchor: '', anchorItemId: '', direction: 'before', limit: 80 });
    expect(presentation.window?.metadata.hasBefore).toBe(false);
    expect(presentation.window?.metadata.hasAfter).toBe(true);
    expect(presentation.nodes().map((item) => item.message.id)).toEqual(['older']);
  });

  it('carrega mensagens posteriores depois do cursor', async () => {
    const presentation = fixture.presentation('surface', 'conversation-1');
    await presentation.open();
    vi.mocked(fixture.port.navigate).mockResolvedValueOnce(page(['newer'], {
      totalCount: 3,
      startIndex: 2,
      endIndex: 2,
      hasBefore: true,
      hasAfter: false,
    }));

    await presentation.navigate('newer');

    expect(fixture.port.navigate).toHaveBeenLastCalledWith('surface', 'epoch', '1', 'historical',
      { cursor: 'after', anchor: '', anchorItemId: '', direction: 'after', limit: 80 });
    expect(presentation.window?.metadata.hasBefore).toBe(true);
    expect(presentation.window?.metadata.hasAfter).toBe(false);
    expect(presentation.nodes()[0].originalIndex).toBe(2);
  });

  it('não expande fontes do turno em índices locais: usa o item consolidado pelo backend', async () => {
    const presentation = fixture.presentation('surface', 'conversation-1');
    await presentation.open();
    const response = canonicalTestItem('response', 'conclusão', {
      conversationId: 'conversation-1', messageIds: ['expanded-before', 'raw-window'],
      segments: [{ segmentId: 'intermediate', kind: 'text', text: 'intermediário' },
        { segmentId: 'text', kind: 'text', text: 'conclusão' }],
    });
    vi.mocked(fixture.port.navigate).mockResolvedValueOnce(page(['response'], {
      items: [response],
      totalCount: 10,
      startIndex: 4,
      endIndex: 4,
      hasBefore: true,
      hasAfter: true,
    }));

    await presentation.navigate('newer');

    expect(presentation.nodes()).toHaveLength(1);
    expect(presentation.nodes()[0].originalIndex).toBe(4);
    expect(presentation.nodes()[0].message.turnSegments?.map(segment => segment.content)).toEqual(['intermediário', 'conclusão']);
    expect(presentation.window?.metadata).toMatchObject({ totalCount: 10, startIndex: 4, endIndex: 4 });
  });

  it('recarrega snapshot sem metadados da conversa', async () => {
    const presentation = fixture.presentation('surface', 'conversation-1');
    await presentation.open();
    vi.mocked(fixture.port.refresh).mockResolvedValueOnce(page(['message-1']));

    await presentation.refresh();

    expect(mockGetConversationInfo).not.toHaveBeenCalled();
    expect(presentation.window?.metadata.hasBefore).toBe(false);
    expect(presentation.nodes().map((item) => item.message.id)).toEqual(['message-1']);
  });

  it('carrega filhos de mensagem', async () => {
    const parent = canonicalTestItem('parent', 'pai', { conversationId: 'conversation-1' });
    parent.representativeMessageId = 'message-1';
    fixture.setItems([parent,
      canonicalTestItem('child-1', 'um', { conversationId: 'conversation-1', scopeParentId: 'message-1' }),
      canonicalTestItem('child-2', 'dois', { conversationId: 'conversation-1', scopeParentId: 'message-1' })]);
    const presentation = fixture.presentation('surface', 'conversation-1');
    await presentation.open();

    await presentation.openThread('message-1');
    const children = presentation.nodes('message-1');

    expect(mockGetMessageChildren).not.toHaveBeenCalled();
    expect(fixture.port.open).toHaveBeenLastCalledWith('conversation-1', 'surface:thread:message-1', 'message-1',
      { cursor: '', anchor: 'end', anchorItemId: '', direction: 'before', limit: 80 });
    expect(children.map((item) => item.message.id)).toEqual(['child-1', 'child-2']);
    expect(children.map((item) => item.originalIndex)).toEqual([0, 1]);
  });
});
