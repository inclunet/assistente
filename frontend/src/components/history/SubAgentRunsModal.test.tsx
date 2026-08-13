/** @vitest-environment jsdom */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe } from '../../test/a11yAxe';
import { SubAgentRunsModal } from './SubAgentRunsModal';
import { useSubAgentRunsStore } from '../../store/subAgentRunsStore';

const mockListSubAgentRuns = vi.fn();
const mockCancelSubAgentRun = vi.fn();

vi.mock('@wailsjs/go/app/App', () => ({
  ListSubAgentRuns: (limit: number) => mockListSubAgentRuns(limit),
  CancelSubAgentRun: (conversationId: string, runId: string) =>
    mockCancelSubAgentRun(conversationId, runId),
}));

const announceSpy = vi.fn();
vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceSpy, announceRequest: vi.fn() }),
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: string | { defaultValue?: string; title?: string }) => {
      if (typeof options === 'string') return options;
      if (options?.defaultValue) {
        return options.defaultValue.replace('{{title}}', options.title ?? '');
      }
      if (options?.title) return `${key}:${options.title}`;
      return key;
    },
    i18n: { language: 'pt-BR' },
  }),
}));

vi.mock('../../lib/dateUtils', () => ({
  formatRelativeTimeLocalized: () => 'há 5 min',
}));

vi.mock('../../store/authStore', () => ({
  useAuthStore: (selector: (s: { isAuthenticated: boolean }) => unknown) =>
    selector({ isAuthenticated: true }),
}));

function run(overrides: Record<string, unknown> = {}) {
  return {
    runId: 'run-1',
    conversationId: 'conv-1',
    parentConversationId: 'parent-1',
    title: 'Revisar PR',
    status: 'running',
    background: true,
    active: true,
    createdAt: '2026-08-13T12:00:00Z',
    startedAt: '2026-08-13T12:00:00Z',
    ...overrides,
  };
}

function listResult(runs: Array<Record<string, unknown>>) {
  return {
    runs,
    activeForUser: runs.filter((r) => r.active).length,
    activeGlobal: runs.filter((r) => r.active).length,
    maxConcurrentPerUser: 4,
    maxConcurrentGlobal: 16,
  };
}

describe('SubAgentRunsModal', () => {
  beforeEach(() => {
    mockListSubAgentRuns.mockReset();
    mockCancelSubAgentRun.mockReset();
    announceSpy.mockClear();
    useSubAgentRunsStore.getState().reset();
    mockListSubAgentRuns.mockResolvedValue(listResult([run()]));
    mockCancelSubAgentRun.mockResolvedValue({
      conversation_id: 'conv-1',
      run_id: 'run-1',
      status: 'cancelled',
      cancelled: true,
    });
  });

  it('lista os runs ao abrir e oferece cancelar com rótulo acessível', async () => {
    render(<SubAgentRunsModal isOpen onClose={vi.fn()} />);

    await waitFor(() => expect(mockListSubAgentRuns).toHaveBeenCalled());
    expect(await screen.findByText('Revisar PR')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Cancelar run Revisar PR' })).toBeTruthy();
  });

  it('não carrega nada enquanto está fechado', () => {
    render(<SubAgentRunsModal isOpen={false} onClose={vi.fn()} />);
    expect(mockListSubAgentRuns).not.toHaveBeenCalled();
  });

  it('cancela o run pelo backend e anuncia o resultado', async () => {
    const user = userEvent.setup();
    render(<SubAgentRunsModal isOpen onClose={vi.fn()} />);

    const button = await screen.findByRole('button', { name: 'Cancelar run Revisar PR' });
    await user.click(button);

    await waitFor(() => expect(mockCancelSubAgentRun).toHaveBeenCalledWith('conv-1', 'run-1'));
    expect(announceSpy).toHaveBeenCalledWith('subAgentRuns.announce.cancelled:Revisar PR');
    // A lista é recarregada: quem manda no estado do run é o backend.
    expect(mockListSubAgentRuns).toHaveBeenCalledTimes(2);
  });

  // O contrato do cancelamento tem um no-op: o run pode ter terminado entre a
  // renderização da lista e o clique. Dizer "cancelado" nesse caso seria mentir.
  it('distingue o no-op do cancelamento efetivo no aviso', async () => {
    mockCancelSubAgentRun.mockResolvedValue({
      conversation_id: 'conv-1',
      run_id: 'run-1',
      status: 'succeeded',
      cancelled: false,
    });
    const user = userEvent.setup();
    render(<SubAgentRunsModal isOpen onClose={vi.fn()} />);

    await user.click(await screen.findByRole('button', { name: 'Cancelar run Revisar PR' }));

    await waitFor(() => expect(announceSpy).toHaveBeenCalled());
    expect(announceSpy).toHaveBeenCalledWith('subAgentRuns.announce.cancelNoop:Revisar PR');
  });

  it('não oferece cancelar em run já terminal', async () => {
    mockListSubAgentRuns.mockResolvedValue(
      listResult([run({ status: 'succeeded', active: false })]),
    );
    render(<SubAgentRunsModal isOpen onClose={vi.fn()} />);

    expect(await screen.findByText('Revisar PR')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Cancelar run Revisar PR' })).toBeNull();
  });

  it('mostra erro de carga em vez de lista vazia', async () => {
    mockListSubAgentRuns.mockRejectedValue(new Error('sem sessão'));
    render(<SubAgentRunsModal isOpen onClose={vi.fn()} />);

    expect(await screen.findByText('subAgentRuns.loadFailed')).toBeTruthy();
    expect(screen.queryByText('Nenhum run de sub-agente registrado.')).toBeNull();
  });

  it('mostra estado vazio quando não há runs', async () => {
    mockListSubAgentRuns.mockResolvedValue(listResult([]));
    render(<SubAgentRunsModal isOpen onClose={vi.fn()} />);

    expect(await screen.findByText('Nenhum run de sub-agente registrado.')).toBeTruthy();
  });

  it('não tem violações de acessibilidade', async () => {
    render(<SubAgentRunsModal isOpen onClose={vi.fn()} />);
    await screen.findByText('Revisar PR');

    const dialog = screen.getByRole('dialog');
    expect(await axe(dialog)).toHaveNoViolations();
  });
});
