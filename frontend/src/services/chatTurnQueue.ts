export interface ConversationTurnQueue {
  enqueue: <T>(conversationId: string, task: () => Promise<T>) => Promise<T>;
  clear: (conversationId: string) => void;
  isQueued: (conversationId: string) => boolean;
}

export function createConversationTurnQueue(): ConversationTurnQueue {
  const tails = new Map<string, Promise<unknown>>();

  const enqueue = async <T,>(conversationId: string, task: () => Promise<T>): Promise<T> => {
    const previous = tails.get(conversationId) ?? Promise.resolve();
    const run = previous
      .catch(() => undefined)
      .then(task);

    tails.set(conversationId, run.finally(() => {
      if (tails.get(conversationId) === run) {
        tails.delete(conversationId);
      }
    }));

    return run;
  };

  return {
    enqueue,
    clear: (conversationId) => {
      tails.delete(conversationId);
    },
    isQueued: (conversationId) => tails.has(conversationId),
  };
}
