import { describe, expect, it, vi } from 'vitest';
import { createConversationTurnQueue } from './chatTurnQueue';

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

  it('permite limpar a marca de fila para cancelamento explicito', () => {
    const queue = createConversationTurnQueue();
    const task = vi.fn(() => new Promise<void>(() => undefined));

    void queue.enqueue('conversation-1', task);
    expect(queue.isQueued('conversation-1')).toBe(true);

    queue.clear('conversation-1');
    expect(queue.isQueued('conversation-1')).toBe(false);
  });
});
