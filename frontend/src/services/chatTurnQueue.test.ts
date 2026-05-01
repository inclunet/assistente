import { describe, expect, it } from 'vitest';
import {
  ConversationTurnQueueClearedError,
  createConversationTurnQueue,
} from './chatTurnQueue';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

async function flushMicrotasks() {
  await Promise.resolve();
  await Promise.resolve();
}

describe('chatTurnQueue', () => {
  it('serializa turnos da mesma conversa', async () => {
    const queue = createConversationTurnQueue();
    const first = deferred<void>();
    const calls: string[] = [];

    const firstRun = queue.enqueue('conversation-1', async () => {
      calls.push('first:start');
      await first.promise;
      calls.push('first:end');
    });
    const secondRun = queue.enqueue('conversation-1', async () => {
      calls.push('second:start');
    });

    await flushMicrotasks();
    expect(calls).toEqual(['first:start']);

    first.resolve();
    await firstRun;
    await secondRun;

    expect(calls).toEqual(['first:start', 'first:end', 'second:start']);
  });

  it('mantem conversas diferentes em paralelo', async () => {
    const queue = createConversationTurnQueue();
    const first = deferred<void>();
    const calls: string[] = [];

    const firstRun = queue.enqueue('conversation-1', async () => {
      calls.push('first:start');
      await first.promise;
    });
    const secondRun = queue.enqueue('conversation-2', async () => {
      calls.push('second:start');
    });

    await flushMicrotasks();
    expect(calls).toEqual(['first:start', 'second:start']);

    first.resolve();
    await Promise.all([firstRun, secondRun]);
  });

  it('continua a fila mesmo quando um turno falha', async () => {
    const queue = createConversationTurnQueue();
    const calls: string[] = [];

    const firstRun = queue.enqueue('conversation-1', async () => {
      calls.push('first:start');
      throw new Error('boom');
    });
    const secondRun = queue.enqueue('conversation-1', async () => {
      calls.push('second:start');
    });

    await expect(firstRun).rejects.toThrow('boom');
    await secondRun;

    expect(calls).toEqual(['first:start', 'second:start']);
  });

  it('invalida pendentes sem liberar paralelismo durante turno ativo', async () => {
    const queue = createConversationTurnQueue();
    const first = deferred<void>();
    const calls: string[] = [];

    const firstRun = queue.enqueue('conversation-1', async () => {
      calls.push('first:start');
      await first.promise;
      calls.push('first:end');
    });
    const secondRun = queue.enqueue('conversation-1', async () => {
      calls.push('second:start');
    });

    await flushMicrotasks();
    queue.clear('conversation-1');
    expect(queue.isQueued('conversation-1')).toBe(true);

    const thirdRun = queue.enqueue('conversation-1', async () => {
      calls.push('third:start');
    });

    await flushMicrotasks();
    expect(calls).toEqual(['first:start']);

    first.resolve();
    await firstRun;
    await expect(secondRun).rejects.toBeInstanceOf(ConversationTurnQueueClearedError);
    await thirdRun;

    expect(calls).toEqual(['first:start', 'first:end', 'third:start']);
    expect(queue.isQueued('conversation-1')).toBe(false);
  });

  it('limpa fila inexistente sem efeito colateral', () => {
    const queue = createConversationTurnQueue();
    queue.clear('conversation-1');
    expect(queue.isQueued('conversation-1')).toBe(false);
  });
});
