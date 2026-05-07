import { describe, expect, it } from 'vitest';
import { main } from '../../wailsjs/go/models';
import {
  appendInternalMessageToTree,
  attachChildrenToMessage,
  finalizeStreamingNode,
  flattenThreadedMessages,
  hasMessageId,
  updateMessageContentInTree,
  type Message,
  type MessageNode,
} from './chatMessageTree';

const message = (id: string, role: string, content = id, parentId?: string): Message => new main.EnrichedMessage({
  id,
  role,
  content,
  parentId,
  conversationId: 'conversation-1',
  createdAt: '2026-04-30T00:00:00.000Z',
}) as Message;

const node = (msg: Message, children: MessageNode[] = [], level = 0): MessageNode => new main.MessageNode({
  message: msg,
  children,
  level,
  childCount: children.length,
}) as MessageNode;

describe('chatMessageTree', () => {
  it('flattens and searches threaded messages recursively', () => {
    const nodes = [
      node(message('root', 'user'), [
        node(message('child', 'assistant'), [
          node(message('grandchild', 'tool')),
        ], 1),
      ]),
    ];

    expect(flattenThreadedMessages(nodes).map((item) => item.id)).toEqual(['root', 'child', 'grandchild']);
    expect(hasMessageId(nodes, 'grandchild')).toBe(true);
    expect(hasMessageId(nodes, 'missing')).toBe(false);
  });

  it('updates message content inside nested nodes', () => {
    const nodes = [node(message('root', 'user'), [node(message('child', 'assistant', 'old'), [], 1)])];

    const updated = updateMessageContentInTree(nodes, 'child', 'new');

    expect(updated[0].children?.[0].message.content).toBe('new');
  });

  it('appends internal messages below their parent', () => {
    const nodes = [node(message('parent', 'user'))];
    const child = message('child', 'assistant', 'reply', 'parent');

    const updated = appendInternalMessageToTree(nodes, child);

    expect(updated[0].children?.[0].message.id).toBe('child');
    expect(updated[0].childCount).toBe(1);
  });

  it('attaches loaded children to the requested message', () => {
    const nodes = [node(message('root', 'user'), [node(message('parent', 'assistant'), [], 1)])];
    const loadedChildren = [node(message('loaded', 'tool'), [], 2)];

    const updated = attachChildrenToMessage(nodes, 'parent', loadedChildren);

    expect(updated[0].children?.[0].children?.[0].message.id).toBe('loaded');
  });

  it('replaces synthetic streaming id with backend id', () => {
    const conversation = {
      id: 'conversation-1',
      title: 'Conversation',
      threadedMessages: [node(message('synthetic', 'assistant'))],
    };
    conversation.threadedMessages[0].message.isStreaming = true;

    const updated = finalizeStreamingNode(conversation, 'synthetic', 'backend');

    expect(updated.threadedMessages[0].message.id).toBe('backend');
    expect(updated.threadedMessages[0].message.isStreaming).toBe(false);
  });

  it('preserves backend turn id when finalizing a streaming node', () => {
    const conversation = {
      id: 'conversation-1',
      title: 'Conversation',
      threadedMessages: [node(message('synthetic', 'assistant'))],
    };
    conversation.threadedMessages[0].message.isStreaming = true;

    const updated = finalizeStreamingNode(conversation, 'synthetic', 'backend', 'turn-1');

    expect(updated.threadedMessages[0].message.id).toBe('backend');
    expect(updated.threadedMessages[0].message.turnId).toBe('turn-1');
    expect(updated.threadedMessages[0].message.isStreaming).toBe(false);
  });
});
