import { useState, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { jobs } from '@wailsjs/go/models';
import { DataGrid, DataGridColumn } from '../ui/DataGrid';
import { Button } from '../ui/Button';
import { OutputExplorer } from './builder/OutputExplorer';
import './RunLogViewer.css';

interface RunLogViewerProps {
  logs: jobs.RunLog[];
  isLoading?: boolean;
  onReplay?: (run: jobs.RunLog) => Promise<jobs.TestToolResult | null>;
  onRerun?: (jobId: string) => void;
}

function formatTriggerType(type: string, t: (key: string) => string): string {
  const map: Record<string, string> = {
    cron: t('jobs.triggerCron'),
    interval: t('jobs.triggerInterval'),
    event: t('jobs.triggerEvent'),
    hotkey: t('jobs.triggerHotkey'),
    manual: t('jobs.triggerManual'),
  };
  return map[type] || type;
}

function formatStatus(status: string, t: (key: string) => string): string {
  const map: Record<string, string> = {
    completed: t('jobs.completed'),
    failed: t('jobs.failed'),
    skipped: t('jobs.skipped'),
    retrying: t('jobs.retrying'),
  };
  return map[status] || status;
}

function DataBlock({ data, label }: { data: Record<string, unknown> | undefined | null; label: string }) {
  if (!data || Object.keys(data).length === 0) {
    return <span className="run-detail__empty">{'\u2014'}</span>;
  }
  return (
    <div className="run-detail__explorer" aria-label={label}>
      <OutputExplorer data={data} />
    </div>
  );
}

function JsonPre({ data, label }: { data: unknown; label: string }) {
  if (data == null || (typeof data === 'object' && Object.keys(data as object).length === 0)) {
    return <span className="run-detail__empty">{'\u2014'}</span>;
  }
  return (
    <pre className="run-detail__json" tabIndex={0} aria-label={label}>
      {JSON.stringify(data, null, 2)}
    </pre>
  );
}

function RunDetail({
  run, onReplay, onRerun,
}: {
  run: jobs.RunLog;
  onReplay?: (run: jobs.RunLog) => Promise<jobs.TestToolResult | null>;
  onRerun?: (jobId: string) => void;
}) {
  const { t } = useTranslation();
  const [replaying, setReplaying] = useState(false);
  const [replayResult, setReplayResult] = useState<jobs.TestToolResult | null>(null);

  const handleReplay = useCallback(async () => {
    if (!onReplay) return;
    setReplaying(true);
    setReplayResult(null);
    try {
      const result = await onReplay(run);
      setReplayResult(result);
    } finally {
      setReplaying(false);
    }
  }, [onReplay, run]);

  const canReplay = Boolean(run.replayable && run.tool_name && run.resolved_inputs && onReplay);

  return (
    <section className="run-detail" aria-label={t('jobs.runDetailLabel', 'Detalhes da execução')}>
      {run.tool_name && (
        <div className="run-detail__row">
          <h4 className="run-detail__label">{t('jobs.logTool')}</h4>
          <code className="run-detail__tool-name">{run.tool_name}</code>
        </div>
      )}

      <div className="run-detail__grid">
        <div className="run-detail__col">
          <h4 className="run-detail__label">{t('jobs.logInputs')}</h4>
          <DataBlock data={run.resolved_inputs} label={t('jobs.logInputs')} />
        </div>
        <div className="run-detail__col">
          <h4 className="run-detail__label">{t('jobs.logOutput')}</h4>
          <DataBlock data={run.output} label={t('jobs.logOutput')} />
        </div>
      </div>

      {run.error && (
        <div className="run-detail__row">
          <h4 className="run-detail__label">{t('jobs.logError')}</h4>
          <pre className="run-detail__error-full" tabIndex={0} aria-label={t('jobs.logError')}>
            {run.error}
          </pre>
        </div>
      )}

      {run.events_emitted && run.events_emitted.length > 0 && (
        <div className="run-detail__row">
          <h4 className="run-detail__label">{t('jobs.logEventsEmitted')}</h4>
          <span>{run.events_emitted.join(', ')}</span>
        </div>
      )}

      {replayResult && (
        <div
          className={`run-detail__replay-result ${replayResult.success ? 'run-detail__replay-result--ok' : 'run-detail__replay-result--err'}`}
          role="status"
          aria-live="polite"
        >
          <h4 className="run-detail__label">
            {replayResult.success ? t('jobs.replaySuccess') : t('jobs.replayFailed')}
          </h4>
          {replayResult.duration && <span className="run-detail__replay-duration">({replayResult.duration})</span>}
          <JsonPre data={replayResult.success ? replayResult.output : undefined} label="Replay output" />
          {replayResult.error && <pre className="run-detail__error-full" tabIndex={0}>{replayResult.error}</pre>}
        </div>
      )}

      <div className="run-detail__actions">
        {canReplay && (
          <Button size="sm" variant="outline" onClick={handleReplay} loading={replaying} disabled={replaying}>
            {t('jobs.replayExact')}
          </Button>
        )}
        {onRerun && (
          <Button size="sm" variant="ghost" onClick={() => onRerun(run.job_id)}>
            {t('jobs.rerunJob')}
          </Button>
        )}
      </div>
    </section>
  );
}

export function RunLogViewer({ logs, isLoading, onReplay, onRerun }: RunLogViewerProps) {
  const { t } = useTranslation();
  const [selectedRun, setSelectedRun] = useState<jobs.RunLog | null>(null);

  const getItemId = useCallback((log: jobs.RunLog) => log.run_id, []);

  const columns: DataGridColumn<jobs.RunLog>[] = useMemo(() => [
    {
      key: 'status',
      label: t('jobs.logStatus'),
      width: '100px',
      format: (_, item) => formatStatus((item as jobs.RunLog).status, t),
    },
    {
      key: 'trigger',
      label: t('jobs.logTrigger'),
      width: '90px',
      format: (_, item) => formatTriggerType((item as jobs.RunLog).trigger?.type || '', t),
    },
    {
      key: 'started_at',
      label: t('jobs.logStarted'),
      width: '170px',
      format: (val) => val ? new Date(val as string).toLocaleString() : '\u2014',
    },
    {
      key: 'duration',
      label: t('jobs.logDuration'),
      width: '90px',
      format: (val) => (val as string) || '\u2014',
    },
    {
      key: 'error',
      label: t('jobs.logError'),
      truncate: true,
      format: (val) => (val as string) || '\u2014',
    },
  ], [t]);

  const handleActivate = useCallback((item: jobs.RunLog) => {
    setSelectedRun((prev) => (prev?.run_id === item.run_id ? null : item));
  }, []);

  const handleFocusChange = useCallback((item: jobs.RunLog | null) => {
    if (item) {
      setSelectedRun(item);
    }
  }, []);

  if (isLoading) {
    return <div className="run-log-viewer run-log-viewer--loading" role="status">{t('common.loading', 'Loading...')}</div>;
  }

  if (!logs || logs.length === 0) {
    return <div className="run-log-viewer run-log-viewer--empty" role="status">{t('jobs.logsEmpty')}</div>;
  }

  return (
    <div className="run-log-viewer">
      <DataGrid<jobs.RunLog>
        items={logs}
        columns={columns}
        label={t('jobs.logsTitle')}
        getItemId={getItemId}
        onActivate={handleActivate}
        onFocusChange={handleFocusChange}
        autoFocusOnMount={false}
      />

      {selectedRun && (
        <RunDetail run={selectedRun} onReplay={onReplay} onRerun={onRerun} />
      )}
    </div>
  );
}
