export interface ConversationTurnQueue {
  enqueue: <T>(conversationId: string, task: () => Promise<T>) => Promise<T>;
  clear: (conversationId: string) => void;
  isQueued: (conversationId: string) => boolean;
}

interface ConversationQueueState {
  generation: number;
  tail: Promise<unknown>;
}

export class ConversationTurnQueueClearedError extends Error {
  constructor(conversationId: string) {
    super(`Conversation turn queue cleared for conversation "${conversationId}"`);
    this.name = 'ConversationTurnQueueClearedError';
  }
}

export function isConversationTurnQueueClearedError(error: unknown): error is ConversationTurnQueueClearedError {
  return error instanceof ConversationTurnQueueClearedError;
}

export function createConversationTurnQueue(): ConversationTurnQueue {
  const tails = new Map<string, ConversationQueueState>();

  const enqueue = async <T,>(conversationId: string, task: () => Promise<T>): Promise<T> => {
    let state = tails.get(conversationId);
    if (!state) {
      state = { generation: 0, tail: Promise.resolve() };
      tails.set(conversationId, state);
    }

    const generation = state.generation;
    const previous = state.tail;
    const run = previous
      .catch(() => undefined)
      .then(() => {
        if (state?.generation !== generation) {
          throw new ConversationTurnQueueClearedError(conversationId);
        }
        return task();
      });

    const tail = run.finally(() => {
      if (tails.get(conversationId) === state && state.tail === tail && state.generation === generation) {
        tails.delete(conversationId);
      }
    });
    state.tail = tail;

    return run;
  };

  return {
    enqueue,
    clear: (conversationId) => {
      const state = tails.get(conversationId);
      if (state) {
        state.generation += 1;
        const generation = state.generation;
        const tail = state.tail
          .catch(() => undefined)
          .finally(() => {
            if (tails.get(conversationId) === state && state.tail === tail && state.generation === generation) {
              tails.delete(conversationId);
            }
          });
        state.tail = tail;
      }
    },
    isQueued: (conversationId) => tails.has(conversationId),
  };
}
