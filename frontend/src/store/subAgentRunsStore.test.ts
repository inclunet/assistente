/** @vitest-environment jsdom */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useSubAgentRunsStore, SUBAGENT_RUNS_PAGE_SIZE } from './subAgentRunsStore';

const mockListSubAgentRuns = vi.fn();
const mockCancelSubAgentRun = vi.fn();

vi.mock('@wailsjs/go/app/App', () => ({
  ListSubAgentRuns: (limit: number) => mockListSubAgentRuns(limit),
  CancelSubAgentRun: (conversationId: string, runId: string) =>
    mockCancelSubAgentRun(conversationId, runId),
}));

describe('subAgentRunsStore', () => {
  beforeEach(() => {
    mockListSubAgentRuns.mockReset();
    mockCancelSubAgentRun.mockReset();
    useSubAgentRunsStore.getState().reset();
  });

  it('carrega runs e a ocupação dos tetos de concorrência', async () => {
    mockListSubAgentRuns.mockResolvedValue({
      runs: [{ runId: 'run-1', conversationId: 'conv-1', status: 'running', active: true }],
      activeForUser: 1,
      activeGlobal: 3,
      maxConcurrentPerUser: 4,
      maxConcurrentGlobal: 16,
    });

    await useSubAgentRunsStore.getState().fetchRuns();

    const state = useSubAgentRunsStore.getState();
    expect(mockListSubAgentRuns).toHaveBeenCalledWith(SUBAGENT_RUNS_PAGE_SIZE);
    expect(state.runs).toHaveLength(1);
    expect(state.activeForUser).toBe(1);
    expect(state.activeGlobal).toBe(3);
    expect(state.maxConcurrentGlobal).toBe(16);
    expect(state.isLoading).toBe(false);
    expect(state.error).toBeNull();
  });

  it('registra o erro sem derrubar a lista já carregada', async () => {
    mockListSubAgentRuns.mockRejectedValue(new Error('sem sessão'));

    await useSubAgentRunsStore.getState().fetchRuns();

    const state = useSubAgentRunsStore.getState();
    expect(state.error).toContain('sem sessão');
    expect(state.isLoading).toBe(false);
  });

  it('delega o cancelamento ao backend sem alterar a lista otimisticamente', async () => {
    mockListSubAgentRuns.mockResolvedValue({
      runs: [{ runId: 'run-1', conversationId: 'conv-1', status: 'running', active: true }],
      activeForUser: 1,
      activeGlobal: 1,
      maxConcurrentPerUser: 4,
      maxConcurrentGlobal: 16,
    });
    mockCancelSubAgentRun.mockResolvedValue({ run_id: 'run-1', status: 'cancelled', cancelled: true });

    await useSubAgentRunsStore.getState().fetchRuns();
    const result = await useSubAgentRunsStore.getState().cancelRun('conv-1', 'run-1');

    expect(mockCancelSubAgentRun).toHaveBeenCalledWith('conv-1', 'run-1');
    expect(result.cancelled).toBe(true);
    // Quem manda no estado do run é o backend: a lista só muda no próximo fetch.
    expect(useSubAgentRunsStore.getState().runs[0].status).toBe('running');
  });
});
