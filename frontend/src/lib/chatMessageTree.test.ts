import { describe, expect, it } from 'vitest';
import { chat } from '../../wailsjs/go/models';
import {
  attachChildrenToMessage,
  finalizeStreamingNode,
  getMessageTurnSegments,
  flattenThreadedMessages,
  hasMessageId,
  updateMessageContentInTree,
  updateEditedMessageContentInTree,
  updateMessageSegmentsInTree,
  type Message,
  type MessageNode,
} from './chatMessageTree';

const message = (id: string, role: string, content = id, parentId?: string): Message => new chat.EnrichedMessage({
  id,
  role,
  content,
  parentId,
  conversationId: 'conversation-1',
  createdAt: '2026-04-30T00:00:00.000Z',
}) as Message;

const node = (msg: Message, children: MessageNode[] = [], level = 0): MessageNode => new chat.MessageNode({
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
    const untouched = node(message('untouched', 'assistant'));
    const sibling = node(message('sibling', 'user'));
    const nodes = [
      node(message('root', 'user'), [
        node(message('child', 'assistant', 'old'), [], 1),
        untouched,
      ]),
      sibling,
    ];
    const untouchedInTree = nodes[0].children?.[1];
    const originalChildMessage = nodes[0].children?.[0].message;

    const updated = updateMessageContentInTree(nodes, 'child', 'new');

    expect(updated[0].children?.[0].message.content).toBe('new');
    expect(updated[0].children?.[0].message).not.toBe(originalChildMessage);
    expect(updated[0].children?.[1]).toBe(untouchedInTree);
    expect(updated[1]).toBe(sibling);
    expect(updateMessageContentInTree(nodes, 'missing', 'new')).toBe(nodes);
  });

  it('invalidates received segments only for authoritative edits, preserving children and streaming updates', () => {
    const nodes = [node(message('root', 'assistant', 'old'), [node(message('child', 'tool'))])];
    const original = nodes[0].message as Message;
    original._turnSegments = [{ type: 'text', content: 'old' }];
    original._resumableContent = 'old';
    const streamed = updateMessageContentInTree(nodes, 'root', 'delta');
    expect((streamed[0].message as Message)._turnSegments).toBe(original._turnSegments);
    expect((streamed[0].message as Message)._resumableContent).toBe('old');
    const edited = updateEditedMessageContentInTree(streamed, 'root', 'edited');
    expect(edited[0].message.content).toBe('edited');
    expect((edited[0].message as Message)._turnSegments).toEqual([{ type: 'text', content: 'edited' }]);
    expect((edited[0].message as Message)._resumableContent).toBeUndefined();
    expect(edited[0].children).toBe(nodes[0].children);
    expect(updateEditedMessageContentInTree(edited, 'root', 'edited')).toBe(edited);
  });

  it('preserves canonical segments while an empty retry override hides them until an edit removes it', () => {
    const nodes = [node(message('root', 'assistant', 'Parcial'))];
    nodes[0].message.turnSegments = [new chat.TurnSegment({ type: 'text', content: 'Parcial' })];
    const received = updateMessageSegmentsInTree(nodes, 'root', [], '');
    expect((received[0].message as Message)._resumableContent).toBe('');
    const retry = updateMessageSegmentsInTree(received, 'root', []);
    expect((retry[0].message as Message)._resumableContent).toBeUndefined();
    expect(getMessageTurnSegments(retry[0].message as Message)).toEqual([]);
    expect(retry[0].message.turnSegments?.[0].content).toBe('Parcial');
    const edited = updateEditedMessageContentInTree(retry, 'root', 'Parcial');
    expect((edited[0].message as Message)._turnSegments).toEqual([{ type: 'text', content: 'Parcial' }]);
    expect(getMessageTurnSegments(edited[0].message as Message)?.[0].content).toBe('Parcial');
  });

  it('projects edited text even when canonical chronology contains no tools', () => {
    const nodes = [node(message('root', 'assistant', 'old'))];
    nodes[0].message.turnSegments = [new chat.TurnSegment({ type: 'text', content: 'old' })];
    const edited = updateEditedMessageContentInTree(nodes, 'root', 'new');
    expect(getMessageTurnSegments(edited[0].message as Message)).toEqual([{ type: 'text', content: 'new' }]);
    expect(edited[0].message.turnSegments?.[0].content).toBe('old');
  });

  it('attaches loaded children to the requested message', () => {
    const nodes = [node(message('root', 'user'), [node(message('parent', 'assistant'), [], 1)])];
    const parentMessage = nodes[0].children?.[0].message;
    const loadedChildren = [node(message('loaded', 'tool'), [], 2)];

    const updated = attachChildrenToMessage(nodes, 'parent', loadedChildren);

    expect(updated[0].children?.[0].children?.[0].message.id).toBe('loaded');
    expect(updated[0].message).toBe(nodes[0].message);
    expect(updated[0].children?.[0].message).toBe(parentMessage);
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
