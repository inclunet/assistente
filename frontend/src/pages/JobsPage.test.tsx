import { useSyncExternalStore, type ReactNode } from 'react';
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe } from 'vitest-axe';
import type { jobs } from '@wailsjs/go/models';

const confirmMock = vi.hoisted(() => vi.fn(() => Promise.resolve(true)));
const announceMock = vi.hoisted(() => vi.fn());
const addToastMock = vi.hoisted(() => vi.fn());
const restoreDefaultFocusMock = vi.hoisted(() => vi.fn(() => {
  const target = document.querySelector<HTMLElement>('.jobs-empty-cta, .jobs-page [role="toolbar"] button:not([disabled])');
  target?.focus();
  return Boolean(target);
}));

type TestStore = {
  jobs: jobs.JobInfo[];
  isLoading: boolean;
  runLogs: jobs.RunLog[];
  events: jobs.EventEntry[];
  jobDetail: jobs.Job | null;
  fetchJobs: ReturnType<typeof vi.fn>;
  toggleJob: ReturnType<typeof vi.fn>;
  runJob: ReturnType<typeof vi.fn>;
  fetchJobRuns: ReturnType<typeof vi.fn>;
  fetchJobEvents: ReturnType<typeof vi.fn>;
  fetchJobDetail: ReturnType<typeof vi.fn>;
  deleteJob: ReturnType<typeof vi.fn>;
};

let storeState: TestStore;
const storeListeners = new Set<() => void>();

function replaceStore(patch: Partial<TestStore>) {
  storeState = { ...storeState, ...patch };
  storeListeners.forEach((listener) => listener());
}

vi.mock('../store/jobStore', () => {
  const useJobStore = (<T,>(selector?: (state: TestStore) => T) => {
    const select = selector ?? ((state: TestStore) => state as unknown as T);
    return useSyncExternalStore(
      (listener) => {
        storeListeners.add(listener);
        return () => storeListeners.delete(listener);
      },
      () => select(storeState),
      () => select(storeState),
    );
  }) as {
    <T>(selector?: (state: TestStore) => T): T;
    getState: () => TestStore;
  };
  useJobStore.getState = () => storeState;
  return { useJobStore };
});

vi.mock('../hooks/useConfirm', () => ({
  useConfirm: () => confirmMock,
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceMock }),
}));

vi.mock('../hooks/useDefaultFocus', () => ({
  restoreDefaultFocus: restoreDefaultFocusMock,
}));

vi.mock('../hooks/useGridPageLandmarks', () => ({
  useGridPageLandmarks: vi.fn(),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: (selector: (state: { addToast: typeof addToastMock }) => unknown) =>
    selector({ addToast: addToastMock }),
}));

vi.mock('@wailsjs/go/wailsapi/Jobs', () => ({
  ReplayRun: vi.fn(),
  RunJob: vi.fn(),
}));

vi.mock('../components/jobs/RunLogViewer', () => ({
  RunLogViewer: () => <div>logs</div>,
}));

vi.mock('../components/jobs/EventTimeline', () => ({
  EventTimeline: () => <div>events</div>,
}));

vi.mock('../components/jobs/builder', () => ({
  JobBuilder: ({ onClose }: { onClose: () => void }) => (
    <button type="button" onClick={onClose}>fechar-builder</button>
  ),
}));

vi.mock('../components/ui/Modal', () => ({
  Modal: ({
    isOpen,
    onClose,
    title,
    children,
  }: {
    isOpen: boolean;
    onClose: () => void;
    title: string;
    children: ReactNode;
  }) => isOpen ? (
    <div role="dialog" aria-label={title}>
      {children}
      <button type="button" onClick={onClose}>fechar-modal</button>
    </div>
  ) : null,
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: string | { defaultValue?: string; name?: string }) => {
      if (typeof options === 'string') return options;
      return options?.defaultValue ?? key;
    },
  }),
}));

const initialJobs = [
  { id: 'a', name: 'Alpha', tool: 'tool.alpha', pipeline: '', enabled: true, effective_enabled: true, status: 'idle', triggers: [] },
  { id: 'b', name: 'Bravo', tool: 'tool.bravo', pipeline: '', enabled: true, effective_enabled: true, status: 'idle', triggers: [] },
  { id: 'c', name: 'Charlie', tool: 'tool.charlie', pipeline: '', enabled: false, effective_enabled: false, status: 'idle', triggers: [] },
] as unknown as jobs.JobInfo[];

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

beforeEach(() => {
  confirmMock.mockReset();
  confirmMock.mockResolvedValue(true);
  announceMock.mockClear();
  addToastMock.mockClear();
  restoreDefaultFocusMock.mockClear();

  const fetchJobs = vi.fn(async () => {});
  const toggleJob = vi.fn(async (id: string, enabled: boolean) => {
    replaceStore({
      jobs: storeState.jobs.map((job) => job.id === id ? { ...job, enabled, effective_enabled: enabled } as jobs.JobInfo : job),
    });
  });
  const runJob = vi.fn(async () => ({ status: 'completed' }));
  const deleteJob = vi.fn(async (id: string) => {
    replaceStore({ jobs: storeState.jobs.filter((job) => job.id !== id) });
  });
  storeState = {
    jobs: initialJobs.map((job) => ({ ...job })) as jobs.JobInfo[],
    isLoading: false,
    runLogs: [],
    events: [],
    jobDetail: null,
    fetchJobs,
    toggleJob,
    runJob,
    fetchJobRuns: vi.fn(async () => {}),
    fetchJobEvents: vi.fn(async () => {}),
    fetchJobDetail: vi.fn(async () => {}),
    deleteJob,
  };
});

async function focusRow(rowIndex: number) {
  const grid = screen.getByRole('grid');
  grid.focus();
  for (let index = 0; index < rowIndex; index += 1) {
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
  }
  await waitFor(() => expect(getFirstCell(initialJobs[rowIndex].name)).toHaveFocus());
  return grid;
}

function getFirstCell(jobName: string): HTMLElement {
  const row = screen.getByText(jobName).closest('[role="row"]');
  const cell = row?.querySelector<HTMLElement>('[role="gridcell"]');
  if (!cell) throw new Error(`Célula não encontrada para ${jobName}`);
  return cell;
}

import JobsPage from './JobsPage';

describe('JobsPage', () => {
  it('não apresenta violações básicas de acessibilidade', async () => {
    const { container } = render(<JobsPage />);
    await screen.findByRole('grid');
    expect(await axe(container, {
      rules: { 'color-contrast': { enabled: false } },
    })).toHaveNoViolations();
  });

  it('Delete no grid confirma e exclui somente o job focado', async () => {
    render(<JobsPage />);
    const grid = await focusRow(1);

    fireEvent.keyDown(grid, { key: 'Delete' });

    await waitFor(() => expect(storeState.deleteJob).toHaveBeenCalledWith('b'));
    expect(confirmMock).toHaveBeenCalledOnce();
    await waitFor(() => expect(getFirstCell('Charlie')).toHaveFocus());
  });

  it('cancelar Delete mantém job e foco originais', async () => {
    confirmMock.mockResolvedValue(false);
    render(<JobsPage />);
    const grid = await focusRow(1);

    fireEvent.keyDown(grid, { key: 'Delete' });

    await waitFor(() => expect(confirmMock).toHaveBeenCalledOnce());
    expect(storeState.deleteJob).not.toHaveBeenCalled();
    expect(getFirstCell('Bravo')).toHaveFocus();
  });

  it('após toggle e run da toolbar retorna à célula válida', async () => {
    const user = userEvent.setup();
    render(<JobsPage />);
    await focusRow(1);
    const toolbar = screen.getByRole('toolbar');

    await user.click(within(toolbar).getByRole('button', { name: 'jobs.disable' }));
    await waitFor(() => expect(getFirstCell('Bravo')).toHaveFocus());

    await user.click(within(toolbar).getByRole('button', { name: 'jobs.run' }));
    await waitFor(() => expect(getFirstCell('Bravo')).toHaveFocus());
  });

  it('ativa ou desativa o job com Espaço na célula de estado', async () => {
    render(<JobsPage />);
    const grid = await focusRow(1);

    fireEvent.keyDown(grid, { key: ' ' });

    await waitFor(() => expect(storeState.toggleJob).toHaveBeenCalledWith('b', false));
    expect(getFirstCell('Bravo')).toHaveFocus();
  });

  it('abre o menu de ações com Enter na última coluna', async () => {
    render(<JobsPage />);
    const grid = await focusRow(0);
    fireEvent.keyDown(grid, { key: 'End' });

    fireEvent.keyDown(grid, { key: 'Enter' });

    expect(await screen.findByRole('menu')).toBeInTheDocument();
  });

  it('fechar modal aberto pelo grid restaura a célula do job', async () => {
    const user = userEvent.setup();
    render(<JobsPage />);
    const grid = await focusRow(1);
    fireEvent.keyDown(grid, { key: 'ArrowRight' });
    fireEvent.keyDown(grid, { key: 'Enter' });

    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'fechar-modal' }));

    await waitFor(() => expect(screen.getByText('Bravo').closest('[role="gridcell"]')).toHaveFocus());
  });

  it('ao excluir o único job retorna ao controle padrão utilizável', async () => {
    replaceStore({ jobs: [initialJobs[0]] });
    render(<JobsPage />);
    const grid = await focusRow(0);

    fireEvent.keyDown(grid, { key: 'Delete' });

    await waitFor(() => expect(restoreDefaultFocusMock).toHaveBeenCalled());
    expect(document.activeElement).toHaveAccessibleName('jobs.builder.newJob');
    expect(document.activeElement).not.toBe(document.body);
  });

  it('filtro remove referência focada sem roubar foco da busca', async () => {
    const user = userEvent.setup();
    render(<JobsPage />);
    await focusRow(1);
    const search = screen.getByPlaceholderText('jobs.search');

    await user.type(search, 'sem resultado');

    expect(search).toHaveFocus();
    expect(within(screen.getByRole('toolbar')).getByRole('button', { name: 'jobs.run' })).toBeDisabled();
    expect(screen.getByText('jobs.noSearchResults')).toBeInTheDocument();
  });

  it('não restaura foco de ação assíncrona depois do unmount', async () => {
    let resolveRun: ((value: { status: string }) => void) | undefined;
    storeState.runJob.mockImplementation(() => new Promise((resolve) => {
      resolveRun = resolve;
    }));
    const { unmount } = render(<JobsPage />);
    await focusRow(0);
    fireEvent.click(within(screen.getByRole('toolbar')).getByRole('button', { name: 'jobs.run' }));

    unmount();
    await act(async () => {
      resolveRun?.({ status: 'completed' });
      await Promise.resolve();
    });
    await new Promise((resolve) => requestAnimationFrame(resolve));

    expect(restoreDefaultFocusMock).not.toHaveBeenCalled();
  });
});
