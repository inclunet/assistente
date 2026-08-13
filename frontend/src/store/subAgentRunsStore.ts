import { create } from 'zustand';
import { CancelSubAgentRun, ListSubAgentRuns } from '@wailsjs/go/app/App';
import type { subagent } from '@wailsjs/go/models';

/**
 * Estado da superfície de runs de sub-agente (AEP-0068 F5): o que está rodando
 * em segundo plano, o que terminou há pouco e quanto dos tetos de concorrência
 * já está em uso.
 *
 * O estado é global de propósito. O contador de runs ativos aparece no
 * histórico mesmo com o painel fechado, e os eventos de run chegam a qualquer
 * momento — inclusive com a página de histórico desmontada.
 */
export const SUBAGENT_RUNS_PAGE_SIZE = 50;

interface SubAgentRunsState {
  runs: subagent.RunListItem[];
  activeForUser: number;
  activeGlobal: number;
  maxConcurrentPerUser: number;
  maxConcurrentGlobal: number;
  isLoading: boolean;
  error: string | null;

  fetchRuns: () => Promise<void>;
  cancelRun: (conversationId: string, runId: string) => Promise<subagent.CancelResult>;
  reset: () => void;
}

const INITIAL_STATE = {
  runs: [] as subagent.RunListItem[],
  activeForUser: 0,
  activeGlobal: 0,
  maxConcurrentPerUser: 0,
  maxConcurrentGlobal: 0,
  isLoading: false,
  error: null as string | null,
};

export const useSubAgentRunsStore = create<SubAgentRunsState>((set) => ({
  ...INITIAL_STATE,

  fetchRuns: async () => {
    set({ isLoading: true, error: null });
    try {
      const result = await ListSubAgentRuns(SUBAGENT_RUNS_PAGE_SIZE);
      set({
        runs: result?.runs ?? [],
        activeForUser: result?.activeForUser ?? 0,
        activeGlobal: result?.activeGlobal ?? 0,
        maxConcurrentPerUser: result?.maxConcurrentPerUser ?? 0,
        maxConcurrentGlobal: result?.maxConcurrentGlobal ?? 0,
        isLoading: false,
      });
    } catch (err) {
      set({ error: String(err), isLoading: false });
    }
  },

  // O cancelamento não atualiza a lista otimisticamente: quem manda no estado do
  // run é o backend, e o evento de conclusão chega logo depois com o desfecho
  // real (que pode ser um no-op, se o run já havia terminado).
  cancelRun: async (conversationId: string, runId: string) => {
    return CancelSubAgentRun(conversationId, runId);
  },

  reset: () => set({ ...INITIAL_STATE }),
}));
