import { create } from 'zustand';
import type { QuestionnairePayload } from '../components/ui/QuestionnaireDialog';

export type QuestionnaireUIResult = {
  answers: Record<string, unknown>;
  cancelled: boolean;
};

type PendingItem = {
  data: QuestionnairePayload;
  resolve: (result: QuestionnaireUIResult) => void;
};

interface QuestionnaireUIState {
  active: QuestionnairePayload | null;
  queue: PendingItem[];
  _activeResolve: ((result: QuestionnaireUIResult) => void) | null;

  request: (data: QuestionnairePayload) => Promise<QuestionnaireUIResult>;
  submit: (answers: Record<string, unknown>) => void;
  cancel: (answers?: Record<string, unknown>) => void;
  /** Remove e resolve somente o item que tem este id, ativo ou enfileirado. */
  cancelById: (id: string, answers?: Record<string, unknown>) => boolean;
}

export const useQuestionnaireUIStore = create<QuestionnaireUIState>((set, get) => ({
  active: null,
  queue: [],
  _activeResolve: null,

  request: (data) => {
    if (!data || typeof data.id !== 'string' || data.id.length === 0 || data.id.trim() !== data.id) {
      return Promise.reject(new Error('questionnaire id is invalid'));
    }
    const state = get();
    if (state.active?.id === data.id || state.queue.some((item) => item.data.id === data.id)) {
      return Promise.reject(new Error('questionnaire id is already pending'));
    }
    return new Promise<QuestionnaireUIResult>((resolve) => {
      if (state.active) {
        set((s) => ({
          queue: [...s.queue, { data, resolve }],
        }));
        return;
      }

      set({ active: data, _activeResolve: resolve });
    });
  },

  submit: (answers) => {
    const state = get();
    const resolve = state._activeResolve;
    if (resolve) resolve({ answers, cancelled: false });

    const next = state.queue[0];
    const rest = state.queue.slice(1);
    if (next) {
      set({ active: next.data, queue: rest, _activeResolve: next.resolve });
    } else {
      set({ active: null, queue: [], _activeResolve: null });
    }
  },

  cancel: (answers) => {
    const state = get();
    const resolve = state._activeResolve;
    if (resolve) resolve({ answers: answers ?? {}, cancelled: true });

    const next = state.queue[0];
    const rest = state.queue.slice(1);
    if (next) {
      set({ active: next.data, queue: rest, _activeResolve: next.resolve });
    } else {
      set({ active: null, queue: [], _activeResolve: null });
    }
  },

  cancelById: (id, answers) => {
    if (typeof id !== 'string' || id.length === 0 || id.trim() !== id) return false;
    const state = get();
    if (state.active?.id === id) {
      state.cancel(answers);
      return true;
    }

    const index = state.queue.findIndex((item) => item.data.id === id);
    if (index < 0) return false;

    const item = state.queue[index];
    item.resolve({ answers: answers ?? {}, cancelled: true });
    set({ queue: state.queue.filter((_, itemIndex) => itemIndex !== index) });
    return true;
  },
}));
