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
}

export const useQuestionnaireUIStore = create<QuestionnaireUIState>((set, get) => ({
  active: null,
  queue: [],
  _activeResolve: null,

  request: (data) => {
    return new Promise<QuestionnaireUIResult>((resolve) => {
      const state = get();
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
}));
