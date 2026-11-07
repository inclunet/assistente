import { messageFixture, nodeFixture } from '../test/chatPresentationFixture';
import { describe, expect, it } from 'vitest';
import { extractMessagesFromThreads, isAgentMessage } from './chatUtils';
import type { Message, MessageNode } from '../store/chatStore';

describe('chatUtils', () => {
  it('extrai mensagens de threads', () => {
    const message1 = messageFixture({
      id: '1',
      conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
      role: 'user',
      content: 'Oi',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
    }) as Message;

    const message2 = messageFixture({
      id: '2',
      conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
      role: 'assistant',
      content: 'Ola',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
    }) as Message;

    const threads: MessageNode[] = [
      nodeFixture({
        message: message1,
        childCount: 1,
        children: [
          nodeFixture({
            message: message2,
            childCount: 0,
            children: [],
          }),
        ],
      }) as MessageNode,
    ];

    const result = extractMessagesFromThreads(threads);

    expect(result).toHaveLength(2);
    expect(result[0].id).toBe('1');
    expect(result[1].id).toBe('2');
  });

  it('isAgentMessage retorna false', () => {
    const message = messageFixture({
      id: '3',
      conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
      role: 'assistant',
      content: 'Teste',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
    }) as Message;

    expect(isAgentMessage(message)).toBe(false);
  });
});
