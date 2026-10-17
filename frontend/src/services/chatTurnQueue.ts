export interface ConversationTurnQueue {
  enqueue: <T>(conversationId: string, task: () => Promise<T>, onCancel?: () => void, onStart?: () => void) => Promise<T>;
  registerPendingAdmission: (conversationId: string, onCancel: () => void) => { activate: () => boolean; complete: () => void; cancelled: Promise<never> };
  clear: (conversationId: string) => void;
  clearAll: () => void;
  isQueued: (conversationId: string) => boolean;
  hasPendingAdmission: (conversationId: string) => boolean;
  cancelNext: (conversationId: string) => boolean;
}

interface QueueEntry {
  started: boolean;
  cancelled: boolean;
  cancelable: boolean;
  pendingAdmission: boolean;
  reject: (error: unknown) => void;
  onCancel?: () => void;
  onStart?: () => void;
}

interface ConversationQueueState {
  generation: number;
  tail: Promise<unknown>;
  tailPending: boolean;
  entries: QueueEntry[];
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

export class ConversationTurnQueueCancelledError extends Error {
  constructor(conversationId: string) {
    super(`Queued conversation turn cancelled for conversation "${conversationId}"`);
    this.name = 'ConversationTurnQueueCancelledError';
  }
}

export function isConversationTurnQueueCancelledError(error: unknown): error is ConversationTurnQueueCancelledError {
  return error instanceof ConversationTurnQueueCancelledError;
}

export function createConversationTurnQueue(): ConversationTurnQueue {
  const tails = new Map<string, ConversationQueueState>();

  const enqueue = <T,>(conversationId: string, task: () => Promise<T>, onCancel?: () => void, onStart?: () => void): Promise<T> => {
    let state = tails.get(conversationId);
    if (!state) {
      state = { generation: 0, tail: Promise.resolve(), tailPending: false, entries: [] };
      tails.set(conversationId, state);
    }

    const generation = state.generation;
    const previous = state.tail;
    let resolveResult!: (value: T | PromiseLike<T>) => void;
    let rejectResult!: (error: unknown) => void;
    const result = new Promise<T>((resolve, reject) => {
      resolveResult = resolve;
      rejectResult = reject;
    });
    const entry: QueueEntry = {
      started: false,
      cancelled: false,
      cancelable: Boolean(onCancel),
      pendingAdmission: false,
      reject: rejectResult,
      onCancel,
      onStart,
    };
    state.entries.push(entry);

    const run = previous
      .catch(() => undefined)
      .then(() => {
        if (state?.generation !== generation) {
          throw new ConversationTurnQueueClearedError(conversationId);
        }
        if (entry.cancelled) throw new ConversationTurnQueueCancelledError(conversationId);
        entry.started = true;
        try {
          entry.onStart?.();
        } catch {
          // Uma falha no estado visual não pode impedir a execução do turno.
        }
        return task();
      });

    const tail = run.catch(() => undefined).finally(() => {
      const entryIndex = state?.entries.indexOf(entry) ?? -1;
      if (entryIndex >= 0) state?.entries.splice(entryIndex, 1);
      if (tails.get(conversationId) === state && state.tail === tail) {
        state.tailPending = false;
        if (state.generation === generation && state.entries.length === 0) tails.delete(conversationId);
      }
    });
    void run.then(resolveResult, rejectResult);
    // A cauda serve apenas à ordenação e pode ficar sem sucessor. Observar
    // sua rejeição evita um unhandled rejection; o caller ainda recebe a
    // rejeição original de run, sem transformar a operação em sucesso.
    void tail.catch(() => undefined);
    state.tail = tail;
    state.tailPending = true;

    return result;
  };

  const registerPendingAdmission = (conversationId: string, onCancel: () => void) => {
    let state = tails.get(conversationId);
    if (!state) {
      state = { generation: 0, tail: Promise.resolve(), tailPending: false, entries: [] };
      tails.set(conversationId, state);
    }
    let rejectCancellation!: (error: unknown) => void;
    const cancelled = new Promise<never>((_resolve, reject) => { rejectCancellation = reject; });
    void cancelled.catch(() => undefined);
    const entry: QueueEntry = {
      started: false,
      cancelled: false,
      cancelable: true,
      pendingAdmission: true,
      reject: rejectCancellation,
      onCancel,
    };
    state.entries.push(entry);
    let active = true;
    const remove = () => {
      if (!active) return false;
      active = false;
      const index = state?.entries.indexOf(entry) ?? -1;
      if (index >= 0) state?.entries.splice(index, 1);
      if (state && tails.get(conversationId) === state && state.entries.length === 0 && !state.tailPending) tails.delete(conversationId);
      return true;
    };
    return {
      activate: () => {
        if (entry.cancelled || entry.started || !active) return false;
        entry.started = true;
        return true;
      },
      complete: () => { remove(); },
      cancelled,
    };
  };

  const cancelNext = (conversationId: string): boolean => {
    const state = tails.get(conversationId);
    const cancelable = (candidate: QueueEntry) => candidate.cancelable && !candidate.started && !candidate.cancelled;
    const entry = state?.entries.find(candidate => cancelable(candidate) && !candidate.pendingAdmission) ??
      state?.entries.find(candidate => cancelable(candidate) && candidate.pendingAdmission);
    if (!entry) return false;
    entry.cancelled = true;
    try {
      entry.onCancel?.();
    } catch {
      // A falha ao atualizar a contagem visual não pode reativar o turno.
    } finally {
      entry.reject(new ConversationTurnQueueCancelledError(conversationId));
      const entryIndex = state?.entries.indexOf(entry) ?? -1;
      if (entryIndex >= 0) state?.entries.splice(entryIndex, 1);
      if (tails.get(conversationId) === state && state.entries.length === 0 && !state.tailPending) tails.delete(conversationId);
    }
    return true;
  };

  const clear = (conversationId: string) => {
    const state = tails.get(conversationId);
    if (state) {
      state.generation += 1;
      // Entradas pendentes desta geração continuam na cauda para receber o
      // erro de limpeza, mas deixam de participar do cancelamento individual.
      // Assim cancelNext só pode atingir envios aceitos depois do clear.
      const pendingAdmissions = state.entries.filter(entry => entry.pendingAdmission);
      for (const entry of pendingAdmissions) {
        if (!entry.started) {
          entry.cancelled = true;
          try { entry.onCancel?.(); } catch { /* invalidation must continue */ }
          entry.reject(new ConversationTurnQueueClearedError(conversationId));
        }
      }
      state.entries = state.entries.filter((entry) => entry.started && !entry.pendingAdmission);
      const generation = state.generation;
      const tail = state.tail
        .catch(() => undefined)
        .finally(() => {
          if (tails.get(conversationId) === state && state.tail === tail && state.generation === generation) {
            tails.delete(conversationId);
          }
        });
      state.tail = tail;
      state.tailPending = true;
      void tail.finally(() => {
        if (tails.get(conversationId) === state && state.tail === tail) {
          state.tailPending = false;
          if (state.entries.length === 0) tails.delete(conversationId);
        }
      });
    }
  };

  return {
    enqueue,
    registerPendingAdmission,
    clear,
    clearAll: () => {
      for (const conversationId of Array.from(tails.keys())) {
        clear(conversationId);
      }
    },
    isQueued: (conversationId) => Boolean(tails.get(conversationId)?.entries.some(entry => !entry.pendingAdmission)),
    hasPendingAdmission: (conversationId) => Boolean(tails.get(conversationId)?.entries.some(entry => entry.pendingAdmission && !entry.cancelled)),
    cancelNext,
  };
}
