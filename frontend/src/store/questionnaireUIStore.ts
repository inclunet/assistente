import { create } from 'zustand';
import type { QuestionnairePayload } from '../components/ui/QuestionnaireDialog';
import { DECISION_REPEAT_TRIGGER, DECISION_RESPOND_COMMAND_ID, type DialogCommandScope } from '../lib/commandBridge';

export type QuestionnaireUIResult = {
  answers: Record<string, unknown>;
  cancelled: boolean;
};

type PendingItem = {
  data: QuestionnairePayload;
  scope: DialogCommandScope | null;
  resolve: (result: QuestionnaireUIResult) => void;
};

interface QuestionnaireUIState {
  active: QuestionnairePayload | null;
  activeScope: DialogCommandScope | null;
  queue: PendingItem[];
  _activeResolve: ((result: QuestionnaireUIResult) => void) | null;

  request: (data: QuestionnairePayload, scope?: DialogCommandScope) => Promise<QuestionnaireUIResult>;
  submit: (answers: Record<string, unknown>) => void;
  cancel: (answers?: Record<string, unknown>) => void;
  /** Remove e resolve somente o item que tem este id, ativo ou enfileirado. */
  cancelById: (id: string, answers?: Record<string, unknown>) => boolean;
}

export const useQuestionnaireUIStore = create<QuestionnaireUIState>((set, get) => ({
  active: null,
  activeScope: null,
  queue: [],
  _activeResolve: null,

  request: (data, scope) => {
    if (!data || typeof data.id !== 'string' || data.id.length === 0 || data.id.trim() !== data.id) {
      return Promise.reject(new Error('questionnaire id is invalid'));
    }

    // O escopo é uma restrição da UI, nunca autoridade do backend. Não guardar
    // referências mutáveis nem associar o escopo de outro diálogo a este item.
    if (scope && (data.kind !== 'decision' || scope.dialogId !== data.id || scope.kind !== 'decision' ||
      !/^[1-9][0-9]*$/.test(scope.generation) ||
      !Array.isArray(scope.allowedCommandIds) || !Array.isArray(scope.allowedTriggerSpecs) ||
      scope.allowedCommandIds.length !== 1 || scope.allowedCommandIds[0] !== DECISION_RESPOND_COMMAND_ID ||
      scope.allowedTriggerSpecs.length !== 1 || scope.allowedTriggerSpecs[0] !== DECISION_REPEAT_TRIGGER)) {
      return Promise.reject(new Error('questionnaire command scope is invalid'));
    }
    const stableScope: DialogCommandScope | null = scope ? Object.freeze({
      dialogId: scope.dialogId, kind: 'decision', generation: scope.generation,
      allowedCommandIds: Object.freeze([DECISION_RESPOND_COMMAND_ID] as const),
      allowedTriggerSpecs: Object.freeze([DECISION_REPEAT_TRIGGER] as const),
    }) : null;
    const state = get();
    if (state.active?.id === data.id || state.queue.some((item) => item.data.id === data.id)) {
      return Promise.reject(new Error('questionnaire id is already pending'));
    }
    return new Promise<QuestionnaireUIResult>((resolve) => {
      if (state.active) {
        set((s) => ({
          queue: [...s.queue, { data, scope: stableScope, resolve }],
        }));
        return;
      }

      set({ active: data, activeScope: stableScope, _activeResolve: resolve });
    });
  },

  submit: (answers) => {
    const state = get();
    const resolve = state._activeResolve;
    if (resolve) resolve({ answers, cancelled: false });

    const next = state.queue[0];
    const rest = state.queue.slice(1);
    if (next) {
      set({ active: next.data, activeScope: next.scope, queue: rest, _activeResolve: next.resolve });
    } else {
      set({ active: null, activeScope: null, queue: [], _activeResolve: null });
    }
  },

  cancel: (answers) => {
    const state = get();
    const resolve = state._activeResolve;
    if (resolve) resolve({ answers: answers ?? {}, cancelled: true });

    const next = state.queue[0];
    const rest = state.queue.slice(1);
    if (next) {
      set({ active: next.data, activeScope: next.scope, queue: rest, _activeResolve: next.resolve });
    } else {
      set({ active: null, activeScope: null, queue: [], _activeResolve: null });
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
