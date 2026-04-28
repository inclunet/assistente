import { describe, expect, it } from 'vitest';
import { extractMessagesFromThreads, isAgentMessage } from './chatUtils';
import type { Message, MessageNode } from '../store/chatStore';
import { main } from '../../wailsjs/go/models';

describe('chatUtils', () => {
  it('extrai mensagens de threads', () => {
    const message1 = new main.EnrichedMessage({
      id: '1',
      conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
      role: 'user',
      content: 'Oi',
      createdAt: new Date().toISOString(),
      timestamp: Date.now(),
      isStreaming: false,
      internal: false,
    }) as Message;

    const message2 = new main.EnrichedMessage({
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
      main.MessageNode.createFrom({
        message: message1,
        childCount: 1,
        level: 0,
        children: [
          main.MessageNode.createFrom({
            message: message2,
            childCount: 0,
            level: 1,
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
    const message = new main.EnrichedMessage({
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
