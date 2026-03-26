import { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useJobStore } from '../store/jobStore';
import { Modal } from '../components/ui/Modal';
import { Toolbar } from '../components/ui/Toolbar';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { MenuButton } from '../components/layout/MenuButton';
import { RunLogViewer } from '../components/jobs/RunLogViewer';
import { EventTimeline } from '../components/jobs/EventTimeline';
import { JobBuilder } from '../components/jobs/builder';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { useGridFocus } from '../hooks/useGridFocus';
import { useUIStore } from '../store/uiStore';
import { jobs } from '@wailsjs/go/models';
import { ReplayRun, RunJob as WailsRunJob } from '@wailsjs/go/main/App';
import './JobsPage.css';

function formatTriggers(triggers: jobs.Trigger[] | undefined, t: (key: string) => string): string {
  if (!triggers || triggers.length === 0) return '—';
  return triggers
    .map((tr) => {
      const typeKey = `jobs.trigger${(tr.type as string).charAt(0).toUpperCase() + (tr.type as string).slice(1)}` as const;
      const label = t(typeKey);
      if (tr.type === 'cron') return `${label}: ${tr.expression}`;
      if (tr.type === 'interval') return `${label}: ${tr.every}`;
      if (tr.type === 'event') return `${label}: ${tr.listen}`;
      if (tr.type === 'hotkey') return `${label}: ${tr.keys}`;
      return label;
    })
    .join(', ');
}

function statusBadge(status: string, t: (key: string) => string): { label: string; className: string } {
  switch (status) {
    case 'running':
      return { label: t('jobs.statusRunning'), className: 'job-status--running' };
    case 'error':
      return { label: t('jobs.statusError'), className: 'job-status--error' };
    default:
      return { label: t('jobs.statusIdle'), className: 'job-status--idle' };
  }
}

export default function JobsPage() {
  const { t } = useTranslation();
  const { addToast } = useUIStore();
  const { announce } = useAnnouncer();
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'jobs-page' });

  const jobsList = useJobStore((s) => s.jobs);
  const isLoading = useJobStore((s) => s.isLoading);
  const runLogs = useJobStore((s) => s.runLogs);
  const events = useJobStore((s) => s.events);
  const { fetchJobs, toggleJob, runJob, fetchJobRuns, fetchJobEvents, fetchJobDetail, deleteJob } = useJobStore();

  const [searchTerm, setSearchTerm] = useState('');
  const [focusedJob, setFocusedJob] = useState<jobs.JobInfo | null>(null);
  const [logsModalOpen, setLogsModalOpen] = useState(false);
  const [logsJobId, setLogsJobId] = useState<string | null>(null);
  const [eventsModalOpen, setEventsModalOpen] = useState(false);
  const [runningJobId, setRunningJobId] = useState<string | null>(null);
  const [builderOpen, setBuilderOpen] = useState(false);
  const [editingJob, setEditingJob] = useState<jobs.Job | null>(null);

  const toolbarRef = useRef<HTMLDivElement>(null);
  const gridRef = useRef<HTMLDivElement>(null);

  const getRowId = useCallback((item: jobs.JobInfo) => item.id, []);
  const handleFocusChange = useCallback((item: jobs.JobInfo | null) => setFocusedJob(item), []);

  const loadedRef = useRef(false);
  useEffect(() => {
    if (loadedRef.current) return;
    loadedRef.current = true;
    void fetchJobs();
  }, [fetchJobs]);

  const filteredJobs = useMemo(
    () =>
      jobsList.filter(
        (job) =>
          job.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
          job.id.toLowerCase().includes(searchTerm.toLowerCase()) ||
          job.tool.toLowerCase().includes(searchTerm.toLowerCase()) ||
          (job.pipeline || '').toLowerCase().includes(searchTerm.toLowerCase())
      ),
    [jobsList, searchTerm]
  );

  const handleToggle = useCallback(
    async (job: jobs.JobInfo) => {
      try {
        await toggleJob(job.id, !job.enabled);
        addToast(t('jobs.toggleSuccess'), 'success');
        announce(t('jobs.toggleSuccess'));
      } catch {
        addToast(t('common.error', 'Error'), 'error');
      }
    },
    [toggleJob, addToast, announce, t]
  );

  const handleRun = useCallback(
    async (job: jobs.JobInfo) => {
      setRunningJobId(job.id);
      try {
        const result = await runJob(job.id);
        if (result && result.status === 'completed') {
          addToast(t('jobs.runSuccess'), 'success');
          announce(t('jobs.runSuccess'));
        } else {
          addToast(t('jobs.runFailed'), 'error');
          announce(t('jobs.runFailed'));
        }
      } catch {
        addToast(t('jobs.runFailed'), 'error');
      } finally {
        setRunningJobId(null);
      }
    },
    [runJob, addToast, announce, t]
  );

  const handleViewLogs = useCallback(
    async (jobId: string) => {
      setLogsJobId(jobId);
      setLogsModalOpen(true);
      await fetchJobRuns(jobId);
    },
    [fetchJobRuns]
  );

  const handleViewEvents = useCallback(async () => {
    setEventsModalOpen(true);
    const today = new Date().toISOString().slice(0, 10);
    await fetchJobEvents(today);
  }, [fetchJobEvents]);

  const handleReplay = useCallback(
    async (run: jobs.RunLog): Promise<jobs.TestToolResult | null> => {
      try {
        const result = await ReplayRun(run.job_id, run.run_id);
        return result;
      } catch (err) {
        addToast(String(err), 'error');
        return null;
      }
    },
    [addToast]
  );

  const handleRerun = useCallback(
    async (jobId: string) => {
      try {
        await WailsRunJob(jobId);
        addToast(t('jobs.runSuccess'), 'success');
        await fetchJobRuns(jobId);
      } catch (err) {
        addToast(String(err), 'error');
      }
    },
    [addToast, t, fetchJobRuns]
  );

  const handleNewJob = useCallback(() => {
    setEditingJob(null);
    setBuilderOpen(true);
  }, []);

  const handleEditJob = useCallback(async (jobId: string) => {
    try {
      await fetchJobDetail(jobId);
      const detail = useJobStore.getState().jobDetail;
      setEditingJob(detail);
      setBuilderOpen(true);
    } catch {
      addToast(t('common.error'), 'error');
    }
  }, [fetchJobDetail, addToast, t]);

  const handleDeleteJob = useCallback(async (job: jobs.JobInfo) => {
    if (!window.confirm(t('jobs.builder.deleteConfirm', { name: job.name || job.id }))) return;
    try {
      await deleteJob(job.id);
      addToast(t('jobs.builder.deleteSuccess'), 'success');
      announce(t('jobs.builder.deleteSuccess'));
    } catch {
      addToast(t('common.error'), 'error');
    }
  }, [deleteJob, addToast, announce, t]);

  const getJobRowActions = useCallback(
    (job: jobs.JobInfo) => [
      {
        id: 'run',
        label: runningJobId === job.id ? t('jobs.running') : t('jobs.run'),
        icon: '▶',
        onClick: () => handleRun(job),
      },
      {
        id: 'toggle',
        label: job.enabled ? t('jobs.disable') : t('jobs.enable'),
        icon: job.enabled ? '⏸' : '⏵',
        onClick: () => handleToggle(job),
      },
      {
        id: 'logs',
        label: t('jobs.viewLogs'),
        icon: '📋',
        onClick: () => handleViewLogs(job.id),
      },
      {
        id: 'edit',
        label: t('common.edit'),
        icon: '✏️',
        onClick: () => handleEditJob(job.id),
      },
      {
        id: 'events',
        label: t('jobs.viewEvents'),
        icon: '📡',
        onClick: () => handleViewEvents(),
      },
      {
        id: 'delete',
        label: t('common.delete'),
        icon: '🗑',
        onClick: () => handleDeleteJob(job),
      },
    ],
    [t, handleRun, handleToggle, handleViewLogs, handleEditJob, handleViewEvents, handleDeleteJob, runningJobId]
  );

  const columns: DataGridColumn<jobs.JobInfo>[] = useMemo(
    () => [
      {
        key: 'enabled' as keyof jobs.JobInfo,
        label: '',
        width: '36px',
        format: (_value, item) => (
          <button
            className={`job-toggle ${(item as jobs.JobInfo).enabled ? 'job-toggle--on' : 'job-toggle--off'}`}
            onClick={(e) => { e.stopPropagation(); handleToggle(item as jobs.JobInfo); }}
            aria-label={(item as jobs.JobInfo).enabled ? t('jobs.disable') : t('jobs.enable')}
            title={(item as jobs.JobInfo).enabled ? t('jobs.disable') : t('jobs.enable')}
          >
            {(item as jobs.JobInfo).enabled ? '●' : '○'}
          </button>
        ),
      },
      {
        key: 'name' as keyof jobs.JobInfo,
        label: t('jobs.name'),
        width: '22%',
        format: (value, item) => (
          <span className="job-name-cell">
            <span className="job-name">{(value as string) || (item as jobs.JobInfo).id}</span>
            {(item as jobs.JobInfo).pipeline && (
              <span className="job-pipeline-badge">{(item as jobs.JobInfo).pipeline}</span>
            )}
          </span>
        ),
      },
      {
        key: 'tool' as keyof jobs.JobInfo,
        label: t('jobs.tool'),
        width: '25%',
        truncate: true,
      },
      {
        key: 'triggers' as keyof jobs.JobInfo,
        label: t('jobs.triggers'),
        width: '25%',
        format: (_value, item) => formatTriggers((item as jobs.JobInfo).triggers, t),
        truncate: true,
      },
      {
        key: 'status' as keyof jobs.JobInfo,
        label: t('jobs.status'),
        width: '10%',
        format: (value) => {
          const badge = statusBadge(value as string, t);
          return <span className={`job-status-badge ${badge.className}`}>{badge.label}</span>;
        },
      },
      {
        key: 'id' as keyof jobs.JobInfo,
        label: '',
        width: '5%',
        format: (_value, item) => (
          <MenuButton
            items={getJobRowActions(item as jobs.JobInfo)}
            buttonLabel={t('jobs.actions')}
          />
        ),
      },
    ],
    [t, handleToggle, getJobRowActions]
  );

  const hasJobs = filteredJobs.length > 0;

  const homeActions = [
    {
      key: 'new-job',
      label: t('jobs.builder.newJob'),
      onClick: handleNewJob,
      variant: 'primary' as const,
    },
    {
      key: 'run-job',
      label: t('jobs.run'),
      onClick: () => focusedJob && handleRun(focusedJob),
      disabled: !focusedJob || runningJobId === focusedJob?.id,
    },
    {
      key: 'toggle-job',
      label: focusedJob?.enabled ? t('jobs.disable') : t('jobs.enable'),
      onClick: () => focusedJob && handleToggle(focusedJob),
      disabled: !focusedJob,
    },
    {
      key: 'view-events',
      label: t('jobs.viewEvents'),
      onClick: () => handleViewEvents(),
    },
  ];

  return (
    <div className="jobs-page">
      <Toolbar
        ref={toolbarRef}
        left={
          <h1 className="page-toolbar__title">
            {t('jobs.allJobs')}
          </h1>
        }
        searchPlaceholder={t('jobs.search')}
        searchValue={searchTerm}
        onSearchChange={hasJobs || isLoading ? setSearchTerm : undefined}
        actions={homeActions}
      />

      {isLoading && jobsList.length === 0 ? (
        <div className="jobs-loading">{t('common.loading', 'Loading...')}</div>
      ) : hasJobs ? (
        <div ref={gridRef}>
          <DataGrid
            items={filteredJobs}
            columns={columns}
            getItemId={getRowId}
            onActivate={(item: jobs.JobInfo) => handleViewLogs(item.id)}
            getRowActions={getJobRowActions}
            onFocusChange={handleFocusChange}
            onGridReady={handleGridReady}
            label={t('jobs.gridLabel')}
          />
        </div>
      ) : (
        <div className="jobs-empty-state">
          <p className="jobs-empty-message">{t('jobs.noJobs')}</p>
          <p className="jobs-empty-hint">{t('jobs.builder.noJobsHintBuilder')}</p>
          <button className="jobs-empty-cta" onClick={handleNewJob}>
            {t('jobs.builder.newJob')}
          </button>
        </div>
      )}

      <Modal
        isOpen={logsModalOpen}
        onClose={() => setLogsModalOpen(false)}
        title={`${t('jobs.logsTitle')} — ${logsJobId || ''}`}
      >
        <RunLogViewer logs={runLogs} onReplay={handleReplay} onRerun={handleRerun} />
      </Modal>

      <Modal
        isOpen={eventsModalOpen}
        onClose={() => setEventsModalOpen(false)}
        title={t('jobs.eventsTitle')}
      >
        <EventTimeline events={events} />
      </Modal>

      <Modal
        isOpen={builderOpen}
        onClose={() => setBuilderOpen(false)}
        title={editingJob ? t('jobs.builder.editJob') : t('jobs.builder.newJob')}
        size="lg"
      >
        <JobBuilder
          editJob={editingJob}
          onClose={() => setBuilderOpen(false)}
          onSaved={() => fetchJobs()}
        />
      </Modal>
    </div>
  );
}
