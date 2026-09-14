import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe } from 'vitest-axe';
import type { jobs } from '@wailsjs/go/models';
import { RunLogViewer } from './RunLogViewer';

const announce = vi.hoisted(() => vi.fn());

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce }),
}));

vi.mock('../ui/DataGrid', () => ({
  DataGrid: ({
    items, onActivate,
  }: {
    items: jobs.RunLog[];
    onActivate: (item: jobs.RunLog) => void;
  }) => items.map((item) => (
    <button key={item.run_id} type="button" onClick={() => onActivate(item)}>
      abrir run {item.run_id}
    </button>
  )),
}));

vi.mock('./builder/OutputExplorer', () => ({
  OutputExplorer: ({ data }: { data: unknown }) => <pre>{JSON.stringify(data)}</pre>,
}));

const run = {
  run_id: 'run-1',
  job_id: 'job-1',
  trigger: { type: 'manual', at: '2026-09-14T00:00:00Z' },
  status: 'completed',
  started_at: '2026-09-14T00:00:00Z',
  replayable: false,
} as jobs.RunLog;

const detail = {
  ...run,
  tool_name: 'read_file',
  resolved_inputs: { path: 'documento.md' },
  output: { content: 'ok' },
  replayable: true,
  run_events: [],
  domain_events: [],
} as unknown as jobs.RunDetail;

const secondRun = { ...run, run_id: 'run-2' } as jobs.RunLog;
const secondDetail = {
  ...detail,
  run_id: 'run-2',
  tool_name: 'web_search',
} as unknown as jobs.RunDetail;

describe('RunLogViewer', () => {
  it('carrega detalhes do ledger somente ao abrir o run', async () => {
    const user = userEvent.setup();
    const onLoadDetail = vi.fn().mockResolvedValue(detail);

    render(<RunLogViewer logs={[run]} onLoadDetail={onLoadDetail} />);
    expect(onLoadDetail).not.toHaveBeenCalled();
    expect(screen.queryByText('read_file')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'abrir run run-1' }));

    expect(onLoadDetail).toHaveBeenCalledWith('job-1', 'run-1');
    expect(await screen.findByText('read_file')).toBeInTheDocument();
    expect(screen.getByText('{"path":"documento.md"}')).toBeInTheDocument();
  });

  it('não introduz violações axe após carregar o detalhe', async () => {
    const user = userEvent.setup();
    const { container } = render(
      <RunLogViewer logs={[run]} onLoadDetail={vi.fn().mockResolvedValue(detail)} />,
    );
    await user.click(screen.getByRole('button', { name: 'abrir run run-1' }));
    await screen.findByText('read_file');

    expect(await axe(container)).toHaveNoViolations();
  });

  it('descarta resposta obsoleta ao abrir outro run', async () => {
    const user = userEvent.setup();
    let resolveFirst: (value: jobs.RunDetail) => void = () => undefined;
    const first = new Promise<jobs.RunDetail>((resolve) => {
      resolveFirst = resolve;
    });
    const onLoadDetail = vi.fn()
      .mockReturnValueOnce(first)
      .mockResolvedValueOnce(secondDetail);

    render(<RunLogViewer logs={[run, secondRun]} onLoadDetail={onLoadDetail} />);
    await user.click(screen.getByRole('button', { name: 'abrir run run-1' }));
    await user.click(screen.getByRole('button', { name: 'abrir run run-2' }));
    resolveFirst(detail);

    expect(await screen.findByText('web_search')).toBeInTheDocument();
    expect(screen.queryByText('read_file')).not.toBeInTheDocument();
  });

  it('oculta o detalhe anterior enquanto outro run carrega', async () => {
    const user = userEvent.setup();
    const pending = new Promise<jobs.RunDetail>(() => undefined);
    const onLoadDetail = vi.fn()
      .mockResolvedValueOnce(detail)
      .mockReturnValueOnce(pending);

    render(<RunLogViewer logs={[run, secondRun]} onLoadDetail={onLoadDetail} />);
    await user.click(screen.getByRole('button', { name: 'abrir run run-1' }));
    expect(await screen.findByText('read_file')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'abrir run run-2' }));

    expect(screen.queryByText('read_file')).not.toBeInTheDocument();
    expect(screen.getByText('common.loading')).toBeInTheDocument();
  });

  it('exibe e anuncia falha ao carregar detalhe', async () => {
    const user = userEvent.setup();
    announce.mockClear();
    render(<RunLogViewer logs={[run]} onLoadDetail={vi.fn().mockResolvedValue(null)} />);

    await user.click(screen.getByRole('button', { name: 'abrir run run-1' }));

    expect(await screen.findByText('jobs.runDetailLoadError')).toBeInTheDocument();
    expect(announce).toHaveBeenCalledWith('jobs.runDetailLoadError', 'assertive');
  });
});

